-- Tenant-aware storage metadata.
-- Existing self-host rows are backfilled by the NOT NULL DEFAULT '' sentinel so
-- metadata can converge without moving physical bytes in this schema-only stage.
-- Storage usage remains user-keyed because quota accounting is still owned by
-- users, while tenant-scoped metadata queries and backend keys are later stages.

ALTER TABLE _ayb_storage_objects
    ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';

ALTER TABLE _ayb_storage_buckets
    ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';

ALTER TABLE _ayb_storage_uploads
    ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';

DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT con.conname
        FROM pg_constraint con
        JOIN pg_class tbl ON tbl.oid = con.conrelid
        JOIN pg_namespace n ON n.oid = tbl.relnamespace
        WHERE n.nspname = 'public'
          AND tbl.relname = '_ayb_storage_objects'
          AND con.contype = 'u'
          AND (
              SELECT string_agg(att.attname, ',' ORDER BY keys.ordinality)
              FROM unnest(con.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
              JOIN pg_attribute att ON att.attrelid = tbl.oid AND att.attnum = keys.attnum
          ) = 'bucket,name'
    LOOP
        EXECUTE format('ALTER TABLE public._ayb_storage_objects DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END
$$;

DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT con.conname
        FROM pg_constraint con
        JOIN pg_class tbl ON tbl.oid = con.conrelid
        JOIN pg_namespace n ON n.oid = tbl.relnamespace
        WHERE n.nspname = 'public'
          AND tbl.relname = '_ayb_storage_buckets'
          AND con.contype = 'u'
          AND (
              SELECT string_agg(att.attname, ',' ORDER BY keys.ordinality)
              FROM unnest(con.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
              JOIN pg_attribute att ON att.attrelid = tbl.oid AND att.attnum = keys.attnum
          ) = 'name'
    LOOP
        EXECUTE format('ALTER TABLE public._ayb_storage_buckets DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END
$$;

DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT con.conname
        FROM pg_constraint con
        JOIN pg_class tbl ON tbl.oid = con.conrelid
        JOIN pg_namespace n ON n.oid = tbl.relnamespace
        WHERE n.nspname = 'public'
          AND tbl.relname = '_ayb_storage_uploads'
          AND con.contype = 'u'
          AND (
              SELECT string_agg(att.attname, ',' ORDER BY keys.ordinality)
              FROM unnest(con.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
              JOIN pg_attribute att ON att.attrelid = tbl.oid AND att.attnum = keys.attnum
          ) = 'bucket,name,path'
    LOOP
        EXECUTE format('ALTER TABLE public._ayb_storage_uploads DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END
$$;

DROP INDEX IF EXISTS idx_ayb_storage_objects_bucket;
DROP INDEX IF EXISTS idx_ayb_storage_buckets_name;
DROP INDEX IF EXISTS idx_ayb_storage_uploads_bucket_name;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'uq_ayb_storage_objects_tenant_bucket_name'
          AND conrelid = 'public._ayb_storage_objects'::regclass
    ) THEN
        ALTER TABLE public._ayb_storage_objects
            ADD CONSTRAINT uq_ayb_storage_objects_tenant_bucket_name
            UNIQUE (tenant_id, bucket, name);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'uq_ayb_storage_buckets_tenant_name'
          AND conrelid = 'public._ayb_storage_buckets'::regclass
    ) THEN
        ALTER TABLE public._ayb_storage_buckets
            ADD CONSTRAINT uq_ayb_storage_buckets_tenant_name
            UNIQUE (tenant_id, name);
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'uq_ayb_storage_uploads_tenant_bucket_name_path'
          AND conrelid = 'public._ayb_storage_uploads'::regclass
    ) THEN
        ALTER TABLE public._ayb_storage_uploads
            ADD CONSTRAINT uq_ayb_storage_uploads_tenant_bucket_name_path
            UNIQUE (tenant_id, bucket, name, path);
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_ayb_storage_objects_tenant_bucket
    ON _ayb_storage_objects (tenant_id, bucket);

CREATE INDEX IF NOT EXISTS idx_ayb_storage_uploads_tenant_bucket_name
    ON _ayb_storage_uploads (tenant_id, bucket, name);
