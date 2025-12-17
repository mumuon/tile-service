package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/mvt"
	"github.com/paulmach/orb/maptile"
)

// RoadGeometry represents a road's bounding box extracted from tiles
type RoadGeometry struct {
	RoadID    string  `json:"roadId"`
	Region    string  `json:"region"`
	MinLat    float64 `json:"minLat"`
	MaxLat    float64 `json:"maxLat"`
	MinLng    float64 `json:"minLng"`
	MaxLng    float64 `json:"maxLng"`
	Curvature *string `json:"curvature,omitempty"`
}

// ExtractionProgress tracks the extraction process
type ExtractionProgress struct {
	Region           string  `json:"region"`
	TotalTiles       int     `json:"totalTiles"`
	ProcessedTiles   int     `json:"processedTiles"`
	ExtractedRoads   int     `json:"extractedRoads"`
	LastProcessedTile *string `json:"lastProcessedTile"`
	StartedAt        int64   `json:"startedAt"`
	Status           string  `json:"status"` // "extracting", "inserting", "complete"
}

// GeometryExtractor handles road geometry extraction from vector tiles
type GeometryExtractor struct {
	logger *slog.Logger
}

// NewGeometryExtractor creates a new geometry extractor
func NewGeometryExtractor() *GeometryExtractor {
	return &GeometryExtractor{
		logger: slog.Default(),
	}
}

// ExtractRoadGeometriesFromTiles extracts road bounding boxes from all tiles in a directory
func (e *GeometryExtractor) ExtractRoadGeometriesFromTiles(ctx context.Context, tilesDir, region string) ([]RoadGeometry, error) {
	logger := e.logger.With("region", region, "tiles_dir", tilesDir)
	logger.Info("starting road geometry extraction from tiles")

	// Find all .pbf files
	pbfFiles, err := e.findPBFFiles(tilesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find PBF files: %w", err)
	}

	logger.Info("found tiles", "count", len(pbfFiles))

	// Load or initialize progress
	progress := e.loadProgress(region)
	if progress == nil {
		progress = &ExtractionProgress{
			Region:         region,
			TotalTiles:     len(pbfFiles),
			ProcessedTiles: 0,
			ExtractedRoads: 0,
			StartedAt:      int64(os.Getpid()),
			Status:         "extracting",
		}
		logger.Info("starting new extraction")
	} else {
		logger.Info("resuming extraction", "processed", progress.ProcessedTiles, "total", progress.TotalTiles)
	}

	// Map to deduplicate roads across tiles
	roadsMap := make(map[string]*RoadGeometry)

	// Track invalid roads with zero coordinates
	invalidRoadCount := 0

	// Load existing roads from extraction file if resuming
	extractionFile := e.getExtractionFile(region)
	if progress.ProcessedTiles > 0 {
		existingRoads, err := e.loadRoadsFromFile(extractionFile)
		if err != nil {
			logger.Warn("failed to load existing roads", "error", err)
		} else {
			for _, road := range existingRoads {
				key := fmt.Sprintf("%s_%s", road.RoadID, road.Region)
				roadsMap[key] = &road
			}
			logger.Info("loaded existing roads", "count", len(existingRoads))
		}
	}

	// Find starting index
	startIndex := 0
	if progress.LastProcessedTile != nil {
		for i, file := range pbfFiles {
			if file == *progress.LastProcessedTile {
				startIndex = i + 1
				break
			}
		}
	}

	// Process tiles
	for i := startIndex; i < len(pbfFiles); i++ {
		select {
		case <-ctx.Done():
			logger.Info("extraction cancelled")
			e.saveProgress(progress)
			return nil, ctx.Err()
		default:
		}

		pbfFile := pbfFiles[i]

		// Extract tile coordinates from path
		tileCoords, err := e.parseTilePath(pbfFile)
		if err != nil {
			logger.Warn("failed to parse tile path", "file", pbfFile, "error", err)
			continue
		}

		// Process tile
		roads, invalidCountFromTile, err := e.extractRoadsFromTile(pbfFile, region, tileCoords)
		if err != nil {
			logger.Warn("failed to extract from tile", "file", pbfFile, "error", err)
			continue
		}

		// Track invalid roads
		invalidRoadCount += invalidCountFromTile

		// Fail fast if we're seeing too many invalid roads
		if invalidRoadCount > 100 {
			return nil, fmt.Errorf("ABORTING: found %d roads with zero coordinates - this indicates a bug in calculateBounds()", invalidRoadCount)
		}

		// Merge roads into map
		for _, road := range roads {
			key := fmt.Sprintf("%s_%s", road.RoadID, road.Region)
			existing, exists := roadsMap[key]
			if exists {
				// Expand bounding box
				existing.MinLat = math.Min(existing.MinLat, road.MinLat)
				existing.MaxLat = math.Max(existing.MaxLat, road.MaxLat)
				existing.MinLng = math.Min(existing.MinLng, road.MinLng)
				existing.MaxLng = math.Max(existing.MaxLng, road.MaxLng)
			} else {
				roadsMap[key] = &road
				progress.ExtractedRoads++
			}
		}

		progress.ProcessedTiles++
		progress.LastProcessedTile = &pbfFile

		// Log progress every 500 tiles (no file I/O during extraction for speed)
		if progress.ProcessedTiles%500 == 0 {
			logger.Info("progress checkpoint",
				"processed", progress.ProcessedTiles,
				"total", progress.TotalTiles,
				"roads", progress.ExtractedRoads)
		}
	}

	// Convert map to slice
	result := make([]RoadGeometry, 0, len(roadsMap))
	for _, road := range roadsMap {
		result = append(result, *road)
	}

	// Save final results
	progress.Status = "complete"
	e.saveProgress(progress)
	e.saveRoadsToFile(extractionFile, roadsMap)

	logger.Info("extraction complete", "roads_extracted", len(result))
	return result, nil
}

