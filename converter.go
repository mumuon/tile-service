package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
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

		// Calculate road metrics
		roadLength := calculateRoadLength(geometry)
		startLat, startLng, endLat, endLng, hasPoints := extractStartEndPoints(geometry)
		curvature := parseCurvature(folder.Description)

		// Generate deterministic UUID based on region + start coordinates
		// This ensures the same road gets the same UUID across processing runs
		var roadUUID string
		if hasPoints {
			// Create deterministic UUID v5 using region + start coords
			namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // DNS namespace
			name := fmt.Sprintf("%s:%.6f,%.6f", region, startLat, startLng)
			roadUUID = uuid.NewSHA1(namespace, []byte(name)).String()
		} else {
			// Fallback to random UUID if no coordinates available
			roadUUID = uuid.New().String()
		}

		feature := map[string]interface{}{
			"type": "Feature",
			"properties": map[string]interface{}{
				"id":     roadUUID,
				"Name":   folderName,
				"length": roadLength,
			},
			"geometry": geometry,
		}

		// Add optional properties
		if curvature != nil {
			feature["properties"].(map[string]interface{})["curvature"] = *curvature
		}
		if hasPoints {
			props := feature["properties"].(map[string]interface{})
			props["startLat"] = startLat
			props["startLng"] = startLng
			props["endLat"] = endLat
			props["endLng"] = endLng
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

	return coordinates
}

// haversineDistance calculates distance between two points in meters
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0 // meters

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// calculateLineStringLength calculates length of a LineString using Haversine
func calculateLineStringLength(coords [][]float64) float64 {
	if len(coords) < 2 {
		return 0.0
	}

	totalLength := 0.0
	for i := 0; i < len(coords)-1; i++ {
		lng1, lat1 := coords[i][0], coords[i][1]
		lng2, lat2 := coords[i+1][0], coords[i+1][1]
		totalLength += haversineDistance(lat1, lng1, lat2, lng2)
	}
	return totalLength
}

// calculateRoadLength calculates total length of road in meters
func calculateRoadLength(geometry map[string]interface{}) float64 {
	geomType, ok := geometry["type"].(string)
	if !ok {
		return 0.0
	}

	if geomType == "LineString" {
		coords, ok := geometry["coordinates"].([][]float64)
		if !ok {
			return 0.0
		}
		return calculateLineStringLength(coords)
	} else if geomType == "MultiLineString" {
		coordsArray, ok := geometry["coordinates"].([][][]float64)
		if !ok {
			return 0.0
		}
		totalLength := 0.0
		for _, coords := range coordsArray {
			totalLength += calculateLineStringLength(coords)
		}
		return totalLength
	}
	return 0.0
}

// extractStartEndPoints gets start and end coordinates from geometry
func extractStartEndPoints(geometry map[string]interface{}) (startLat, startLng, endLat, endLng float64, ok bool) {
	geomType, typeOk := geometry["type"].(string)
	if !typeOk {
		return 0, 0, 0, 0, false
	}

	if geomType == "LineString" {
		coords, coordsOk := geometry["coordinates"].([][]float64)
		if !coordsOk || len(coords) < 2 {
			return 0, 0, 0, 0, false
		}
		startLng, startLat = coords[0][0], coords[0][1]
		endLng, endLat = coords[len(coords)-1][0], coords[len(coords)-1][1]
		return startLat, startLng, endLat, endLng, true
	} else if geomType == "MultiLineString" {
		coordsArray, coordsOk := geometry["coordinates"].([][][]float64)
		if !coordsOk || len(coordsArray) == 0 {
			return 0, 0, 0, 0, false
		}

		firstSegment := coordsArray[0]
		lastSegment := coordsArray[len(coordsArray)-1]

		if len(firstSegment) == 0 || len(lastSegment) == 0 {
			return 0, 0, 0, 0, false
		}

		startLng, startLat = firstSegment[0][0], firstSegment[0][1]
		endLng, endLat = lastSegment[len(lastSegment)-1][0], lastSegment[len(lastSegment)-1][1]
		return startLat, startLng, endLat, endLng, true
	}

	return 0, 0, 0, 0, false
}

// parseCurvature extracts curvature value from KML description
// Looks for patterns like "c_1000" or "curvature: 1000"
func parseCurvature(description string) *string {
	if description == "" {
		return nil
	}

	// Look for c_XXX pattern (e.g., "c_1000")
	re := regexp.MustCompile(`c_(\d+)`)
	if matches := re.FindStringSubmatch(description); len(matches) > 1 {
		return &matches[1]
	}

	// Look for "curvature: XXX" pattern
	re2 := regexp.MustCompile(`curvature:\s*(\d+)`)
	if matches := re2.FindStringSubmatch(description); len(matches) > 1 {
		return &matches[1]
	}

	return nil
}
