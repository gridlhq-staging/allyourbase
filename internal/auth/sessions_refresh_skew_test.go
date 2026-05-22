//go:build integration

package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
)

// TestRefreshTokenRejectsSessionWithPastExpiresAt proves the refresh path
// rejects a session row once the authoritative SQL expiry boundary has moved
// into the past. This path is intentionally SQL-backed (`expires_at > NOW()`)
// rather than the auth service's JWT clock seam.
func TestRefreshTokenRejectsSessionWithPastExpiresAt(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "refresh-skew-past@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	hash := hashRefreshToken(resp.RefreshToken)
	tag, err := sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_sessions
		 SET expires_at = NOW() - INTERVAL '1 second'
		 WHERE token_hash = $1`,
		hash,
	)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(1), tag.RowsAffected())

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	var errResp httputil.ErrorResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	testutil.Equal(t, http.StatusUnauthorized, errResp.Code)
	testutil.Equal(t, "invalid or expired refresh token", errResp.Message)
	testutil.Equal(t, "https://allyourbase.io/guide/authentication", errResp.DocURL)
}

// TestRefreshTokenRejectsSessionAtExactNowBoundary locks in the strict `>`
// comparison in the refresh SQL gate. A row set to `expires_at = NOW()` must
// already be excluded before the handler attempts rotation.
func TestRefreshTokenRejectsSessionAtExactNowBoundary(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "refresh-skew-equality@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	hash := hashRefreshToken(resp.RefreshToken)
	tag, err := sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_sessions
		 SET expires_at = NOW()
		 WHERE token_hash = $1`,
		hash,
	)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(1), tag.RowsAffected())

	// Assert the exact SQL predicate used by RefreshToken already excludes the row
	// at equality, before the HTTP handler turns that miss into a 401 response.
	var activeRows int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM _ayb_sessions
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&activeRows)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, activeRows)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	var errResp httputil.ErrorResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	testutil.Equal(t, http.StatusUnauthorized, errResp.Code)
	testutil.Equal(t, "invalid or expired refresh token", errResp.Message)
	testutil.Equal(t, "https://allyourbase.io/guide/authentication", errResp.DocURL)
}
