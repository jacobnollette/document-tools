// Package store persists document metadata and extracted text in Postgres,
// while original files and page previews stay on the local filesystem
// (typically a mounted volume in a home-lab deployment):
//
//	<root>/documents/<id>/original<ext>
//	<root>/documents/<id>/previews/page-0001.png ...
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	PageCount        int        `json:"page_count"`
	UploadedAt       time.Time  `json:"uploaded_at"`
	ProcessedAt      *time.Time `json:"processed_at,omitempty"`
}

// ErrNotFound is returned when a document ID does not exist.
var ErrNotFound = errors.New("document not found")

const docColumns = `id, original_filename, content_type, size_bytes, status,
	error, page_count, uploaded_at, processed_at`

// Store persists metadata in Postgres and file content under filesRoot.
type Store struct {
	db        *sql.DB
	filesRoot string
}

// New creates (if needed) the files directory and returns a Store.
func New(conn *sql.DB, filesRoot string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(filesRoot, "documents"), 0o755); err != nil {
		return nil, fmt.Errorf("create files directory: %w", err)
	}
	return &Store{db: conn, filesRoot: filesRoot}, nil
}

func (s *Store) docDir(id string) string {
	return filepath.Join(s.filesRoot, "documents", id)
}

// PreviewDir is where per-page preview images for a document live.
func (s *Store) PreviewDir(id string) string {
	return filepath.Join(s.docDir(id), "previews")
}

// PreviewPath returns the path of one page's preview image (1-based).
func (s *Store) PreviewPath(id string, page int) string {
	return filepath.Join(s.PreviewDir(id), fmt.Sprintf("page-%04d.png", page))
}

// newID returns a sortable, unique document ID: upload time plus random hex.
func newID(now time.Time) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b), nil
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

// Create stores the uploaded file on disk and its metadata row in Postgres,
// returning the new document with status pending.
func (s *Store) Create(ctx context.Context, originalFilename, contentType string, content io.Reader) (Document, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	id, err := newID(now)
	if err != nil {
		return Document{}, err
	}

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
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO documents (id, original_filename, content_type, size_bytes, status, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		doc.ID, doc.OriginalFilename, doc.ContentType, doc.SizeBytes, doc.Status, doc.UploadedAt,
	); err != nil {
		os.RemoveAll(dir)
		return Document{}, fmt.Errorf("insert document: %w", err)
	}
	return doc, nil
}

func scanDoc(row interface{ Scan(...any) error }) (Document, error) {
	var doc Document
	var processedAt sql.NullTime
	err := row.Scan(&doc.ID, &doc.OriginalFilename, &doc.ContentType, &doc.SizeBytes,
		&doc.Status, &doc.Error, &doc.PageCount, &doc.UploadedAt, &processedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("scan document: %w", err)
	}
	if processedAt.Valid {
		t := processedAt.Time.UTC()
		doc.ProcessedAt = &t
	}
	doc.UploadedAt = doc.UploadedAt.UTC()
	return doc, nil
}

// Get returns the metadata for one document.
func (s *Store) Get(ctx context.Context, id string) (Document, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+docColumns+` FROM documents WHERE id = $1`, id)
	return scanDoc(row)
}

// List returns all documents, newest first.
func (s *Store) List(ctx context.Context) ([]Document, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+docColumns+` FROM documents ORDER BY uploaded_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	docs := []Document{}
	for rows.Next() {
		doc, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// UpdateStatus transitions a document's pipeline status. The error message is
// recorded for failed documents and cleared otherwise, and processed_at is
// set on terminal states.
func (s *Store) UpdateStatus(ctx context.Context, id string, status Status, errMsg string) (Document, error) {
	if status != StatusFailed {
		errMsg = ""
	}
	var processedAt any
	if status == StatusCompleted || status == StatusFailed {
		processedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE documents
		SET status = $2, error = $3, processed_at = COALESCE($4, processed_at)
		WHERE id = $1
		RETURNING `+docColumns, id, status, errMsg, processedAt)
	return scanDoc(row)
}

// SetOCRResult stores the extracted text and page count for a document.
func (s *Store) SetOCRResult(ctx context.Context, id, text string, pageCount int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE documents SET ocr_text = $2, page_count = $3 WHERE id = $1`,
		id, text, pageCount)
	if err != nil {
		return fmt.Errorf("store ocr result: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// OCRText returns the extracted text, or "" if OCR has not completed.
func (s *Store) OCRText(ctx context.Context, id string) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT ocr_text FROM documents WHERE id = $1`, id).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read ocr text: %w", err)
	}
	return text, nil
}

// FilePath returns the on-disk path of the original uploaded file.
func (s *Store) FilePath(ctx context.Context, id string) (string, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(s.docDir(id), "original*"))
	if err != nil || len(matches) == 0 {
		return "", ErrNotFound
	}
	return matches[0], nil
}

// Delete removes a document row and all of its files.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := os.RemoveAll(s.docDir(id)); err != nil {
		return fmt.Errorf("delete document files: %w", err)
	}
	return nil
}

// Recoverable returns IDs of documents stuck in pending or processing state,
// e.g. after a restart.
func (s *Store) Recoverable(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM documents WHERE status IN ($1, $2) ORDER BY id`,
		StatusPending, StatusProcessing)
	if err != nil {
		return nil, fmt.Errorf("list recoverable documents: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
