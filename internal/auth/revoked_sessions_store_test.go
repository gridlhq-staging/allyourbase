package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRevokedSessionStoreNotConfigured(t *testing.T) {
	ctx := context.Background()
	var store *auth.RevokedSessionStore

	err := store.Upsert(ctx, "session-id", time.Now().Add(time.Hour))
	testutil.True(t, errors.Is(err, auth.ErrRevokedSessionStoreNotConfigured))

	sessions, err := store.LoadActive(ctx)
	testutil.True(t, errors.Is(err, auth.ErrRevokedSessionStoreNotConfigured))
	testutil.Equal(t, 0, len(sessions))

	deleted, err := store.CleanupExpired(ctx)
	testutil.True(t, errors.Is(err, auth.ErrRevokedSessionStoreNotConfigured))
	testutil.Equal(t, int64(0), deleted)
}

func TestRevokedSessionStoreUpsertValidation(t *testing.T) {
	ctx := context.Background()
	store := auth.NewRevokedSessionStore(&pgxpool.Pool{})

	err := store.Upsert(ctx, "  ", time.Now().Add(time.Hour))
	testutil.True(t, errors.Is(err, auth.ErrValidation))

	err = store.Upsert(ctx, "session-id", time.Time{})
	testutil.True(t, errors.Is(err, auth.ErrValidation))
}
