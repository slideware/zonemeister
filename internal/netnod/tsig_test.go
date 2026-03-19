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

func newTestTSIGClient(server *httptest.Server) *netnod.TSIGClient {
	return netnod.NewTSIGClient(server.URL, testToken)
}

func TestListKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/apiv3/tsig/" {
			t.Errorf("path = %s, want /apiv3/tsig/", r.URL.Path)
		}

		keys := []netnod.TSIGKey{
			{Key: "abc123", Name: "netnod-test.foo.", Alg: "hmac-sha256"},
			{Key: "def456", Name: "netnod-test.bar.", Alg: "hmac-sha256"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keys)
	}))
	defer server.Close()

	client := newTestTSIGClient(server)
	keys, err := client.ListKeys(context.Background())
	if err != nil {
		t.Fatalf("ListKeys error: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
	if keys[0].Name != "netnod-test.foo." {
		t.Errorf("keys[0].Name = %q, want %q", keys[0].Name, "netnod-test.foo.")
	}
	if keys[1].Name != "netnod-test.bar." {
		t.Errorf("keys[1].Name = %q, want %q", keys[1].Name, "netnod-test.bar.")
	}
}

func TestListKeys_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer server.Close()

	client := newTestTSIGClient(server)
	_, err := client.ListKeys(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *netnod.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
}
