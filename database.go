package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Database wraps database operations
type Database struct {
	conn *sql.DB
}

// NewDatabase creates a new database connection
func NewDatabase(cfg DatabaseConfig) (*Database, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	slog.Info("database connected successfully")

	return &Database{conn: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.conn.Close()
}

// GetPendingJobs retrieves pending jobs from the database
func (d *Database) GetPendingJobs(ctx context.Context, limit int) ([]*TileJob, error) {
	query := `
		SELECT id, region, status, "currentStep", "roadsExtracted", "tilesGenerated",
		       "totalSizeBytes", "uploadProgress", "uploadedBytes", "errorMessage", "errorLog",
		       "createdAt", "updatedAt", "startedAt", "completedAt"
		FROM "TileJob"
		WHERE status = 'pending'
		LIMIT $1
	`

	rows, err := d.conn.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*TileJob
	for rows.Next() {
		job := &TileJob{}
		err := rows.Scan(
			&job.ID, &job.Region, &job.Status, &job.CurrentStep,
			&job.RoadsExtracted, &job.TilesGenerated,
			&job.TotalSizeBytes, &job.UploadProgress, &job.UploadedBytes,
			&job.ErrorMessage, &job.ErrorLog,
			&job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.CompletedAt,
		)
		if err != nil {
			slog.Error("failed to scan job row", "error", err)
			continue
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return jobs, nil
}

// UpdateJobStatus updates the status of a job
func (d *Database) UpdateJobStatus(ctx context.Context, jobID, status string) error {
	query := `
		UPDATE "TileJob"
		SET status = $1, "updatedAt" = NOW(), "startedAt" = CASE WHEN "startedAt" IS NULL THEN NOW() ELSE "startedAt" END
		WHERE id = $2
	`

	result, err := d.conn.ExecContext(ctx, query, status, jobID)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}

	return nil
}

// UpdateJobProgress updates the progress of a job
func (d *Database) UpdateJobProgress(ctx context.Context, jobID string, roadsExtracted, tilesGenerated int) error {
	query := `
		UPDATE "TileJob"
		SET "roadsExtracted" = $1, "tilesGenerated" = $2, "updatedAt" = NOW()
		WHERE id = $3
	`

	_, err := d.conn.ExecContext(ctx, query, roadsExtracted, tilesGenerated, jobID)
	if err != nil {
		return fmt.Errorf("failed to update job progress: %w", err)
	}

	return nil
}

// UpdateJobError updates the error information for a failed job
func (d *Database) UpdateJobError(ctx context.Context, jobID, errorMsg string) error {
	query := `
		UPDATE "TileJob"
		SET status = 'failed', "errorMessage" = $1, "updatedAt" = NOW()
		WHERE id = $2
	`

	_, err := d.conn.ExecContext(ctx, query, errorMsg, jobID)
	if err != nil {
		return fmt.Errorf("failed to update job error: %w", err)
	}

	return nil
}

// CompleteJob marks a job as completed
func (d *Database) CompleteJob(ctx context.Context, jobID string, roadsExtracted, tilesGenerated int, totalSizeBytes int64) error {
	query := `
		UPDATE "TileJob"
		SET
			status = 'completed',
			"roadsExtracted" = $1,
			"tilesGenerated" = $2,
			"totalSizeBytes" = $3,
			"uploadProgress" = 100,
			"uploadedBytes" = $3,
			"completedAt" = NOW(),
			"updatedAt" = NOW()
		WHERE id = $4
	`

	result, err := d.conn.ExecContext(ctx, query, roadsExtracted, tilesGenerated, totalSizeBytes, jobID)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}

	return nil
}

// GetJobByID retrieves a specific job by ID
func (d *Database) GetJobByID(ctx context.Context, jobID string) (*TileJob, error) {
	query := `
		SELECT id, region, status, "currentStep", "roadsExtracted", "tilesGenerated",
		       "totalSizeBytes", "uploadProgress", "uploadedBytes", "errorMessage", "errorLog",
		       "createdAt", "updatedAt", "startedAt", "completedAt"
		FROM "TileJob"
		WHERE id = $1
	`

	job := &TileJob{}
	err := d.conn.QueryRowContext(ctx, query, jobID).Scan(
		&job.ID, &job.Region, &job.Status, &job.CurrentStep,
		&job.RoadsExtracted, &job.TilesGenerated,
		&job.TotalSizeBytes, &job.UploadProgress, &job.UploadedBytes,
		&job.ErrorMessage, &job.ErrorLog,
		&job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query job: %w", err)
	}

	return job, nil
}

// UpsertRoadGeometry inserts or updates a road geometry record
func (d *Database) UpsertRoadGeometry(ctx context.Context, road *RoadGeometry) error {
	query := `
		INSERT INTO "RoadGeometry" (id, "roadId", region, "minLat", "maxLat", "minLng", "maxLng", curvature, "createdAt", "updatedAt")
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT ("roadId", region)
		DO UPDATE SET
			"minLat" = EXCLUDED."minLat",
			"maxLat" = EXCLUDED."maxLat",
			"minLng" = EXCLUDED."minLng",
			"maxLng" = EXCLUDED."maxLng",
			curvature = EXCLUDED.curvature,
			"updatedAt" = NOW()
	`

	_, err := d.conn.ExecContext(ctx, query,
		road.RoadID, road.Region,
		road.MinLat, road.MaxLat, road.MinLng, road.MaxLng,
		road.Curvature,
	)

	if err != nil {
		return fmt.Errorf("failed to upsert road geometry: %w", err)
	}

	return nil
}

// BatchUpsertRoadGeometries inserts or updates multiple road geometry records in a batch
// Uses optimized chunked-transaction approach: fast + resilient to failures
func (d *Database) BatchUpsertRoadGeometries(ctx context.Context, roads []RoadGeometry, batchSize int) (int, error) {
	logger := slog.With("total_roads", len(roads), "batch_size", batchSize)
	logger.Info("starting optimized batch upsert of road geometries")

	// PostgreSQL parameter limit: 65535 parameters max per query
	// Each road needs 7 parameters, so max batch size = 65535 / 7 = 9362
	// Use 9000 for safety margin
	const maxBatchSize = 9000
	if batchSize < 5000 {
		batchSize = 5000
	}
	if batchSize > maxBatchSize {
		batchSize = maxBatchSize
	}

	// Commit every 500k rows (good balance of speed and resilience)
	const rowsPerTransaction = 500000

	inserted := 0
	var tx *sql.Tx
	var err error
	rowsInCurrentTx := 0

	for i := 0; i < len(roads); i += batchSize {
		// Start a new transaction if needed
		if tx == nil {
			tx, err = d.conn.BeginTx(ctx, nil)
			if err != nil {
				return inserted, fmt.Errorf("failed to begin transaction: %w", err)
			}
			rowsInCurrentTx = 0
		}

		end := i + batchSize
		if end > len(roads) {
			end = len(roads)
		}

		batch := roads[i:end]

		// Build multi-row INSERT statement
		valuesStrings := make([]string, 0, len(batch))
		valueArgs := make([]interface{}, 0, len(batch)*7)

		for idx, road := range batch {
			basePos := idx * 7
			valuesStrings = append(valuesStrings,
				fmt.Sprintf("(gen_random_uuid(), $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW(), NOW())",
					basePos+1, basePos+2, basePos+3, basePos+4, basePos+5, basePos+6, basePos+7))

			valueArgs = append(valueArgs,
				road.RoadID, road.Region,
				road.MinLat, road.MaxLat, road.MinLng, road.MaxLng,
				road.Curvature,
			)
		}

		query := fmt.Sprintf(`
			INSERT INTO "RoadGeometry" (id, "roadId", region, "minLat", "maxLat", "minLng", "maxLng", curvature, "createdAt", "updatedAt")
			VALUES %s
			ON CONFLICT ("roadId", region)
			DO UPDATE SET
				"minLat" = EXCLUDED."minLat",
				"maxLat" = EXCLUDED."maxLat",
				"minLng" = EXCLUDED."minLng",
				"maxLng" = EXCLUDED."maxLng",
				curvature = EXCLUDED.curvature,
				"updatedAt" = NOW()
		`, strings.Join(valuesStrings, ", "))

		// Execute within transaction
		_, err = tx.ExecContext(ctx, query, valueArgs...)
		if err != nil {
			tx.Rollback()
			return inserted, fmt.Errorf("failed to insert batch at row %d: %w", i, err)
		}

		inserted += len(batch)
		rowsInCurrentTx += len(batch)

		// Commit transaction every 500k rows to ensure progress is saved
		if rowsInCurrentTx >= rowsPerTransaction || inserted == len(roads) {
			if err := tx.Commit(); err != nil {
				return inserted - rowsInCurrentTx, fmt.Errorf("failed to commit transaction: %w", err)
			}
			logger.Info("transaction committed", "inserted", inserted, "total", len(roads))
			tx = nil // Will start new transaction on next iteration
		} else if inserted%50000 == 0 {
			// Log progress within transaction
			logger.Info("batch progress", "inserted", inserted, "total", len(roads), "uncommitted", rowsInCurrentTx)
		}
	}

	// Commit any remaining uncommitted transaction
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return inserted - rowsInCurrentTx, fmt.Errorf("failed to commit final transaction: %w", err)
		}
	}

	logger.Info("batch upsert complete", "total_inserted", inserted)
	return inserted, nil
}

// DeleteRoadGeometriesByRegion deletes all road geometries for a specific region
func (d *Database) DeleteRoadGeometriesByRegion(ctx context.Context, region string) (int64, error) {
	query := `DELETE FROM "RoadGeometry" WHERE region = $1`

	result, err := d.conn.ExecContext(ctx, query, region)
	if err != nil {
		return 0, fmt.Errorf("failed to delete road geometries: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rows, nil
}

// GetRoadGeometryCount returns the number of road geometries for a region
func (d *Database) GetRoadGeometryCount(ctx context.Context, region string) (int, error) {
	query := `SELECT COUNT(*) FROM "RoadGeometry" WHERE region = $1`

	var count int
	err := d.conn.QueryRowContext(ctx, query, region).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count road geometries: %w", err)
	}

	return count, nil
}
