package store

import (
	"errors"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)

	doc, err := s.Create("receipt.jpg", "image/jpeg", strings.NewReader("fake image bytes"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if doc.Status != StatusPending {
		t.Errorf("status = %q, want %q", doc.Status, StatusPending)
	}
	if doc.OriginalFilename != "receipt.jpg" {
		t.Errorf("filename = %q, want receipt.jpg", doc.OriginalFilename)
	}
	if doc.SizeBytes != int64(len("fake image bytes")) {
		t.Errorf("size = %d", doc.SizeBytes)
	}

	got, err := s.Get(doc.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != doc.ID || got.ContentType != "image/jpeg" {
		t.Errorf("Get returned %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPathTraversalIDsRejected(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"../etc", "a/b", `a\b`, "..", ""} {
		if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestUploadedFilenameCannotEscape(t *testing.T) {
	s := newTestStore(t)
	doc, err := s.Create("../../evil.sh", "image/png", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	path, err := s.FilePath(doc.ID)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if !strings.Contains(path, doc.ID) {
		t.Errorf("file stored outside document dir: %s", path)
	}
}

func TestListNewestFirst(t *testing.T) {
	s := newTestStore(t)
	first, _ := s.Create("a.jpg", "image/jpeg", strings.NewReader("a"))
	second, _ := s.Create("b.jpg", "image/jpeg", strings.NewReader("b"))

	docs, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len = %d, want 2", len(docs))
	}
	// IDs share a second-resolution timestamp prefix, so we only assert both
	// are present and the ordering is by descending ID.
	if docs[0].ID < docs[1].ID {
		t.Errorf("not sorted newest first: %s before %s", docs[0].ID, docs[1].ID)
	}
	found := map[string]bool{docs[0].ID: true, docs[1].ID: true}
	if !found[first.ID] || !found[second.ID] {
		t.Errorf("missing documents in listing")
	}
}

func TestStatusLifecycle(t *testing.T) {
	s := newTestStore(t)
	doc, _ := s.Create("r.jpg", "image/jpeg", strings.NewReader("x"))

	if _, err := s.UpdateStatus(doc.ID, StatusProcessing, ""); err != nil {
		t.Fatalf("UpdateStatus processing: %v", err)
	}
	updated, err := s.UpdateStatus(doc.ID, StatusCompleted, "")
	if err != nil {
		t.Fatalf("UpdateStatus completed: %v", err)
	}
	if updated.ProcessedAt == nil {
		t.Error("ProcessedAt not set on completion")
	}

	failed, err := s.UpdateStatus(doc.ID, StatusFailed, "boom")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if failed.Error != "boom" {
		t.Errorf("Error = %q, want boom", failed.Error)
	}
}

func TestOCRTextRoundTrip(t *testing.T) {
	s := newTestStore(t)
	doc, _ := s.Create("r.jpg", "image/jpeg", strings.NewReader("x"))

	text, err := s.OCRText(doc.ID)
	if err != nil || text != "" {
		t.Fatalf("OCRText before set = %q, %v", text, err)
	}
	if err := s.SetOCRText(doc.ID, "TOTAL $12.34"); err != nil {
		t.Fatalf("SetOCRText: %v", err)
	}
	text, err = s.OCRText(doc.ID)
	if err != nil {
		t.Fatalf("OCRText: %v", err)
	}
	if text != "TOTAL $12.34" {
		t.Errorf("text = %q", text)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	doc, _ := s.Create("r.jpg", "image/jpeg", strings.NewReader("x"))

	if err := s.Delete(doc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(doc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(doc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete err = %v, want ErrNotFound", err)
	}
}
