// Package storage resumable_staging.go owns the byte mechanics behind resumable
// uploads: the reserved staging bucket, staging-token generation, the single
// offset/chunk-size enforcement seam (appendChunk), and the backend-backed
// rewrite-on-append staging step. Session lifecycle and DB state live in
// resumable.go.
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// resumableStagingBucket is the reserved backend bucket holding in-progress
// resumable upload bytes. Staging bytes live in the same backend as finalized
// objects so any node sharing Postgres plus backend state can resume an upload.
// CreateBucket refuses this name so staged blobs never collide with real
// objects.
const resumableStagingBucket = "ayb_resumable_staging"

// Rewrite-on-append trade-off: each PATCH re-reads the whole staged object,
// appends the new chunk, and rewrites the object. Total bytes moved for an N-byte
// upload delivered in fixed-size chunks is O(N^2). This keeps the Backend
// interface unchanged (Get/Put only) and is acceptable for the current chunk
// sizes; the future escape hatch is per-part staging objects (one blob per
// chunk, concatenated at finalize) which trades O(N^2) rewrites for O(parts)
// object bookkeeping.

// newStagingToken returns a random backend object key for a resumable upload's
// staging blob. It is stored as _ayb_storage_uploads.path.
func newStagingToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating resumable staging token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// appendChunk appends a chunk from src to the file at path starting at offset,
// limited to remaining bytes. It validates offset consistency and truncates the
// file if necessary. Returns the number of bytes written or an error if the chunk
// would exceed remaining bytes. This is the single offset and chunk-size
// enforcement seam for resumable uploads.
func appendChunk(path string, offset int64, remaining int64, src io.Reader) (int64, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return 0, fmt.Errorf("opening resumable file: %w", err)
	}
	defer f.Close()

	if offset < 0 {
		return 0, fmt.Errorf("offset must not be negative")
	}

	existing, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("seeking resumable file: %w", err)
	}
	if existing < offset {
		return 0, ErrResumableUploadOffsetMismatch
	}
	if existing != offset {
		if err := f.Truncate(offset); err != nil {
			return 0, fmt.Errorf("rewinding resumable file: %w", err)
		}
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("positioning resumable file: %w", err)
	}

	if remaining <= 0 {
		return 0, ErrResumableUploadInvalidState
	}

	limited := io.LimitReader(src, remaining+1)
	written, err := io.Copy(f, limited)
	if err != nil {
		return written, fmt.Errorf("writing resumable chunk: %w", err)
	}
	if written > remaining {
		return written, ErrResumableUploadChunkTooLarge
	}
	return written, nil
}

// stageAppendedChunk merges one chunk into the backend staging object. It loads
// the already-staged bytes into a transient per-request scratch file, appends
// through appendChunk (the single offset/size enforcement seam), and rewrites the
// staged object via s.backend.Put. An absent staging object is the empty-upload
// state. Returns the number of bytes written from src.
func (s *Service) stageAppendedChunk(ctx context.Context, upload *ResumableUpload, offset, remaining int64, src io.Reader) (int64, error) {
	scratch, err := os.CreateTemp("", "ayb-resumable-scratch-*")
	if err != nil {
		return 0, fmt.Errorf("creating resumable scratch file: %w", err)
	}
	scratchPath := scratch.Name()
	defer os.Remove(scratchPath)

	existing, err := s.backend.Get(ctx, upload.TenantID, resumableStagingBucket, upload.Path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		scratch.Close()
		return 0, err
	}
	if err == nil {
		_, copyErr := io.Copy(scratch, existing)
		existing.Close()
		if copyErr != nil {
			scratch.Close()
			return 0, fmt.Errorf("loading staged upload: %w", copyErr)
		}
	}
	if err := scratch.Close(); err != nil {
		return 0, fmt.Errorf("closing resumable scratch file: %w", err)
	}

	written, err := appendChunk(scratchPath, offset, remaining, src)
	if err != nil {
		return 0, err
	}

	merged, err := os.Open(scratchPath)
	if err != nil {
		return 0, fmt.Errorf("reopening resumable scratch file: %w", err)
	}
	defer merged.Close()
	if _, err := s.backend.Put(ctx, upload.TenantID, resumableStagingBucket, upload.Path, merged); err != nil {
		return 0, err
	}
	return written, nil
}
