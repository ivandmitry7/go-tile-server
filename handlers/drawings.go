package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type DrawingsHandler struct {
	Dir string
}

func (h *DrawingsHandler) HandleDrawings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPost:
		h.handlePost(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *DrawingsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	if name == "" {
		// List all saved drawings
		entries, err := os.ReadDir(h.Dir)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]string{})
			return
		}
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				names = append(names, strings.TrimSuffix(e.Name(), ".json"))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
		return
	}

	// Load specific drawing
	filename := sanitizeFilename(name) + ".json"
	data, err := os.ReadFile(filepath.Join(h.Dir, filename))
	if err != nil {
		http.Error(w, "drawing not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *DrawingsHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	name, ok := config["name"].(string)
	if !ok || name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(h.Dir, 0755); err != nil {
		http.Error(w, "failed to create drawings directory", http.StatusInternalServerError)
		return
	}

	filename := sanitizeFilename(name) + ".json"
	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(h.Dir, filename), data, 0644); err != nil {
		http.Error(w, "failed to save", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "saved", "name": name})
}

func (h *DrawingsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	filename := sanitizeFilename(name) + ".json"
	path := filepath.Join(h.Dir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.Error(w, "drawing not found", http.StatusNotFound)
		return
	}

	if err := os.Remove(path); err != nil {
		http.Error(w, "failed to delete", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)

func sanitizeFilename(name string) string {
	s := safeNameRe.ReplaceAllString(name, "_")
	if s == "" {
		s = "unnamed"
	}
	return s
}
