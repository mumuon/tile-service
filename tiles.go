package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// GenerateTiles generates vector tiles from GeoJSON using Tippecanoe
func GenerateTiles(ctx context.Context, geoJSONPath, region string, outputBaseDir string) (string, int, int64, error) {
	logger := slog.With("region", region, "geojson", geoJSONPath)
	logger.Info("generating tiles with Tippecanoe")

	// Create output directory
	tilesDir := filepath.Join(outputBaseDir, region)
	if err := os.RemoveAll(tilesDir); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove existing tiles directory", "error", err)
	}

	if err := os.MkdirAll(tilesDir, 0755); err != nil {
		return "", 0, 0, fmt.Errorf("failed to create tiles directory: %w", err)
	}

	// Build Tippecanoe command
	cmd := exec.CommandContext(ctx, "tippecanoe",
		"--force",
		fmt.Sprintf("--output-to-directory=%s", tilesDir),
		"--minimum-zoom=5",
		"--maximum-zoom=16",
		"--drop-fraction-as-needed",
		"--extend-zooms-if-still-dropping",
		"--layer=roads",
		fmt.Sprintf("--name=%s Curvy Roads", region),
		"--attribution=Data © OpenStreetMap contributors",
		"--preserve-input-order",
		"--maximum-string-attribute-length=1000",
		"--no-tile-compression",
		"--include", "Name",
		geoJSONPath,
	)

	logger.Debug("running Tippecanoe", "cmd", cmd.String())

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("Tippecanoe failed", "error", err, "output", string(output))
		return "", 0, 0, fmt.Errorf("Tippecanoe generation failed: %w", err)
	}

	logger.Debug("Tippecanoe output", "output", string(output))

	// Count generated tiles
	tilesCount, err := countTiles(tilesDir)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to count tiles: %w", err)
	}

	// Calculate total size
	totalSize, err := getDirectorySize(tilesDir)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to calculate directory size: %w", err)
	}

	logger.Info("tiles generated successfully",
		"tiles_count", tilesCount,
		"total_size_bytes", totalSize,
	)

	return tilesDir, tilesCount, totalSize, nil
}

// countTiles counts the number of .pbf tile files in a directory
func countTiles(dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".pbf" {
			count++
		}
		return nil
	})
	return count, err
}

// getDirectorySize calculates the total size of a directory
func getDirectorySize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// CalculateMetadata calculates statistics about generated tiles
type TileMetadata struct {
	TilesCount int
	TotalSize  int64
	MinZoom    int
	MaxZoom    int
}

// GetTileMetadata gets metadata about the tiles
func GetTileMetadata(tilesDir string) (*TileMetadata, error) {
	tilesCount, err := countTiles(tilesDir)
	if err != nil {
		return nil, err
	}

	totalSize, err := getDirectorySize(tilesDir)
	if err != nil {
		return nil, err
	}

	return &TileMetadata{
		TilesCount: tilesCount,
		TotalSize:  totalSize,
		MinZoom:    5,
		MaxZoom:    16,
	}, nil
}
