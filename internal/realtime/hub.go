package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/pgnotify"
)

// eventBufferSize is the per-client channel buffer. Events are dropped when full.
const eventBufferSize = 256

// RLSFilteredTenantScope marks clients whose transport applies per-event RLS
// visibility checks after hub delivery. These clients receive candidate events
// from every tenant on subscribed tables; CanSeeRecord remains the security
// boundary before any event reaches the network.
const RLSFilteredTenantScope = "__ayb_rls_filtered__"

const (
	tableEventBusChannel        = "realtime_table_events"
	tableEventBusKind           = "table_event"
	tableEventBusStartTimeout   = 2 * time.Second
	tableEventBusPublishTimeout = 2 * time.Second
	tableEventBusCloseTimeout   = 2 * time.Second

	oauthEventBusChannel        = "realtime_oauth_events"
	oauthEventBusKind           = "oauth_event"
	oauthEventBusPublishTimeout = 2 * time.Second
	oauthEventBusStartTimeout   = 2 * time.Second
	oauthEventBusCloseTimeout   = 2 * time.Second
)

// Event represents a data change on a table.
type Event struct {
	Action string `json:"action"` // "create", "update", "delete"
	Table  string `json:"table"`
	// TenantID scopes delivery to same-tenant subscribers. It is JSON-serialized
	// so a shared pgnotify.Bus carries the same tenant tag across nodes. An empty
	// TenantID is the intentional _ayb_notifications wildcard (see tenantMatches).
	TenantID  string         `json:"tenant_id,omitempty"`
	Record    map[string]any `json:"record"`
	OldRecord map[string]any `json:"old_record,omitempty"` // for UPDATE events
}

// oauthEventBusEnvelope wraps an OAuth event with its target client ID for cross-node delivery.
type oauthEventBusEnvelope struct {
	ClientID string           `json:"clientID"`
	Event    *auth.OAuthEvent `json:"event"`
}

// Hub manages realtime SSE client connections and broadcasts events.
// It is safe for concurrent use.
type Hub struct {
	mu                  sync.RWMutex
	clients             map[string]*Client
	nextID              atomic.Uint64
	dropped             atomic.Uint64
	logger              *slog.Logger
	tableEventBus       *pgnotify.Bus
	tableEventBusCancel context.CancelFunc
	tableEventBusDone   chan error
	oauthEventBusCancel context.CancelFunc
	oauthEventBusDone   chan error
}

// Client represents a connected SSE subscriber.
type Client struct {
	ID      string
	tables  map[string]bool
	filters Filters
	tenant  string // tenant scope; empty means wildcard-only (see tenantMatches)
	events  chan *Event
	oauthCh chan *auth.OAuthEvent // non-nil only for OAuth SSE clients
}

// Events returns a read-only channel of table events for this client.
func (c *Client) Events() <-chan *Event {
	return c.events
}

// Filters returns the column-level filters for this client.
func (c *Client) Filters() Filters {
	return c.filters
}

// OAuthEvents returns a read-only channel of OAuth events, or nil for non-OAuth clients.
func (c *Client) OAuthEvents() <-chan *auth.OAuthEvent {
	return c.oauthCh
}

// NewHub creates a new realtime event hub.
func NewHub(logger *slog.Logger) *Hub {
	return NewHubWithBus(logger, nil)
}

// NewHubWithBus creates a new realtime event hub with optional cross-node table fanout.
func NewHubWithBus(logger *slog.Logger, bus *pgnotify.Bus) *Hub {
	hub := &Hub{
		clients: make(map[string]*Client),
		logger:  logger,
	}
	if bus != nil {
		hub.tableEventBus = bus
		hub.startTableEventBus()
		hub.startOAuthEventBus()
	}
	return hub
}

// Subscribe creates a new client subscribed to the given tables and registers it.
func (h *Hub) Subscribe(tables map[string]bool) *Client {
	return h.SubscribeWithFilter(tables, nil, "")
}

// SubscribeWithFilter creates a new client subscribed to the given tables with
// column-level filters and a tenant scope. An empty tenant delivers only
// wildcard (empty-tenant) events; see tenantMatches for the full truth table.
func (h *Hub) SubscribeWithFilter(tables map[string]bool, filters Filters, tenant string) *Client {
	id := fmt.Sprintf("c%d", h.nextID.Add(1))
	client := &Client{
		ID:      id,
		tables:  tables,
		filters: filters,
		tenant:  tenant,
		events:  make(chan *Event, eventBufferSize),
	}

	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	h.logger.Debug("client subscribed", "id", id, "tables", tables, "filters", filters, "tenant", tenant)
	return client
}

// SubscribeOAuth creates a client for an OAuth SSE flow.
// The client's ID serves as the CSRF state token for the popup flow
// and as the cross-node routing key, so it must be globally unique
// and unpredictable.
func (h *Hub) SubscribeOAuth() *Client {
	id := newOAuthClientID()
	client := &Client{
		ID:      id,
		events:  make(chan *Event, eventBufferSize),
		oauthCh: make(chan *auth.OAuthEvent, 1),
	}

	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	h.logger.Debug("oauth client subscribed", "id", id)
	return client
}

