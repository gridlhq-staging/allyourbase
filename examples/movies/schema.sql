-- Movies demo corpus schema.
-- This schema is idempotent so the demo loader can safely reapply it.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS movies (
  id UUID PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL CHECK (length(title) > 0),
  overview TEXT NOT NULL DEFAULT '',
  release_year INTEGER NOT NULL CHECK (release_year >= 1888 AND release_year <= 2100),
  genres TEXT[] NOT NULL DEFAULT '{}'::TEXT[],
  embedding VECTOR(3) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_movies_slug ON movies(slug);
CREATE INDEX IF NOT EXISTS idx_movies_search_doc ON movies USING GIN (to_tsvector('simple', title || ' ' || overview));

DROP POLICY IF EXISTS movies_read ON movies;
ALTER TABLE movies ENABLE ROW LEVEL SECURITY;
CREATE POLICY movies_read ON movies FOR SELECT USING (true);

-- movies_notes: user-generated notes embedded against a referenced movie.
-- Stage 3 owner — no parallel DDL elsewhere. The embedding column matches
-- movies.embedding's VECTOR(3) dimension so the Stage 2 deterministic
-- embedding codec remains usable without per-call dimension translation.
CREATE TABLE IF NOT EXISTS movies_notes (
  id UUID PRIMARY KEY,
  movie_slug TEXT NOT NULL REFERENCES movies(slug),
  text TEXT NOT NULL CHECK (length(text) BETWEEN 1 AND 2000),
  embedding VECTOR(3) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_movies_notes_movie_slug ON movies_notes(movie_slug);

DROP POLICY IF EXISTS movies_notes_read ON movies_notes;
ALTER TABLE movies_notes ENABLE ROW LEVEL SECURITY;
CREATE POLICY movies_notes_read ON movies_notes FOR SELECT USING (true);

-- movies_chat_history: persisted turns from the SSE chat endpoint, keyed
-- by client-supplied session_id so retrieving a session yields its full
-- transcript. partial=true marks assistant rows that were cut short by
-- client disconnect or upstream error.
CREATE TABLE IF NOT EXISTS movies_chat_history (
  id UUID PRIMARY KEY,
  session_id UUID NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user','assistant')),
  content TEXT NOT NULL,
  partial BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_movies_chat_history_session_id ON movies_chat_history(session_id);

DROP FUNCTION IF EXISTS search_movies(TEXT, VECTOR(3), INTEGER);
CREATE OR REPLACE FUNCTION search_movies(p_query TEXT, p_embedding VECTOR(3), p_limit INTEGER DEFAULT 10)
RETURNS TABLE (
  slug TEXT,
  title TEXT,
  overview TEXT,
  release_year INTEGER,
  similarity DOUBLE PRECISION
) AS $$
  WITH params AS (
    SELECT
      btrim(coalesce(p_query, '')) AS normalized_query,
      p_embedding AS query_embedding,
      GREATEST(coalesce(p_limit, 10), 1) AS row_limit
  )
  SELECT
    m.slug,
    m.title,
    m.overview,
    m.release_year,
    (
      (1 - (m.embedding <=> params.query_embedding)) +
      CASE
        WHEN params.normalized_query = '' THEN 0
        ELSE ts_rank(
          to_tsvector('simple', m.title || ' ' || m.overview),
          websearch_to_tsquery('simple', params.normalized_query)
        )
      END
    )::DOUBLE PRECISION AS similarity
  FROM movies AS m
  CROSS JOIN params
  WHERE
    params.normalized_query = ''
    OR to_tsvector('simple', m.title || ' ' || m.overview) @@ websearch_to_tsquery('simple', params.normalized_query)
  ORDER BY similarity DESC, m.slug ASC
  LIMIT (SELECT row_limit FROM params);
$$ LANGUAGE SQL STABLE;
