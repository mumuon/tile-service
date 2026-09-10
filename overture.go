package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// BuildingsJobOptions configures a single Overture buildings extract → PMTiles run.
type BuildingsJobOptions struct {
	Region    string // label used for output filename and tippecanoe layer name
	BBox      string // "minLon,minLat,maxLon,maxLat" — required, keeps every run bounded
	MinZoom   int
	MaxZoom   int
	OutputDir string // where the final .pmtiles is written
	TempDir   string // base scratch dir; a per-run subdirectory is created and cleaned up here
	SkipUpload bool
	NoCleanup  bool
}

// GenerateBuildingsPMTiles downloads Overture building footprints for a bounding box via the
// `overturemaps` CLI, builds a single PMTiles archive with Tippecanoe, and (unless skipped)
// uploads it to S3/R2. It never touches Postgres — this job has no DB dependency, so there is
// nothing to accidentally point at a shared/live database.
func GenerateBuildingsPMTiles(ctx context.Context, opts BuildingsJobOptions, s3Client *S3Client) (pmtilesPath string, sizeBytes int64, err error) {
	if opts.BBox == "" {
		return "", 0, fmt.Errorf("bbox is required (minLon,minLat,maxLon,maxLat) — refusing to run an unbounded global extract")
	}
	if opts.Region == "" {
		return "", 0, fmt.Errorf("region label is required")
	}

	minZoom, maxZoom := opts.MinZoom, opts.MaxZoom
	if maxZoom == 0 {
		maxZoom = 14
	}

	logger := slog.With("job", "overture-buildings", "region", opts.Region, "bbox", opts.BBox)

	tempBase := opts.TempDir
	if tempBase == "" {
		tempBase = os.TempDir()
	}
	runDir := filepath.Join(tempBase, fmt.Sprintf("overture-buildings-%s-%d", opts.Region, time.Now().UnixNano()))
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create scratch dir: %w", err)
	}
	if !opts.NoCleanup {
		defer func() {
			if rmErr := os.RemoveAll(runDir); rmErr != nil {
				logger.Warn("failed to clean up scratch dir", "dir", runDir, "error", rmErr)
			}
		}()
	} else {
		logger.Info("--no-cleanup set, leaving scratch dir in place", "dir", runDir)
	}

	geojsonPath := filepath.Join(runDir, "buildings.geojson")

	logger.Info("downloading Overture buildings extract", "output", geojsonPath)
	downloadCmd := exec.CommandContext(ctx, "overturemaps", "download",
		fmt.Sprintf("--bbox=%s", opts.BBox),
		"-f", "geojson",
		"--type=building",
		"-o", geojsonPath,
	)
	if output, downloadErr := downloadCmd.CombinedOutput(); downloadErr != nil {
		logger.Error("overturemaps download failed", "error", downloadErr, "output", string(output))
		return "", 0, fmt.Errorf("overturemaps download failed: %w", downloadErr)
	}

	if info, statErr := os.Stat(geojsonPath); statErr != nil {
		return "", 0, fmt.Errorf("overturemaps download produced no output: %w", statErr)
	} else if info.Size() == 0 {
		return "", 0, fmt.Errorf("overturemaps download produced an empty file — bbox %q likely has no buildings", opts.BBox)
	}

	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create output dir: %w", err)
	}
	pmtilesPath = filepath.Join(outputDir, fmt.Sprintf("buildings-%s.pmtiles", opts.Region))

	logger.Info("building PMTiles with Tippecanoe", "min_zoom", minZoom, "max_zoom", maxZoom, "output", pmtilesPath)
	// Completeness flags: building footprints must not be silently thinned the way road
	// segments are (--drop-densest-as-needed is fine for roads, wrong for buildings — a
	// missing building looks like a data bug, not a rendering optimization).
	tippecanoeCmd := exec.CommandContext(ctx, "tippecanoe",
		"--force",
		"-o", pmtilesPath,
		fmt.Sprintf("--minimum-zoom=%d", minZoom),
		fmt.Sprintf("--maximum-zoom=%d", maxZoom),
		"--no-feature-limit",
		"--no-tile-size-limit",
		"--no-tile-compression",
		"--detect-shared-borders",
		fmt.Sprintf("--temporary-directory=%s", runDir),
		"--layer=buildings",
		fmt.Sprintf("--name=%s Overture Buildings", opts.Region),
		"--attribution=© OpenStreetMap contributors, Overture Maps Foundation",
		geojsonPath,
	)
	if output, tcErr := tippecanoeCmd.CombinedOutput(); tcErr != nil {
		logger.Error("tippecanoe failed", "error", tcErr, "output", string(output))
		return "", 0, fmt.Errorf("tippecanoe failed: %w", tcErr)
	}

	info, statErr := os.Stat(pmtilesPath)
	if statErr != nil {
		return "", 0, fmt.Errorf("tippecanoe reported success but PMTiles file is missing: %w", statErr)
	}
	sizeBytes = info.Size()
	logger.Info("PMTiles generated", "size_bytes", sizeBytes)

	if opts.SkipUpload {
		logger.Info("--skip-upload set, leaving PMTiles on local disk only")
		return pmtilesPath, sizeBytes, nil
	}

	if s3Client == nil {
		return "", 0, fmt.Errorf("upload requested but no S3 client configured")
	}

	s3Key := fmt.Sprintf("buildings/%s.pmtiles", opts.Region)
	logger.Info("uploading PMTiles to R2/S3", "key", s3Key)
	if _, uploadErr := s3Client.UploadFile(ctx, pmtilesPath, s3Key); uploadErr != nil {
		return "", 0, fmt.Errorf("upload failed: %w", uploadErr)
	}

	return pmtilesPath, sizeBytes, nil
}
