package sbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// migrateAuthUsers copies user accounts from the source auth.users table to the target _ayb_users table, excluding anonymous and deleted users unless configured otherwise.
func (m *Migrator) migrateAuthUsers(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Auth users", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0) // unknown count until we query
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating auth users...")

	query, err := m.authUsersSelectQuery(ctx)
	if err != nil {
		return err
	}

	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying auth.users: %w", err)
	}
	defer rows.Close()

	if err := m.copyAuthUsers(ctx, tx, phase, rows); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, m.stats.Users, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d users migrated (%d skipped)\n", m.stats.Users, m.stats.Skipped)
	return nil
}

func (m *Migrator) authUsersSelectQuery(ctx context.Context) (string, error) {
	hasIsAnonymous, err := m.sourceColumnExists(ctx, "auth", "users", "is_anonymous")
	if err != nil {
		return "", err
	}
	hasDeletedAt, err := m.sourceColumnExists(ctx, "auth", "users", "deleted_at")
	if err != nil {
		return "", err
	}
	hasEmailConfirmedAt, err := m.sourceColumnExists(ctx, "auth", "users", "email_confirmed_at")
	if err != nil {
		return "", err
	}
	hasConfirmedAt, err := m.sourceColumnExists(ctx, "auth", "users", "confirmed_at")
	if err != nil {
		return "", err
	}
	hasRawUserMetaData, err := m.sourceColumnExists(ctx, "auth", "users", "raw_user_meta_data")
	if err != nil {
		return "", err
	}
	hasRawAppMetaData, err := m.sourceColumnExists(ctx, "auth", "users", "raw_app_meta_data")
	if err != nil {
		return "", err
	}

	confirmedAtExpr := "NULL::timestamptz"
	if hasEmailConfirmedAt {
		confirmedAtExpr = "email_confirmed_at"
	} else if hasConfirmedAt {
		confirmedAtExpr = "confirmed_at"
	}
	return buildAuthUsersSelectQuery(authUsersSourceColumns{
		includeAnonymous:   m.opts.IncludeAnonymous,
		hasIsAnonymous:     hasIsAnonymous,
		hasDeletedAt:       hasDeletedAt,
		hasRawUserMetaData: hasRawUserMetaData,
		hasRawAppMetaData:  hasRawAppMetaData,
		confirmedAtExpr:    confirmedAtExpr,
	}), nil
}

