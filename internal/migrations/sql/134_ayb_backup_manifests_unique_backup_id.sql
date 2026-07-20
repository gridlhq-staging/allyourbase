-- Migration 134: Add UNIQUE constraint on backup_id in _ayb_backup_manifests.
-- This ensures one manifest per physical backup. The idempotent create
-- in PgManifestRepo depends on this constraint for ON CONFLICT semantics.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class idx
        JOIN pg_namespace nsp ON nsp.oid = idx.relnamespace
        WHERE nsp.nspname = 'public'
          AND idx.relname = '_ayb_backup_manifests_backup_id_key'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = '_ayb_backup_manifests_backup_id_key'
    ) THEN
        ALTER TABLE _ayb_backup_manifests
        ADD CONSTRAINT _ayb_backup_manifests_backup_id_key
        UNIQUE USING INDEX _ayb_backup_manifests_backup_id_key;
    ELSIF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = '_ayb_backup_manifests_backup_id_key'
    ) THEN
        ALTER TABLE _ayb_backup_manifests
        ADD CONSTRAINT _ayb_backup_manifests_backup_id_key UNIQUE (backup_id);
    END IF;
END $$;
