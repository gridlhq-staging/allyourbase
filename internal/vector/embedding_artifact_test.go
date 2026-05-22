package vector

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testMoviesSeedSQL = `INSERT INTO movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '33333333-3333-3333-3333-333333333333',
    'moonlight',
    'Moonlight',
    'overview',
    2016,
    ARRAY['drama'],
    '[0.06,0.26,0.97]'
  ),
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi', 'thriller'],
    '[0.91,0.12,0.18]'
  );`

func TestBuildMoviesEmbeddingArtifactFromSeedDeterministicOrdering(t *testing.T) {
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(testMoviesSeedSQL)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if artifact.FormatVersion != MoviesEmbeddingArtifactFormatVersion {
		t.Fatalf("format version = %d, want %d", artifact.FormatVersion, MoviesEmbeddingArtifactFormatVersion)
	}
	if len(artifact.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(artifact.Records))
	}
	if artifact.Records[0].Slug != "inception" || artifact.Records[1].Slug != "moonlight" {
		t.Fatalf("records are not sorted by slug: %+v", artifact.Records)
	}
}

// SQL doubles a single quote inside a string literal to embed an apostrophe.
// Legitimate seed entries can contain apostrophes (e.g. titles like O'Brien)
// and the canonical rebuild path must not crash on them.
func TestBuildMoviesEmbeddingArtifactFromSeedHandlesEscapedQuotes(t *testing.T) {
	const seedWithApostrophe = `INSERT INTO movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '44444444-4444-4444-4444-444444444444',
    'obriens-tale',
    'O''Brien''s Tale',
    'A film about O''Brien''s journey, with quotes and commas, too.',
    1999,
    ARRAY['drama'],
    '[0.10,0.20,0.30]'
  );`
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(seedWithApostrophe)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(artifact.Records))
	}
	rec := artifact.Records[0]
	if rec.Slug != "obriens-tale" {
		t.Fatalf("slug = %q, want %q", rec.Slug, "obriens-tale")
	}
	if rec.Title != "O'Brien's Tale" {
		t.Fatalf("title = %q, want %q (escaped quotes must resolve to a single apostrophe)", rec.Title, "O'Brien's Tale")
	}
	want := []float64{0.10, 0.20, 0.30}
	if !reflect.DeepEqual(rec.Embedding, want) {
		t.Fatalf("embedding = %v, want %v", rec.Embedding, want)
	}
}

// The parser must resolve columns by their INSERT column-list position, so a
// reasonable shape change (e.g. inserting an extra column ahead of embedding
// or reordering non-target columns) does not silently break artifact rebuilds.
func TestBuildMoviesEmbeddingArtifactFromSeedToleratesColumnReorder(t *testing.T) {
	const reorderedSeed = `INSERT INTO movies (slug, id, embedding, title, overview, release_year, genres)
VALUES
  (
    'inception',
    '11111111-1111-1111-1111-111111111111',
    '[0.91,0.12,0.18]',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi']
  );`
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(reorderedSeed)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(artifact.Records))
	}
	rec := artifact.Records[0]
	if rec.ID != "11111111-1111-1111-1111-111111111111" || rec.Slug != "inception" || rec.Title != "Inception" {
		t.Fatalf("column resolution failed: %+v", rec)
	}
	if !reflect.DeepEqual(rec.Embedding, []float64{0.91, 0.12, 0.18}) {
		t.Fatalf("embedding mismatch: %v", rec.Embedding)
	}
}

// Embedding fields in SQL can legally include an explicit cast suffix after
// the quoted vector literal. The canonical parser must accept this shape.
func TestBuildMoviesEmbeddingArtifactFromSeedAcceptsExplicitVectorCast(t *testing.T) {
	const castedEmbeddingSeed = `INSERT INTO movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'::vector(3)
  );`
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(castedEmbeddingSeed)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(artifact.Records))
	}
	if !reflect.DeepEqual(artifact.Records[0].Embedding, []float64{0.91, 0.12, 0.18}) {
		t.Fatalf("embedding mismatch: %v", artifact.Records[0].Embedding)
	}
}

