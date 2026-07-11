package auth

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	tokenRevokeChannel = "token_revoke"
	tokenRevokeKind    = "token_revoke"
)

type RevokedSessionPersistence interface {
	Upsert(ctx context.Context, sessionID string, expiresAt time.Time) error
	LoadActive(ctx context.Context) ([]RevokedSession, error)
	CleanupExpired(ctx context.Context) (int64, error)
}

type TokenRevokeBus interface {
	Publish(ctx context.Context, name string, kind string, data any) error
	Subscribe(ctx context.Context, name string, handler func(kind string, data json.RawMessage)) error
}

type tokenRevokeEvent struct {
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TokenDenyList tracks revoked session IDs for the remaining access-token TTL.
type TokenDenyList struct {
	mu      sync.RWMutex
	entries map[string]time.Time
	store   RevokedSessionPersistence
	bus     TokenRevokeBus
	now     func() time.Time
}

// NewTokenDenyList creates an empty in-memory deny list.
func NewTokenDenyList() *TokenDenyList {
	return &TokenDenyList{
		entries: make(map[string]time.Time),
		now:     time.Now,
	}
}

// Add marks a session as denied until now+ttl.
func (d *TokenDenyList) Add(sessionID string, ttl time.Duration) error {
	return d.AddContext(context.Background(), sessionID, ttl)
}

func (d *TokenDenyList) AddContext(ctx context.Context, sessionID string, ttl time.Duration) error {
	if sessionID == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	expiresAt := d.currentTime().Add(ttl)
	store := d.currentStore()
	if store != nil {
		if err := store.Upsert(ctx, sessionID, expiresAt); err != nil {
			return err
		}
	}
	return d.applyAndPublish(ctx, sessionID, expiresAt)
}

func (d *TokenDenyList) configure(store RevokedSessionPersistence, bus TokenRevokeBus, now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	d.mu.Lock()
	d.store = store
	d.bus = bus
	d.now = now
	d.mu.Unlock()
}

func (d *TokenDenyList) subscribe(ctx context.Context) error {
	bus := d.currentBus()
	if bus == nil {
		return nil
	}
	return bus.Subscribe(ctx, tokenRevokeChannel, d.handleTokenRevokeEvent)
}

func (d *TokenDenyList) handleTokenRevokeEvent(kind string, data json.RawMessage) {
	if kind != tokenRevokeKind {
		return
	}
	var event tokenRevokeEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	d.applyRevokedSession(event.SessionID, event.ExpiresAt)
}

func (d *TokenDenyList) applyRevokedSessions(sessions []RevokedSession) {
	for _, session := range sessions {
		d.applyRevokedSession(session.SessionID, session.ExpiresAt)
	}
}

func (d *TokenDenyList) applyRevokedSession(sessionID string, expiresAt time.Time) {
	if sessionID == "" || !d.currentTime().Before(expiresAt) {
		return
	}
	d.mu.Lock()
	d.entries[sessionID] = expiresAt
	d.mu.Unlock()
}

func (d *TokenDenyList) applyAndPublish(ctx context.Context, sessionID string, expiresAt time.Time) error {
	if sessionID == "" {
		return nil
	}
	d.applyRevokedSession(sessionID, expiresAt)
	bus := d.currentBus()
	if bus == nil {
		return nil
	}
	if err := bus.Publish(ctx, tokenRevokeChannel, tokenRevokeKind, tokenRevokeEvent{
		SessionID: sessionID,
		ExpiresAt: expiresAt,
	}); err != nil && d.currentStore() == nil {
		return err
	}
	return nil
}

// IsDenied returns true when a session is still within its deny window.
// Expired entries are lazily evicted.
func (d *TokenDenyList) IsDenied(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	d.mu.RLock()
	expiresAt, ok := d.entries[sessionID]
	d.mu.RUnlock()
	if !ok {
		return false
	}

	if d.currentTime().Before(expiresAt) {
		return true
	}

	d.mu.Lock()
	// Remove only if the map still points to the same expired timestamp.
	if current, found := d.entries[sessionID]; found && current.Equal(expiresAt) {
		delete(d.entries, sessionID)
	}
	d.mu.Unlock()

	return false
}

// Len returns the number of non-expired entries.
func (d *TokenDenyList) Len() int {
	now := d.currentTime()

	d.mu.Lock()
	defer d.mu.Unlock()

	for sessionID, expiresAt := range d.entries {
		if !now.Before(expiresAt) {
			delete(d.entries, sessionID)
		}
	}
	return len(d.entries)
}

func (d *TokenDenyList) currentTime() time.Time {
	d.mu.RLock()
	now := d.now
	d.mu.RUnlock()
	if now == nil {
		return time.Now()
	}
	return now()
}

func (d *TokenDenyList) currentBus() TokenRevokeBus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.bus
}

func (d *TokenDenyList) currentStore() RevokedSessionPersistence {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.store
}

func (d *TokenDenyList) expiresAtForTest(sessionID string) time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.entries[sessionID]
}
