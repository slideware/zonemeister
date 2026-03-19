package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
	"zonemeister/internal/netnod"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"

	"github.com/go-chi/chi/v5"
)

// AssignmentEntry pairs an assignment with the customer name for display.
type AssignmentEntry struct {
	Assignment *models.ZoneAssignment
	Customer   *models.Customer
}

// AssignPageData is passed to the zones-assign template.
type AssignPageData struct {
	ZoneID      string
	ZoneName    string
	Assignments []AssignmentEntry
	Customers   []*models.Customer
}

// AssignmentHandler handles zone assignment HTTP requests.
type AssignmentHandler struct {
	assignRepo   repository.ZoneAssignmentRepository
	customerRepo repository.CustomerRepository
	netnodClient *netnod.Client
	tsigKeyRepo  repository.CustomerTSIGKeyRepository
	renderer     *templates.Renderer
}

// NewAssignmentHandler creates a new AssignmentHandler.
func NewAssignmentHandler(
	assignRepo repository.ZoneAssignmentRepository,
	customerRepo repository.CustomerRepository,
	netnodClient *netnod.Client,
	tsigKeyRepo repository.CustomerTSIGKeyRepository,
	renderer *templates.Renderer,
) *AssignmentHandler {
	return &AssignmentHandler{
		assignRepo:   assignRepo,
		customerRepo: customerRepo,
		netnodClient: netnodClient,
		tsigKeyRepo:  tsigKeyRepo,
		renderer:     renderer,
	}
}

// Show renders the assignment management page for a zone.
// GET /admin/zones/{zoneId}/assign
func (h *AssignmentHandler) Show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	// Resolve zone name from Netnod API.
	zoneName, err := h.resolveZoneName(w, r, zoneID)
	if err != nil {
		return
	}

	// Fetch all assignments for this zone.
	allAssignments, err := h.assignRepo.ListAll(ctx)
	if err != nil {
		slog.Error("list all zone assignments", "zone_id", zoneID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Build assignment entries with customer details.
	var entries []AssignmentEntry
	for _, a := range allAssignments {
		if a.ZoneID != zoneID {
			continue
		}
		customer, err := h.customerRepo.GetByID(ctx, a.CustomerID)
		if err != nil {
			slog.Error("get customer for assignment", "customer_id", a.CustomerID, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		entries = append(entries, AssignmentEntry{
			Assignment: a,
			Customer:   customer,
		})
	}

	// Fetch all customers for the assignment form.
	customers, err := h.customerRepo.List(ctx)
	if err != nil {
		slog.Error("list customers for assignment form", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := AssignPageData{
		ZoneID:      zoneID,
		ZoneName:    zoneName,
		Assignments: entries,
		Customers:   customers,
	}
	if err := h.renderer.Render(w, r, w, "zones-assign", data); err != nil {
		slog.Error("render zones assign", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Assign handles zone assignment to a customer.
// POST /admin/zones/{zoneId}/assign
func (h *AssignmentHandler) Assign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	customerID, err := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Resolve zone name from Netnod API.
	zoneName, err := h.resolveZoneName(w, r, zoneID)
	if err != nil {
		return
	}

	assignment := &models.ZoneAssignment{
		CustomerID: customerID,
		ZoneID:     zoneID,
		ZoneName:   zoneName,
	}

	if err := h.assignRepo.Assign(ctx, assignment); err != nil {
		slog.Error("assign zone to customer", "zone_id", zoneID, "customer_id", customerID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Apply customer's TSIG keys to the zone.
	keyNames, err := h.tsigKeyRepo.ListByCustomerID(ctx, customerID)
	if err != nil {
		slog.Error("list tsig keys for customer", "customer_id", customerID, "error", err)
	} else if len(keyNames) > 0 {
		if err := h.netnodClient.UpdateZone(ctx, zoneID, netnod.UpdateZoneRequest{
			AllowTransferKeys: keyNames,
		}); err != nil {
			slog.Error("update zone transfer keys", "zone_id", zoneID, "error", err)
		}
	}

	middleware.SetFlash(w, "Zone assigned.")
	http.Redirect(w, r, "/admin/zones/"+zoneID+"/assign", http.StatusFound)
}

// Unassign handles removal of a zone assignment.
// POST /admin/zones/{zoneId}/unassign
func (h *AssignmentHandler) Unassign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	customerID, err := strconv.ParseInt(r.FormValue("customer_id"), 10, 64)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := h.assignRepo.Unassign(ctx, customerID, zoneID); err != nil {
		slog.Error("unassign zone from customer", "zone_id", zoneID, "customer_id", customerID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Clear TSIG transfer keys from the zone.
	if err := h.netnodClient.UpdateZone(ctx, zoneID, netnod.UpdateZoneRequest{
		AllowTransferKeys: []string{},
	}); err != nil {
		slog.Error("clear zone transfer keys", "zone_id", zoneID, "error", err)
	}

	middleware.SetFlash(w, "Zone unassigned.")
	http.Redirect(w, r, "/admin/zones/"+zoneID+"/assign", http.StatusFound)
}

// resolveZoneName looks up the zone name from the Netnod API, writing an error
// response and returning a non-nil error if anything goes wrong.
func (h *AssignmentHandler) resolveZoneName(w http.ResponseWriter, r *http.Request, zoneID string) (string, error) {
	resp, err := h.netnodClient.ListZones(r.Context(), 0, 1000)
	if err != nil {
		slog.Error("list zones from netnod for assignment", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return "", err
	}
	for _, z := range resp.Data {
		if z.ID == zoneID {
			return z.Name, nil
		}
	}
	return zoneID, nil
}
