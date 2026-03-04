# Go Tile Server

A self-contained offline map tile server with a web-based management UI. Download, cache, and serve map tiles from multiple providers (Google Maps, OpenStreetMap, OpenTopoMap) for offline use.

Built as a single binary with an embedded web UI — no external dependencies required.

## Features

- **Polygon-based tile download** — draw polygons on the map to select download areas with hole/exclusion zone support
- **Multiple tile sources** — Google Roadmap, Satellite, Hybrid, Terrain, OpenStreetMap, OpenTopoMap
- **Tile deduplication** — SHA256-based dedup using hardlinks to save storage (effective for ocean/uniform areas)
- **Real-time progress** — Server-Sent Events (SSE) for live download progress with auto-reconnect on page reload
- **Offline tile serving** — standard `/{z}/{x}/{y}.png` URL format, compatible with Leaflet, OpenLayers, MapLibre GL
- **Project management** — save/load drawing configurations, export/import as JSON files
- **KML export/import** — exchange polygon data with Google Earth and other GIS tools
- **Annotations** — markers, polylines, and circle markers with custom icons, colors, and labels
- **API documentation** — built-in API docs page at `/doc.html`
- **Single binary** — web UI embedded via Go `embed`, no external files needed

## Quick Start

### Prerequisites

- Go 1.23+

### Build & Run

```bash
go build -o tile-server .
./tile-server
```

The server starts at [http://localhost:8080](http://localhost:8080).

Use the `PORT` environment variable to change the port:

```bash
PORT=3000 ./tile-server
```

### Directory Structure

```
tiles/       — downloaded tile cache (auto-created)
drawings/    — saved project configurations (auto-created)
```

## Usage

1. Open [http://localhost:8080](http://localhost:8080) in your browser
2. Select a base layer (Google Roadmap, Satellite, OSM, etc.)
3. Draw a polygon on the map to define the download area
4. Set zoom min/max range
5. Click **Download Tiles**
6. Monitor progress in the status bar

### Offline Mode

Toggle **Online/Offline** to switch between live tile sources and locally cached tiles. In offline mode, the map only displays tiles that have been downloaded.

### Tile Deduplication

Click the **Dedup** button next to storage info to scan existing tiles and replace duplicates with hardlinks. This is especially effective for areas with uniform tiles (ocean, desert).

### Projects

- **Save/Load** — persist polygon, holes, annotations, and map settings to the server
- **Export/Import** — download/upload project configurations as JSON files for sharing
- **KML Export/Import** — exchange data with Google Earth

## API

Full API documentation is available at `/doc.html` when the server is running.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/tiles/{z}/{x}/{y}.png` | Serve cached tile |
| POST | `/api/download` | Start tile download task |
| GET | `/api/progress?task_id=X` | SSE progress stream |
| GET | `/api/tasks` | List all download tasks |
| GET | `/api/cancel?task_id=X` | Cancel a download task |
| POST | `/api/count` | Count tiles in polygon |
| GET | `/api/dedup` | Deduplicate existing tiles |
| GET/POST/DELETE | `/api/drawings` | Project CRUD |
| GET | `/api/tile-size` | Storage usage info |

### Tile Integration

Use the tile server with any map library:

```javascript
// Leaflet
L.tileLayer('http://localhost:8080/tiles/{z}/{x}/{y}.png').addTo(map);

// OpenLayers
new ol.source.XYZ({ url: 'http://localhost:8080/tiles/{z}/{x}/{y}.png' });
```

## Project Structure

```
main.go                     — entry point, HTTP server setup
downloader/downloader.go    — tile download engine with dedup
handlers/api.go             — REST API handlers
handlers/tile.go            — tile serving + storage info
handlers/drawings.go        — project save/load
web/index.html              — main web UI (embedded)
web/doc.html                — API documentation page (embedded)
```

## License

[MIT](LICENSE)
