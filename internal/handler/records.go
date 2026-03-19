package handler

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"zonemeister/internal/middleware"
	"zonemeister/internal/netnod"
	"zonemeister/internal/repository"
	"zonemeister/internal/templates"

	"github.com/go-chi/chi/v5"
)

// LabelToFQDN converts a label (e.g. "www") to a fully qualified domain name
// using the zone name. "@" or empty string maps to the zone apex. If the label
// already ends with a dot, it is treated as an FQDN and returned as-is.
func LabelToFQDN(label, zoneName string) string {
	if label == "@" || label == "" {
		return zoneName
	}
	if strings.HasSuffix(label, ".") {
		return label
	}
	return label + "." + zoneName
}

// ensureTrailingDot adds a trailing dot to s if it doesn't already have one.
func ensureTrailingDot(s string) string {
	if s == "" || strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}

// formatRecordContent ensures record content is in the format expected by the
// Netnod API. TXT/SPF records must be double-quoted. Record types that contain
// hostnames (CNAME, NS, MX, SRV, PTR, ALIAS) must have a trailing dot on the
// hostname part.
func FormatRecordContent(rrType, content string) string {
	content = strings.TrimSpace(content)
	switch rrType {
	case "TXT", "SPF":
		if !strings.HasPrefix(content, "\"") || !strings.HasSuffix(content, "\"") {
			return "\"" + content + "\""
		}
	case "CNAME", "NS", "PTR", "ALIAS":
		return ensureTrailingDot(content)
	case "MX":
		// MX format: "priority hostname"
		parts := strings.Fields(content)
		if len(parts) == 2 {
			return parts[0] + " " + ensureTrailingDot(parts[1])
		}
	case "SRV":
		// SRV format: "priority weight port target"
		parts := strings.Fields(content)
		if len(parts) == 4 {
			return parts[0] + " " + parts[1] + " " + parts[2] + " " + ensureTrailingDot(parts[3])
		}
	}
	return content
}

// assembleContent builds a single content string from type-specific form fields.
// For structured types (MX, SRV, CAA) it reads dedicated fields; for all others
// it falls back to the generic "content" field.
func assembleContent(r *http.Request, rrType string) (string, error) {
	switch rrType {
	case "MX":
		priority := strings.TrimSpace(r.FormValue("mx_priority"))
		hostname := strings.TrimSpace(r.FormValue("mx_hostname"))
		if priority == "" || hostname == "" {
			return "", fmt.Errorf("priority and hostname are required for MX records")
		}
		if v, err := strconv.Atoi(priority); err != nil || v < 0 || v > 65535 {
			return "", fmt.Errorf("MX priority must be 0–65535")
		}
		return priority + " " + hostname, nil
	case "SRV":
		priority := strings.TrimSpace(r.FormValue("srv_priority"))
		weight := strings.TrimSpace(r.FormValue("srv_weight"))
		port := strings.TrimSpace(r.FormValue("srv_port"))
		target := strings.TrimSpace(r.FormValue("srv_target"))
		if priority == "" || weight == "" || port == "" || target == "" {
			return "", fmt.Errorf("priority, weight, port, and target are required for SRV records")
		}
		for _, pair := range []struct {
			name, val string
			max       int
		}{
			{"priority", priority, 65535},
			{"weight", weight, 65535},
			{"port", port, 65535},
		} {
			if v, err := strconv.Atoi(pair.val); err != nil || v < 0 || v > pair.max {
				return "", fmt.Errorf("SRV %s must be 0–%d", pair.name, pair.max)
			}
		}
		return priority + " " + weight + " " + port + " " + target, nil
	case "CAA":
		flag := strings.TrimSpace(r.FormValue("caa_flag"))
		tag := strings.TrimSpace(r.FormValue("caa_tag"))
		value := strings.TrimSpace(r.FormValue("caa_value"))
		if flag == "" || tag == "" || value == "" {
			return "", fmt.Errorf("flag, tag, and value are required for CAA records")
		}
		if v, err := strconv.Atoi(flag); err != nil || v < 0 || v > 255 {
			return "", fmt.Errorf("CAA flag must be 0–255")
		}
		// CAA value must be quoted in wire format.
		if !strings.HasPrefix(value, "\"") {
			value = "\"" + value + "\""
		}
		return flag + " " + tag + " " + value, nil
	default:
		content := strings.TrimSpace(r.FormValue("content"))
		if content == "" {
			return "", fmt.Errorf("content is required")
		}
		return content, nil
	}
}

