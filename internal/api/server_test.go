package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"document-tools/internal/auth"
	"document-tools/internal/ocr"
	"document-tools/internal/store"
	"document-tools/internal/testdb"
)

// pngBytes is a minimal payload that http.DetectContentType sniffs as image/png.
var pngBytes = []byte("\x89PNG\r\n\x1a\nfake-image-data")

// fakeEngine returns canned OCR text without shelling out.
type fakeEngine struct{ text string }

func (f *fakeEngine) Process(ctx context.Context, path, contentType, previewDir string) (ocr.Result, error) {
	return ocr.Result{Text: f.text, PageCount: 1}, nil
}

type testEnv struct {
	ts     *httptest.Server
	store  *store.Store
	client *http.Client
}

// newTestEnv boots the full server with a fake OCR engine, creates a user,
// and returns a client that is logged in.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	conn := testdb.Open(t, "api")
	st, err := store.New(conn, t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authSvc := auth.New(conn, []byte("test-secret"))
	worker := ocr.NewWorker(st, &fakeEngine{text: "TOTAL $42.00"}, logger, 1)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	worker.Start(ctx)

	ts := httptest.NewServer(New(st, worker, authSvc, logger, "").Handler())
	t.Cleanup(ts.Close)

	if _, err := authSvc.CreateUser(ctx, "jacob", "test-password"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{"username": "jacob", "password": "test-password"})
	resp, err := client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}

	return &testEnv{ts: ts, store: st, client: client}
}

func (e *testEnv) upload(t *testing.T, filename string, content []byte) *http.Response {
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

	resp, err := e.client.Post(e.ts.URL+"/api/documents", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func decodeMap(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return m
}

func waitForStatus(t *testing.T, st *store.Store, id string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		doc, err := st.Get(context.Background(), id)
		if err == nil && doc.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("document %s never reached status %s", id, want)
}

func TestRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	resp, err := http.Get(e.ts.URL + "/api/documents") // no cookie jar
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	e := newTestEnv(t)
	body, _ := json.Marshal(map[string]string{"username": "jacob", "password": "nope"})
	resp, err := http.Post(e.ts.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestUploadListGetLifecycle(t *testing.T) {
	e := newTestEnv(t)

	resp := e.upload(t, "receipt.png", pngBytes)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d: %s", resp.StatusCode, body)
	}
	doc := decodeMap(t, resp.Body)
	id, _ := doc["id"].(string)
	if id == "" {
		t.Fatalf("no id in response: %v", doc)
	}
	waitForStatus(t, e.store, id, store.StatusCompleted)

	detailResp, err := e.client.Get(e.ts.URL + "/api/documents/" + id)
	if err != nil {
		t.Fatalf("GET detail: %v", err)
	}
	defer detailResp.Body.Close()
	detail := decodeMap(t, detailResp.Body)
	if detail["status"] != string(store.StatusCompleted) {
		t.Errorf("status = %v", detail["status"])
	}
	if detail["ocr_text"] != "TOTAL $42.00" {
		t.Errorf("ocr_text = %v", detail["ocr_text"])
	}

	listResp, err := e.client.Get(e.ts.URL + "/api/documents")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	defer listResp.Body.Close()
	list := decodeMap(t, listResp.Body)
	if docs, _ := list["documents"].([]any); len(docs) != 1 {
		t.Errorf("list has %d documents, want 1", len(docs))
	}
}

func TestUploadAcceptsPDF(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "scan.pdf", []byte("%PDF-1.4 fake pdf content"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if doc := decodeMap(t, resp.Body); doc["content_type"] != "application/pdf" {
		t.Errorf("content_type = %v", doc["content_type"])
	}
}

func TestUploadDetectsMarkdown(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "notes.md", []byte("# Notes\n\nhello"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if doc := decodeMap(t, resp.Body); doc["content_type"] != "text/markdown" {
		t.Errorf("content_type = %v", doc["content_type"])
	}
}

func TestUploadRejectsBinary(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "app.bin", []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00, 0x00, 0x00})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", resp.StatusCode)
	}
}

func TestDownloadMarkdown(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "receipt.png", pngBytes)
	doc := decodeMap(t, resp.Body)
	resp.Body.Close()
	id := doc["id"].(string)
	waitForStatus(t, e.store, id, store.StatusCompleted)

	dlResp, err := e.client.Get(e.ts.URL + "/api/documents/" + id + "/download?format=markdown")
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", dlResp.StatusCode)
	}
	body, _ := io.ReadAll(dlResp.Body)
	if string(body) != "TOTAL $42.00" {
		t.Errorf("body = %q", body)
	}
	if cd := dlResp.Header.Get("Content-Disposition"); cd != `attachment; filename="receipt.md"` {
		t.Errorf("content-disposition = %q", cd)
	}
}

func TestDownloadUnknownFormat(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "receipt.png", pngBytes)
	doc := decodeMap(t, resp.Body)
	resp.Body.Close()
	id := doc["id"].(string)

	dlResp, err := e.client.Get(e.ts.URL + "/api/documents/" + id + "/download?format=docx")
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", dlResp.StatusCode)
	}
}

func TestPageServesOriginalForImages(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "receipt.png", pngBytes)
	doc := decodeMap(t, resp.Body)
	resp.Body.Close()
	id := doc["id"].(string)
	waitForStatus(t, e.store, id, store.StatusCompleted)

	pageResp, err := e.client.Get(e.ts.URL + "/api/documents/" + id + "/pages/1")
	if err != nil {
		t.Fatalf("GET page: %v", err)
	}
	defer pageResp.Body.Close()
	if pageResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", pageResp.StatusCode)
	}
	body, _ := io.ReadAll(pageResp.Body)
	if !bytes.Equal(body, pngBytes) {
		t.Errorf("page 1 differs from original image")
	}

	missing, err := e.client.Get(e.ts.URL + "/api/documents/" + id + "/pages/2")
	if err != nil {
		t.Fatalf("GET page 2: %v", err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("page 2 status = %d, want 404", missing.StatusCode)
	}
}

func TestDeleteDocument(t *testing.T) {
	e := newTestEnv(t)
	resp := e.upload(t, "receipt.png", pngBytes)
	doc := decodeMap(t, resp.Body)
	resp.Body.Close()
	id := doc["id"].(string)
	waitForStatus(t, e.store, id, store.StatusCompleted)

	req, _ := http.NewRequest(http.MethodDelete, e.ts.URL+"/api/documents/"+id, nil)
	delResp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", delResp.StatusCode)
	}

	getResp, _ := e.client.Get(e.ts.URL + "/api/documents/" + id)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", getResp.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	e := newTestEnv(t)
	resp, err := e.client.Post(e.ts.URL+"/api/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("POST logout: %v", err)
	}
	resp.Body.Close()

	listResp, err := e.client.Get(e.ts.URL + "/api/documents")
	if err != nil {
		t.Fatalf("GET after logout: %v", err)
	}
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status after logout = %d, want 401", listResp.StatusCode)
	}
}
