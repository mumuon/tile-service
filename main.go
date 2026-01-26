package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

func main() {
	// Parse flags
	configPath := flag.String("config", ".env", "Path to config file")
	debug := flag.Bool("debug", false, "Enable debug logging")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Show help if requested or no arguments provided
	args := flag.Args()
	if *help || len(args) == 0 {
		showHelp()
		os.Exit(0)
	}

	command := args[0]

	// Setup logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Handle different commands
	if command == "generate" {
		cmdGenerate(args[1:], configPath, debug)
	} else if command == "upload" {
		cmdUpload(args[1:], configPath, debug)
	} else if command == "extract" {
		cmdExtract(args[1:], configPath, debug)
	} else if command == "insert-geometries" {
		cmdInsertGeometries(args[1:], configPath, debug)
	} else if command == "merge" {
		cmdMerge(args[1:], configPath, debug)
	} else if command == "serve" {
		cmdServe(args[1:], configPath, debug)
	} else if command == "verify" {
		cmdVerify(args[1:], configPath, debug)
	} else {
		slog.Error("unknown command", "command", command)
		showHelp()
		os.Exit(1)
	}
}

// cmdGenerate handles tile generation for one or more regions
func cmdGenerate(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	maxZoom := fs.Int("max-zoom", 16, "Maximum zoom level for tiles")
	minZoom := fs.Int("min-zoom", 0, "Minimum zoom level for tiles")
	skipUpload := fs.Bool("skip-upload", false, "Skip R2 upload")
	skipMerge := fs.Bool("skip-merge", false, "Skip merging with other regions (for batch processing)")
	noCleanup := fs.Bool("no-cleanup", false, "Don't cleanup temporary files")
	extractGeometry := fs.Bool("extract-geometry", true, "Extract road geometries into database")
	skipGeometryInsertion := fs.Bool("skip-geometry-insertion", false, "Extract geometries to file but don't insert to database")
	mergeAll := fs.Bool("merge-all", false, "Merge all regions instead of just overlapping neighbors")
	workers := fs.Int("workers", 1, "Number of parallel workers for multi-region generation")
	fs.Parse(args)

	regions := fs.Args()
	if len(regions) == 0 {
		slog.Error("at least one region required")
		os.Exit(1)
	}

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database connection (optional)
	db, err := NewDatabase(cfg.Database)
	if err != nil {
		slog.Warn("failed to connect to database (continuing without job tracking)", "error", err)
		db = nil
	} else {
		defer db.Close()
	}

	// Initialize S3 client
	s3Client, err := NewS3Client(cfg.S3)
	if err != nil {
		slog.Error("failed to initialize S3 client", "error", err)
		os.Exit(1)
	}

	// Create service
	service := NewTileService(db, s3Client, cfg)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Build job options (shared across all regions)
	opts := &JobOptions{
		MaxZoom:               *maxZoom,
		MinZoom:               *minZoom,
		SkipUpload:            *skipUpload,
		SkipMerge:             *skipMerge,
		NoCleanup:             *noCleanup,
		ExtractGeometry:       *extractGeometry,
		SkipGeometryInsertion: *skipGeometryInsertion,
		MergeAll:              *mergeAll,
	}

	// Single region - simple path
	if len(regions) == 1 {
		region := regions[0]
		slog.Info("starting tile generation", "region", region, "max_zoom", *maxZoom, "min_zoom", *minZoom, "skip_upload", *skipUpload)

		done := make(chan error, 1)
		go func() {
			job := &TileJob{
				ID:     "1",
				Region: region,
				Status: "pending",
			}
			done <- service.ProcessJobWithOptions(ctx, job, opts)
		}()

		select {
		case err := <-done:
			if err != nil {
				slog.Error("tile generation failed", "error", err)
				os.Exit(1)
			}
			slog.Info("tile generation completed successfully")
		case sig := <-sigChan:
			slog.Info("received shutdown signal", "signal", sig)
			cancel()
			<-done
			os.Exit(1)
		}
		return
	}

	// Multiple regions - parallel processing with worker pool
	numWorkers := *workers
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > len(regions) {
		numWorkers = len(regions)
	}

	slog.Info("starting batch tile generation",
		"regions", len(regions),
		"workers", numWorkers,
		"skip_upload", *skipUpload,
		"skip_merge", *skipMerge,
	)

	// Create work channel and results tracking
	workChan := make(chan string, len(regions))
	for _, region := range regions {
		workChan <- region
	}
	close(workChan)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	var succeeded []string

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for region := range workChan {
				// Check if context cancelled
				select {
				case <-ctx.Done():
					return
				default:
				}

				logger := slog.With("worker", workerID, "region", region)
				logger.Info("starting region")

				job := &TileJob{
					ID:     fmt.Sprintf("batch-%d-%s", workerID, region),
					Region: region,
					Status: "pending",
				}

				err := service.ProcessJobWithOptions(ctx, job, opts)

				mu.Lock()
				if err != nil {
					logger.Error("region failed", "error", err)
					failed = append(failed, region)
				} else {
					logger.Info("region completed")
					succeeded = append(succeeded, region)
				}
				mu.Unlock()
			}
		}(i)
	}

	// Wait for completion or signal
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("batch generation completed",
			"succeeded", len(succeeded),
			"failed", len(failed),
		)
		if len(failed) > 0 {
			slog.Error("failed regions", "regions", failed)
			os.Exit(1)
		}
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
		<-done
		os.Exit(1)
	}
}

