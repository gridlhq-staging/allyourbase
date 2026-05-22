package vector

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const MoviesEmbeddingArtifactFormatVersion = 2

// MovieEmbeddingRecord is one row of the canonical movies embedding artifact.
type MovieEmbeddingRecord struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Embedding []float64 `json:"embedding"`
}

// MoviesEmbeddingArtifact is the committed, deterministic projection of the
// movies seed used by later demo stages. SeedChecksum binds the artifact to
// the exact seed bytes it was generated from, so a stale committed artifact
// cannot silently be consumed after the seed changes.
type MoviesEmbeddingArtifact struct {
	FormatVersion int                    `json:"format_version"`
	SeedChecksum  string                 `json:"seed_checksum"`
	Records       []MovieEmbeddingRecord `json:"records"`
}

// ComputeSeedChecksum returns the canonical checksum used to bind an artifact
// to a seed file. The checksum is computed on raw bytes so any change to
// `seed.sql` (formatting included) invalidates the cached artifact and forces
// a rebuild.
func ComputeSeedChecksum(seedBytes []byte) string {
	sum := sha256.Sum256(seedBytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// BuildMoviesEmbeddingArtifactFromSeed parses the canonical movies seed SQL
// into a deterministic artifact. It tolerates SQL doubled single-quote
// escapes inside titles or overviews and resolves columns by their position in the
// `INSERT INTO movies (...)` column list rather than by hardcoded tuple
// offsets, so adding or reordering non-embedding columns will not break the
// rebuild path.
func BuildMoviesEmbeddingArtifactFromSeed(seedSQL string) (MoviesEmbeddingArtifact, error) {
	moviesInsert, err := extractInsertStatementForTable(seedSQL, "movies")
	if err != nil {
		return MoviesEmbeddingArtifact{}, err
	}

	cols, err := parseInsertColumns(moviesInsert)
	if err != nil {
		return MoviesEmbeddingArtifact{}, err
	}
	idIdx, ok := columnIndex(cols, "id")
	if !ok {
		return MoviesEmbeddingArtifact{}, errors.New("seed INSERT is missing required column 'id'")
	}
	slugIdx, ok := columnIndex(cols, "slug")
	if !ok {
		return MoviesEmbeddingArtifact{}, errors.New("seed INSERT is missing required column 'slug'")
	}
	titleIdx, ok := columnIndex(cols, "title")
	if !ok {
		return MoviesEmbeddingArtifact{}, errors.New("seed INSERT is missing required column 'title'")
	}
	embeddingIdx, ok := columnIndex(cols, "embedding")
	if !ok {
		return MoviesEmbeddingArtifact{}, errors.New("seed INSERT is missing required column 'embedding'")
	}

	tuples, err := extractValuesTuples(moviesInsert)
	if err != nil {
		return MoviesEmbeddingArtifact{}, err
	}
	if len(tuples) == 0 {
		return MoviesEmbeddingArtifact{}, errors.New("no movie seed tuples found")
	}

	records := make([]MovieEmbeddingRecord, 0, len(tuples))
	for _, tuple := range tuples {
		fields := splitTopLevelFields(tuple)
		if len(fields) != len(cols) {
			return MoviesEmbeddingArtifact{}, fmt.Errorf(
				"seed tuple has %d fields, expected %d to match column list",
				len(fields), len(cols))
		}
		id, ok := unquoteSQLString(fields[idIdx])
		if !ok {
			return MoviesEmbeddingArtifact{}, fmt.Errorf("id field is not a SQL string literal: %q", fields[idIdx])
		}
		slug, ok := unquoteSQLString(fields[slugIdx])
		if !ok {
			return MoviesEmbeddingArtifact{}, fmt.Errorf("slug field is not a SQL string literal: %q", fields[slugIdx])
		}
		title, ok := unquoteSQLString(fields[titleIdx])
		if !ok {
			return MoviesEmbeddingArtifact{}, fmt.Errorf("title field is not a SQL string literal: %q", fields[titleIdx])
		}
		embedding, err := parseSeedEmbeddingLiteral(fields[embeddingIdx])
		if err != nil {
			return MoviesEmbeddingArtifact{}, fmt.Errorf("parsing embedding for slug %q: %w", slug, err)
		}
		records = append(records, MovieEmbeddingRecord{
			ID:        id,
			Slug:      slug,
			Title:     title,
			Embedding: embedding,
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Slug < records[j].Slug })

	return MoviesEmbeddingArtifact{
		FormatVersion: MoviesEmbeddingArtifactFormatVersion,
		Records:       records,
	}, nil
}

// EncodeMoviesEmbeddingArtifact produces canonical bytes for the artifact.
// Output is sorted by slug, indented with two spaces, and HTML-escape free so
// committing the result yields byte-stable diffs across runs.
func EncodeMoviesEmbeddingArtifact(artifact MoviesEmbeddingArtifact) ([]byte, error) {
	if artifact.FormatVersion != MoviesEmbeddingArtifactFormatVersion {
		return nil, fmt.Errorf("unsupported format version %d", artifact.FormatVersion)
	}

	records := append([]MovieEmbeddingRecord(nil), artifact.Records...)
	sort.Slice(records, func(i, j int) bool { return records[i].Slug < records[j].Slug })
	toEncode := MoviesEmbeddingArtifact{
		FormatVersion: artifact.FormatVersion,
		SeedChecksum:  artifact.SeedChecksum,
		Records:       records,
	}

	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(toEncode); err != nil {
		return nil, fmt.Errorf("encoding artifact JSON: %w", err)
	}
	return out.Bytes(), nil
}

// DecodeMoviesEmbeddingArtifact validates the format version on read so older
// artifacts cannot silently be consumed against new code.
func DecodeMoviesEmbeddingArtifact(data []byte) (MoviesEmbeddingArtifact, error) {
	var artifact MoviesEmbeddingArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return MoviesEmbeddingArtifact{}, fmt.Errorf("decoding artifact JSON: %w", err)
	}
	if artifact.FormatVersion != MoviesEmbeddingArtifactFormatVersion {
		return MoviesEmbeddingArtifact{}, fmt.Errorf("unsupported format version %d", artifact.FormatVersion)
	}
	return artifact, nil
}

// LoadOrRebuildMoviesEmbeddingArtifact is the single entrypoint demos and
// later stages call to obtain the deterministic movies artifact. In default
// mode it loads the committed artifact and refuses to return a stale one
// (seed checksum must match current `seed.sql`). In rebuild mode it
// regenerates the artifact and rewrites the committed file.
func LoadOrRebuildMoviesEmbeddingArtifact(seedPath, artifactPath, rebuildEnvValue string) (MoviesEmbeddingArtifact, bool, error) {
	seedBytes, err := os.ReadFile(seedPath)
	if err != nil {
		return MoviesEmbeddingArtifact{}, false, fmt.Errorf("reading seed SQL %q: %w", seedPath, err)
	}
	seedChecksum := ComputeSeedChecksum(seedBytes)

	if !shouldRebuildArtifact(rebuildEnvValue) {
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			return MoviesEmbeddingArtifact{}, false, fmt.Errorf("reading artifact %q: %w", artifactPath, err)
		}
		artifact, err := loadCommittedMoviesEmbeddingArtifact(data, seedChecksum)
		if err != nil {
			return MoviesEmbeddingArtifact{}, false, fmt.Errorf("loading committed artifact %q: %w", artifactPath, err)
		}
		return artifact, true, nil
	}

	artifact, err := BuildMoviesEmbeddingArtifactFromSeed(string(seedBytes))
	if err != nil {
		return MoviesEmbeddingArtifact{}, false, err
	}
	artifact.SeedChecksum = seedChecksum
	encoded, err := EncodeMoviesEmbeddingArtifact(artifact)
	if err != nil {
		return MoviesEmbeddingArtifact{}, false, err
	}
	if err := os.WriteFile(artifactPath, encoded, 0o644); err != nil {
		return MoviesEmbeddingArtifact{}, false, fmt.Errorf("writing rebuilt artifact %q: %w", artifactPath, err)
	}
	return artifact, false, nil
}

// LoadCommittedMoviesEmbeddingArtifact validates and decodes committed
// artifact bytes against the current seed bytes. This is shared by both
// embedded asset reads (examples.FS) and disk-based readers.
func LoadCommittedMoviesEmbeddingArtifact(seedBytes, artifactBytes []byte) (MoviesEmbeddingArtifact, error) {
	return loadCommittedMoviesEmbeddingArtifact(artifactBytes, ComputeSeedChecksum(seedBytes))
}

// ShouldRebuildMoviesEmbeddingsEnvValue is the single rebuild-mode parser for
// AYB_MOVIES_REBUILD_EMBEDDINGS across vector and demo entrypoints.
func ShouldRebuildMoviesEmbeddingsEnvValue(rebuildEnvValue string) bool {
	value := strings.TrimSpace(strings.ToLower(rebuildEnvValue))
	return value == "1" || value == "true" || value == "yes"
}

func shouldRebuildArtifact(rebuildEnvValue string) bool {
	return ShouldRebuildMoviesEmbeddingsEnvValue(rebuildEnvValue)
}

// parseSeedEmbeddingLiteral accepts a seed embedding field like
// `'[0.91,0.12,0.18]'` and returns the parsed float slice.
func parseSeedEmbeddingLiteral(field string) ([]float64, error) {
	baseLiteral := strings.TrimSpace(field)
	if castIdx := strings.Index(baseLiteral, "::"); castIdx >= 0 {
		baseLiteral = strings.TrimSpace(baseLiteral[:castIdx])
	}

	inner, ok := unquoteSQLString(baseLiteral)
	if !ok {
		return nil, fmt.Errorf("embedding field is not a SQL string literal: %q", field)
	}
	inner = strings.TrimSpace(inner)
	if !strings.HasPrefix(inner, "[") || !strings.HasSuffix(inner, "]") {
		return nil, fmt.Errorf("embedding literal must be bracketed, got %q", inner)
	}
	body := strings.TrimSpace(inner[1 : len(inner)-1])
	if body == "" {
		return nil, errors.New("embedding vector is empty")
	}
	parts := strings.Split(body, ",")
	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float %q", trimmed)
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("embedding vector is empty")
	}
	return values, nil
}