// HasClient returns true if a client with the given ID is connected.
func (h *Hub) HasClient(id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[id]
	return ok
}

// PublishOAuth sends an OAuth event to the specific client identified by clientID.
// Delivers locally first, then emits a cross-node bus message when a bus is available.
func (h *Hub) PublishOAuth(clientID string, event *auth.OAuthEvent) {
	h.deliverLocalOAuthEvent(clientID, event)
	h.publishOAuthEventToBus(clientID, event)
}

// deliverLocalOAuthEvent sends an event to the identified local OAuth client without blocking.
func (h *Hub) deliverLocalOAuthEvent(clientID string, event *auth.OAuthEvent) {
	h.mu.RLock()
	client, ok := h.clients[clientID]
	h.mu.RUnlock()

	if !ok {
		h.logger.Debug("oauth publish: client not found locally", "clientID", clientID)
		return
	}
	if client.oauthCh == nil {
		h.logger.Warn("oauth publish: client is not an oauth client", "clientID", clientID)
		return
	}

	select {
	case client.oauthCh <- event:
		h.logger.Debug("oauth event published", "clientID", clientID)
	default:
		h.logger.Warn("oauth publish: channel full", "clientID", clientID)
	}
}

// publishOAuthEventToBus emits a targeted OAuth event for cross-node delivery.
func (h *Hub) publishOAuthEventToBus(clientID string, event *auth.OAuthEvent) {
	if h.tableEventBus == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), oauthEventBusPublishTimeout)
	defer cancel()

	envelope := &oauthEventBusEnvelope{ClientID: clientID, Event: event}
	err := h.tableEventBus.Publish(ctx, oauthEventBusChannel, oauthEventBusKind, envelope)
	if err == nil {
		return
	}
	if errors.Is(err, pgnotify.ErrPayloadTooLarge) {
		h.logger.Warn("oauth event too large for cross-node fanout", "clientID", clientID, "error", err)
		return
	}
	h.logger.Warn("oauth event cross-node fanout failed", "clientID", clientID, "error", err)
}

// SetTables atomically replaces the subscription tables for a client.
// This enables dynamic subscription changes (e.g. WebSocket clients)
// without unsubscribe/re-subscribe cycling. No-op if the client doesn't exist.
func (h *Hub) SetTables(clientID string, tables map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client, ok := h.clients[clientID]; ok {
		client.tables = tables
	}
}

// SetFilters atomically replaces the subscription filters for a client.
// No-op if the client doesn't exist.
func (h *Hub) SetFilters(clientID string, filters Filters) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client, ok := h.clients[clientID]; ok {
		client.filters = filters
	}
}

// SetTenant atomically updates the tenant scope for a client, so subscribe
// callers (e.g. WebSocket clients authenticating after connect) can attach
// tenant identity without unsubscribe/re-subscribe churn. No-op if the client
// doesn't exist. See tenantMatches for how the tenant scope gates delivery.
func (h *Hub) SetTenant(clientID, tenant string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client, ok := h.clients[clientID]; ok {
		client.tenant = tenant
	}
}

// Unsubscribe removes a client and closes its event channel(s).
func (h *Hub) Unsubscribe(clientID string) {
	h.mu.Lock()
	client, ok := h.clients[clientID]
	if ok {
		delete(h.clients, clientID)
		close(client.events)
		if client.oauthCh != nil {
			close(client.oauthCh)
		}
	}
	h.mu.Unlock()

	if ok {
		h.logger.Debug("client unsubscribed", "id", clientID)
	}
}

// Publish sends an event to all clients subscribed to the event's table.
// Uses non-blocking sends — events are dropped for clients with full buffers.
func (h *Hub) Publish(event *Event) {
	h.deliverLocalTableEvent(event)
	h.publishTableEventToBus(event)
}

// deliverLocalTableEvent fans an event out to every locally-registered client
// subscribed to the event's table whose tenant scope matches. It is the single
// tenant/table delivery predicate for both local publishes and replayed bus
// messages (see handleTableEventBusMessage).
func (h *Hub) deliverLocalTableEvent(event *Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if !client.tables[event.Table] {
			continue
		}
		if !tenantMatches(event.TenantID, client.tenant) {
			continue
		}
		select {
		case client.events <- event:
		default:
			h.dropped.Add(1)
			h.logger.Warn("client buffer full, dropping event", "clientID", client.ID)
		}
	}
}

// tenantMatches reports whether an event tagged with eventTenant should be
// delivered to a subscriber whose tenant scope is clientTenant. It is the
// Stage-2 tenant-isolation contract, applied identically to local and
// cross-node (bus-replayed) delivery. The truth table:
//
//   - Non-empty eventTenant: delivered only when clientTenant == eventTenant,
//     and suppressed for every other tenant, so tenant-scoped rows never leak
//     to another tenant subscribed to the same table.
//   - Empty eventTenant: the intentional _ayb_notifications wildcard —
//     delivered regardless of the subscriber's tenant.
//   - Empty clientTenant with a non-empty eventTenant: suppressed (fail closed),
//     so an unauthenticated / no-tenant subscriber receives ONLY wildcard
//     (empty-tenant) events and can never see tenant-scoped rows.
func tenantMatches(eventTenant, clientTenant string) bool {
	if clientTenant == RLSFilteredTenantScope {
		return true
	}
	if eventTenant == "" {
		return true
	}
	return clientTenant == eventTenant
}

