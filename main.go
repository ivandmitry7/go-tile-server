package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"go-tile-server/downloader"
	"go-tile-server/handlers"
)

//go:embed web
var webFS embed.FS

func main() {
	tileDir := filepath.Join(".", "tiles")
	if err := os.MkdirAll(tileDir, 0755); err != nil {
		log.Fatalf("Failed to create tiles directory: %v", err)
	}

	manager := downloader.NewManager(tileDir)

	api := &handlers.APIHandler{Manager: manager}
	tileHandler := &handlers.TileHandler{TileDir: tileDir}
	drawingsHandler := &handlers.DrawingsHandler{Dir: filepath.Join(".", "drawings")}

	// Serve embedded web UI
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Failed to access embedded web files: %v", err)
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/download", api.HandleDownload)
	mux.HandleFunc("/api/progress", api.HandleProgress)
	mux.HandleFunc("/api/tasks", api.HandleTasks)
	mux.HandleFunc("/api/cancel", api.HandleCancel)
	mux.HandleFunc("/api/count", api.HandleCount)
	mux.HandleFunc("/api/dedup", api.HandleDedup)
	mux.HandleFunc("/api/drawings", drawingsHandler.HandleDrawings)
	mux.HandleFunc("/api/tile-size", tileHandler.HandleTileSize)
	mux.HandleFunc("/api/export-tiles", tileHandler.HandleExportTiles)
	mux.HandleFunc("/api/import-tiles", tileHandler.HandleImportTiles)

	// Tile server
	mux.Handle("/tiles/", tileHandler)

	// Web UI
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	fmt.Printf("===========================================\n")
	fmt.Printf("  Google Offline Tile Server\n")
	fmt.Printf("  http://localhost:%s\n", port)
	fmt.Printf("  Tiles directory: %s\n", tileDir)
	fmt.Printf("===========================================\n")

	log.Fatal(http.ListenAndServe(":"+port, mux))
}
