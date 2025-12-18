package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: analyze-tiles <tile-directory>")
		fmt.Println("Example: analyze-tiles ~/data/df/tiles/delaware")
		os.Exit(1)
	}

	tileDir := os.Args[1]

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
