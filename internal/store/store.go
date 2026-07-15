// Package store persists uploaded documents and their metadata on the local
// filesystem. Each document lives in its own directory under the data root:
//
//	<root>/documents/<id>/metadata.json
//	<root>/documents/<id>/original<ext>
//	<root>/documents/<id>/ocr.txt
//
// The layout is deliberately simple so the storage backend can be swapped for
// object storage later without touching the API surface.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status describes where a document is in the OCR pipeline.
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Document is the metadata record for one uploaded file.
type Document struct {
	ID               string     `json:"id"`
	OriginalFilename string     `json:"original_filename"`
	ContentType      string     `json:"content_type"`
	SizeBytes        int64      `json:"size_bytes"`
	Status           Status     `json:"status"`
	Error            string     `json:"error,omitempty"`
	UploadedAt       time.Time  `json:"uploaded_at"`
	ProcessedAt      *time.Time `json:"processed_at,omitempty"`
}

// ErrNotFound is returned when a document ID does not exist.
var ErrNotFound = errors.New("document not found")

const (
	metadataFile = "metadata.json"
	ocrFile      = "ocr.txt"
)

// Store is a filesystem-backed document store. It is safe for concurrent use.
type Store struct {
	root string
	mu   sync.RWMutex
}

// New creates (if needed) the data directory and returns a Store rooted there.
func New(root string) (*Store, error) {
	docs := filepath.Join(root, "documents")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) docDir(id string) string {
	return filepath.Join(s.root, "documents", id)
}

// newID returns a sortable, unique document ID: upload time plus random hex.
func newID(now time.Time) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b), nil
}

// validID rejects anything that could escape the documents directory.
func validID(id string) bool {
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return false
	}
	return true
}

// Create stores the uploaded file and its initial metadata, returning the new
// document with status pending.
func (s *Store) Create(originalFilename, contentType string, content io.Reader) (Document, error) {
	now := time.Now().UTC()
	id, err := newID(now)
	if err != nil {
		return Document{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.docDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Document{}, fmt.Errorf("create document directory: %w", err)
	}

	filePath := filepath.Join(dir, originalName(originalFilename))
	f, err := os.Create(filePath)
	if err != nil {
		os.RemoveAll(dir)
		return Document{}, fmt.Errorf("create document file: %w", err)
	}
	size, err := io.Copy(f, content)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.RemoveAll(dir)
		return Document{}, fmt.Errorf("write document file: %w", err)
	}

	doc := Document{
		ID:               id,
		OriginalFilename: filepath.Base(originalFilename),
		ContentType:      contentType,
		SizeBytes:        size,
		Status:           StatusPending,
		UploadedAt:       now,
	}
	if err := s.writeMetadata(doc); err != nil {
		os.RemoveAll(dir)
		return Document{}, err
	}
	return doc, nil
}

// originalName maps an uploaded filename to the on-disk name, keeping only the
// extension so user-supplied names never influence paths.
func originalName(uploadedName string) string {
	ext := strings.ToLower(filepath.Ext(filepath.Base(uploadedName)))
	if ext == "" || strings.ContainsAny(ext, "/\\") {
		return "original"
	}
	return "original" + ext
}

func (s *Store) writeMetadata(doc Document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	tmp := filepath.Join(s.docDir(doc.ID), metadataFile+".tmp")
	final := filepath.Join(s.docDir(doc.ID), metadataFile)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("commit metadata: %w", err)
	}
	return nil
}

// Get returns the metadata for one document.
func (s *Store) Get(id string) (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readMetadata(id)
}

func (s *Store) readMetadata(id string) (Document, error) {
	if !validID(id) {
		return Document{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.docDir(id), metadataFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("read metadata: %w", err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("decode metadata: %w", err)
	}
	return doc, nil
}

// List returns all documents, newest first.
func (s *Store) List() ([]Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(filepath.Join(s.root, "documents"))
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	docs := make([]Document, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		doc, err := s.readMetadata(e.Name())
		if err != nil {
			// Skip partially written or foreign directories rather than
			// failing the whole listing.
			continue
		}
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID > docs[j].ID })
	return docs, nil
}

// UpdateStatus transitions a document's pipeline status. The error message is
// recorded for failed documents and cleared otherwise, and ProcessedAt is set
// on terminal states.
func (s *Store) UpdateStatus(id string, status Status, errMsg string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, err := s.readMetadata(id)
	if err != nil {
		return Document{}, err
	}
	doc.Status = status
	doc.Error = ""
	if status == StatusFailed {
		doc.Error = errMsg
	}
	if status == StatusCompleted || status == StatusFailed {
		now := time.Now().UTC()
		doc.ProcessedAt = &now
	}
	if err := s.writeMetadata(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// FilePath returns the on-disk path of the original uploaded file.
func (s *Store) FilePath(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, err := s.readMetadata(id)
	if err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(s.docDir(doc.ID), "original*"))
	if err != nil || len(matches) == 0 {
		return "", ErrNotFound
	}
	return matches[0], nil
}

// SetOCRText stores the extracted text for a document.
func (s *Store) SetOCRText(id string, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.readMetadata(id); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.docDir(id), ocrFile), []byte(text), 0o644); err != nil {
		return fmt.Errorf("write ocr text: %w", err)
	}
	return nil
}

// OCRText returns the extracted text, or "" if OCR has not completed.
func (s *Store) OCRText(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.readMetadata(id); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(s.docDir(id), ocrFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read ocr text: %w", err)
	}
	return string(data), nil
}

// Delete removes a document and all of its files.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.readMetadata(id); err != nil {
		return err
	}
	if err := os.RemoveAll(s.docDir(id)); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}
