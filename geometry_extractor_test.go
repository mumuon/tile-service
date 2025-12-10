package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paulmach/orb/maptile"
)

// TestGeometryExtractorBasic tests basic functionality
func TestGeometryExtractorBasic(t *testing.T) {
	extractor := NewGeometryExtractor()

	if extractor == nil {
		t.Fatal("NewGeometryExtractor returned nil")
	}
}

// TestTilePathParsing tests tile coordinate extraction from file paths
func TestTilePathParsing(t *testing.T) {
	extractor := NewGeometryExtractor()

	testCases := []struct {
		name        string
		path        string
		expectError bool
		expectedZ   maptile.Zoom
		expectedX   uint32
		expectedY   uint32
	}{
		{
			name:        "Valid tile path",
			path:        "public/tiles/oregon/8/42/95.pbf",
			expectError: false,
			expectedZ:   maptile.Zoom(8),
			expectedX:   42,
			expectedY:   95,
		},
		{
			name:        "Valid tile path with deeper nesting",
			path:        "/some/deep/path/tiles/region/12/1024/2048.pbf",
			expectError: false,
			expectedZ:   maptile.Zoom(12),
			expectedX:   1024,
			expectedY:   2048,
		},
		{
			name:        "Invalid path - too short",
			path:        "5/10.pbf",
			expectError: true,
		},
		{
			name:        "Invalid path - non-numeric",
			path:        "public/tiles/foo/bar/baz.pbf",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tile, err := extractor.parseTilePath(tc.path)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tile.Z != tc.expectedZ {
				t.Errorf("Expected Z=%d, got %d", tc.expectedZ, tile.Z)
			}
			if tile.X != tc.expectedX {
				t.Errorf("Expected X=%d, got %d", tc.expectedX, tile.X)
			}
			if tile.Y != tc.expectedY {
				t.Errorf("Expected Y=%d, got %d", tc.expectedY, tile.Y)
			}
		})
	}
}

// TestProgressSaveLoad tests progress file persistence
func TestProgressSaveLoad(t *testing.T) {
	extractor := NewGeometryExtractor()
	region := "test_region"

	// Create progress
	progress := &ExtractionProgress{
		Region:         region,
		TotalTiles:     1000,
		ProcessedTiles: 500,
		ExtractedRoads: 250,
		StartedAt:      12345,
		Status:         "extracting",
	}

	// Save progress
	err := extractor.saveProgress(progress)
	if err != nil {
		t.Fatalf("Failed to save progress: %v", err)
	}

	// Load progress
	loaded := extractor.loadProgress(region)
	if loaded == nil {
		t.Fatal("Failed to load progress")
	}

	// Verify fields
	if loaded.Region != progress.Region {
		t.Errorf("Region mismatch: expected %s, got %s", progress.Region, loaded.Region)
	}
	if loaded.TotalTiles != progress.TotalTiles {
		t.Errorf("TotalTiles mismatch: expected %d, got %d", progress.TotalTiles, loaded.TotalTiles)
	}
	if loaded.ProcessedTiles != progress.ProcessedTiles {
		t.Errorf("ProcessedTiles mismatch: expected %d, got %d", progress.ProcessedTiles, loaded.ProcessedTiles)
	}
	if loaded.ExtractedRoads != progress.ExtractedRoads {
		t.Errorf("ExtractedRoads mismatch: expected %d, got %d", progress.ExtractedRoads, loaded.ExtractedRoads)
	}

	// Cleanup
	progressFile := extractor.getProgressFile(region)
	os.Remove(progressFile)
}

