package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-tile-server/downloader"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func ginContext(w *httptest.ResponseRecorder, method, url string, body string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	if body != "" {
		c.Request = httptest.NewRequest(method, url, strings.NewReader(body))
	} else {
		c.Request = httptest.NewRequest(method, url, nil)
	}
	return c
}

// --- HandleCount ---

func TestHandleCount_Valid(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/count",
		`{"polygon":[{"lat":10,"lng":100},{"lat":10,"lng":110},{"lat":20,"lng":110},{"lat":20,"lng":100}],"zoom_min":5,"zoom_max":5}`)
	h.HandleCount(c)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["count"].(float64) <= 0 {
		t.Errorf("expected positive count, got %v", resp["count"])
	}
}

func TestHandleCount_InvalidJSON(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/count", "{invalid")
	h.HandleCount(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCount_TooFewPoints(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/count",
		`{"polygon":[{"lat":10,"lng":100},{"lat":10,"lng":110}],"zoom_min":5,"zoom_max":5}`)
	h.HandleCount(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- HandleDownload validation ---

func TestHandleDownload_InvalidJSON(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/download", "bad")
	h.HandleDownload(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDownload_TooFewPoints(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/download",
		`{"polygon":[{"lat":1,"lng":1}],"zoom_min":1,"zoom_max":1}`)
	h.HandleDownload(c)

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
			w := httptest.NewRecorder()
			c := ginContext(w, "POST", "/api/download", tt.body)
			h.HandleDownload(c)
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

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/cancel", "")
	h.HandleCancel(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCancel_TaskNotFound(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/cancel?task_id=nonexistent", "")
	h.HandleCancel(c)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- HandleTasks ---

func TestHandleTasks_Empty(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/tasks", "")
	h.HandleTasks(c)

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

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/progress", "")
	h.HandleProgress(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleProgress_TaskNotFound(t *testing.T) {
	m := downloader.NewManager(t.TempDir())
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/progress?task_id=nope", "")
	h.HandleProgress(c)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- HandleDedup ---

func TestHandleDedup(t *testing.T) {
	dir := t.TempDir()
	m := downloader.NewManager(dir)
	h := &APIHandler{Manager: m}

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/dedup", "")
	h.HandleDedup(c)

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

func TestServeTile_NotFound(t *testing.T) {
	h := NewTileHandler(t.TempDir(), 1024*1024)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/tiles/1/0/0.png", nil)
	c.Params = gin.Params{
		{Key: "z", Value: "1"},
		{Key: "x", Value: "0"},
		{Key: "filename", Value: "0.png"},
	}
	h.ServeTile(c)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServeTile_InvalidPath(t *testing.T) {
	h := NewTileHandler(t.TempDir(), 1024*1024)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/tiles/1/0/0.txt", nil)
	c.Params = gin.Params{
		{Key: "z", Value: "1"},
		{Key: "x", Value: "0"},
		{Key: "filename", Value: "0.txt"},
	}
	h.ServeTile(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestServeTile_Found(t *testing.T) {
	dir := t.TempDir()
	tilePath := filepath.Join(dir, "5", "10", "15.png")
	os.MkdirAll(filepath.Dir(tilePath), 0755)
	os.WriteFile(tilePath, []byte("fake png"), 0644)

	h := NewTileHandler(dir, 1024*1024)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/tiles/5/10/15.png", nil)
	c.Params = gin.Params{
		{Key: "z", Value: "5"},
		{Key: "x", Value: "10"},
		{Key: "filename", Value: "15.png"},
	}
	h.ServeTile(c)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}
}

func TestServeTile_CacheHit(t *testing.T) {
	dir := t.TempDir()
	tilePath := filepath.Join(dir, "1", "0", "0.png")
	os.MkdirAll(filepath.Dir(tilePath), 0755)
	os.WriteFile(tilePath, []byte("tile data"), 0644)

	h := NewTileHandler(dir, 1024*1024)

	// First request — disk read + cache put
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("GET", "/tiles/1/0/0.png", nil)
	c1.Params = gin.Params{{Key: "z", Value: "1"}, {Key: "x", Value: "0"}, {Key: "filename", Value: "0.png"}}
	h.ServeTile(c1)

	if w1.Code != 200 {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	// Delete file — cache should still serve
	os.Remove(tilePath)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/tiles/1/0/0.png", nil)
	c2.Params = gin.Params{{Key: "z", Value: "1"}, {Key: "x", Value: "0"}, {Key: "filename", Value: "0.png"}}
	h.ServeTile(c2)

	if w2.Code != 200 {
		t.Errorf("cached request: expected 200, got %d", w2.Code)
	}
	if w2.Body.String() != "tile data" {
		t.Errorf("expected cached data, got %q", w2.Body.String())
	}
}

// --- HandleTileSize ---

func TestHandleTileSize_Empty(t *testing.T) {
	h := NewTileHandler(t.TempDir(), 1024*1024)

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/tile-size", "")
	h.HandleTileSize(c)

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

	h := NewTileHandler(dir, 1024*1024)

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/tile-size", "")
	h.HandleTileSize(c)

	var resp map[string]int64
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["files"] != 1 {
		t.Errorf("expected 1 file, got %d", resp["files"])
	}
	if resp["size"] != 5 {
		t.Errorf("expected size=5, got %d", resp["size"])
	}
}

func TestHandleTileSize_Cached(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "1", "0"), 0755)
	os.WriteFile(filepath.Join(dir, "1", "0", "0.png"), []byte("12345"), 0644)

	h := NewTileHandler(dir, 1024*1024)

	// First call computes
	w1 := httptest.NewRecorder()
	h.HandleTileSize(ginContext(w1, "GET", "/api/tile-size", ""))

	// Add file
	os.WriteFile(filepath.Join(dir, "1", "0", "1.png"), []byte("abc"), 0644)

	// Second call returns cached
	w2 := httptest.NewRecorder()
	h.HandleTileSize(ginContext(w2, "GET", "/api/tile-size", ""))

	var resp map[string]int64
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp["files"] != 1 {
		t.Errorf("expected cached 1 file, got %d", resp["files"])
	}

	// Invalidate and re-check
	h.InvalidateSizeCache()
	w3 := httptest.NewRecorder()
	h.HandleTileSize(ginContext(w3, "GET", "/api/tile-size", ""))

	json.NewDecoder(w3.Body).Decode(&resp)
	if resp["files"] != 2 {
		t.Errorf("expected 2 files after invalidation, got %d", resp["files"])
	}
}

// --- DrawingsHandler ---

func TestDrawingsHandler_ListEmpty(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	w := httptest.NewRecorder()
	c := ginContext(w, "GET", "/api/drawings", "")
	h.HandleGetDrawings(c)

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
	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/drawings",
		`{"name":"test-project","polygon":[{"lat":1,"lng":2}]}`)
	h.HandlePostDrawings(c)

	if w.Code != 200 {
		t.Fatalf("save: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List
	w = httptest.NewRecorder()
	c = ginContext(w, "GET", "/api/drawings", "")
	h.HandleGetDrawings(c)

	var names []string
	json.NewDecoder(w.Body).Decode(&names)
	if len(names) != 1 || names[0] != "test-project" {
		t.Errorf("expected [test-project], got %v", names)
	}

	// Load
	w = httptest.NewRecorder()
	c = ginContext(w, "GET", "/api/drawings?name=test-project", "")
	h.HandleGetDrawings(c)

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

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/drawings", `{"name":"to-delete"}`)
	h.HandlePostDrawings(c)

	w = httptest.NewRecorder()
	c = ginContext(w, "DELETE", "/api/drawings?name=to-delete", "")
	h.HandleDeleteDrawings(c)

	if w.Code != 200 {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	c = ginContext(w, "GET", "/api/drawings?name=to-delete", "")
	h.HandleGetDrawings(c)
	if w.Code != 404 {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestDrawingsHandler_DeleteNotFound(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	w := httptest.NewRecorder()
	c := ginContext(w, "DELETE", "/api/drawings?name=nonexistent", "")
	h.HandleDeleteDrawings(c)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDrawingsHandler_PostNoName(t *testing.T) {
	h := &DrawingsHandler{Dir: t.TempDir()}

	w := httptest.NewRecorder()
	c := ginContext(w, "POST", "/api/drawings", `{"data":123}`)
	h.HandlePostDrawings(c)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
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

// --- LRU Cache ---

func TestTileCache_PutGet(t *testing.T) {
	cache := NewTileCache(1024)
	cache.Put("1/0/0", []byte("data1"))

	data, ok := cache.Get("1/0/0")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(data) != "data1" {
		t.Errorf("expected data1, got %s", data)
	}
}

func TestTileCache_Eviction(t *testing.T) {
	cache := NewTileCache(10) // 10 bytes max
	cache.Put("a", []byte("12345"))  // 5 bytes
	cache.Put("b", []byte("67890"))  // 5 bytes, total 10
	cache.Put("c", []byte("abcde")) // 5 bytes, total 15 -> evict "a"

	if _, ok := cache.Get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}
	if _, ok := cache.Get("b"); !ok {
		t.Error("expected 'b' to still be cached")
	}
	if _, ok := cache.Get("c"); !ok {
		t.Error("expected 'c' to still be cached")
	}
}

func TestTileCache_Clear(t *testing.T) {
	cache := NewTileCache(1024)
	cache.Put("a", []byte("data"))
	cache.Clear()

	if _, ok := cache.Get("a"); ok {
		t.Error("expected cache to be empty after clear")
	}
}
