package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"document-tools/internal/config"
	"document-tools/internal/testdb"
)

// setupTarget derives installer form values pointing at the dedicated setup
// test database.
func setupTarget(t *testing.T) config.Database {
	t.Helper()
	testdb.Open(t, "setup").Close() // ensure the database exists and is empty
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	password, _ := u.User.Password()
	return config.Database{
		Host:     u.Hostname(),
		Port:     u.Port(),
		User:     u.User.Username(),
		Password: password,
		Name:     "dt_test_setup",
		SSLMode:  "disable",
	}
}

func TestSetupFlow(t *testing.T) {
	target := setupTarget(t)
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	activated := false
	setup := NewSetup(dataDir, logger, "", func(cfg *config.Config, conn *sql.DB) error {
		activated = true
		conn.Close()
		return nil
	})
	ts := httptest.NewServer(setup.Handler())
	t.Cleanup(ts.Close)

	// Status reports not installed, with env-derived defaults.
	statusResp, err := http.Get(ts.URL + "/api/setup/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	status := struct {
		Installed bool `json:"installed"`
	}{Installed: true}
	json.NewDecoder(statusResp.Body).Decode(&status)
	statusResp.Body.Close()
	if status.Installed {
		t.Error("status reports installed before setup")
	}

	// Documents API is unavailable before install.
	docsResp, _ := http.Get(ts.URL + "/api/documents")
	docsResp.Body.Close()
	if docsResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("documents status = %d, want 503", docsResp.StatusCode)
	}

	// Wrong database credentials are rejected without installing.
	bad := target
	bad.Password = "wrong-password"
	badBody, _ := json.Marshal(map[string]any{
		"database": bad,
		"admin":    map[string]string{"username": "jacob", "password": "first-password"},
	})
	badResp, err := http.Post(ts.URL+"/api/setup", "application/json", bytes.NewReader(badBody))
	if err != nil {
		t.Fatalf("POST setup: %v", err)
	}
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad credentials status = %d, want 400", badResp.StatusCode)
	}

	// Valid install succeeds, persists config, and activates the app.
	goodBody, _ := json.Marshal(map[string]any{
		"database": target,
		"admin":    map[string]string{"username": "jacob", "password": "first-password"},
	})
	goodResp, err := http.Post(ts.URL+"/api/setup", "application/json", bytes.NewReader(goodBody))
	if err != nil {
		t.Fatalf("POST setup: %v", err)
	}
	body, _ := io.ReadAll(goodResp.Body)
	goodResp.Body.Close()
	if goodResp.StatusCode != http.StatusCreated {
		t.Fatalf("install status = %d: %s", goodResp.StatusCode, body)
	}
	if !activated {
		t.Error("onInstalled was not called")
	}
	if len(goodResp.Header.Values("Set-Cookie")) == 0 {
		t.Error("install did not log the first user in")
	}

	cfg, err := config.Load(dataDir)
	if err != nil || cfg == nil {
		t.Fatalf("config not persisted: %v", err)
	}
	if cfg.SessionSecret == "" || cfg.Database.Name != "dt_test_setup" {
		t.Errorf("persisted config incomplete: %+v", cfg)
	}

	// A second install attempt is refused.
	againResp, err := http.Post(ts.URL+"/api/setup", "application/json", bytes.NewReader(goodBody))
	if err != nil {
		t.Fatalf("POST setup again: %v", err)
	}
	againResp.Body.Close()
	if againResp.StatusCode != http.StatusConflict {
		t.Errorf("second install status = %d, want 409", againResp.StatusCode)
	}
}
