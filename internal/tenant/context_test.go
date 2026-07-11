package tenant

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTenantFromContextReturnsEmptyForMissingTenant(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "", TenantFromContext(context.Background()))
}

func TestContextWithTenantIDRoundTrip(t *testing.T) {
	t.Parallel()
	base := context.Background()
	ctx := ContextWithTenantID(base, "tenant-1")
	testutil.Equal(t, "tenant-1", TenantFromContext(ctx))
}

func TestActiveSchemaFromContextDefaultsToPublic(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "public", ActiveSchemaFromContext(context.Background()))
}

func TestActiveSchemaFromContextDefaultsToPublicForInvalidValues(t *testing.T) {
	t.Parallel()

	emptySchemaCtx := ContextWithActiveSchema(context.Background(), "")
	testutil.Equal(t, "public", ActiveSchemaFromContext(emptySchemaCtx))

	nonStringCtx := context.WithValue(context.Background(), activeSchemaCtxKey{}, 42)
	testutil.Equal(t, "public", ActiveSchemaFromContext(nonStringCtx))
}

func TestContextWithActiveSchemaRoundTrip(t *testing.T) {
	t.Parallel()
	base := ContextWithTenantID(context.Background(), "tenant-1")
	ctx := ContextWithActiveSchema(base, "globex")

	testutil.Equal(t, "globex", ActiveSchemaFromContext(ctx))
	testutil.Equal(t, "tenant-1", TenantFromContext(ctx))
}

func TestContextWithTenantIDEmptyString(t *testing.T) {
	t.Parallel()
	base := context.Background()
	ctx := ContextWithTenantID(base, "")
	testutil.Equal(t, "", TenantFromContext(ctx))
}
