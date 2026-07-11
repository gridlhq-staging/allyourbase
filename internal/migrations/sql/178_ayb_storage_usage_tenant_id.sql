-- Tenant-scoped System 1 storage usage accounting.
-- Legacy rows keep the empty tenant sentinel because historical usage rows were
-- user-keyed and cannot be attributed to a tenant after the fact.

ALTER TABLE public._ayb_storage_usage
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
          AND tbl.relname = '_ayb_storage_usage'
          AND con.contype IN ('p', 'u')
          AND (
              SELECT string_agg(att.attname, ',' ORDER BY keys.ordinality)
              FROM unnest(con.conkey) WITH ORDINALITY AS keys(attnum, ordinality)
              JOIN pg_attribute att ON att.attrelid = tbl.oid AND att.attnum = keys.attnum
          ) = 'user_id'
    LOOP
        EXECUTE format('ALTER TABLE public._ayb_storage_usage DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'pk_ayb_storage_usage_tenant_user'
          AND conrelid = 'public._ayb_storage_usage'::regclass
    ) THEN
        ALTER TABLE public._ayb_storage_usage
            ADD CONSTRAINT pk_ayb_storage_usage_tenant_user
            PRIMARY KEY (tenant_id, user_id);
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_ayb_storage_usage_tenant
    ON public._ayb_storage_usage (tenant_id);
