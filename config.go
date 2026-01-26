package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents the service configuration
type Config struct {
	Database DatabaseConfig
	S3       S3Config
	Paths    PathsConfig
	Service  ServiceConfig
}

// DatabaseConfig represents database connection settings
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// S3Config represents S3/R2 connection settings
type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	Bucket          string
	BucketPath      string // e.g., "tiles"
}

// PathsConfig represents file system paths
type PathsConfig struct {
	CurvatureData string // Where KMZ files are located
	TempDir       string // Temporary working directory
	OutputDir     string // Where generated tiles are stored
}

// ServiceConfig represents service-level settings
type ServiceConfig struct {
	Workers     int
	PollInterval int // seconds
}

// LoadConfig loads configuration from environment variables and .env file
func LoadConfig(envPath string) (*Config, error) {
	// Prefer .env.local over .env (like Next.js)
	// This allows local development configuration to override production config
	localEnvPath := strings.TrimSuffix(envPath, ".env") + ".env.local"
	if _, err := os.Stat(localEnvPath); err == nil {
		if err := loadEnvFile(localEnvPath); err != nil {
			return nil, fmt.Errorf("failed to load local env file: %w", err)
		}
	} else if _, err := os.Stat(envPath); err == nil {
		// Fall back to regular .env file if .env.local doesn't exist
		if err := loadEnvFile(envPath); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Default output to ~/data/df/tiles (not ./public/tiles)
	defaultOutputDir := "./public/tiles"
	if home, err := os.UserHomeDir(); err == nil {
		defaultOutputDir = filepath.Join(home, "data", "df", "tiles")
	}

	cfg := &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "drivefinder"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		S3: S3Config{
			Endpoint:        getEnv("S3_ENDPOINT", "https://s3.us-west-1.wasabisys.com"),
			AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
			Region:          getEnv("S3_REGION", "us-west-1"),
			Bucket:          getEnv("S3_BUCKET", "drivefinder-tiles"),
			BucketPath:      getEnv("S3_BUCKET_PATH", "tiles"),
		},
		Paths: PathsConfig{
			CurvatureData: getEnv("CURVATURE_DATA_DIR", "./curvature-data"),
			TempDir:       getEnv("TEMP_DIR", "/tmp"),
			OutputDir:     getEnv("OUTPUT_DIR", defaultOutputDir),
		},
		Service: ServiceConfig{
			Workers:     getEnvInt("WORKERS", 3),
			PollInterval: getEnvInt("POLL_INTERVAL_SECONDS", 10),
		},
	}

	// Validate required config
	if cfg.Database.Password == "" {
		return nil, fmt.Errorf("DB_PASSWORD environment variable is required")
	}
	// Note: S3 credentials are optional - only needed if you plan to upload tiles to R2
	// For local development with --skip-upload, S3 credentials are not required

	return cfg, nil
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Simple env file parsing - split by newlines and set env vars
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split by = and set environment variable
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			os.Setenv(key, value)
		}
	}

	return nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}

// getEnvInt gets an environment variable as integer with a default value
func getEnvInt(key string, defaultVal int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}
