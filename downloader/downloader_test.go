package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// --- pointInPolygon ---

func TestPointInPolygon_Inside(t *testing.T) {
	square := []Point{{0, 0}, {0, 10}, {10, 10}, {10, 0}}
	if !pointInPolygon(Point{5, 5}, square) {
		t.Error("expected point (5,5) to be inside square")
	}
}

func TestPointInPolygon_Outside(t *testing.T) {
	square := []Point{{0, 0}, {0, 10}, {10, 10}, {10, 0}}
	if pointInPolygon(Point{15, 5}, square) {
		t.Error("expected point (15,5) to be outside square")
	}
}

func TestPointInPolygon_OnEdge(t *testing.T) {
	square := []Point{{0, 0}, {0, 10}, {10, 10}, {10, 0}}
	// Edge behavior is implementation-defined for ray casting; just ensure no panic
	_ = pointInPolygon(Point{0, 5}, square)
}

func TestPointInPolygon_Triangle(t *testing.T) {
	tri := []Point{{0, 0}, {5, 10}, {10, 0}}
	if !pointInPolygon(Point{5, 3}, tri) {
		t.Error("expected point inside triangle")
	}
	if pointInPolygon(Point{0, 10}, tri) {
		t.Error("expected point outside triangle")
	}
}

// --- polygonBBox ---

func TestPolygonBBox(t *testing.T) {
	polygon := []Point{{10, 20}, {30, 40}, {50, 60}}
	bbox := polygonBBox(polygon)

	if bbox.South != 10 {
		t.Errorf("expected South=10, got %f", bbox.South)
	}
	if bbox.North != 50 {
		t.Errorf("expected North=50, got %f", bbox.North)
	}
	if bbox.West != 20 {
		t.Errorf("expected West=20, got %f", bbox.West)
	}
	if bbox.East != 60 {
		t.Errorf("expected East=60, got %f", bbox.East)
	}
}

func TestPolygonBBox_SinglePoint(t *testing.T) {
	polygon := []Point{{10, 20}}
	bbox := polygonBBox(polygon)
	if bbox.South != 10 || bbox.North != 10 || bbox.West != 20 || bbox.East != 20 {
		t.Errorf("single point bbox should have equal bounds, got %+v", bbox)
	}
}

// --- latLonToTile ---

func TestLatLonToTile_Origin(t *testing.T) {
	// At zoom 0, everything maps to tile 0,0
	x, y := latLonToTile(0, 0, 0)
	if x != 0 || y != 0 {
		t.Errorf("expected (0,0) at zoom 0, got (%d,%d)", x, y)
	}
}

func TestLatLonToTile_Zoom1(t *testing.T) {
	// At zoom 1, (0,0) should be tile (1,1)
	x, y := latLonToTile(0, 0, 1)
	if x != 1 || y != 1 {
		t.Errorf("expected (1,1) at zoom 1 for (0,0), got (%d,%d)", x, y)
	}
}

func TestLatLonToTile_Clamp(t *testing.T) {
	// Extreme values should be clamped
	x, y := latLonToTile(85.1, 180.1, 2)
	maxTile := int(math.Pow(2, 2)) - 1
	if x > maxTile || y > maxTile || x < 0 || y < 0 {
		t.Errorf("tile coords should be clamped, got (%d,%d), max=%d", x, y, maxTile)
	}
}

// --- bboxToTileRange ---

func TestBboxToTileRange(t *testing.T) {
	bbox := BBox{South: 10, West: 100, North: 20, East: 110}
	xMin, yMin, xMax, yMax := bboxToTileRange(bbox, 5)

	if xMin > xMax {
		t.Errorf("xMin (%d) should be <= xMax (%d)", xMin, xMax)
	}
	if yMin > yMax {
		t.Errorf("yMin (%d) should be <= yMax (%d)", yMin, yMax)
	}
}

// --- tileIntersectsPolygon ---

func TestTileIntersectsPolygon_Inside(t *testing.T) {
	// Large polygon covering most of the world
	polygon := []Point{{-80, -170}, {-80, 170}, {80, 170}, {80, -170}}
	if !tileIntersectsPolygon(1, 0, 0, polygon) {
		t.Error("tile (1,0,0) should intersect large polygon")
	}
}

