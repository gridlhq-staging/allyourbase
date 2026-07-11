package tenant

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type tenantCtxKey struct{}
type requestConnCtxKey struct{}
type activeSchemaCtxKey struct{}

// RequestConn is a request-scoped database connection that should be preferred
// over the pool when a middleware pins query execution to a specific session.
type RequestConn interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ContextWithTenantID returns a copy of ctx containing tenantID.
func ContextWithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// TenantFromContext extracts the tenant ID from context.
func TenantFromContext(ctx context.Context) string {
	id, _ := ctx.Value(tenantCtxKey{}).(string)
	return id
}

// ContextWithActiveSchema returns a copy of ctx containing the active schema.
func ContextWithActiveSchema(ctx context.Context, schema string) context.Context {
	return context.WithValue(ctx, activeSchemaCtxKey{}, schema)
}

// ActiveSchemaFromContext extracts the active schema from context.
func ActiveSchemaFromContext(ctx context.Context) string {
	schema, ok := ctx.Value(activeSchemaCtxKey{}).(string)
	if !ok || schema == "" {
		return "public"
	}
	return schema
}

// ContextWithRequestConn returns a copy of ctx containing a request-scoped
// database connection.
func ContextWithRequestConn(ctx context.Context, conn RequestConn) context.Context {
	return context.WithValue(ctx, requestConnCtxKey{}, conn)
}

// RequestConnFromContext extracts a request-scoped database connection from ctx.
func RequestConnFromContext(ctx context.Context) RequestConn {
	conn, _ := ctx.Value(requestConnCtxKey{}).(RequestConn)
	return conn
}
