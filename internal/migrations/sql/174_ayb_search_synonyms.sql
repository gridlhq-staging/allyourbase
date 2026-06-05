-- Canonical per-collection synonym memberships for search expansion.
-- Each row maps one lowercased term to the stable group it belongs to within
-- a single schema/table collection.
CREATE TABLE IF NOT EXISTS _ayb_search_synonyms (
    schema_name     TEXT NOT NULL,
    table_name      TEXT NOT NULL,
    group_id        UUID NOT NULL,
    term            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ayb_search_synonyms_term_lowercase CHECK (term = lower(term)),
    CONSTRAINT uq_ayb_search_synonyms_collection_term UNIQUE (schema_name, table_name, term)
);
