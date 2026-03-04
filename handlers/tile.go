package handlers

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
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

func (h *TileHandler) HandleExportTiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="tiles.tar.gz"`)

	gzw := gzip.NewWriter(w)
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

func (h *TileHandler) HandleImportTiles(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(2 << 30) // 2GB max
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		http.Error(w, "invalid gzip", http.StatusBadRequest)
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
			http.Error(w, "invalid tar", http.StatusBadRequest)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"files":  count,
	})
}
