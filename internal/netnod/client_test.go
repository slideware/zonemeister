package netnod_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"zonemeister/internal/netnod"
)

const testToken = "test-secret-token"

func newTestClient(server *httptest.Server) *netnod.Client {
	return netnod.NewClient(server.URL, testToken)
}

func assertAuthHeader(t *testing.T, r *http.Request) {
	t.Helper()
	want := "Token " + testToken
	if got := r.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// TestListZones verifies that ListZones returns the correct data and sends the
// Authorization header.
func TestListZones(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/zones" {
			t.Errorf("path = %s, want /api/v1/zones", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, netnod.ZoneListResponse{
			Data: []netnod.ZoneSummary{
				{ID: "example.com.", Name: "example.com.", NotifiedSerial: 2025110401},
			},
			Offset: 0,
			Limit:  100,
			Total:  1,
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.ListZones(context.Background(), 0, 100)
	if err != nil {
		t.Fatalf("ListZones error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if len(result.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(result.Data))
	}
	if result.Data[0].ID != "example.com." {
		t.Errorf("Data[0].ID = %q, want %q", result.Data[0].ID, "example.com.")
	}
	if result.Data[0].NotifiedSerial != 2025110401 {
		t.Errorf("Data[0].NotifiedSerial = %d, want 2025110401", result.Data[0].NotifiedSerial)
	}
}

// TestGetZone verifies that GetZone returns a zone with RRsets.
func TestGetZone(t *testing.T) {
	ttl := 3600
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.URL.Path != "/api/v1/zones/example.com." {
			t.Errorf("path = %s, want /api/v1/zones/example.com.", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, netnod.Zone{
			ID:             "example.com.",
			Name:           "example.com.",
			NotifiedSerial: 2025110401,
			AlsoNotify:     []string{"1.2.3.4"},
			RRsets: []netnod.RRset{
				{
					Name:    "example.com.",
					Type:    "A",
					TTL:     &ttl,
					Records: []netnod.Record{{Content: "1.2.3.4", Disabled: false}},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	zone, err := client.GetZone(context.Background(), "example.com.")
	if err != nil {
		t.Fatalf("GetZone error: %v", err)
	}
	if zone.ID != "example.com." {
		t.Errorf("ID = %q, want %q", zone.ID, "example.com.")
	}
	if len(zone.RRsets) != 1 {
		t.Fatalf("len(RRsets) = %d, want 1", len(zone.RRsets))
	}
	if zone.RRsets[0].Type != "A" {
		t.Errorf("RRsets[0].Type = %q, want A", zone.RRsets[0].Type)
	}
	if zone.RRsets[0].Records[0].Content != "1.2.3.4" {
		t.Errorf("Records[0].Content = %q, want 1.2.3.4", zone.RRsets[0].Records[0].Content)
	}
}

// TestCreateZone verifies that CreateZone sends the correct body and returns
// the created zone.
func TestCreateZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		var body netnod.CreateZoneRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Name != "newzone.com." {
			t.Errorf("body.Name = %q, want newzone.com.", body.Name)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}

		writeJSON(w, http.StatusCreated, netnod.Zone{
			ID:   "newzone.com.",
			Name: "newzone.com.",
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	zone, err := client.CreateZone(context.Background(), netnod.CreateZoneRequest{Name: "newzone.com."})
	if err != nil {
		t.Fatalf("CreateZone error: %v", err)
	}
	if zone.Name != "newzone.com." {
		t.Errorf("Name = %q, want newzone.com.", zone.Name)
	}
}

// TestPatchZone verifies that PatchZone sends the correct changetype.
func TestPatchZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/zones/example.com." {
			t.Errorf("path = %s, want /api/v1/zones/example.com.", r.URL.Path)
		}

		var body netnod.PatchZoneRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if len(body.RRsets) != 1 {
			t.Fatalf("len(RRsets) = %d, want 1", len(body.RRsets))
		}
		if body.RRsets[0].ChangeType != "REPLACE" {
			t.Errorf("ChangeType = %q, want REPLACE", body.RRsets[0].ChangeType)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ttl := 3600
	client := newTestClient(server)
	err := client.PatchZone(context.Background(), "example.com.", netnod.PatchZoneRequest{
		RRsets: []netnod.RRsetChange{
			{
				Name:       "example.com.",
				Type:       "A",
				ChangeType: "REPLACE",
				TTL:        &ttl,
				Records:    []netnod.Record{{Content: "2.3.4.5"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("PatchZone error: %v", err)
	}
}

// TestDeleteZone verifies that DeleteZone returns no error on 204.
func TestDeleteZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/zones/example.com." {
			t.Errorf("path = %s, want /api/v1/zones/example.com.", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(server)
	if err := client.DeleteZone(context.Background(), "example.com."); err != nil {
		t.Fatalf("DeleteZone error: %v", err)
	}
}

// TestExportZone verifies that ExportZone returns the plain-text zone file.
func TestExportZone(t *testing.T) {
	const zoneFile = "$ORIGIN example.com.\n$TTL 3600\nexample.com. IN SOA ns1.example.com. hostmaster.example.com. 2025110401 3600 900 604800 300\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.URL.Path != "/api/v1/zones/example.com./export" {
			t.Errorf("path = %s, want /api/v1/zones/example.com./export", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(zoneFile))
	}))
	defer server.Close()

	client := newTestClient(server)
	got, err := client.ExportZone(context.Background(), "example.com.")
	if err != nil {
		t.Fatalf("ExportZone error: %v", err)
	}
	if got != zoneFile {
		t.Errorf("ExportZone body mismatch\ngot:  %q\nwant: %q", got, zoneFile)
	}
}

// TestErrorHandling verifies that a 404 response is returned as an APIError
// with the correct status code and message.
func TestErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Zone not found"})
	}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.GetZone(context.Background(), "missing.com.")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *netnod.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *netnod.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
	if apiErr.Message != "Zone not found" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "Zone not found")
	}
}

func TestGetZone_InvalidZoneID(t *testing.T) {
	client := netnod.NewClient("http://localhost", "token")

	tests := []struct {
		name   string
		zoneID string
	}{
		{"path traversal", "../../../etc"},
		{"slash", "foo/bar"},
		{"empty", ""},
		{"space", "foo bar"},
		{"query injection", "zone?admin=true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.GetZone(context.Background(), tt.zoneID)
			if err == nil {
				t.Errorf("expected error for zone ID %q, got nil", tt.zoneID)
			}
		})
	}
}

func TestGetZone_ValidZoneID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, netnod.Zone{
			ID:   "example.com.",
			Name: "example.com.",
		})
	}))
	defer server.Close()

	client := newTestClient(server)

	validIDs := []string{
		"example.com.",
		"123",
		"my-zone_01.test.",
	}

	for _, id := range validIDs {
		t.Run(id, func(t *testing.T) {
			zone, err := client.GetZone(context.Background(), id)
			if err != nil {
				t.Errorf("unexpected error for zone ID %q: %v", id, err)
			}
			if zone == nil {
				t.Error("expected non-nil zone")
			}
		})
	}
}
