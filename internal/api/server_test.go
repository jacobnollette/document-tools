package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"document-tools/internal/ocr"
	"document-tools/internal/store"
)

// pngBytes is a minimal payload that http.DetectContentType sniffs as image/png.
var pngBytes = []byte("\x89PNG\r\n\x1a\nfake-image-data")

// fakeEngine returns canned OCR text without shelling out.
type fakeEngine struct{ text string }

func (f *fakeEngine) Extract(ctx context.Context, path string) (string, error) {
	return f.text, nil
}

func newTestServer(t *testing.T) (*httptest.Server, *store.Store, *ocr.Worker) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := ocr.NewWorker(st, &fakeEngine{text: "TOTAL $42.00"}, logger, 1)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	worker.Start(ctx)

	srv := New(st, worker, logger, "")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, st, worker
}

func uploadFile(t *testing.T, url, filename string, content []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	mw.Close()

	resp, err := http.Post(url+"/api/documents", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func decodeDoc(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return m
}

func TestHealth(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/healthz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestUploadListGetLifecycle(t *testing.T) {
	ts, st, _ := newTestServer(t)

	resp := uploadFile(t, ts.URL, "receipt.png", pngBytes)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d: %s", resp.StatusCode, body)
	}
	doc := decodeDoc(t, resp.Body)
	id, _ := doc["id"].(string)
	if id == "" {
		t.Fatalf("no id in response: %v", doc)
	}
	if doc["content_type"] != "image/png" {
		t.Errorf("content_type = %v", doc["content_type"])
	}

	// The fake engine is instant, but processing is async; wait for the
	// terminal state via the store.
	waitForStatus(t, st, id, store.StatusCompleted)

	detailResp, err := http.Get(ts.URL + "/api/documents/" + id)
	if err != nil {
		t.Fatalf("GET detail: %v", err)
	}
	defer detailResp.Body.Close()
	detail := decodeDoc(t, detailResp.Body)
	if detail["status"] != string(store.StatusCompleted) {
		t.Errorf("status = %v", detail["status"])
	}
	if detail["ocr_text"] != "TOTAL $42.00" {
		t.Errorf("ocr_text = %v", detail["ocr_text"])
	}

	listResp, err := http.Get(ts.URL + "/api/documents")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	defer listResp.Body.Close()
	list := decodeDoc(t, listResp.Body)
	docs, _ := list["documents"].([]any)
	if len(docs) != 1 {
		t.Errorf("list has %d documents, want 1", len(docs))
	}
}

func TestUploadRejectsNonImage(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp := uploadFile(t, ts.URL, "notes.txt", []byte("plain text, not an image"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", resp.StatusCode)
	}
}

func TestUploadRequiresFileField(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/documents", "multipart/form-data; boundary=x", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetUnknownDocument(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/documents/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServeOriginalFile(t *testing.T) {
	ts, st, _ := newTestServer(t)
	resp := uploadFile(t, ts.URL, "receipt.png", pngBytes)
	doc := decodeDoc(t, resp.Body)
	resp.Body.Close()
	id := doc["id"].(string)
	waitForStatus(t, st, id, store.StatusCompleted)

	fileResp, err := http.Get(ts.URL + "/api/documents/" + id + "/file")
	if err != nil {
		t.Fatalf("GET file: %v", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", fileResp.StatusCode)
	}
	body, _ := io.ReadAll(fileResp.Body)
	if !bytes.Equal(body, pngBytes) {
		t.Errorf("file bytes differ from upload")
	}
	if ct := fileResp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestDeleteDocument(t *testing.T) {
	ts, st, _ := newTestServer(t)
	resp := uploadFile(t, ts.URL, "receipt.png", pngBytes)
	doc := decodeDoc(t, resp.Body)
	resp.Body.Close()
	id := doc["id"].(string)
	waitForStatus(t, st, id, store.StatusCompleted)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/documents/"+id, nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", delResp.StatusCode)
	}

	getResp, _ := http.Get(ts.URL + "/api/documents/" + id)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", getResp.StatusCode)
	}
}

func waitForStatus(t *testing.T, st *store.Store, id string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		doc, err := st.Get(id)
		if err == nil && doc.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("document %s never reached status %s", id, want)
}