// cmdUpload handles uploading pre-generated tiles to R2
func cmdUpload(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	maxZoom := fs.Int("max-zoom", -1, "Maximum zoom level to upload (-1 = all)")
	minZoom := fs.Int("min-zoom", -1, "Minimum zoom level to upload (-1 = all)")
	fs.Parse(args)

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("tiles directory required")
		os.Exit(1)
	}
	tilesDir := parsedArgs[0]

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting tile upload", "tiles_dir", tilesDir, "min_zoom", *minZoom, "max_zoom", *maxZoom)

	// Initialize S3 client
	s3Client, err := NewS3Client(cfg.S3)
	if err != nil {
		slog.Error("failed to initialize S3 client", "error", err)
		os.Exit(1)
	}

	// Create service
	service := NewTileService(nil, s3Client, cfg)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run upload
	done := make(chan error, 1)
	go func() {
		// Extract region from directory name (e.g., "~/data/df/tiles/oregon" -> "oregon")
		region := filepath.Base(tilesDir)
		uploadedBytes, err := service.UploadToR2WithZoomFilter(ctx, tilesDir, region, *minZoom, *maxZoom)
		if err != nil {
			done <- err
		} else {
			slog.Info("upload completed successfully", "uploaded_bytes", uploadedBytes)
			done <- nil
		}
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil {
			slog.Error("upload failed", "error", err)
			os.Exit(1)
		}
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
		<-done
		os.Exit(1)
	}
}

// cmdExtract handles extracting road geometries from existing tiles
func cmdExtract(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	fs.Parse(args)

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("tiles directory required")
		os.Exit(1)
	}
	tilesDir := parsedArgs[0]

	// Extract region from directory path (e.g., "~/data/df/tiles/oregon" -> "oregon")
	region := filepath.Base(tilesDir)

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting road geometry extraction", "tiles_dir", tilesDir, "region", region)

	// Initialize database connection (required for extraction)
	db, err := NewDatabase(cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database (required for extraction)", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create service
	service := NewTileService(db, nil, cfg)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run extraction
	done := make(chan error, 1)
	go func() {
		count, err := service.ExtractRoadGeometriesFromExistingTiles(ctx, tilesDir, region)
		if err != nil {
			done <- err
		} else {
			slog.Info("extraction completed successfully", "roads_inserted", count)
			done <- nil
		}
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil {
			slog.Error("extraction failed", "error", err)
			os.Exit(1)
		}
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
		<-done
		os.Exit(1)
	}
}

