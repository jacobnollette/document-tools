// Package api exposes the HTTP interface: a JSON API under /api plus the
// static single-page web app for everything else. Document routes require a
// session; a separate setup handler (setup.go) runs before installation.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"document-tools/internal/auth"
	"document-tools/internal/convert"
	"document-tools/internal/store"
)

// MaxUploadBytes bounds a single uploaded file.
const MaxUploadBytes = 50 << 20 // 50 MiB

type ctxKey int

const userKey ctxKey = 0

// Enqueuer schedules a stored document for processing.
type Enqueuer interface {
	Enqueue(id string)
}

// Server wires the document store, auth, and processing queue into an
// http.Handler for the installed application.
type Server struct {
	store     *store.Store
	queue     Enqueuer
	auth      *auth.Service
	converter *convert.Converter
	logger    *slog.Logger
	webDist   string
}

// New returns a Server. webDist is the directory holding the built web app;
// empty disables static serving (API only).
func New(s *store.Store, queue Enqueuer, authSvc *auth.Service, logger *slog.Logger, webDist string) *Server {
	return &Server{
		store:     s,
		queue:     queue,
		auth:      authSvc,
		converter: &convert.Converter{},
		logger:    logger,
		webDist:   webDist,
	}
}

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", s.handleHealth)
	mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.handleMe))

	mux.HandleFunc("POST /api/documents", s.requireAuth(s.handleUpload))
	mux.HandleFunc("GET /api/documents", s.requireAuth(s.handleList))
	mux.HandleFunc("GET /api/documents/{id}", s.requireAuth(s.handleGet))
	mux.HandleFunc("GET /api/documents/{id}/file", s.requireAuth(s.handleFile))
	mux.HandleFunc("GET /api/documents/{id}/pages/{page}", s.requireAuth(s.handlePage))
	mux.HandleFunc("GET /api/documents/{id}/download", s.requireAuth(s.handleDownload))
	mux.HandleFunc("DELETE /api/documents/{id}", s.requireAuth(s.handleDelete))

	if s.webDist != "" {
		mux.Handle("/", spaHandler(s.webDist))
	}
	return s.logRequests(mux)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		s.logger.Info("request", "method", r.Method, "path", r.URL.Path)
	})
}

// requireAuth rejects requests without a valid session cookie.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := s.auth.UserID(r, time.Now())
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, userID)))
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"installed": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := s.auth.Authenticate(r.Context(), req.Username, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		s.logger.Error("login", "error", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	now := time.Now()
	auth.SetCookie(w, s.auth.IssueToken(user.ID, now), now)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(userKey).(int64)
	user, err := s.auth.GetUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// documentResponse is the API shape for a document; OCRText is only included
// on detail responses.
type documentResponse struct {
	store.Document
	OCRText *string `json:"ocr_text,omitempty"`
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds the %d MB upload limit", MaxUploadBytes>>20))
			return
		}
		writeError(w, http.StatusBadRequest, `multipart form field "file" is required`)
		return
	}
	defer file.Close()

	contentType, err := sniffContentType(file, header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}
	if !allowedType(contentType) {
		writeError(w, http.StatusUnsupportedMediaType, "supported uploads: images (jpeg, png, webp, tiff, bmp), PDF, and markdown/plain text")
		return
	}

	doc, err := s.store.Create(r.Context(), header.Filename, contentType, file)
	if err != nil {
		s.logger.Error("store upload", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store document")
		return
	}

	s.queue.Enqueue(doc.ID)
	writeJSON(w, http.StatusCreated, documentResponse{Document: doc})
}

func allowedType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/webp", "image/tiff", "image/bmp",
		"application/pdf", "text/plain", "text/markdown":
		return true
	}
	return false
}

// sniffContentType determines the real content type from the file bytes.
// Markdown files sniff as text/plain, so the extension refines that case.
func sniffContentType(file io.ReadSeeker, filename string) (string, error) {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	sniffed := http.DetectContentType(buf[:n])
	if mt, _, err := mime.ParseMediaType(sniffed); err == nil {
		sniffed = mt
	}
	if sniffed == "text/plain" {
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".md" || ext == ".markdown" {
			return "text/markdown", nil
		}
	}
	return sniffed, nil
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.List(r.Context())
	if err != nil {
		s.logger.Error("list documents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("get document", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load document")
		return
	}

	resp := documentResponse{Document: doc}
	if doc.Status == store.StatusCompleted {
		text, err := s.store.OCRText(r.Context(), id)
		if err != nil {
			s.logger.Error("read ocr text", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load extracted text")
			return
		}
		resp.OCRText = &text
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("get document", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load document")
		return
	}

	path, err := s.store.FilePath(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "document file not found")
		return
	}
	if doc.ContentType != "" {
		w.Header().Set("Content-Type", doc.ContentType)
	}
	http.ServeFile(w, r, path)
}

// handlePage serves a rendered page preview. Single-page image documents have
// no separate previews — page 1 is the original file.
func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	page, err := strconv.Atoi(r.PathValue("page"))
	if err != nil || page < 1 {
		writeError(w, http.StatusBadRequest, "page must be a positive integer")
		return
	}

	doc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("get document", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load document")
		return
	}

	previewPath := s.store.PreviewPath(id, page)
	if _, err := os.Stat(previewPath); err == nil {
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, previewPath)
		return
	}

	if page == 1 && strings.HasPrefix(doc.ContentType, "image/") {
		s.handleFile(w, r)
		return
	}
	writeError(w, http.StatusNotFound, "page not found")
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format, err := convert.ParseFormat(r.URL.Query().Get("format"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	doc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("get document", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load document")
		return
	}

	base := convert.BaseName(doc.OriginalFilename)
	if base == "" {
		base = doc.ID
	}

	switch format {
	case convert.FormatOriginal:
		w.Header().Set("Content-Disposition", `attachment; filename="`+doc.OriginalFilename+`"`)
		s.handleFile(w, r)

	case convert.FormatMarkdown, convert.FormatText:
		if doc.Status != store.StatusCompleted {
			writeError(w, http.StatusConflict, "text has not been extracted yet")
			return
		}
		text, err := s.store.OCRText(r.Context(), id)
		if err != nil {
			s.logger.Error("read ocr text", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load extracted text")
			return
		}
		ext, contentType := ".md", "text/markdown; charset=utf-8"
		if format == convert.FormatText {
			ext, contentType = ".txt", "text/plain; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+base+ext+`"`)
		io.WriteString(w, text)

	case convert.FormatPDF:
		if doc.Status != store.StatusCompleted && doc.ContentType != "application/pdf" {
			writeError(w, http.StatusConflict, "document has not been processed yet")
			return
		}
		srcPath, err := s.store.FilePath(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "document file not found")
			return
		}
		text, err := s.store.OCRText(r.Context(), id)
		if err != nil {
			text = ""
		}
		pdfPath, cleanup, err := s.converter.ToPDF(r.Context(), srcPath, doc.ContentType, text)
		if err != nil {
			s.logger.Error("convert to pdf", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "PDF conversion failed: "+err.Error())
			return
		}
		defer cleanup()
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+base+`.pdf"`)
		http.ServeFile(w, r, pdfPath)
	}
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("delete document", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// spaHandler serves the built web app, falling back to index.html for
// client-side routes.
func spaHandler(dist string) http.Handler {
	fileServer := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dist, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil && !strings.Contains(err.Error(), "broken pipe") {
		slog.Default().Error("encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