// assembleContentMultiple builds records from type-specific form arrays (for
// the edit/update form where multiple records exist in the same RRset).
func assembleContentMultiple(r *http.Request, rrType string) ([]string, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parse form: %w", err)
	}

	switch rrType {
	case "MX":
		priorities := r.Form["mx_priority"]
		hostnames := r.Form["mx_hostname"]
		if len(priorities) == 0 || len(hostnames) == 0 {
			return nil, fmt.Errorf("priority and hostname are required for MX records")
		}
		var result []string
		for i := range priorities {
			if i >= len(hostnames) {
				break
			}
			p := strings.TrimSpace(priorities[i])
			h := strings.TrimSpace(hostnames[i])
			if p != "" && h != "" {
				result = append(result, p+" "+h)
			}
		}
		return result, nil
	case "SRV":
		pris := r.Form["srv_priority"]
		weights := r.Form["srv_weight"]
		ports := r.Form["srv_port"]
		targets := r.Form["srv_target"]
		if len(pris) == 0 || len(targets) == 0 {
			return nil, fmt.Errorf("all SRV fields are required")
		}
		var result []string
		for i := range pris {
			if i >= len(weights) || i >= len(ports) || i >= len(targets) {
				break
			}
			parts := []string{
				strings.TrimSpace(pris[i]),
				strings.TrimSpace(weights[i]),
				strings.TrimSpace(ports[i]),
				strings.TrimSpace(targets[i]),
			}
			allSet := true
			for _, p := range parts {
				if p == "" {
					allSet = false
					break
				}
			}
			if allSet {
				result = append(result, strings.Join(parts, " "))
			}
		}
		return result, nil
	case "CAA":
		flags := r.Form["caa_flag"]
		tags := r.Form["caa_tag"]
		values := r.Form["caa_value"]
		if len(flags) == 0 || len(tags) == 0 || len(values) == 0 {
			return nil, fmt.Errorf("flag, tag, and value are required for CAA records")
		}
		var result []string
		for i := range flags {
			if i >= len(tags) || i >= len(values) {
				break
			}
			f := strings.TrimSpace(flags[i])
			tg := strings.TrimSpace(tags[i])
			v := strings.TrimSpace(values[i])
			if f != "" && tg != "" && v != "" {
				if !strings.HasPrefix(v, "\"") {
					v = "\"" + v + "\""
				}
				result = append(result, f+" "+tg+" "+v)
			}
		}
		return result, nil
	default:
		contentValues := r.Form["content"]
		var result []string
		for _, c := range contentValues {
			c = strings.TrimSpace(c)
			if c != "" {
				result = append(result, c)
			}
		}
		return result, nil
	}
}

// RecordHandler handles DNS record management HTTP requests.
type RecordHandler struct {
	netnodClient *netnod.Client
	assignRepo   repository.ZoneAssignmentRepository
	renderer     *templates.Renderer
}

// NewRecordHandler creates a new RecordHandler.
func NewRecordHandler(client *netnod.Client, assignRepo repository.ZoneAssignmentRepository, renderer *templates.Renderer) *RecordHandler {
	return &RecordHandler{
		netnodClient: client,
		assignRepo:   assignRepo,
		renderer:     renderer,
	}
}

// checkZoneAccess verifies the current user has access to the specified zone.
// Returns true if access is granted. On denial it writes the HTTP error response.
func (h *RecordHandler) checkZoneAccess(w http.ResponseWriter, r *http.Request, zoneID string) bool {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if user.IsSuperAdmin() {
		return true
	}
	if user.CustomerID == nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}

	assignments, err := h.assignRepo.ListByCustomerID(r.Context(), *user.CustomerID)
	if err != nil {
		slog.Error("check zone access", "zone_id", zoneID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return false
	}
	for _, a := range assignments {
		if a.ZoneID == zoneID {
			return true
		}
	}

	http.Error(w, "Forbidden", http.StatusForbidden)
	return false
}

// renderFormError returns an HTML error message as a 422 response with
// HX-Retarget so htmx swaps it into the error display area instead of the
// records table.
func renderFormError(w http.ResponseWriter, msg string) {
	w.Header().Set("HX-Retarget", "#record-form-error")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusUnprocessableEntity)
	fmt.Fprintf(w, `<div class="error">%s</div>`, html.EscapeString(msg))
}