// cmdInsertGeometries handles batch insertion of extracted road geometries
func cmdInsertGeometries(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("insert-geometries", flag.ExitOnError)
	fs.Parse(args)

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("extraction file or region required")
		slog.Info("Usage: tile-service insert-geometries <extraction_file_or_region>")
		os.Exit(1)
	}
	fileOrRegion := parsedArgs[0]

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database connection (required)
	db, err := NewDatabase(cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database (required for insertion)", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	extractor := NewGeometryExtractor()

	// Determine if input is a file or region name
	var extractionFile string
	var region string

	if _, err := os.Stat(fileOrRegion); err == nil {
		// It's a file
		extractionFile = fileOrRegion
		// Try to extract region from filename: .extracted-roads-{region}.json
		base := filepath.Base(fileOrRegion)
		region = strings.TrimPrefix(base, ".extracted-roads-")
		region = strings.TrimSuffix(region, ".json")
		slog.Info("inserting from file", "file", extractionFile, "region", region)
	} else {
		// It's a region name
		region = fileOrRegion
		extractionFile = extractor.getExtractionFile(region)
		if _, err := os.Stat(extractionFile); os.IsNotExist(err) {
			slog.Error("extraction file not found", "file", extractionFile, "region", region)
			slog.Info("Run extraction first: tile-service generate -skip-geometry-insertion " + region)
			os.Exit(1)
		}
		slog.Info("inserting from region", "region", region, "file", extractionFile)
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run insertion
	done := make(chan error, 1)
	go func() {
		// Load roads from file
		roads, err := extractor.loadRoadsFromFile(extractionFile)
		if err != nil {
			done <- fmt.Errorf("failed to load extraction file: %w", err)
			return
		}

		slog.Info("loaded roads from file", "count", len(roads))

		// Insert into database with large batch size (multi-row INSERT is efficient)
		inserted, err := db.BatchUpsertRoadGeometries(ctx, roads, 9000)
		if err != nil {
			done <- fmt.Errorf("failed to insert road geometries: %w", err)
			return
		}

		slog.Info("insertion completed successfully", "roads_inserted", inserted)

		// Cleanup extraction files after successful insertion
		if err := extractor.CleanupExtractionFiles(region); err != nil {
			slog.Warn("failed to cleanup extraction files", "error", err)
		}

		done <- nil
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil {
			slog.Error("insertion failed", "error", err)
			os.Exit(1)
		}
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
		<-done
		os.Exit(1)
	}
}

// cmdMerge handles merging regional tiles into a single merged output
func cmdMerge(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	skipUpload := fs.Bool("skip-upload", false, "Skip R2 upload after merging")
	forRegion := fs.String("for", "", "Only merge regions that overlap with specified region (e.g., --for washington)")
	minZoom := fs.Int("min-zoom", -1, "Minimum zoom level to merge (-1 = all)")
	maxZoom := fs.Int("max-zoom", -1, "Maximum zoom level to merge (-1 = all)")
	fs.Parse(args)

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Get list of regions to merge
	parsedArgs := fs.Args()
	var inputDirs []string

	if *forRegion != "" {
		// Find only overlapping regions for efficiency
		inputDirs, err = FindOverlappingRegions(cfg.Paths.OutputDir, *forRegion)
		if err != nil {
			slog.Error("failed to find overlapping regions", "error", err, "for_region", *forRegion)
			os.Exit(1)
		}
		if len(inputDirs) == 0 {
			slog.Error("no overlapping regions found", "for_region", *forRegion)
			os.Exit(1)
		}
		slog.Info("merging overlapping regions", "for_region", *forRegion, "count", len(inputDirs), "dirs", inputDirs)
	} else if len(parsedArgs) > 0 {
		// Merge specific regions
		for _, region := range parsedArgs {
			regionDir := filepath.Join(cfg.Paths.OutputDir, region)
			if _, err := os.Stat(regionDir); os.IsNotExist(err) {
				slog.Error("region tiles not found", "region", region, "dir", regionDir)
				os.Exit(1)
			}
			inputDirs = append(inputDirs, regionDir)
		}
		slog.Info("merging specified regions", "regions", parsedArgs)
	} else {
		// Find all regional tile directories
		inputDirs, err = FindRegionalTileDirs(cfg.Paths.OutputDir)
		if err != nil {
			slog.Error("failed to find regional tile directories", "error", err)
			os.Exit(1)
		}
		if len(inputDirs) == 0 {
			slog.Error("no regional tile directories found", "base_dir", cfg.Paths.OutputDir)
			os.Exit(1)
		}
		slog.Info("merging all regions", "count", len(inputDirs), "dirs", inputDirs)
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run merge
	done := make(chan error, 1)
	go func() {
		mergedDir := filepath.Join(cfg.Paths.OutputDir, "merged")

		// Setup merge options with zoom filtering
		var mergeOpts *MergeTilesOptions
		if *minZoom >= 0 || *maxZoom >= 0 {
			mergeOpts = &MergeTilesOptions{
				MinZoom: *minZoom,
				MaxZoom: *maxZoom,
			}
		}

		metadata, err := MergeTilesWithOptions(ctx, inputDirs, mergedDir, mergeOpts)
		if err != nil {
			done <- err
			return
		}

		slog.Info("merge completed", "tiles_count", metadata.TilesCount, "size_bytes", metadata.TotalSize, "output_dir", mergedDir)

		if !*skipUpload {
			// Initialize S3 client and upload
			s3Client, err := NewS3Client(cfg.S3)
			if err != nil {
				done <- fmt.Errorf("failed to initialize S3 client: %w", err)
				return
			}

			service := NewTileService(nil, s3Client, cfg)
			uploadedBytes, err := service.UploadToR2(ctx, mergedDir, "merged")
			if err != nil {
				done <- fmt.Errorf("failed to upload merged tiles: %w", err)
				return
			}

			slog.Info("upload completed", "uploaded_bytes", uploadedBytes)
		} else {
			slog.Info("skipping R2 upload, merged tiles saved locally", "output_dir", mergedDir)
		}

		done <- nil
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil {
			slog.Error("merge failed", "error", err)
			os.Exit(1)
		}
		slog.Info("merge operation completed successfully")
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
		<-done
		os.Exit(1)
	}
}

// cmdServe starts the REST API server
func cmdServe(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8080, "Port to listen on")
	fs.Parse(args)

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting tile service API server", "port", *port)

	// Initialize database connection (optional)
	db, err := NewDatabase(cfg.Database)
	if err != nil {
		slog.Warn("failed to connect to database (continuing without job tracking)", "error", err)
		db = nil
	} else {
		defer db.Close()
	}

	// Initialize S3 client
	s3Client, err := NewS3Client(cfg.S3)
	if err != nil {
		slog.Error("failed to initialize S3 client", "error", err)
		os.Exit(1)
	}

	// Create API server
	apiServer := NewAPIServer(db, s3Client, cfg)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := apiServer.Start(*port); err != nil {
			errChan <- err
		}
	}()

	// Wait for signal or error
	select {
	case err := <-errChan:
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	case sig := <-sigChan:
		slog.Info("received shutdown signal, stopping server", "signal", sig)
		os.Exit(0)
	}
}

