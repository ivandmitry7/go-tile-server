package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type BBox struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

type Task struct {
	ID          string    `json:"id"`
	BBox        BBox      `json:"bbox"`
	ZoomMin     int       `json:"zoom_min"`
	ZoomMax     int       `json:"zoom_max"`
	Total       int64     `json:"total"`
	Done        int64     `json:"done"`
	Errors      int64     `json:"errors"`
	Deduped     int64     `json:"deduped"`
	CurrentZoom int64     `json:"current_zoom"`
	Status      string    `json:"status"` // "running", "completed", "cancelled", "error"
	StartedAt   time.Time `json:"started_at"`
	polygon     []Point
	holes       [][]Point
	baseLayer   string
	cancel      context.CancelFunc
}

func (t *Task) Cancel() {
	if t.cancel != nil {
		t.cancel()
		t.Status = "cancelled"
	}
}

type Manager struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	tileDir  string
	client   *http.Client
	taskSeq  int64
	hashMu   sync.Mutex
	hashPool map[string]string // sha256 -> file path
}

func NewManager(tileDir string) *Manager {
	return &Manager{
		tasks:    make(map[string]*Task),
		tileDir:  tileDir,
		hashPool: make(map[string]string),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (m *Manager) GetTask(id string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[id]
}

func (m *Manager) ListTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, t)
	}
	return result
}

func (m *Manager) StartDownload(polygon []Point, holes [][]Point, zoomMin, zoomMax int, baseLayer string) *Task {
	ctx, cancel := context.WithCancel(context.Background())

	id := fmt.Sprintf("task-%d", atomic.AddInt64(&m.taskSeq, 1))
	bbox := polygonBBox(polygon)

	total := CountTilesPolygon(polygon, holes, zoomMin, zoomMax)

	task := &Task{
		ID:        id,
		BBox:      bbox,
		ZoomMin:   zoomMin,
		ZoomMax:   zoomMax,
		Total:     total,
		Status:    "running",
		StartedAt: time.Now(),
		polygon:   polygon,
		holes:     holes,
		baseLayer: baseLayer,
		cancel:    cancel,
	}

	m.mu.Lock()
	m.tasks[id] = task
	m.mu.Unlock()

	go m.runDownload(ctx, task)

	return task
}

func (m *Manager) runDownload(ctx context.Context, task *Task) {
	sem := make(chan struct{}, 8)

	var wg sync.WaitGroup

	for z := task.ZoomMin; z <= task.ZoomMax; z++ {
		atomic.StoreInt64(&task.CurrentZoom, int64(z))
		xMin, yMin, xMax, yMax := bboxToTileRange(task.BBox, z)

		for x := xMin; x <= xMax; x++ {
			for y := yMin; y <= yMax; y++ {
				if !tileIncluded(z, x, y, task.polygon, task.holes) {
					continue
				}

				select {
				case <-ctx.Done():
					wg.Wait()
					if task.Status != "cancelled" {
						task.Status = "cancelled"
					}
					return
				default:
				}

				wg.Add(1)
				sem <- struct{}{}

				go func(z, x, y int) {
					defer wg.Done()
					defer func() { <-sem }()

					if err := m.downloadTile(ctx, z, x, y, task); err != nil {
						atomic.AddInt64(&task.Errors, 1)
					}
					atomic.AddInt64(&task.Done, 1)
				}(z, x, y)
			}
		}
	}

	wg.Wait()

	if task.Status == "running" {
		task.Status = "completed"
	}
}

