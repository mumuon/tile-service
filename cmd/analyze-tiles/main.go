package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/paulmach/orb/encoding/mvt"
)

type TileStats struct {
	TotalTiles    int
	TotalFeatures int
	FeaturesByZoom map[int]int
	UniqueRoadIDs map[string]bool
	LayersFound   map[string]int
}

// TileInfo represents detailed information about a single tile
type TileInfo struct {
	Path         string       `json:"tile"`
	Z            int          `json:"z"`
	X            int          `json:"x"`
	Y            int          `json:"y"`
	FileSizeBytes int64        `json:"fileSizeBytes"`
	Layers       []LayerInfo  `json:"layers"`
}

// LayerInfo represents a layer within a tile
type LayerInfo struct {
	Name         string        `json:"name"`
	FeatureCount int           `json:"featureCount"`
	Features     []FeatureInfo `json:"features"`
}

// FeatureInfo represents a feature within a layer
type FeatureInfo struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
}

func main() {
	// Define command-line flags
	tilePath := flag.String("tile", "", "Path to a single tile file to inspect")
	verbose := flag.Bool("verbose", false, "Show all features (not just first 10)")
	jsonOutput := flag.Bool("json", false, "Output in JSON format")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: analyze-tiles [options] [tile-directory]\n\n")
		fmt.Fprintf(os.Stderr, "Modes:\n")
		fmt.Fprintf(os.Stderr, "  1. Single tile inspection: analyze-tiles --tile <path>\n")
		fmt.Fprintf(os.Stderr, "  2. Directory analysis:     analyze-tiles <directory>\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  analyze-tiles --tile ~/data/df/tiles/colorado/10/200/400.pbf\n")
		fmt.Fprintf(os.Stderr, "  analyze-tiles --tile ~/data/df/tiles/colorado/10/200/400.pbf --verbose\n")
		fmt.Fprintf(os.Stderr, "  analyze-tiles --tile ~/data/df/tiles/colorado/10/200/400.pbf --json\n")
		fmt.Fprintf(os.Stderr, "  analyze-tiles ~/data/df/tiles/colorado\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Single tile inspection mode
	if *tilePath != "" {
		info, err := inspectSingleTile(*tilePath)
		if err != nil {
			fmt.Printf("Error inspecting tile: %v\n", err)
			os.Exit(1)
		}

		if *jsonOutput {
			printTileJSON(info)
		} else {
			printTileInfo(info, *verbose)
		}
		return
	}

	// Directory analysis mode (existing behavior)
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	tileDir := args[0]

	stats := &TileStats{
		FeaturesByZoom: make(map[int]int),
		UniqueRoadIDs: make(map[string]bool),
		LayersFound:   make(map[string]int),
	}

	if err := analyzeTileDirectory(tileDir, stats); err != nil {
		fmt.Printf("Error analyzing tiles: %v\n", err)
		os.Exit(1)
	}

	printStats(stats, tileDir)
}

// inspectSingleTile inspects a single tile file and returns detailed information
func inspectSingleTile(path string) (*TileInfo, error) {
	// Get file info
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Parse tile coordinates from path (e.g., .../z/x/y.pbf)
	z, x, y, err := parseTileCoordinates(path)
	if err != nil {
		// If we can't parse coordinates, just use zeros
		z, x, y = 0, 0, 0
	}

	// Read tile data
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tile: %w", err)
	}

	// Unmarshal MVT data
	layers, err := mvt.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal MVT: %w", err)
	}

	// Build TileInfo
	info := &TileInfo{
		Path:         path,
		Z:            z,
		X:            x,
		Y:            y,
		FileSizeBytes: fileInfo.Size(),
		Layers:       make([]LayerInfo, 0, len(layers)),
	}

	// Process each layer
	for _, layer := range layers {
		layerInfo := LayerInfo{
			Name:         layer.Name,
			FeatureCount: len(layer.Features),
			Features:     make([]FeatureInfo, 0, len(layer.Features)),
		}

		// Process each feature
		for _, feature := range layer.Features {
			featureInfo := FeatureInfo{
				Type:       feature.Geometry.GeoJSONType(),
				Properties: feature.Properties,
			}
			layerInfo.Features = append(layerInfo.Features, featureInfo)
		}

		info.Layers = append(info.Layers, layerInfo)
	}

	return info, nil
}

// parseTileCoordinates extracts z/x/y coordinates from a tile path
// Expected path format: .../z/x/y.pbf
func parseTileCoordinates(path string) (z, x, y int, err error) {
	// Remove .pbf extension
	pathWithoutExt := strings.TrimSuffix(path, ".pbf")

	// Use regex to extract z/x/y pattern
	re := regexp.MustCompile(`(\d+)/(\d+)/(\d+)$`)
	matches := re.FindStringSubmatch(pathWithoutExt)

	if len(matches) != 4 {
		return 0, 0, 0, fmt.Errorf("could not parse tile coordinates from path")
	}

	z, _ = strconv.Atoi(matches[1])
	x, _ = strconv.Atoi(matches[2])
	y, _ = strconv.Atoi(matches[3])

	return z, x, y, nil
}

