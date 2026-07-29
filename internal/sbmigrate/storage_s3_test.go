package sbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestStorageS3SourceConfigured(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want bool
	}{
		{
			name: "complete S3 source",
			opts: s3StorageSourceOptions(),
			want: true,
		},
		{
			name: "missing endpoint",
			opts: MigrationOptions{
				StorageS3Region:    "us-east-1",
				StorageS3AccessKey: "access",
				StorageS3SecretKey: "secret",
			},
			want: false,
		},
		{
			name: "missing region",
			opts: MigrationOptions{
				StorageS3Endpoint:  "http://127.0.0.1:9000",
				StorageS3AccessKey: "access",
				StorageS3SecretKey: "secret",
			},
			want: false,
		},
		{
			name: "missing access key",
			opts: MigrationOptions{
				StorageS3Endpoint:  "http://127.0.0.1:9000",
				StorageS3Region:    "us-east-1",
				StorageS3SecretKey: "secret",
			},
			want: false,
		},
		{
			name: "missing secret key",
			opts: MigrationOptions{
				StorageS3Endpoint:  "http://127.0.0.1:9000",
				StorageS3Region:    "us-east-1",
				StorageS3AccessKey: "access",
			},
			want: false,
		},
		{
			name: "SSL flag alone",
			opts: MigrationOptions{StorageS3UseSSL: true},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, tt.opts.storageS3SourceConfigured())
		})
	}
}

func TestValidateStorageSourceOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    MigrationOptions
		wantErr string
	}{
		{
			name:    "no storage source",
			opts:    MigrationOptions{},
			wantErr: "",
		},
		{
			name:    "local export only",
			opts:    MigrationOptions{StorageExportPath: "/tmp/export"},
			wantErr: "",
		},
		{
			name:    "complete S3 source",
			opts:    s3StorageSourceOptions(),
			wantErr: "",
		},
		{
			name: "missing region",
			opts: MigrationOptions{
				StorageS3Endpoint:  "http://127.0.0.1:9000",
				StorageS3AccessKey: "access",
				StorageS3SecretKey: "secret",
			},
			wantErr: "S3 storage source region is required",
		},
		{
			name: "export and S3 combined",
			opts: MigrationOptions{
				StorageExportPath:  "/tmp/export",
				StorageS3Endpoint:  "http://127.0.0.1:9000",
				StorageS3Region:    "us-east-1",
				StorageS3AccessKey: "access",
				StorageS3SecretKey: "secret",
			},
			wantErr: "storage export path cannot be combined with S3 storage source options",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.opts.validateStorageSourceOptions()
			if tt.wantErr == "" {
				testutil.NoError(t, err)
				return
			}
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func s3StorageSourceOptionsWithSkipStorage() MigrationOptions {
	opts := s3StorageSourceOptions()
	opts.SkipStorage = true
	opts.StorageS3UseSSL = true
	return opts
}
