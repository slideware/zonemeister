package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration.
type Config struct {
	NetnodAPIURL    string
	NetnodAPIToken  string
	NetnodNDSAPIURL string
	ServerHost     string
	ServerPort     int
	SessionSecret  string
	DBDriver       string
	DBPath         string
	DBURL          string
	LogLevel       string
	SecureCookies  bool
	SMTPHost       string
	SMTPPort       int
	SMTPUser       string
	SMTPPassword   string
	SMTPFrom       string
	BaseURL        string
}

// loadEnvFile reads a .env file and sets environment variables for any keys
// not already set in the environment. This means real env vars take precedence.
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // missing .env is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		key = strings.TrimPrefix(key, "export ")
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip surrounding quotes if present.
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		// Only set if not already present in environment.
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// Load reads configuration from environment variables with sensible defaults.
// If a .env file exists in the working directory, it is loaded first (existing
// env vars take precedence over .env values).
func Load() (*Config, error) {
	loadEnvFile(".env")

	cfg := &Config{
		NetnodAPIURL:    envOrDefault("NETNOD_API_URL", "https://primarydnsapi.netnod.se"),
		NetnodNDSAPIURL: envOrDefault("NETNOD_NDSAPI_URL", "https://dnsnodeapi.netnod.se"),
		ServerHost:   envOrDefault("SERVER_HOST", "localhost"),
		DBDriver:     envOrDefault("DB_DRIVER", "sqlite"),
		DBPath:       envOrDefault("DB_PATH", "data/netnod.db"),
		DBURL:        os.Getenv("DB_URL"),
		LogLevel:     envOrDefault("LOG_LEVEL", "info"),
	}

	portStr := envOrDefault("SERVER_PORT", "3000")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT %q: %w", portStr, err)
	}
	cfg.ServerPort = port

	cfg.NetnodAPIToken = os.Getenv("NETNOD_API_TOKEN")
	if cfg.NetnodAPIToken == "" {
		return nil, fmt.Errorf("NETNOD_API_TOKEN is required")
	}

	cfg.SessionSecret = os.Getenv("SESSION_SECRET")
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required")
	}

	cfg.SecureCookies = os.Getenv("SECURE_COOKIES") == "true"

	// SMTP configuration (optional — enables password reset via email).
	cfg.SMTPHost = os.Getenv("SMTP_HOST")
	smtpPortStr := envOrDefault("SMTP_PORT", "587")
	cfg.SMTPPort, err = strconv.Atoi(smtpPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_PORT %q: %w", smtpPortStr, err)
	}
	cfg.SMTPUser = os.Getenv("SMTP_USER")
	cfg.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	cfg.SMTPFrom = os.Getenv("SMTP_FROM")
	cfg.BaseURL = os.Getenv("BASE_URL")

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