func TestTileIntersectsPolygon_Outside(t *testing.T) {
	// Small polygon near equator
	polygon := []Point{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
	// Tile at zoom 1, position (0,0) covers NW quarter — check a distant tile
	if tileIntersectsPolygon(5, 0, 0, polygon) {
		t.Error("tile (5,0,0) should not intersect small equatorial polygon")
	}
}

// --- tileIncluded ---

func TestTileIncluded_NoHoles(t *testing.T) {
	polygon := []Point{{-80, -170}, {-80, 170}, {80, 170}, {80, -170}}
	if !tileIncluded(1, 0, 0, polygon, nil) {
		t.Error("tile should be included with no holes")
	}
}

func TestTileIncluded_WithHole(t *testing.T) {
	polygon := []Point{{-80, -170}, {-80, 170}, {80, 170}, {80, -170}}
	// Hole covering tile (1,0,0) area
	hole := []Point{{0, -180}, {0, 0}, {85, 0}, {85, -180}}
	result := tileIncluded(1, 0, 0, polygon, [][]Point{hole})
	// Result depends on whether tile is fully inside hole
	_ = result // no panic
}

// --- tileFullyInPolygon ---

func TestTileFullyInPolygon(t *testing.T) {
	// Large polygon — use zoom 1 tile (1,1) which covers ~0 to -85 lat, 0 to 180 lon
	polygon := []Point{{-86, -1}, {-86, 181}, {1, 181}, {1, -1}}
	if !tileFullyInPolygon(1, 1, 1, polygon) {
		t.Error("tile (1,1,1) should be fully inside polygon")
	}
}

func TestTileFullyInPolygon_PartiallyOutside(t *testing.T) {
	// Small polygon — tile at zoom 0 covers the world, so not fully inside
	polygon := []Point{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
	if tileFullyInPolygon(0, 0, 0, polygon) {
		t.Error("world tile should not be fully inside small polygon")
	}
}

// --- CountTilesPolygon ---

func TestCountTilesPolygon_SingleZoom(t *testing.T) {
	polygon := []Point{{10, 100}, {10, 110}, {20, 110}, {20, 100}}
	count := CountTilesPolygon(polygon, nil, 5, 5)
	if count <= 0 {
		t.Errorf("expected positive tile count, got %d", count)
	}
}

func TestCountTilesPolygon_ZoomRange(t *testing.T) {
	polygon := []Point{{10, 100}, {10, 110}, {20, 110}, {20, 100}}
	countSingle := CountTilesPolygon(polygon, nil, 5, 5)
	countRange := CountTilesPolygon(polygon, nil, 4, 5)
	if countRange <= countSingle {
		t.Errorf("wider zoom range should have more tiles: range=%d, single=%d", countRange, countSingle)
	}
}

func TestCountTilesPolygon_WithHoles(t *testing.T) {
	polygon := []Point{{10, 100}, {10, 110}, {20, 110}, {20, 100}}
	hole := []Point{{13, 103}, {13, 107}, {17, 107}, {17, 103}}
	// Use higher zoom so tiles are smaller than the hole
	countNoHole := CountTilesPolygon(polygon, nil, 8, 8)
	countWithHole := CountTilesPolygon(polygon, [][]Point{hole}, 8, 8)
	if countWithHole >= countNoHole {
		t.Errorf("hole should reduce tile count: without=%d, with=%d", countNoHole, countWithHole)
	}
}

// --- tileURL ---

func TestTileURL_Roadmap(t *testing.T) {
	url := tileURL("roadmap", 1, 2, 3)
	expected := "https://mt1.google.com/vt/lyrs=m&x=1&y=2&z=3"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestTileURL_Default(t *testing.T) {
	url := tileURL("", 1, 2, 3)
	if url != "https://mt1.google.com/vt/lyrs=m&x=1&y=2&z=3" {
		t.Errorf("empty base layer should default to roadmap, got %s", url)
	}
}

func TestTileURL_Satellite(t *testing.T) {
	url := tileURL("satellite", 5, 10, 8)
	if url != "https://mt1.google.com/vt/lyrs=s&x=5&y=10&z=8" {
		t.Errorf("unexpected satellite URL: %s", url)
	}
}

func TestTileURL_Hybrid(t *testing.T) {
	url := tileURL("hybrid", 1, 2, 3)
	if url != "https://mt1.google.com/vt/lyrs=y&x=1&y=2&z=3" {
		t.Errorf("unexpected hybrid URL: %s", url)
	}
}

func TestTileURL_Terrain(t *testing.T) {
	url := tileURL("terrain", 1, 2, 3)
	if url != "https://mt1.google.com/vt/lyrs=p&x=1&y=2&z=3" {
		t.Errorf("unexpected terrain URL: %s", url)
	}
}

func TestTileURL_OSM(t *testing.T) {
	url := tileURL("osm", 1, 2, 3)
	// z%3=0 -> "a"
	if url != "https://a.tile.openstreetmap.org/3/1/2.png" {
		t.Errorf("unexpected OSM URL: %s", url)
	}
}

func TestTileURL_OSMTopo(t *testing.T) {
	url := tileURL("osm_topo", 1, 2, 4)
	// z%3=1 -> "b"
	if url != "https://b.tile.opentopomap.org/4/1/2.png" {
		t.Errorf("unexpected OSM topo URL: %s", url)
	}
}

// --- Manager ---

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/test-tiles")
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.tileDir != "/tmp/test-tiles" {
		t.Errorf("expected tileDir=/tmp/test-tiles, got %s", m.tileDir)
	}
	if m.tasks == nil || m.hashPool == nil {
		t.Error("maps should be initialized")
	}
}

func TestManager_GetTask_NotFound(t *testing.T) {
	m := NewManager("/tmp/test-tiles")
	if m.GetTask("nonexistent") != nil {
		t.Error("expected nil for nonexistent task")
	}
}

func TestManager_ListTasks_Empty(t *testing.T) {
	m := NewManager("/tmp/test-tiles")
	tasks := m.ListTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestTask_Cancel(t *testing.T) {
	called := false
	task := &Task{
		Status: "running",
		cancel: func() { called = true },
	}
	task.Cancel()
	if !called {
		t.Error("cancel function should have been called")
	}
	if task.Status != "cancelled" {
		t.Errorf("expected status=cancelled, got %s", task.Status)
	}
}

func TestTask_Cancel_NilFunc(t *testing.T) {
	task := &Task{Status: "running"}
	task.Cancel() // should not panic
	if task.Status != "running" {
		t.Error("status should remain unchanged when cancel is nil")
	}
}

// --- DeduplicateTiles ---

func TestDeduplicateTiles(t *testing.T) {
	dir := t.TempDir()

	// Create 3 files: 2 identical, 1 different
	data1 := []byte("identical tile content")
	data2 := []byte("different tile content")

	os.MkdirAll(filepath.Join(dir, "1", "0"), 0755)
	os.MkdirAll(filepath.Join(dir, "1", "1"), 0755)

	os.WriteFile(filepath.Join(dir, "1", "0", "0.png"), data1, 0644)
	os.WriteFile(filepath.Join(dir, "1", "1", "0.png"), data1, 0644) // duplicate
	os.WriteFile(filepath.Join(dir, "1", "0", "1.png"), data2, 0644) // different

	m := NewManager(dir)
	result, err := m.DeduplicateTiles()
	if err != nil {
		t.Fatalf("DeduplicateTiles failed: %v", err)
	}

	if result.TotalFiles != 3 {
		t.Errorf("expected 3 total files, got %d", result.TotalFiles)
	}
	if result.UniqueFiles != 2 {
		t.Errorf("expected 2 unique files, got %d", result.UniqueFiles)
	}
	if result.Duplicates != 1 {
		t.Errorf("expected 1 duplicate, got %d", result.Duplicates)
	}
	if result.SavedBytes <= 0 {
		t.Error("expected positive saved bytes")
	}

	// Verify files still readable and identical
	content1, _ := os.ReadFile(filepath.Join(dir, "1", "0", "0.png"))
	content2, _ := os.ReadFile(filepath.Join(dir, "1", "1", "0.png"))
	h1 := sha256.Sum256(content1)
	h2 := sha256.Sum256(content2)
	if hex.EncodeToString(h1[:]) != hex.EncodeToString(h2[:]) {
		t.Error("deduplicated files should have identical content")
	}
}

func TestDeduplicateTiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	result, err := m.DeduplicateTiles()
	if err != nil {
		t.Fatalf("DeduplicateTiles failed: %v", err)
	}
	if result.TotalFiles != 0 {
		t.Errorf("expected 0 files, got %d", result.TotalFiles)
	}
}

func TestDeduplicateTiles_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "1", "0"), 0755)
	os.WriteFile(filepath.Join(dir, "1", "0", "0.png"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "1", "0", "1.png"), []byte("bbb"), 0644)

	m := NewManager(dir)
	result, err := m.DeduplicateTiles()
	if err != nil {
		t.Fatalf("DeduplicateTiles failed: %v", err)
	}
	if result.Duplicates != 0 {
		t.Errorf("expected 0 duplicates, got %d", result.Duplicates)
	}
	if result.UniqueFiles != 2 {
		t.Errorf("expected 2 unique, got %d", result.UniqueFiles)
	}
}
