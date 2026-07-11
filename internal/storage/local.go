// Package storage LocalBackend provides file storage on the local filesystem with path traversal protection.
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalBackend stores files on the local filesystem.
type LocalBackend struct {
	root string
}

// NewLocalBackend creates a local filesystem backend rooted at the given path.
func NewLocalBackend(root string) (*LocalBackend, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving storage path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("creating storage directory: %w", err)
	}
	return &LocalBackend{root: abs}, nil
}

// Put writes data from r to the named object in the given bucket, creating parent
// directories as needed. It returns the number of bytes written. The write is an
// atomic replace: data is written to a temp file in the destination directory and
// renamed over the target, so a failed or interrupted write leaves any existing
// object intact (matching S3 PutObject semantics). This lets resumable staging
// safely rewrite-on-append.
func (b *LocalBackend) Put(_ context.Context, tenantID, bucket, name string, r io.Reader) (int64, error) {
	path, err := b.objectPath(tenantID, bucket, name)
	if err != nil {
		return 0, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("creating directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".ayb-put-*")
	if err != nil {
		return 0, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	n, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath) // discard partial temp file; existing object untouched
		return 0, fmt.Errorf("writing file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("closing file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("finalizing file: %w", err)
	}

	return n, nil
}

func (b *LocalBackend) Get(_ context.Context, tenantID, bucket, name string) (io.ReadCloser, error) {
	path, err := b.objectPath(tenantID, bucket, name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return f, nil
}

func (b *LocalBackend) Delete(_ context.Context, tenantID, bucket, name string) error {
	path, err := b.objectPath(tenantID, bucket, name)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing file: %w", err)
	}
	return nil
}

func (b *LocalBackend) Exists(_ context.Context, tenantID, bucket, name string) (bool, error) {
	path, err := b.objectPath(tenantID, bucket, name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}
	return true, nil
}

// objectPath returns the absolute filesystem path for the named object within
// the tenant namespace. Empty tenantID is intentionally unprefixed so existing
// self-hosted objects at root/bucket/name remain reachable without byte moves.
func (b *LocalBackend) objectPath(tenantID, bucket, name string) (string, error) {
	if err := validateBucket(bucket); err != nil {
		return "", err
	}
	if err := validateName(name); err != nil {
		return "", err
	}
	parts := []string{b.root}
	if tenantID != "" {
		parts = append(parts, "t", tenantID)
	}
	parts = append(parts, bucket, name)
	target := filepath.Join(parts...)
	rel, err := filepath.Rel(b.root, target)
	if err != nil {
		return "", fmt.Errorf("resolving object path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: object path escapes storage root", ErrInvalidName)
	}
	return target, nil
}