// extractRoadsFromTile extracts roads from a single tile file
// Returns: roads slice, count of invalid roads (with zero coordinates), error
func (e *GeometryExtractor) extractRoadsFromTile(pbfFile, region string, tileCoords maptile.Tile) ([]RoadGeometry, int, error) {
	// Read tile file
	data, err := os.ReadFile(pbfFile)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read tile: %w", err)
	}

	// Decode MVT
	layers, err := mvt.Unmarshal(data)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal MVT: %w", err)
	}

	var roads []RoadGeometry
	invalidCount := 0

	// Look for "roads" layer
	for _, layer := range layers {
		if layer.Name != "roads" {
			continue
		}

		// Process each feature
		for featureIdx, feature := range layer.Features {
			// DEBUG: Log first 3 features' properties to see what's available
			if featureIdx < 3 {
				e.logger.Info("DEBUG: Feature properties in tile",
					"tile", fmt.Sprintf("%d/%d/%d", tileCoords.Z, tileCoords.X, tileCoords.Y),
					"featureIdx", featureIdx,
					"properties", feature.Properties,
					"propertyKeys", getPropertyKeys(feature.Properties))
			}

			// Get road name/ID - include region prefix to avoid collisions across regions
			roadID := ""
			if name, ok := feature.Properties["Name"].(string); ok && name != "" {
				roadID = fmt.Sprintf("%s_%s", region, name)
			}
			if roadID == "" {
				if id, ok := feature.Properties["id"].(string); ok && id != "" {
					roadID = fmt.Sprintf("%s_%s", region, id)
				}
			}
			if roadID == "" {
				// Generate unique ID from tile coordinates and feature index
				roadID = fmt.Sprintf("%s_road_%d_%d_%d_%d", region, tileCoords.Z, tileCoords.X, tileCoords.Y, featureIdx)
			}

			// Get curvature if available
			var curvature *string
			if curv, ok := feature.Properties["curvature"].(string); ok {
				curvature = &curv
			} else if curv, ok := feature.Properties["curvature"].(float64); ok {
				curvStr := fmt.Sprintf("%.2f", curv)
				curvature = &curvStr
			}

			// Convert tile coordinates to lat/lng bounds
			bounds := e.calculateBounds(feature.Geometry, tileCoords)
			if bounds == nil {
				continue
			}

			// Extract coordinates
			minLat := bounds.Min.Lat()
			maxLat := bounds.Max.Lat()
			minLng := bounds.Min.Lon()
			maxLng := bounds.Max.Lon()

			// Validate: skip and log if any coordinate is zero
			if minLat == 0 || maxLat == 0 || minLng == 0 || maxLng == 0 {
				invalidCount++
				e.logger.Error("INVALID ROAD: zero coordinates detected",
					"roadId", roadID,
					"tile", fmt.Sprintf("%d/%d/%d", tileCoords.Z, tileCoords.X, tileCoords.Y),
					"minLat", minLat, "maxLat", maxLat,
					"minLng", minLng, "maxLng", maxLng,
					"bounds.Min.Lat()", bounds.Min.Lat(),
					"bounds.Max.Lat()", bounds.Max.Lat(),
					"bounds.Min.Lon()", bounds.Min.Lon(),
					"bounds.Max.Lon()", bounds.Max.Lon())
				continue
			}

			roads = append(roads, RoadGeometry{
				RoadID:    roadID,
				Region:    region,
				MinLat:    minLat,
				MaxLat:    maxLat,
				MinLng:    minLng,
				MaxLng:    maxLng,
				Curvature: curvature,
			})
		}
	}

	return roads, invalidCount, nil
}

