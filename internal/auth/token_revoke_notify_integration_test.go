//go:build integration

package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/pgnotify"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestTokenRevokeNotifyDurabilityPaths(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := auth.NewRevokedSessionStore(sharedPG.Pool)
	busA := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	busB := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())

	serviceA := newAuthService()
	serviceB := newAuthService()
	testutil.NoError(t, serviceA.ConfigureSessionRevocation(ctx, auth.RevocationOptions{
		Store:             store,
		Bus:               busA,
		ReconcileInterval: time.Hour,
	}))
	t.Cleanup(serviceA.StopSessionRevocation)
	testutil.NoError(t, serviceB.ConfigureSessionRevocation(ctx, auth.RevocationOptions{
		Store:             store,
		Bus:               busB,
		ReconcileInterval: time.Hour,
	}))
	t.Cleanup(serviceB.StopSessionRevocation)

	user, token, _, err := serviceA.Register(ctx, "notify-revoke@example.com", "password123")
	testutil.NoError(t, err)
	claims, err := serviceA.ValidateToken(token)
	testutil.NoError(t, err)
	sessionID := claims.SessionID

	_, err = serviceB.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.NoError(t, serviceA.RevokeSession(ctx, user.ID, sessionID))
	waitForTokenRevoked(t, serviceB, token)

	_, subscriberReadyToken, _, err := serviceA.Register(ctx, "notify-service-a-ready@example.com", "password123")
	testutil.NoError(t, err)
	subscriberReadyClaims, err := serviceA.ValidateToken(subscriberReadyToken)
	testutil.NoError(t, err)
	testutil.NoError(t, busB.Publish(ctx, "token_revoke", "token_revoke", map[string]any{
		"session_id": subscriberReadyClaims.SessionID,
		"expires_at": time.Now().Add(time.Hour),
	}))
	waitForTokenRevoked(t, serviceA, subscriberReadyToken)

	_, selfEchoToken, _, err := serviceA.Register(ctx, "notify-self-echo@example.com", "password123")
	testutil.NoError(t, err)
	selfEchoClaims, err := serviceA.ValidateToken(selfEchoToken)
	testutil.NoError(t, err)
	testutil.NoError(t, busA.Publish(ctx, "token_revoke", "token_revoke", map[string]any{
		"session_id": selfEchoClaims.SessionID,
		"expires_at": time.Now().Add(time.Hour),
	}))
	waitForTokenStillValid(t, serviceA, selfEchoToken)

	serviceC := newAuthService()
	testutil.NoError(t, serviceC.ConfigureSessionRevocation(ctx, auth.RevocationOptions{
		Store:             store,
		ReconcileInterval: time.Hour,
	}))
	t.Cleanup(serviceC.StopSessionRevocation)
	_, err = serviceC.ValidateToken(token)
	testutil.True(t, errors.Is(err, auth.ErrTokenRevoked), "startup LoadActive should reject revoked token")

	_, stillValidToken, _, err := serviceA.Register(ctx, "notify-expired@example.com", "password123")
	testutil.NoError(t, err)
	stillValidClaims, err := serviceA.ValidateToken(stillValidToken)
	testutil.NoError(t, err)
	expiredAt := time.Now().Add(-time.Minute)
	testutil.NoError(t, busA.Publish(ctx, "token_revoke", "token_revoke", map[string]any{
		"session_id": stillValidClaims.SessionID,
		"expires_at": expiredAt,
	}))
	time.Sleep(150 * time.Millisecond)
	_, err = serviceB.ValidateToken(stillValidToken)
	testutil.NoError(t, err)

	testutil.NoError(t, store.Upsert(ctx, "already-expired", expiredAt))
	serviceD := newAuthService()
	testutil.NoError(t, serviceD.ConfigureSessionRevocation(ctx, auth.RevocationOptions{
		Store:             store,
		ReconcileInterval: time.Hour,
	}))
	t.Cleanup(serviceD.StopSessionRevocation)
	_, err = serviceD.ValidateToken(stillValidToken)
	testutil.NoError(t, err)
}

func waitForTokenRevoked(t *testing.T, service *auth.Service, token string) {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := service.ValidateToken(token)
		if errors.Is(err, auth.ErrTokenRevoked) {
			return
		}
		if err != nil {
			t.Fatalf("validating token while waiting for revoke: %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("timed out waiting for token revoke fanout")
		}
	}
}

func waitForTokenStillValid(t *testing.T, service *auth.Service, token string) {
	t.Helper()

	deadline := time.After(250 * time.Millisecond)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := service.ValidateToken(token)
		if err != nil {
			t.Fatalf("token should remain valid while checking self echo suppression: %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline:
			return
		}
	}
}
