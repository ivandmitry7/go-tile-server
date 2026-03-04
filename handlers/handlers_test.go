package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-tile-server/downloader"
)

// --- HandleCount ---

func TestHandleCount_Valid(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	body := `{"polygon":[{"lat":10,"lng":100},{"lat":10,"lng":110},{"lat":20,"lng":110},{"lat":20,"lng":100}],"zoom_min":5,"zoom_max":5}`
	req := httptest.NewRequest("POST", "/api/count", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.HandleCount(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]int64
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["count"] <= 0 {
		t.Errorf("expected positive count, got %d", resp["count"])
	}
}

func TestHandleCount_InvalidJSON(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("POST", "/api/count", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()

	h.HandleCount(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCount_TooFewPoints(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	body := `{"polygon":[{"lat":10,"lng":100},{"lat":10,"lng":110}],"zoom_min":5,"zoom_max":5}`
	req := httptest.NewRequest("POST", "/api/count", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.HandleCount(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- HandleDownload validation ---

func TestHandleDownload_InvalidJSON(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("POST", "/api/download", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDownload_TooFewPoints(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	body := `{"polygon":[{"lat":1,"lng":1}],"zoom_min":1,"zoom_max":1}`
	req := httptest.NewRequest("POST", "/api/download", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDownload_InvalidZoom(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	tests := []struct {
		name string
		body string
	}{
		{"zoom_min negative", `{"polygon":[{"lat":0,"lng":0},{"lat":1,"lng":0},{"lat":0,"lng":1}],"zoom_min":-1,"zoom_max":5}`},
		{"zoom_max over 20", `{"polygon":[{"lat":0,"lng":0},{"lat":1,"lng":0},{"lat":0,"lng":1}],"zoom_min":0,"zoom_max":21}`},
		{"zoom_min > zoom_max", `{"polygon":[{"lat":0,"lng":0},{"lat":1,"lng":0},{"lat":0,"lng":1}],"zoom_min":10,"zoom_max":5}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/download", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.HandleDownload(w, req)
			if w.Code != 400 {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

// --- HandleCancel ---

func TestHandleCancel_MissingTaskID(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("GET", "/api/cancel", nil)
	w := httptest.NewRecorder()
	h.HandleCancel(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCancel_TaskNotFound(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("GET", "/api/cancel?task_id=nonexistent", nil)
	w := httptest.NewRecorder()
	h.HandleCancel(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- HandleTasks ---

func TestHandleTasks_Empty(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()
	h.HandleTasks(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var tasks []interface{}
	json.NewDecoder(w.Body).Decode(&tasks)
	if len(tasks) != 0 {
		t.Errorf("expected empty tasks, got %d", len(tasks))
	}
}

// --- HandleProgress ---

func TestHandleProgress_MissingTaskID(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("GET", "/api/progress", nil)
	w := httptest.NewRecorder()
	h.HandleProgress(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleProgress_TaskNotFound(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("GET", "/api/progress?task_id=nope", nil)
	w := httptest.NewRecorder()
	h.HandleProgress(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- HandleDedup ---

func TestHandleDedup(t *testing.T) {
	dir := t.TempDir()
	m := downloader.NewManager(dir)
	h := &APIHandler{Manager: m}

	req := httptest.NewRequest("GET", "/api/dedup", nil)
	w := httptest.NewRecorder()
	h.HandleDedup(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]int64
	json.NewDecoder(w.Body).Decode(&result)
	if result["total_files"] != 0 {
		t.Errorf("expected 0 files in empty dir, got %d", result["total_files"])
	}
}

// --- TileHandler ---

func TestTileHandler_ServeHTTP_NotFound(t *testing.T) {
	h := &TileHandler{TileDir: t.TempDir()}

	req := httptest.NewRequest("GET", "/tiles/1/0/0.png", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTileHandler_ServeHTTP_InvalidPath(t *testing.T) {
	h := &TileHandler{TileDir: t.TempDir()}

	tests := []struct {
		name string
		path string
	}{
		{"too few parts", "/tiles/1/0"},
		{"invalid zoom", "/tiles/abc/0/0.png"},
		{"invalid x", "/tiles/1/abc/0.png"},
		{"invalid y", "/tiles/1/0/abc.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != 400 {
				t.Errorf("expected 400 for %s, got %d", tt.path, w.Code)
			}
		})
	}
}

func TestTileHandler_ServeHTTP_Found(t *testing.T) {
	dir := t.TempDir()
	tilePath := filepath.Join(dir, "5", "10", "15.png")
	os.MkdirAll(filepath.Dir(tilePath), 0755)
	os.WriteFile(tilePath, []byte("fake png"), 0644)

	h := &TileHandler{TileDir: dir}
	req := httptest.NewRequest("GET", "/tiles/5/10/15.png", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}
}

// --- HandleTileSize ---

func TestHandleTileSize_Empty(t *testing.T) {
	h := &TileHandler{TileDir: t.TempDir()}

	req := httptest.NewRequest("GET", "/api/tile-size", nil)
	w := httptest.NewRecorder()
	h.HandleTileSize(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]int64
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["size"] != 0 || resp["files"] != 0 {
		t.Errorf("expected 0 size/files, got size=%d files=%d", resp["size"], resp["files"])
	}
}

func TestHandleTileSize_WithFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "1", "0"), 0755)
	os.WriteFile(filepath.Join(dir, "1", "0", "0.png"), []byte("12345"), 0644)

	h := &TileHandler{TileDir: dir}
	req := httptest.NewRequest("GET", "/api/tile-size", nil)
	w := httptest.NewRecorder()
	h.HandleTileSize(w, req)

	var resp map[string]int64
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["files"] != 1 {
		t.Errorf("expected 1 file, got %d", resp["files"])
	}
	if resp["size"] != 5 {
		t.Errorf("expected size=5, got %d", resp["size"])
	}
}

// --- DrawingsHandler ---

func TestDrawingsHandler_ListEmpty(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	req := httptest.NewRequest("GET", "/api/drawings", nil)
	w := httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var names []string
	json.NewDecoder(w.Body).Decode(&names)
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestDrawingsHandler_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	h := &DrawingsHandler{Dir: dir}

	// Save
	body := `{"name":"test-project","polygon":[{"lat":1,"lng":2}]}`
	req := httptest.NewRequest("POST", "/api/drawings", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 200 {
		t.Fatalf("save: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List
	req = httptest.NewRequest("GET", "/api/drawings", nil)
	w = httptest.NewRecorder()
	h.HandleDrawings(w, req)

	var names []string
	json.NewDecoder(w.Body).Decode(&names)
	if len(names) != 1 || names[0] != "test-project" {
		t.Errorf("expected [test-project], got %v", names)
	}

	// Load
	req = httptest.NewRequest("GET", "/api/drawings?name=test-project", nil)
	w = httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 200 {
		t.Fatalf("load: expected 200, got %d", w.Code)
	}

	var config map[string]interface{}
	json.NewDecoder(w.Body).Decode(&config)
	if config["name"] != "test-project" {
		t.Errorf("expected name=test-project, got %v", config["name"])
	}
}

func TestDrawingsHandler_Delete(t *testing.T) {
	dir := t.TempDir()
	h := &DrawingsHandler{Dir: dir}

	// Save first
	body := `{"name":"to-delete"}`
	req := httptest.NewRequest("POST", "/api/drawings", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleDrawings(w, req)

	// Delete
	req = httptest.NewRequest("DELETE", "/api/drawings?name=to-delete", nil)
	w = httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 200 {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/api/drawings?name=to-delete", nil)
	w = httptest.NewRecorder()
	h.HandleDrawings(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestDrawingsHandler_DeleteNotFound(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	req := httptest.NewRequest("DELETE", "/api/drawings?name=nonexistent", nil)
	w := httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDrawingsHandler_PostNoName(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	req := httptest.NewRequest("POST", "/api/drawings", strings.NewReader(`{"data":123}`))
	w := httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDrawingsHandler_MethodNotAllowed(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	req := httptest.NewRequest("PUT", "/api/drawings", nil)
	w := httptest.NewRecorder()
	h.HandleDrawings(w, req)

	if w.Code != 405 {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- sanitizeFilename ---

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "hello_world"},
		{"../../../etc/passwd", "_________etc_passwd"},
		{"test-project_1", "test-project_1"},
		{"", "unnamed"},
		{"@#$%", "____"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
