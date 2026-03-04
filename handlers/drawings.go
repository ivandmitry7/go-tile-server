package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

type DrawingsHandler struct {
	Dir string
}

func (h *DrawingsHandler) HandleGetDrawings(c *gin.Context) {
	name := c.Query("name")

	if name == "" {
		entries, err := os.ReadDir(h.Dir)
		if err != nil {
			c.JSON(http.StatusOK, []string{})
			return
		}
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				names = append(names, strings.TrimSuffix(e.Name(), ".json"))
			}
		}
		c.JSON(http.StatusOK, names)
		return
	}

	filename := sanitizeFilename(name) + ".json"
	data, err := os.ReadFile(filepath.Join(h.Dir, filename))
	if err != nil {
		c.String(http.StatusNotFound, "drawing not found")
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

func (h *DrawingsHandler) HandlePostDrawings(c *gin.Context) {
	var config map[string]interface{}
	if err := json.NewDecoder(c.Request.Body).Decode(&config); err != nil {
		c.String(http.StatusBadRequest, "invalid JSON")
		return
	}

	name, ok := config["name"].(string)
	if !ok || name == "" {
		c.String(http.StatusBadRequest, "name is required")
		return
	}

	if err := os.MkdirAll(h.Dir, 0755); err != nil {
		c.String(http.StatusInternalServerError, "failed to create drawings directory")
		return
	}

	filename := sanitizeFilename(name) + ".json"
	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(h.Dir, filename), data, 0644); err != nil {
		c.String(http.StatusInternalServerError, "failed to save")
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "saved", "name": name})
}

func (h *DrawingsHandler) HandleDeleteDrawings(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.String(http.StatusBadRequest, "name is required")
		return
	}

	filename := sanitizeFilename(name) + ".json"
	path := filepath.Join(h.Dir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "drawing not found")
		return
	}

	if err := os.Remove(path); err != nil {
		c.String(http.StatusInternalServerError, "failed to delete")
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)

func sanitizeFilename(name string) string {
	s := safeNameRe.ReplaceAllString(name, "_")
	if s == "" {
		s = "unnamed"
	}
	return s
}
