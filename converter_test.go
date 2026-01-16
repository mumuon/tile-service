package main

import (
	"math"
	"testing"
)

func TestHaversineDistance(t *testing.T) {
	testCases := []struct {
		name           string
		lat1, lng1     float64
		lat2, lng2     float64
		expectedMeters float64
		tolerance      float64
	}{
		{
			name:           "Seattle to Portland (~233 km)",
			lat1:           47.6062,
			lng1:           -122.3321,
			lat2:           45.5152,
			lng2:           -122.6784,
			expectedMeters: 233000,
			tolerance:      5000,
		},
		{
			name:           "Zero distance",
			lat1:           45.0,
			lng1:           -122.0,
			lat2:           45.0,
			lng2:           -122.0,
			expectedMeters: 0,
			tolerance:      1,
		},
		{
			name:           "1 degree latitude (~111 km)",
			lat1:           45.0,
			lng1:           -122.0,
			lat2:           46.0,
			lng2:           -122.0,
			expectedMeters: 111000,
			tolerance:      2000,
		},
		{
			name:           "Short distance (100m)",
			lat1:           45.0,
			lng1:           -122.0,
			lat2:           45.001,
			lng2:           -122.0,
			expectedMeters: 111,
			tolerance:      10,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			distance := haversineDistance(tc.lat1, tc.lng1, tc.lat2, tc.lng2)
			diff := math.Abs(distance - tc.expectedMeters)

			if diff > tc.tolerance {
				t.Errorf("Distance mismatch: got %.0fm, expected %.0fm (±%.0fm), diff=%.0fm",
					distance, tc.expectedMeters, tc.tolerance, diff)
			}
		})
	}
}

func TestCalculateLineStringLength(t *testing.T) {
	testCases := []struct {
		name     string
		coords   [][]float64
		expected float64
		minLen   float64
		maxLen   float64
	}{
		{
			name: "Simple straight line (2 degrees latitude)",
			coords: [][]float64{
				{-122.0, 45.0},
				{-122.0, 46.0},
				{-122.0, 47.0},
			},
			minLen: 220000, // ~222km
			maxLen: 224000,
		},
		{
			name: "Empty coordinates",
			coords: [][]float64{},
			expected: 0,
		},
		{
			name: "Single point",
			coords: [][]float64{
				{-122.0, 45.0},
			},
			expected: 0,
		},
		{
			name: "Two identical points",
			coords: [][]float64{
				{-122.0, 45.0},
				{-122.0, 45.0},
			},
			expected: 0,
			minLen:   0,
			maxLen:   1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			length := calculateLineStringLength(tc.coords)

			if tc.minLen > 0 || tc.maxLen > 0 {
				if length < tc.minLen || length > tc.maxLen {
					t.Errorf("Length out of range: got %.0fm, expected between %.0fm and %.0fm",
						length, tc.minLen, tc.maxLen)
				}
			} else {
				if math.Abs(length-tc.expected) > 1 {
					t.Errorf("Length mismatch: got %.0fm, expected %.0fm",
						length, tc.expected)
				}
			}
		})
	}
}

func TestCalculateRoadLength(t *testing.T) {
	testCases := []struct {
		name     string
		geometry map[string]interface{}
		minLen   float64
		maxLen   float64
	}{
		{
			name: "LineString",
			geometry: map[string]interface{}{
				"type": "LineString",
				"coordinates": [][]float64{
					{-122.0, 45.0},
					{-122.0, 46.0},
				},
			},
			minLen: 110000,
			maxLen: 112000,
		},
		{
			name: "MultiLineString",
			geometry: map[string]interface{}{
				"type": "MultiLineString",
				"coordinates": [][][]float64{
					{{-122.0, 45.0}, {-122.0, 45.5}},
					{{-122.0, 45.5}, {-122.0, 46.0}},
				},
			},
			minLen: 110000,
			maxLen: 112000,
		},
		{
			name: "Invalid geometry type",
			geometry: map[string]interface{}{
				"type": "Point",
			},
			minLen: 0,
			maxLen: 0,
		},
		{
			name:   "Missing type",
			geometry: map[string]interface{}{},
			minLen: 0,
			maxLen: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			length := calculateRoadLength(tc.geometry)

			if length < tc.minLen || length > tc.maxLen {
				t.Errorf("Length out of range: got %.0fm, expected between %.0fm and %.0fm",
					length, tc.minLen, tc.maxLen)
			}
		})
	}
}

