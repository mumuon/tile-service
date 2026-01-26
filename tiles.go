package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GenerateTilesOptions contains options for tile generation
type GenerateTilesOptions struct {
	MinZoom int // Minimum zoom level (default 5)
	MaxZoom int // Maximum zoom level (default 16)
}

// GenerateTiles generates vector tiles from GeoJSON using Tippecanoe
func GenerateTiles(ctx context.Context, geoJSONPath, region string, outputBaseDir string) (string, int, int64, error) {
	return GenerateTilesWithOptions(ctx, geoJSONPath, region, outputBaseDir, nil)
}

// GenerateTilesWithOptions generates vector tiles with configurable zoom levels
func GenerateTilesWithOptions(ctx context.Context, geoJSONPath, region string, outputBaseDir string, opts *GenerateTilesOptions) (string, int, int64, error) {
	// Default zoom levels
	minZoom := 5
	maxZoom := 16
	if opts != nil {
		if opts.MinZoom >= 0 {
			minZoom = opts.MinZoom
		}
		if opts.MaxZoom > 0 {
			maxZoom = opts.MaxZoom
		}
	}

	logger := slog.With("region", region, "geojson", geoJSONPath, "min_zoom", minZoom, "max_zoom", maxZoom)
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
		"--read-parallel",
		"--temporary-directory=/tmp",
		fmt.Sprintf("--minimum-zoom=%d", minZoom),
		fmt.Sprintf("--maximum-zoom=%d", maxZoom),
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

// removeDirectoryContents removes all contents of a directory except .DS_Store files
// We skip .DS_Store because macOS recreates them constantly, causing race conditions
// The root directory is preserved to avoid race conditions with Finder
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
		// Skip .DS_Store files - macOS recreates them constantly
		if filepath.Base(path) == ".DS_Store" {
			return nil
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
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove file %s: %w", file, err)
		}
	}

	// Remove directories in reverse order (deepest first)
	// Ignore errors for directories that may have .DS_Store files
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Remove(dirs[i]) // Best effort - may fail if .DS_Store exists
	}

	return nil
}

// MergeTilesOptions contains options for tile merging
type MergeTilesOptions struct {
	MinZoom int // Minimum zoom level (-1 for no filter)
	MaxZoom int // Maximum zoom level (-1 for no filter)
}

// MergeTiles merges multiple regional tile directories into a single output using tile-join
// This combines tiles from overlapping regions so all roads are visible at low zoom levels
func MergeTiles(ctx context.Context, inputDirs []string, outputDir string) (*TileMetadata, error) {
	return MergeTilesWithOptions(ctx, inputDirs, outputDir, nil)
}

// MergeTilesWithOptions merges tiles with optional zoom filtering
func MergeTilesWithOptions(ctx context.Context, inputDirs []string, outputDir string, opts *MergeTilesOptions) (*TileMetadata, error) {
	logger := slog.With("output_dir", outputDir, "input_count", len(inputDirs))
	if opts != nil && (opts.MinZoom >= 0 || opts.MaxZoom >= 0) {
		logger = logger.With("min_zoom", opts.MinZoom, "max_zoom", opts.MaxZoom)
	}
	logger.Info("merging regional tiles with tile-join")

	if len(inputDirs) == 0 {
		return nil, fmt.Errorf("no input directories provided for merge")
	}

	// Clean output directory to ensure fresh merge
	if err := removeDirectoryContents(outputDir); err != nil {
		return nil, fmt.Errorf("failed to clean merged directory: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create merged directory: %w", err)
	}

	// Build tile-join command
	// tile-join merges multiple tile sources into one, combining features from overlapping tiles
	// --no-tile-size-limit is needed because merged tiles can exceed the 500KB default limit
	args := []string{
		"--force",
		"--no-tile-compression",
		"--no-tile-size-limit",
		fmt.Sprintf("--output-to-directory=%s", outputDir),
	}

	// Add zoom filtering if specified
	if opts != nil {
		if opts.MinZoom >= 0 {
			args = append(args, fmt.Sprintf("--minimum-zoom=%d", opts.MinZoom))
		}
		if opts.MaxZoom >= 0 {
			args = append(args, fmt.Sprintf("--maximum-zoom=%d", opts.MaxZoom))
		}
	}

	args = append(args, inputDirs...)

	cmd := exec.CommandContext(ctx, "tile-join", args...)

	// Set TIPPECANOE_MAX_THREADS to use all available CPUs for faster merging
	// Inherit current environment and add/override the thread setting
	cmd.Env = append(os.Environ(), "TIPPECANOE_MAX_THREADS=16")

	logger.Debug("running tile-join", "cmd", cmd.String(), "threads", 16)

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("tile-join failed", "error", err, "output", string(output))
		return nil, fmt.Errorf("tile-join merge failed: %w", err)
	}

	logger.Debug("tile-join output", "output", string(output))

	// Get metadata about merged tiles
	metadata, err := GetTileMetadata(outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get merged tile metadata: %w", err)
	}

	logger.Info("tiles merged successfully",
		"tiles_count", metadata.TilesCount,
		"total_size_bytes", metadata.TotalSize,
	)

	return metadata, nil
}

