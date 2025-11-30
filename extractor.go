package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ExtractKMZ extracts KMZ file to find doc.kml
func ExtractKMZ(ctx context.Context, region string) (string, error) {
	return ExtractKMZFromDir(ctx, region, "./curvature-data")
}

// ExtractKMZFromDir extracts KMZ file from a specific directory to find doc.kml
func ExtractKMZFromDir(ctx context.Context, region, curvatureDataDir string) (string, error) {
	logger := slog.With("region", region, "data_dir", curvatureDataDir)
	logger.Debug("extracting KMZ")

	// Build KMZ file path - try multiple naming patterns
	// First, try: us-{region}.c_1000.curves.kmz (for US states)
	// Second, try: {region}.c_1000.curves.kmz (for other regions like asia-japan, canada-ontario)
	regionLower := strings.ToLower(region)

	var kmzPath string
	potentialNames := []string{
		fmt.Sprintf("us-%s.c_1000.curves.kmz", regionLower),
		fmt.Sprintf("%s.c_1000.curves.kmz", regionLower),
	}

	for _, name := range potentialNames {
		path := filepath.Join(curvatureDataDir, name)
		if _, err := os.Stat(path); err == nil {
			kmzPath = path
			break
		}
	}

	if kmzPath == "" {
		return "", fmt.Errorf("KMZ file not found for region '%s' in %s", region, curvatureDataDir)
	}

	// Create temporary extraction directory
	extractDir := filepath.Join(os.TempDir(), fmt.Sprintf("kmz-extract-%s-%d", region, os.Getpid()))
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create extraction directory: %w", err)
	}

	logger.Debug("KMZ file found", "path", kmzPath, "extract_dir", extractDir)

	// Open KMZ file
	reader, err := zip.OpenReader(kmzPath)
	if err != nil {
		os.RemoveAll(extractDir)
		return "", fmt.Errorf("failed to open KMZ file: %w", err)
	}
	defer reader.Close()

	// Extract all files
	for _, file := range reader.File {
		if err := extractZipFile(file, extractDir); err != nil {
			os.RemoveAll(extractDir)
			return "", fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}
	}

	// Find doc.kml file
	kmlPath, err := findKMLFile(extractDir)
	if err != nil {
		os.RemoveAll(extractDir)
		return "", fmt.Errorf("failed to find doc.kml in extracted archive: %w", err)
	}

	logger.Debug("KML file found", "path", kmlPath)
	return kmlPath, nil
}

// extractZipFile extracts a single file from ZIP
func extractZipFile(file *zip.File, destDir string) error {
	filePath := filepath.Join(destDir, file.Name)

	if file.FileInfo().IsDir() {
		return os.MkdirAll(filePath, file.Mode())
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	srcFile, err := file.Open()
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// findKMLFile searches for doc.kml recursively
func findKMLFile(dir string) (string, error) {
	var kmlPath string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == "doc.kml" {
			kmlPath = path
			return filepath.SkipDir
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	if kmlPath == "" {
		return "", fmt.Errorf("doc.kml not found in extracted archive")
	}

	return kmlPath, nil
}

// CleanupTemporaryFiles removes temporary files created during processing
func CleanupTemporaryFiles(ctx context.Context, kmlPath, geoJSONPath, tilesDir string) error {
	logger := slog.With(
		"kml_path", kmlPath,
		"geojson_path", geoJSONPath,
		"tiles_dir", tilesDir,
	)

	// Remove KML parent directory (the entire extraction directory)
	if kmlPath != "" {
		extractDir := filepath.Dir(kmlPath)
		if err := os.RemoveAll(extractDir); err != nil {
			logger.Warn("failed to cleanup KML extraction directory", "error", err)
		}
	}

	// Remove GeoJSON file
	if geoJSONPath != "" {
		if err := os.Remove(geoJSONPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to cleanup GeoJSON file", "error", err)
		}
	}

	// Remove tiles directory (optional, can be kept for debugging)
	// if tilesDir != "" {
	// 	if err := os.RemoveAll(tilesDir); err != nil {
	// 		logger.Warn("failed to cleanup tiles directory", "error", err)
	// 	}
	// }

	return nil
}
