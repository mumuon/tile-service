package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// TileService orchestrates tile generation
type TileService struct {
	db     *Database
	s3     *S3Client
	config *Config
}

// NewTileService creates a new tile service
func NewTileService(db *Database, s3 *S3Client, config *Config) *TileService {
	return &TileService{
		db:     db,
		s3:     s3,
		config: config,
	}
}

// UploadToR2 uploads generated tiles to R2 storage
func (s *TileService) UploadToR2(ctx context.Context, tilesDir, region string) (int64, error) {
	logger := slog.With("region", region, "tiles_dir", tilesDir)
	logger.Info("starting R2 upload")

	totalBytes, err := s.s3.UploadDirectory(ctx, tilesDir, s.config.S3.BucketPath)
	if err != nil {
		return 0, fmt.Errorf("failed to upload to R2: %w", err)
	}

	logger.Info("R2 upload completed", "total_bytes", totalBytes)
	return totalBytes, nil
}

// UploadToR2WithZoomFilter uploads generated tiles to R2 with optional zoom level filtering
// Filters locally by only walking through the specified zoom level directories
func (s *TileService) UploadToR2WithZoomFilter(ctx context.Context, tilesDir, region string, minZoom, maxZoom int) (int64, error) {
	logger := slog.With("region", region, "tiles_dir", tilesDir, "min_zoom", minZoom, "max_zoom", maxZoom)
	logger.Info("starting R2 upload with zoom filter")

	// If no zoom filtering, just upload everything
	if minZoom == -1 && maxZoom == -1 {
		totalBytes, err := s.s3.UploadDirectory(ctx, tilesDir, s.config.S3.BucketPath)
		if err != nil {
			return 0, fmt.Errorf("failed to upload to R2: %w", err)
		}
		logger.Info("R2 upload completed", "total_bytes", totalBytes)
		return totalBytes, nil
	}

	// Walk the tiles directory and identify zoom level directories to include
	logger.Info("filtering tiles by zoom level", "min_zoom", minZoom, "max_zoom", maxZoom)

	var totalBytes int64

	// Read the tiles directory to find zoom level folders
	entries, err := os.ReadDir(tilesDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read tiles directory: %w", err)
	}

	// Process each zoom level directory that's in our range
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		zoomLevel, err := strconv.Atoi(dirName)
		if err != nil {
			// Skip non-numeric directories (like metadata.json parent)
			continue
		}

		// Check if zoom level is in range
		if minZoom != -1 && zoomLevel < minZoom {
			continue
		}
		if maxZoom != -1 && zoomLevel > maxZoom {
			continue
		}

		// Upload this zoom level directory
		zoomDir := filepath.Join(tilesDir, dirName)
		logger.Info("uploading zoom level", "zoom", zoomLevel)

		bytes, err := s.s3.UploadDirectory(ctx, zoomDir, filepath.Join(s.config.S3.BucketPath, dirName))
		if err != nil {
			return 0, fmt.Errorf("failed to upload zoom level %d: %w", zoomLevel, err)
		}

		totalBytes += bytes
	}

	logger.Info("R2 upload completed", "total_bytes", totalBytes)
	return totalBytes, nil
}

// ProcessJobWithOptions orchestrates the entire tile generation pipeline with custom options
func (s *TileService) ProcessJobWithOptions(ctx context.Context, job *TileJob, opts *JobOptions) error {
	logger := slog.With("region", job.Region, "min_zoom", opts.MinZoom, "max_zoom", opts.MaxZoom)

	// Update database status if available
	if s.db != nil {
		if err := s.db.UpdateJobStatus(ctx, job.ID, "extracting"); err != nil {
			logger.Warn("failed to update job status", "error", err)
		}
	}

	// Phase 1: Extract KMZ
	logger.Info("extracting KMZ")
	kmlPath, err := ExtractKMZFromDir(ctx, job.Region, s.config.Paths.CurvatureData)
	if err != nil {
		if s.db != nil {
			s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("extraction failed: %v", err))
		}
		return fmt.Errorf("failed to extract KMZ: %w", err)
	}
	logger.Debug("KMZ extracted", "kml_path", kmlPath)

	// Phase 2: Convert KML to GeoJSON
	logger.Info("converting KML to GeoJSON")
	geoJSONPath, roadsCount, err := ConvertKMLToGeoJSON(ctx, kmlPath, job.Region)
	if err != nil {
		if s.db != nil {
			s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("conversion failed: %v", err))
		}
		return fmt.Errorf("failed to convert KML: %w", err)
	}
	logger.Info("KML converted", "geojson_path", geoJSONPath, "roads_count", roadsCount)

	if s.db != nil {
		if err := s.db.UpdateJobProgress(ctx, job.ID, roadsCount, 0); err != nil {
			logger.Warn("failed to update progress", "error", err)
		}
	}

	// Phase 3: Generate tiles with Tippecanoe
	logger.Info("generating tiles with Tippecanoe")
	if s.db != nil {
		if err := s.db.UpdateJobStatus(ctx, job.ID, "generating"); err != nil {
			logger.Warn("failed to update job status", "error", err)
		}
	}

	// Note: Currently GenerateTiles uses hardcoded 5-16 zoom levels
	// TODO: Make Tippecanoe zoom levels configurable via opts.MinZoom/opts.MaxZoom
	tilesDir, tilesCount, totalSize, err := GenerateTiles(ctx, geoJSONPath, job.Region)
	if err != nil {
		if s.db != nil {
			s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("tile generation failed: %v", err))
		}
		return fmt.Errorf("failed to generate tiles: %w", err)
	}
	logger.Info("tiles generated", "tiles_dir", tilesDir, "tiles_count", tilesCount, "size_bytes", totalSize)

	if s.db != nil {
		if err := s.db.UpdateJobProgress(ctx, job.ID, roadsCount, tilesCount); err != nil {
			logger.Warn("failed to update progress", "error", err)
		}
	}

	// Phase 4: Upload to R2 (unless skipped)
	if !opts.SkipUpload {
		logger.Info("uploading tiles to R2")
		if s.db != nil {
			if err := s.db.UpdateJobStatus(ctx, job.ID, "uploading"); err != nil {
				logger.Warn("failed to update job status", "error", err)
			}
		}

		uploadedBytes, err := s.UploadToR2(ctx, tilesDir, job.Region)
		if err != nil {
			if s.db != nil {
				s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("R2 upload failed: %v", err))
			}
			return fmt.Errorf("failed to upload to R2: %w", err)
		}
		logger.Info("upload completed", "uploaded_bytes", uploadedBytes)
	} else {
		logger.Info("skipping R2 upload, tiles saved locally", "tiles_dir", tilesDir)
	}

	// Phase 5: Mark as complete
	if s.db != nil {
		if err := s.db.CompleteJob(ctx, job.ID, roadsCount, tilesCount, totalSize); err != nil {
			logger.Warn("failed to mark job complete", "error", err)
		}
	}

	// Cleanup temporary files (unless disabled)
	if !opts.NoCleanup {
		if err := CleanupTemporaryFiles(ctx, kmlPath, geoJSONPath, tilesDir); err != nil {
			logger.Warn("failed to cleanup temporary files", "error", err)
		}
	} else {
		logger.Info("skipping cleanup, temporary files preserved", "kml_path", kmlPath, "geojson_path", geoJSONPath)
	}

	logger.Info("job processing complete")
	return nil
}
