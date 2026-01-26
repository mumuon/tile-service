package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createFakeTileWithSize creates a fake .pbf tile file at z/x/y.pbf with the given size
func createFakeTileWithSize(t *testing.T, baseDir string, z, x, y int, size int) {
	t.Helper()
	dir := filepath.Join(baseDir, fmt.Sprintf("%d/%d", z, x))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.pbf", y))
	data := make([]byte, size)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyTileDirectory_AllZoomsPresent(t *testing.T) {
	dir := t.TempDir()

	// Create tiles at zoom levels 5-8
	for z := 5; z <= 8; z++ {
		createFakeTileWithSize(t,dir, z, 10, 20, 100)
	}

	report, err := VerifyTileDirectory(dir, 5, 8)
	if err != nil {
		t.Fatal(err)
	}

	if !report.OK {
		t.Errorf("expected OK=true, got false, missing zooms: %v", report.MissingZooms)
	}
	if len(report.MissingZooms) != 0 {
		t.Errorf("expected no missing zooms, got %v", report.MissingZooms)
	}
	if len(report.ZoomStats) != 4 {
		t.Errorf("expected 4 zoom stats, got %d", len(report.ZoomStats))
	}
}

func TestVerifyTileDirectory_MissingMiddleZoom(t *testing.T) {
	dir := t.TempDir()

	// Create tiles at zoom 5, 7, 8 — missing zoom 6
	createFakeTileWithSize(t,dir, 5, 10, 20, 100)
	createFakeTileWithSize(t,dir, 7, 40, 80, 100)
	createFakeTileWithSize(t,dir, 8, 80, 160, 100)

	report, err := VerifyTileDirectory(dir, 5, 8)
	if err != nil {
		t.Fatal(err)
	}

	if report.OK {
		t.Error("expected OK=false for missing zoom 6")
	}
	if len(report.MissingZooms) != 1 || report.MissingZooms[0] != 6 {
		t.Errorf("expected missing zoom [6], got %v", report.MissingZooms)
	}
}

func TestVerifyTileDirectory_MissingHighZooms(t *testing.T) {
	dir := t.TempDir()

	// Only zoom 5 and 6 present, expecting 5-10
	createFakeTileWithSize(t,dir, 5, 10, 20, 100)
	createFakeTileWithSize(t,dir, 6, 20, 40, 100)

	report, err := VerifyTileDirectory(dir, 5, 10)
	if err != nil {
		t.Fatal(err)
	}

	if report.OK {
		t.Error("expected OK=false for missing high zooms")
	}
	expected := []int{7, 8, 9, 10}
	if len(report.MissingZooms) != len(expected) {
		t.Errorf("expected %d missing zooms, got %d: %v", len(expected), len(report.MissingZooms), report.MissingZooms)
	}
	for i, z := range expected {
		if i < len(report.MissingZooms) && report.MissingZooms[i] != z {
			t.Errorf("expected missing zoom %d at index %d, got %d", z, i, report.MissingZooms[i])
		}
	}
}

func TestVerifyTileDirectory_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	report, err := VerifyTileDirectory(dir, 5, 8)
	if err != nil {
		t.Fatal(err)
	}

	if report.OK {
		t.Error("expected OK=false for empty directory")
	}
	if len(report.MissingZooms) != 4 {
		t.Errorf("expected 4 missing zooms, got %d", len(report.MissingZooms))
	}
}

func TestVerifyTileDirectory_PerZoomStats(t *testing.T) {
	dir := t.TempDir()

	// Create multiple tiles at zoom 8
	createFakeTileWithSize(t,dir, 8, 10, 20, 200)
	createFakeTileWithSize(t,dir, 8, 12, 25, 300)
	createFakeTileWithSize(t,dir, 8, 11, 22, 150)

	report, err := VerifyTileDirectory(dir, 8, 8)
	if err != nil {
		t.Fatal(err)
	}

	if !report.OK {
		t.Error("expected OK=true")
	}

	stats := report.ZoomStats[8]
	if stats == nil {
		t.Fatal("expected stats for zoom 8")
	}
	if stats.TileCount != 3 {
		t.Errorf("expected 3 tiles, got %d", stats.TileCount)
	}
	if stats.TotalSize != 650 {
		t.Errorf("expected total size 650, got %d", stats.TotalSize)
	}
	if stats.MinX != 10 || stats.MaxX != 12 {
		t.Errorf("expected x range 10-12, got %d-%d", stats.MinX, stats.MaxX)
	}
	if stats.MinY != 20 || stats.MaxY != 25 {
		t.Errorf("expected y range 20-25, got %d-%d", stats.MinY, stats.MaxY)
	}
}

func TestVerifyMergeIntegrity_AllPresent(t *testing.T) {
	regionDir := t.TempDir()
	mergedDir := t.TempDir()

	// Create same tiles in both dirs
	createFakeTileWithSize(t,regionDir, 8, 10, 20, 100)
	createFakeTileWithSize(t,regionDir, 9, 20, 40, 200)
	createFakeTileWithSize(t,mergedDir, 8, 10, 20, 150) // merged is bigger (has more data)
	createFakeTileWithSize(t,mergedDir, 9, 20, 40, 250)

	report, err := VerifyMergeIntegrity(regionDir, mergedDir)
	if err != nil {
		t.Fatal(err)
	}

	if !report.OK {
		t.Errorf("expected OK=true, got false, missing: %v", report.MissingTiles)
	}
	if len(report.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", report.Warnings)
	}
}

func TestVerifyMergeIntegrity_MissingTile(t *testing.T) {
	regionDir := t.TempDir()
	mergedDir := t.TempDir()

	createFakeTileWithSize(t,regionDir, 8, 10, 20, 100)
	createFakeTileWithSize(t,regionDir, 9, 20, 40, 200)
	// Only one tile in merged
	createFakeTileWithSize(t,mergedDir, 8, 10, 20, 100)

	report, err := VerifyMergeIntegrity(regionDir, mergedDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.OK {
		t.Error("expected OK=false for missing tile")
	}
	if len(report.MissingTiles) != 1 {
		t.Errorf("expected 1 missing tile, got %d", len(report.MissingTiles))
	}
	if len(report.MissingTiles) > 0 {
		tc := report.MissingTiles[0]
		if tc.Z != 9 || tc.X != 20 || tc.Y != 40 {
			t.Errorf("expected missing tile 9/20/40, got %d/%d/%d", tc.Z, tc.X, tc.Y)
		}
	}
}

func TestVerifyMergeIntegrity_SmallerMergedTile(t *testing.T) {
	regionDir := t.TempDir()
	mergedDir := t.TempDir()

	createFakeTileWithSize(t,regionDir, 8, 10, 20, 500)
	createFakeTileWithSize(t,mergedDir, 8, 10, 20, 100) // smaller than regional

	report, err := VerifyMergeIntegrity(regionDir, mergedDir)
	if err != nil {
		t.Fatal(err)
	}

	if !report.OK {
		t.Error("expected OK=true (tile exists, just smaller)")
	}
	if len(report.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(report.Warnings))
	}
}
