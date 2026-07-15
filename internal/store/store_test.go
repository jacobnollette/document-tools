package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"document-tools/internal/testdb"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn := testdb.Open(t, "store")
	s, err := New(conn, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc, err := s.Create(ctx, "receipt.jpg", "image/jpeg", strings.NewReader("fake image bytes"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if doc.Status != StatusPending {
		t.Errorf("status = %q, want %q", doc.Status, StatusPending)
	}
	if doc.SizeBytes != int64(len("fake image bytes")) {
		t.Errorf("size = %d", doc.SizeBytes)
	}

	got, err := s.Get(ctx, doc.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != doc.ID || got.ContentType != "image/jpeg" || got.OriginalFilename != "receipt.jpg" {
		t.Errorf("Get returned %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUploadedFilenameCannotEscape(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	doc, err := s.Create(ctx, "../../evil.sh", "image/png", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	path, err := s.FilePath(ctx, doc.ID)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if !strings.Contains(path, doc.ID) {
		t.Errorf("file stored outside document dir: %s", path)
	}
}

func TestListNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	first, _ := s.Create(ctx, "a.jpg", "image/jpeg", strings.NewReader("a"))
	second, _ := s.Create(ctx, "b.jpg", "image/jpeg", strings.NewReader("b"))

	docs, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len = %d, want 2", len(docs))
	}
	found := map[string]bool{docs[0].ID: true, docs[1].ID: true}
	if !found[first.ID] || !found[second.ID] {
		t.Errorf("missing documents in listing")
	}
	if docs[0].UploadedAt.Before(docs[1].UploadedAt) {
		t.Errorf("not sorted newest first")
	}
}

func TestStatusLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	doc, _ := s.Create(ctx, "r.jpg", "image/jpeg", strings.NewReader("x"))

	if _, err := s.UpdateStatus(ctx, doc.ID, StatusProcessing, ""); err != nil {
		t.Fatalf("UpdateStatus processing: %v", err)
	}
	updated, err := s.UpdateStatus(ctx, doc.ID, StatusCompleted, "")
	if err != nil {
		t.Fatalf("UpdateStatus completed: %v", err)
	}
	if updated.ProcessedAt == nil {
		t.Error("ProcessedAt not set on completion")
	}

	failed, err := s.UpdateStatus(ctx, doc.ID, StatusFailed, "boom")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if failed.Error != "boom" {
		t.Errorf("Error = %q, want boom", failed.Error)
	}
}

func TestOCRResultRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	doc, _ := s.Create(ctx, "r.jpg", "image/jpeg", strings.NewReader("x"))

	text, err := s.OCRText(ctx, doc.ID)
	if err != nil || text != "" {
		t.Fatalf("OCRText before set = %q, %v", text, err)
	}
	if err := s.SetOCRResult(ctx, doc.ID, "TOTAL $12.34", 3); err != nil {
		t.Fatalf("SetOCRResult: %v", err)
	}
	text, err = s.OCRText(ctx, doc.ID)
	if err != nil || text != "TOTAL $12.34" {
		t.Fatalf("OCRText = %q, %v", text, err)
	}
	got, _ := s.Get(ctx, doc.ID)
	if got.PageCount != 3 {
		t.Errorf("PageCount = %d, want 3", got.PageCount)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	doc, _ := s.Create(ctx, "r.jpg", "image/jpeg", strings.NewReader("x"))

	if err := s.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete err = %v, want ErrNotFound", err)
	}
}

func TestRecoverable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pending, _ := s.Create(ctx, "a.jpg", "image/jpeg", strings.NewReader("a"))
	done, _ := s.Create(ctx, "b.jpg", "image/jpeg", strings.NewReader("b"))
	s.UpdateStatus(ctx, done.ID, StatusCompleted, "")

	ids, err := s.Recoverable(ctx)
	if err != nil {
		t.Fatalf("Recoverable: %v", err)
	}
	if len(ids) != 1 || ids[0] != pending.ID {
		t.Errorf("Recoverable = %v, want [%s]", ids, pending.ID)
	}
}
