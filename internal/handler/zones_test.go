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
	"zonemeister/internal/netnod"
)

// mockTSIGKeyRepo implements repository.CustomerTSIGKeyRepository for testing.
type mockTSIGKeyRepo struct {
	keys map[int64][]string
}

func (m *mockTSIGKeyRepo) ListByCustomerID(_ context.Context, customerID int64) ([]string, error) {
	return m.keys[customerID], nil
}

func (m *mockTSIGKeyRepo) SetForCustomer(_ context.Context, customerID int64, keyNames []string) error {
	m.keys[customerID] = keyNames
	return nil
}

func newZoneCreateServer(t *testing.T) (*httptest.Server, *netnod.CreateZoneRequest) {
	t.Helper()
	var captured netnod.CreateZoneRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/zones":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			zone := netnod.Zone{
				ID:   "zone-123",
				Name: captured.Name,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(zone)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	return server, &captured
}

func setupZoneHandler(t *testing.T, server *httptest.Server, assignRepo *mockAssignRepo, tsigRepo *mockTSIGKeyRepo) *handler.ZoneHandler {
	t.Helper()
	client := netnod.NewClient(server.URL, "test-token")
	renderer := newTestRenderer(t)
	return handler.NewZoneHandler(client, assignRepo, nil, tsigRepo, renderer)
}

func TestZoneHandler_CustomerNew(t *testing.T) {
	server, _ := newZoneCreateServer(t)
	defer server.Close()

	h := setupZoneHandler(t, server, &mockAssignRepo{}, &mockTSIGKeyRepo{keys: map[int64][]string{}})

	req := httptest.NewRequest(http.MethodGet, "/zones/new", nil)
	req = withUser(req, customerUser(42))

	w := httptest.NewRecorder()
	h.CustomerNew(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestZoneHandler_CustomerNew_Forbidden(t *testing.T) {
	server, _ := newZoneCreateServer(t)
	defer server.Close()

	h := setupZoneHandler(t, server, &mockAssignRepo{}, &mockTSIGKeyRepo{keys: map[int64][]string{}})

	req := httptest.NewRequest(http.MethodGet, "/zones/new", nil)
	req = withUser(req, superAdmin()) // superadmin has no CustomerID

	w := httptest.NewRecorder()
	h.CustomerNew(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestZoneHandler_CustomerCreate(t *testing.T) {
	server, captured := newZoneCreateServer(t)
	defer server.Close()

	tsigRepo := &mockTSIGKeyRepo{keys: map[int64][]string{
		42: {"key1.example.com.", "key2.example.com."},
	}}
	assignRepo := &mockAssignRepo{}
	h := setupZoneHandler(t, server, assignRepo, tsigRepo)

	form := url.Values{}
	form.Set("name", "newzone.com.")

	req := httptest.NewRequest(http.MethodPost, "/zones", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, customerUser(42))

	w := httptest.NewRecorder()
	h.CustomerCreate(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusSeeOther, w.Body.String())
	}

	if captured.Name != "newzone.com." {
		t.Errorf("zone name = %q, want %q", captured.Name, "newzone.com.")
	}

	if len(captured.AllowTransferKeys) != 2 {
		t.Errorf("allow_transfer_keys count = %d, want 2", len(captured.AllowTransferKeys))
	}

	if len(assignRepo.assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignRepo.assignments))
	}
	if assignRepo.assignments[0].CustomerID != 42 {
		t.Errorf("assignment customer_id = %d, want 42", assignRepo.assignments[0].CustomerID)
	}
	if assignRepo.assignments[0].ZoneID != "zone-123" {
		t.Errorf("assignment zone_id = %q, want %q", assignRepo.assignments[0].ZoneID, "zone-123")
	}
}

func TestZoneHandler_CustomerCreate_Forbidden(t *testing.T) {
	server, _ := newZoneCreateServer(t)
	defer server.Close()

	h := setupZoneHandler(t, server, &mockAssignRepo{}, &mockTSIGKeyRepo{keys: map[int64][]string{}})

	form := url.Values{}
	form.Set("name", "newzone.com.")

	req := httptest.NewRequest(http.MethodPost, "/zones", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, superAdmin())

	w := httptest.NewRecorder()
	h.CustomerCreate(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestZoneHandler_CustomerCreate_EmptyName(t *testing.T) {
	server, _ := newZoneCreateServer(t)
	defer server.Close()

	h := setupZoneHandler(t, server, &mockAssignRepo{}, &mockTSIGKeyRepo{keys: map[int64][]string{}})

	form := url.Values{}
	form.Set("name", "")

	req := httptest.NewRequest(http.MethodPost, "/zones", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, customerUser(42))

	w := httptest.NewRecorder()
	h.CustomerCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestZoneHandler_CustomerImport(t *testing.T) {
	server, captured := newZoneCreateServer(t)
	defer server.Close()

	tsigRepo := &mockTSIGKeyRepo{keys: map[int64][]string{
		42: {"key1.example.com."},
	}}
	assignRepo := &mockAssignRepo{}
	h := setupZoneHandler(t, server, assignRepo, tsigRepo)

	zoneContent := `$ORIGIN example.com.
$TTL 3600
example.com. IN A 1.2.3.4
www          IN A 5.6.7.8
`

	form := url.Values{}
	form.Set("name", "example.com.")
	form.Set("zonefile", zoneContent)

	req := httptest.NewRequest(http.MethodPost, "/zones/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, customerUser(42))

	w := httptest.NewRecorder()
	h.CustomerImport(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusSeeOther, w.Body.String())
	}

	if captured.Name != "example.com." {
		t.Errorf("zone name = %q, want %q", captured.Name, "example.com.")
	}

	if len(captured.RRsets) != 2 {
		t.Errorf("rrsets count = %d, want 2", len(captured.RRsets))
	}

	if len(captured.AllowTransferKeys) != 1 {
		t.Errorf("allow_transfer_keys count = %d, want 1", len(captured.AllowTransferKeys))
	}

	if len(assignRepo.assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignRepo.assignments))
	}
}

func TestZoneHandler_CustomerImport_EmptyZonefile(t *testing.T) {
	server, _ := newZoneCreateServer(t)
	defer server.Close()

	h := setupZoneHandler(t, server, &mockAssignRepo{}, &mockTSIGKeyRepo{keys: map[int64][]string{}})

	form := url.Values{}
	form.Set("name", "example.com.")
	form.Set("zonefile", "")

	req := httptest.NewRequest(http.MethodPost, "/zones/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withUser(req, customerUser(42))

	w := httptest.NewRecorder()
	h.CustomerImport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
