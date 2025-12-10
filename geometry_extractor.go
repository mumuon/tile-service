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
		roads, err := e.extractRoadsFromTile(pbfFile, region, tileCoords)
		if err != nil {
			logger.Warn("failed to extract from tile", "file", pbfFile, "error", err)
			continue
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
func (e *GeometryExtractor) extractRoadsFromTile(pbfFile, region string, tileCoords maptile.Tile) ([]RoadGeometry, error) {
	// Read tile file
	data, err := os.ReadFile(pbfFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read tile: %w", err)
	}

	// Decode MVT
	layers, err := mvt.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal MVT: %w", err)
	}

	var roads []RoadGeometry

	// Look for "roads" layer
	for _, layer := range layers {
		if layer.Name != "roads" {
			continue
		}

		// Process each feature
		for featureIdx, feature := range layer.Features {
			// Get road name/ID
			roadID := ""
			if name, ok := feature.Properties["Name"].(string); ok && name != "" {
				roadID = name
			}
			if roadID == "" {
				if id, ok := feature.Properties["id"].(string); ok && id != "" {
					roadID = id
				}
			}
			if roadID == "" {
				// Generate unique ID from tile coordinates and feature index
				roadID = fmt.Sprintf("road_%d_%d_%d_%d", tileCoords.Z, tileCoords.X, tileCoords.Y, featureIdx)
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

			roads = append(roads, RoadGeometry{
				RoadID:    roadID,
				Region:    region,
				MinLat:    bounds.Min.Lat(),
				MaxLat:    bounds.Max.Lat(),
				MinLng:    bounds.Min.Lon(),
				MaxLng:    bounds.Max.Lon(),
				Curvature: curvature,
			})
		}
	}

	return roads, nil
}

// calculateBounds calculates the geographic bounding box for a geometry
func (e *GeometryExtractor) calculateBounds(geom orb.Geometry, tile maptile.Tile) *orb.Bound {
	if geom == nil {
		return nil
	}

	// Get tile bounds in geographic coordinates
	tileBound := tile.Bound()

	// Helper function to convert tile-space coords (0-4096) to lat/lng
	tileCoordToLatLng := func(x, y float64) orb.Point {
		// Interpolate between tile bounds
		lng := tileBound.Min.Lon() + (x/4096.0)*(tileBound.Max.Lon()-tileBound.Min.Lon())
		lat := tileBound.Max.Lat() + (y/4096.0)*(tileBound.Min.Lat()-tileBound.Max.Lat())
		return orb.Point{lng, lat}
	}

	// Convert tile space coordinates to geographic coordinates
	var bound orb.Bound

	switch g := geom.(type) {
	case orb.Point:
		point := tileCoordToLatLng(g[0], g[1])
		bound = orb.Bound{Min: point, Max: point}
	case orb.LineString:
		for _, coord := range g {
			point := tileCoordToLatLng(coord[0], coord[1])
			bound = bound.Extend(point)
		}
	case orb.Polygon:
		for _, ring := range g {
			for _, coord := range ring {
				point := tileCoordToLatLng(coord[0], coord[1])
				bound = bound.Extend(point)
			}
		}
	case orb.MultiLineString:
		for _, line := range g {
			for _, coord := range line {
				point := tileCoordToLatLng(coord[0], coord[1])
				bound = bound.Extend(point)
			}
		}
	case orb.MultiPolygon:
		for _, poly := range g {
			for _, ring := range poly {
				for _, coord := range ring {
					point := tileCoordToLatLng(coord[0], coord[1])
					bound = bound.Extend(point)
				}
			}
		}
	default:
		// Try to get bound from geometry if possible
		if b := geom.Bound(); b.Left() != 0 || b.Right() != 0 {
			// These are still tile coordinates, need to convert
			minPoint := tileCoordToLatLng(b.Min[0], b.Min[1])
			maxPoint := tileCoordToLatLng(b.Max[0], b.Max[1])
			return &orb.Bound{Min: minPoint, Max: maxPoint}
		}
		return nil
	}

	if bound.Left() == 0 && bound.Right() == 0 && bound.Top() == 0 && bound.Bottom() == 0 {
		return nil
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
