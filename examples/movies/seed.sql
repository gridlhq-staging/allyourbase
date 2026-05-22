-- Deterministic movies corpus seed.
-- Re-runnable: upserts by slug so repeated loads never duplicate rows.
-- State-idempotent: the conflict update path only fires when at least one
-- seeded business field actually differs from the row already in the table.
-- Reapplying an unchanged seed leaves `updated_at` (and the row body) intact.

INSERT INTO movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'A thief enters dreams to steal secrets and perform a final heist inside layered realities.',
    2010,
    ARRAY['sci-fi', 'thriller'],
    '[0.91,0.12,0.18]'
  ),
  (
    '22222222-2222-2222-2222-222222222222',
    'arrival',
    'Arrival',
    'A linguist helps decode alien language after mysterious ships appear around the world.',
    2016,
    ARRAY['sci-fi', 'drama'],
    '[0.31,0.88,0.22]'
  ),
  (
    '33333333-3333-3333-3333-333333333333',
    'moonlight',
    'Moonlight',
    'A young man navigates identity, family, and belonging across three defining chapters of life.',
    2016,
    ARRAY['drama'],
    '[0.06,0.26,0.97]'
  )
ON CONFLICT (slug) DO UPDATE SET
  id = EXCLUDED.id,
  title = EXCLUDED.title,
  overview = EXCLUDED.overview,
  release_year = EXCLUDED.release_year,
  genres = EXCLUDED.genres,
  embedding = EXCLUDED.embedding,
  updated_at = now()
WHERE
  movies.id IS DISTINCT FROM EXCLUDED.id
  OR movies.title IS DISTINCT FROM EXCLUDED.title
  OR movies.overview IS DISTINCT FROM EXCLUDED.overview
  OR movies.release_year IS DISTINCT FROM EXCLUDED.release_year
  OR movies.genres IS DISTINCT FROM EXCLUDED.genres
  OR movies.embedding IS DISTINCT FROM EXCLUDED.embedding;
