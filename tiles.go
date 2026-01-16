package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// GenerateTiles generates vector tiles from GeoJSON using Tippecanoe
func GenerateTiles(ctx context.Context, geoJSONPath, region string, outputBaseDir string) (string, int, int64, error) {
	logger := slog.With("region", region, "geojson", geoJSONPath)
	logger.Info("generating tiles with Tippecanoe")

	// Create output directory - must fully remove old tiles to prevent stale data
	tilesDir := filepath.Join(outputBaseDir, region)
	if err := removeDirectoryContents(tilesDir); err != nil {
		return "", 0, 0, fmt.Errorf("failed to clean tiles directory: %w", err)
	}

	if err := os.MkdirAll(tilesDir, 0755); err != nil {
		return "", 0, 0, fmt.Errorf("failed to create tiles directory: %w", err)
	}

	// Build Tippecanoe command
	// NOTE: Must use separate --include flags for each property (not --include=Name)
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
		"--include", "id",
		"--include", "Name",
		"--include", "curvature",
		"--include", "length",
		"--include", "startLat",
		"--include", "startLng",
		"--include", "endLat",
		"--include", "endLng",
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

	// Copy tiles to parent directory for compatibility
	parentDir := filepath.Dir(tilesDir)
	if err := copyTilesToParent(tilesDir, parentDir, logger); err != nil {
		logger.Warn("failed to copy tiles to parent directory", "error", err)
		// Don't fail the job, just log the warning
	}

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

// copyTilesToParent copies tiles from region subdirectory to parent directory
// This allows tiles to be accessed at both /tiles/washington/14/... and /tiles/14/...
func copyTilesToParent(sourceDir, parentDir string, logger *slog.Logger) error {
	logger.Info("copying tiles to parent directory", "source", sourceDir, "parent", parentDir)

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == sourceDir {
			return nil
		}

		// Calculate relative path from source
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Destination path in parent directory
		destPath := filepath.Join(parentDir, relPath)

		// If it's a directory, create it in parent
		if info.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			return nil
		}

		// If it's a file, copy it
		return copyFile(path, destPath)
	})
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// removeDirectoryContents removes all contents of a directory, handling stubborn files like .DS_Store
// This is more robust than os.RemoveAll which can fail on non-empty directories
func removeDirectoryContents(dir string) error {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // Nothing to remove
	}

	// Collect all paths to remove (files first, then directories in reverse order)
	var files []string
	var dirs []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil // Skip the root directory itself
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		} else {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// Remove all files first
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove file %s: %w", file, err)
		}
	}

	// Remove directories in reverse order (deepest first)
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(dirs[i]); err != nil {
			return fmt.Errorf("failed to remove directory %s: %w", dirs[i], err)
		}
	}

	// Finally remove the root directory
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("failed to remove root directory %s: %w", dir, err)
	}

	return nil
}
