package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type GeoJSON struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

type Feature struct {
	Type       string                 `json:"type"`
	Geometry   Geometry               `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: compare-geojson <old-geojson> <new-geojson>")
		fmt.Println("Example: compare-geojson old/delaware.geojson new/delaware.geojson")
		os.Exit(1)
	}

	oldPath := os.Args[1]
	newPath := os.Args[2]

	oldGJ, err := loadGeoJSON(oldPath)
	if err != nil {
		fmt.Printf("Error loading old GeoJSON: %v\n", err)
		os.Exit(1)
	}

	newGJ, err := loadGeoJSON(newPath)
	if err != nil {
		fmt.Printf("Error loading new GeoJSON: %v\n", err)
		os.Exit(1)
	}

	compare(oldGJ, newGJ, oldPath, newPath)
}

func loadGeoJSON(path string) (*GeoJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var gj GeoJSON
	if err := json.Unmarshal(data, &gj); err != nil {
		return nil, err
	}

	return &gj, nil
}

func compare(old, new *GeoJSON, oldPath, newPath string) {
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("GeoJSON Comparison")
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Printf("OLD: %s\n", oldPath)
	fmt.Printf("NEW: %s\n", newPath)
	fmt.Println()

	// Feature counts
	fmt.Println("📊 Feature Counts:")
	fmt.Printf("  OLD features: %d\n", len(old.Features))
	fmt.Printf("  NEW features: %d\n", len(new.Features))
	diff := len(new.Features) - len(old.Features)
	if diff > 0 {
		fmt.Printf("  Difference:   +%d (NEW has more)\n", diff)
	} else if diff < 0 {
		fmt.Printf("  Difference:   %d (NEW has fewer) ⚠️\n", diff)
	} else {
		fmt.Printf("  Difference:   0 (equal)\n")
	}
	fmt.Println()

	// Coordinate counts
	oldCoordCount := countTotalCoordinates(old)
	newCoordCount := countTotalCoordinates(new)
	fmt.Println("📍 Coordinate Point Counts:")
	fmt.Printf("  OLD total coordinates: %d\n", oldCoordCount)
	fmt.Printf("  NEW total coordinates: %d\n", newCoordCount)
	coordDiff := newCoordCount - oldCoordCount
	if coordDiff > 0 {
		fmt.Printf("  Difference:            +%d\n", coordDiff)
	} else if coordDiff < 0 {
		fmt.Printf("  Difference:            %d ⚠️ DATA LOSS!\n", coordDiff)
	} else {
		fmt.Printf("  Difference:            0 ✓\n")
	}
	fmt.Println()

	// Geometry type distribution
	fmt.Println("🗺️  Geometry Types:")
	oldTypes := countGeometryTypes(old)
	newTypes := countGeometryTypes(new)

	fmt.Println("  OLD:")
	for gtype, count := range oldTypes {
		fmt.Printf("    %s: %d\n", gtype, count)
	}
	fmt.Println("  NEW:")
	for gtype, count := range newTypes {
		fmt.Printf("    %s: %d\n", gtype, count)
	}
	fmt.Println()

	// Road name analysis
	fmt.Println("🛣️  Road Name Analysis:")
	oldNames := extractRoadNames(old)
	newNames := extractRoadNames(new)

	fmt.Printf("  OLD unique road names: %d\n", len(oldNames))
	fmt.Printf("  NEW unique road names: %d\n", len(newNames))

	// Find missing roads
	missing := findMissing(oldNames, newNames)
	extra := findMissing(newNames, oldNames)

	if len(missing) > 0 {
		fmt.Printf("  ⚠️  Roads in OLD but not NEW: %d\n", len(missing))
		if len(missing) <= 10 {
			for _, name := range missing {
				fmt.Printf("    - %s\n", name)
			}
		} else {
			for i := 0; i < 10; i++ {
				fmt.Printf("    - %s\n", missing[i])
			}
			fmt.Printf("    ... and %d more\n", len(missing)-10)
		}
	} else {
		fmt.Println("  ✓ All OLD roads found in NEW")
	}

	if len(extra) > 0 {
		fmt.Printf("  Roads in NEW but not OLD: %d\n", len(extra))
	}
	fmt.Println()

	// Property completeness
	fmt.Println("📋 Property Analysis:")
	oldProps := analyzeProperties(old)
	newProps := analyzeProperties(new)

	fmt.Println("  OLD properties found:")
	for prop, count := range oldProps {
		fmt.Printf("    %s: %d features (%.1f%%)\n", prop, count, float64(count)/float64(len(old.Features))*100)
	}
	fmt.Println("  NEW properties found:")
	for prop, count := range newProps {
		fmt.Printf("    %s: %d features (%.1f%%)\n", prop, count, float64(count)/float64(len(new.Features))*100)
	}
	fmt.Println()

	// Sample features
	fmt.Println("🔍 Sample Features (first 5 from each):")
	fmt.Println("  OLD:")
	for i := 0; i < min(5, len(old.Features)); i++ {
		name := getPropertyString(old.Features[i].Properties, "Name")
		gtype := old.Features[i].Geometry.Type
		coords := countFeatureCoordinates(&old.Features[i])
		fmt.Printf("    %d. %s (%s, %d coords)\n", i+1, name, gtype, coords)
	}
	fmt.Println("  NEW:")
	for i := 0; i < min(5, len(new.Features)); i++ {
		name := getPropertyString(new.Features[i].Properties, "Name")
		gtype := new.Features[i].Geometry.Type
		coords := countFeatureCoordinates(&new.Features[i])
		fmt.Printf("    %d. %s (%s, %d coords)\n", i+1, name, gtype, coords)
	}
	fmt.Println()

	// Assessment
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("Assessment:")
	if coordDiff < 0 {
		fmt.Println("  ⚠️  CRITICAL: Coordinate data loss detected!")
		fmt.Printf("     %d coordinates missing from NEW GeoJSON\n", -coordDiff)
	} else if coordDiff == 0 {
		fmt.Println("  ✓ No coordinate data loss - all points preserved")
	}

	if diff < 0 {
		ratio := float64(len(old.Features)) / float64(len(new.Features))
		fmt.Printf("  ℹ️  Feature count difference (%.1fx) is likely due to:\n", ratio)
		fmt.Println("     OLD: 1 feature per Placemark (segment-level)")
		fmt.Println("     NEW: 1 feature per Folder (road-level, merged)")
		if coordDiff == 0 {
			fmt.Println("     This is OK if coordinates are preserved (which they are!)")
		}
	}

	if len(missing) > 0 {
		fmt.Printf("  ⚠️  WARNING: %d road names missing from NEW\n", len(missing))
		fmt.Println("     Investigate KML parsing logic")
	}

	fmt.Println("=" + strings.Repeat("=", 70))
}

func countTotalCoordinates(gj *GeoJSON) int {
	total := 0
	for i := range gj.Features {
		total += countFeatureCoordinates(&gj.Features[i])
	}
	return total
}

func countFeatureCoordinates(f *Feature) int {
	var coords interface{}
	if err := json.Unmarshal(f.Geometry.Coordinates, &coords); err != nil {
		return 0
	}
	return countCoords(coords)
}

func countCoords(coords interface{}) int {
	switch v := coords.(type) {
	case []interface{}:
		if len(v) == 0 {
			return 0
		}
		// Check if this is a coordinate pair [lng, lat] or array of coordinates
		if _, ok := v[0].(float64); ok {
			return 1 // This is a single coordinate
		}
		// Array of coordinates or nested arrays
		total := 0
		for _, item := range v {
			total += countCoords(item)
		}
		return total
	default:
		return 0
	}
}

func countGeometryTypes(gj *GeoJSON) map[string]int {
	types := make(map[string]int)
	for _, f := range gj.Features {
		types[f.Geometry.Type]++
	}
	return types
}

func extractRoadNames(gj *GeoJSON) []string {
	names := make(map[string]bool)
	for _, f := range gj.Features {
		if name := getPropertyString(f.Properties, "Name"); name != "" {
			names[name] = true
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func findMissing(set1, set2 []string) []string {
	set2Map := make(map[string]bool)
	for _, name := range set2 {
		set2Map[name] = true
	}

	missing := []string{}
	for _, name := range set1 {
		if !set2Map[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func analyzeProperties(gj *GeoJSON) map[string]int {
	props := make(map[string]int)
	for _, f := range gj.Features {
		for key := range f.Properties {
			props[key]++
		}
	}
	return props
}

func getPropertyString(props map[string]interface{}, key string) string {
	if val, ok := props[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
