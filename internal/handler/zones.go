package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
	"zonemeister/internal/netnod"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"
	"zonemeister/internal/zonefile"

	"github.com/go-chi/chi/v5"
)

// ZoneListEntry enriches a ZoneSummary with customer assignment info.
type ZoneListEntry struct {
	netnod.ZoneSummary
	Customer *models.Customer
}

// ZoneListData is passed to the zones list template.
type ZoneListData struct {
	Zones        []ZoneListEntry
	Filter       string // "all" or "unassigned"
	IsSuperAdmin bool
}

// ZoneHandler handles zone-related HTTP requests.
type ZoneHandler struct {
	netnodClient *netnod.Client
	assignRepo   repository.ZoneAssignmentRepository
	customerRepo repository.CustomerRepository
	tsigKeyRepo  repository.CustomerTSIGKeyRepository
	renderer     *templates.Renderer
}

// NewZoneHandler creates a new ZoneHandler.
func NewZoneHandler(client *netnod.Client, assignRepo repository.ZoneAssignmentRepository, customerRepo repository.CustomerRepository, tsigKeyRepo repository.CustomerTSIGKeyRepository, renderer *templates.Renderer) *ZoneHandler {
	return &ZoneHandler{
		netnodClient: client,
		assignRepo:   assignRepo,
		customerRepo: customerRepo,
		tsigKeyRepo:  tsigKeyRepo,
		renderer:     renderer,
	}
}