func loadCommittedMoviesEmbeddingArtifact(artifactBytes []byte, expectedSeedChecksum string) (MoviesEmbeddingArtifact, error) {
	artifact, err := DecodeMoviesEmbeddingArtifact(artifactBytes)
	if err != nil {
		return MoviesEmbeddingArtifact{}, err
	}
	if artifact.SeedChecksum != expectedSeedChecksum {
		return MoviesEmbeddingArtifact{}, fmt.Errorf(
			"committed artifact is stale: seed checksum %s does not match current seed %s; rerun with AYB_MOVIES_REBUILD_EMBEDDINGS=1",
			artifact.SeedChecksum, expectedSeedChecksum)
	}
	return artifact, nil
}

// parseInsertColumns extracts the column-name list from the first
// `INSERT INTO <table> (col1, col2, ...)` clause it finds in the seed SQL.
// Column names are returned lowercased so callers can match without caring
// about source-file casing.
func parseInsertColumns(sql string) ([]string, error) {
	start := indexOfKeyword(sql, "INSERT")
	if start < 0 {
		return nil, errors.New("no INSERT statement found in seed SQL")
	}
	intoIdx := indexOfKeywordAfter(sql, "INTO", start)
	if intoIdx < 0 {
		return nil, errors.New("INSERT clause missing INTO keyword")
	}
	// After INTO we expect the table identifier, then '('.
	i := intoIdx + len("INTO")
	for i < len(sql) && sql[i] != '(' {
		i++
	}
	if i >= len(sql) {
		return nil, errors.New("INSERT clause missing column list")
	}
	depth := 1
	listStart := i + 1
	i++
	for i < len(sql) && depth > 0 {
		switch sql[i] {
		case '\'':
			i = skipSQLString(sql, i)
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				cols := splitTopLevelFields(sql[listStart:i])
				lowered := make([]string, len(cols))
				for j, c := range cols {
					lowered[j] = normalizeSQLIdentifier(c)
				}
				return lowered, nil
			}
		}
		i++
	}
	return nil, errors.New("INSERT column list is unterminated")
}