// reorderFlagsFirst moves flag arguments before positional arguments so Go's
// flag package parses them correctly. Go's flag stops at the first non-flag arg.
// This allows "verify tiles <dir> --min-zoom 0" to work like "--min-zoom 0 <dir>".
func reorderFlagsFirst(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i])
			// If flag uses "--key value" form (not "--key=value"), grab the next arg as the value
			if !strings.Contains(args[i], "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	return append(flags, positional...)
}

// cmdVerify handles tile verification commands
func cmdVerify(args []string, configPath *string, debug *bool) {
	if len(args) == 0 {
		slog.Error("verify subcommand required: tiles, merge, or upload")
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "tiles":
		cmdVerifyTiles(subArgs)
	case "merge":
		cmdVerifyMerge(subArgs, configPath)
	case "upload":
		cmdVerifyUpload(subArgs, configPath)
	default:
		slog.Error("unknown verify subcommand", "subcommand", subcommand)
		slog.Info("available: tiles, merge, upload")
		os.Exit(1)
	}
}

func cmdVerifyTiles(args []string) {
	fs := flag.NewFlagSet("verify tiles", flag.ExitOnError)
	minZoom := fs.Int("min-zoom", 0, "Minimum expected zoom level")
	maxZoom := fs.Int("max-zoom", 16, "Maximum expected zoom level")
	fs.Parse(reorderFlagsFirst(args))

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("tiles directory required")
		slog.Info("Usage: tile-service verify tiles <dir> [--min-zoom N] [--max-zoom N]")
		os.Exit(1)
	}
	dir := parsedArgs[0]

	report, err := VerifyTileDirectory(dir, *minZoom, *maxZoom)
	if err != nil {
		slog.Error("verification failed", "error", err)
		os.Exit(1)
	}

	report.Print()

	if !report.OK {
		os.Exit(1)
	}
}

func cmdVerifyMerge(args []string, configPath *string) {
	fs := flag.NewFlagSet("verify merge", flag.ExitOnError)
	fs.Parse(args)

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("region required")
		slog.Info("Usage: tile-service verify merge <region>")
		os.Exit(1)
	}
	region := parsedArgs[0]

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	regionDir := filepath.Join(cfg.Paths.OutputDir, region)
	mergedDir := filepath.Join(cfg.Paths.OutputDir, "merged")

	report, err := VerifyMergeIntegrity(regionDir, mergedDir)
	if err != nil {
		slog.Error("merge verification failed", "error", err)
		os.Exit(1)
	}

	report.Print()

	if !report.OK {
		os.Exit(1)
	}
}

