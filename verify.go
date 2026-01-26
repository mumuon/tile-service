package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ZoomStats holds per-zoom-level tile statistics
type ZoomStats struct {
	Zoom      int
	TileCount int
	TotalSize int64
	MinX, MaxX int
	MinY, MaxY int
}

// TileIntegrityReport is the result of verifying a tile directory
type TileIntegrityReport struct {
	Dir          string
	MinZoom      int
	MaxZoom      int
	OK           bool
	MissingZooms []int
	ZoomStats    map[int]*ZoomStats
}

// Print logs the report details
func (r *TileIntegrityReport) Print() {
	logger := slog.With("dir", r.Dir, "min_zoom", r.MinZoom, "max_zoom", r.MaxZoom)

	if r.OK {
		logger.Info("tile integrity check PASSED", "zoom_levels", len(r.ZoomStats))
	} else {
		logger.Error("tile integrity check FAILED", "missing_zooms", r.MissingZooms)
	}

	for z := r.MinZoom; z <= r.MaxZoom; z++ {
		if stats, ok := r.ZoomStats[z]; ok {
			slog.Info("zoom level stats",
				"zoom", z,
				"tiles", stats.TileCount,
				"size_bytes", stats.TotalSize,
				"x_range", fmt.Sprintf("%d-%d", stats.MinX, stats.MaxX),
				"y_range", fmt.Sprintf("%d-%d", stats.MinY, stats.MaxY),
			)
		} else {
			slog.Warn("zoom level MISSING", "zoom", z)
		}
	}
}

// MergeIntegrityReport is the result of verifying merge completeness
type MergeIntegrityReport struct {
	RegionDir    string
	MergedDir    string
	OK           bool
	MissingTiles []TileCoord
	Warnings     []string // e.g., merged tile smaller than regional tile
}

// Print logs the merge integrity report
func (r *MergeIntegrityReport) Print() {
	logger := slog.With("region_dir", r.RegionDir, "merged_dir", r.MergedDir)

	if r.OK && len(r.Warnings) == 0 {
		logger.Info("merge integrity check PASSED")
	} else if r.OK {
		logger.Warn("merge integrity check PASSED with warnings", "warnings", len(r.Warnings))
	} else {
		logger.Error("merge integrity check FAILED", "missing_tiles", len(r.MissingTiles))
	}

	for _, w := range r.Warnings {
		slog.Warn("merge warning", "detail", w)
	}

	if len(r.MissingTiles) > 0 {
		// Show up to 20 missing tiles
		show := r.MissingTiles
		if len(show) > 20 {
			show = show[:20]
		}
		for _, tc := range show {
			slog.Error("missing tile in merged output", "z", tc.Z, "x", tc.X, "y", tc.Y)
		}
		if len(r.MissingTiles) > 20 {
			slog.Error("... and more missing tiles", "total", len(r.MissingTiles))
		}
	}
}

// UploadVerifyReport is the result of spot-checking uploaded tiles on R2
type UploadVerifyReport struct {
	TilesDir       string
	S3Prefix       string
	OK             bool
	Checked        int
	Missing        []string // s3 keys that were missing
	SamplesPerZoom int
}

// Print logs the upload verification report
func (r *UploadVerifyReport) Print() {
	logger := slog.With("tiles_dir", r.TilesDir, "s3_prefix", r.S3Prefix, "checked", r.Checked)

	if r.OK {
		logger.Info("upload verification PASSED")
	} else {
		logger.Error("upload verification FAILED", "missing", len(r.Missing))
		for _, key := range r.Missing {
			slog.Error("missing from R2", "key", key)
		}
	}
}

