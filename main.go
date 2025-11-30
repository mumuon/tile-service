package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
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
			MaxZoom:    *maxZoom,
			MinZoom:    *minZoom,
			SkipUpload: *skipUpload,
			NoCleanup:  *noCleanup,
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

Generate Command:
  Usage: tile-service generate [options] <region>

  Arguments:
    <region>              Region name (e.g., washington, maryland, japan, oregon)

  Options:
    -max-zoom int         Maximum zoom level (default 16)
    -min-zoom int         Minimum zoom level (default 5)
    -skip-upload          Skip R2 upload, keep tiles locally in ./public/tiles
    -no-cleanup           Don't cleanup temporary files after completion

Upload Command:
  Usage: tile-service upload [options] <tiles_directory>

  Arguments:
    <tiles_directory>     Path to the tiles directory to upload (e.g., public/tiles/oregon)

  Options:
    -min-zoom int         Minimum zoom level to upload (-1 = all, default -1)
    -max-zoom int         Maximum zoom level to upload (-1 = all, default -1)

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

  # Debug mode
  ./tile-service -debug generate -max-zoom 8 washington
`
	fmt.Print(help)
}

