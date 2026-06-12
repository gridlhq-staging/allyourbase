// Package searchsettings stores and validates per-table search ranking settings.
package searchsettings

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const saveSettingsSQL = `
	INSERT INTO _ayb_search_settings (schema_name, table_name, settings)
	VALUES ($1, $2, $3::jsonb)
	ON CONFLICT (schema_name, table_name)
	DO UPDATE SET settings = EXCLUDED.settings
`

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
	payload, err := settingsPayload(settings)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, saveSettingsSQL, schemaName, tableName, payload)
	return err
}

// SaveSQLTx saves one collection's search settings inside the caller's tx.
func SaveSQLTx(ctx context.Context, tx *sql.Tx, schemaName, tableName string, settings Settings) error {
	payload, err := settingsPayload(settings)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, saveSettingsSQL, schemaName, tableName, payload)
	return err
}

func settingsPayload(settings Settings) ([]byte, error) {
	normalized, err := Validate(settings)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}
