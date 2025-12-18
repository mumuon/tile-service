package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type KML struct {
	XMLName xml.Name `xml:"kml"`
	Document Document `xml:"Document"`
}

type Document struct {
	Folders []Folder `xml:"Folder"`
}

type Folder struct {
	Name        string      `xml:"name"`
	Description string      `xml:"description"`
	Placemarks  []Placemark `xml:"Placemark"`
}

type Placemark struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	LineString  LineString `xml:"LineString"`
}

type LineString struct {
	Coordinates string `xml:"coordinates"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: analyze-kml <path-to-kmz-or-kml>")
		fmt.Println("Example: analyze-kml ~/data/df/curvature-data/delaware.kmz")
		os.Exit(1)
	}

	filePath := os.Args[1]

	var kmlData []byte
	var err error

	// Check if KMZ or KML
	if strings.HasSuffix(strings.ToLower(filePath), ".kmz") {
		kmlData, err = extractKMLFromKMZ(filePath)
		if err != nil {
			fmt.Printf("Error extracting KML from KMZ: %v\n", err)
			os.Exit(1)
		}
	} else {
		kmlData, err = os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading KML file: %v\n", err)
			os.Exit(1)
		}
	}

	// Parse KML
	var kml KML
	if err := xml.Unmarshal(kmlData, &kml); err != nil {
		fmt.Printf("Error parsing KML: %v\n", err)
		os.Exit(1)
	}

	analyze(kml, filepath.Base(filePath))
}

func extractKMLFromKMZ(kmzPath string) ([]byte, error) {
	r, err := zip.OpenReader(kmzPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		// Look for any .kml file (handles both "doc.kml" and "folder/doc.kml")
		if strings.HasSuffix(strings.ToLower(f.Name), ".kml") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}

	return nil, fmt.Errorf("no .kml file found in KMZ archive")
}

func analyze(kml KML, filename string) {
	totalFolders := len(kml.Document.Folders)
	totalPlacemarks := 0
	totalCoordinates := 0
	segmentCounts := make(map[int]int) // segments per road distribution
	sampleRoads := []string{}

	for i, folder := range kml.Document.Folders {
		numPlacemarks := len(folder.Placemarks)
		totalPlacemarks += numPlacemarks
		segmentCounts[numPlacemarks]++

		// Collect sample road names
		if i < 10 {
			sampleRoads = append(sampleRoads, folder.Name)
		}

		// Count coordinate points
		for _, placemark := range folder.Placemarks {
			coords := strings.TrimSpace(placemark.LineString.Coordinates)
			if coords != "" {
				// Each coordinate is "lng,lat,alt" or "lng,lat"
				points := strings.Split(coords, " ")
				totalCoordinates += len(points)
			}
		}
	}

	avgSegments := float64(0)
	if totalFolders > 0 {
		avgSegments = float64(totalPlacemarks) / float64(totalFolders)
	}

	avgCoordsPerPlacemark := float64(0)
	if totalPlacemarks > 0 {
		avgCoordsPerPlacemark = float64(totalCoordinates) / float64(totalPlacemarks)
	}

	// Print analysis
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Printf("KML/KMZ Analysis: %s\n", filename)
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println()

	fmt.Println("📊 Ground Truth Counts:")
	fmt.Printf("  Folders (semantic roads):     %d\n", totalFolders)
	fmt.Printf("  Placemarks (road segments):   %d\n", totalPlacemarks)
	fmt.Printf("  Total coordinate points:      %d\n", totalCoordinates)
	fmt.Println()

	fmt.Println("📈 Ratios:")
	fmt.Printf("  Avg segments per road:        %.2f\n", avgSegments)
	fmt.Printf("  Avg coordinates per segment:  %.2f\n", avgCoordsPerPlacemark)
	fmt.Println()

	fmt.Println("🔢 Segment Distribution:")
	// Sort by segment count
	counts := make([]int, 0, len(segmentCounts))
	for count := range segmentCounts {
		counts = append(counts, count)
	}
	sort.Ints(counts)

	for _, count := range counts {
		numRoads := segmentCounts[count]
		bar := strings.Repeat("█", min(numRoads, 50))
		fmt.Printf("  %2d segment(s): %4d roads %s\n", count, numRoads, bar)
	}
	fmt.Println()

	fmt.Println("🛣️  Sample Road Names (first 10):")
	for i, name := range sampleRoads {
		fmt.Printf("  %2d. %s\n", i+1, name)
	}
	fmt.Println()

	fmt.Println("⚠️  Expected Behavior:")
	fmt.Println("  OLD Python script:")
	fmt.Printf("    - Extracts 1 feature per Placemark: ~%d GeoJSON features\n", totalPlacemarks)
	fmt.Printf("    - Database entries: ~%d (segment-level)\n", totalPlacemarks)
	fmt.Println("  NEW Go service:")
	fmt.Printf("    - Extracts 1 feature per Folder: ~%d GeoJSON features\n", totalFolders)
	fmt.Printf("    - Database entries: ~%d (road-level)\n", totalFolders)
	fmt.Println()

	if totalPlacemarks > 0 && totalFolders > 0 {
		ratio := float64(totalPlacemarks) / float64(totalFolders)
		fmt.Printf("  Expected count difference: %.1fx (%d vs %d)\n", ratio, totalPlacemarks, totalFolders)
	}

	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 70))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
