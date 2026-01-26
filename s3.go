package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Client wraps AWS S3 operations for R2
type S3Client struct {
	client     *s3.Client
	bucket     string
	bucketPath string
	uploader   *manager.Uploader
}

// NewS3Client creates a new S3 client for Cloudflare R2
func NewS3Client(cfg S3Config) (*S3Client, error) {
	logger := slog.With("endpoint", cfg.Endpoint, "bucket", cfg.Bucket)
	logger.Info("initializing S3 client for R2")

	// Create custom resolver for R2 endpoint
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           cfg.Endpoint,
				SigningRegion: cfg.Region,
			}, nil
		}
		return aws.Endpoint{}, &smithy.GenericAPIError{Code: "UnknownEndpoint"}
	})

	// Create custom HTTP client with connection pooling optimized for parallel uploads
	// MaxIdleConnsPerHost should match or exceed the number of upload workers (100)
	// to ensure connections are reused instead of constantly opened/closed
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        150,
			MaxIdleConnsPerHost: 150, // Must match or exceed worker count for connection reuse
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 5 * time.Minute, // Overall request timeout
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithHTTPClient(httpClient),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
		config.WithRegion(cfg.Region),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// Create uploader
	uploader := manager.NewUploader(s3Client)

	logger.Info("S3 client initialized successfully")

	return &S3Client{
		client:     s3Client,
		bucket:     cfg.Bucket,
		bucketPath: cfg.BucketPath,
		uploader:   uploader,
	}, nil
}

// UploadDirectory uploads all files from a directory to S3 using parallel workers
func (s *S3Client) UploadDirectory(ctx context.Context, localDir, s3Prefix string) (int64, error) {
	logger := slog.With("local_dir", localDir, "s3_prefix", s3Prefix)
	logger.Info("starting parallel directory upload to R2")

	// First, collect all files to upload
	type fileToUpload struct {
		path     string
		relPath  string
		s3Key    string
		size     int64
	}

	var files []fileToUpload

	err := filepath.Walk(localDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(localDir, filePath)
		if err != nil {
			return err
		}

		s3Key := filepath.Join(s3Prefix, filepath.ToSlash(relPath))

		files = append(files, fileToUpload{
			path:    filePath,
			relPath: relPath,
			s3Key:   s3Key,
			size:    info.Size(),
		})

		return nil
	})

	if err != nil {
		logger.Error("failed to scan directory", "error", err)
		return 0, fmt.Errorf("failed to scan directory: %w", err)
	}

	logger.Info("found files to upload", "count", len(files))

	// Upload files in parallel using worker pool
	const numWorkers = 100 // Parallel upload workers
	var totalBytes int64
	var fileCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create channel for work distribution
	workChan := make(chan fileToUpload, numWorkers*2)
	errChan := make(chan error, 1)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for file := range workChan {
				// Open file
				f, err := os.Open(file.path)
				if err != nil {
					select {
					case errChan <- fmt.Errorf("failed to open file %s: %w", file.relPath, err):
					default:
					}
					return
				}

				// Upload file
				_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{
					Bucket: aws.String(s.bucket),
					Key:    aws.String(file.s3Key),
					Body:   f,
					ACL:    types.ObjectCannedACLPublicRead,
				})
				f.Close()

				if err != nil {
					select {
					case errChan <- fmt.Errorf("failed to upload file %s: %w", file.relPath, err):
					default:
					}
					return
				}

				// Update stats
				mu.Lock()
				totalBytes += file.size
				fileCount++
				currentCount := fileCount
				currentBytes := totalBytes
				mu.Unlock()

				// Log progress every 1000 files
				if currentCount%1000 == 0 {
					logger.Info("upload progress", "files_uploaded", currentCount, "bytes_uploaded", currentBytes)
				}
			}
		}(i)
	}

	// Send work to workers
	go func() {
		for _, file := range files {
			select {
			case <-ctx.Done():
				return
			case workChan <- file:
			}
		}
		close(workChan)
	}()

	// Wait for completion
	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		logger.Error("upload failed", "error", err)
		return 0, err
	}

	logger.Info("directory upload completed", "total_files", fileCount, "total_bytes", totalBytes)
	return totalBytes, nil
}

