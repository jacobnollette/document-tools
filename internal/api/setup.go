package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"document-tools/internal/auth"
	"document-tools/internal/config"
	"document-tools/internal/db"
)

// SetupServer serves the first-run installer. It answers setup status with
// env-derived defaults, and on POST /api/setup connects to the database,
// applies the schema, creates the first user, and persists the config. The
// onInstalled callback then swaps in the full application handler.
type SetupServer struct {
	dataDir     string
	logger      *slog.Logger
	webDist     string
	onInstalled func(cfg *config.Config, conn *sql.DB) error

	mu        sync.Mutex
	installed bool
}

// NewSetup returns the installer handler.
func NewSetup(dataDir string, logger *slog.Logger, webDist string, onInstalled func(cfg *config.Config, conn *sql.DB) error) *SetupServer {
	return &SetupServer{
		dataDir:     dataDir,
		logger:      logger,
		webDist:     webDist,
		onInstalled: onInstalled,
	}
}

// Handler builds the setup-mode route table. Everything except the setup
// endpoints and the web app answers 503 so half-configured instances don't
// look healthy.
func (s *SetupServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "awaiting setup"})
	})
	mux.HandleFunc("GET /api/setup/status", s.handleStatus)
	mux.HandleFunc("POST /api/setup", s.handleInstall)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusServiceUnavailable, "application is not installed yet")
	})
	if s.webDist != "" {
		mux.Handle("/", spaHandler(s.webDist))
	}
	return mux
}

// handleStatus reports that setup is required and pre-fills the installer
// form with database settings from the environment. This endpoint is only
// reachable before installation, after which the full app takes over.
func (s *SetupServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"installed": false,
		"defaults":  config.FromEnv(),
	})
}

type installRequest struct {
	Database config.Database `json:"database"`
	Admin    struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"admin"`
}

func (s *SetupServer) handleInstall(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installed {
		writeError(w, http.StatusConflict, "application is already installed")
		return
	}

	var req installRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Database.Host == "" || req.Database.Port == "" || req.Database.User == "" || req.Database.Name == "" {
		writeError(w, http.StatusBadRequest, "database host, port, user, and name are required")
		return
	}

	cfg := &config.Config{Database: req.Database}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	conn, err := db.Open(ctx, cfg.Database.DSN())
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not connect to the database: "+err.Error())
		return
	}

	if err := db.Migrate(ctx, conn); err != nil {
		conn.Close()
		s.logger.Error("apply migrations", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create the database schema: "+err.Error())
		return
	}

	if err := config.Save(s.dataDir, cfg); err != nil {
		conn.Close()
		s.logger.Error("save config", "error", err)
		writeError(w, http.StatusInternalServerError, "could not save configuration: "+err.Error())
		return
	}

	authSvc := auth.New(conn, []byte(cfg.SessionSecret))
	if n, err := authSvc.UserCount(ctx); err != nil {
		conn.Close()
		writeError(w, http.StatusInternalServerError, "could not inspect users: "+err.Error())
		return
	} else if n > 0 {
		conn.Close()
		writeError(w, http.StatusConflict, "this database already has users — it belongs to an existing installation")
		return
	}

	user, err := authSvc.CreateUser(ctx, req.Admin.Username, req.Admin.Password)
	if err != nil {
		conn.Close()
		writeError(w, http.StatusBadRequest, "could not create the first user: "+err.Error())
		return
	}

	if err := s.onInstalled(cfg, conn); err != nil {
		s.logger.Error("activate application", "error", err)
		writeError(w, http.StatusInternalServerError, "installed, but failed to start the application — restart the server")
		return
	}
	s.installed = true
	s.logger.Info("installation complete", "user", user.Username)

	now := time.Now()
	auth.SetCookie(w, authSvc.IssueToken(user.ID, now), now)
	writeJSON(w, http.StatusCreated, map[string]any{"installed": true, "user": user})
}