func TestExtractStartEndPoints(t *testing.T) {
	testCases := []struct {
		name             string
		geometry         map[string]interface{}
		expectedStartLat float64
		expectedStartLng float64
		expectedEndLat   float64
		expectedEndLng   float64
		expectedOK       bool
	}{
		{
			name: "LineString",
			geometry: map[string]interface{}{
				"type": "LineString",
				"coordinates": [][]float64{
					{-122.0, 45.0},
					{-122.5, 45.5},
					{-123.0, 46.0},
				},
			},
			expectedStartLat: 45.0,
			expectedStartLng: -122.0,
			expectedEndLat:   46.0,
			expectedEndLng:   -123.0,
			expectedOK:       true,
		},
		{
			name: "MultiLineString",
			geometry: map[string]interface{}{
				"type": "MultiLineString",
				"coordinates": [][][]float64{
					{{-122.0, 45.0}, {-122.5, 45.5}},
					{{-122.5, 45.5}, {-123.0, 46.0}},
				},
			},
			expectedStartLat: 45.0,
			expectedStartLng: -122.0,
			expectedEndLat:   46.0,
			expectedEndLng:   -123.0,
			expectedOK:       true,
		},
		{
			name: "LineString with insufficient points",
			geometry: map[string]interface{}{
				"type": "LineString",
				"coordinates": [][]float64{
					{-122.0, 45.0},
				},
			},
			expectedOK: false,
		},
		{
			name: "Invalid geometry type",
			geometry: map[string]interface{}{
				"type": "Point",
			},
			expectedOK: false,
		},
		{
			name:       "Missing type",
			geometry:   map[string]interface{}{},
			expectedOK: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			startLat, startLng, endLat, endLng, ok := extractStartEndPoints(tc.geometry)

			if ok != tc.expectedOK {
				t.Errorf("OK mismatch: got %v, expected %v", ok, tc.expectedOK)
			}

			if tc.expectedOK {
				if startLat != tc.expectedStartLat || startLng != tc.expectedStartLng {
					t.Errorf("Start point mismatch: got (%.1f, %.1f), expected (%.1f, %.1f)",
						startLat, startLng, tc.expectedStartLat, tc.expectedStartLng)
				}

				if endLat != tc.expectedEndLat || endLng != tc.expectedEndLng {
					t.Errorf("End point mismatch: got (%.1f, %.1f), expected (%.1f, %.1f)",
						endLat, endLng, tc.expectedEndLat, tc.expectedEndLng)
				}
			}
		})
	}
}

func TestParseCurvature(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		expected    *string
	}{
		{
			name:        "c_1000 pattern",
			description: "This road has c_1000 curvature",
			expected:    stringPtr("1000"),
		},
		{
			name:        "curvature: pattern",
			description: "curvature: 500",
			expected:    stringPtr("500"),
		},
		{
			name:        "curvature with spaces",
			description: "curvature:   750",
			expected:    stringPtr("750"),
		},
		{
			name:        "No curvature",
			description: "Just a regular road",
			expected:    nil,
		},
		{
			name:        "Empty description",
			description: "",
			expected:    nil,
		},
		{
			name:        "c_ pattern takes precedence",
			description: "c_2000 and curvature: 1000",
			expected:    stringPtr("2000"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseCurvature(tc.description)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", *result)
				}
			} else {
				if result == nil {
					t.Errorf("Expected %s, got nil", *tc.expected)
				} else if *result != *tc.expected {
					t.Errorf("Curvature mismatch: got %s, expected %s", *result, *tc.expected)
				}
			}
		})
	}
}
