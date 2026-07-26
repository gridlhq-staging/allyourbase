package sbmigrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/allyourbase/ayb/internal/storage"
)

type storageObjectBackup struct {
	path string
}

func backupStorageObject(
	ctx context.Context,
	backend storage.Backend,
	scratchDir string,
	bucketName string,
	objectName string,
) (*storageObjectBackup, error) {
	existing, err := backend.Get(ctx, "", bucketName, objectName)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading existing object: %w", err)
	}
	defer existing.Close()

	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}
	scratch, err := os.CreateTemp(scratchDir, "ayb-storage-migration-backup-*")
	if err != nil {
		return nil, fmt.Errorf("creating backup: %w", err)
	}
	backup := &storageObjectBackup{path: scratch.Name()}
	if _, err := io.Copy(scratch, existing); err != nil {
		scratch.Close()
		backup.discard()
		return nil, fmt.Errorf("writing backup: %w", err)
	}
	if err := scratch.Close(); err != nil {
		backup.discard()
		return nil, fmt.Errorf("closing backup: %w", err)
	}
	return backup, nil
}

func rollbackCopiedStorageObjects(
	ctx context.Context,
	backend storage.Backend,
	objects []copiedStorageObject,
) error {
	var rollbackErrors []error
	for _, object := range objects {
		if err := object.backup.restore(ctx, backend, object.bucketName, object.name); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf(
				"restoring storage object %s/%s after metadata failure: %w",
				object.sourceBucket,
				object.name,
				err,
			))
		}
	}
	return errors.Join(rollbackErrors...)
}

func (b *storageObjectBackup) restore(
	ctx context.Context,
	backend storage.Backend,
	bucketName string,
	objectName string,
) error {
	if b == nil {
		return backend.Delete(ctx, "", bucketName, objectName)
	}
	defer b.discard()

	previous, err := os.Open(b.path)
	if err != nil {
		return fmt.Errorf("opening backup: %w", err)
	}
	defer previous.Close()
	if _, err := backend.Put(ctx, "", bucketName, objectName, previous); err != nil {
		return fmt.Errorf("replacing bytes from backup: %w", err)
	}
	return nil
}

func (b *storageObjectBackup) discard() {
	if b != nil {
		_ = os.Remove(b.path)
	}
}
