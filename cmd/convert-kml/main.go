package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	converter "github.com/mumuon/drivefinder/tile-service"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: convert-kml <kml-file> <output-geojson>")
		fmt.Println("Example: convert-kml input.kml output.geojson")
		os.Exit(1)
	}

	kmlPath := os.Args[1]
	outputPath := os.Args[2]

	// Extract region from filename
	region := strings.TrimSuffix(filepath.Base(kmlPath), filepath.Ext(kmlPath))

	ctx := context.Background()
	geojsonPath, count, err := converter.ConvertKMLToGeoJSON(ctx, kmlPath, region)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Move the generated file to the desired output location
	if geojsonPath != outputPath {
		data, err := os.ReadFile(geojsonPath)
		if err != nil {
			fmt.Printf("Error reading generated file: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			fmt.Printf("Error writing output file: %v\n", err)
			os.Exit(1)
		}
		os.Remove(geojsonPath)
	}

	fmt.Printf("✅ Converted to GeoJSON: %d features\n", count)
	fmt.Printf("   Output: %s\n", outputPath)
}
