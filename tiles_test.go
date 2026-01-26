package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// createFakeTile creates a .pbf file at z/x/y.pbf within baseDir with some content
func createFakeTile(t *testing.T, baseDir string, z, x, y int) {
	t.Helper()
	dir := filepath.Join(baseDir, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%d.pbf", y)))
	if err != nil {
		t.Fatal(err)
	}
	// Write some bytes so size calculations work
	f.Write([]byte("fake-tile-data"))
	f.Close()
}

// --- GetTileCoords tests ---

func TestGetTileCoords_Basic(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)
	createFakeTile(t, dir, 7, 30, 40)
	createFakeTile(t, dir, 16, 100, 200)

	coords, err := GetTileCoords(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(coords) != 3 {
		t.Fatalf("expected 3 coords, got %d", len(coords))
	}

	expected := []TileCoord{
		{5, 10, 20},
		{7, 30, 40},
		{16, 100, 200},
	}
	for _, c := range expected {
		if !coords[c] {
			t.Errorf("expected coord %v to be present", c)
		}
	}
}

func TestGetTileCoords_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	coords, err := GetTileCoords(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(coords) != 0 {
		t.Fatalf("expected 0 coords, got %d", len(coords))
	}
}

func TestGetTileCoords_IgnoresNonPbfFiles(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)

	// Create a non-pbf file in the same structure
	nonPbf := filepath.Join(dir, "5", "10", "20.json")
	os.WriteFile(nonPbf, []byte("{}"), 0644)

	// Create a file at wrong depth
	os.MkdirAll(filepath.Join(dir, "extra"), 0755)
	os.WriteFile(filepath.Join(dir, "extra", "file.pbf"), []byte("x"), 0644)

	coords, err := GetTileCoords(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(coords) != 1 {
		t.Fatalf("expected 1 coord, got %d", len(coords))
	}
}

func TestGetTileCoords_MalformedPaths(t *testing.T) {
	dir := t.TempDir()

	// Non-numeric z directory
	os.MkdirAll(filepath.Join(dir, "abc", "10"), 0755)
	os.WriteFile(filepath.Join(dir, "abc", "10", "20.pbf"), []byte("x"), 0644)

	// Non-numeric x directory
	os.MkdirAll(filepath.Join(dir, "5", "abc"), 0755)
	os.WriteFile(filepath.Join(dir, "5", "abc", "20.pbf"), []byte("x"), 0644)

	// Non-numeric y file
	os.MkdirAll(filepath.Join(dir, "5", "10"), 0755)
	os.WriteFile(filepath.Join(dir, "5", "10", "abc.pbf"), []byte("x"), 0644)

	coords, err := GetTileCoords(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(coords) != 0 {
		t.Fatalf("expected 0 coords from malformed paths, got %d", len(coords))
	}
}

// --- FindRegionalTileDirs tests ---

func TestFindRegionalTileDirs_Basic(t *testing.T) {
	dir := t.TempDir()

	// Create valid region directories with numeric subdirs (zoom levels)
	for _, region := range []string{"washington", "oregon", "california"} {
		regionDir := filepath.Join(dir, region, "5")
		os.MkdirAll(regionDir, 0755)
	}

	dirs, err := FindRegionalTileDirs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 3 {
		t.Fatalf("expected 3 dirs, got %d: %v", len(dirs), dirs)
	}

	// Check all regions found
	names := make([]string, len(dirs))
	for i, d := range dirs {
		names[i] = filepath.Base(d)
	}
	sort.Strings(names)

	expected := []string{"california", "oregon", "washington"}
	for i, e := range expected {
		if names[i] != e {
			t.Errorf("expected %s at index %d, got %s", e, i, names[i])
		}
	}
}

func TestFindRegionalTileDirs_SkipsMerged(t *testing.T) {
	dir := t.TempDir()

	// "merged" directory should be skipped
	os.MkdirAll(filepath.Join(dir, "merged", "5"), 0755)
	os.MkdirAll(filepath.Join(dir, "washington", "5"), 0755)

	dirs, err := FindRegionalTileDirs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir (merged should be skipped), got %d: %v", len(dirs), dirs)
	}

	if filepath.Base(dirs[0]) != "washington" {
		t.Errorf("expected washington, got %s", filepath.Base(dirs[0]))
	}
}

func TestFindRegionalTileDirs_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, ".hidden", "5"), 0755)
	os.MkdirAll(filepath.Join(dir, "washington", "5"), 0755)

	dirs, err := FindRegionalTileDirs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir (hidden should be skipped), got %d", len(dirs))
	}
}

func TestFindRegionalTileDirs_SkipsNumericDirs(t *testing.T) {
	dir := t.TempDir()

	// Numeric directories (zoom levels at root) should be skipped
	os.MkdirAll(filepath.Join(dir, "14", "5"), 0755)
	os.MkdirAll(filepath.Join(dir, "washington", "5"), 0755)

	dirs, err := FindRegionalTileDirs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir (numeric should be skipped), got %d", len(dirs))
	}
}

func TestFindRegionalTileDirs_SkipsDirsWithoutZoomLevels(t *testing.T) {
	dir := t.TempDir()

	// Directory without numeric subdirs should be skipped
	os.MkdirAll(filepath.Join(dir, "notes"), 0755)
	os.WriteFile(filepath.Join(dir, "notes", "readme.txt"), []byte("hi"), 0644)

	os.MkdirAll(filepath.Join(dir, "washington", "5"), 0755)

	dirs, err := FindRegionalTileDirs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir (no-zoom-level dir should be skipped), got %d", len(dirs))
	}
}

// --- hasOverlappingTiles tests ---

