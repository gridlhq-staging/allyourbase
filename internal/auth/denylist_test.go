package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeRevokedSessionStore struct {
	upserts  []fakeRevokedSessionUpsert
	active   []RevokedSession
	cleanups int
}

type fakeRevokedSessionUpsert struct {
	sessionID string
	expiresAt time.Time
}

func (s *fakeRevokedSessionStore) Upsert(ctx context.Context, sessionID string, expiresAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.upserts = append(s.upserts, fakeRevokedSessionUpsert{sessionID: sessionID, expiresAt: expiresAt})
	return nil
}

func (s *fakeRevokedSessionStore) LoadActive(context.Context) ([]RevokedSession, error) {
	return append([]RevokedSession(nil), s.active...), nil
}

func (s *fakeRevokedSessionStore) CleanupExpired(context.Context) (int64, error) {
	s.cleanups++
	return 0, nil
}

type fakeTokenRevokeBus struct {
	published []fakeTokenRevokePublish
	handler   func(kind string, data json.RawMessage)
}

type fakeTokenRevokePublish struct {
	name string
	kind string
	data tokenRevokeEvent
}

func (b *fakeTokenRevokeBus) Publish(_ context.Context, name string, kind string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var event tokenRevokeEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	b.published = append(b.published, fakeTokenRevokePublish{name: name, kind: kind, data: event})
	return nil
}

func (b *fakeTokenRevokeBus) Subscribe(_ context.Context, name string, handler func(kind string, data json.RawMessage)) error {
	if name != tokenRevokeChannel {
		return fmt.Errorf("unexpected channel %q", name)
	}
	b.handler = handler
	return nil
}

func TestTokenDenyListAddIsDenied(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-1", time.Hour)

	testutil.True(t, list.IsDenied("session-1"), "session should be denied after add")
	testutil.Equal(t, 1, list.Len())
}

func TestTokenDenyListZeroAndNegativeTTLNotDenied(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-zero", 0)
	list.Add("session-negative", -time.Second)

	testutil.False(t, list.IsDenied("session-zero"), "zero ttl should not deny")
	testutil.False(t, list.IsDenied("session-negative"), "negative ttl should not deny")
	testutil.Equal(t, 0, list.Len())
}

func TestTokenDenyListTracksEntriesIndependently(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-a", time.Hour)
	list.Add("session-b", time.Hour)

	testutil.True(t, list.IsDenied("session-a"), "session-a should be denied")
	testutil.True(t, list.IsDenied("session-b"), "session-b should be denied")
	testutil.False(t, list.IsDenied("session-c"), "unknown session should not be denied")
	testutil.Equal(t, 2, list.Len())
}

func TestTokenDenyListLazyEvictsExpiredEntryOnRead(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	list.Add("session-expired", 5*time.Millisecond)

	time.Sleep(25 * time.Millisecond)

	testutil.False(t, list.IsDenied("session-expired"), "expired session should not be denied")
	testutil.Equal(t, 0, list.Len())
}

func TestTokenDenyListConcurrentAccess(t *testing.T) {
	t.Parallel()

	list := NewTokenDenyList()
	var wg sync.WaitGroup

	for i := 0; i < 32; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("session-%d", i)
			for j := 0; j < 100; j++ {
				list.Add(id, time.Second)
				_ = list.IsDenied(id)
			}
		}()
	}

	wg.Wait()
	testutil.True(t, list.Len() > 0, "deny list should contain entries")
}

func TestTokenDenyListAddUsesOneAbsoluteExpiryForStoreAndMemory(t *testing.T) {
	t.Parallel()

	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	store := &fakeRevokedSessionStore{}
	list := NewTokenDenyList()
	list.configure(store, nil, func() time.Time { return now })

	testutil.NoError(t, list.Add("session-1", time.Hour))

	testutil.Equal(t, 1, len(store.upserts))
	wantExpiresAt := now.Add(time.Hour)
	testutil.Equal(t, wantExpiresAt, store.upserts[0].expiresAt)
	testutil.Equal(t, wantExpiresAt, list.expiresAtForTest("session-1"))
}

func TestTokenDenyListApplyRevokedSessionsFiltersExpiredAndEmptyRows(t *testing.T) {
	t.Parallel()

	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	list := NewTokenDenyList()
	list.configure(nil, nil, func() time.Time { return now })

	list.applyRevokedSessions([]RevokedSession{
		{SessionID: "session-live-a", ExpiresAt: now.Add(time.Hour)},
		{SessionID: "", ExpiresAt: now.Add(time.Hour)},
		{SessionID: "session-expired", ExpiresAt: now.Add(-time.Second)},
		{SessionID: "session-live-b", ExpiresAt: now.Add(2 * time.Hour)},
	})

	testutil.True(t, list.IsDenied("session-live-a"), "live session a should be denied")
	testutil.True(t, list.IsDenied("session-live-b"), "live session b should be denied")
	testutil.False(t, list.IsDenied("session-expired"), "expired session should be ignored")
	testutil.Equal(t, 2, list.Len())
}

func TestTokenDenyListAddPublishesTokenRevokeEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	bus := &fakeTokenRevokeBus{}
	list := NewTokenDenyList()
	list.configure(nil, bus, func() time.Time { return now })

	testutil.NoError(t, list.Add("session-1", time.Hour))

	testutil.Equal(t, 1, len(bus.published))
	testutil.Equal(t, tokenRevokeChannel, bus.published[0].name)
	testutil.Equal(t, tokenRevokeKind, bus.published[0].kind)
	testutil.Equal(t, "session-1", bus.published[0].data.SessionID)
	testutil.Equal(t, now.Add(time.Hour), bus.published[0].data.ExpiresAt)
}

func TestTokenDenyListInboundTokenRevokeDoesNotRepublish(t *testing.T) {
	t.Parallel()

	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	bus := &fakeTokenRevokeBus{}
	list := NewTokenDenyList()
	list.configure(nil, bus, func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.NoError(t, list.subscribe(ctx))

	raw, err := json.Marshal(tokenRevokeEvent{SessionID: "session-2", ExpiresAt: now.Add(time.Hour)})
	testutil.NoError(t, err)
	bus.handler(tokenRevokeKind, raw)

	testutil.True(t, list.IsDenied("session-2"), "inbound revoke should deny session")
	testutil.Equal(t, 0, len(bus.published))
}

func TestTokenDenyListInboundTokenRevokeIgnoresMalformedAndExpiredEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	bus := &fakeTokenRevokeBus{}
	list := NewTokenDenyList()
	list.configure(nil, bus, func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.NoError(t, list.subscribe(ctx))

	bus.handler(tokenRevokeKind, json.RawMessage(`{"session_id":"","expires_at":"2030-01-02T04:04:05Z"}`))
	bus.handler(tokenRevokeKind, json.RawMessage(`{"session_id":"expired","expires_at":"2030-01-02T02:04:05Z"}`))
	bus.handler("other", json.RawMessage(`{"session_id":"other","expires_at":"2030-01-02T04:04:05Z"}`))
	bus.handler(tokenRevokeKind, json.RawMessage(`{`))

	testutil.Equal(t, 0, list.Len())
}