func tileURL(baseLayer string, x, y, z int) string {
	switch baseLayer {
	case "satellite":
		return fmt.Sprintf("https://mt1.google.com/vt/lyrs=s&x=%d&y=%d&z=%d", x, y, z)
	case "hybrid":
		return fmt.Sprintf("https://mt1.google.com/vt/lyrs=y&x=%d&y=%d&z=%d", x, y, z)
	case "terrain":
		return fmt.Sprintf("https://mt1.google.com/vt/lyrs=p&x=%d&y=%d&z=%d", x, y, z)
	case "osm":
		s := []string{"a", "b", "c"}[z%3]
		return fmt.Sprintf("https://%s.tile.openstreetmap.org/%d/%d/%d.png", s, z, x, y)
	case "osm_topo":
		s := []string{"a", "b", "c"}[z%3]
		return fmt.Sprintf("https://%s.tile.opentopomap.org/%d/%d/%d.png", s, z, x, y)
	default: // "roadmap" or empty
		return fmt.Sprintf("https://mt1.google.com/vt/lyrs=m&x=%d&y=%d&z=%d", x, y, z)
	}
}

func (m *Manager) downloadTile(ctx context.Context, z, x, y int, task *Task) error {
	tilePath := filepath.Join(m.tileDir, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))

	// Skip if already exists
	if _, err := os.Stat(tilePath); err == nil {
		return nil
	}

	url := tileURL(task.baseLayer, x, y, z)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for tile %d/%d/%d", resp.StatusCode, z, x, y)
	}

	// Read body into memory for hashing
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Compute hash for dedup
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	dir := filepath.Dir(tilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Check dedup pool
	m.hashMu.Lock()
	if existing, ok := m.hashPool[hashStr]; ok {
		m.hashMu.Unlock()
		// Try hardlink from existing identical tile
		if err := os.Link(existing, tilePath); err == nil {
			atomic.AddInt64(&task.Deduped, 1)
			return nil
		}
		// Fallback: write normally
		return os.WriteFile(tilePath, data, 0644)
	}
	m.hashMu.Unlock()

	// Write new unique tile
	if err := os.WriteFile(tilePath, data, 0644); err != nil {
		return err
	}

	// Add to pool
	m.hashMu.Lock()
	m.hashPool[hashStr] = tilePath
	m.hashMu.Unlock()

	return nil
}

// DeduplicateResult holds stats from a dedup scan
type DeduplicateResult struct {
	TotalFiles  int64 `json:"total_files"`
	UniqueFiles int64 `json:"unique_files"`
	Duplicates  int64 `json:"duplicates"`
	SavedBytes  int64 `json:"saved_bytes"`
}

