package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paulmach/orb"
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

// TestCalculateBoundsComprehensive tests the calculateBounds function thoroughly
func TestBoundAccessors(t *testing.T) {
	// Test to understand how orb.Bound accessors work
	minLng := -123.5
	minLat := 45.2
	maxLng := -123.1
	maxLat := 45.6

	bound := orb.Bound{
		Min: orb.Point{minLng, minLat},
		Max: orb.Point{maxLng, maxLat},
	}

	t.Logf("Created bound with:")
	t.Logf("  Min: orb.Point{%.6f, %.6f}", minLng, minLat)
	t.Logf("  Max: orb.Point{%.6f, %.6f}", maxLng, maxLat)
	t.Logf("")
	t.Logf("Accessing via methods:")
	t.Logf("  bound.Min.Lon() = %.6f (expected %.6f)", bound.Min.Lon(), minLng)
	t.Logf("  bound.Min.Lat() = %.6f (expected %.6f)", bound.Min.Lat(), minLat)
	t.Logf("  bound.Max.Lon() = %.6f (expected %.6f)", bound.Max.Lon(), maxLng)
	t.Logf("  bound.Max.Lat() = %.6f (expected %.6f)", bound.Max.Lat(), maxLat)
	t.Logf("")
	t.Logf("Accessing via array indices:")
	t.Logf("  bound.Min[0] = %.6f (should be Lon = %.6f)", bound.Min[0], minLng)
	t.Logf("  bound.Min[1] = %.6f (should be Lat = %.6f)", bound.Min[1], minLat)
	t.Logf("  bound.Max[0] = %.6f (should be Lon = %.6f)", bound.Max[0], maxLng)
	t.Logf("  bound.Max[1] = %.6f (should be Lat = %.6f)", bound.Max[1], maxLat)

	// Extract like the code does
	extractedMinLat := bound.Min.Lat()
	extractedMaxLat := bound.Max.Lat()
	extractedMinLng := bound.Min.Lon()
	extractedMaxLng := bound.Max.Lon()

	t.Logf("")
	t.Logf("Extracted values (as code does at line 248-251):")
	t.Logf("  minLat = %.6f", extractedMinLat)
	t.Logf("  maxLat = %.6f", extractedMaxLat)
	t.Logf("  minLng = %.6f", extractedMinLng)
	t.Logf("  maxLng = %.6f", extractedMaxLng)

	// Check if any are zero
	if extractedMinLat == 0 || extractedMaxLat == 0 || extractedMinLng == 0 || extractedMaxLng == 0 {
		t.Errorf("BUG: Some extracted values are zero!")
		t.Errorf("  minLat: %.6f", extractedMinLat)
		t.Errorf("  maxLat: %.6f", extractedMaxLat)
		t.Errorf("  minLng: %.6f", extractedMinLng)
		t.Errorf("  maxLng: %.6f", extractedMaxLng)
	}
}

func TestSingleTileExtraction(t *testing.T) {
	extractor := NewGeometryExtractor()

	// Test with a real tile file
	tilePath := "test-tiles/test-region/9/66/157.pbf"

	// Check if test tile exists
	if _, err := os.Stat(tilePath); os.IsNotExist(err) {
		t.Skip("Test tile not found at", tilePath)
	}

	// Parse tile coordinates from path
	tile, err := extractor.parseTilePath(tilePath)
	if err != nil {
		t.Fatalf("Failed to parse tile path: %v", err)
	}

	t.Logf("Testing extraction from tile: %d/%d/%d", tile.Z, tile.X, tile.Y)

	// Extract roads from this single tile
	roads, invalidCount, err := extractor.extractRoadsFromTile(tilePath, "test-region", tile)
	if err != nil {
		t.Fatalf("Failed to extract roads: %v", err)
	}

	t.Logf("Extracted %d roads, %d invalid", len(roads), invalidCount)

	// Show first few roads
	for i, road := range roads {
		if i >= 5 {
			break
		}
		t.Logf("Road %d:", i)
		t.Logf("  ID: %s", road.RoadID)
		t.Logf("  Bounds: [%.6f, %.6f] to [%.6f, %.6f]",
			road.MinLng, road.MinLat, road.MaxLng, road.MaxLat)
		if road.Curvature != nil {
			t.Logf("  Curvature: %s", *road.Curvature)
		}
	}

	// Check if any roads have zero coordinates
	zeroCount := 0
	for _, road := range roads {
		if road.MinLat == 0 || road.MaxLat == 0 || road.MinLng == 0 || road.MaxLng == 0 {
			zeroCount++
			if zeroCount <= 3 {
				t.Errorf("Road has zero coordinates: %s [%.6f, %.6f] to [%.6f, %.6f]",
					road.RoadID, road.MinLng, road.MinLat, road.MaxLng, road.MaxLat)
			}
		}
	}
	if zeroCount > 0 {
		t.Errorf("Found %d roads with zero coordinates!", zeroCount)
	}
}