func cmdVerifyUpload(args []string, configPath *string) {
	fs := flag.NewFlagSet("verify upload", flag.ExitOnError)
	samplesPerZoom := fs.Int("samples-per-zoom", 5, "Number of tiles to spot-check per zoom level")
	fs.Parse(reorderFlagsFirst(args))

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("region required")
		slog.Info("Usage: tile-service verify upload <region> [--samples-per-zoom N]")
		os.Exit(1)
	}
	region := parsedArgs[0]

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	s3Client, err := NewS3Client(cfg.S3)
	if err != nil {
		slog.Error("failed to initialize S3 client", "error", err)
		os.Exit(1)
	}

	tilesDir := filepath.Join(cfg.Paths.OutputDir, region)
	ctx := context.Background()

	report, err := VerifyUpload(ctx, s3Client, tilesDir, cfg.S3.BucketPath, *samplesPerZoom)
	if err != nil {
		slog.Error("upload verification failed", "error", err)
		os.Exit(1)
	}

	report.Print()

	if !report.OK {
		os.Exit(1)
	}
}

func showHelp() {
	help := `Tile Service - Generate vector tiles from road geometry data

Usage:
  tile-service [global options] <command> [command options] [arguments]

Global Options:
  -config string        Path to .env configuration file (default ".env")
  -debug                Enable debug logging
  -help                 Show this help message

Commands:
  generate              Generate tiles from road geometry data
  upload                Upload pre-generated tiles to R2
  extract               Extract road geometries from existing tiles into database
  insert-geometries     Insert extracted road geometries from file into database
  merge                 Merge regional tiles and upload to R2
  verify                Verify tile integrity, merge completeness, or upload status
  serve                 Start the REST API server

Generate Command:
  Usage: tile-service generate [options] <region> [region2] [region3] ...

  Arguments:
    <region>              One or more region names (e.g., washington oregon california)

  Options:
    -max-zoom int         Maximum zoom level (default 16)
    -min-zoom int         Minimum zoom level (default 0)
    -skip-upload          Skip R2 upload, keep tiles locally in ~/data/df/tiles
    -skip-merge           Skip merging with other regions (for batch processing)
    -no-cleanup           Don't cleanup temporary files after completion
    -extract-geometry     Extract road geometries into database (default true)
    -skip-geometry-insertion  Extract to file but don't insert to database
    -merge-all            Merge all regions instead of just overlapping neighbors
    -workers int          Number of parallel workers for multi-region generation (default 1)

Upload Command:
  Usage: tile-service upload [options] <tiles_directory>

  Arguments:
    <tiles_directory>     Path to the tiles directory to upload (e.g., ~/data/df/tiles/oregon)

  Options:
    -min-zoom int         Minimum zoom level to upload (-1 = all, default -1)
    -max-zoom int         Maximum zoom level to upload (-1 = all, default -1)

Extract Command:
  Usage: tile-service extract <tiles_directory>

  Arguments:
    <tiles_directory>     Path to the tiles directory (e.g., ~/data/df/tiles/oregon)

  Description:
    Extracts road bounding boxes from vector tiles and stores them in the database.
    This enables the "Find Nearby Roads" feature in the application.
    Can be run on existing tiles without regenerating them.

Insert Geometries Command:
  Usage: tile-service insert-geometries <extraction_file_or_region>

  Arguments:
    <extraction_file_or_region>  Either:
                                 - Path to extraction file (.extracted-roads-{region}.json)
                                 - Region name (will look for .extracted-roads-{region}.json)

  Description:
    Batch inserts road geometries from extraction file into database.
    Use this after generating tiles with -skip-geometry-insertion flag.
    Allows you to review extracted data before inserting to database.

Merge Command:
  Usage: tile-service merge [options] [regions...]

  Arguments:
    [regions...]          Optional list of regions to merge (e.g., washington oregon)
                          If not specified, merges all regional tile directories

  Options:
    -skip-upload          Skip R2 upload after merging (keep tiles locally)
    -for <region>         Only merge regions that have overlapping tiles with <region>
                          This is more efficient than merging all regions

  Description:
    Merges multiple regional tile directories into a single "merged" directory
    using tile-join. This combines tiles from overlapping regions so all roads
    are visible at low zoom levels. The merged tiles are then uploaded to R2.

    This command is useful for:
    - Re-merging after adding new regional tiles without regenerating
    - Fixing R2 after accidental overwrites from individual region uploads
    - Manual control over which regions to include in the merged output
    - Using --for to efficiently merge only neighboring regions

Verify Command:
  Usage: tile-service verify <subcommand> [options] [arguments]

  Subcommands:
    tiles                 Verify tile directory has all expected zoom levels
    merge                 Verify merge completeness for a region
    upload                Spot-check that tiles exist on R2

  Verify Tiles:
    Usage: tile-service verify tiles <dir> [--min-zoom N] [--max-zoom N]

    Options:
      -min-zoom int         Minimum expected zoom level (default 0)
      -max-zoom int         Maximum expected zoom level (default 16)

  Verify Merge:
    Usage: tile-service verify merge <region>

  Verify Upload:
    Usage: tile-service verify upload <region> [--samples-per-zoom N]

    Options:
      -samples-per-zoom int Number of tiles to spot-check per zoom level (default 5)

  Description:
    Exits 0 if verification passes, 1 if issues are found.

Serve Command:
  Usage: tile-service serve [options]

  Options:
    -port int             Port to listen on (default 8080)

  Description:
    Starts the REST API server for tile generation.

    API Endpoints:
      POST   /api/generate          - Submit a new tile generation job
      GET    /api/jobs              - List all active jobs
      GET    /api/jobs/{jobId}      - Get status of a specific job
      GET    /api/stream/{jobId}    - Stream real-time job updates (SSE)
      GET    /health                - Health check endpoint

Examples:
  # Generate tiles for Washington with full pipeline
  ./tile-service generate washington

  # Generate tiles with custom zoom level
  ./tile-service generate -max-zoom 14 washington

  # Generate tiles without uploading to R2
  ./tile-service generate -skip-upload oregon

  # Generate tiles with minimal zoom levels for debugging
  ./tile-service generate -max-zoom 7 -skip-upload -no-cleanup maryland

  # Batch generate multiple regions with 4 parallel workers (no upload/merge)
  ./tile-service generate -workers 4 -skip-upload -skip-merge washington oregon california idaho

  # Batch generate all US states, then merge once (efficient workflow)
  ./tile-service generate -workers 4 -skip-upload -skip-merge alabama alaska arizona ...
  ./tile-service merge

  # Upload pre-generated tiles
  ./tile-service upload ~/data/df/tiles/oregon

  # Upload only the most zoomed-out level
  ./tile-service upload -min-zoom 5 -max-zoom 5 ~/data/df/tiles/oregon

  # Upload zoom levels 7-12
  ./tile-service upload -min-zoom 7 -max-zoom 12 ~/data/df/tiles/oregon

  # Extract road geometries from existing tiles
  ./tile-service extract ~/data/df/tiles/oregon

  # Generate tiles and extract geometries to file (don't insert yet)
  ./tile-service generate -skip-upload -skip-geometry-insertion florida

  # Later, insert the extracted geometries
  ./tile-service insert-geometries florida
  # or
  ./tile-service insert-geometries .extracted-roads-florida.json

  # Generate tiles without geometry extraction
  ./tile-service generate -extract-geometry=false washington

  # Merge all regional tiles and upload to R2
  ./tile-service merge

  # Merge only regions that overlap with washington (faster)
  ./tile-service merge --for washington

  # Merge specific regions only
  ./tile-service merge washington oregon california

  # Merge tiles locally without uploading
  ./tile-service merge --skip-upload

  # Start the REST API server
  ./tile-service serve

  # Start the REST API server on a custom port
  ./tile-service serve -port 3000

  # Verify tiles have all expected zoom levels
  ./tile-service verify tiles ~/data/df/tiles/arkansas --min-zoom 0 --max-zoom 16

  # Verify merge completeness for a region
  ./tile-service verify merge arkansas

  # Spot-check uploaded tiles on R2
  ./tile-service verify upload arkansas --samples-per-zoom 10

  # Debug mode
  ./tile-service -debug generate -max-zoom 8 washington
`
	fmt.Print(help)
}