// extractValuesTuples returns each tuple body from the first VALUES clause in
// the SQL, honouring nested parens and SQL `”` string escapes.
func extractValuesTuples(sql string) ([]string, error) {
	valuesIdx := indexOfKeyword(sql, "VALUES")
	if valuesIdx < 0 {
		return nil, errors.New("no VALUES clause found in seed SQL")
	}
	i := valuesIdx + len("VALUES")

	var tuples []string
	for i < len(sql) {
		for i < len(sql) && (sql[i] == ' ' || sql[i] == '\n' || sql[i] == '\t' || sql[i] == '\r' || sql[i] == ',') {
			i++
		}
		if i >= len(sql) || sql[i] != '(' {
			break
		}
		depth := 1
		start := i + 1
		i++
		for i < len(sql) && depth > 0 {
			switch sql[i] {
			case '\'':
				i = skipSQLString(sql, i)
				continue
			case '(':
				depth++
				i++
			case ')':
				depth--
				if depth == 0 {
					tuples = append(tuples, sql[start:i])
				}
				i++
			default:
				i++
			}
		}
		if depth != 0 {
			return nil, errors.New("unbalanced parentheses in VALUES clause")
		}
	}
	return tuples, nil
}

// splitTopLevelFields splits a tuple body on top-level commas, respecting
// nested parens/brackets and SQL string literals.
func splitTopLevelFields(body string) []string {
	var fields []string
	depth := 0
	start := 0
	i := 0
	for i < len(body) {
		switch body[i] {
		case '\'':
			i = skipSQLString(body, i)
			continue
		case '(', '[':
			depth++
			i++
		case ')', ']':
			depth--
			i++
		case ',':
			if depth == 0 {
				fields = append(fields, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
			i++
		default:
			i++
		}
	}
	fields = append(fields, strings.TrimSpace(body[start:]))
	return fields
}

// skipSQLString advances past a single-quoted SQL string starting at index i
// (which must point at the opening quote) and returns the index of the first
// character after the closing quote. `”` inside the string is treated as an
// embedded single quote per SQL.
func skipSQLString(s string, i int) int {
	i++ // past opening quote
	for i < len(s) {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

// unquoteSQLString returns the contents of a SQL string literal with `”`
// escapes resolved. ok=false if the input is not a quoted string.
func unquoteSQLString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}
	inner := s[1 : len(s)-1]
	return strings.ReplaceAll(inner, "''", "'"), true
}

// indexOfKeyword finds a SQL keyword (case-insensitive, word-aligned) outside
// of any string literal. Returns -1 if not found.
func indexOfKeyword(s, keyword string) int {
	return indexOfKeywordAfter(s, keyword, 0)
}

func indexOfKeywordAfter(s, keyword string, from int) int {
	upperKW := strings.ToUpper(keyword)
	kwLen := len(upperKW)
	i := from
	for i < len(s) {
		if s[i] == '\'' {
			i = skipSQLString(s, i)
			continue
		}
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			// SQL line comment — skip to newline so keywords inside comments
			// can't fool the scanner.
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if i+kwLen <= len(s) && strings.EqualFold(s[i:i+kwLen], upperKW) {
			// Ensure word alignment (avoid matching INSERTINTO inside an identifier).
			leftOK := i == 0 || !isIdentByte(s[i-1])
			rightOK := i+kwLen == len(s) || !isIdentByte(s[i+kwLen])
			if leftOK && rightOK {
				return i
			}
		}
		i++
	}
	return -1
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

func columnIndex(cols []string, name string) (int, bool) {
	target := strings.ToLower(name)
	for i, c := range cols {
		if c == target {
			return i, true
		}
	}
	return -1, false
}