// TestRoadSegmentBoundingBox - Test for debugging bounding box calculation for road segments
// This test helps identify if the bounding box issue is in calculation or storage
func TestRoadSegmentBoundingBox(t *testing.T) {
	extractor := NewGeometryExtractor()

	// Test case 1: Road segment in Oregon (typical use case)
	t.Run("OregonRoadSegment", func(t *testing.T) {
		// Tile coordinates for a typical Oregon road
		tile := maptile.New(163, 357, 10) // z=10, x=163, y=357

		// Get the geographic bounds of the tile itself
		tileBound := tile.Bound()
		t.Logf("Tile %d/%d/%d geographic bounds:", tile.Z, tile.X, tile.Y)
		t.Logf("  SW corner (Min): %.6f°, %.6f°", tileBound.Min.Lon(), tileBound.Min.Lat())
		t.Logf("  NE corner (Max): %.6f°, %.6f°", tileBound.Max.Lon(), tileBound.Max.Lat())
		t.Logf("  Tile lat span: %.6f° (%.2f km)", tileBound.Max.Lat()-tileBound.Min.Lat(), (tileBound.Max.Lat()-tileBound.Min.Lat())*111.0)

		// Simulate a road segment as LineString with tile coordinates (0-4096)
		// This represents a diagonal road across the tile
		roadGeometry := orb.LineString{
			{512.0, 512.0},   // Point in tile space
			{1024.0, 1024.0}, // Another point in tile space
			{2048.0, 2048.0}, // Third point
		}

		t.Logf("\nRoad geometry in tile space (0-4096):")
		for i, point := range roadGeometry {
			t.Logf("  Point %d: x=%.1f, y=%.1f", i, point[0], point[1])
		}

		// Calculate bounds
		bounds := extractor.calculateBounds(roadGeometry, tile)
		if bounds == nil {
			t.Fatal("calculateBounds returned nil")
		}

		// Extract coordinates as the code does
		minLat := bounds.Min.Lat()
		maxLat := bounds.Max.Lat()
		minLng := bounds.Min.Lon()
		maxLng := bounds.Max.Lon()

		t.Logf("\nCalculated bounding box:")
		t.Logf("  Min Lat: %.6f°", minLat)
		t.Logf("  Max Lat: %.6f°", maxLat)
		t.Logf("  Min Lng: %.6f°", minLng)
		t.Logf("  Max Lng: %.6f°", maxLng)
		t.Logf("\nBounding box dimensions:")
		t.Logf("  Lat range: %.6f° (%.2f km)", maxLat-minLat, (maxLat-minLat)*111.0)
		t.Logf("  Lng range: %.6f° (%.2f km)", maxLng-minLng, (maxLng-minLng)*111.0)

		// Check if road bounds are reasonable compared to tile bounds
		t.Logf("\nRoad bounds as percent of tile:")
		t.Logf("  Lat: %.1f%% of tile height", (maxLat-minLat)/(tileBound.Max.Lat()-tileBound.Min.Lat())*100)
		t.Logf("  Lng: %.1f%% of tile width", (maxLng-minLng)/(tileBound.Max.Lon()-tileBound.Min.Lon())*100)

		// Validation checks
		if minLat == 0 || maxLat == 0 || minLng == 0 || maxLng == 0 {
			t.Errorf("ERROR: Zero coordinate detected!")
			t.Errorf("  This indicates a bug in calculateBounds()")
			t.Errorf("  minLat=%.6f, maxLat=%.6f, minLng=%.6f, maxLng=%.6f",
				minLat, maxLat, minLng, maxLng)
		}

		// Check if bounds make sense
		if minLat >= maxLat {
			t.Errorf("ERROR: minLat (%.6f) >= maxLat (%.6f)", minLat, maxLat)
		}
		if minLng >= maxLng {
			t.Errorf("ERROR: minLng (%.6f) >= maxLng (%.6f)", minLng, maxLng)
		}

		// Check if bounds are within tile bounds
		if minLat < tileBound.Min.Lat() || maxLat > tileBound.Max.Lat() {
			t.Errorf("WARNING: Latitude bounds extend outside tile bounds")
			t.Errorf("  Road: [%.6f, %.6f]", minLat, maxLat)
			t.Errorf("  Tile: [%.6f, %.6f]", tileBound.Min.Lat(), tileBound.Max.Lat())
		}
		if minLng < tileBound.Min.Lon() || maxLng > tileBound.Max.Lon() {
			t.Errorf("WARNING: Longitude bounds extend outside tile bounds")
			t.Errorf("  Road: [%.6f, %.6f]", minLng, maxLng)
			t.Errorf("  Tile: [%.6f, %.6f]", tileBound.Min.Lon(), tileBound.Max.Lon())
		}

		// Validate coordinate ranges
		if minLat < -90 || minLat > 90 || maxLat < -90 || maxLat > 90 {
			t.Errorf("ERROR: Latitude out of range [-90, 90]")
		}
		if minLng < -180 || minLng > 180 || maxLng < -180 || maxLng > 180 {
			t.Errorf("ERROR: Longitude out of range [-180, 180]")
		}
	})

	// Test case 2: Very small road segment (single tile coordinate point)
	t.Run("SmallRoadSegment", func(t *testing.T) {
		tile := maptile.New(163, 357, 10)

		// Very small road - just 2 points close together
		roadGeometry := orb.LineString{
			{2048.0, 2048.0},
			{2049.0, 2049.0},
		}

		bounds := extractor.calculateBounds(roadGeometry, tile)
		if bounds == nil {
			t.Fatal("calculateBounds returned nil")
		}

		minLat := bounds.Min.Lat()
		maxLat := bounds.Max.Lat()
		minLng := bounds.Min.Lon()
		maxLng := bounds.Max.Lon()

		t.Logf("Small road segment bounds:")
		t.Logf("  [%.8f, %.8f] to [%.8f, %.8f]", minLng, minLat, maxLng, maxLat)

		if minLat == 0 || maxLat == 0 || minLng == 0 || maxLng == 0 {
			t.Errorf("ERROR: Zero coordinate in small segment")
		}

		// The bounds should be very small but not zero
		latRange := maxLat - minLat
		lngRange := maxLng - minLng

		t.Logf("  Lat range: %.8f° (%.2f meters)", latRange, latRange*111000)
		t.Logf("  Lng range: %.8f° (%.2f meters)", lngRange, lngRange*111000)

		if latRange == 0 || lngRange == 0 {
			t.Logf("  WARNING: Zero range detected (might be too small)")
		}
	})

	// Test case 3: Road at tile edges
	t.Run("RoadAtTileEdges", func(t *testing.T) {
		tile := maptile.New(163, 357, 10)

		testCases := []struct {
			name     string
			geometry orb.LineString
		}{
			{
				name: "Road at left edge",
				geometry: orb.LineString{
					{0.0, 2048.0},
					{100.0, 2048.0},
				},
			},
			{
				name: "Road at right edge",
				geometry: orb.LineString{
					{3996.0, 2048.0},
					{4096.0, 2048.0},
				},
			},
			{
				name: "Road at top edge",
				geometry: orb.LineString{
					{2048.0, 0.0},
					{2048.0, 100.0},
				},
			},
			{
				name: "Road at bottom edge",
				geometry: orb.LineString{
					{2048.0, 3996.0},
					{2048.0, 4096.0},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				bounds := extractor.calculateBounds(tc.geometry, tile)
				if bounds == nil {
					t.Fatal("calculateBounds returned nil")
				}

				minLat := bounds.Min.Lat()
				maxLat := bounds.Max.Lat()
				minLng := bounds.Min.Lon()
				maxLng := bounds.Max.Lon()

				t.Logf("%s bounds: [%.6f, %.6f] to [%.6f, %.6f]",
					tc.name, minLng, minLat, maxLng, maxLat)

				if minLat == 0 || maxLat == 0 || minLng == 0 || maxLng == 0 {
					t.Errorf("ERROR: Zero coordinate detected in %s", tc.name)
				}
			})
		}
	})
}

