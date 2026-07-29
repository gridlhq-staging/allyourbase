package sbmigrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type storageObjectSource interface {
	open(ctx context.Context, bucket StorageBucket, obj StorageObject) (io.ReadCloser, error)
}

type localStorageExportSource struct {
	root string
}

func (s localStorageExportSource) open(
	_ context.Context,
	bucket StorageBucket,
	obj StorageObject,
) (io.ReadCloser, error) {
	exportBucketDir := filepath.Join(s.root, bucket.Name)
	srcFile := filepath.Join(exportBucketDir, obj.Name)
	if !isStoragePathWithinRoot(exportBucketDir, s.root) ||
		!isStoragePathWithinRoot(srcFile, exportBucketDir) {
		return nil, fmt.Errorf("path traversal detected")
	}
	sf, err := os.Open(srcFile)
	if err != nil {
		return nil, fmt.Errorf("opening source: %w", err)
	}
	return sf, nil
}

type s3StorageObjectSource struct {
	client *s3.Client
}

func newS3StorageObjectSource(ctx context.Context, opts MigrationOptions) (*s3StorageObjectSource, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(opts.StorageS3Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				opts.StorageS3AccessKey,
				opts.StorageS3SecretKey,
				"",
			),
		),
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("loading S3 source config: %w", err)
	}

	endpoint := storageS3BaseEndpoint(opts.StorageS3Endpoint, opts.StorageS3UseSSL)
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &s3StorageObjectSource{client: client}, nil
}

func (s *s3StorageObjectSource) open(
	ctx context.Context,
	bucket StorageBucket,
	obj StorageObject,
) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket.Name),
		Key:    aws.String(obj.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("S3 GetObject %s/%s: %w", bucket.Name, obj.Name, err)
	}
	return out.Body, nil
}

func (m *Migrator) prepareStorageObjectSource(ctx context.Context) (storageObjectSource, error) {
	if strings.TrimSpace(m.opts.StorageExportPath) != "" {
		return localStorageExportSource{root: m.opts.StorageExportPath}, nil
	}
	if m.opts.storageS3SourceConfigured() {
		return newS3StorageObjectSource(ctx, m.opts)
	}
	return nil, fmt.Errorf("storage source is not configured")
}

func storageS3BaseEndpoint(endpoint string, useSSL bool) string {
	endpoint = strings.TrimSpace(endpoint)
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	if useSSL {
		return "https://" + endpoint
	}
	return "http://" + endpoint
}
