//go:build integration

package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRevokedSessionStore(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := auth.NewRevokedSessionStore(sharedPG.Pool)
	now := time.Now().UTC().Truncate(time.Second)
	liveExpiresAt := now.Add(time.Hour)
	expiredAt := now.Add(-time.Hour)

	testutil.NoError(t, store.Upsert(ctx, "live-session", liveExpiresAt))
	testutil.NoError(t, store.Upsert(ctx, "expired-session", expiredAt))

	active, err := store.LoadActive(ctx)
	testutil.NoError(t, err)
	assertRevokedSessionIDs(t, active, []string{"live-session"})
	testutil.Equal(t, liveExpiresAt, active[0].ExpiresAt.UTC().Truncate(time.Second))

	deleted, err := store.CleanupExpired(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(1), deleted)

	active, err = store.LoadActive(ctx)
	testutil.NoError(t, err)
	assertRevokedSessionIDs(t, active, []string{"live-session"})

	var expiredCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_revoked_sessions WHERE session_id = 'expired-session'`,
	).Scan(&expiredCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, expiredCount)
}

func assertRevokedSessionIDs(t *testing.T, sessions []auth.RevokedSession, expected []string) {
	t.Helper()

	got := make([]string, 0, len(sessions))
	for _, session := range sessions {
		got = append(got, session.SessionID)
	}
	testutil.Equal(t, len(expected), len(got))
	for i := range expected {
		testutil.Equal(t, expected[i], got[i])
	}
}