func TestCalculateBoundsComprehensive(t *testing.T) {
	extractor := NewGeometryExtractor()

	t.Run("TileCoordinateConversion", func(t *testing.T) {
		// Test various tiles to understand the coordinate conversion
		testCases := []struct {
			name string
			tile maptile.Tile
		}{
			{"Oregon tile", maptile.New(163, 357, 10)},
			{"Equator crossing", maptile.New(512, 512, 10)}, // Should cross equator
			{"Prime meridian crossing", maptile.New(512, 256, 10)}, // Should cross prime meridian
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tileBound := tc.tile.Bound()
				t.Logf("Tile %d/%d/%d bounds:", tc.tile.Z, tc.tile.X, tc.tile.Y)
				t.Logf("  Geographic bounds: [%.6f, %.6f] to [%.6f, %.6f]",
					tileBound.Min.Lon(), tileBound.Min.Lat(),
					tileBound.Max.Lon(), tileBound.Max.Lat())

				// Test with a simple LineString
				geometry := orb.LineString{
					{100.0, 100.0},
					{200.0, 200.0},
				}

				bounds := extractor.calculateBounds(geometry, tc.tile)
				if bounds == nil {
					t.Fatal("Expected non-nil bounds")
				}

				t.Logf("  Calculated bounds: [%.6f, %.6f] to [%.6f, %.6f]",
					bounds.Min.Lon(), bounds.Min.Lat(),
					bounds.Max.Lon(), bounds.Max.Lat())

				// Check if bounds are actually valid
				if bounds.Min.Lat() > bounds.Max.Lat() {
					t.Error("Invalid: Min.Lat > Max.Lat")
				}
				if bounds.Min.Lon() > bounds.Max.Lon() {
					t.Error("Invalid: Min.Lon > Max.Lon")
				}
			})
		}
	})

	t.Run("EdgeCases", func(t *testing.T) {
		tile := maptile.New(163, 357, 10)

		testCases := []struct {
			name     string
			geometry orb.Geometry
		}{
			{
				name: "Point at tile origin (0,0)",
				geometry: orb.Point{0.0, 0.0},
			},
			{
				name: "Point at tile max (4096,4096)",
				geometry: orb.Point{4096.0, 4096.0},
			},
			{
				name: "LineString spanning full tile",
				geometry: orb.LineString{
					{0.0, 0.0},
					{4096.0, 4096.0},
				},
			},
			{
				name: "Single point in middle",
				geometry: orb.Point{2048.0, 2048.0},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				bounds := extractor.calculateBounds(tc.geometry, tile)
				if bounds == nil {
					t.Fatal("Expected non-nil bounds")
				}

				t.Logf("Geometry: %s", tc.name)
				t.Logf("  Bounds: [%.6f, %.6f] to [%.6f, %.6f]",
					bounds.Min.Lon(), bounds.Min.Lat(),
					bounds.Max.Lon(), bounds.Max.Lat())

				// IMPORTANT: Zero IS a valid coordinate (equator/prime meridian)
				// So we should NOT reject bounds just because they equal zero!
				// Instead, check for truly invalid conditions:

				// 1. Min should be <= Max
				if bounds.Min.Lat() > bounds.Max.Lat() {
					t.Errorf("INVALID: Min.Lat (%.6f) > Max.Lat (%.6f)",
						bounds.Min.Lat(), bounds.Max.Lat())
				}
				if bounds.Min.Lon() > bounds.Max.Lon() {
					t.Errorf("INVALID: Min.Lon (%.6f) > Max.Lon (%.6f)",
						bounds.Min.Lon(), bounds.Max.Lon())
				}

				// 2. Latitude should be in valid range [-90, 90]
				if bounds.Min.Lat() < -90 || bounds.Min.Lat() > 90 {
					t.Errorf("INVALID: Min.Lat (%.6f) out of range [-90, 90]", bounds.Min.Lat())
				}
				if bounds.Max.Lat() < -90 || bounds.Max.Lat() > 90 {
					t.Errorf("INVALID: Max.Lat (%.6f) out of range [-90, 90]", bounds.Max.Lat())
				}

				// 3. Longitude should be in valid range [-180, 180]
				if bounds.Min.Lon() < -180 || bounds.Min.Lon() > 180 {
					t.Errorf("INVALID: Min.Lon (%.6f) out of range [-180, 180]", bounds.Min.Lon())
				}
				if bounds.Max.Lon() < -180 || bounds.Max.Lon() > 180 {
					t.Errorf("INVALID: Max.Lon (%.6f) out of range [-180, 180]", bounds.Max.Lon())
				}
			})
		}
	})

	t.Run("ZeroIsValidCoordinate", func(t *testing.T) {
		// Demonstrate that zero IS a valid coordinate
		// 0° latitude = equator, 0° longitude = prime meridian

		// Tile that should cross the equator
		equatorTile := maptile.New(512, 512, 10)
		tileBound := equatorTile.Bound()

		t.Logf("Equator-crossing tile bounds: [%.6f, %.6f] to [%.6f, %.6f]",
			tileBound.Min.Lon(), tileBound.Min.Lat(),
			tileBound.Max.Lon(), tileBound.Max.Lat())

		// If the tile crosses the equator, one of the latitude bounds might be near zero
		// This is VALID and should not be rejected!
		if tileBound.Min.Lat() < 0 && tileBound.Max.Lat() > 0 {
			t.Log("✓ This tile crosses the equator (lat bounds span zero)")
			t.Log("  Zero latitude is VALID - it's the equator!")
		}
	})
}
