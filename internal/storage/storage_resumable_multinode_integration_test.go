//go:build integration

package storage_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

// setupMultinodeStorage migrates the shared database and returns a backend root
// plus a fresh tenant so each multinode test starts from clean, isolated state.
func setupMultinodeStorage(t *testing.T) (dir, tenantID string) {
	t.Helper()
	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	runner := migrations.NewRunner(pool, logger)
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)

	dir = t.TempDir()
	tenantSvc := tenant.NewService(pool, logger)
	tenantID = createQuotaTestTenant(t, ctx, tenantSvc, "multinode-storage").ID
	return dir, tenantID
}

// newStorageNode builds an independent storage.Service backed by its own
// LocalBackend instance rooted at dir. Two nodes sharing dir model separate
// server processes that share Postgres plus backend state.
func newStorageNode(t *testing.T, dir string) *storage.Service {
	t.Helper()
	backend, err := storage.NewLocalBackend(dir)
	testutil.NoError(t, err)
	return storage.NewService(sharedPG.Pool, backend, "test-sign-key-at-least-32-chars!!", testutil.DiscardLogger(), 0)
}

// countFilesUnder returns the number of regular files anywhere under root. It
// proves staged bytes live in the shared backend root rather than a node-local
// OS temp directory.
func countFilesUnder(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	testutil.NoError(t, err)
	return count
}

// TestResumableCompletedByDifferentNode proves a resumable upload started on one
// node can be appended to and finalized by a different node over one shared
// backend root and Postgres, with exact finalized bytes.
func TestResumableCompletedByDifferentNode(t *testing.T) {
	dir, tenantID := setupMultinodeStorage(t)
	clearStorageData(t)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)

	node1 := newStorageNode(t, dir)
	node2 := newStorageNode(t, dir)

	_, err := node1.CreateBucket(ctx, "shared", false)
	testutil.NoError(t, err)

	const part1 = "hello "
	const part2 = "multinode world"
	full := part1 + part2
	upload, err := node1.CreateResumableUpload(ctx, "shared", "obj.txt", "text/plain", nil, int64(len(full)))
	testutil.NoError(t, err)

	_, done, err := node1.AppendResumableUpload(ctx, upload.ID, 0, nil, strings.NewReader(part1))
	testutil.NoError(t, err)
	testutil.False(t, done)

	// A different node finishes the same upload; only shared Postgres + backend
	// state is available to it.
	got, done, err := node2.AppendResumableUpload(ctx, upload.ID, int64(len(part1)), nil, strings.NewReader(part2))
	testutil.NoError(t, err)
	testutil.True(t, done)
	testutil.Equal(t, int64(len(full)), got.UploadedSize)

	obj, err := node2.FinalizeResumableUpload(ctx, upload.ID, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(full)), obj.Size)

	assertStorageDownloadBody(t, ctx, node2, "shared", "obj.txt", full)
	assertStorageDownloadBody(t, ctx, node1, "shared", "obj.txt", full)
}

// TestResumableStagingSurvivesNoLocalState proves the staged bytes live in the
// shared backend root (not node-local OS temp), so a pristine node with its own
// backend instance can finalize an upload it never touched while active.
func TestResumableStagingSurvivesNoLocalState(t *testing.T) {
	dir, tenantID := setupMultinodeStorage(t)
	clearStorageData(t)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)

	node1 := newStorageNode(t, dir)
	_, err := node1.CreateBucket(ctx, "shared", false)
	testutil.NoError(t, err)

	const body = "staging survives without any node-local temp files"
	upload, err := node1.CreateResumableUpload(ctx, "shared", "survive.bin", "application/octet-stream", nil, int64(len(body)))
	testutil.NoError(t, err)

	_, done, err := node1.AppendResumableUpload(ctx, upload.ID, 0, nil, strings.NewReader(body))
	testutil.NoError(t, err)
	testutil.True(t, done)

	// Staged bytes must be reachable through the shared backend root, not an OS
	// temp directory outside it.
	testutil.True(t, countFilesUnder(t, dir) >= 1, "staged upload bytes must reside under the shared backend root")

	node2 := newStorageNode(t, dir)
	obj, err := node2.FinalizeResumableUpload(ctx, upload.ID, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(body)), obj.Size)
	assertStorageDownloadBody(t, ctx, node2, "shared", "survive.bin", body)
}

// TestCreateBucketRejectsReservedStagingName proves the reserved staging bucket
// name cannot be created by callers, so staged blobs never collide with real
// objects.
func TestCreateBucketRejectsReservedStagingName(t *testing.T) {
	dir, tenantID := setupMultinodeStorage(t)
	clearStorageData(t)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)

	node := newStorageNode(t, dir)

	_, err := node.CreateBucket(ctx, "ayb_resumable_staging", false)
	testutil.Error(t, err)
	testutil.True(t, errors.Is(err, storage.ErrInvalidBucket), "reserved staging bucket must be rejected as invalid")
	testutil.ErrorContains(t, err, "reserved")

	// A normal bucket name still succeeds.
	_, err = node.CreateBucket(ctx, "normalbucket", false)
	testutil.NoError(t, err)
}