// VerifyTileDirectory checks that a tile directory has tiles at every zoom level
// from minZoom to maxZoom, and collects per-zoom statistics.
func VerifyTileDirectory(dir string, minZoom, maxZoom int) (*TileIntegrityReport, error) {
	report := &TileIntegrityReport{
		Dir:       dir,
		MinZoom:   minZoom,
		MaxZoom:   maxZoom,
		ZoomStats: make(map[int]*ZoomStats),
	}

	// Walk the directory and collect stats
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".pbf" {
			return nil
		}

		// Parse z/x/y.pbf
		rel, err := filepath.Rel(dir, path)
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

		stats, ok := report.ZoomStats[z]
		if !ok {
			stats = &ZoomStats{
				Zoom: z,
				MinX: x, MaxX: x,
				MinY: y, MaxY: y,
			}
			report.ZoomStats[z] = stats
		}

		stats.TileCount++
		stats.TotalSize += info.Size()

		if x < stats.MinX {
			stats.MinX = x
		}
		if x > stats.MaxX {
			stats.MaxX = x
		}
		if y < stats.MinY {
			stats.MinY = y
		}
		if y > stats.MaxY {
			stats.MaxY = y
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk tile directory: %w", err)
	}

	// Check which zoom levels are missing
	for z := minZoom; z <= maxZoom; z++ {
		if _, ok := report.ZoomStats[z]; !ok {
			report.MissingZooms = append(report.MissingZooms, z)
		}
	}

	report.OK = len(report.MissingZooms) == 0
	return report, nil
}

// VerifyMergeIntegrity checks that every tile from regionDir exists in mergedDir.
// It flags missing tiles as errors and merged tiles smaller than regional tiles as warnings.
func VerifyMergeIntegrity(regionDir, mergedDir string) (*MergeIntegrityReport, error) {
	report := &MergeIntegrityReport{
		RegionDir: regionDir,
		MergedDir: mergedDir,
	}

	err := filepath.Walk(regionDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".pbf" {
			return nil
		}

		rel, err := filepath.Rel(regionDir, path)
		if err != nil {
			return nil
		}

		mergedPath := filepath.Join(mergedDir, rel)
		mergedInfo, statErr := os.Stat(mergedPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				// Parse z/x/y for the error report
				parts := strings.Split(rel, string(filepath.Separator))
				if len(parts) == 3 {
					z, _ := strconv.Atoi(parts[0])
					x, _ := strconv.Atoi(parts[1])
					yStr := strings.TrimSuffix(parts[2], ".pbf")
					y, _ := strconv.Atoi(yStr)
					report.MissingTiles = append(report.MissingTiles, TileCoord{z, x, y})
				}
			}
			return nil
		}

		// Check if merged tile is smaller than regional tile
		if mergedInfo.Size() < info.Size() {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("merged tile %s is smaller than regional (%d < %d bytes)",
					rel, mergedInfo.Size(), info.Size()))
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk region directory: %w", err)
	}

	report.OK = len(report.MissingTiles) == 0
	return report, nil
}

// VerifyUpload spot-checks that tiles exist on R2 by sampling N tiles per zoom level.
func VerifyUpload(ctx context.Context, s3Client *S3Client, tilesDir, s3Prefix string, samplesPerZoom int) (*UploadVerifyReport, error) {
	report := &UploadVerifyReport{
		TilesDir:       tilesDir,
		S3Prefix:       s3Prefix,
		SamplesPerZoom: samplesPerZoom,
	}

	// Collect tiles per zoom level
	tilesByZoom := make(map[int][]string) // zoom -> list of relative paths

	err := filepath.Walk(tilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".pbf" {
			return nil
		}

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

		tilesByZoom[z] = append(tilesByZoom[z], rel)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk tiles directory: %w", err)
	}

	// Sample and check tiles per zoom level
	for z, tiles := range tilesByZoom {
		// Select samples
		samples := tiles
		if len(samples) > samplesPerZoom {
			// Shuffle and take first N
			rand.Shuffle(len(samples), func(i, j int) {
				samples[i], samples[j] = samples[j], samples[i]
			})
			samples = samples[:samplesPerZoom]
		}

		for _, rel := range samples {
			s3Key := filepath.Join(s3Prefix, filepath.ToSlash(rel))

			_, exists, err := s3Client.HeadObject(ctx, s3Key)
			if err != nil {
				slog.Warn("error checking tile on R2", "key", s3Key, "zoom", z, "error", err)
				continue
			}

			report.Checked++
			if !exists {
				report.Missing = append(report.Missing, s3Key)
			}
		}
	}

	report.OK = len(report.Missing) == 0
	return report, nil
}
