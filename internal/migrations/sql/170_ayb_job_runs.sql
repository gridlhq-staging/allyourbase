-- Per-attempt execution history for jobs.
CREATE TABLE IF NOT EXISTS _ayb_job_runs (
    job_id          UUID NOT NULL REFERENCES _ayb_jobs(id) ON DELETE CASCADE,
    attempt         INT NOT NULL CHECK (attempt >= 1),
    status          VARCHAR(20) NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'canceled')),
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ NOT NULL,
    duration_ms     INT NOT NULL CHECK (duration_ms >= 0),
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (finished_at >= started_at),
    UNIQUE (job_id, attempt)
);

CREATE INDEX IF NOT EXISTS idx_ayb_job_runs_job_started_desc
    ON _ayb_job_runs (job_id, started_at DESC);
