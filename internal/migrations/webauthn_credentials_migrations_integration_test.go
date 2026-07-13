//go:build integration

package migrations_test

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	legacyWebAuthnUserA        = "21111111-1111-1111-1111-111111111111"
	legacyWebAuthnUserB        = "22222222-2222-2222-2222-222222222222"
	legacyDisabledWebAuthnUser = "23333333-3333-3333-3333-333333333333"
)

func TestWebAuthnCredentialsMigrationContract(t *testing.T) {
	ctx := context.Background()

	t.Run("fresh install", func(t *testing.T) {
		resetDB(t, ctx)
		runFullMigrations(t, ctx)

		assertWebAuthnCredentialsSchema(t, ctx)
		assertLegacyWebAuthnCredentialColumnsDropped(t, ctx)
		assertWebAuthnSessionColumnsPresent(t, ctx)
	})

	t.Run("legacy upgrade", func(t *testing.T) {
		resetDB(t, ctx)
		runLegacyWebAuthnMigrations(t, ctx)
		seedLegacyWebAuthnFactors(t, ctx)

		runFullMigrations(t, ctx)

		assertWebAuthnCredentialsSchema(t, ctx)
		assertLegacyWebAuthnCredentialColumnsDropped(t, ctx)
		assertWebAuthnSessionColumnsPresent(t, ctx)
		assertLegacyWebAuthnFactorsBackfilled(t, ctx)
	})
}

func runLegacyWebAuthnMigrations(t *testing.T, ctx context.Context) {
	t.Helper()

	runner := migrations.NewRunnerWithFS(sharedPG.Pool, testutil.DiscardLogger(), migrationFSUpTo(t, 178))
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err := runner.Run(ctx)
	testutil.NoError(t, err)
}

func seedLegacyWebAuthnFactors(t *testing.T, ctx context.Context) {
	t.Helper()

	seedLegacyWebAuthnUser(t, ctx, legacyWebAuthnUserA, "legacy-webauthn-a@example.com")
	seedLegacyWebAuthnUser(t, ctx, legacyWebAuthnUserB, "legacy-webauthn-b@example.com")
	seedLegacyWebAuthnUser(t, ctx, legacyDisabledWebAuthnUser, "legacy-webauthn-disabled@example.com")

	seedLegacyWebAuthnFactor(t, ctx, legacyWebAuthnUserA, true, []byte("credential-a"), []byte("public-key-a"), 11, sql.NullString{
		String: "Work key",
		Valid:  true,
	})
	seedLegacyWebAuthnFactor(t, ctx, legacyWebAuthnUserB, true, []byte("credential-b"), []byte("public-key-b"), 22, sql.NullString{})
	seedLegacyWebAuthnFactor(t, ctx, legacyDisabledWebAuthnUser, false, []byte("credential-disabled"), []byte("public-key-disabled"), 33, sql.NullString{
		String: "Disabled key",
		Valid:  true,
	})
}

func seedLegacyWebAuthnUser(t *testing.T, ctx context.Context, userID, email string) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, email,
	)
	testutil.NoError(t, err)
}

