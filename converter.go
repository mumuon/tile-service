package main

import (
	"context"

	"github.com/mumuon/drivefinder/tile-service/kmlconv"
)

// ConvertKMLToGeoJSON converts a KML file to GeoJSON format
func ConvertKMLToGeoJSON(ctx context.Context, kmlPath, region string) (string, int, error) {
	return kmlconv.ConvertKMLToGeoJSON(ctx, kmlPath, region)
}

// parseKMLCoordinates parses KML coordinate string into [[lng, lat], ...] format
func parseKMLCoordinates(coordString string) [][]float64 {
	return kmlconv.ParseKMLCoordinates(coordString)
}

// haversineDistance calculates distance between two points in meters
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	return kmlconv.HaversineDistance(lat1, lng1, lat2, lng2)
}

// calculateLineStringLength calculates length of a LineString using Haversine
func calculateLineStringLength(coords [][]float64) float64 {
	return kmlconv.CalculateLineStringLength(coords)
}

// calculateRoadLength calculates total length of road in meters
func calculateRoadLength(geometry map[string]interface{}) float64 {
	return kmlconv.CalculateRoadLength(geometry)
}

// extractStartEndPoints gets start and end coordinates from geometry
func extractStartEndPoints(geometry map[string]interface{}) (startLat, startLng, endLat, endLng float64, ok bool) {
	return kmlconv.ExtractStartEndPoints(geometry)
}

// parseCurvature extracts curvature value from KML description
func parseCurvature(description string) *string {
	return kmlconv.ParseCurvature(description)
}
