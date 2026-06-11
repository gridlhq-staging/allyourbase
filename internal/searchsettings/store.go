// Package searchsettings Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun09_pm_4_search_relevance_weighting_and_custom_ranking/allyourbase_dev/internal/searchsettings/store.go.
package searchsettings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return Store{pool: pool}
}

func (s Store) Load(ctx context.Context, schemaName, tableName string) (Settings, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		SELECT settings
		FROM _ayb_search_settings
		WHERE schema_name = $1 AND table_name = $2
	`, schemaName, tableName).Scan(&payload)
	if err == pgx.ErrNoRows {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}

	var settings Settings
	if err := json.Unmarshal(payload, &settings); err != nil {
		return Settings{}, err
	}
	return Validate(settings)
}

func (s Store) Save(ctx context.Context, schemaName, tableName string, settings Settings) error {
	normalized, err := Validate(settings)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO _ayb_search_settings (schema_name, table_name, settings)
		VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (schema_name, table_name)
		DO UPDATE SET settings = EXCLUDED.settings
	`, schemaName, tableName, payload)
	return err
}
