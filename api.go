package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// APIServer handles HTTP requests for tile generation
type APIServer struct {
	db          *Database
	s3Client    *S3Client
	config      *Config
	jobQueue    chan *TileJob
	activeJobs  map[string]*JobStatus
	jobsMutex   sync.RWMutex
	subscribers map[string][]chan JobStatusUpdate
	subsMutex   sync.RWMutex
}

// JobStatus tracks the current status of a job
type JobStatus struct {
	Job       *TileJob
	Progress  *JobProgress
	Error     error
	UpdatedAt time.Time
}

// JobStatusUpdate represents a status update for streaming
type JobStatusUpdate struct {
	JobID      string    `json:"jobId"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Message    string    `json:"message"`
	Error      string    `json:"error,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// GenerateRequest represents a tile generation request
type GenerateRequest struct {
	Region                string `json:"region"`
	MaxZoom               int    `json:"maxZoom"`
	MinZoom               int    `json:"minZoom"`
	SkipUpload            bool   `json:"skipUpload"`
	SkipGeneration        bool   `json:"skipGeneration"`
	ExtractGeometry       bool   `json:"extractGeometry"`
	SkipGeometryInsertion bool   `json:"skipGeometryInsertion"`
}

// GenerateResponse represents the response to a generate request
type GenerateResponse struct {
	JobID   string `json:"jobId"`
	Message string `json:"message"`
}

// JobStatusResponse represents the response to a status request
type JobStatusResponse struct {
	JobID                 string  `json:"jobId"`
	Region                string  `json:"region"`
	Status                string  `json:"status"`
	CurrentStep           *string `json:"currentStep,omitempty"`
	RoadsExtracted        *int    `json:"roadsExtracted,omitempty"`
	TilesGenerated        *int    `json:"tilesGenerated,omitempty"`
	UploadProgress        int     `json:"uploadProgress"`
	ErrorMessage          *string `json:"errorMessage,omitempty"`
	UpdatedAt             string  `json:"updatedAt"`
	MaxZoom               int     `json:"maxZoom"`
	MinZoom               int     `json:"minZoom"`
	SkipUpload            bool    `json:"skipUpload"`
	SkipGeneration        bool    `json:"skipGeneration"`
	ExtractGeometry       bool    `json:"extractGeometry"`
	SkipGeometryInsertion bool    `json:"skipGeometryInsertion"`
}

// NewAPIServer creates a new API server
func NewAPIServer(db *Database, s3Client *S3Client, config *Config) *APIServer {
	return &APIServer{
		db:          db,
		s3Client:    s3Client,
		config:      config,
		jobQueue:    make(chan *TileJob, 100),
		activeJobs:  make(map[string]*JobStatus),
		subscribers: make(map[string][]chan JobStatusUpdate),
	}
}

// Start starts the API server
func (s *APIServer) Start(port int) error {
	// Start job processor
	go s.processJobs()

	// Setup routes
	http.HandleFunc("/api/generate", s.handleGenerate)
	http.HandleFunc("/api/jobs/", s.handleJobStatus)
	http.HandleFunc("/api/jobs", s.handleListJobs)
	http.HandleFunc("/api/stream/", s.handleJobStream)
	http.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf(":%d", port)
	slog.Info("starting API server", "port", port)
	return http.ListenAndServe(addr, nil)
}

// handleGenerate handles POST /api/generate
func (s *APIServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Region == "" {
		http.Error(w, "Region is required", http.StatusBadRequest)
		return
	}
	if req.MaxZoom == 0 {
		req.MaxZoom = 16
	}
	if req.MinZoom == 0 {
		req.MinZoom = 5
	}

	// Create job with options from request
	jobID := uuid.New().String()
	job := &TileJob{
		ID:                    jobID,
		Region:                req.Region,
		Status:                "pending",
		MaxZoom:               req.MaxZoom,
		MinZoom:               req.MinZoom,
		SkipUpload:            req.SkipUpload,
		SkipGeneration:        req.SkipGeneration,
		NoCleanup:             false,
		ExtractGeometry:       req.ExtractGeometry,
		SkipGeometryInsertion: req.SkipGeometryInsertion,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	// Store job in database if available
	if s.db != nil {
		if err := s.createJob(r.Context(), job); err != nil {
			slog.Error("failed to create job in database", "error", err)
			http.Error(w, "Failed to create job", http.StatusInternalServerError)
			return
		}
	}

	// Add job to queue
	s.jobsMutex.Lock()
	s.activeJobs[jobID] = &JobStatus{
		Job:       job,
		Progress:  &JobProgress{},
		UpdatedAt: time.Now(),
	}
	s.jobsMutex.Unlock()

	// Queue job for processing
	select {
	case s.jobQueue <- job:
		slog.Info("job queued", "job_id", jobID, "region", req.Region)
	default:
		http.Error(w, "Job queue is full", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GenerateResponse{
		JobID:   jobID,
		Message: "Job queued successfully",
	})
}

// handleJobStatus handles GET /api/jobs/{jobId}
func (s *APIServer) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := r.URL.Path[len("/api/jobs/"):]
	if jobID == "" {
		http.Error(w, "Job ID is required", http.StatusBadRequest)
		return
	}

	// Get job status
	s.jobsMutex.RLock()
	status, exists := s.activeJobs[jobID]
	s.jobsMutex.RUnlock()

	if !exists {
		// Try to get from database
		if s.db != nil {
			job, err := s.getJobFromDB(r.Context(), jobID)
			if err != nil {
				http.Error(w, "Job not found", http.StatusNotFound)
				return
			}
			status = &JobStatus{
				Job:       job,
				UpdatedAt: job.UpdatedAt,
			}
		} else {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}
	}

	// Build response
	resp := JobStatusResponse{
		JobID:                 status.Job.ID,
		Region:                status.Job.Region,
		Status:                status.Job.Status,
		CurrentStep:           status.Job.CurrentStep,
		RoadsExtracted:        status.Job.RoadsExtracted,
		TilesGenerated:        status.Job.TilesGenerated,
		UploadProgress:        status.Job.UploadProgress,
		ErrorMessage:          status.Job.ErrorMessage,
		UpdatedAt:             status.UpdatedAt.Format(time.RFC3339),
		MaxZoom:               status.Job.MaxZoom,
		MinZoom:               status.Job.MinZoom,
		SkipUpload:            status.Job.SkipUpload,
		SkipGeneration:        status.Job.SkipGeneration,
		ExtractGeometry:       status.Job.ExtractGeometry,
		SkipGeometryInsertion: status.Job.SkipGeometryInsertion,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleListJobs handles GET /api/jobs
func (s *APIServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	var jobs []JobStatusResponse
	for _, status := range s.activeJobs {
		jobs = append(jobs, JobStatusResponse{
			JobID:                 status.Job.ID,
			Region:                status.Job.Region,
			Status:                status.Job.Status,
			CurrentStep:           status.Job.CurrentStep,
			RoadsExtracted:        status.Job.RoadsExtracted,
			TilesGenerated:        status.Job.TilesGenerated,
			UploadProgress:        status.Job.UploadProgress,
			ErrorMessage:          status.Job.ErrorMessage,
			UpdatedAt:             status.UpdatedAt.Format(time.RFC3339),
			MaxZoom:               status.Job.MaxZoom,
			MinZoom:               status.Job.MinZoom,
			SkipUpload:            status.Job.SkipUpload,
			SkipGeneration:        status.Job.SkipGeneration,
			ExtractGeometry:       status.Job.ExtractGeometry,
			SkipGeometryInsertion: status.Job.SkipGeometryInsertion,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// handleJobStream handles GET /api/stream/{jobId} for Server-Sent Events
func (s *APIServer) handleJobStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID
	jobID := r.URL.Path[len("/api/stream/"):]
	if jobID == "" {
		http.Error(w, "Job ID is required", http.StatusBadRequest)
		return
	}

	// Check if job exists
	s.jobsMutex.RLock()
	_, exists := s.activeJobs[jobID]
	s.jobsMutex.RUnlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Setup SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create channel for updates
	updateChan := make(chan JobStatusUpdate, 10)

	// Subscribe to job updates
	s.subsMutex.Lock()
	if s.subscribers[jobID] == nil {
		s.subscribers[jobID] = []chan JobStatusUpdate{}
	}
	s.subscribers[jobID] = append(s.subscribers[jobID], updateChan)
	s.subsMutex.Unlock()

	// Cleanup on disconnect
	defer func() {
		s.subsMutex.Lock()
		subs := s.subscribers[jobID]
		for i, ch := range subs {
			if ch == updateChan {
				s.subscribers[jobID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(updateChan)
		s.subsMutex.Unlock()
	}()

	// Stream updates
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial status
	s.jobsMutex.RLock()
	status := s.activeJobs[jobID]
	s.jobsMutex.RUnlock()

	if status != nil {
		update := JobStatusUpdate{
			JobID:     jobID,
			Status:    status.Job.Status,
			Message:   "Connected to job stream",
			UpdatedAt: time.Now(),
		}
		data, _ := json.Marshal(update)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Stream updates until job completes or client disconnects
	for {
		select {
		case update := <-updateChan:
			data, err := json.Marshal(update)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// Close stream when job completes
			if update.Status == "completed" || update.Status == "failed" {
				return
			}
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			// Send keepalive
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// handleHealth handles GET /health
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// processJobs processes jobs from the queue
func (s *APIServer) processJobs() {
	for job := range s.jobQueue {
		s.processJob(job)
	}
}

// processJob processes a single job
func (s *APIServer) processJob(job *TileJob) {
	ctx := context.Background()
	slog.Info("processing job", "job_id", job.ID, "region", job.Region)

	// Update status to processing
	job.Status = "processing"
	s.updateJobStatus(job.ID, "processing", "Starting tile generation")

	// Create service
	service := NewTileService(s.db, s.s3Client, s.config)

	// Create job options from TileJob fields
	opts := &JobOptions{
		MaxZoom:               job.MaxZoom,
		MinZoom:               job.MinZoom,
		SkipUpload:            job.SkipUpload,
		SkipGeneration:        job.SkipGeneration,
		NoCleanup:             job.NoCleanup,
		ExtractGeometry:       job.ExtractGeometry,
		SkipGeometryInsertion: job.SkipGeometryInsertion,
	}

	err := service.ProcessJobWithOptions(ctx, job, opts)

	if err != nil {
		job.Status = "failed"
		errMsg := err.Error()
		job.ErrorMessage = &errMsg
		s.updateJobStatus(job.ID, "failed", fmt.Sprintf("Job failed: %v", err))
		slog.Error("job failed", "job_id", job.ID, "error", err)
	} else {
		job.Status = "completed"
		s.updateJobStatus(job.ID, "completed", "Job completed successfully")
		slog.Info("job completed", "job_id", job.ID)
	}

	// Update final status
	s.jobsMutex.Lock()
	if status, exists := s.activeJobs[job.ID]; exists {
		status.Job = job
		status.UpdatedAt = time.Now()
		if err != nil {
			status.Error = err
		}
	}
	s.jobsMutex.Unlock()
}

// updateJobStatus updates job status and notifies subscribers
func (s *APIServer) updateJobStatus(jobID, status, message string) {
	update := JobStatusUpdate{
		JobID:     jobID,
		Status:    status,
		Message:   message,
		UpdatedAt: time.Now(),
	}

	// Notify subscribers
	s.subsMutex.RLock()
	subscribers := s.subscribers[jobID]
	s.subsMutex.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- update:
		default:
			// Channel full, skip
		}
	}
}

// createJob creates a job in the database
func (s *APIServer) createJob(ctx context.Context, job *TileJob) error {
	query := `
		INSERT INTO "TileJob" (
			id, region, status, "maxZoom", "minZoom", "skipUpload", "skipGeneration",
			"noCleanup", "extractGeometry", "skipGeometryInsertion",
			"createdAt", "updatedAt"
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := s.db.conn.ExecContext(ctx, query,
		job.ID, job.Region, job.Status,
		job.MaxZoom, job.MinZoom, job.SkipUpload, job.SkipGeneration,
		job.NoCleanup, job.ExtractGeometry, job.SkipGeometryInsertion,
		job.CreatedAt, job.UpdatedAt,
	)
	return err
}

// getJobFromDB retrieves a job from the database
func (s *APIServer) getJobFromDB(ctx context.Context, jobID string) (*TileJob, error) {
	query := `
		SELECT id, region, status, "maxZoom", "minZoom", "skipUpload", "skipGeneration",
		       "noCleanup", "extractGeometry", "skipGeometryInsertion",
		       "currentStep", "roadsExtracted", "tilesGenerated",
		       "totalSizeBytes", "uploadProgress", "uploadedBytes", "errorMessage",
		       "createdAt", "updatedAt", "startedAt", "completedAt"
		FROM "TileJob"
		WHERE id = $1
	`

	job := &TileJob{}
	err := s.db.conn.QueryRowContext(ctx, query, jobID).Scan(
		&job.ID, &job.Region, &job.Status,
		&job.MaxZoom, &job.MinZoom, &job.SkipUpload, &job.SkipGeneration,
		&job.NoCleanup, &job.ExtractGeometry, &job.SkipGeometryInsertion,
		&job.CurrentStep,
		&job.RoadsExtracted, &job.TilesGenerated, &job.TotalSizeBytes,
		&job.UploadProgress, &job.UploadedBytes, &job.ErrorMessage,
		&job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return job, nil
}