func seedLegacyWebAuthnFactor(
	t *testing.T,
	ctx context.Context,
	userID string,
	enabled bool,
	credentialID []byte,
	publicKey []byte,
	signCount int64,
	displayName sql.NullString,
) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_user_mfa (
		     user_id, method, enabled, webauthn_credential_id, webauthn_public_key,
		     webauthn_sign_count, webauthn_display_name, webauthn_session_data
		 )
		 VALUES ($1, 'webauthn', $2, $3, $4, $5, $6, $7)`,
		userID, enabled, credentialID, publicKey, signCount, displayName, []byte("legacy-session"),
	)
	testutil.NoError(t, err)
}

func assertWebAuthnCredentialsSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	assertWebAuthnCredentialsColumn(t, ctx, "id", "uuid", false, "gen_random_uuid()")
	assertWebAuthnCredentialsColumn(t, ctx, "factor_id", "uuid", false, "")
	assertWebAuthnCredentialsColumn(t, ctx, "credential_id", "bytea", false, "")
	assertWebAuthnCredentialsColumn(t, ctx, "public_key", "bytea", false, "")
	assertWebAuthnCredentialsColumn(t, ctx, "transports", "ARRAY", false, "'{}'::text[]")
	assertWebAuthnCredentialsColumn(t, ctx, "sign_count", "bigint", false, "0")
	assertWebAuthnCredentialsColumn(t, ctx, "display_name", "text", false, "''::text")
	assertWebAuthnCredentialsColumn(t, ctx, "created_at", "timestamp with time zone", false, "now()")
	assertWebAuthnCredentialsColumn(t, ctx, "last_used_at", "timestamp with time zone", true, "")
	assertWebAuthnCredentialsForeignKey(t, ctx)
	assertUniqueKeyPresent(t, ctx, "_ayb_webauthn_credentials", "credential_id")
	assertIndexKeyPresent(t, ctx, "_ayb_webauthn_credentials", "factor_id")
}

func assertWebAuthnCredentialsColumn(
	t *testing.T,
	ctx context.Context,
	columnName string,
	expectedType string,
	expectedNullable bool,
	expectedDefault string,
) {
	t.Helper()

	var dataType string
	var nullable bool
	var defaultExpression *string
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT c.data_type,
		        c.is_nullable = 'YES',
		        pg_get_expr(d.adbin, d.adrelid)
		   FROM information_schema.columns c
		   LEFT JOIN pg_class tbl ON tbl.relname = c.table_name
		   LEFT JOIN pg_namespace n ON n.oid = tbl.relnamespace
		   LEFT JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attname = c.column_name
		   LEFT JOIN pg_attrdef d ON d.adrelid = tbl.oid AND d.adnum = a.attnum
		  WHERE c.table_schema = 'public'
		    AND c.table_name = '_ayb_webauthn_credentials'
		    AND c.column_name = $1
		    AND n.nspname = 'public'`,
		columnName,
	).Scan(&dataType, &nullable, &defaultExpression)
	testutil.NoError(t, err)
	testutil.Equal(t, expectedType, dataType)
	testutil.Equal(t, expectedNullable, nullable)
	if expectedDefault == "" {
		testutil.True(t, defaultExpression == nil || strings.TrimSpace(*defaultExpression) == "",
			"%s should not have a default expression", columnName)
		return
	}
	if defaultExpression == nil {
		t.Fatalf("%s should have default expression %q", columnName, expectedDefault)
	}
	testutil.Equal(t, expectedDefault, strings.TrimSpace(*defaultExpression))
}

func assertWebAuthnCredentialsForeignKey(t *testing.T, ctx context.Context) {
	t.Helper()

	var exists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		       FROM pg_constraint con
		       JOIN pg_class child ON child.oid = con.conrelid
		       JOIN pg_class parent ON parent.oid = con.confrelid
		       JOIN pg_namespace n ON n.oid = child.relnamespace
		       JOIN unnest(con.conkey) WITH ORDINALITY AS ck(attnum, ordinality) ON TRUE
		       JOIN pg_attribute child_attr ON child_attr.attrelid = child.oid AND child_attr.attnum = ck.attnum
		      WHERE n.nspname = 'public'
		        AND child.relname = '_ayb_webauthn_credentials'
		        AND parent.relname = '_ayb_user_mfa'
		        AND con.contype = 'f'
		        AND con.confdeltype = 'c'
		      GROUP BY con.oid
		     HAVING string_agg(child_attr.attname, ',' ORDER BY ck.ordinality) = 'factor_id'
		 )`,
	).Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_webauthn_credentials.factor_id should cascade to _ayb_user_mfa(id)")
}

func assertLegacyWebAuthnFactorsBackfilled(t *testing.T, ctx context.Context) {
	t.Helper()

	var count int
	err := sharedPG.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_webauthn_credentials`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, count)

	assertLegacyWebAuthnCredential(t, ctx, legacyWebAuthnUserA, []byte("credential-a"), []byte("public-key-a"), 11, "Work key")
	assertLegacyWebAuthnCredential(t, ctx, legacyWebAuthnUserB, []byte("credential-b"), []byte("public-key-b"), 22, "")
	assertLegacyDisabledWebAuthnFactorNotBackfilled(t, ctx)
}

