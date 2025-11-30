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

	// Parse KML - handle both Document>Folder>Placemark and Document>Folder>Folder>Placemark structures
	var doc struct {
		XMLName xml.Name
		Folders []struct {
			Name        string `xml:"name"`
			Folders     []struct {
				Name        string `xml:"name"`
				Placemarks  []struct {
					Name       string `xml:"name"`
					LineString struct {
						Coordinates string `xml:"coordinates"`
					} `xml:"LineString"`
					Polygon struct {
						OuterBoundaryIs struct {
							LinearRing struct {
								Coordinates string `xml:"coordinates"`
							} `xml:"LinearRing"`
						} `xml:"outerBoundaryIs"`
					} `xml:"Polygon"`
				} `xml:"Placemark"`
			} `xml:"Folder"`
			Placemarks []struct {
				Name       string `xml:"name"`
				LineString struct {
					Coordinates string `xml:"coordinates"`
				} `xml:"LineString"`
				Polygon struct {
					OuterBoundaryIs struct {
						LinearRing struct {
							Coordinates string `xml:"coordinates"`
						} `xml:"LinearRing"`
					} `xml:"outerBoundaryIs"`
				} `xml:"Polygon"`
			} `xml:"Placemark"`
		} `xml:"Document>Folder"`
	}

	if err := xml.Unmarshal(kmlContent, &doc); err != nil {
		return "", 0, fmt.Errorf("failed to parse KML: %w", err)
	}

	logger.Debug("KML parsed", "folders", len(doc.Folders))

	// Build GeoJSON FeatureCollection
	features := make([]map[string]interface{}, 0)

	for _, folder := range doc.Folders {
		// Process placemarks directly in the folder
		for _, pm := range folder.Placemarks {
			var coords [][]float64
			if pm.LineString.Coordinates != "" {
				coords = parseKMLCoordinates(pm.LineString.Coordinates)
			}
			if pm.Polygon.OuterBoundaryIs.LinearRing.Coordinates != "" {
				coords = parseKMLCoordinates(pm.Polygon.OuterBoundaryIs.LinearRing.Coordinates)
			}
			if len(coords) == 0 {
				continue
			}

			feature := map[string]interface{}{
				"type": "Feature",
				"properties": map[string]interface{}{
					"Name":   pm.Name,
					"Folder": folder.Name,
				},
				"geometry": map[string]interface{}{
					"type":        "LineString",
					"coordinates": coords,
				},
			}
			features = append(features, feature)
		}

		// Process placemarks in nested folders
		for _, subfolder := range folder.Folders {
			for _, pm := range subfolder.Placemarks {
				var coords [][]float64
				if pm.LineString.Coordinates != "" {
					coords = parseKMLCoordinates(pm.LineString.Coordinates)
				}
				if pm.Polygon.OuterBoundaryIs.LinearRing.Coordinates != "" {
					coords = parseKMLCoordinates(pm.Polygon.OuterBoundaryIs.LinearRing.Coordinates)
				}
				if len(coords) == 0 {
					continue
				}

				feature := map[string]interface{}{
					"type": "Feature",
					"properties": map[string]interface{}{
						"Name":   pm.Name,
						"Folder": subfolder.Name,
					},
					"geometry": map[string]interface{}{
						"type":        "LineString",
						"coordinates": coords,
					},
				}
				features = append(features, feature)
			}
		}
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

	return coordinates
}
