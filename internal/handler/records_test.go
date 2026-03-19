package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"zonemeister/internal/handler"
	"zonemeister/internal/middleware"
	"zonemeister/internal/models"
	"zonemeister/internal/netnod"
	"zonemeister/internal/templates"

	"github.com/go-chi/chi/v5"
)

// mockAssignRepo implements repository.ZoneAssignmentRepository for testing.
type mockAssignRepo struct {
	assignments []*models.ZoneAssignment
}

func (m *mockAssignRepo) ListByCustomerID(_ context.Context, customerID int64) ([]*models.ZoneAssignment, error) {
	var result []*models.ZoneAssignment
	for _, a := range m.assignments {
		if a.CustomerID == customerID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockAssignRepo) ListAll(_ context.Context) ([]*models.ZoneAssignment, error) {
	return m.assignments, nil
}

func (m *mockAssignRepo) Assign(_ context.Context, a *models.ZoneAssignment) error {
	m.assignments = append(m.assignments, a)
	return nil
}

func (m *mockAssignRepo) Unassign(_ context.Context, customerID int64, zoneID string) error {
	return nil
}

func (m *mockAssignRepo) GetCustomerForZone(_ context.Context, zoneID string) (*models.Customer, error) {
	return nil, nil
}

func (m *mockAssignRepo) IsZoneAssigned(_ context.Context, zoneID string) (bool, error) {
	for _, a := range m.assignments {
		if a.ZoneID == zoneID {
			return true, nil
		}
	}
	return false, nil
}

// withUser injects a user into the request context via the middleware helper.
func withUser(r *http.Request, user *models.User) *http.Request {
	ctx := middleware.WithUser(r.Context(), user)
	return r.WithContext(ctx)
}

// newTestNetnodServer creates an httptest.Server that handles GET and PATCH
// for a single zone with the provided RRsets.
func newTestNetnodServer(t *testing.T, zoneID string, rrsets []netnod.RRset) *httptest.Server {
	t.Helper()
	ttl := 3600
	if rrsets == nil {
		rrsets = []netnod.RRset{
			{
				Name:    "example.com.",
				Type:    "A",
				TTL:     &ttl,
				Records: []netnod.Record{{Content: "1.2.3.4"}},
			},
		}
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/zones/"+zoneID:
			zone := netnod.Zone{
				ID:             zoneID,
				Name:           zoneID,
				NotifiedSerial: 2025110401,
				RRsets:         rrsets,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(zone)

		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/zones/"+zoneID:
			var req netnod.PatchZoneRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			// Update the in-memory rrsets to reflect the patch for subsequent GETs.
			for _, change := range req.RRsets {
				switch change.ChangeType {
				case "REPLACE":
					found := false
					for i, rs := range rrsets {
						if rs.Name == change.Name && rs.Type == change.Type {
							rrsets[i].Records = change.Records
							if change.TTL != nil {
								rrsets[i].TTL = change.TTL
							}
							found = true
							break
						}
					}
					if !found {
						rrsets = append(rrsets, netnod.RRset{
							Name:    change.Name,
							Type:    change.Type,
							TTL:     change.TTL,
							Records: change.Records,
						})
					}
				case "DELETE":
					for i, rs := range rrsets {
						if rs.Name == change.Name && rs.Type == change.Type {
							rrsets = append(rrsets[:i], rrsets[i+1:]...)
							break
						}
					}
				}
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

// newTestRenderer creates a renderer from the project templates directory.
func newTestRenderer(t *testing.T) *templates.Renderer {
	t.Helper()
	r, err := templates.New("../../templates", "../../static")
	if err != nil {
		t.Fatalf("create renderer: %v", err)
	}
	return r
}

// setupRecordHandler creates a RecordHandler wired to the test server and repo.
func setupRecordHandler(t *testing.T, server *httptest.Server, repo *mockAssignRepo) *handler.RecordHandler {
	t.Helper()
	client := netnod.NewClient(server.URL, "test-token")
	renderer := newTestRenderer(t)
	return handler.NewRecordHandler(client, repo, renderer)
}

func superAdmin() *models.User {
	return &models.User{
		ID:    1,
		Email: "admin@test.com",
		Role:  models.RoleSuperAdmin,
	}
}

func customerUser(customerID int64) *models.User {
	return &models.User{
		ID:         2,
		Email:      "user@test.com",
		Role:       models.RoleCustomer,
		CustomerID: &customerID,
	}
}

// executeWithChi sets up a chi router context with the given URL param.
func executeWithChi(h http.HandlerFunc, method, path, pattern string, params map[string]string, r *http.Request) *httptest.ResponseRecorder {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestRecordHandler_Delete_SuperAdmin(t *testing.T) {
	ttl := 3600
	rrsets := []netnod.RRset{
		{Name: "example.com.", Type: "A", TTL: &ttl, Records: []netnod.Record{{Content: "1.2.3.4"}}},
		{Name: "mail.example.com.", Type: "MX", TTL: &ttl, Records: []netnod.Record{{Content: "10 mail.example.com."}}},
	}
	server := newTestNetnodServer(t, "example.com.", rrsets)
	defer server.Close()

	repo := &mockAssignRepo{}
	h := setupRecordHandler(t, server, repo)

	form := url.Values{}
	form.Set("name", "mail.example.com.")
	form.Set("type", "MX")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Delete, http.MethodPost, "/zones/example.com./records/delete", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// The response should be the records table partial without the MX record.
	body := w.Body.String()
	if strings.Contains(body, "mail.example.com.") {
		t.Error("response should not contain deleted MX record")
	}
	if !strings.Contains(body, "example.com.") {
		t.Error("response should still contain the A record")
	}
}

func TestRecordHandler_Delete_SingleFromMultiple(t *testing.T) {
	ttl := 3600
	rrsets := []netnod.RRset{
		{
			Name: "example.com.", Type: "A", TTL: &ttl,
			Records: []netnod.Record{
				{Content: "1.2.3.4"},
				{Content: "5.6.7.8"},
				{Content: "9.10.11.12"},
			},
		},
	}
	server := newTestNetnodServer(t, "example.com.", rrsets)
	defer server.Close()

	repo := &mockAssignRepo{}
	h := setupRecordHandler(t, server, repo)

	// Delete only "5.6.7.8" from the RRset.
	form := url.Values{}
	form.Set("name", "example.com.")
	form.Set("type", "A")
	form.Set("content", "5.6.7.8")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Delete, http.MethodPost, "/zones/example.com./records/delete", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "5.6.7.8") {
		t.Error("response should not contain the deleted record 5.6.7.8")
	}
	if !strings.Contains(body, "1.2.3.4") {
		t.Error("response should still contain 1.2.3.4")
	}
	if !strings.Contains(body, "9.10.11.12") {
		t.Error("response should still contain 9.10.11.12")
	}
}

func TestRecordHandler_Add_SuperAdmin(t *testing.T) {
	server := newTestNetnodServer(t, "example.com.", nil)
	defer server.Close()

	repo := &mockAssignRepo{}
	h := setupRecordHandler(t, server, repo)

	form := url.Values{}
	form.Set("name", "new.example.com.")
	form.Set("type", "AAAA")
	form.Set("ttl", "7200")
	form.Set("content", "::1")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Add, http.MethodPost, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "new.example.com.") {
		t.Error("response should contain the new AAAA record")
	}
}

func TestRecordHandler_AccessDenied_Customer(t *testing.T) {
	server := newTestNetnodServer(t, "example.com.", nil)
	defer server.Close()

	// Customer 42 has no assignments.
	repo := &mockAssignRepo{}
	h := setupRecordHandler(t, server, repo)

	req := httptest.NewRequest(http.MethodGet, "/zones/example.com./records", nil)
	req = withUser(req, customerUser(42))

	w := executeWithChi(h.RecordsPartial, http.MethodGet, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRecordHandler_AccessGranted_Customer(t *testing.T) {
	server := newTestNetnodServer(t, "example.com.", nil)
	defer server.Close()

	repo := &mockAssignRepo{
		assignments: []*models.ZoneAssignment{
			{CustomerID: 42, ZoneID: "example.com."},
		},
	}
	h := setupRecordHandler(t, server, repo)

	req := httptest.NewRequest(http.MethodGet, "/zones/example.com./records", nil)
	req = withUser(req, customerUser(42))

	w := executeWithChi(h.RecordsPartial, http.MethodGet, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestRecordHandler_Update_SuperAdmin(t *testing.T) {
	server := newTestNetnodServer(t, "example.com.", nil)
	defer server.Close()

	repo := &mockAssignRepo{}
	h := setupRecordHandler(t, server, repo)

	form := url.Values{}
	form.Set("name", "example.com.")
	form.Set("type", "A")
	form.Set("ttl", "1800")
	form.Add("content", "5.6.7.8")
	form.Add("content", "9.10.11.12")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records/update", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Update, http.MethodPost, "/zones/example.com./records/update", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "5.6.7.8") {
		t.Error("response should contain updated record 5.6.7.8")
	}
	if !strings.Contains(body, "9.10.11.12") {
		t.Error("response should contain updated record 9.10.11.12")
	}
}

func TestLabelToFQDN(t *testing.T) {
	tests := []struct {
		label    string
		zoneName string
		want     string
	}{
		{"www", "example.com.", "www.example.com."},
		{"@", "example.com.", "example.com."},
		{"", "example.com.", "example.com."},
		{"sub.host", "example.com.", "sub.host.example.com."},
		{"example.com.", "example.com.", "example.com."},
		{"foo.example.com.", "example.com.", "foo.example.com."},
	}
	for _, tt := range tests {
		got := handler.LabelToFQDN(tt.label, tt.zoneName)
		if got != tt.want {
			t.Errorf("LabelToFQDN(%q, %q) = %q, want %q", tt.label, tt.zoneName, got, tt.want)
		}
	}
}

func TestRecordHandler_Add_TXT_AutoQuotes(t *testing.T) {
	var patchedRecords []netnod.Record
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			zone := netnod.Zone{ID: "example.com.", Name: "example.com."}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(zone)
		case r.Method == http.MethodPatch:
			var req netnod.PatchZoneRequest
			json.NewDecoder(r.Body).Decode(&req)
			if len(req.RRsets) > 0 {
				patchedRecords = req.RRsets[0].Records
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := netnod.NewClient(server.URL, "test-token")
	renderer := newTestRenderer(t)
	h := handler.NewRecordHandler(client, &mockAssignRepo{}, renderer)

	form := url.Values{}
	form.Set("name", "example.com.")
	form.Set("type", "TXT")
	form.Set("ttl", "3600")
	form.Set("content", "v=spf1 include:example.com ~all")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Add, http.MethodPost, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if len(patchedRecords) == 0 {
		t.Fatal("expected patched records, got none")
	}

	want := "\"v=spf1 include:example.com ~all\""
	if patchedRecords[0].Content != want {
		t.Errorf("TXT content = %q, want %q", patchedRecords[0].Content, want)
	}
}

func TestRecordHandler_Add_TXT_AlreadyQuoted(t *testing.T) {
	var patchedRecords []netnod.Record
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			zone := netnod.Zone{ID: "example.com.", Name: "example.com."}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(zone)
		case r.Method == http.MethodPatch:
			var req netnod.PatchZoneRequest
			json.NewDecoder(r.Body).Decode(&req)
			if len(req.RRsets) > 0 {
				patchedRecords = req.RRsets[0].Records
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := netnod.NewClient(server.URL, "test-token")
	renderer := newTestRenderer(t)
	h := handler.NewRecordHandler(client, &mockAssignRepo{}, renderer)

	form := url.Values{}
	form.Set("name", "example.com.")
	form.Set("type", "TXT")
	form.Set("ttl", "3600")
	form.Set("content", "\"already quoted\"")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Add, http.MethodPost, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if len(patchedRecords) == 0 {
		t.Fatal("expected patched records, got none")
	}

	want := "\"already quoted\""
	if patchedRecords[0].Content != want {
		t.Errorf("TXT content = %q, want %q (should not double-quote)", patchedRecords[0].Content, want)
	}
}

func TestFormatRecordContent(t *testing.T) {
	tests := []struct {
		rrType  string
		content string
		want    string
	}{
		// TXT/SPF quoting
		{"TXT", "v=spf1 ~all", "\"v=spf1 ~all\""},
		{"TXT", "\"already quoted\"", "\"already quoted\""},
		{"SPF", "v=spf1 ~all", "\"v=spf1 ~all\""},
		// CNAME, NS, PTR, ALIAS — trailing dot
		{"CNAME", "www.example.com", "www.example.com."},
		{"CNAME", "www.example.com.", "www.example.com."},
		{"NS", "ns1.example.com", "ns1.example.com."},
		{"PTR", "host.example.com", "host.example.com."},
		{"ALIAS", "example.com", "example.com."},
		// MX — trailing dot on hostname
		{"MX", "10 mail.example.com", "10 mail.example.com."},
		{"MX", "10 mail.example.com.", "10 mail.example.com."},
		{"MX", "20 backup.mail.example.com", "20 backup.mail.example.com."},
		// SRV — trailing dot on target
		{"SRV", "10 60 5060 sip.example.com", "10 60 5060 sip.example.com."},
		{"SRV", "10 60 5060 sip.example.com.", "10 60 5060 sip.example.com."},
		// A, AAAA — no transformation
		{"A", "1.2.3.4", "1.2.3.4"},
		{"AAAA", "::1", "::1"},
	}
	for _, tt := range tests {
		got := handler.FormatRecordContent(tt.rrType, tt.content)
		if got != tt.want {
			t.Errorf("FormatRecordContent(%q, %q) = %q, want %q", tt.rrType, tt.content, got, tt.want)
		}
	}
}

func TestRecordHandler_Add_MX_StructuredFields(t *testing.T) {
	var patchedRecords []netnod.Record
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			zone := netnod.Zone{ID: "example.com.", Name: "example.com."}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(zone)
		case r.Method == http.MethodPatch:
			var req netnod.PatchZoneRequest
			json.NewDecoder(r.Body).Decode(&req)
			if len(req.RRsets) > 0 {
				patchedRecords = req.RRsets[0].Records
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := netnod.NewClient(server.URL, "test-token")
	renderer := newTestRenderer(t)
	h := handler.NewRecordHandler(client, &mockAssignRepo{}, renderer)

	form := url.Values{}
	form.Set("name", "@")
	form.Set("type", "MX")
	form.Set("ttl", "3600")
	form.Set("mx_priority", "10")
	form.Set("mx_hostname", "mail.example.com")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Add, http.MethodPost, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if len(patchedRecords) == 0 {
		t.Fatal("expected patched records, got none")
	}

	// Should assemble "10 mail.example.com" then FormatRecordContent adds trailing dot.
	want := "10 mail.example.com."
	if patchedRecords[0].Content != want {
		t.Errorf("MX content = %q, want %q", patchedRecords[0].Content, want)
	}
}

func TestRecordHandler_Add_MissingFields(t *testing.T) {
	server := newTestNetnodServer(t, "example.com.", nil)
	defer server.Close()

	repo := &mockAssignRepo{}
	h := setupRecordHandler(t, server, repo)

	// Missing content field.
	form := url.Values{}
	form.Set("name", "test.example.com.")
	form.Set("type", "A")

	req := httptest.NewRequest(http.MethodPost, "/zones/example.com./records", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := executeWithChi(h.Add, http.MethodPost, "/zones/example.com./records", "", map[string]string{"zoneId": "example.com."}, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}

	// Error should be returned as HTML with HX-Retarget header.
	if w.Header().Get("HX-Retarget") != "#record-form-error" {
		t.Errorf("HX-Retarget = %q, want %q", w.Header().Get("HX-Retarget"), "#record-form-error")
	}
	if !strings.Contains(w.Body.String(), "content is required") {
		t.Errorf("expected error message about content, got: %s", w.Body.String())
	}
}

