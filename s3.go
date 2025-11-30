package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
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

// UploadDirectory uploads all files from a directory to S3
func (s *S3Client) UploadDirectory(ctx context.Context, localDir, s3Prefix string) (int64, error) {
	logger := slog.With("local_dir", localDir, "s3_prefix", s3Prefix)
	logger.Info("starting directory upload to R2")

	var totalBytes int64

	// Walk through all files in the directory
	err := filepath.Walk(localDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(localDir, filePath)
		if err != nil {
			return err
		}

		// Build S3 key
		s3Key := filepath.Join(s3Prefix, filepath.ToSlash(relPath))
		logger := logger.With("file", relPath, "s3_key", s3Key)

		// Open file
		file, err := os.Open(filePath)
		if err != nil {
			logger.Error("failed to open file", "error", err)
			return err
		}
		defer file.Close()

		// Upload file
		result, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s3Key),
			Body:   file,
			ACL:    types.ObjectCannedACLPublicRead, // Make public for CDN
		})

		if err != nil {
			logger.Error("failed to upload file", "error", err)
			return err
		}

		totalBytes += info.Size()
		logger.Debug("file uploaded", "size_bytes", info.Size(), "s3_location", result.Location)

		return nil
	})

	if err != nil {
		logger.Error("directory upload failed", "error", err)
		return 0, fmt.Errorf("failed to upload directory: %w", err)
	}

	logger.Info("directory upload completed", "total_bytes", totalBytes)
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

// GetPublicURL returns the public URL for an object
func (s *S3Client) GetPublicURL(s3Key string) string {
	// For Cloudflare R2, construct the public URL
	// This will depend on your domain setup
	// Default: https://<account>.r2.cloudflarestorage.com/<bucket>/<key>
	return fmt.Sprintf("https://tiles.drivefinder.com/%s", strings.TrimPrefix(s3Key, s.bucketPath+"/"))
}