// Seed files can contain multiple INSERT statements. The artifact parser must
// bind columns and tuples to the same `INSERT INTO movies ... VALUES` statement
// instead of mixing the first INSERT with an unrelated VALUES clause.
func TestBuildMoviesEmbeddingArtifactFromSeedBindsMoviesInsertStatement(t *testing.T) {
	const multiInsertSeed = `INSERT INTO genres (id, slug)
VALUES
  (
    'g1',
    'drama'
  );

INSERT INTO movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(multiInsertSeed)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("expected 1 movie record, got %d", len(artifact.Records))
	}
	if artifact.Records[0].Slug != "inception" {
		t.Fatalf("slug = %q, want %q", artifact.Records[0].Slug, "inception")
	}
}

// PostgreSQL accepts quoted schema/table identifiers in INSERT targets.
// The artifact parser must still resolve the movies statement in this shape.
func TestBuildMoviesEmbeddingArtifactFromSeedAcceptsQuotedSchemaTableIdentifier(t *testing.T) {
	const quotedTableSeed = `INSERT INTO "public"."movies" (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(quotedTableSeed)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("expected 1 movie record, got %d", len(artifact.Records))
	}
	if artifact.Records[0].Slug != "inception" {
		t.Fatalf("slug = %q, want %q", artifact.Records[0].Slug, "inception")
	}
}

// PostgreSQL allows whitespace around qualification separators in table
// targets. Both unquoted and quoted schema/table forms remain valid and must
// still resolve to the canonical movies INSERT statement.
func TestBuildMoviesEmbeddingArtifactFromSeedAcceptsWhitespaceAroundQualifiedTableSeparator(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{
			name: "unquoted schema and table",
			sql: `INSERT INTO public . movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`,
		},
		{
			name: "quoted schema and table",
			sql: `INSERT INTO "public" . "movies" (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := BuildMoviesEmbeddingArtifactFromSeed(tc.sql)
			if err != nil {
				t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
			}
			if len(artifact.Records) != 1 {
				t.Fatalf("expected 1 movie record, got %d", len(artifact.Records))
			}
			if artifact.Records[0].Slug != "inception" {
				t.Fatalf("slug = %q, want %q", artifact.Records[0].Slug, "inception")
			}
		})
	}
}

// Column names can be quoted as well. Required-column lookup must normalize
// quoted identifiers before matching canonical names like id/slug/title.
func TestBuildMoviesEmbeddingArtifactFromSeedAcceptsQuotedColumnIdentifiers(t *testing.T) {
	const quotedColumnsSeed = `INSERT INTO movies ("id", "slug", "title", "overview", "release_year", "genres", "embedding")
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(quotedColumnsSeed)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	if len(artifact.Records) != 1 {
		t.Fatalf("expected 1 movie record, got %d", len(artifact.Records))
	}
	if artifact.Records[0].ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("id = %q, want movie id", artifact.Records[0].ID)
	}
	if artifact.Records[0].Slug != "inception" {
		t.Fatalf("slug = %q, want %q", artifact.Records[0].Slug, "inception")
	}
}

