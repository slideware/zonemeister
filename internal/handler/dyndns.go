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

// DynDNSHandler handles DynDNS management HTTP requests.
type DynDNSHandler struct {
	netnodClient *netnod.Client
	renderer     *templates.Renderer
}

// NewDynDNSHandler creates a new DynDNSHandler.
func NewDynDNSHandler(client *netnod.Client, renderer *templates.Renderer) *DynDNSHandler {
	return &DynDNSHandler{
		netnodClient: client,
		renderer:     renderer,
	}
}

// DynDNSListData is passed to the dyndns-list partial template.
type DynDNSListData struct {
	ZoneID string
	Labels []netnod.DynDNSLabel
	Token  string // set after enabling DynDNS; one-time display
}

// List returns the DynDNS labels partial for a zone.
// GET /admin/zones/{zoneId}/dyndns
func (h *DynDNSHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	labels, err := h.netnodClient.ListDynDNSLabels(ctx, zoneID)
	if err != nil {
		slog.Error("list dyndns labels", "zone_id", zoneID, "error", err)
		http.Error(w, "Failed to list DynDNS labels: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := DynDNSListData{
		ZoneID: zoneID,
		Labels: labels,
	}
	if err := h.renderer.RenderPartial(w, r, w, "dyndns-list", data); err != nil {
		slog.Error("render dyndns-list partial", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Enable enables DynDNS for a label in a zone and displays the token in a flash message.
// POST /admin/zones/{zoneId}/dyndns
func (h *DynDNSHandler) Enable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")
	label := strings.TrimSpace(r.FormValue("label"))

	if label == "" {
		http.Error(w, "Label is required", http.StatusBadRequest)
		return
	}

	result, err := h.netnodClient.EnableDynDNS(ctx, zoneID, label)
	if err != nil {
		slog.Error("enable dyndns", "zone_id", zoneID, "label", label, "error", err)
		http.Error(w, "Failed to enable DynDNS: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("dyndns enabled", "zone_id", zoneID, "label", label, "hostname", result.Hostname)

	// Re-fetch labels so the list is up-to-date.
	labels, err := h.netnodClient.ListDynDNSLabels(ctx, zoneID)
	if err != nil {
		slog.Error("list dyndns labels after enable", "zone_id", zoneID, "error", err)
		http.Error(w, "DynDNS enabled but failed to refresh list", http.StatusInternalServerError)
		return
	}

	// Return the token directly in the response — never store it in a cookie.
	data := DynDNSListData{
		ZoneID: zoneID,
		Labels: labels,
		Token:  fmt.Sprintf("DynDNS enabled for %s. Token (save it now, it cannot be retrieved again): %s", result.Hostname, result.Token),
	}
	if err := h.renderer.RenderPartial(w, r, w, "dyndns-list", data); err != nil {
		slog.Error("render dyndns-list partial", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Disable disables DynDNS for a label in a zone.
// POST /admin/zones/{zoneId}/dyndns/{label}/delete
func (h *DynDNSHandler) Disable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")
	label := chi.URLParam(r, "label")

	if err := h.netnodClient.DisableDynDNS(ctx, zoneID, label); err != nil {
		slog.Error("disable dyndns", "zone_id", zoneID, "label", label, "error", err)
		http.Error(w, "Failed to disable DynDNS: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("dyndns disabled", "zone_id", zoneID, "label", label)
	middleware.SetFlash(w, fmt.Sprintf("DynDNS disabled for label %q.", label))
	http.Redirect(w, r, "/zones/"+zoneID, http.StatusSeeOther)
}