// calculateBounds calculates the geographic bounding box for a geometry
func (e *GeometryExtractor) calculateBounds(geom orb.Geometry, tile maptile.Tile) *orb.Bound {
	if geom == nil {
		return nil
	}

	// Helper function to convert tile-space coords (0-4096) to lat/lng
	// Uses Web Mercator projection (same as Mapbox tiles)
	tileCoordToLatLng := func(x, y float64) orb.Point {
		// Get tile bounds in Web Mercator space
		n := math.Pow(2.0, float64(tile.Z))

		// Calculate tile-space fraction (0-1)
		xFrac := x / 4096.0
		yFrac := y / 4096.0

		// Tile indices with fractional part
		tileX := float64(tile.X) + xFrac
		tileY := float64(tile.Y) + yFrac

		// Convert to longitude (simple linear)
		lng := (tileX/n)*360.0 - 180.0

		// Convert to latitude (inverse Mercator projection)
		lat := math.Atan(math.Sinh(math.Pi*(1.0-2.0*tileY/n))) * (180.0 / math.Pi)

		return orb.Point{lng, lat}
	}

	// Collect all points from geometry
	var points []orb.Point

	switch g := geom.(type) {
	case orb.Point:
		points = append(points, tileCoordToLatLng(g[0], g[1]))
	case orb.LineString:
		for _, coord := range g {
			points = append(points, tileCoordToLatLng(coord[0], coord[1]))
		}
	case orb.Polygon:
		for _, ring := range g {
			for _, coord := range ring {
				points = append(points, tileCoordToLatLng(coord[0], coord[1]))
			}
		}
	case orb.MultiLineString:
		for _, line := range g {
			for _, coord := range line {
				points = append(points, tileCoordToLatLng(coord[0], coord[1]))
			}
		}
	case orb.MultiPolygon:
		for _, poly := range g {
			for _, ring := range poly {
				for _, coord := range ring {
					points = append(points, tileCoordToLatLng(coord[0], coord[1]))
				}
			}
		}
	default:
		// Try to get bound from geometry if possible
		if b := geom.Bound(); b.Left() != 0 || b.Right() != 0 {
			// These are still tile coordinates, need to convert
			// Note: In tile coords, b.Min[1] (small y) = north/top, b.Max[1] (large y) = south/bottom
			// So we need to convert all 4 corners and recalculate proper geographic min/max
			swPoint := tileCoordToLatLng(b.Min[0], b.Max[1]) // southwest: min x, max y
			nePoint := tileCoordToLatLng(b.Max[0], b.Min[1]) // northeast: max x, min y

			// Construct proper geographic bound
			result := orb.Bound{
				Min: orb.Point{swPoint.Lon(), swPoint.Lat()}, // min lng, min lat
				Max: orb.Point{nePoint.Lon(), nePoint.Lat()}, // max lng, max lat
			}
			return &result
		}
		return nil
	}

	if len(points) == 0 {
		return nil
	}

	// Manually calculate min/max from all points
	minLng := points[0].Lon()
	maxLng := points[0].Lon()
	minLat := points[0].Lat()
	maxLat := points[0].Lat()

	for _, p := range points[1:] {
		if p.Lon() < minLng {
			minLng = p.Lon()
		}
		if p.Lon() > maxLng {
			maxLng = p.Lon()
		}
		if p.Lat() < minLat {
			minLat = p.Lat()
		}
		if p.Lat() > maxLat {
			maxLat = p.Lat()
		}
	}

	bound := orb.Bound{
		Min: orb.Point{minLng, minLat},
		Max: orb.Point{maxLng, maxLat},
	}

	// Debug: validate the bound - CHECK ALL 4 COORDINATES
	if bound.Min.Lat() == 0 || bound.Max.Lat() == 0 || bound.Min.Lon() == 0 || bound.Max.Lon() == 0 {
		e.logger.Warn("Invalid bound detected",
			"minLng", minLng, "minLat", minLat,
			"maxLng", maxLng, "maxLat", maxLat,
			"bound.Min.Lat()", bound.Min.Lat(),
			"bound.Max.Lat()", bound.Max.Lat(),
			"bound.Min.Lon()", bound.Min.Lon(),
			"bound.Max.Lon()", bound.Max.Lon())
	}

	return &bound
}

