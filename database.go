package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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
		SELECT id, region, status, current_step, roads_extracted, tiles_generated,
		       total_size_bytes, upload_progress, uploaded_bytes, error_message, error_log,
		       created_at, updated_at, started_at, completed_at
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
		SET status = $1, updated_at = NOW(), started_at = CASE WHEN started_at IS NULL THEN NOW() ELSE started_at END
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
		SET roads_extracted = $1, tiles_generated = $2, updated_at = NOW()
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
		SET status = 'failed', error_message = $1, updated_at = NOW()
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
			roads_extracted = $1,
			tiles_generated = $2,
			total_size_bytes = $3,
			upload_progress = 100,
			uploaded_bytes = $3,
			completed_at = NOW(),
			updated_at = NOW()
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
		SELECT id, region, status, current_step, roads_extracted, tiles_generated,
		       total_size_bytes, upload_progress, uploaded_bytes, error_message, error_log,
		       created_at, updated_at, started_at, completed_at
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
