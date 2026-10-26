package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/config"
)

func TestHealthz_OK(t *testing.T) {
	cfg := config.Config{
		DBPath: filepath.Join(t.TempDir(), "test.db"),
	}

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want %q", body.Status, "ok")
	}
}
