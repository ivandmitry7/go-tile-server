package handlers

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"go-tile-server/downloader"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	Manager     *downloader.Manager
	TileHandler *TileHandler
}

type DownloadRequest struct {
	Polygon   []downloader.Point   `json:"polygon"`
	Holes     [][]downloader.Point `json:"holes"`
	ZoomMin   int                  `json:"zoom_min"`
	ZoomMax   int                  `json:"zoom_max"`
	BaseLayer string               `json:"base_layer"`
}

func (h *APIHandler) HandleDownload(c *gin.Context) {
	var req DownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Polygon) < 3 {
		c.String(http.StatusBadRequest, "polygon must have at least 3 points")
		return
	}
	if req.ZoomMin < 0 || req.ZoomMin > 20 {
		c.String(http.StatusBadRequest, "invalid zoom_min (0-20)")
		return
	}
	if req.ZoomMax < 0 || req.ZoomMax > 20 {
		c.String(http.StatusBadRequest, "invalid zoom_max (0-20)")
		return
	}
	if req.ZoomMin > req.ZoomMax {
		c.String(http.StatusBadRequest, "zoom_min must be <= zoom_max")
		return
	}

	task := h.Manager.StartDownload(req.Polygon, req.Holes, req.ZoomMin, req.ZoomMax, req.BaseLayer)

	// Invalidate tile-size cache when download starts
	if h.TileHandler != nil {
		h.TileHandler.InvalidateSizeCache()
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"total":   task.Total,
		"status":  task.Status,
	})
}

func (h *APIHandler) HandleProgress(c *gin.Context) {
	taskID := c.Query("task_id")
	if taskID == "" {
		c.String(http.StatusBadRequest, "missing task_id")
		return
	}

	task := h.Manager.GetTask(taskID)
	if task == nil {
		c.String(http.StatusNotFound, "task not found")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	c.Stream(func(w io.Writer) bool {
		done := atomic.LoadInt64(&task.Done)
		errors := atomic.LoadInt64(&task.Errors)
		currentZoom := atomic.LoadInt64(&task.CurrentZoom)
		deduped := atomic.LoadInt64(&task.Deduped)

		data := fmt.Sprintf(`{"done":%d,"total":%d,"errors":%d,"deduped":%d,"status":"%s","current_zoom":%d,"zoom_min":%d,"zoom_max":%d}`,
			done, task.Total, errors, deduped, task.Status, currentZoom, task.ZoomMin, task.ZoomMax)

		fmt.Fprintf(w, "data: %s\n\n", data)

		if task.Status == "completed" || task.Status == "cancelled" || task.Status == "error" {
			if h.TileHandler != nil {
				h.TileHandler.InvalidateSizeCache()
			}
			return false
		}

		select {
		case <-c.Request.Context().Done():
			return false
		case <-time.After(500 * time.Millisecond):
		}
		return true
	})
}

func (h *APIHandler) HandleTasks(c *gin.Context) {
	c.JSON(http.StatusOK, h.Manager.ListTasks())
}

func (h *APIHandler) HandleCancel(c *gin.Context) {
	taskID := c.Query("task_id")
	if taskID == "" {
		c.String(http.StatusBadRequest, "missing task_id")
		return
	}

	task := h.Manager.GetTask(taskID)
	if task == nil {
		c.String(http.StatusNotFound, "task not found")
		return
	}

	task.Cancel()
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func (h *APIHandler) HandleCount(c *gin.Context) {
	var req DownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Polygon) < 3 {
		c.String(http.StatusBadRequest, "polygon must have at least 3 points")
		return
	}

	count := downloader.CountTilesPolygon(req.Polygon, req.Holes, req.ZoomMin, req.ZoomMax)
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *APIHandler) HandleDedup(c *gin.Context) {
	result, err := h.Manager.DeduplicateTiles()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	if h.TileHandler != nil {
		h.TileHandler.InvalidateSizeCache()
	}

	c.JSON(http.StatusOK, result)
}