// findPBFFiles finds all .pbf files in a directory tree
func (e *GeometryExtractor) findPBFFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".pbf") {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// parseTilePath extracts tile coordinates (z/x/y) from file path
func (e *GeometryExtractor) parseTilePath(path string) (maptile.Tile, error) {
	// Expected format: .../z/x/y.pbf
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 3 {
		return maptile.Tile{}, fmt.Errorf("invalid tile path format")
	}

	// Get z, x, y from last 3 parts
	yFile := parts[len(parts)-1]
	x := parts[len(parts)-2]
	z := parts[len(parts)-3]

	// Remove .pbf extension from y
	y := strings.TrimSuffix(yFile, ".pbf")

	// Parse to integers
	zInt, err := strconv.ParseUint(z, 10, 32)
	if err != nil {
		return maptile.Tile{}, fmt.Errorf("invalid z coordinate: %w", err)
	}

	xInt, err := strconv.ParseUint(x, 10, 32)
	if err != nil {
		return maptile.Tile{}, fmt.Errorf("invalid x coordinate: %w", err)
	}

	yInt, err := strconv.ParseUint(y, 10, 32)
	if err != nil {
		return maptile.Tile{}, fmt.Errorf("invalid y coordinate: %w", err)
	}

	return maptile.New(uint32(xInt), uint32(yInt), maptile.Zoom(zInt)), nil
}

// Progress and file management functions

func (e *GeometryExtractor) getProgressFile(region string) string {
	return filepath.Join(".", fmt.Sprintf(".extract-progress-%s.json", region))
}

func (e *GeometryExtractor) getExtractionFile(region string) string {
	return filepath.Join(".", fmt.Sprintf(".extracted-roads-%s.json", region))
}

func (e *GeometryExtractor) loadProgress(region string) *ExtractionProgress {
	progressFile := e.getProgressFile(region)
	data, err := os.ReadFile(progressFile)
	if err != nil {
		return nil
	}

	var progress ExtractionProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		e.logger.Error("failed to unmarshal progress", "error", err)
		return nil
	}

	return &progress
}

func (e *GeometryExtractor) saveProgress(progress *ExtractionProgress) error {
	progressFile := e.getProgressFile(progress.Region)
	data, err := json.MarshalIndent(progress, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(progressFile, data, 0644)
}

func (e *GeometryExtractor) loadRoadsFromFile(filename string) ([]RoadGeometry, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var roads []RoadGeometry
	if err := json.Unmarshal(data, &roads); err != nil {
		return nil, err
	}

	return roads, nil
}

func (e *GeometryExtractor) saveRoadsToFile(filename string, roadsMap map[string]*RoadGeometry) error {
	// Convert map to slice
	roads := make([]RoadGeometry, 0, len(roadsMap))
	for _, road := range roadsMap {
		roads = append(roads, *road)
	}

	data, err := json.Marshal(roads)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// CleanupExtractionFiles removes extraction progress and data files
func (e *GeometryExtractor) CleanupExtractionFiles(region string) error {
	progressFile := e.getProgressFile(region)
	extractionFile := e.getExtractionFile(region)

	os.Remove(progressFile)
	os.Remove(extractionFile)

	return nil
}

// getPropertyKeys returns all keys from a property map for debugging
func getPropertyKeys(props map[string]interface{}) []string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	return keys
}
