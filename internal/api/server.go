// Package api exposes the HTTP interface: a JSON API under /api plus the
// static single-page web app for everything else.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"document-tools/internal/store"
)

// MaxUploadBytes bounds a single uploaded file (phone photos are a few MB).
const MaxUploadBytes = 25 << 20 // 25 MiB

// allowedContentTypes are the upload types the OCR pipeline can handle.
var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/tiff": true,
	"image/bmp":  true,
}

// Enqueuer schedules a stored document for OCR processing.
type Enqueuer interface {
	Enqueue(id string)
}

// Server wires the document store and OCR queue into an http.Handler.
type Server struct {
	store   *store.Store
	queue   Enqueuer
	logger  *slog.Logger
	webDist string
}

// New returns a Server. webDist is the directory holding the built web app;
// empty disables static serving (API only).
func New(s *store.Store, queue Enqueuer, logger *slog.Logger, webDist string) *Server {
	return &Server{store: s, queue: queue, logger: logger, webDist: webDist}
}

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", s.handleHealth)
	mux.HandleFunc("POST /api/documents", s.handleUpload)
	mux.HandleFunc("GET /api/documents", s.handleList)
	mux.HandleFunc("GET /api/documents/{id}", s.handleGet)
	mux.HandleFunc("GET /api/documents/{id}/file", s.handleFile)
	mux.HandleFunc("DELETE /api/documents/{id}", s.handleDelete)
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

// documentResponse is the API shape for a document; OCRText is only included
// on detail responses.
type documentResponse struct {
	store.Document
	OCRText *string `json:"ocr_text,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

	contentType, err := sniffContentType(file, header.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}
	if !allowedContentTypes[contentType] {
		writeError(w, http.StatusUnsupportedMediaType, "only image uploads are supported (jpeg, png, webp, tiff, bmp)")
		return
	}

	doc, err := s.store.Create(header.Filename, contentType, file)
	if err != nil {
		s.logger.Error("store upload", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store document")
		return
	}

	s.queue.Enqueue(doc.ID)
	writeJSON(w, http.StatusCreated, documentResponse{Document: doc})
}

// sniffContentType determines the real content type from the file bytes,
// falling back to the client-declared type only when sniffing is inconclusive.
func sniffContentType(file io.ReadSeeker, declared string) (string, error) {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	sniffed := http.DetectContentType(buf[:n])
	if sniffed == "application/octet-stream" && declared != "" {
		if mt, _, err := mime.ParseMediaType(declared); err == nil {
			return mt, nil
		}
	}
	return sniffed, nil
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.List()
	if err != nil {
		s.logger.Error("list documents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(id)
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
		text, err := s.store.OCRText(id)
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
	doc, err := s.store.Get(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("get document", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load document")
		return
	}

	path, err := s.store.FilePath(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "document file not found")
		return
	}
	if doc.ContentType != "" {
		w.Header().Set("Content-Type", doc.ContentType)
	}
	http.ServeFile(w, r, path)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.Delete(id)
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
