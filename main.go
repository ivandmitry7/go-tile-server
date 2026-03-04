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

	"github.com/gin-gonic/gin"
)

//go:embed web
var webFS embed.FS

func main() {
	tileDir := filepath.Join(".", "tiles")
	if err := os.MkdirAll(tileDir, 0755); err != nil {
		log.Fatalf("Failed to create tiles directory: %v", err)
	}

	manager := downloader.NewManager(tileDir)
	tileHandler := handlers.NewTileHandler(tileDir, 512*1024*1024) // 512MB cache
	drawingsHandler := &handlers.DrawingsHandler{Dir: filepath.Join(".", "drawings")}
	api := &handlers.APIHandler{Manager: manager, TileHandler: tileHandler}

	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Failed to access embedded web files: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// API routes
	r.POST("/api/download", api.HandleDownload)
	r.GET("/api/progress", api.HandleProgress)
	r.GET("/api/tasks", api.HandleTasks)
	r.GET("/api/cancel", api.HandleCancel)
	r.POST("/api/count", api.HandleCount)
	r.GET("/api/dedup", api.HandleDedup)
	r.GET("/api/tile-size", tileHandler.HandleTileSize)
	r.GET("/api/export-tiles", tileHandler.HandleExportTiles)
	r.POST("/api/import-tiles", tileHandler.HandleImportTiles)

	// Drawings
	r.GET("/api/drawings", drawingsHandler.HandleGetDrawings)
	r.POST("/api/drawings", drawingsHandler.HandlePostDrawings)
	r.DELETE("/api/drawings", drawingsHandler.HandleDeleteDrawings)

	// Tiles
	r.GET("/tiles/:z/:x/:filename", tileHandler.ServeTile)

	// Web UI — serve embedded files for unmatched routes
	staticFS := http.FS(webContent)
	r.NoRoute(func(c *gin.Context) {
		c.FileFromFS(c.Request.URL.Path, staticFS)
	})

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	fmt.Printf("===========================================\n")
	fmt.Printf("  Offline Tile Server (Gin)\n")
	fmt.Printf("  http://localhost:%s\n", port)
	fmt.Printf("  Tiles directory: %s\n", tileDir)
	fmt.Printf("  Tile cache: 512 MB\n")
	fmt.Printf("===========================================\n")

	log.Fatal(r.Run(":" + port))
}