// renderRecordsTable fetches the zone and renders only the records table partial.
func (h *RecordHandler) renderRecordsTable(w http.ResponseWriter, r *http.Request, zoneID string) {
	zone, err := h.netnodClient.GetZone(r.Context(), zoneID)
	if err != nil {
		slog.Error("get zone for records table", "zone_id", zoneID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := ZoneDetailData{
		Zone:        zone,
		RecordTypes: supportedRecordTypes,
	}
	if err := h.renderer.RenderPartial(w, r, w, "records-table", data); err != nil {
		slog.Error("render records table partial", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// RecordsPartial returns just the records table HTML for htmx swaps.
// GET /zones/{zoneId}/records
func (h *RecordHandler) RecordsPartial(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "zoneId")
	if !h.checkZoneAccess(w, r, zoneID) {
		return
	}
	h.renderRecordsTable(w, r, zoneID)
}

// Add handles adding a new DNS record to a zone.
// POST /zones/{zoneId}/records
func (h *RecordHandler) Add(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "zoneId")
	if !h.checkZoneAccess(w, r, zoneID) {
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	rrType := strings.TrimSpace(r.FormValue("type"))
	ttlStr := strings.TrimSpace(r.FormValue("ttl"))

	if rrType == "" {
		renderFormError(w, "Type is required.")
		return
	}

	// Assemble content from type-specific form fields.
	content, err := assembleContent(r, rrType)
	if err != nil {
		renderFormError(w, err.Error())
		return
	}

	// Fetch current zone (needed for label→FQDN conversion and merge check).
	zone, err := h.netnodClient.GetZone(r.Context(), zoneID)
	if err != nil {
		slog.Error("get zone for record add", "zone_id", zoneID, "error", err)
		renderFormError(w, "Failed to load zone.")
		return
	}

	// Convert label to FQDN (e.g. "www" → "www.example.com.").
	name = LabelToFQDN(name, zone.Name)

	var ttl *int
	if ttlStr != "" {
		v, err := strconv.Atoi(ttlStr)
		if err != nil {
			renderFormError(w, "Invalid TTL value.")
			return
		}
		ttl = &v
	}

	// Build records: split content by newlines to support multiple records.
	var records []netnod.Record
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			records = append(records, netnod.Record{Content: FormatRecordContent(rrType, line)})
		}
	}

	change := netnod.RRsetChange{
		Name:       name,
		Type:       rrType,
		ChangeType: "REPLACE",
		TTL:        ttl,
		Records:    records,
	}

	for _, rrset := range zone.RRsets {
		if rrset.Name == name && rrset.Type == rrType {
			// Merge: keep existing records and add the new ones.
			existing := make([]netnod.Record, len(rrset.Records))
			copy(existing, rrset.Records)
			change.Records = append(existing, records...)
			if ttl == nil && rrset.TTL != nil {
				change.TTL = rrset.TTL
			}
			break
		}
	}

	req := netnod.PatchZoneRequest{
		RRsets: []netnod.RRsetChange{change},
	}

	if err := h.netnodClient.PatchZone(r.Context(), zoneID, req); err != nil {
		slog.Error("add record", "zone_id", zoneID, "name", name, "type", rrType, "error", err)
		renderFormError(w, "Failed to add record: "+err.Error())
		return
	}

	slog.Info("record added", "zone_id", zoneID, "name", name, "type", rrType)
	h.renderRecordsTable(w, r, zoneID)
}

// Update handles updating an existing RRset.
// POST /zones/{zoneId}/records/update
func (h *RecordHandler) Update(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "zoneId")
	if !h.checkZoneAccess(w, r, zoneID) {
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	rrType := strings.TrimSpace(r.FormValue("type"))
	ttlStr := strings.TrimSpace(r.FormValue("ttl"))

	if name == "" || rrType == "" {
		renderFormError(w, "Name and type are required.")
		return
	}

	var ttl *int
	if ttlStr != "" {
		v, err := strconv.Atoi(ttlStr)
		if err != nil {
			renderFormError(w, "Invalid TTL value.")
			return
		}
		ttl = &v
	}

	// Assemble content from type-specific form fields.
	contentValues, err := assembleContentMultiple(r, rrType)
	if err != nil {
		renderFormError(w, err.Error())
		return
	}

	var records []netnod.Record
	for _, c := range contentValues {
		records = append(records, netnod.Record{Content: FormatRecordContent(rrType, c)})
	}

	if len(records) == 0 {
		renderFormError(w, "At least one content value is required.")
		return
	}

	change := netnod.RRsetChange{
		Name:       name,
		Type:       rrType,
		ChangeType: "REPLACE",
		TTL:        ttl,
		Records:    records,
	}

	req := netnod.PatchZoneRequest{
		RRsets: []netnod.RRsetChange{change},
	}

	if err := h.netnodClient.PatchZone(r.Context(), zoneID, req); err != nil {
		slog.Error("update record", "zone_id", zoneID, "name", name, "type", rrType, "error", err)
		renderFormError(w, "Failed to update record: "+err.Error())
		return
	}

	slog.Info("record updated", "zone_id", zoneID, "name", name, "type", rrType)
	h.renderRecordsTable(w, r, zoneID)
}

// Delete handles deleting a record from a zone.
// POST /zones/{zoneId}/records/delete
//
// If "content" is provided, only the matching record is removed from the RRset.
// If it was the last record (or "content" is empty), the entire RRset is deleted.
func (h *RecordHandler) Delete(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "zoneId")
	if !h.checkZoneAccess(w, r, zoneID) {
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	rrType := strings.TrimSpace(r.FormValue("type"))
	content := strings.TrimSpace(r.FormValue("content"))

	if name == "" || rrType == "" {
		renderFormError(w, "Name and type are required.")
		return
	}

	// If a specific content value was given, try to remove just that record.
	if content != "" {
		zone, err := h.netnodClient.GetZone(r.Context(), zoneID)
		if err != nil {
			slog.Error("get zone for record delete", "zone_id", zoneID, "error", err)
			renderFormError(w, "Failed to load zone.")
			return
		}

		for _, rrset := range zone.RRsets {
			if rrset.Name == name && rrset.Type == rrType && len(rrset.Records) > 1 {
				// More than one record: keep all except the matching one.
				var remaining []netnod.Record
				for _, rec := range rrset.Records {
					if rec.Content != content {
						remaining = append(remaining, rec)
					}
				}
				if len(remaining) > 0 && len(remaining) < len(rrset.Records) {
					change := netnod.RRsetChange{
						Name:       name,
						Type:       rrType,
						ChangeType: "REPLACE",
						TTL:        rrset.TTL,
						Records:    remaining,
					}
					req := netnod.PatchZoneRequest{RRsets: []netnod.RRsetChange{change}}
					if err := h.netnodClient.PatchZone(r.Context(), zoneID, req); err != nil {
						slog.Error("delete single record", "zone_id", zoneID, "name", name, "type", rrType, "error", err)
						renderFormError(w, "Failed to delete record: "+err.Error())
						return
					}
					slog.Info("record deleted", "zone_id", zoneID, "name", name, "type", rrType, "content", content)
					h.renderRecordsTable(w, r, zoneID)
					return
				}
				break
			}
		}
		// Fall through: single record left or not found — delete entire RRset.
	}

	change := netnod.RRsetChange{
		Name:       name,
		Type:       rrType,
		ChangeType: "DELETE",
	}

	req := netnod.PatchZoneRequest{
		RRsets: []netnod.RRsetChange{change},
	}

	if err := h.netnodClient.PatchZone(r.Context(), zoneID, req); err != nil {
		slog.Error("delete record", "zone_id", zoneID, "name", name, "type", rrType, "error", err)
		renderFormError(w, "Failed to delete record: "+err.Error())
		return
	}

	slog.Info("record deleted", "zone_id", zoneID, "name", name, "type", rrType)
	h.renderRecordsTable(w, r, zoneID)
}

// EditForm returns an inline edit form for an RRset (htmx partial).
// GET /zones/{zoneId}/records/edit?name=...&type=...
func (h *RecordHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "zoneId")
	if !h.checkZoneAccess(w, r, zoneID) {
		return
	}

	name := r.URL.Query().Get("name")
	rrType := r.URL.Query().Get("type")

	zone, err := h.netnodClient.GetZone(r.Context(), zoneID)
	if err != nil {
		slog.Error("get zone for edit form", "zone_id", zoneID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var target *netnod.RRset
	for i := range zone.RRsets {
		if zone.RRsets[i].Name == name && zone.RRsets[i].Type == rrType {
			target = &zone.RRsets[i]
			break
		}
	}

	if target == nil {
		http.Error(w, "RRset not found", http.StatusNotFound)
		return
	}

	data := struct {
		ZoneID   string
		ZoneName string
		RRset    *netnod.RRset
	}{
		ZoneID:   zoneID,
		ZoneName: zone.Name,
		RRset:    target,
	}

	if err := h.renderer.RenderPartial(w, r, w, "records-edit", data); err != nil {
		slog.Error("render edit form partial", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
