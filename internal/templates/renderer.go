package templates

import (
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
)

// funcMap contains custom template functions available in all templates.
var funcMap = template.FuncMap{
	"deref": func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	},
	"field": func(content string, index int) string {
		parts := strings.Fields(content)
		if index < len(parts) {
			return parts[index]
		}
		return ""
	},
	"fieldFrom": func(content string, skip int) string {
		parts := strings.Fields(content)
		if skip < len(parts) {
			return strings.Join(parts[skip:], " ")
		}
		return ""
	},
	"label": func(fqdn, zoneName string) string {
		if fqdn == zoneName {
			return "@"
		}
		suffix := "." + zoneName
		if strings.HasSuffix(fqdn, suffix) {
			return strings.TrimSuffix(fqdn, suffix)
		}
		return fqdn
	},
}

// TemplateData is the data passed to every template render.
type TemplateData struct {
	User        *models.User
	CSRFToken   string
	Flash       string
	Data        any
	PartnerCSS  bool
	MailEnabled bool
}

// Renderer manages template rendering.
type Renderer struct {
	templates   map[string]*template.Template
	partials    map[string]*template.Template
	partnerCSS  bool
	mailEnabled bool
}

// New creates a Renderer by parsing templates from the given base directory.
// staticDir is the path to the static assets directory (for loading icons and detecting partner CSS).
func New(baseDir, staticDir string) (*Renderer, error) {
	// Load SVG icons into memory for the icon template function.
	icons := make(map[string]template.HTML)
	iconDir := filepath.Join(staticDir, "img", "icons")
	entries, err := os.ReadDir(iconDir)
	if err != nil {
		slog.Warn("could not read icon directory", "path", iconDir, "error", err)
	} else {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".svg") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".svg")
			data, err := os.ReadFile(filepath.Join(iconDir, e.Name()))
			if err != nil {
				slog.Warn("could not read icon", "name", name, "error", err)
				continue
			}
			icons[name] = template.HTML(data)
		}
		slog.Info("loaded icons", "count", len(icons))
	}

	funcMap["icon"] = func(name string) template.HTML {
		if svg, ok := icons[name]; ok {
			return svg
		}
		slog.Warn("missing icon", "name", name)
		return ""
	}

	// Detect partner CSS for white-label support.
	partnerCSSPath := filepath.Join(staticDir, "css", "partner.css")
	_, partnerErr := os.Stat(partnerCSSPath)
	hasPartnerCSS := partnerErr == nil

	templates := make(map[string]*template.Template)

	pages := map[string]string{
		"login":              baseDir + "/auth/login.html",
		"dashboard-customer": baseDir + "/dashboard/customer.html",
		"dashboard-admin":    baseDir + "/dashboard/admin.html",
		"zones-list":     baseDir + "/zones/list.html",
		"zones-assign":   baseDir + "/zones/assign.html",
		"zones-detail":   baseDir + "/zones/detail.html",
		"zones-create":          baseDir + "/zones/create.html",
		"zones-customer-create": baseDir + "/zones/customer-create.html",
		"customers-list":   baseDir + "/customers/list.html",
		"customers-form":   baseDir + "/customers/form.html",
		"customers-detail": baseDir + "/customers/detail.html",
		"account":        baseDir + "/account/index.html",
		"totp-setup":     baseDir + "/account/totp_setup.html",
		"totp-verify":           baseDir + "/auth/totp.html",
		"forgot-password":       baseDir + "/auth/forgot_password.html",
		"forgot-password-sent":  baseDir + "/auth/forgot_password_sent.html",
		"reset-password":        baseDir + "/auth/reset_password.html",
	}

	layoutFiles := []string{
		baseDir + "/layouts/base.html",
		baseDir + "/partials/navbar.html",
	}

	for name, page := range pages {
		files := append([]string{page}, layoutFiles...)
		t, err := template.New(filepath.Base(page)).Funcs(funcMap).ParseFiles(files...)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		templates[name] = t
	}

	// Partials are standalone templates rendered without the base layout.
	// They are used for htmx partial responses.
	partials := make(map[string]*template.Template)

	partialDefs := map[string]string{
		"records-table": baseDir + "/records/table.html",
		"records-edit":  baseDir + "/records/edit.html",
		"dyndns-list":   baseDir + "/dyndns/list.html",
		"acme-list":     baseDir + "/acme/list.html",
	}

	for name, file := range partialDefs {
		t, err := template.New(filepath.Base(file)).Funcs(funcMap).ParseFiles(file)
		if err != nil {
			return nil, fmt.Errorf("parse partial %s: %w", name, err)
		}
		partials[name] = t
	}

	return &Renderer{templates: templates, partials: partials, partnerCSS: hasPartnerCSS}, nil
}

// SetMailEnabled marks that email is configured, so templates can show
// email-dependent UI elements like the "Forgot password?" link.
func (rn *Renderer) SetMailEnabled(enabled bool) {
	rn.mailEnabled = enabled
}

// Render renders a named template with automatic injection of User, CSRFToken, and Flash.
func (rn *Renderer) Render(w io.Writer, r *http.Request, hw http.ResponseWriter, name string, data any) error {
	t, ok := rn.templates[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}

	td := TemplateData{
		User:        middleware.UserFromContext(r.Context()),
		CSRFToken:   middleware.CSRFToken(r),
		Flash:       middleware.GetFlash(hw, r),
		Data:        data,
		PartnerCSS:  rn.partnerCSS,
		MailEnabled: rn.mailEnabled,
	}

	return t.ExecuteTemplate(w, "base", td)
}

// RenderPartial renders a partial template (without base layout) with the
// standard TemplateData wrapper. Used for htmx partial responses.
func (rn *Renderer) RenderPartial(w io.Writer, r *http.Request, hw http.ResponseWriter, name string, data any) error {
	t, ok := rn.partials[name]
	if !ok {
		return fmt.Errorf("partial %q not found", name)
	}

	td := TemplateData{
		User:        middleware.UserFromContext(r.Context()),
		CSRFToken:   middleware.CSRFToken(r),
		Data:        data,
		PartnerCSS:  rn.partnerCSS,
		MailEnabled: rn.mailEnabled,
	}

	return t.Execute(w, td)
}