// printTileInfo prints tile information in a human-readable format
func printTileInfo(info *TileInfo, verbose bool) {
	fmt.Println("=" + strings.Repeat("=", 78))
	fmt.Printf("Tile: %s\n", filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(info.Path)))) + "/" +
		filepath.Base(filepath.Dir(filepath.Dir(info.Path))) + "/" +
		filepath.Base(filepath.Dir(info.Path)) + "/" +
		filepath.Base(info.Path))
	fmt.Println("=" + strings.Repeat("=", 78))
	fmt.Println()

	fmt.Println("📍 Tile Info:")
	if info.Z != 0 || info.X != 0 || info.Y != 0 {
		fmt.Printf("  Coordinates: Z%d, X%d, Y%d\n", info.Z, info.X, info.Y)
	}
	fmt.Printf("  File size: %s\n", formatBytes(info.FileSizeBytes))
	fmt.Println()

	fmt.Printf("📋 Layers: %d\n\n", len(info.Layers))

	for _, layer := range info.Layers {
		fmt.Printf("Layer: %s\n", layer.Name)
		fmt.Printf("  Features: %d\n\n", layer.FeatureCount)

		featuresToShow := layer.Features
		if !verbose && len(layer.Features) > 10 {
			featuresToShow = layer.Features[:10]
		}

		for i, feature := range featuresToShow {
			fmt.Printf("  Feature %d (%s)\n", i+1, feature.Type)

			// Sort property keys for consistent output
			keys := make([]string, 0, len(feature.Properties))
			for k := range feature.Properties {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, key := range keys {
				value := feature.Properties[key]
				fmt.Printf("    %s: %v\n", key, value)
			}
			fmt.Println()
		}

		if !verbose && len(layer.Features) > 10 {
			fmt.Printf("  ... (%d more features, use --verbose to show all)\n\n", len(layer.Features)-10)
		}
	}

	fmt.Println("=" + strings.Repeat("=", 78))
}

// printTileJSON prints tile information in JSON format
func printTileJSON(info *TileInfo) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(info); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func analyzeTileDirectory(dir string, stats *TileStats) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, ".pbf") {
			// Parse tile coordinates from path
			// Expected: .../z/x/y.pbf
			parts := strings.Split(filepath.Dir(path), string(os.PathSeparator))
			var z int
			if len(parts) >= 1 {
				fmt.Sscanf(parts[len(parts)-2], "%d", &z)
			}

			if err := analyzeTile(path, z, stats); err != nil {
				fmt.Printf("Warning: failed to analyze %s: %v\n", path, err)
			}
		}

		return nil
	})
}

func analyzeTile(path string, z int, stats *TileStats) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	layers, err := mvt.Unmarshal(data)
	if err != nil {
		return err
	}

	stats.TotalTiles++

	for _, layer := range layers {
		layerName := layer.Name
		stats.LayersFound[layerName]++

		numFeatures := len(layer.Features)
		stats.TotalFeatures += numFeatures
		stats.FeaturesByZoom[z] += numFeatures

		// Extract road IDs from features (if roads layer)
		if layerName == "roads" {
			for _, feature := range layer.Features {
				if roadID := getPropertyString(feature.Properties, "Name"); roadID != "" {
					stats.UniqueRoadIDs[roadID] = true
				} else if id := getPropertyString(feature.Properties, "id"); id != "" {
					stats.UniqueRoadIDs[id] = true
				}
			}
		}
	}

	return nil
}

func getPropertyString(props map[string]interface{}, key string) string {
	if val, ok := props[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func printStats(stats *TileStats, dir string) {
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Printf("Tile Analysis: %s\n", filepath.Base(dir))
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()

	fmt.Println("📊 Tile Counts:")
	fmt.Printf("  Total tiles:     %d\n", stats.TotalTiles)
	fmt.Printf("  Total features:  %d\n", stats.TotalFeatures)
	fmt.Printf("  Unique road IDs: %d\n", len(stats.UniqueRoadIDs))
	fmt.Println()

	fmt.Println("📋 Layers Found:")
	for layer, count := range stats.LayersFound {
		fmt.Printf("  %s: %d tiles\n", layer, count)
	}
	fmt.Println()

	fmt.Println("🗺️  Features by Zoom Level:")
	zooms := make([]int, 0, len(stats.FeaturesByZoom))
	for z := range stats.FeaturesByZoom {
		zooms = append(zooms, z)
	}
	sort.Ints(zooms)

	for _, z := range zooms {
		count := stats.FeaturesByZoom[z]
		bar := strings.Repeat("█", min(count/10, 50))
		fmt.Printf("  Z%2d: %6d features %s\n", z, count, bar)
	}
	fmt.Println()

	fmt.Println("=" + strings.Repeat("=", 70))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
