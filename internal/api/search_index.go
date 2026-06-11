// Package api.
package api

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/searchsettings"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const searchIndexNamePrefix = "idx_ayb_search"
const searchIndexHashLength = 10

// RegisterSearchIndexPostReloadHook wires API-owned FTS index maintenance into
// the schema reload lifecycle without making schema import api.
func RegisterSearchIndexPostReloadHook(ch *schema.CacheHolder, pool *pgxpool.Pool, apiCfg config.APIConfig, logger *slog.Logger) {
	if ch == nil || pool == nil {
		return
	}
	ch.RegisterPostReloadHook(func(ctx context.Context, sc *schema.SchemaCache) error {
		return EnsureSearchIndexes(ctx, pool, sc, apiCfg, logger)
	})
}

// EnsureSearchIndexes creates or refreshes API full-text-search expression
// indexes using pool-direct DDL because CREATE/DROP INDEX CONCURRENTLY cannot
// run inside a transaction.
func EnsureSearchIndexes(ctx context.Context, pool *pgxpool.Pool, sc *schema.SchemaCache, apiCfg config.APIConfig, logger *slog.Logger) error {
	if pool == nil || sc == nil {
		return nil
	}
	store := searchsettings.NewStore(pool)
	for _, tbl := range sc.TableList() {
		if tbl.Kind != "table" {
			continue
		}
		settings, err := store.Load(ctx, tbl.Schema, tbl.Name)
		if err != nil {
			return fmt.Errorf("load search settings for %s.%s: %w", tbl.Schema, tbl.Name, err)
		}
		spec, ok, err := buildSearchIndexSpec(tbl, apiCfg.TextSearchConfig, settings)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := dropStaleSearchIndexes(ctx, pool, tbl, spec.namePrefix, spec.name); err != nil {
			return err
		}
		if err := rebuildInvalidSearchIndex(ctx, pool, tbl.Schema, spec.name); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, spec.createSQL); err != nil {
			return fmt.Errorf("creating search index %s on %s.%s: %w", spec.name, tbl.Schema, tbl.Name, err)
		}
		if logger != nil {
			logger.Debug("ensured search index", "schema", tbl.Schema, "table", tbl.Name, "index", spec.name)
		}
	}
	return nil
}

type searchIndexSpec struct {
	name       string
	namePrefix string
	createSQL  string
}

func buildSearchIndexSpec(tbl *schema.Table, textSearchConfig string, settings searchsettings.Settings) (searchIndexSpec, bool, error) {
	normalizedSettings, err := searchsettings.ValidateForTable(tbl, settings)
	if err != nil {
		return searchIndexSpec{}, false, err
	}
	settings = normalizedSettings
	cols := textColumns(tbl)
	if len(cols) == 0 {
		return searchIndexSpec{}, false, nil
	}
	regConfig, err := buildSearchRegConfigSQL(effectiveSearchOptions(searchOptions{textSearchConfig: textSearchConfig}).textSearchConfig)
	if err != nil {
		return searchIndexSpec{}, false, err
	}
	indexExpr, err := buildSearchVectorExpression(cols, regConfig, settings)
	if err != nil {
		return searchIndexSpec{}, false, err
	}
	namePrefix := truncateSearchIndexNamePrefix(buildSearchIndexNamePrefix(tbl))
	name := buildSearchIndexName(namePrefix, tbl, indexExpr)
	createSQL := fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s USING gin (%s)`,
		sqlutil.QuoteIdent(name),
		sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name),
		"("+indexExpr+")",
	)
	return searchIndexSpec{name: name, namePrefix: namePrefix, createSQL: createSQL}, true, nil
}

func dropStaleSearchIndexes(ctx context.Context, pool *pgxpool.Pool, tbl *schema.Table, namePrefix, currentName string) error {
	for _, idx := range tbl.Indexes {
		if !strings.HasPrefix(idx.Name, namePrefix) || idx.Name == currentName {
			continue
		}
		if _, err := pool.Exec(ctx, `DROP INDEX CONCURRENTLY IF EXISTS `+sqlutil.QuoteQualifiedName(tbl.Schema, idx.Name)); err != nil {
			return fmt.Errorf("dropping stale search index %s on %s.%s: %w", idx.Name, tbl.Schema, tbl.Name, err)
		}
	}
	return nil
}

func rebuildInvalidSearchIndex(ctx context.Context, pool *pgxpool.Pool, schemaName, indexName string) error {
	var valid bool
	err := pool.QueryRow(ctx, `
		SELECT i.indisvalid
		FROM pg_index i
		  JOIN pg_class ic ON ic.oid = i.indexrelid
		  JOIN pg_namespace n ON n.oid = ic.relnamespace
	WHERE n.nspname = $1 AND ic.relname = $2`, schemaName, indexName).Scan(&valid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("checking search index %s.%s validity: %w", schemaName, indexName, err)
	}
	if valid {
		return nil
	}
	if _, err := pool.Exec(ctx, `DROP INDEX CONCURRENTLY IF EXISTS `+sqlutil.QuoteQualifiedName(schemaName, indexName)); err != nil {
		return fmt.Errorf("dropping invalid search index %s.%s: %w", schemaName, indexName, err)
	}
	return nil
}

func buildSearchIndexNamePrefix(tbl *schema.Table) string {
	return searchIndexNamePrefix + "_" + sanitizeSearchIndexNamePart(tbl.Schema) + "_" + sanitizeSearchIndexNamePart(tbl.Name) + "_"
}

func buildSearchIndexName(prefix string, tbl *schema.Table, indexExpr string) string {
	sum := sha1.Sum([]byte(tbl.Schema + "." + tbl.Name + "\x00" + indexExpr))
	hash := hex.EncodeToString(sum[:])[:searchIndexHashLength]
	return prefix + hash
}

func truncateSearchIndexNamePrefix(prefix string) string {
	maxPrefixLen := 63 - searchIndexHashLength
	if len(prefix) > maxPrefixLen {
		return prefix[:maxPrefixLen]
	}
	return prefix
}

func sanitizeSearchIndexNamePart(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if isSearchIndexNameASCII(r) {
			b.WriteByte(byte(r))
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "unnamed"
	}
	return b.String()
}

func isSearchIndexNameASCII(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}