// Quoted SQL identifiers are case-sensitive in PostgreSQL. "ID" is distinct
// from the unquoted owner column id, so the artifact parser must reject it.
func TestBuildMoviesEmbeddingArtifactFromSeedRejectsCaseMismatchedQuotedColumnIdentifiers(t *testing.T) {
	const quotedColumnsSeed = `INSERT INTO movies ("ID", "slug", "title", "overview", "release_year", "genres", "embedding")
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	_, err := BuildMoviesEmbeddingArtifactFromSeed(quotedColumnsSeed)
	if err == nil {
		t.Fatal("expected case-mismatched quoted identifier \"ID\" to be rejected")
	}
	if !strings.Contains(err.Error(), "required column 'id'") {
		t.Fatalf("error %q should mention missing required id column", err.Error())
	}
}

// Quoted table identifiers are case-sensitive. "Movies" should not match the
// canonical unquoted movies target used by the seed loader.
func TestBuildMoviesEmbeddingArtifactFromSeedRejectsCaseMismatchedQuotedTableIdentifier(t *testing.T) {
	const quotedTableSeed = `INSERT INTO "public"."Movies" (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	_, err := BuildMoviesEmbeddingArtifactFromSeed(quotedTableSeed)
	if err == nil {
		t.Fatal("expected case-mismatched quoted table \"Movies\" to be rejected")
	}
	if !strings.Contains(err.Error(), "no INSERT INTO movies statement found") {
		t.Fatalf("error %q should mention missing movies INSERT", err.Error())
	}
}

// Quoted identifiers preserve interior whitespace in PostgreSQL. "id " is
// distinct from unquoted id, so the parser must reject it.
func TestBuildMoviesEmbeddingArtifactFromSeedRejectsWhitespaceMismatchedQuotedColumnIdentifier(t *testing.T) {
	const quotedColumnsSeed = `INSERT INTO movies ("id ", "slug", "title", "overview", "release_year", "genres", "embedding")
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	_, err := BuildMoviesEmbeddingArtifactFromSeed(quotedColumnsSeed)
	if err == nil {
		t.Fatal("expected whitespace-mismatched quoted identifier \"id \" to be rejected")
	}
	if !strings.Contains(err.Error(), "required column 'id'") {
		t.Fatalf("error %q should mention missing required id column", err.Error())
	}
}

// Quoted table identifiers preserve interior whitespace in PostgreSQL.
// "movies " should not match canonical unquoted movies.
func TestBuildMoviesEmbeddingArtifactFromSeedRejectsWhitespaceMismatchedQuotedTableIdentifier(t *testing.T) {
	const quotedTableSeed = `INSERT INTO "public"."movies " (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	_, err := BuildMoviesEmbeddingArtifactFromSeed(quotedTableSeed)
	if err == nil {
		t.Fatal("expected whitespace-mismatched quoted table \"movies \" to be rejected")
	}
	if !strings.Contains(err.Error(), "no INSERT INTO movies statement found") {
		t.Fatalf("error %q should mention missing movies INSERT", err.Error())
	}
}

// A single quoted identifier can contain dots as identifier bytes in
// PostgreSQL. "archive.movies" is one table name, not schema qualification.
func TestBuildMoviesEmbeddingArtifactFromSeedRejectsQuotedTableNameContainingDot(t *testing.T) {
	const quotedTableSeed = `INSERT INTO "archive.movies" (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'overview',
    2010,
    ARRAY['sci-fi'],
    '[0.91,0.12,0.18]'
  );`
	_, err := BuildMoviesEmbeddingArtifactFromSeed(quotedTableSeed)
	if err == nil {
		t.Fatal("expected quoted table name \"archive.movies\" to be rejected")
	}
	if !strings.Contains(err.Error(), "no INSERT INTO movies statement found") {
		t.Fatalf("error %q should mention missing movies INSERT", err.Error())
	}
}

func TestScanQualifiedIdentifierPartsPreservesQuotedDotNameAsSinglePart(t *testing.T) {
	sql := `INSERT INTO "archive.movies" (id, slug) VALUES ('1', 'x');`
	from := strings.Index(sql, `"archive.movies"`)
	if from < 0 {
		t.Fatal("test setup failed to find quoted identifier")
	}

	parts, next := scanQualifiedIdentifierParts(sql, from)
	if len(parts) != 1 {
		t.Fatalf("expected one identifier part, got %d (%v)", len(parts), parts)
	}
	if parts[0] != `"archive.movies"` {
		t.Fatalf("identifier part = %q, want %q", parts[0], `"archive.movies"`)
	}
	if next <= from {
		t.Fatalf("next index should advance past identifier: from=%d next=%d", from, next)
	}
}

func TestEncodeDecodeMoviesEmbeddingArtifactRoundTrip(t *testing.T) {
	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(testMoviesSeedSQL)
	if err != nil {
		t.Fatalf("BuildMoviesEmbeddingArtifactFromSeed returned error: %v", err)
	}
	artifact.SeedChecksum = ComputeSeedChecksum([]byte(testMoviesSeedSQL))

	encoded, err := EncodeMoviesEmbeddingArtifact(artifact)
	if err != nil {
		t.Fatalf("EncodeMoviesEmbeddingArtifact returned error: %v", err)
	}
	encodedAgain, err := EncodeMoviesEmbeddingArtifact(artifact)
	if err != nil {
		t.Fatalf("second EncodeMoviesEmbeddingArtifact returned error: %v", err)
	}
	if string(encoded) != string(encodedAgain) {
		t.Fatal("encoding should be deterministic across repeated calls")
	}

	decoded, err := DecodeMoviesEmbeddingArtifact(encoded)
	if err != nil {
		t.Fatalf("DecodeMoviesEmbeddingArtifact returned error: %v", err)
	}
	if !reflect.DeepEqual(decoded, artifact) {
		t.Fatalf("decoded artifact mismatch\n got: %+v\nwant: %+v", decoded, artifact)
	}
}

func TestDecodeMoviesEmbeddingArtifactRejectsUnknownVersion(t *testing.T) {
	_, err := DecodeMoviesEmbeddingArtifact([]byte(`{"format_version":999,"records":[]}`))
	if err == nil {
		t.Fatal("expected error for unsupported format version")
	}
}

func TestShouldRebuildMoviesEmbeddingsEnvValue(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "numeric true", value: "1", want: true},
		{name: "word true", value: "true", want: true},
		{name: "word yes", value: "yes", want: true},
		{name: "mixed case and spaces", value: "  YeS  ", want: true},
		{name: "numeric false", value: "0", want: false},
		{name: "word false", value: "false", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldRebuildMoviesEmbeddingsEnvValue(tc.value)
			if got != tc.want {
				t.Fatalf("ShouldRebuildMoviesEmbeddingsEnvValue(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestLoadOrRebuildMoviesEmbeddingArtifactRebuildEmbedsCurrentSeedChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "seed.sql")
	artifactPath := filepath.Join(tmpDir, "embeddings.json")

	if err := os.WriteFile(seedPath, []byte(testMoviesSeedSQL), 0o644); err != nil {
		t.Fatalf("WriteFile(seed) error: %v", err)
	}

	artifact, fromCache, err := LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, "1")
	if err != nil {
		t.Fatalf("LoadOrRebuildMoviesEmbeddingArtifact(rebuild) error: %v", err)
	}
	if fromCache {
		t.Fatal("expected rebuild mode to regenerate artifact")
	}
	wantChecksum := ComputeSeedChecksum([]byte(testMoviesSeedSQL))
	if artifact.SeedChecksum != wantChecksum {
		t.Fatalf("seed checksum = %q, want %q", artifact.SeedChecksum, wantChecksum)
	}

	// The committed file on disk must match the in-memory artifact exactly so a
	// follow-up default-mode load is a true round-trip.
	rewritten, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile(rewritten artifact) error: %v", err)
	}
	redecoded, err := DecodeMoviesEmbeddingArtifact(rewritten)
	if err != nil {
		t.Fatalf("DecodeMoviesEmbeddingArtifact(rewritten) error: %v", err)
	}
	if !reflect.DeepEqual(redecoded, artifact) {
		t.Fatalf("rewritten artifact mismatch\n got: %+v\nwant: %+v", redecoded, artifact)
	}

	// Default mode must now succeed because the checksum matches.
	loaded, fromCache, err := LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, "")
	if err != nil {
		t.Fatalf("LoadOrRebuildMoviesEmbeddingArtifact(default) error: %v", err)
	}
	if !fromCache {
		t.Fatal("expected default mode to consume committed artifact")
	}
	if !reflect.DeepEqual(loaded, artifact) {
		t.Fatalf("default-mode load mismatch\n got: %+v\nwant: %+v", loaded, artifact)
	}
}

// Default mode must refuse to silently serve a committed artifact when its
// seed checksum no longer matches the canonical seed.sql on disk. The previous
// behaviour was a silent stale read, which is exactly the bug this guards.
func TestLoadOrRebuildMoviesEmbeddingArtifactRejectsStaleCachedArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "seed.sql")
	artifactPath := filepath.Join(tmpDir, "embeddings.json")

	if err := os.WriteFile(seedPath, []byte(testMoviesSeedSQL), 0o644); err != nil {
		t.Fatalf("WriteFile(seed) error: %v", err)
	}
	// First, build a valid committed artifact bound to the current seed.
	if _, _, err := LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, "1"); err != nil {
		t.Fatalf("initial rebuild error: %v", err)
	}

	// Now rewrite the seed to a different (still valid) corpus without
	// rebuilding the artifact. Default-mode load must refuse the stale
	// artifact instead of silently returning it.
	const drift = `INSERT INTO movies (id, slug, title, overview, release_year, genres, embedding)
VALUES
  (
    '99999999-9999-9999-9999-999999999999',
    'new-film',
    'New Film',
    'overview',
    2024,
    ARRAY['drama'],
    '[0.50,0.50,0.50]'
  );`
	if err := os.WriteFile(seedPath, []byte(drift), 0o644); err != nil {
		t.Fatalf("WriteFile(drifted seed) error: %v", err)
	}

	_, _, err := LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, "")
	if err == nil {
		t.Fatal("expected default-mode load to refuse stale committed artifact")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Fatalf("error %q should explain that the committed artifact is stale", err.Error())
	}

	// And the rebuild path must heal the mismatch.
	healed, fromCache, err := LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, "1")
	if err != nil {
		t.Fatalf("rebuild-after-drift error: %v", err)
	}
	if fromCache {
		t.Fatal("rebuild mode should not report cache hit")
	}
	if healed.SeedChecksum != ComputeSeedChecksum([]byte(drift)) {
		t.Fatalf("healed checksum = %q, want %q", healed.SeedChecksum, ComputeSeedChecksum([]byte(drift)))
	}
}
