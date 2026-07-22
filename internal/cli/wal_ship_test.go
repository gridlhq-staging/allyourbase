package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func TestWalShipCommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "wal-ship" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected wal-ship command to be registered")
	}
}

func TestWalShipRequiresArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"wal-ship"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected args error")
	}
}

func TestWalShipMissingConfig(t *testing.T) {
	rootCmd.SetArgs([]string{"wal-ship", "/tmp/seg", "000000010000000000000001", "--config", "/nonexistent/ayb.toml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected config load error")
	}
}

func TestRunWALShipReturnsErrorWhenShipperFails(t *testing.T) {
	const walFileName = "000000010000000000000001"
	walFilePath := filepath.Join(t.TempDir(), walFileName)
	testutil.NoError(t, os.WriteFile(walFilePath, []byte("wal segment"), 0o600))

	origConfigLoader := loadWALShipConfig
	origStoreFactory := newWALShipStore
	origPoolFactory := newWALShipPool
	origRepoFactory := newWALShipRepo
	origShipperFactory := newWALShipShipper
	t.Cleanup(func() {
		loadWALShipConfig = origConfigLoader
		newWALShipStore = origStoreFactory
		newWALShipPool = origPoolFactory
		newWALShipRepo = origRepoFactory
		newWALShipShipper = origShipperFactory
	})

	configPath := filepath.Join(t.TempDir(), "ayb.toml")
	cfg := config.Default()
	cfg.Database.URL = "postgres://user:pass@localhost:5432/app?sslmode=disable"
	cfg.Backup.Enabled = true
	cfg.Backup.PITR.Enabled = true

	loadWALShipConfig = func(path string, flags map[string]string) (*config.Config, error) {
		testutil.Equal(t, configPath, path)
		testutil.Nil(t, flags)
		return cfg, nil
	}
	newWALShipStore = func(context.Context, backup.S3Config) (backup.Store, error) {
		return fakeWALShipStore{}, nil
	}
	fakePool := &fakeWALShipPool{}
	newWALShipPool = func(context.Context, string) (walShipPool, error) {
		return fakePool, nil
	}
	newWALShipRepo = func(pool walShipPool) (backup.WALSegmentRepo, error) {
		if pool != fakePool {
			t.Fatalf("repo factory pool = %#v, want fake pool", pool)
		}
		return fakeWALShipRepo{}, nil
	}
	fakeShipper := &fakeWALSegmentShipper{err: errors.New("upload denied")}
	newWALShipShipper = func(
		backup.Store,
		backup.WALSegmentRepo,
		backup.PITRConfig,
		string,
		string,
		backup.Notifier,
	) walSegmentShipper {
		return fakeShipper
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	testutil.NoError(t, cmd.Flags().Set("config", configPath))

	err := runWALShip(cmd, []string{walFilePath, walFileName})

	testutil.ErrorContains(t, err, "shipping WAL segment")
	testutil.ErrorContains(t, err, "upload denied")
	testutil.True(t, fakeShipper.called, "fake shipper should be called")
	testutil.Equal(t, walFilePath, fakeShipper.walFilePath)
	testutil.Equal(t, walFileName, fakeShipper.walFileName)
	testutil.Equal(t, 1, fakePool.closeCalls)
}

type fakeWALSegmentShipper struct {
	err         error
	called      bool
	walFilePath string
	walFileName string
}

func (f *fakeWALSegmentShipper) Ship(_ context.Context, walFilePath string, walFileName string) error {
	f.called = true
	f.walFilePath = walFilePath
	f.walFileName = walFileName
	return f.err
}

type fakeWALShipPool struct {
	closeCalls int
}

func (f *fakeWALShipPool) Close() {
	f.closeCalls++
}

type fakeWALShipStore struct{}

func (fakeWALShipStore) PutObject(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (fakeWALShipStore) GetObject(context.Context, string) (io.ReadCloser, int64, error) {
	return io.NopCloser(strings.NewReader("")), 0, nil
}

func (fakeWALShipStore) HeadObject(context.Context, string) (int64, error) {
	return 0, errors.New("not found")
}

func (fakeWALShipStore) ListObjects(context.Context, string) ([]backup.StoreObject, error) {
	return nil, nil
}

func (fakeWALShipStore) DeleteObject(context.Context, string) error {
	return nil
}

type fakeWALShipRepo struct{}

func (fakeWALShipRepo) Record(context.Context, backup.WALSegment) error {
	return nil
}

func (fakeWALShipRepo) GetByName(context.Context, string, string, int, string) (*backup.WALSegment, error) {
	return nil, errors.New("not found")
}

func (fakeWALShipRepo) ListRange(context.Context, string, string, string, string) ([]backup.WALSegment, error) {
	return nil, nil
}

func (fakeWALShipRepo) ListOlderThan(context.Context, string, string, time.Time) ([]backup.WALSegment, error) {
	return nil, nil
}

func (fakeWALShipRepo) SumSizeBytes(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (fakeWALShipRepo) Delete(context.Context, string) error {
	return nil
}

func (fakeWALShipRepo) LatestByProject(context.Context, string, string) (*backup.WALSegment, error) {
	return nil, errors.New("not found")
}

func (fakeWALShipRepo) CoveringSegment(context.Context, string, string, string) (*backup.WALSegment, error) {
	return nil, errors.New("not found")
}