// List renders a list of zones. Superadmins see all zones; customers see only
// their assigned zones. Superadmins can filter by ?filter=unassigned.
func (h *ZoneHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)

	// Fetch all zones from the Netnod API.
	resp, err := h.netnodClient.ListZones(ctx, 0, 1000)
	if err != nil {
		slog.Error("list zones from netnod", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	apiZones := resp.Data

	// Sort zones alphabetically by name.
	sort.Slice(apiZones, func(i, j int) bool {
		return apiZones[i].Name < apiZones[j].Name
	})

	// For customer users, filter to only the zones they are assigned to.
	if user != nil && user.Role == models.RoleCustomer && user.CustomerID != nil {
		assignments, err := h.assignRepo.ListByCustomerID(ctx, *user.CustomerID)
		if err != nil {
			slog.Error("list zone assignments", "customer_id", *user.CustomerID, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		assignedIDs := make(map[string]struct{}, len(assignments))
		for _, a := range assignments {
			assignedIDs[a.ZoneID] = struct{}{}
		}

		var entries []ZoneListEntry
		for _, z := range apiZones {
			if _, ok := assignedIDs[z.ID]; ok {
				entries = append(entries, ZoneListEntry{ZoneSummary: z})
			}
		}

		data := ZoneListData{Zones: entries}
		if err := h.renderer.Render(w, r, w, "zones-list", data); err != nil {
			slog.Error("render zones list", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Superadmin: build zone→customer map from all assignments + all customers.
	assignments, err := h.assignRepo.ListAll(ctx)
	if err != nil {
		slog.Error("list all zone assignments", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	customers, err := h.customerRepo.List(ctx)
	if err != nil {
		slog.Error("list customers", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	customerByID := make(map[int64]*models.Customer, len(customers))
	for _, c := range customers {
		customerByID[c.ID] = c
	}

	zoneCustomer := make(map[string]*models.Customer, len(assignments))
	for _, a := range assignments {
		if c, ok := customerByID[a.CustomerID]; ok {
			zoneCustomer[a.ZoneID] = c
		}
	}

	filter := r.URL.Query().Get("filter")

	var entries []ZoneListEntry
	for _, z := range apiZones {
		c := zoneCustomer[z.ID]
		if filter == "unassigned" && c != nil {
			continue
		}
		entries = append(entries, ZoneListEntry{ZoneSummary: z, Customer: c})
	}

	data := ZoneListData{
		Zones:        entries,
		Filter:       filter,
		IsSuperAdmin: true,
	}
	if err := h.renderer.Render(w, r, w, "zones-list", data); err != nil {
		slog.Error("render zones list", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ZoneDetailData is passed to the zone detail template.
type ZoneDetailData struct {
	Zone       *netnod.Zone
	RecordTypes []string
}

// supportedRecordTypes lists DNS record types available for creation.
var supportedRecordTypes = []string{
	"A", "AAAA", "ALIAS", "CAA", "CERT", "CNAME", "CSYNC",
	"HTTPS", "LOC", "MX", "NS", "PTR", "SOA", "SPF", "SRV",
	"TLSA", "TXT",
}

// New renders the create zone form.
// GET /admin/zones/new
func (h *ZoneHandler) New(w http.ResponseWriter, r *http.Request) {
	if err := h.renderer.Render(w, r, w, "zones-create", nil); err != nil {
		slog.Error("render zones create", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Create handles zone creation via the Netnod API.
// POST /admin/zones
func (h *ZoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		middleware.SetFlash(w, "Zone name is required.")
		http.Redirect(w, r, "/admin/zones/new", http.StatusSeeOther)
		return
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}

	var alsoNotify []string
	for _, line := range strings.Split(r.FormValue("also_notify"), "\n") {
		if ip := strings.TrimSpace(line); ip != "" {
			alsoNotify = append(alsoNotify, ip)
		}
	}

	var allowTransferKeys []string
	for _, line := range strings.Split(r.FormValue("allow_transfer_keys"), "\n") {
		if key := strings.TrimSpace(line); key != "" {
			allowTransferKeys = append(allowTransferKeys, key)
		}
	}

	req := netnod.CreateZoneRequest{
		Name:              name,
		AlsoNotify:        alsoNotify,
		AllowTransferKeys: allowTransferKeys,
	}

	zone, err := h.netnodClient.CreateZone(ctx, req)
	if err != nil {
		slog.Error("create zone", "name", name, "error", err)
		middleware.SetFlash(w, fmt.Sprintf("Failed to create zone: %s", err.Error()))
		http.Redirect(w, r, "/admin/zones/new", http.StatusSeeOther)
		return
	}

	slog.Info("zone created", "zone_id", zone.ID, "name", zone.Name)
	middleware.SetFlash(w, fmt.Sprintf("Zone %q created successfully.", zone.Name))
	http.Redirect(w, r, "/zones/"+zone.ID, http.StatusSeeOther)
}

// Delete handles zone deletion via the Netnod API.
// POST /admin/zones/{zoneId}/delete
func (h *ZoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	if err := h.netnodClient.DeleteZone(ctx, zoneID); err != nil {
		slog.Error("delete zone", "zone_id", zoneID, "error", err)
		http.Error(w, "Failed to delete zone: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("zone deleted", "zone_id", zoneID)
	middleware.SetFlash(w, "Zone deleted successfully.")
	http.Redirect(w, r, "/zones", http.StatusSeeOther)
}

// Notify triggers a DNS NOTIFY for the zone.
// POST /admin/zones/{zoneId}/notify
func (h *ZoneHandler) Notify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	if err := h.netnodClient.NotifyZone(ctx, zoneID); err != nil {
		slog.Error("notify zone", "zone_id", zoneID, "error", err)
		http.Error(w, "Failed to send NOTIFY: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("zone notify sent", "zone_id", zoneID)
	middleware.SetFlash(w, "DNS NOTIFY sent successfully.")
	http.Redirect(w, r, "/zones/"+zoneID, http.StatusSeeOther)
}

// Export returns the zone in BIND format as a text/plain download.
// GET /admin/zones/{zoneId}/export
func (h *ZoneHandler) Export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zoneID := chi.URLParam(r, "zoneId")

	data, err := h.netnodClient.ExportZone(ctx, zoneID)
	if err != nil {
		slog.Error("export zone", "zone_id", zoneID, "error", err)
		http.Error(w, "Failed to export zone: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.zone", zoneID))
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, data)
}

// Detail renders the zone detail page with DNS records.
func (h *ZoneHandler) Detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)
	zoneID := chi.URLParam(r, "zoneId")

	// Access control: customer users may only view assigned zones.
	if user.Role == models.RoleCustomer && user.CustomerID != nil {
		assignments, err := h.assignRepo.ListByCustomerID(ctx, *user.CustomerID)
		if err != nil {
			slog.Error("check zone assignment", "zone_id", zoneID, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		found := false
		for _, a := range assignments {
			if a.ZoneID == zoneID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	zone, err := h.netnodClient.GetZone(ctx, zoneID)
	if err != nil {
		slog.Error("get zone from netnod", "zone_id", zoneID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := ZoneDetailData{
		Zone:        zone,
		RecordTypes: supportedRecordTypes,
	}
	if err := h.renderer.Render(w, r, w, "zones-detail", data); err != nil {
		slog.Error("render zone detail", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// CustomerNew renders the zone creation form for customer users.
// GET /zones/new
func (h *ZoneHandler) CustomerNew(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if user.CustomerID == nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := h.renderer.Render(w, r, w, "zones-customer-create", nil); err != nil {
		slog.Error("render customer create zone", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// CustomerCreate handles zone creation for customer users.
// POST /zones
func (h *ZoneHandler) CustomerCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)
	if user.CustomerID == nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		middleware.SetFlash(w, "Zone name is required.")
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}

	tsigKeys, err := h.tsigKeyRepo.ListByCustomerID(ctx, *user.CustomerID)
	if err != nil {
		slog.Error("list tsig keys", "customer_id", *user.CustomerID, "error", err)
		middleware.SetFlash(w, "Internal server error.")
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}

	req := netnod.CreateZoneRequest{
		Name:              name,
		AllowTransferKeys: tsigKeys,
	}

	zone, err := h.netnodClient.CreateZone(ctx, req)
	if err != nil {
		slog.Error("create zone", "name", name, "error", err)
		middleware.SetFlash(w, fmt.Sprintf("Failed to create zone: %s", err.Error()))
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}

	if err := h.assignRepo.Assign(ctx, &models.ZoneAssignment{
		CustomerID: *user.CustomerID,
		ZoneID:     zone.ID,
		ZoneName:   zone.Name,
	}); err != nil {
		slog.Error("auto-assign zone", "zone_id", zone.ID, "customer_id", *user.CustomerID, "error", err)
	}

	slog.Info("customer created zone", "zone_id", zone.ID, "name", zone.Name, "customer_id", *user.CustomerID)
	middleware.SetFlash(w, fmt.Sprintf("Zone %q created successfully.", zone.Name))
	http.Redirect(w, r, "/zones/"+zone.ID, http.StatusSeeOther)
}

// CustomerImport handles zone creation from a BIND zone file for customer users.
// POST /zones/import
func (h *ZoneHandler) CustomerImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)
	if user.CustomerID == nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB limit

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		middleware.SetFlash(w, "Zone name is required.")
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}
	if !strings.HasSuffix(name, ".") {
		name += "."
	}

	content := r.FormValue("zonefile")
	if content == "" {
		middleware.SetFlash(w, "Zone file content is required.")
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}

	rrsets, err := zonefile.Parse(name, content)
	if err != nil {
		slog.Error("parse zone file", "name", name, "error", err)
		middleware.SetFlash(w, fmt.Sprintf("Failed to parse zone file: %s", err.Error()))
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}

	tsigKeys, err := h.tsigKeyRepo.ListByCustomerID(ctx, *user.CustomerID)
	if err != nil {
		slog.Error("list tsig keys", "customer_id", *user.CustomerID, "error", err)
		middleware.SetFlash(w, "Internal server error.")
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}

	req := netnod.CreateZoneRequest{
		Name:              name,
		RRsets:            rrsets,
		AllowTransferKeys: tsigKeys,
	}

	zone, err := h.netnodClient.CreateZone(ctx, req)
	if err != nil {
		slog.Error("create zone from import", "name", name, "error", err)
		middleware.SetFlash(w, fmt.Sprintf("Failed to create zone: %s", err.Error()))
		http.Redirect(w, r, "/zones/new", http.StatusSeeOther)
		return
	}

	if err := h.assignRepo.Assign(ctx, &models.ZoneAssignment{
		CustomerID: *user.CustomerID,
		ZoneID:     zone.ID,
		ZoneName:   zone.Name,
	}); err != nil {
		slog.Error("auto-assign zone", "zone_id", zone.ID, "customer_id", *user.CustomerID, "error", err)
	}

	slog.Info("customer imported zone", "zone_id", zone.ID, "name", zone.Name, "customer_id", *user.CustomerID, "rrsets", len(rrsets))
	middleware.SetFlash(w, fmt.Sprintf("Zone %q created with %d record sets.", zone.Name, len(rrsets)))
	http.Redirect(w, r, "/zones/"+zone.ID, http.StatusSeeOther)
}
