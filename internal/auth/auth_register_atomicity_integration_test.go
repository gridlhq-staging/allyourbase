//go:build integration

package auth_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	atomicSignupEmail        = "atomic-signup@example.com"
	atomicSignupUserID       = "00000000-0000-4000-8000-000000000055"
	atomicSignupSchema       = "user-" + atomicSignupUserID
	renamedAuthenticatedRole = "ayb_authenticated_atomic_signup_test"
)

func TestRegisterRollsBackAllStateAfterLaterStepFailure(t *testing.T) {
	testCases := []struct {
		name       string
		inject     func(*testing.T, context.Context)
		newService func() *auth.Service
		wantError  string
	}{
		{
			name: "session insert",
			inject: func(t *testing.T, ctx context.Context) {
				installSignupInsertFailure(t, ctx, "_ayb_sessions")
			},
			newService: newAuthService,
			wantError:  "inserting session",
		},
		{
			name: "tenant insert",
			inject: func(t *testing.T, ctx context.Context) {
				installSignupInsertFailure(t, ctx, "_ayb_tenants")
			},
			newService: newAuthService,
			wantError:  "creating default tenant",
		},
		{
			name:       "schema grant after schema creation",
			inject:     renameAuthenticatedRole,
			newService: newAuthService,
			wantError:  "provisioning tenant schema",
		},
		{
			name: "membership insert",
			inject: func(t *testing.T, ctx context.Context) {
				installSignupInsertFailure(t, ctx, "_ayb_tenant_memberships")
			},
			newService: newAuthService,
			wantError:  "adding default tenant membership",
		},
		{
			name:   "JWT generation",
			inject: func(*testing.T, context.Context) {},
			newService: func() *auth.Service {
				return auth.NewService(
					sharedPG.Pool,
					"",
					time.Hour,
					7*24*time.Hour,
					8,
					testutil.DiscardLogger(),
				)
			},
			wantError: "jwt secret is not configured",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := t.Context()
			resetAndMigrate(t, ctx)
			_, err := sharedPG.Pool.Exec(ctx,
				fmt.Sprintf(
					`ALTER TABLE _ayb_users ALTER COLUMN id SET DEFAULT '%s'::uuid`,
					atomicSignupUserID,
				),
			)
			testutil.NoError(t, err)
			testCase.inject(t, ctx)

			mailer := &captureEmailMailer{}
			service := testCase.newService()
			service.SetMailer(mailer, "TestApp", "http://localhost:8090/api")

			user, accessToken, refreshToken, err := service.Register(ctx, atomicSignupEmail, "password123")
			testutil.ErrorContains(t, err, testCase.wantError)
			testutil.Nil(t, user)
			testutil.Equal(t, "", accessToken)
			testutil.Equal(t, "", refreshToken)
			assertNoAtomicSignupState(t, ctx, mailer)
		})
	}
}

func installSignupInsertFailure(t *testing.T, ctx context.Context, tableName string) {
	t.Helper()
	allowedTables := map[string]bool{
		"_ayb_sessions":           true,
		"_ayb_tenants":            true,
		"_ayb_tenant_memberships": true,
	}
	if !allowedTables[tableName] {
		t.Fatalf("unsupported fail-injection table %q", tableName)
	}

	const functionName = "fail_atomic_signup_insert"
	_, err := sharedPG.Pool.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'injected atomic signup failure';
		END;
		$$;
		CREATE TRIGGER reject_atomic_signup_insert
		BEFORE INSERT ON %s
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, functionName, tableName, functionName))
	testutil.NoError(t, err)
}

func renameAuthenticatedRole(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx,
		`ALTER ROLE ayb_authenticated RENAME TO `+renamedAuthenticatedRole,
	)
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_, restoreErr := sharedPG.Pool.Exec(context.Background(),
			`ALTER ROLE `+renamedAuthenticatedRole+` RENAME TO ayb_authenticated`,
		)
		testutil.NoError(t, restoreErr)
	})
}

func assertNoAtomicSignupState(t *testing.T, ctx context.Context, mailer *captureEmailMailer) {
	t.Helper()
	stateQueries := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "users", query: `SELECT COUNT(*) FROM _ayb_users WHERE email = $1`, args: []any{atomicSignupEmail}},
		{name: "sessions", query: `SELECT COUNT(*) FROM _ayb_sessions`},
		{name: "tenants", query: `SELECT COUNT(*) FROM _ayb_tenants`},
		{name: "memberships", query: `SELECT COUNT(*) FROM _ayb_tenant_memberships`},
		{name: "schema", query: `SELECT COUNT(*) FROM pg_namespace WHERE nspname = $1`, args: []any{atomicSignupSchema}},
		{name: "verifications", query: `SELECT COUNT(*) FROM _ayb_email_verifications`},
	}
	for _, stateQuery := range stateQueries {
		t.Run(stateQuery.name, func(t *testing.T) {
			var count int
			err := sharedPG.Pool.QueryRow(ctx, stateQuery.query, stateQuery.args...).Scan(&count)
			testutil.NoError(t, err)
			testutil.Equal(t, 0, count)
		})
	}

	mailer.mu.Lock()
	sentEmailCount := len(mailer.calls)
	mailer.mu.Unlock()
	testutil.Equal(t, 0, sentEmailCount)
}
