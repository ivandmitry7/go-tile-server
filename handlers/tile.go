package handlers

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type TileHandler struct {
	TileDir string
}

func (h *TileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse path: /tiles/{z}/{x}/{y}.png
	path := strings.TrimPrefix(r.URL.Path, "/tiles/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		http.Error(w, "invalid tile path", http.StatusBadRequest)
		return
	}

	z, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid zoom", http.StatusBadRequest)
		return
	}

	x, err := strconv.Atoi(parts[1])
	if err != nil {
		http.Error(w, "invalid x", http.StatusBadRequest)
		return
	}

	yStr := strings.TrimSuffix(parts[2], ".png")
	y, err := strconv.Atoi(yStr)
	if err != nil {
		http.Error(w, "invalid y", http.StatusBadRequest)
		return
	}

	tilePath := filepath.Join(h.TileDir, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))

	if _, err := os.Stat(tilePath); os.IsNotExist(err) {
		http.Error(w, "tile not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, tilePath)
}

func (h *TileHandler) HandleTileSize(w http.ResponseWriter, r *http.Request) {
	var totalSize int64
	var fileCount int64

	filepath.WalkDir(h.TileDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				totalSize += info.Size()
				fileCount++
			}
		}
		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{
		"size":  totalSize,
		"files": fileCount,
	})
}
