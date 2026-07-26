ALTER TABLE _ayb_request_logs
    ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_ayb_request_logs_tenant
    ON _ayb_request_logs (tenant_id);