// UploadTilesWithFilter uploads only tiles from a directory that match the given coordinates
// This allows uploading merged tiles but only for specific region's tile coordinates
func (s *S3Client) UploadTilesWithFilter(ctx context.Context, localDir, s3Prefix string, coords map[TileCoord]bool) (int64, error) {
	logger := slog.With("local_dir", localDir, "s3_prefix", s3Prefix, "filter_count", len(coords))
	logger.Info("starting filtered upload to R2")

	// Collect files that match the filter
	type fileToUpload struct {
		path    string
		relPath string
		s3Key   string
		size    int64
	}

	var files []fileToUpload

	err := filepath.Walk(localDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || filepath.Ext(filePath) != ".pbf" {
			return nil
		}

		// Parse z/x/y.pbf from path
		relPath, err := filepath.Rel(localDir, filePath)
		if err != nil {
			return nil
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) != 3 {
			return nil
		}

		z, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}
		x, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil
		}
		yStr := strings.TrimSuffix(parts[2], ".pbf")
		y, err := strconv.Atoi(yStr)
		if err != nil {
			return nil
		}

		// Check if this tile coordinate is in our filter
		if !coords[TileCoord{z, x, y}] {
			return nil // Skip tiles not in filter
		}

		s3Key := filepath.Join(s3Prefix, filepath.ToSlash(relPath))

		files = append(files, fileToUpload{
			path:    filePath,
			relPath: relPath,
			s3Key:   s3Key,
			size:    info.Size(),
		})

		return nil
	})

	if err != nil {
		logger.Error("failed to scan directory", "error", err)
		return 0, fmt.Errorf("failed to scan directory: %w", err)
	}

	logger.Info("found tiles matching filter", "count", len(files), "filter_count", len(coords))

	// Upload files in parallel using worker pool
	const numWorkers = 100
	var totalBytes int64
	var fileCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	workChan := make(chan fileToUpload, numWorkers*2)
	errChan := make(chan error, 1)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for file := range workChan {
				f, err := os.Open(file.path)
				if err != nil {
					select {
					case errChan <- fmt.Errorf("failed to open file %s: %w", file.relPath, err):
					default:
					}
					return
				}

				_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{
					Bucket: aws.String(s.bucket),
					Key:    aws.String(file.s3Key),
					Body:   f,
					ACL:    types.ObjectCannedACLPublicRead,
				})
				f.Close()

				if err != nil {
					select {
					case errChan <- fmt.Errorf("failed to upload file %s: %w", file.relPath, err):
					default:
					}
					return
				}

				mu.Lock()
				totalBytes += file.size
				fileCount++
				currentCount := fileCount
				currentBytes := totalBytes
				mu.Unlock()

				if currentCount%1000 == 0 {
					logger.Info("upload progress", "files_uploaded", currentCount, "bytes_uploaded", currentBytes)
				}
			}
		}(i)
	}

	// Send work to workers
	go func() {
		for _, file := range files {
			select {
			case <-ctx.Done():
				return
			case workChan <- file:
			}
		}
		close(workChan)
	}()

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		logger.Error("upload failed", "error", err)
		return 0, err
	}

	logger.Info("filtered upload completed", "total_files", fileCount, "total_bytes", totalBytes)
	return totalBytes, nil
}

// UploadFile uploads a single file to S3
func (s *S3Client) UploadFile(ctx context.Context, filePath, s3Key string) (int64, error) {
	logger := slog.With("file_path", filePath, "s3_key", s3Key)
	logger.Debug("uploading file to R2")

	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Upload file
	result, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
		Body:   file,
		ACL:    types.ObjectCannedACLPublicRead,
	})

	if err != nil {
		logger.Error("upload failed", "error", err)
		return 0, fmt.Errorf("failed to upload file: %w", err)
	}

	logger.Debug("file uploaded", "location", result.Location)
	return info.Size(), nil
}

// DeleteObject deletes an object from S3
func (s *S3Client) DeleteObject(ctx context.Context, s3Key string) error {
	logger := slog.With("s3_key", s3Key)
	logger.Debug("deleting object from R2")

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})

	if err != nil {
		logger.Error("delete failed", "error", err)
		return fmt.Errorf("failed to delete object: %w", err)
	}

	logger.Debug("object deleted")
	return nil
}

// ListObjects lists objects in S3
func (s *S3Client) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	logger := slog.With("prefix", prefix)
	logger.Debug("listing objects from R2")

	var objects []string

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			objects = append(objects, *obj.Key)
		}
	}

	logger.Debug("objects listed", "count", len(objects))
	return objects, nil
}

// HeadObject checks if an object exists in S3 and returns its size.
// Returns (size, exists, error). If the object doesn't exist, exists is false and error is nil.
func (s *S3Client) HeadObject(ctx context.Context, s3Key string) (int64, bool, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		// Check if the error is a "not found" type
		var notFound *types.NotFound
		if ok := errors.As(err, &notFound); ok {
			return 0, false, nil
		}
		// Also check for 404 status code in generic API errors
		var apiErr smithy.APIError
		if ok := errors.As(err, &apiErr); ok && apiErr.ErrorCode() == "NotFound" {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to head object %s: %w", s3Key, err)
	}

	var size int64
	if result.ContentLength != nil {
		size = *result.ContentLength
	}
	return size, true, nil
}

// GetPublicURL returns the public URL for an object
func (s *S3Client) GetPublicURL(s3Key string) string {
	// For Cloudflare R2, construct the public URL
	// This will depend on your domain setup
	// Default: https://<account>.r2.cloudflarestorage.com/<bucket>/<key>
	return fmt.Sprintf("https://tiles.drivefinder.com/%s", strings.TrimPrefix(s3Key, s.bucketPath+"/"))
}
