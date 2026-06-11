-- Canonical per-collection search settings for future weighted search ranking.
-- The JSONB row shape is {"attributes":[{"column":"title","weight":"high"}]},
-- preserving user-defined attribute order as one atomic collection setting.
CREATE TABLE IF NOT EXISTS _ayb_search_settings (
    schema_name     TEXT NOT NULL,
    table_name      TEXT NOT NULL,
    settings        JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ayb_search_settings_object CHECK (jsonb_typeof(settings) = 'object'),
    CONSTRAINT chk_ayb_search_settings_attributes CHECK (COALESCE(jsonb_typeof(settings->'attributes') = 'array', FALSE)),
    CONSTRAINT uq_ayb_search_settings_collection UNIQUE (schema_name, table_name)
);
