package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"zonemeister/internal/middleware"
	"zonemeister/internal/netnod"
	"zonemeister/internal/templates"

	"github.com/go-chi/chi/v5"
)

// ACMEHandler handles ACME DNS-01 challenge management HTTP requests.
type ACMEHandler struct {
	netnodClient *netnod.Client
	renderer     *templates.Renderer
}

// NewACMEHandler creates a new ACMEHandler.
func NewACMEHandler(client *netnod.Client, renderer *templates.Renderer) *ACMEHandler {
	return &ACMEHandler{
		netnodClient: client,
		renderer:     renderer,
	}
}

// ACMEListData is passed to the acme-list partial template.
type ACMEListData struct {
	ZoneID string
	Labels []netnod.ACMELabel
	Token  string // set after enabling ACME; one-time display
}

// List returns the ACME labels partial for a zone.
// GET /admin/zones/{zoneId}/acme
func (h *ACMEHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	labels, err := h.netnodClient.ListACMELabels(ctx, zoneID)
	if err != nil {
		slog.Error("list acme labels", "zone_id", zoneID, "error", err)
		http.Error(w, "Failed to list ACME labels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := ACMEListData{
		ZoneID: zoneID,
		Labels: labels,
	}
	if err := h.renderer.RenderPartial(w, r, w, "acme-list", data); err != nil {
		slog.Error("render acme-list partial", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Enable enables ACME for a label in a zone and displays the token in a flash message.
// POST /admin/zones/{zoneId}/acme
func (h *ACMEHandler) Enable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")
	label := strings.TrimSpace(r.FormValue("label"))

	if label == "" {
		http.Error(w, "Label is required", http.StatusBadRequest)
		return
	}

	result, err := h.netnodClient.EnableACME(ctx, zoneID, label)
	if err != nil {
		slog.Error("enable acme", "zone_id", zoneID, "label", label, "error", err)
		http.Error(w, "Failed to enable ACME: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("acme enabled", "zone_id", zoneID, "label", label, "hostname", result.Hostname, "challenge_hostname", result.ChallengeHostname)

	// Re-fetch labels so the list is up-to-date.
	labels, err := h.netnodClient.ListACMELabels(ctx, zoneID)
	if err != nil {
		slog.Error("list acme labels after enable", "zone_id", zoneID, "error", err)
		http.Error(w, "ACME enabled but failed to refresh list", http.StatusInternalServerError)
		return
	}

	// Return the token directly in the response — never store it in a cookie.
	data := ACMEListData{
		ZoneID: zoneID,
		Labels: labels,
		Token: fmt.Sprintf(
			"ACME enabled for %s (challenge: %s). Token (save it now, it cannot be retrieved again): %s",
			result.Hostname, result.ChallengeHostname, result.Token,
		),
	}
	if err := h.renderer.RenderPartial(w, r, w, "acme-list", data); err != nil {
		slog.Error("render acme-list partial", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Disable disables ACME for a label in a zone.
// POST /admin/zones/{zoneId}/acme/{label}/delete
func (h *ACMEHandler) Disable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")
	label := chi.URLParam(r, "label")

	if err := h.netnodClient.DisableACME(ctx, zoneID, label); err != nil {
		slog.Error("disable acme", "zone_id", zoneID, "label", label, "error", err)
		http.Error(w, "Failed to disable ACME: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("acme disabled", "zone_id", zoneID, "label", label)
	middleware.SetFlash(w, fmt.Sprintf("ACME disabled for label %q.", label))
	http.Redirect(w, r, "/zones/"+zoneID, http.StatusSeeOther)
}
