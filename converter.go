package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ConvertKMLToGeoJSON converts a KML file to GeoJSON format
func ConvertKMLToGeoJSON(ctx context.Context, kmlPath, region string) (string, int, error) {
	logger := slog.With("kml_path", kmlPath, "region", region)
	logger.Info("converting KML to GeoJSON")

	// Read KML file
	kmlContent, err := os.ReadFile(kmlPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read KML file: %w", err)
	}

	// Parse KML - structure is: kml > Document > Folder > Placemark > LineString
	// Each Folder represents one road with multiple Placemarks (road segments)
	// Note: KML files use the namespace "http://www.opengis.net/kml/2.2" - must be specified on ALL elements
	var doc struct {
		XMLName  xml.Name `xml:"http://www.opengis.net/kml/2.2 kml"`
		Document struct {
			Folders []struct {
				Name        string `xml:"http://www.opengis.net/kml/2.2 name"`
				Description string `xml:"http://www.opengis.net/kml/2.2 description"`
				Placemarks  []struct {
					Name       string `xml:"http://www.opengis.net/kml/2.2 name"`
					LineString struct {
						Coordinates string `xml:"http://www.opengis.net/kml/2.2 coordinates"`
					} `xml:"http://www.opengis.net/kml/2.2 LineString"`
				} `xml:"http://www.opengis.net/kml/2.2 Placemark"`
			} `xml:"http://www.opengis.net/kml/2.2 Folder"`
		} `xml:"http://www.opengis.net/kml/2.2 Document"`
	}

	if err := xml.Unmarshal(kmlContent, &doc); err != nil {
		return "", 0, fmt.Errorf("failed to parse KML: %w", err)
	}

	logger.Debug("KML parsed", "folders", len(doc.Document.Folders))

	// Build GeoJSON FeatureCollection
	features := make([]map[string]interface{}, 0)
	roadCount := 0

	// Process each Folder (each folder = one road with multiple segments)
	for _, folder := range doc.Document.Folders {
		// Get folder name with fallback
		folderName := folder.Name
		if folderName == "" {
			folderName = fmt.Sprintf("Road_%d", roadCount)
			roadCount++
		}

		// Collect all LineStrings from all Placemarks in this folder
		var lineStrings [][][]float64

		for _, pm := range folder.Placemarks {
			// Extract LineString coordinates
			if pm.LineString.Coordinates == "" {
				continue
			}

			coords := parseKMLCoordinates(pm.LineString.Coordinates)
			if len(coords) < 2 {
				continue
			}

			lineStrings = append(lineStrings, coords)
		}

		// Skip if no valid geometries found
		if len(lineStrings) == 0 {
			continue
		}

		// Create ONE feature per road (folder) with MultiLineString geometry
		var geometry map[string]interface{}
		if len(lineStrings) == 1 {
			// Single segment - use LineString
			geometry = map[string]interface{}{
				"type":        "LineString",
				"coordinates": lineStrings[0],
			}
		} else {
			// Multiple segments - use MultiLineString
			geometry = map[string]interface{}{
				"type":        "MultiLineString",
				"coordinates": lineStrings,
			}
		}

		feature := map[string]interface{}{
			"type": "Feature",
			"properties": map[string]interface{}{
				"Name": folderName, // Road name from folder
				// Description omitted to reduce file size
			},
			"geometry": geometry,
		}
		features = append(features, feature)
	}

	logger.Info("features extracted from KML", "count", len(features))

	// Create GeoJSON FeatureCollection
	featureCollection := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": features,
	}

	// Write to file
	geoJSONPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_roads.geojson", region))
	geoJSONBytes, err := json.Marshal(featureCollection)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal GeoJSON: %w", err)
	}

	if err := os.WriteFile(geoJSONPath, geoJSONBytes, 0644); err != nil {
		return "", 0, fmt.Errorf("failed to write GeoJSON file: %w", err)
	}

	logger.Info("GeoJSON created", "path", geoJSONPath, "size_bytes", len(geoJSONBytes))

	return geoJSONPath, len(features), nil
}

// parseKMLCoordinates parses KML coordinate string into [[lng, lat], ...] format
// KML format: "lng,lat,elev lng,lat,elev ..." (space-separated, comma-separated inner)
func parseKMLCoordinates(coordString string) [][]float64 {
	var coordinates [][]float64

	// Split by space to get individual coordinate triples
	parts := strings.Fields(strings.TrimSpace(coordString))

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Split by comma to get lng,lat[,elev]
		values := strings.Split(part, ",")
		if len(values) < 2 {
			continue
		}

		lng, err1 := strconv.ParseFloat(values[0], 64)
		lat, err2 := strconv.ParseFloat(values[1], 64)

		if err1 != nil || err2 != nil {
			continue
		}

		// GeoJSON format is [lng, lat]
		coordinates = append(coordinates, []float64{lng, lat})
	}

	return coordinates}
