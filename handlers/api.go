package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"go-tile-server/downloader"
)

type APIHandler struct {
	Manager *downloader.Manager
}

type DownloadRequest struct {
	Polygon   []downloader.Point   `json:"polygon"`
	Holes     [][]downloader.Point `json:"holes"`
	ZoomMin   int                  `json:"zoom_min"`
	ZoomMax   int                  `json:"zoom_max"`
	BaseLayer string               `json:"base_layer"`
}

func (h *APIHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	var req DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(req.Polygon) < 3 {
		http.Error(w, "polygon must have at least 3 points", http.StatusBadRequest)
		return
	}
	if req.ZoomMin < 0 || req.ZoomMin > 20 {
		http.Error(w, "invalid zoom_min (0-20)", http.StatusBadRequest)
		return
	}
	if req.ZoomMax < 0 || req.ZoomMax > 20 {
		http.Error(w, "invalid zoom_max (0-20)", http.StatusBadRequest)
		return
	}
	if req.ZoomMin > req.ZoomMax {
		http.Error(w, "zoom_min must be <= zoom_max", http.StatusBadRequest)
		return
	}

	task := h.Manager.StartDownload(req.Polygon, req.Holes, req.ZoomMin, req.ZoomMax, req.BaseLayer)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"task_id": task.ID,
		"total":   task.Total,
		"status":  task.Status,
	})
}

func (h *APIHandler) HandleProgress(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "missing task_id", http.StatusBadRequest)
		return
	}

	task := h.Manager.GetTask(taskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for {
		done := atomic.LoadInt64(&task.Done)
		errors := atomic.LoadInt64(&task.Errors)
		currentZoom := atomic.LoadInt64(&task.CurrentZoom)

		deduped := atomic.LoadInt64(&task.Deduped)

		data := fmt.Sprintf(`{"done":%d,"total":%d,"errors":%d,"deduped":%d,"status":"%s","current_zoom":%d,"zoom_min":%d,"zoom_max":%d}`,
			done, task.Total, errors, deduped, task.Status, currentZoom, task.ZoomMin, task.ZoomMax)

		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if task.Status == "completed" || task.Status == "cancelled" || task.Status == "error" {
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (h *APIHandler) HandleTasks(w http.ResponseWriter, r *http.Request) {
	tasks := h.Manager.ListTasks()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (h *APIHandler) HandleCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "missing task_id", http.StatusBadRequest)
		return
	}

	task := h.Manager.GetTask(taskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	task.Cancel()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cancelled",
	})
}

func (h *APIHandler) HandleCount(w http.ResponseWriter, r *http.Request) {
	var req DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(req.Polygon) < 3 {
		http.Error(w, "polygon must have at least 3 points", http.StatusBadRequest)
		return
	}

	count := downloader.CountTilesPolygon(req.Polygon, req.Holes, req.ZoomMin, req.ZoomMax)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{
		"count": count,
	})
}

func (h *APIHandler) HandleDedup(w http.ResponseWriter, r *http.Request) {
	result, err := h.Manager.DeduplicateTiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
