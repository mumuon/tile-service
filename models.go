package main

import "time"

// TileJob represents a tile generation job
type TileJob struct {
	ID                    string
	Region                string
	Status                string // "pending", "extracting", "generating", "uploading", "completed", "failed", "cancelled"
	MaxZoom               int
	MinZoom               int
	SkipUpload            bool
	SkipGeneration        bool
	NoCleanup             bool
	ExtractGeometry       bool
	SkipGeometryInsertion bool
	CurrentStep           *string
	RoadsExtracted        *int
	TilesGenerated        *int
	TotalSizeBytes        *int64
	UploadProgress        int
	UploadedBytes         int64
	ErrorMessage          *string
	ErrorLog              *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	StartedAt             *time.Time
	CompletedAt           *time.Time
}

// JobProgress represents progress update for a job
type JobProgress struct {
	RoadsExtracted int
	TilesGenerated int
	UploadProgress int
	UploadedBytes  int64
}

// GeoJSONFeature represents a single road feature in GeoJSON
type GeoJSONFeature struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Geometry   GeoJSONGeometry        `json:"geometry"`
}

// GeoJSONGeometry represents a geometry object
type GeoJSONGeometry struct {
	Type        string        `json:"type"`
	Coordinates []interface{} `json:"coordinates"`
}

// KMLPlacemark represents a placemark in KML
type KMLPlacemark struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	LineString  struct {
		Coordinates string `xml:"coordinates"`
	} `xml:"LineString"`
	Polygon struct {
		OuterBoundaryIs struct {
			LinearRing struct {
				Coordinates string `xml:"coordinates"`
			} `xml:"LinearRing"`
		} `xml:"outerBoundaryIs"`
	} `xml:"Polygon"`
}

// KMLDocument represents a KML document
type KMLDocument struct {
	Placemarks []KMLPlacemark `xml:"Document>Folder>Placemark"`
}

// ProcessingResult represents the result of a processing step
type ProcessingResult struct {
	Success bool
	Error   error
	Message string
	Data    interface{}
}

// JobOptions represents optional configuration for tile generation
type JobOptions struct {
	MaxZoom               int
	MinZoom               int
	SkipUpload            bool
	SkipGeneration        bool // Skip tile generation, only upload existing tiles
	NoCleanup             bool
	ExtractGeometry       bool // Extract road geometries into database for nearby roads feature
	SkipGeometryInsertion bool // Extract to file but don't insert into database
}
