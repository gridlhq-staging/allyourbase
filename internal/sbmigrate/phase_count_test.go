package sbmigrate

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPhaseCountIncludesFunctionAndTriggerPhases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want int
	}{
		{
			name: "default includes function and trigger phases",
			opts: MigrationOptions{},
			want: 7,
		},
		{
			name: "skip functions removes function and trigger phases",
			opts: MigrationOptions{SkipFunctions: true},
			want: 5,
		},
		{
			name: "skip data implies skip functions",
			opts: MigrationOptions{SkipData: true},
			want: 3,
		},
		{
			name: "storage includes storage phase after function and trigger phases",
			opts: MigrationOptions{StorageExportPath: "/tmp/export"},
			want: 8,
		},
		{
			name: "S3 storage source includes storage phase",
			opts: s3StorageSourceOptions(),
			want: 8,
		},
		{
			name: "S3 SSL flag alone does not include storage phase",
			opts: MigrationOptions{StorageS3UseSSL: true},
			want: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Migrator{opts: tt.opts}
			testutil.Equal(t, tt.want, m.phaseCount())
		})
	}
}

func TestPhaseCountWithStorage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts MigrationOptions
		want int
	}{
		{
			name: "all phases with storage",
			opts: MigrationOptions{StorageExportPath: "/tmp/export"},
			want: 8,
		},
		{
			name: "skip storage explicitly",
			opts: MigrationOptions{SkipStorage: true, StorageExportPath: "/tmp/export"},
			want: 7,
		},
		{
			name: "S3 storage source enables storage phase",
			opts: s3StorageSourceOptions(),
			want: 8,
		},
		{
			name: "skip storage explicitly with S3 source",
			opts: s3StorageSourceOptionsWithSkipStorage(),
			want: 7,
		},
		{
			name: "S3 SSL flag alone does not enable storage phase",
			opts: MigrationOptions{StorageS3UseSSL: true},
			want: 7,
		},
		{
			name: "no storage path = no storage phase",
			opts: MigrationOptions{},
			want: 7,
		},
		{
			name: "skip all with storage",
			opts: MigrationOptions{SkipData: true, SkipOAuth: true, SkipRLS: true, StorageExportPath: "/tmp/export"},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Migrator{opts: tt.opts}
			testutil.Equal(t, tt.want, m.phaseCount())
		})
	}
}

func s3StorageSourceOptions() MigrationOptions {
	return MigrationOptions{
		StorageS3Endpoint:  "http://127.0.0.1:9000",
		StorageS3Region:    "us-east-1",
		StorageS3AccessKey: "access",
		StorageS3SecretKey: "secret",
	}
}
