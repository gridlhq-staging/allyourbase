//go:build integration

package auth_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGetUserMFAFactors_WebAuthnIgnoresLegacyDisplayName(t *testing.T) {
	ctx := t.Context()
	resetAndRunAuthMigrationsThrough(t, ctx, 180)
	svc := newAuthService()

	userID := registerNamedUser(t, svc, "webauthn-legacy-display@example.com")
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_user_mfa (
		     user_id, method, enabled, enrolled_at, webauthn_display_name, webauthn_session_data
		 )
		 VALUES ($1, 'webauthn', true, NOW(), 'Legacy fallback label', 'legacy-session')`,
		userID,
	)
	testutil.NoError(t, err)

	factors, err := svc.GetUserMFAFactors(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "webauthn", factors[0].Method)
	testutil.Equal(t, "Passkey", factors[0].Label)
	testutil.Equal(t, "", factors[0].DisplayName)
}

func resetAndRunAuthMigrationsThrough(t *testing.T, ctx context.Context, maxNumber int) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunnerWithFS(sharedPG.Pool, testutil.DiscardLogger(), authMigrationFSUpTo(t, maxNumber))
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}

func authMigrationFSUpTo(t *testing.T, maxNumber int) fstest.MapFS {
	t.Helper()

	entries, err := os.ReadDir("../migrations/sql")
	testutil.NoError(t, err)

	filtered := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if authMigrationNumber(t, entry.Name()) > maxNumber {
			continue
		}

		path := "sql/" + entry.Name()
		data, err := os.ReadFile("../migrations/" + path)
		testutil.NoError(t, err)
		filtered[path] = &fstest.MapFile{Data: data}
	}

	return filtered
}

func authMigrationNumber(t *testing.T, name string) int {
	t.Helper()

	prefix := strings.SplitN(name, "_", 2)[0]
	number, err := strconv.Atoi(prefix)
	testutil.NoError(t, err)
	return number
}
