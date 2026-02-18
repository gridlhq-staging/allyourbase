package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

type ctxKey struct{}

// RequireAuth returns middleware that rejects requests without a valid JWT or API key.
func RequireAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r)
			if !ok {
				httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
					"missing or invalid authorization header",
					"https://allyourbase.io/guide/authentication")
				return
			}

			claims, err := validateTokenOrAPIKey(r.Context(), svc, token)
			if err != nil {
				httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
					"invalid or expired token",
					"https://allyourbase.io/guide/authentication")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns middleware that extracts JWT or API key claims if present
// but does not reject unauthenticated requests.
func OptionalAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := extractBearerToken(r); ok {
				if claims, err := validateTokenOrAPIKey(r.Context(), svc, token); err == nil {
					ctx := context.WithValue(r.Context(), ctxKey{}, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClaimsFromContext retrieves auth claims from the request context.
// Returns nil if no claims are present.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(ctxKey{}).(*Claims)
	return claims
}

// ContextWithClaims returns a new context with the given claims attached.
// This is primarily useful for testing.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, claims)
}

// validateTokenOrAPIKey checks if the token is an API key (ayb_ prefix)
// and validates accordingly, falling back to JWT validation.
func validateTokenOrAPIKey(ctx context.Context, svc *Service, token string) (*Claims, error) {
	if IsAPIKey(token) {
		return svc.ValidateAPIKey(ctx, token)
	}
	return svc.ValidateToken(token)
}

// ErrScopeReadOnly is returned when a readonly API key attempts a write operation.
var ErrScopeReadOnly = errors.New("api key scope does not permit write operations")

// ErrScopeTableDenied is returned when an API key is not allowed to access a table.
var ErrScopeTableDenied = errors.New("api key scope does not permit access to this table")

// CheckWriteScope verifies that the current claims allow write operations.
// Returns nil for JWT tokens (no scope) and full-access API keys.
func CheckWriteScope(claims *Claims) error {
	if claims == nil {
		return nil
	}
	if !claims.IsWriteAllowed() {
		return ErrScopeReadOnly
	}
	return nil
}

// CheckTableScope verifies that the current claims allow access to the given table.
// Returns nil for JWT tokens (no scope) and API keys with no table restrictions.
func CheckTableScope(claims *Claims, table string) error {
	if claims == nil {
		return nil
	}
	if !claims.IsTableAllowed(table) {
		return ErrScopeTableDenied
	}
	return nil
}

func extractBearerToken(r *http.Request) (string, bool) {
	return httputil.ExtractBearerToken(r)
}
