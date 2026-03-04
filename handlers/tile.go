package handlers

import (
	"archive/tar"
	"compress/gzip"
	"container/list"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// --- LRU Tile Cache ---

type tileEntry struct {
	key  string
	data []byte
}

type TileCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	order    *list.List
	maxBytes int64
	curBytes int64
}

func NewTileCache(maxBytes int64) *TileCache {
	return &TileCache{
		items:    make(map[string]*list.Element),
		order:    list.New(),
		maxBytes: maxBytes,
	}
}

func (c *TileCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*tileEntry).data, true
	}
	return nil, false
}

func (c *TileCache) Put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		old := el.Value.(*tileEntry)
		c.curBytes += int64(len(data)) - int64(len(old.data))
		old.data = data
	} else {
		el := c.order.PushFront(&tileEntry{key: key, data: data})
		c.items[key] = el
		c.curBytes += int64(len(data))
	}

	for c.curBytes > c.maxBytes && c.order.Len() > 0 {
		oldest := c.order.Back()
		entry := oldest.Value.(*tileEntry)
		c.order.Remove(oldest)
		delete(c.items, entry.key)
		c.curBytes -= int64(len(entry.data))
	}
}

func (c *TileCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.curBytes = 0
}

// --- Tile Size Cache ---

type tileSizeCache struct {
	mu    sync.RWMutex
	size  int64
	files int64
	valid bool
}

// --- TileHandler ---

type TileHandler struct {
	TileDir   string
	Cache     *TileCache
	sizeCache tileSizeCache
}

func NewTileHandler(tileDir string, cacheBytes int64) *TileHandler {
	return &TileHandler{
		TileDir: tileDir,
		Cache:   NewTileCache(cacheBytes),
	}
}

func (h *TileHandler) ServeTile(c *gin.Context) {
	z := c.Param("z")
	x := c.Param("x")
	filename := c.Param("filename")

	if !strings.HasSuffix(filename, ".png") {
		c.String(http.StatusBadRequest, "invalid tile path")
		return
	}
	y := strings.TrimSuffix(filename, ".png")

	cacheKey := z + "/" + x + "/" + y

	// Check cache first
	if data, ok := h.Cache.Get(cacheKey); ok {
		c.Header("Content-Type", "image/png")
		c.Header("Cache-Control", "public, max-age=86400")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Data(http.StatusOK, "image/png", data)
		return
	}

	tilePath := filepath.Join(h.TileDir, z, x, filename)

	data, err := os.ReadFile(tilePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.String(http.StatusNotFound, "tile not found")
		} else {
			c.String(http.StatusInternalServerError, "read error")
		}
		return
	}

	// Store in cache
	h.Cache.Put(cacheKey, data)

	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Data(http.StatusOK, "image/png", data)
}

func (h *TileHandler) HandleTileSize(c *gin.Context) {
	h.sizeCache.mu.RLock()
	if h.sizeCache.valid {
		size, files := h.sizeCache.size, h.sizeCache.files
		h.sizeCache.mu.RUnlock()
		c.JSON(http.StatusOK, gin.H{"size": size, "files": files})
		return
	}
	h.sizeCache.mu.RUnlock()

	// Compute and cache
	var totalSize, fileCount int64
	filepath.WalkDir(h.TileDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})

	h.sizeCache.mu.Lock()
	h.sizeCache.size = totalSize
	h.sizeCache.files = fileCount
	h.sizeCache.valid = true
	h.sizeCache.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"size": totalSize, "files": fileCount})
}

func (h *TileHandler) InvalidateSizeCache() {
	h.sizeCache.mu.Lock()
	h.sizeCache.valid = false
	h.sizeCache.mu.Unlock()
}

func (h *TileHandler) HandleExportTiles(c *gin.Context) {
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", `attachment; filename="tiles.tar.gz"`)

	gzw := gzip.NewWriter(c.Writer)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	filepath.WalkDir(h.TileDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(h.TileDir, path)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		header := &tar.Header{
			Name: rel,
			Size: info.Size(),
			Mode: 0644,
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		io.Copy(tw, f)
		return nil
	})
}

func (h *TileHandler) HandleImportTiles(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid gzip")
		return
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var count int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.String(http.StatusBadRequest, "invalid tar")
			return
		}

		clean := filepath.Clean(header.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}

		target := filepath.Join(h.TileDir, clean)
		if header.Typeflag == tar.TypeDir {
			os.MkdirAll(target, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(target), 0755)
		f, err := os.Create(target)
		if err != nil {
			continue
		}
		io.Copy(f, tr)
		f.Close()
		count++
	}

	h.Cache.Clear()
	h.InvalidateSizeCache()

	c.JSON(http.StatusOK, gin.H{"status": "ok", "files": count})
}