func (m *Migrator) copyAuthUsers(ctx context.Context, tx *sql.Tx, phase migrate.Phase, rows *sql.Rows) error {
	for rows.Next() {
		var u SupabaseUser
		var emailConfAt sql.NullTime
		var userMetaDataJSON, appMetaDataJSON string

		if err := rows.Scan(
			&u.ID, &u.Email, &u.EncryptedPassword,
			&emailConfAt, &userMetaDataJSON, &appMetaDataJSON,
			&u.CreatedAt, &u.UpdatedAt,
			&u.IsAnonymous,
		); err != nil {
			return fmt.Errorf("scanning user: %w", err)
		}
		if err := parseAuthUserMetadata(&u, userMetaDataJSON, appMetaDataJSON); err != nil {
			return err
		}

		// Skip users without email (phone-only, anonymous).
		if u.Email == "" {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (no email)\n", u.ID)
			}
			continue
		}

		// Skip users without a password hash (OAuth-only users get empty string).
		// They'll still be importable via OAuth identity linking.
		if u.EncryptedPassword == "" {
			// Insert with a placeholder — they can only auth via OAuth or password reset.
			u.EncryptedPassword = "$none$"
		}

		emailVerified := emailConfAt.Valid
		if m.verbose {
			fmt.Fprintf(m.output, "  %s (%s) verified=%v\n", u.Email, u.ID, emailVerified)
		}

		inserted, err := m.insertAuthUser(ctx, tx, u, emailVerified)
		if err != nil {
			return err
		}
		if inserted {
			m.stats.Users++
		}
		m.progress.Progress(phase, m.stats.Users, 0)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func (m *Migrator) insertAuthUser(ctx context.Context, tx *sql.Tx, u SupabaseUser, emailVerified bool) (bool, error) {
	rawUserMetaData, rawAppMetaData, err := encodeAuthUserMetadata(u)
	if err != nil {
		return false, err
	}

	result, err := tx.ExecContext(ctx,
		`INSERT INTO _ayb_users (
			id, email, password_hash, email_verified,
			raw_user_meta_data, raw_app_meta_data, created_at, updated_at
		)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8)
		 ON CONFLICT (id) DO NOTHING`,
		u.ID, strings.ToLower(u.Email), u.EncryptedPassword,
		emailVerified, rawUserMetaData, rawAppMetaData, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("inserting user %s: %w", u.Email, err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

func parseAuthUserMetadata(u *SupabaseUser, userMetaDataJSON, appMetaDataJSON string) error {
	if err := decodeAuthUserMetadata(userMetaDataJSON, &u.RawUserMetaData); err != nil {
		return fmt.Errorf("parsing raw_user_meta_data for user %s: %w", u.ID, err)
	}
	if err := decodeAuthUserMetadata(appMetaDataJSON, &u.RawAppMetaData); err != nil {
		return fmt.Errorf("parsing raw_app_meta_data for user %s: %w", u.ID, err)
	}
	return nil
}

func decodeAuthUserMetadata(payload string, out *map[string]any) error {
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func encodeAuthUserMetadata(u SupabaseUser) (string, string, error) {
	rawUserMetaData, err := json.Marshal(u.RawUserMetaData)
	if err != nil {
		return "", "", fmt.Errorf("encoding raw_user_meta_data for user %s: %w", u.ID, err)
	}
	rawAppMetaData, err := json.Marshal(u.RawAppMetaData)
	if err != nil {
		return "", "", fmt.Errorf("encoding raw_app_meta_data for user %s: %w", u.ID, err)
	}
	return string(rawUserMetaData), string(rawAppMetaData), nil
}

// migrateOAuthIdentities copies OAuth identity records from the source auth.identities table to the target _ayb_oauth_accounts table, excluding the email provider.
func (m *Migrator) migrateOAuthIdentities(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "OAuth", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating OAuth identities...")

	hasIdentityData, err := m.sourceColumnExists(ctx, "auth", "identities", "identity_data")
	if err != nil {
		return err
	}
	hasProviderID, err := m.sourceColumnExists(ctx, "auth", "identities", "provider_id")
	if err != nil {
		return err
	}
	hasCreatedAt, err := m.sourceColumnExists(ctx, "auth", "identities", "created_at")
	if err != nil {
		return err
	}
	hasDeletedAt, err := m.sourceColumnExists(ctx, "auth", "users", "deleted_at")
	if err != nil {
		return err
	}
	query := buildOAuthIdentitiesQuery(hasIdentityData, hasProviderID, hasCreatedAt, hasDeletedAt)
	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("querying auth.identities: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, provider, identityDataJSON string
		var createdAt sql.NullTime

		if err := rows.Scan(&userID, &provider, &identityDataJSON, &createdAt); err != nil {
			return fmt.Errorf("scanning identity: %w", err)
		}

		// Skip the "email" provider — that's password auth, not OAuth.
		if provider == "email" {
			continue
		}

		var identityData map[string]any
		if err := json.Unmarshal([]byte(identityDataJSON), &identityData); err != nil {
			m.stats.Errors = append(m.stats.Errors,
				fmt.Sprintf("parsing identity_data for user %s: %v", userID, err))
			continue
		}

		providerUserID := extractString(identityData, "sub", "provider_id")
		email := extractString(identityData, "email")
		name := extractString(identityData, "name", "full_name")

		if providerUserID == "" {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped identity for user %s (no provider_user_id)\n", userID)
			}
			continue
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s → %s (%s)\n", provider, email, providerUserID)
		}

		created := time.Now()
		if createdAt.Valid {
			created = createdAt.Time
		}

		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (provider, provider_user_id) DO NOTHING`,
			userID, provider, providerUserID, email, name, created,
		)
		if err != nil {
			return fmt.Errorf("inserting OAuth account for user %s: %w", userID, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.OAuthLinks++
		}
		m.progress.Progress(phase, m.stats.OAuthLinks, 0)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, m.stats.OAuthLinks, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d OAuth identities migrated\n", m.stats.OAuthLinks)
	return nil
}

func buildAuthUsersCountQuery(includeAnonymous, hasIsAnonymous, hasDeletedAt bool) string {
	query := `SELECT COUNT(*) FROM auth.users WHERE 1=1`
	if hasDeletedAt {
		query += " AND deleted_at IS NULL"
	}
	if hasIsAnonymous && !includeAnonymous {
		query += " AND (is_anonymous = false OR is_anonymous IS NULL)"
	}
	return query
}

type authUsersSourceColumns struct {
	includeAnonymous   bool
	hasIsAnonymous     bool
	hasDeletedAt       bool
	hasRawUserMetaData bool
	hasRawAppMetaData  bool
	confirmedAtExpr    string
}

// buildAuthUsersSelectQuery constructs a SELECT query for auth users from the source, adapting to schema version differences in column names and applying filters for anonymous and deleted users.
func buildAuthUsersSelectQuery(cols authUsersSourceColumns) string {
	anonymousExpr := "false"
	if cols.hasIsAnonymous {
		anonymousExpr = "COALESCE(is_anonymous, false)"
	}
	if strings.TrimSpace(cols.confirmedAtExpr) == "" {
		cols.confirmedAtExpr = "NULL::timestamptz"
	}
	rawUserMetaDataExpr := authUserMetadataSelectExpr("raw_user_meta_data", cols.hasRawUserMetaData)
	rawAppMetaDataExpr := authUserMetadataSelectExpr("raw_app_meta_data", cols.hasRawAppMetaData)

	query := fmt.Sprintf(`
		SELECT id, COALESCE(email, ''), COALESCE(encrypted_password, ''),
		       %s AS email_confirmed_at,
		       %s,
		       %s,
		       created_at, updated_at,
		       %s AS is_anonymous
		FROM auth.users
		WHERE 1=1`, cols.confirmedAtExpr, rawUserMetaDataExpr, rawAppMetaDataExpr, anonymousExpr)
	if cols.hasDeletedAt {
		query += " AND deleted_at IS NULL"
	}
	if cols.hasIsAnonymous && !cols.includeAnonymous {
		query += " AND (is_anonymous = false OR is_anonymous IS NULL)"
	}
	query += " ORDER BY created_at"
	return query
}

func authUserMetadataSelectExpr(column string, exists bool) string {
	if !exists {
		return "'{}'::text"
	}
	return fmt.Sprintf("COALESCE(%s::text, '{}')", column)
}

// buildOAuthIdentitiesQuery constructs a SELECT query for OAuth identities from the source auth.identities table with conditional extraction of identity_data based on available columns.
func buildOAuthIdentitiesQuery(hasIdentityData, hasProviderID, hasCreatedAt, usersHasDeletedAt bool) string {
	identityDataExpr := `'{}'::text`
	if hasIdentityData {
		identityDataExpr = `COALESCE(i.identity_data::text, '{}')`
	} else if hasProviderID {
		identityDataExpr = `jsonb_build_object(
			'provider_id', COALESCE(i.provider_id::text, ''),
			'sub', COALESCE(i.provider_id::text, '')
		)::text`
	}

	createdAtExpr := "NULL::timestamptz"
	orderByExpr := "i.user_id"
	if hasCreatedAt {
		createdAtExpr = "i.created_at"
		orderByExpr = "i.created_at"
	}
	usersWhere := ""
	if usersHasDeletedAt {
		usersWhere = "WHERE u.deleted_at IS NULL"
	}

	return fmt.Sprintf(`
		SELECT i.user_id, i.provider, %s, %s
		FROM auth.identities i
		JOIN auth.users u ON u.id = i.user_id
		%s
		ORDER BY %s
	`, identityDataExpr, createdAtExpr, usersWhere, orderByExpr)
}
