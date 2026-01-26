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

// UploadMergedTilesForRegion uploads only tiles from mergedDir that match the region's tile coordinates
// This is efficient: we get merged content (multi-region roads) but only upload the target region's tiles
func (s *TileService) UploadMergedTilesForRegion(ctx context.Context, mergedDir, regionDir, region string) (int64, error) {
	logger := slog.With("region", region, "merged_dir", mergedDir, "region_dir", regionDir)
	logger.Info("starting R2 upload of merged tiles for region only")

	// Get the tile coordinates from the region directory
	regionCoords, err := GetTileCoords(regionDir)
	if err != nil {
		return 0, fmt.Errorf("failed to get region tile coords: %w", err)
	}

	logger.Info("uploading merged tiles for region coordinates", "tile_count", len(regionCoords))

	// Upload only the tiles that match the region's coordinates
	totalBytes, err := s.s3.UploadTilesWithFilter(ctx, mergedDir, s.config.S3.BucketPath, regionCoords)
	if err != nil {
		return 0, fmt.Errorf("failed to upload to R2: %w", err)
	}

	logger.Info("R2 upload completed", "total_bytes", totalBytes, "tiles_uploaded", len(regionCoords))
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
	logger := slog.With("region", job.Region, "min_zoom", opts.MinZoom, "max_zoom", opts.MaxZoom, "skip_generation", opts.SkipGeneration)

	var tilesDir string
	var tilesCount int
	var totalSize int64
	var roadsCount int
	var kmlPath, geoJSONPath string

	// Ensure cleanup of temporary files on ANY exit (success or failure)
	// This prevents temp directory accumulation when processing fails mid-way
	defer func() {
		if opts.NoCleanup {
			logger.Info("skipping cleanup (--no-cleanup flag set)", "kml_path", kmlPath, "geojson_path", geoJSONPath)
			return
		}
		if kmlPath != "" || geoJSONPath != "" {
			if err := CleanupTemporaryFiles(ctx, kmlPath, geoJSONPath, ""); err != nil {
				logger.Warn("failed to cleanup temporary files", "error", err)
			}
		}
	}()

	// If skipGeneration is true, skip tile generation and use existing tiles
	if opts.SkipGeneration {
		logger.Info("skipping tile generation, using existing tiles")

		// Use existing tiles directory
		tilesDir = filepath.Join(s.config.Paths.OutputDir, job.Region)

		// Check if tiles directory exists
		if _, err := os.Stat(tilesDir); os.IsNotExist(err) {
			if s.db != nil {
				s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("tiles directory does not exist: %s", tilesDir))
			}
			return fmt.Errorf("tiles directory does not exist: %s", tilesDir)
		}

		// Count existing tiles
		var err error
		tilesCount, err = countTiles(tilesDir)
		if err != nil {
			logger.Warn("failed to count tiles", "error", err)
		}

		totalSize, err = getDirectorySize(tilesDir)
		if err != nil {
			logger.Warn("failed to calculate directory size", "error", err)
		}

		logger.Info("using existing tiles", "tiles_dir", tilesDir, "tiles_count", tilesCount, "size_bytes", totalSize)

		if s.db != nil {
			if err := s.db.UpdateJobStatus(ctx, job.ID, "uploading"); err != nil {
				logger.Warn("failed to update job status", "error", err)
			}
		}
	} else {
		// Normal flow: generate tiles

		// Update database status if available
		if s.db != nil {
			if err := s.db.UpdateJobStatus(ctx, job.ID, "extracting"); err != nil {
				logger.Warn("failed to update job status", "error", err)
			}
		}

		// Phase 1: Extract KMZ
		logger.Info("extracting KMZ")
		var err error
		kmlPath, err = ExtractKMZFromDir(ctx, job.Region, s.config.Paths.CurvatureData)
		if err != nil {
			if s.db != nil {
				s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("extraction failed: %v", err))
			}
			return fmt.Errorf("failed to extract KMZ: %w", err)
		}
		logger.Debug("KMZ extracted", "kml_path", kmlPath)

		// Phase 2: Convert KML to GeoJSON
		logger.Info("converting KML to GeoJSON")
		geoJSONPath, roadsCount, err = ConvertKMLToGeoJSON(ctx, kmlPath, job.Region)
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

		// Generate tiles with configurable zoom levels
		genOpts := &GenerateTilesOptions{
			MinZoom: opts.MinZoom,
			MaxZoom: opts.MaxZoom,
		}
		tilesDir, tilesCount, totalSize, err = GenerateTilesWithOptions(ctx, geoJSONPath, job.Region, s.config.Paths.OutputDir, genOpts)
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
	}

	// Phase 4: Merge regional tiles
	// Skip merge if SkipMerge is set (useful for batch processing multiple regions)
	// By default, only merge with overlapping neighbors for efficiency
	// Use MergeAll option to merge all regions
	var mergedDir string
	if opts.SkipMerge {
		logger.Info("skipping merge (--skip-merge flag set)")
	} else {
		logger.Info("merging regional tiles", "merge_all", opts.MergeAll)
		if s.db != nil {
			if err := s.db.UpdateJobStatus(ctx, job.ID, "merging"); err != nil {
				logger.Warn("failed to update job status", "error", err)
			}
		}

		var regionDirs []string
		var err error

		if opts.MergeAll {
			// Merge all regions
			regionDirs, err = FindRegionalTileDirs(s.config.Paths.OutputDir)
			if err != nil {
				return fmt.Errorf("failed to find regional tile directories: %w", err)
			}
			logger.Info("merging all regions", "count", len(regionDirs))
		} else {
			// Only merge with overlapping neighbors (more efficient)
			regionDirs, err = FindOverlappingRegions(s.config.Paths.OutputDir, job.Region)
			if err != nil {
				return fmt.Errorf("failed to find overlapping regions: %w", err)
			}
			logger.Info("merging overlapping regions only", "count", len(regionDirs), "dirs", regionDirs)
		}

		if len(regionDirs) == 0 {
			return fmt.Errorf("no regional tile directories found in %s", s.config.Paths.OutputDir)
		}

		mergedDir = filepath.Join(s.config.Paths.OutputDir, "merged")
		mergeMetadata, err := MergeTiles(ctx, regionDirs, mergedDir)
		if err != nil {
			return fmt.Errorf("failed to merge tiles: %w", err)
		}
		logger.Info("tiles merged", "regions", len(regionDirs), "tiles_count", mergeMetadata.TilesCount, "size_bytes", mergeMetadata.TotalSize)
	}

	// Phase 5 & 6: Run geometry extraction and R2 upload in parallel
	// This significantly speeds up processing by utilizing concurrent I/O operations
	logger.Info("starting parallel operations: geometry extraction and R2 upload")

	// Update job status to show we're in the upload/extraction phase
	if s.db != nil {
		statusMsg := "uploading"
		if opts.ExtractGeometry && !opts.SkipUpload {
			statusMsg = "uploading/extracting"
		} else if opts.ExtractGeometry {
			statusMsg = "extracting"
		}
		if err := s.db.UpdateJobStatus(ctx, job.ID, statusMsg); err != nil {
			logger.Warn("failed to update job status", "error", err)
		}
	}

	// Use channels to collect results and errors
	type uploadResult struct {
		bytes int64
		err   error
	}
	type geometryResult struct {
		count int
		err   error
	}

	uploadChan := make(chan uploadResult, 1)
	geometryChan := make(chan geometryResult, 1)

	// Goroutine 1: Extract and insert road geometries (from regional tiles, not merged)
	if opts.ExtractGeometry {
		go func() {
			logger.Info("starting road geometry extraction (parallel)")
			extractor := NewGeometryExtractor()

			roads, err := extractor.ExtractRoadGeometriesFromTiles(ctx, tilesDir, job.Region)
			if err != nil {
				logger.Warn("failed to extract road geometries", "error", err)
				geometryChan <- geometryResult{0, err}
				return
			}

			logger.Info("road geometries extracted", "count", len(roads))

			if opts.SkipGeometryInsertion {
				logger.Info("skipping database insertion, geometries saved to file",
					"file", extractor.getExtractionFile(job.Region))
				geometryChan <- geometryResult{len(roads), nil}
			} else if s.db != nil {
				// Insert into database with large batch size
				inserted, err := s.db.BatchUpsertRoadGeometries(ctx, roads, 9000)
				if err != nil {
					logger.Warn("failed to insert road geometries", "error", err)
					geometryChan <- geometryResult{0, err}
					return
				}

				logger.Info("road geometries inserted into database", "count", inserted)

				// Cleanup extraction files after successful insertion
				if err := extractor.CleanupExtractionFiles(job.Region); err != nil {
					logger.Warn("failed to cleanup extraction files", "error", err)
				}

				geometryChan <- geometryResult{inserted, nil}
			} else {
				logger.Warn("database not available, geometries saved to file only")
				geometryChan <- geometryResult{len(roads), nil}
			}
		}()
	} else {
		// If not extracting geometry, send a success result immediately
		geometryChan <- geometryResult{0, nil}
	}

	// Goroutine 2: Upload MERGED tiles to R2, but only for the region's tile coordinates
	// This gives us merged content (multi-region roads) but we only pay for the region's tile count
	if !opts.SkipUpload {
		go func() {
			logger.Info("starting R2 upload of merged tiles for region coordinates", "merged_dir", mergedDir, "region", job.Region)
			uploadedBytes, err := s.UploadMergedTilesForRegion(ctx, mergedDir, tilesDir, job.Region)
			if err != nil {
				logger.Error("R2 upload failed", "error", err)
				uploadChan <- uploadResult{0, err}
				return
			}
			logger.Info("R2 upload completed", "uploaded_bytes", uploadedBytes)
			uploadChan <- uploadResult{uploadedBytes, nil}
		}()
	} else {
		logger.Info("skipping R2 upload, tiles saved locally", "tiles_dir", tilesDir)
		uploadChan <- uploadResult{0, nil}
	}

	// Wait for both operations to complete
	uploadRes := <-uploadChan
	geometryRes := <-geometryChan

	// Check for errors
	if uploadRes.err != nil {
		if s.db != nil {
			s.db.UpdateJobError(ctx, job.ID, fmt.Sprintf("R2 upload failed: %v", uploadRes.err))
		}
		return fmt.Errorf("failed to upload to R2: %w", uploadRes.err)
	}

	// Geometry extraction errors are non-fatal (we already logged them)
	if geometryRes.err != nil {
		logger.Warn("geometry extraction completed with errors", "error", geometryRes.err)
	}

	logger.Info("parallel operations completed",
		"uploaded_bytes", uploadRes.bytes,
		"geometry_count", geometryRes.count)

	// Phase 7: Mark as complete
	if s.db != nil {
		if err := s.db.CompleteJob(ctx, job.ID, roadsCount, tilesCount, totalSize); err != nil {
			logger.Warn("failed to mark job complete", "error", err)
		}
	}

	// Note: Cleanup is handled by defer at the top of this function

	logger.Info("job processing complete")
	return nil
}

// ExtractRoadGeometriesFromExistingTiles extracts road geometries from already-generated tiles
func (s *TileService) ExtractRoadGeometriesFromExistingTiles(ctx context.Context, tilesDir, region string) (int, error) {
	logger := slog.With("region", region, "tiles_dir", tilesDir)
	logger.Info("extracting road geometries from existing tiles")

	extractor := NewGeometryExtractor()

	// Extract roads from tiles
	roads, err := extractor.ExtractRoadGeometriesFromTiles(ctx, tilesDir, region)
	if err != nil {
		return 0, fmt.Errorf("failed to extract road geometries: %w", err)
	}

	logger.Info("road geometries extracted", "count", len(roads))

	// Insert into database if available
	if s.db != nil {
		inserted, err := s.db.BatchUpsertRoadGeometries(ctx, roads, 9000)
		if err != nil {
			return 0, fmt.Errorf("failed to insert road geometries: %w", err)
		}
		logger.Info("road geometries inserted into database", "count", inserted)

		// Cleanup extraction files
		if err := extractor.CleanupExtractionFiles(region); err != nil {
			logger.Warn("failed to cleanup extraction files", "error", err)
		}

		return inserted, nil
	}

	return len(roads), nil
}