// TileCoord represents a tile coordinate (zoom/x/y)
type TileCoord struct {
	Z, X, Y int
}

// GetTileCoords returns a set of all tile coordinates in a directory
func GetTileCoords(tilesDir string) (map[TileCoord]bool, error) {
	coords := make(map[TileCoord]bool)

	err := filepath.Walk(tilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".pbf" {
			return nil
		}

		// Parse z/x/y.pbf from path
		rel, err := filepath.Rel(tilesDir, path)
		if err != nil {
			return nil
		}

		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 3 {
			return nil
		}

		z, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil
		}
		yStr := strings.TrimSuffix(parts[2], ".pbf")
		y, err := strconv.Atoi(yStr)
		if err != nil {
			return nil
		}

		coords[TileCoord{z, x, y}] = true
		return nil
	})

	return coords, err
}

// FindOverlappingRegions finds regional tile directories that have overlapping tiles with the target region
// This is more efficient than merging all regions - only merge neighbors that actually overlap
func FindOverlappingRegions(baseDir string, targetRegion string) ([]string, error) {
	logger := slog.With("target_region", targetRegion)

	// Get all tile coordinates from the target region
	targetDir := filepath.Join(baseDir, targetRegion)
	targetCoords, err := GetTileCoords(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get target region coords: %w", err)
	}

	logger.Debug("target region tile count", "count", len(targetCoords))

	// Find all regional directories
	allRegions, err := FindRegionalTileDirs(baseDir)
	if err != nil {
		return nil, err
	}

	// Check each region for overlap
	var overlapping []string
	for _, regionDir := range allRegions {
		regionName := filepath.Base(regionDir)

		// Always include the target region itself
		if regionName == targetRegion {
			overlapping = append(overlapping, regionDir)
			continue
		}

		// Check if this region has any overlapping tiles
		hasOverlap, err := hasOverlappingTiles(regionDir, targetCoords)
		if err != nil {
			logger.Warn("failed to check overlap", "region", regionName, "error", err)
			continue
		}

		if hasOverlap {
			logger.Debug("found overlapping region", "region", regionName)
			overlapping = append(overlapping, regionDir)
		}
	}

	return overlapping, nil
}

// hasOverlappingTiles checks if a region directory has any tiles that overlap with the target coordinates
// Returns early on first match for efficiency
func hasOverlappingTiles(regionDir string, targetCoords map[TileCoord]bool) (bool, error) {
	found := false

	err := filepath.Walk(regionDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if found {
			return filepath.SkipAll // Already found overlap, stop walking
		}
		if info.IsDir() || filepath.Ext(path) != ".pbf" {
			return nil
		}

		// Parse z/x/y.pbf from path
		rel, err := filepath.Rel(regionDir, path)
		if err != nil {
			return nil
		}

		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 3 {
			return nil
		}

		z, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil
		}
		yStr := strings.TrimSuffix(parts[2], ".pbf")
		y, err := strconv.Atoi(yStr)
		if err != nil {
			return nil
		}

		if targetCoords[TileCoord{z, x, y}] {
			found = true
			return filepath.SkipAll
		}
		return nil
	})

	return found, err
}

// FindRegionalTileDirs finds all regional tile directories in the base directory
// It excludes the "merged" directory and any non-tile directories
func FindRegionalTileDirs(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip the merged directory
		if name == "merged" {
			continue
		}

		// Skip hidden directories
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Skip numeric directories (zoom levels that might be at the top level)
		if _, err := strconv.Atoi(name); err == nil {
			continue
		}

		// Check if it looks like a tile directory (has numeric subdirectories for zoom levels)
		dirPath := filepath.Join(baseDir, name)
		subEntries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		// Check for at least one numeric subdirectory (zoom level)
		hasTiles := false
		for _, subEntry := range subEntries {
			if subEntry.IsDir() {
				if _, err := strconv.Atoi(subEntry.Name()); err == nil {
					hasTiles = true
					break
				}
			}
		}

		if hasTiles {
			dirs = append(dirs, dirPath)
		}
	}

	return dirs, nil
}
