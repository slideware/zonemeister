package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"zonemeister/internal/auth"
	"zonemeister/internal/config"
	"zonemeister/internal/dbmigrate"
	"zonemeister/internal/handler"
	"zonemeister/internal/mail"
	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
	"zonemeister/internal/netnod"
	"zonemeister/internal/repository"
	"zonemeister/internal/repository/postgres"
	"zonemeister/internal/repository/sqlite"
	"zonemeister/internal/templates"
	"zonemeister/migrations"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Load config.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Set up slog level.
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	// Open database and create repositories based on driver.
	var (
		db             *sql.DB
		userRepo       repository.UserRepository
		sessionRepo    repository.SessionRepository
		zoneAssignRepo repository.ZoneAssignmentRepository
		customerRepo   repository.CustomerRepository
		tsigKeyRepo    repository.CustomerTSIGKeyRepository
	)

	switch cfg.DBDriver {
	case "postgres":
		db, err = postgres.OpenDB(cfg.DBURL)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		if err := dbmigrate.RunMigrations(db, migrations.PostgresFS(), dbmigrate.PostgresPlaceholder); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}

		userRepo = postgres.NewUserRepo(db)
		sessionRepo = postgres.NewSessionRepo(db)
		zoneAssignRepo = postgres.NewZoneAssignmentRepo(db)
		customerRepo = postgres.NewCustomerRepo(db)
		tsigKeyRepo = postgres.NewCustomerTSIGKeyRepo(db)

	default: // "sqlite"
		// Ensure data directory exists.
		if dir := filepath.Dir(cfg.DBPath); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
		}

		db, err = sqlite.OpenDB(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		if err := sqlite.RunMigrations(db, migrations.SQLiteFS()); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}

		userRepo = sqlite.NewUserRepo(db)
		sessionRepo = sqlite.NewSessionRepo(db)
		zoneAssignRepo = sqlite.NewZoneAssignmentRepo(db)
		customerRepo = sqlite.NewCustomerRepo(db)
		tsigKeyRepo = sqlite.NewCustomerTSIGKeyRepo(db)
	}

	// Seed superadmin if none exists.
	if err := seedSuperAdmin(userRepo); err != nil {
		return fmt.Errorf("seed superadmin: %w", err)
	}

	// Initialize flash cookie signing.
	middleware.SetFlashSecret(cfg.SessionSecret)

	// Create session manager.
	sessionMgr := auth.NewSessionManager(sessionRepo, cfg.SecureCookies)

	// Create Netnod API clients.
	netnodClient := netnod.NewClient(cfg.NetnodAPIURL, cfg.NetnodAPIToken)
	tsigClient := netnod.NewTSIGClient(cfg.NetnodNDSAPIURL, cfg.NetnodAPIToken)

	// Create template renderer.
	renderer, err := templates.New("templates", "static")
	if err != nil {
		return fmt.Errorf("create renderer: %w", err)
	}

	// Create account lockout tracker.
	lockout := auth.NewLockout(5, 15*time.Minute)

	// Set up optional SMTP mailer for password reset emails.
	var mailer *mail.Mailer
	if cfg.SMTPHost != "" {
		mailer = mail.NewMailer(mail.SMTPConfig{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			User:     cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		})
		renderer.SetMailEnabled(true)
		slog.Info("email enabled", "host", cfg.SMTPHost, "port", cfg.SMTPPort)
	}

	// Determine base URL for links in emails.
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.ServerHost, cfg.ServerPort)
	}

	// Create handlers.
	authHandler := handler.NewAuthHandler(userRepo, sessionMgr, renderer, lockout, cfg.SecureCookies, mailer, baseURL, cfg.SessionSecret)
	dashboardHandler := handler.NewDashboardHandler(netnodClient, zoneAssignRepo, customerRepo, renderer)
	zoneHandler := handler.NewZoneHandler(netnodClient, zoneAssignRepo, customerRepo, tsigKeyRepo, renderer)
	customerHandler := handler.NewCustomerHandler(customerRepo, userRepo, tsigClient, tsigKeyRepo, renderer)
	assignmentHandler := handler.NewAssignmentHandler(zoneAssignRepo, customerRepo, netnodClient, tsigKeyRepo, renderer)
	recordHandler := handler.NewRecordHandler(netnodClient, zoneAssignRepo, renderer)
	accountHandler := handler.NewAccountHandler(userRepo, renderer)
	dyndnsHandler := handler.NewDynDNSHandler(netnodClient, renderer)
	acmeHandler := handler.NewACMEHandler(netnodClient, renderer)

	// Set up router.
	r := chi.NewRouter()
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.NewCSRF(cfg.SecureCookies))
	r.Use(middleware.LoadUser(sessionMgr, userRepo))

	// Static files.
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Routes.
	r.Get("/login", authHandler.LoginForm)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/login", authHandler.Login)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/login/totp", authHandler.VerifyTOTP)
	r.Post("/logout", authHandler.Logout)
	r.Get("/forgot-password", authHandler.ForgotPasswordForm)
	r.With(httprate.LimitByIP(10, time.Minute)).Post("/forgot-password", authHandler.ForgotPassword)
	r.Get("/reset-password", authHandler.ResetPasswordForm)
	r.Post("/reset-password", authHandler.ResetPassword)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Get("/", dashboardHandler.Index)
		r.Get("/account", accountHandler.Show)
		r.Post("/account/password", accountHandler.ChangePassword)
		r.Post("/account/totp/setup", accountHandler.SetupTOTP)
		r.Post("/account/totp/enable", accountHandler.EnableTOTP)
		r.Post("/account/totp/disable", accountHandler.DisableTOTP)
		r.Get("/zones", zoneHandler.List)
		r.Get("/zones/new", zoneHandler.CustomerNew)
		r.Post("/zones", zoneHandler.CustomerCreate)
		r.Post("/zones/import", zoneHandler.CustomerImport)
		r.Get("/zones/{zoneId}", zoneHandler.Detail)
		r.Get("/zones/{zoneId}/records", recordHandler.RecordsPartial)
		r.Post("/zones/{zoneId}/records", recordHandler.Add)
		r.Post("/zones/{zoneId}/records/update", recordHandler.Update)
		r.Post("/zones/{zoneId}/records/delete", recordHandler.Delete)
		r.Get("/zones/{zoneId}/records/edit", recordHandler.EditForm)

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin)
			r.Get("/admin/customers", customerHandler.List)
			r.Get("/admin/customers/new", customerHandler.New)
			r.Post("/admin/customers", customerHandler.Create)
			r.Get("/admin/customers/{id}", customerHandler.Show)
			r.Get("/admin/customers/{id}/edit", customerHandler.Edit)
			r.Post("/admin/customers/{id}", customerHandler.Update)
			r.Post("/admin/customers/{id}/delete", customerHandler.Delete)
			r.Post("/admin/customers/{id}/users", customerHandler.AddUser)
			r.Post("/admin/customers/{id}/users/delete", customerHandler.DeleteUser)
			r.Post("/admin/customers/{id}/users/password", customerHandler.ResetUserPassword)
			r.Post("/admin/customers/{id}/tsig", customerHandler.AddTSIG)
			r.Post("/admin/customers/{id}/tsig/delete", customerHandler.RemoveTSIG)
			r.Get("/admin/zones/{zoneId}/assign", assignmentHandler.Show)
			r.Post("/admin/zones/{zoneId}/assign", assignmentHandler.Assign)
			r.Post("/admin/zones/{zoneId}/unassign", assignmentHandler.Unassign)
			r.Get("/admin/zones/new", zoneHandler.New)
			r.Post("/admin/zones", zoneHandler.Create)
			r.Post("/admin/zones/{zoneId}/delete", zoneHandler.Delete)
			r.Post("/admin/zones/{zoneId}/notify", zoneHandler.Notify)
			r.Get("/admin/zones/{zoneId}/export", zoneHandler.Export)
			r.Get("/admin/zones/{zoneId}/dyndns", dyndnsHandler.List)
			r.Post("/admin/zones/{zoneId}/dyndns", dyndnsHandler.Enable)
			r.Post("/admin/zones/{zoneId}/dyndns/{label}/delete", dyndnsHandler.Disable)
			r.Get("/admin/zones/{zoneId}/acme", acmeHandler.List)
			r.Post("/admin/zones/{zoneId}/acme", acmeHandler.Enable)
			r.Post("/admin/zones/{zoneId}/acme/{label}/delete", acmeHandler.Disable)
		})
	})

	// Start server.
	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, r)
}

func seedSuperAdmin(userRepo repository.UserRepository) error {
	ctx := context.Background()
	existing, err := userRepo.GetByEmail(ctx, "admin@example.com")
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	hash, err := auth.HashPassword("changeme")
	if err != nil {
		return err
	}

	user := &models.User{
		Email:        "admin@example.com",
		PasswordHash: hash,
		Role:         models.RoleSuperAdmin,
	}

	if err := userRepo.Create(ctx, user); err != nil {
		return err
	}

	slog.Warn("seeded superadmin user", "email", "admin@example.com", "password", "changeme")
	return nil
}