// DeduplicateTiles scans existing tiles and replaces duplicates with hardlinks
func (m *Manager) DeduplicateTiles() (*DeduplicateResult, error) {
	hashMap := make(map[string]string) // hash -> first file path
	result := &DeduplicateResult{}

	err := filepath.Walk(m.tileDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || filepath.Ext(path) != ".png" {
			return nil
		}

		result.TotalFiles++

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])

		if existing, ok := hashMap[hashStr]; ok {
			// Duplicate found — replace with hardlink
			if err := os.Remove(path); err != nil {
				return nil
			}
			if err := os.Link(existing, path); err != nil {
				// Restore original file if hardlink fails
				os.WriteFile(path, data, 0644)
				return nil
			}
			result.Duplicates++
			result.SavedBytes += info.Size()
		} else {
			hashMap[hashStr] = path
			result.UniqueFiles++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Ray casting algorithm for point-in-polygon
func pointInPolygon(p Point, polygon []Point) bool {
	inside := false
	n := len(polygon)
	j := n - 1
	for i := 0; i < n; i++ {
		if ((polygon[i].Lng > p.Lng) != (polygon[j].Lng > p.Lng)) &&
			(p.Lat < (polygon[j].Lat-polygon[i].Lat)*(p.Lng-polygon[i].Lng)/(polygon[j].Lng-polygon[i].Lng)+polygon[i].Lat) {
			inside = !inside
		}
		j = i
	}
	return inside
}

// Check if tile intersects with polygon
func tileIntersectsPolygon(z, x, y int, polygon []Point) bool {
	n := math.Pow(2, float64(z))
	lonMin := float64(x)/n*360.0 - 180.0
	lonMax := float64(x+1)/n*360.0 - 180.0
	latMax := math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180.0 / math.Pi
	latMin := math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180.0 / math.Pi

	// Check tile corners and center
	checks := []Point{
		{latMin, lonMin}, {latMin, lonMax},
		{latMax, lonMin}, {latMax, lonMax},
		{(latMin + latMax) / 2, (lonMin + lonMax) / 2},
	}
	for _, p := range checks {
		if pointInPolygon(p, polygon) {
			return true
		}
	}

	// Check if any polygon vertex is inside the tile
	for _, p := range polygon {
		if p.Lat >= latMin && p.Lat <= latMax && p.Lng >= lonMin && p.Lng <= lonMax {
			return true
		}
	}

	return false
}

// Get bounding box from polygon
func polygonBBox(polygon []Point) BBox {
	bbox := BBox{
		South: polygon[0].Lat, North: polygon[0].Lat,
		West: polygon[0].Lng, East: polygon[0].Lng,
	}
	for _, p := range polygon[1:] {
		if p.Lat < bbox.South {
			bbox.South = p.Lat
		}
		if p.Lat > bbox.North {
			bbox.North = p.Lat
		}
		if p.Lng < bbox.West {
			bbox.West = p.Lng
		}
		if p.Lng > bbox.East {
			bbox.East = p.Lng
		}
	}
	return bbox
}

// lat/lon to tile number conversion (Slippy Map)
func latLonToTile(lat, lon float64, zoom int) (int, int) {
	n := math.Pow(2, float64(zoom))
	x := int(math.Floor((lon + 180.0) / 360.0 * n))
	latRad := lat * math.Pi / 180.0
	y := int(math.Floor((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n))

	maxTile := int(n) - 1
	if x < 0 {
		x = 0
	}
	if x > maxTile {
		x = maxTile
	}
	if y < 0 {
		y = 0
	}
	if y > maxTile {
		y = maxTile
	}

	return x, y
}

func bboxToTileRange(bbox BBox, zoom int) (xMin, yMin, xMax, yMax int) {
	x1, y1 := latLonToTile(bbox.North, bbox.West, zoom)
	x2, y2 := latLonToTile(bbox.South, bbox.East, zoom)

	xMin = min(x1, x2)
	xMax = max(x1, x2)
	yMin = min(y1, y2)
	yMax = max(y1, y2)

	return
}

// tileIncluded checks if tile is inside polygon but not inside any hole
func tileIncluded(z, x, y int, polygon []Point, holes [][]Point) bool {
	if !tileIntersectsPolygon(z, x, y, polygon) {
		return false
	}
	for _, hole := range holes {
		if tileFullyInPolygon(z, x, y, hole) {
			return false
		}
	}
	return true
}

// tileFullyInPolygon checks if all corners and center of a tile are inside a polygon
func tileFullyInPolygon(z, x, y int, polygon []Point) bool {
	n := math.Pow(2, float64(z))
	lonMin := float64(x)/n*360.0 - 180.0
	lonMax := float64(x+1)/n*360.0 - 180.0
	latMax := math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180.0 / math.Pi
	latMin := math.Atan(math.Sinh(math.Pi*(1-2*float64(y+1)/n))) * 180.0 / math.Pi

	corners := []Point{
		{latMin, lonMin}, {latMin, lonMax},
		{latMax, lonMin}, {latMax, lonMax},
		{(latMin + latMax) / 2, (lonMin + lonMax) / 2},
	}
	for _, p := range corners {
		if !pointInPolygon(p, polygon) {
			return false
		}
	}
	return true
}

func CountTilesPolygon(polygon []Point, holes [][]Point, zoomMin, zoomMax int) int64 {
	bbox := polygonBBox(polygon)
	total := int64(0)
	for z := zoomMin; z <= zoomMax; z++ {
		xMin, yMin, xMax, yMax := bboxToTileRange(bbox, z)
		for x := xMin; x <= xMax; x++ {
			for y := yMin; y <= yMax; y++ {
				if tileIncluded(z, x, y, polygon, holes) {
					total++
				}
			}
		}
	}
	return total
}