// publishTableEventToBus emits a table event for cross-node fanout.
func (h *Hub) publishTableEventToBus(event *Event) {
	if h.tableEventBus == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), tableEventBusPublishTimeout)
	defer cancel()

	err := h.tableEventBus.Publish(ctx, tableEventBusChannel, tableEventBusKind, event)
	if err == nil {
		return
	}
	if errors.Is(err, pgnotify.ErrPayloadTooLarge) {
		h.logger.Warn("realtime table event too large for cross-node fanout", "table", event.Table, "error", err)
		return
	}
	h.logger.Warn("realtime table event cross-node fanout failed", "table", event.Table, "error", err)
}

func (h *Hub) startTableEventBus() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	h.tableEventBusCancel = cancel
	h.tableEventBusDone = done

	go func() {
		done <- h.tableEventBus.Subscribe(ctx, tableEventBusChannel, h.handleTableEventBusMessage)
	}()
	h.waitForTableEventBus()
}

func (h *Hub) waitForTableEventBus() {
	ctx, cancel := context.WithTimeout(context.Background(), tableEventBusStartTimeout)
	defer cancel()
	if err := h.tableEventBus.WaitForListener(ctx, tableEventBusChannel); err != nil {
		h.logger.Warn("realtime table event bus listener not ready", "error", err)
	}
}

func (h *Hub) handleTableEventBusMessage(kind string, data json.RawMessage) {
	if kind != tableEventBusKind {
		h.logger.Warn("realtime table event bus ignored unexpected kind", "kind", kind)
		return
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		h.logger.Warn("realtime table event bus ignored invalid payload", "error", err)
		return
	}
	h.deliverLocalTableEvent(&event)
}

// startOAuthEventBus starts the cross-node OAuth listener and waits briefly for readiness.
func (h *Hub) startOAuthEventBus() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	h.oauthEventBusCancel = cancel
	h.oauthEventBusDone = done

	go func() {
		done <- h.tableEventBus.Subscribe(ctx, oauthEventBusChannel, h.handleOAuthEventBusMessage)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), oauthEventBusStartTimeout)
	defer waitCancel()
	if err := h.tableEventBus.WaitForListener(waitCtx, oauthEventBusChannel); err != nil {
		h.logger.Warn("realtime oauth event bus listener not ready", "error", err)
	}
}

func (h *Hub) handleOAuthEventBusMessage(kind string, data json.RawMessage) {
	if kind != oauthEventBusKind {
		h.logger.Warn("realtime oauth event bus ignored unexpected kind", "kind", kind)
		return
	}

	var envelope oauthEventBusEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		h.logger.Warn("realtime oauth event bus ignored invalid payload", "error", err)
		return
	}
	h.deliverLocalOAuthEvent(envelope.ClientID, envelope.Event)
}

// stopOAuthEventBus cancels the OAuth listener and waits for its bounded shutdown.
func (h *Hub) stopOAuthEventBus() {
	h.mu.Lock()
	cancel := h.oauthEventBusCancel
	done := h.oauthEventBusDone
	h.oauthEventBusCancel = nil
	h.oauthEventBusDone = nil
	h.mu.Unlock()

	if cancel == nil || done == nil {
		return
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			h.logger.Warn("realtime oauth event bus listener stopped", "error", err)
		}
	case <-time.After(oauthEventBusCloseTimeout):
		h.logger.Warn("realtime oauth event bus listener did not stop before timeout")
	}
}

// Close disconnects all clients and clears the hub.
// Safe to call multiple times.
func (h *Hub) Close() {
	h.stopTableEventBus()
	h.stopOAuthEventBus()

	h.mu.Lock()
	defer h.mu.Unlock()

	for id, client := range h.clients {
		close(client.events)
		if client.oauthCh != nil {
			close(client.oauthCh)
		}
		delete(h.clients, id)
	}
}

// stopTableEventBus cancels the table-event listener and waits for its bounded shutdown.
func (h *Hub) stopTableEventBus() {
	h.mu.Lock()
	cancel := h.tableEventBusCancel
	done := h.tableEventBusDone
	h.tableEventBusCancel = nil
	h.tableEventBusDone = nil
	h.mu.Unlock()

	if cancel == nil || done == nil {
		return
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			h.logger.Warn("realtime table event bus listener stopped", "error", err)
		}
	case <-time.After(tableEventBusCloseTimeout):
		h.logger.Warn("realtime table event bus listener did not stop before timeout")
	}
}

func newOAuthClientID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("oauth client id: %v", err))
	}
	return "oauth_" + hex.EncodeToString(b[:])
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
