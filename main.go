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
	} else if command == "serve" {
		cmdServe(args[1:], configPath, debug)
	} else {
		slog.Error("unknown command", "command", command)
		showHelp()
		os.Exit(1)
	}
}

// cmdGenerate handles tile generation
func cmdGenerate(args []string, configPath *string, debug *bool) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	maxZoom := fs.Int("max-zoom", 16, "Maximum zoom level for tiles")
	minZoom := fs.Int("min-zoom", 5, "Minimum zoom level for tiles")
	skipUpload := fs.Bool("skip-upload", false, "Skip R2 upload")
	noCleanup := fs.Bool("no-cleanup", false, "Don't cleanup temporary files")
	extractGeometry := fs.Bool("extract-geometry", true, "Extract road geometries into database")
	skipGeometryInsertion := fs.Bool("skip-geometry-insertion", false, "Extract geometries to file but don't insert to database")
	fs.Parse(args)

	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		slog.Error("region required")
		os.Exit(1)
	}
	region := parsedArgs[0]

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("starting tile generation", "region", region, "max_zoom", *maxZoom, "min_zoom", *minZoom, "skip_upload", *skipUpload)

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

	// Run the pipeline
	done := make(chan error, 1)
	go func() {
		job := &TileJob{
			ID:     "1",
			Region: region,
			Status: "pending",
		}
		done <- service.ProcessJobWithOptions(ctx, job, &JobOptions{
			MaxZoom:              *maxZoom,
			MinZoom:              *minZoom,
			SkipUpload:           *skipUpload,
			NoCleanup:            *noCleanup,
			ExtractGeometry:      *extractGeometry,
			SkipGeometryInsertion: *skipGeometryInsertion,
		})
	}()

	// Wait for completion or signal
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
		// Extract region from directory name (e.g., "public/tiles/oregon" -> "oregon")
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

	// Extract region from directory path (e.g., "public/tiles/oregon" -> "oregon")
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
  serve                 Start the REST API server

Generate Command:
  Usage: tile-service generate [options] <region>

  Arguments:
    <region>              Region name (e.g., washington, maryland, japan, oregon)

  Options:
    -max-zoom int         Maximum zoom level (default 16)
    -min-zoom int         Minimum zoom level (default 5)
    -skip-upload          Skip R2 upload, keep tiles locally in ./public/tiles
    -no-cleanup           Don't cleanup temporary files after completion
    -extract-geometry     Extract road geometries into database (default true)
    -skip-geometry-insertion  Extract to file but don't insert to database

Upload Command:
  Usage: tile-service upload [options] <tiles_directory>

  Arguments:
    <tiles_directory>     Path to the tiles directory to upload (e.g., public/tiles/oregon)

  Options:
    -min-zoom int         Minimum zoom level to upload (-1 = all, default -1)
    -max-zoom int         Maximum zoom level to upload (-1 = all, default -1)

Extract Command:
  Usage: tile-service extract <tiles_directory>

  Arguments:
    <tiles_directory>     Path to the tiles directory (e.g., public/tiles/oregon)

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

  # Upload pre-generated tiles
  ./tile-service upload public/tiles/oregon

  # Upload only the most zoomed-out level
  ./tile-service upload -min-zoom 5 -max-zoom 5 public/tiles/oregon

  # Upload zoom levels 7-12
  ./tile-service upload -min-zoom 7 -max-zoom 12 public/tiles/oregon

  # Extract road geometries from existing tiles
  ./tile-service extract public/tiles/oregon

  # Generate tiles and extract geometries to file (don't insert yet)
  ./tile-service generate -skip-upload -skip-geometry-insertion florida

  # Later, insert the extracted geometries
  ./tile-service insert-geometries florida
  # or
  ./tile-service insert-geometries .extracted-roads-florida.json

  # Generate tiles without geometry extraction
  ./tile-service generate -extract-geometry=false washington

  # Start the REST API server
  ./tile-service serve

  # Start the REST API server on a custom port
  ./tile-service serve -port 3000

  # Debug mode
  ./tile-service -debug generate -max-zoom 8 washington
`
	fmt.Print(help)
}

