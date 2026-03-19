package handler

import (
	"log/slog"
	"net/http"

	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
	"zonemeister/internal/netnod"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"
)

// DashboardData is passed to the dashboard template.
type DashboardData struct {
	ZoneCount     int
	CustomerCount int
}

// DashboardHandler handles the main dashboard.
type DashboardHandler struct {
	netnodClient *netnod.Client
	assignRepo   repository.ZoneAssignmentRepository
	customerRepo repository.CustomerRepository
	renderer     *templates.Renderer
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(client *netnod.Client, assignRepo repository.ZoneAssignmentRepository, customerRepo repository.CustomerRepository, renderer *templates.Renderer) *DashboardHandler {
	return &DashboardHandler{
		netnodClient: client,
		assignRepo:   assignRepo,
		customerRepo: customerRepo,
		renderer:     renderer,
	}
}

// Index renders the dashboard page.
func (h *DashboardHandler) Index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)

	var data DashboardData
	tmpl := "dashboard-customer"

	if user != nil && user.Role == models.RoleCustomer && user.CustomerID != nil {
		assignments, err := h.assignRepo.ListByCustomerID(ctx, *user.CustomerID)
		if err != nil {
			slog.Error("dashboard: list zone assignments", "customer_id", *user.CustomerID, "error", err)
		} else {
			data.ZoneCount = len(assignments)
		}
	} else {
		tmpl = "dashboard-admin"

		resp, err := h.netnodClient.ListZones(ctx, 0, 1)
		if err != nil {
			slog.Error("dashboard: list zones", "error", err)
		} else {
			data.ZoneCount = resp.Total
		}

		customers, err := h.customerRepo.List(ctx)
		if err != nil {
			slog.Error("dashboard: list customers", "error", err)
		} else {
			data.CustomerCount = len(customers)
		}
	}

	if err := h.renderer.Render(w, r, w, tmpl, data); err != nil {
		slog.Error("render dashboard", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