func assertLegacyWebAuthnCredential(
	t *testing.T,
	ctx context.Context,
	userID string,
	expectedCredentialID []byte,
	expectedPublicKey []byte,
	expectedSignCount int64,
	expectedDisplayName string,
) {
	t.Helper()

	var credentialID []byte
	var publicKey []byte
	var signCount int64
	var displayName string
	var transportCount int
	var lastUsedAt sql.NullTime
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT c.credential_id, c.public_key, c.sign_count, c.display_name,
		        cardinality(c.transports), c.last_used_at
		   FROM _ayb_webauthn_credentials c
		   JOIN _ayb_user_mfa f ON f.id = c.factor_id
		  WHERE f.user_id = $1`,
		userID,
	).Scan(&credentialID, &publicKey, &signCount, &displayName, &transportCount, &lastUsedAt)
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal(expectedCredentialID, credentialID), "credential_id should match legacy value")
	testutil.True(t, bytes.Equal(expectedPublicKey, publicKey), "public_key should match legacy value")
	testutil.Equal(t, expectedSignCount, signCount)
	testutil.Equal(t, expectedDisplayName, displayName)
	testutil.Equal(t, 0, transportCount)
	testutil.False(t, lastUsedAt.Valid, "last_used_at should be null")
}

func assertLegacyDisabledWebAuthnFactorNotBackfilled(t *testing.T, ctx context.Context) {
	t.Helper()

	var enabled bool
	var credentialRows int
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT f.enabled, COUNT(c.id)
		   FROM _ayb_user_mfa f
		   LEFT JOIN _ayb_webauthn_credentials c ON c.factor_id = f.id
		  WHERE f.user_id = $1 AND f.method = 'webauthn'
		  GROUP BY f.id`,
		legacyDisabledWebAuthnUser,
	).Scan(&enabled, &credentialRows)
	testutil.NoError(t, err)
	testutil.False(t, enabled, "disabled legacy WebAuthn factor should remain disabled")
	testutil.Equal(t, 0, credentialRows)
}

func assertLegacyWebAuthnCredentialColumnsDropped(t *testing.T, ctx context.Context) {
	t.Helper()

	for _, columnName := range []string{
		"webauthn_credential_id",
		"webauthn_public_key",
		"webauthn_sign_count",
		"webauthn_display_name",
	} {
		testutil.False(t, columnExists(t, ctx, "_ayb_user_mfa", columnName),
			"_ayb_user_mfa.%s should be dropped after full migrate-up", columnName)
	}
}

func assertWebAuthnSessionColumnsPresent(t *testing.T, ctx context.Context) {
	t.Helper()

	for _, tableColumn := range []struct {
		table  string
		column string
	}{
		{"_ayb_user_mfa", "webauthn_session_data"},
		{"_ayb_mfa_challenges", "webauthn_session_data"},
		{"_ayb_webauthn_discoverable_challenges", "webauthn_session_data"},
	} {
		testutil.True(t, columnExists(t, ctx, tableColumn.table, tableColumn.column),
			"%s.%s should remain present after full migrate-up", tableColumn.table, tableColumn.column)
	}
}

func columnExists(t *testing.T, ctx context.Context, tableName, columnName string) bool {
	t.Helper()

	var exists bool
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		       FROM information_schema.columns
		      WHERE table_schema = 'public'
		        AND table_name = $1
		        AND column_name = $2
		 )`,
		tableName,
		columnName,
	).Scan(&exists)
	testutil.NoError(t, err)
	return exists
}