func TestHasOverlappingTiles_WithOverlap(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)
	createFakeTile(t, dir, 7, 30, 40)

	targetCoords := map[TileCoord]bool{
		{5, 10, 20}: true, // This overlaps
		{8, 50, 60}: true,
	}

	overlap, err := hasOverlappingTiles(dir, targetCoords)
	if err != nil {
		t.Fatal(err)
	}

	if !overlap {
		t.Error("expected overlap to be true")
	}
}

func TestHasOverlappingTiles_NoOverlap(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)
	createFakeTile(t, dir, 7, 30, 40)

	targetCoords := map[TileCoord]bool{
		{8, 50, 60}: true,
		{9, 70, 80}: true,
	}

	overlap, err := hasOverlappingTiles(dir, targetCoords)
	if err != nil {
		t.Fatal(err)
	}

	if overlap {
		t.Error("expected overlap to be false")
	}
}

func TestHasOverlappingTiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	targetCoords := map[TileCoord]bool{
		{5, 10, 20}: true,
	}

	overlap, err := hasOverlappingTiles(dir, targetCoords)
	if err != nil {
		t.Fatal(err)
	}

	if overlap {
		t.Error("expected overlap to be false for empty dir")
	}
}

func TestHasOverlappingTiles_EmptyTargetCoords(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)

	targetCoords := map[TileCoord]bool{}

	overlap, err := hasOverlappingTiles(dir, targetCoords)
	if err != nil {
		t.Fatal(err)
	}

	if overlap {
		t.Error("expected overlap to be false with empty target coords")
	}
}

// --- FindOverlappingRegions tests ---

func TestFindOverlappingRegions_Basic(t *testing.T) {
	dir := t.TempDir()

	// Create washington tiles
	createFakeTile(t, filepath.Join(dir, "washington"), 5, 10, 20)
	createFakeTile(t, filepath.Join(dir, "washington"), 7, 30, 40)

	// Create oregon tiles - overlaps with washington at (5,10,20)
	createFakeTile(t, filepath.Join(dir, "oregon"), 5, 10, 20)
	createFakeTile(t, filepath.Join(dir, "oregon"), 7, 99, 99)

	// Create florida tiles - no overlap with washington
	createFakeTile(t, filepath.Join(dir, "florida"), 5, 80, 80)
	createFakeTile(t, filepath.Join(dir, "florida"), 7, 90, 90)

	overlapping, err := FindOverlappingRegions(dir, "washington")
	if err != nil {
		t.Fatal(err)
	}

	// Should include washington (always) and oregon (overlaps), but not florida
	if len(overlapping) != 2 {
		names := make([]string, len(overlapping))
		for i, d := range overlapping {
			names[i] = filepath.Base(d)
		}
		t.Fatalf("expected 2 overlapping regions, got %d: %v", len(overlapping), names)
	}

	names := make([]string, len(overlapping))
	for i, d := range overlapping {
		names[i] = filepath.Base(d)
	}
	sort.Strings(names)

	if names[0] != "oregon" || names[1] != "washington" {
		t.Errorf("expected [oregon, washington], got %v", names)
	}
}

func TestFindOverlappingRegions_AlwaysIncludesTarget(t *testing.T) {
	dir := t.TempDir()

	// Create only the target region
	createFakeTile(t, filepath.Join(dir, "washington"), 5, 10, 20)

	overlapping, err := FindOverlappingRegions(dir, "washington")
	if err != nil {
		t.Fatal(err)
	}

	if len(overlapping) != 1 {
		t.Fatalf("expected 1 region (target itself), got %d", len(overlapping))
	}

	if filepath.Base(overlapping[0]) != "washington" {
		t.Errorf("expected washington, got %s", filepath.Base(overlapping[0]))
	}
}

// --- countTiles tests ---

func TestCountTiles(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)
	createFakeTile(t, dir, 7, 30, 40)
	createFakeTile(t, dir, 16, 100, 200)

	// Add non-pbf file
	os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0644)

	count, err := countTiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	if count != 3 {
		t.Errorf("expected 3 tiles, got %d", count)
	}
}

func TestCountTiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	count, err := countTiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Errorf("expected 0 tiles, got %d", count)
	}
}

// --- getDirectorySize tests ---

func TestGetDirectorySize(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20) // "fake-tile-data" = 14 bytes each
	createFakeTile(t, dir, 7, 30, 40)

	size, err := getDirectorySize(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Each tile is 14 bytes ("fake-tile-data")
	if size != 28 {
		t.Errorf("expected 28 bytes, got %d", size)
	}
}

// --- removeDirectoryContents tests ---

func TestRemoveDirectoryContents(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)
	createFakeTile(t, dir, 7, 30, 40)
	os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0644)

	err := removeDirectoryContents(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Directory itself should still exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("root directory should still exist")
	}

	// Should be empty (or only .DS_Store)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.Name() != ".DS_Store" {
			t.Errorf("unexpected entry remaining: %s", e.Name())
		}
	}
}

func TestRemoveDirectoryContents_NonexistentDir(t *testing.T) {
	err := removeDirectoryContents("/tmp/nonexistent-dir-for-test-12345")
	if err != nil {
		t.Errorf("expected no error for nonexistent dir, got %v", err)
	}
}

// --- GetTileMetadata tests ---

func TestGetTileMetadata(t *testing.T) {
	dir := t.TempDir()
	createFakeTile(t, dir, 5, 10, 20)
	createFakeTile(t, dir, 7, 30, 40)

	meta, err := GetTileMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}

	if meta.TilesCount != 2 {
		t.Errorf("expected 2 tiles, got %d", meta.TilesCount)
	}

	if meta.TotalSize != 28 {
		t.Errorf("expected 28 bytes total, got %d", meta.TotalSize)
	}
}
