//go:build integration

package tenant

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSchemaProvisionerWithNilPoolIsNoOp(t *testing.T) {
	provisioner := NewSchemaProvisioner(nil, nil)

	testutil.NoError(t, provisioner.ProvisionSchema(t.Context(), "unused"))
	testutil.NoError(t, provisioner.DropSchema(t.Context(), "unused"))
}

func TestCreateTenantRollsBackRowAndSchemaWhenSchemaGrantFails(t *testing.T) {
	setupTenantTestDB(t)
	ctx := t.Context()
	const (
		slug                     = "atomic-schema-failure"
		renamedAuthenticatedRole = "ayb_authenticated_tenant_atomicity_test"
	)

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

	created, err := newTestService().CreateTenant(
		ctx,
		"Atomic Schema Failure",
		slug,
		"schema",
		"free",
		"default",
		nil,
		"",
	)
	testutil.ErrorContains(t, err, "granting schema usage")
	testutil.Nil(t, created)

	var tenantCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_tenants WHERE slug = $1`,
		slug,
	).Scan(&tenantCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, tenantCount)

	var schemaCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pg_namespace WHERE nspname = $1`,
		slug,
	).Scan(&schemaCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, schemaCount)
}