var errSentinelBackendGet = errors.New("sentinel backend get failure")

// getErrorBackend makes every Get fail with a non-NotFound sentinel while
// delegating everything else to a real backend.
type getErrorBackend struct {
	storage.Backend
	err error
}

func (b getErrorBackend) Get(_ context.Context, _, _, _ string) (io.ReadCloser, error) {
	return nil, b.err
}

// TestAppendPropagatesNonNotFoundBackendError proves append surfaces backend Get
// errors other than storage.ErrNotFound instead of treating them as an empty
// upload.
func TestAppendPropagatesNonNotFoundBackendError(t *testing.T) {
	dir, tenantID := setupMultinodeStorage(t)
	clearStorageData(t)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)

	real, err := storage.NewLocalBackend(dir)
	testutil.NoError(t, err)
	fake := getErrorBackend{Backend: real, err: errSentinelBackendGet}
	svc := storage.NewService(sharedPG.Pool, fake, "test-sign-key-at-least-32-chars!!", testutil.DiscardLogger(), 0)

	_, err = svc.CreateBucket(ctx, "shared", false)
	testutil.NoError(t, err)
	upload, err := svc.CreateResumableUpload(ctx, "shared", "err.bin", "application/octet-stream", nil, 10)
	testutil.NoError(t, err)

	_, _, err = svc.AppendResumableUpload(ctx, upload.ID, 0, nil, strings.NewReader("0123456789"))
	testutil.Error(t, err)
	testutil.True(t, errors.Is(err, errSentinelBackendGet), "non-NotFound backend Get error must propagate")
}

var errInjectedStagingPut = errors.New("injected staging put failure")

// interruptOncePutBackend interrupts one Put by first driving the underlying
// backend with a reader that fails partway (touching the destination object) and
// then returning an error. It models a crash mid-Put so atomic-replace semantics
// can be verified.
type interruptOncePutBackend struct {
	storage.Backend
	failOnPut int
	puts      int
}

func (b *interruptOncePutBackend) Put(ctx context.Context, tenantID, bucket, name string, r io.Reader) (int64, error) {
	b.puts++
	if b.puts == b.failOnPut {
		_, _ = b.Backend.Put(ctx, tenantID, bucket, name, io.MultiReader(strings.NewReader("XXXX"), erroringReader{}))
		return 0, errInjectedStagingPut
	}
	return b.Backend.Put(ctx, tenantID, bucket, name, r)
}

type erroringReader struct{}

func (erroringReader) Read(_ []byte) (int, error) {
	return 0, errors.New("injected read failure")
}

// TestAppendRemainsResumableAfterFailedStagingPut proves an interrupted staging
// Put does not destroy already-staged bytes: a retry at the same offset succeeds
// and finalized bytes are exact. Requires LocalBackend.Put to be atomic-replace.
func TestAppendRemainsResumableAfterFailedStagingPut(t *testing.T) {
	dir, tenantID := setupMultinodeStorage(t)
	clearStorageData(t)
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)

	real, err := storage.NewLocalBackend(dir)
	testutil.NoError(t, err)
	// Put #1 stages part1; Put #2 (the second append) is interrupted.
	fake := &interruptOncePutBackend{Backend: real, failOnPut: 2}
	svc := storage.NewService(sharedPG.Pool, fake, "test-sign-key-at-least-32-chars!!", testutil.DiscardLogger(), 0)

	_, err = svc.CreateBucket(ctx, "shared", false)
	testutil.NoError(t, err)

	const part1 = "durable-"
	const part2 = "bytes"
	full := part1 + part2
	upload, err := svc.CreateResumableUpload(ctx, "shared", "durable.bin", "application/octet-stream", nil, int64(len(full)))
	testutil.NoError(t, err)

	_, done, err := svc.AppendResumableUpload(ctx, upload.ID, 0, nil, strings.NewReader(part1))
	testutil.NoError(t, err)
	testutil.False(t, done)

	// The interrupted append must not wedge the upload.
	_, _, err = svc.AppendResumableUpload(ctx, upload.ID, int64(len(part1)), nil, strings.NewReader(part2))
	testutil.Error(t, err)
	testutil.True(t, errors.Is(err, errInjectedStagingPut), "expected the injected staging Put failure")

	// Retry at the same offset succeeds because part1 survived intact.
	_, done, err = svc.AppendResumableUpload(ctx, upload.ID, int64(len(part1)), nil, strings.NewReader(part2))
	testutil.NoError(t, err)
	testutil.True(t, done)

	obj, err := svc.FinalizeResumableUpload(ctx, upload.ID, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(len(full)), obj.Size)
	assertStorageDownloadBody(t, ctx, svc, "shared", "durable.bin", full)
}