// TestRoadsSaveLoad tests road data persistence
func TestRoadsSaveLoad(t *testing.T) {
	extractor := NewGeometryExtractor()
	region := "test_region"

	// Create test roads - use proper key format: roadId_region
	roadsMap := map[string]*RoadGeometry{
		"road1_test_region": {
			RoadID:    "road1",
			Region:    region,
			MinLat:    45.0,
			MaxLat:    45.5,
			MinLng:    -123.0,
			MaxLng:    -122.5,
			Curvature: stringPtr("1000"),
		},
		"road2_test_region": {
			RoadID:    "road2",
			Region:    region,
			MinLat:    46.0,
			MaxLat:    46.5,
			MinLng:    -124.0,
			MaxLng:    -123.5,
			Curvature: nil,
		},
	}

	// Save roads
	filename := extractor.getExtractionFile(region)
	err := extractor.saveRoadsToFile(filename, roadsMap)
	if err != nil {
		t.Fatalf("Failed to save roads: %v", err)
	}

	// Load roads
	loaded, err := extractor.loadRoadsFromFile(filename)
	if err != nil {
		t.Fatalf("Failed to load roads: %v", err)
	}

	// Verify count
	if len(loaded) != len(roadsMap) {
		t.Errorf("Road count mismatch: expected %d, got %d", len(roadsMap), len(loaded))
	}

	// Verify roads exist - note: keys are different (original uses "_test" suffix)
	loadedMap := make(map[string]RoadGeometry)
	for _, road := range loaded {
		// Match the original key format: "{roadId}_{region}"
		key := road.RoadID + "_" + road.Region
		loadedMap[key] = road
	}

	// Debug: print what we have
	t.Logf("Original roads map keys: %v", getKeys(roadsMap))
	t.Logf("Loaded roads map keys: %v", getKeys2(loadedMap))

	for key, original := range roadsMap {
		found, exists := loadedMap[key]
		if !exists {
			t.Errorf("Road %s not found in loaded data", key)
			continue
		}

		if found.RoadID != original.RoadID {
			t.Errorf("RoadID mismatch for %s: expected %s, got %s", key, original.RoadID, found.RoadID)
		}
		if found.Region != original.Region {
			t.Errorf("Region mismatch for %s: expected %s, got %s", key, original.Region, found.Region)
		}
		if found.MinLat != original.MinLat {
			t.Errorf("MinLat mismatch for %s: expected %f, got %f", key, original.MinLat, found.MinLat)
		}
		if found.MaxLat != original.MaxLat {
			t.Errorf("MaxLat mismatch for %s: expected %f, got %f", key, original.MaxLat, found.MaxLat)
		}
	}

	// Cleanup
	os.Remove(filename)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func getKeys(m map[string]*RoadGeometry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getKeys2(m map[string]RoadGeometry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestFindPBFFiles tests file discovery
func TestFindPBFFiles(t *testing.T) {
	extractor := NewGeometryExtractor()

	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create some test files
	dirs := []string{
		filepath.Join(tmpDir, "5", "10"),
		filepath.Join(tmpDir, "6", "20"),
		filepath.Join(tmpDir, "7", "30"),
	}

	expectedFiles := []string{}
	for i, dir := range dirs {
		os.MkdirAll(dir, 0755)

		// Create .pbf file
		pbfFile := filepath.Join(dir, "15.pbf")
		os.WriteFile(pbfFile, []byte("test"), 0644)
		expectedFiles = append(expectedFiles, pbfFile)

		// Create non-.pbf file (should be ignored)
		otherFile := filepath.Join(dir, "metadata.json")
		os.WriteFile(otherFile, []byte("{}"), 0644)

		// Create another .pbf file
		if i == 0 {
			pbfFile2 := filepath.Join(dir, "16.pbf")
			os.WriteFile(pbfFile2, []byte("test2"), 0644)
			expectedFiles = append(expectedFiles, pbfFile2)
		}
	}

	// Find PBF files
	found, err := extractor.findPBFFiles(tmpDir)
	if err != nil {
		t.Fatalf("Failed to find PBF files: %v", err)
	}

	// Verify count
	if len(found) != len(expectedFiles) {
		t.Errorf("Expected %d files, found %d", len(expectedFiles), len(found))
	}

	// Verify all expected files were found
	foundMap := make(map[string]bool)
	for _, f := range found {
		foundMap[f] = true
	}

	for _, expected := range expectedFiles {
		if !foundMap[expected] {
			t.Errorf("Expected file not found: %s", expected)
		}
	}

	// Verify no non-.pbf files were found
	for _, f := range found {
		if filepath.Ext(f) != ".pbf" {
			t.Errorf("Non-.pbf file found: %s", f)
		}
	}
}

// TestCleanupExtractionFiles tests file cleanup
func TestCleanupExtractionFiles(t *testing.T) {
	extractor := NewGeometryExtractor()
	region := "test_cleanup_region"

	// Create test files
	progressFile := extractor.getProgressFile(region)
	extractionFile := extractor.getExtractionFile(region)

	os.WriteFile(progressFile, []byte("{}"), 0644)
	os.WriteFile(extractionFile, []byte("[]"), 0644)

	// Verify files exist
	if _, err := os.Stat(progressFile); os.IsNotExist(err) {
		t.Fatal("Progress file was not created")
	}
	if _, err := os.Stat(extractionFile); os.IsNotExist(err) {
		t.Fatal("Extraction file was not created")
	}

	// Cleanup
	err := extractor.CleanupExtractionFiles(region)
	if err != nil {
		t.Fatalf("Failed to cleanup: %v", err)
	}

	// Verify files are gone
	if _, err := os.Stat(progressFile); !os.IsNotExist(err) {
		t.Error("Progress file was not deleted")
	}
	if _, err := os.Stat(extractionFile); !os.IsNotExist(err) {
		t.Error("Extraction file was not deleted")
	}
}
