//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
)

// newWSTestServer creates an integration test server with optional config modifications.
func newWSTestServer(t *testing.T, modifyCfg ...func(*config.Config)) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	for _, fn := range modifyCfg {
		fn(cfg)
	}
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return ts
}

type realtimeWSTenantFixture struct {
	server  *httptest.Server
	tokenA  string
	tokenB  string
	tenantA string
	tenantB string
}

func newAuthRealtimeWSTenantFixture(t *testing.T) realtimeWSTenantFixture {
	t.Helper()
	ctx := context.Background()
	resetPublicSchemaWithUsersAndTenants(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = realtimeTestJWTSecret
	authSvc := auth.NewService(sharedPG.Pool, realtimeTestJWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	tenantSvc := tenant.NewService(sharedPG.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(tenant.NewUsageAccumulator(sharedPG.Pool, logger))
	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})

	tenantA := createActiveSharedTenant(t, ctx, tenantSvc, "ws-tenant-a")
	tenantB := createActiveSharedTenant(t, ctx, tenantSvc, "ws-tenant-b")

	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return realtimeWSTenantFixture{
		server:  ts,
		tokenA:  issueRealtimeTenantTestToken(t, "ws-user-a", tenantA.ID),
		tokenB:  issueRealtimeTenantTestToken(t, "ws-user-b", tenantB.ID),
		tenantA: tenantA.ID,
		tenantB: tenantB.ID,
	}
}

func newAuthRealtimeWSSecureDocsServer(t *testing.T) (*httptest.Server, *auth.Service) {
	t.Helper()
	ctx := context.Background()
	setupSecureDocsRealtimeFixture(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = realtimeTestJWTSecret
	authSvc := auth.NewService(sharedPG.Pool, realtimeTestJWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return ts, authSvc
}

func resetPublicSchemaWithUsersAndTenants(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	ensureIntegrationMigrations(t, ctx)
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email VARCHAR(255) UNIQUE
		);
		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON users TO ayb_authenticated;
		GRANT USAGE, SELECT ON SEQUENCE users_id_seq TO ayb_authenticated;
	`)
	testutil.NoError(t, err)
}

func createActiveSharedTenant(t *testing.T, ctx context.Context, tenantSvc *tenant.Service, slug string) *tenant.Tenant {
	t.Helper()
	ent, err := tenantSvc.CreateTenant(ctx, slug, fmt.Sprintf("%s-%d", slug, time.Now().UnixNano()), "shared", "free", "default", nil, "")
	testutil.NoError(t, err)
	ent, err = tenantSvc.TransitionState(ctx, ent.ID, tenant.TenantStateProvisioning, tenant.TenantStateActive)
	testutil.NoError(t, err)
	return ent
}

// dialWS dials the test server WebSocket endpoint and returns the connection.
func dialWS(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/realtime/ws"
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return wsConn
}

// readWSJSON reads a JSON message from the websocket into a ws.ServerMessage.
func readWSJSON(t *testing.T, wsConn *websocket.Conn) ws.ServerMessage {
	t.Helper()
	// Integration runs against a real Postgres path; allow extra latency to avoid flakes.
	wsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg ws.ServerMessage
	if err := wsConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read ws: %v", err)
	}
	return msg
}

// readWSUntilType reads messages from the websocket, discarding non-matching
// types, until a message of the expected type is received. This handles
// interleaved messages such as auto presence_sync after channel_subscribe
// and presence diff broadcasts between protocol exchanges.
func readWSUntilType(t *testing.T, wsConn *websocket.Conn, msgType string) ws.ServerMessage {
	t.Helper()
	for i := 0; i < 10; i++ {
		msg := readWSJSON(t, wsConn)
		if msg.Type == msgType {
			return msg
		}
	}
	t.Fatalf("did not receive message of type %q within 10 reads", msgType)
	return ws.ServerMessage{}
}

func assertNoWSMessage(t *testing.T, wsConn *websocket.Conn, duration time.Duration, message string) {
	t.Helper()
	testutil.NoError(t, wsConn.SetReadDeadline(time.Now().Add(duration)))
	var msg ws.ServerMessage
	err := wsConn.ReadJSON(&msg)
	testutil.NoError(t, wsConn.SetReadDeadline(time.Time{}))
	if err == nil {
		t.Fatalf("%s: received unexpected WebSocket message: %+v", message, msg)
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return
	}
	t.Fatalf("%s: read ws: %v", message, err)
}

// readWSPresenceSync reads messages until a presence sync message is received,
// discarding interleaved presence diff (join/leave/update) messages.
func readWSPresenceSync(t *testing.T, wsConn *websocket.Conn) ws.ServerMessage {
	t.Helper()
	for i := 0; i < 10; i++ {
		msg := readWSJSON(t, wsConn)
		if msg.Type == ws.MsgTypePresence && msg.PresenceAction == ws.PresenceActionSync {
			return msg
		}
	}
	t.Fatalf("did not receive presence sync within 10 reads")
	return ws.ServerMessage{}
}

// writeWSJSON sends a JSON message to the websocket.
func writeWSJSON(t *testing.T, wsConn *websocket.Conn, msg any) {
	t.Helper()
	data, _ := json.Marshal(msg)
	if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write ws: %v", err)
	}
}

func authenticateWS(t *testing.T, wsConn *websocket.Conn, token, ref string) {
	t.Helper()
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:  ws.MsgTypeAuth,
		Token: token,
		Ref:   ref,
	})
	reply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
	testutil.Equal(t, ws.MsgTypeReply, reply.Type)
	testutil.Equal(t, "ok", reply.Status)
	testutil.Equal(t, ref, reply.Ref)
}

func authenticateWSAfterQuietPeriod(t *testing.T, wsConn *websocket.Conn, token, ref string, duration time.Duration, message string) {
	t.Helper()
	msgCh := make(chan ws.ServerMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		var msg ws.ServerMessage
		if err := wsConn.ReadJSON(&msg); err != nil {
			errCh <- err
			return
		}
		msgCh <- msg
	}()

	select {
	case msg := <-msgCh:
		t.Fatalf("%s: received unexpected WebSocket message before quiet period elapsed: %+v", message, msg)
	case err := <-errCh:
		t.Fatalf("%s: read ws: %v", message, err)
	case <-time.After(duration):
	}

	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:  ws.MsgTypeAuth,
		Token: token,
		Ref:   ref,
	})

	select {
	case reply := <-msgCh:
		testutil.Equal(t, ws.MsgTypeReply, reply.Type)
		testutil.Equal(t, "ok", reply.Status)
		testutil.Equal(t, ref, reply.Ref)
	case err := <-errCh:
		t.Fatalf("read ws auth reply after quiet period: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for auth reply after quiet period")
	}
}

func subscribeWSTable(t *testing.T, wsConn *websocket.Conn, table, ref string) {
	t.Helper()
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{table},
		Ref:    ref,
	})
	reply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
	testutil.Equal(t, ws.MsgTypeReply, reply.Type)
	testutil.Equal(t, "ok", reply.Status)
	testutil.Equal(t, ref, reply.Ref)
}

func assertWSTableEvent(t *testing.T, wsConn *websocket.Conn, action, table string) ws.ServerMessage {
	t.Helper()
	event := readWSUntilType(t, wsConn, ws.MsgTypeEvent)
	testutil.Equal(t, ws.MsgTypeEvent, event.Type)
	testutil.Equal(t, action, event.Action)
	testutil.Equal(t, table, event.Table)
	testutil.True(t, event.Record != nil, "expected record in WebSocket event")
	return event
}

// TestRealtimeWSConnectAuthSubscribeBroadcastPresence verifies the full WebSocket
// E2E flow: connect → subscribe → create record → receive event → broadcast
// with self-receive → presence track → presence sync.
func TestRealtimeWSConnectAuthSubscribeBroadcastPresence(t *testing.T) {
	ts := newWSTestServer(t)

	wsConn := dialWS(t, ts)
	defer wsConn.Close()

	// Read connected message.
	connected := readWSJSON(t, wsConn)
	testutil.Equal(t, ws.MsgTypeConnected, connected.Type)
	testutil.True(t, connected.ClientID != "", "expected non-empty client_id")
	clientID := connected.ClientID

	// Subscribe to users table (no filter — full flow test, not filter test).
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{"users"},
		Ref:    "sub-1",
	})
	reply := readWSJSON(t, wsConn)
	testutil.Equal(t, ws.MsgTypeReply, reply.Type)
	testutil.Equal(t, "ok", reply.Status)
	testutil.Equal(t, "sub-1", reply.Ref)

	// Create a record via API — should trigger realtime event.
	body, _ := json.Marshal(map[string]any{"name": "Alice", "email": "alice@example.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	createResp.Body.Close()

	// Read the create event (async via Postgres notification).
	eventCh := make(chan ws.ServerMessage, 1)
	go func() {
		msg := readWSJSON(t, wsConn)
		eventCh <- msg
	}()

	select {
	case event := <-eventCh:
		testutil.Equal(t, ws.MsgTypeEvent, event.Type)
		testutil.Equal(t, "create", event.Action)
		testutil.Equal(t, "users", event.Table)
		testutil.True(t, event.Record != nil, "expected record in event")
		testutil.Equal(t, "Alice", event.Record["name"])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WebSocket create event")
	}

	// Subscribe to a broadcast channel.
	// handleChannelSubscribe sends replyOK then auto presence_sync.
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:    ws.MsgTypeChannelSubscribe,
		Channel: "test-channel",
		Ref:     "chan-sub-1",
	})
	chanReply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
	testutil.Equal(t, "ok", chanReply.Status)

	// Send a broadcast with self-receive enabled (Self:true) so the single
	// client receives its own relay. Relay is enqueued before replyOK.
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:    ws.MsgTypeBroadcast,
		Channel: "test-channel",
		Event:   "test-event",
		Payload: map[string]any{"message": "hello"},
		Self:    true,
		Ref:     "bc-1",
	})
	// Self-relay arrives before reply (Relay enqueued first in handleBroadcast).
	bcMsg := readWSUntilType(t, wsConn, ws.MsgTypeBroadcast)
	testutil.Equal(t, "test-channel", bcMsg.Channel)
	testutil.Equal(t, "test-event", bcMsg.Event)
	testutil.Equal(t, "hello", bcMsg.Payload["message"])

	bcReply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
	testutil.Equal(t, "ok", bcReply.Status)

	// Subscribe to presence channel (required before presence_track).
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:    ws.MsgTypeChannelSubscribe,
		Channel: "presence-channel",
		Ref:     "chan-sub-2",
	})
	presChReply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
	testutil.Equal(t, "ok", presChReply.Status)

	// Track presence. sendPresenceDiff (join) is broadcast before replyOK.
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:     ws.MsgTypePresenceTrack,
		Channel:  "presence-channel",
		Presence: map[string]any{"status": "online", "name": "Alice"},
		Ref:      "pr-track-1",
	})
	trackReply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
	testutil.Equal(t, "ok", trackReply.Status)

	// Request presence sync — should contain our own presence.
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:    ws.MsgTypePresenceSync,
		Channel: "presence-channel",
		Ref:     "pr-sync-1",
	})
	syncMsg := readWSPresenceSync(t, wsConn)
	testutil.Equal(t, "presence-channel", syncMsg.Channel)
	testutil.True(t, syncMsg.Presences != nil, "expected presences in sync")
	_, hasOwnPresence := syncMsg.Presences[clientID]
	testutil.True(t, hasOwnPresence, "expected own presence in sync")
}

func TestRealtimeWSTenantIsolationAndResubscribeRefresh(t *testing.T) {
	fixture := newAuthRealtimeWSTenantFixture(t)

	wsConn := dialWS(t, fixture.server)
	defer wsConn.Close()
	connected := readWSJSON(t, wsConn)
	testutil.Equal(t, ws.MsgTypeConnected, connected.Type)

	authenticateWS(t, wsConn, fixture.tokenA, "auth-a")
	subscribeWSTable(t, wsConn, "users", "sub-a")

	nameA := fmt.Sprintf("TenantAUser%d", time.Now().UnixNano())
	createdA := decodeAPIRecord(t, apiJSONRequestWithTokenAndTenant(t, http.MethodPost, fixture.server.URL+"/api/collections/users/",
		map[string]any{"name": nameA, "email": strings.ToLower(nameA) + "@example.com"},
		fixture.tokenA,
		fixture.tenantA,
		http.StatusCreated))
	event := assertWSTableEvent(t, wsConn, "create", "users")
	assertRealtimeRecordValue(t, event.Record, "id", createdA["id"])
	assertRealtimeRecordValue(t, event.Record, "name", nameA)
	assertRealtimeRecordValue(t, event.Record, "email", strings.ToLower(nameA)+"@example.com")

	userAID := fmt.Sprint(createdA["id"])
	updatedA := decodeAPIRecord(t, apiJSONRequestWithTokenAndTenant(t, http.MethodPatch, fixture.server.URL+"/api/collections/users/"+userAID,
		map[string]any{"name": nameA + "Updated"},
		fixture.tokenA,
		fixture.tenantA,
		http.StatusOK))
	event = assertWSTableEvent(t, wsConn, "update", "users")
	assertRealtimeRecordValue(t, event.Record, "id", updatedA["id"])
	assertRealtimeRecordValue(t, event.Record, "name", nameA+"Updated")

	apiJSONRequestWithTokenAndTenant(t, http.MethodDelete, fixture.server.URL+"/api/collections/users/"+userAID, nil, fixture.tokenA, fixture.tenantA, http.StatusNoContent)
	event = assertWSTableEvent(t, wsConn, "delete", "users")
	assertRealtimeRecordValue(t, event.Record, "id", userAID)

	nameB := fmt.Sprintf("TenantBUser%d", time.Now().UnixNano())
	createdB := decodeAPIRecord(t, apiJSONRequestWithTokenAndTenant(t, http.MethodPost, fixture.server.URL+"/api/collections/users/",
		map[string]any{"name": nameB, "email": strings.ToLower(nameB) + "@example.com"},
		fixture.tokenB,
		fixture.tenantB,
		http.StatusCreated))
	authenticateWSAfterQuietPeriod(t, wsConn, fixture.tokenB, "auth-b", 250*time.Millisecond, "tenant A socket received tenant B event")
	subscribeWSTable(t, wsConn, "users", "resub-b")

	nameBAfterResub := fmt.Sprintf("TenantBAfterResub%d", time.Now().UnixNano())
	createdBAfterResub := decodeAPIRecord(t, apiJSONRequestWithTokenAndTenant(t, http.MethodPost, fixture.server.URL+"/api/collections/users/",
		map[string]any{"name": nameBAfterResub, "email": strings.ToLower(nameBAfterResub) + "@example.com"},
		fixture.tokenB,
		fixture.tenantB,
		http.StatusCreated))
	event = assertWSTableEvent(t, wsConn, "create", "users")
	assertRealtimeRecordValue(t, event.Record, "id", createdBAfterResub["id"])
	assertRealtimeRecordValue(t, event.Record, "name", nameBAfterResub)

	apiJSONRequestWithTokenAndTenant(t, http.MethodPatch, fixture.server.URL+"/api/collections/users/"+fmt.Sprint(createdB["id"]),
		map[string]any{"name": nameB + "Updated"},
		fixture.tokenB,
		fixture.tenantB,
		http.StatusOK)
	event = assertWSTableEvent(t, wsConn, "update", "users")
	assertRealtimeRecordValue(t, event.Record, "id", createdB["id"])
	assertRealtimeRecordValue(t, event.Record, "name", nameB+"Updated")

	nameAAfterResub := fmt.Sprintf("TenantAAfterResub%d", time.Now().UnixNano())
	apiJSONRequestWithTokenAndTenant(t, http.MethodPost, fixture.server.URL+"/api/collections/users/",
		map[string]any{"name": nameAAfterResub, "email": strings.ToLower(nameAAfterResub) + "@example.com"},
		fixture.tokenA,
		fixture.tenantA,
		http.StatusCreated)
	assertNoWSMessage(t, wsConn, 250*time.Millisecond, "tenant B socket received tenant A event after reauth and resubscribe")
}

func TestRealtimeWSDeleteVisibilityUsesRLS(t *testing.T) {
	ts, authSvc := newAuthRealtimeWSSecureDocsServer(t)
	memberToken := issueRealtimeTestToken(t, authSvc, "user-allowed")
	nonMemberToken := issueRealtimeTestToken(t, authSvc, "user-denied")

	created := decodeAPIRecord(t, apiJSONRequestWithToken(t, http.MethodPost, ts.URL+"/api/collections/secure_docs/",
		map[string]any{"id": "ws-doc-delete-1", "project_id": "project-1", "title": "Delete Visible"},
		memberToken,
		http.StatusCreated))

	memberWS := dialWS(t, ts)
	defer memberWS.Close()
	testutil.Equal(t, ws.MsgTypeConnected, readWSJSON(t, memberWS).Type)
	authenticateWS(t, memberWS, memberToken, "member-auth")
	subscribeWSTable(t, memberWS, "secure_docs", "member-sub")

	nonMemberWS := dialWS(t, ts)
	defer nonMemberWS.Close()
	testutil.Equal(t, ws.MsgTypeConnected, readWSJSON(t, nonMemberWS).Type)
	authenticateWS(t, nonMemberWS, nonMemberToken, "non-member-auth")
	subscribeWSTable(t, nonMemberWS, "secure_docs", "non-member-sub")

	apiJSONRequestWithToken(t, http.MethodDelete, ts.URL+"/api/collections/secure_docs/"+fmt.Sprint(created["id"]), nil, memberToken, http.StatusNoContent)

	event := assertWSTableEvent(t, memberWS, "delete", "secure_docs")
	assertRealtimeRecordValue(t, event.Record, "id", "ws-doc-delete-1")
	assertNoWSMessage(t, nonMemberWS, 250*time.Millisecond, "non-member socket received secure_docs delete event")
}

func TestRealtimeWSCrossNodeTableDelivery(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	tsA, tsB := newRealtimeTwoNodeTestServers(t, ctx, nil)

	wsConn := dialWS(t, tsB)
	defer wsConn.Close()

	connected := readWSJSON(t, wsConn)
	testutil.Equal(t, ws.MsgTypeConnected, connected.Type)
	testutil.True(t, connected.ClientID != "", "expected non-empty client_id")

	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{"users"},
		Ref:    "cross-node-sub",
	})
	reply := readWSJSON(t, wsConn)
	testutil.Equal(t, ws.MsgTypeReply, reply.Type)
	testutil.Equal(t, "ok", reply.Status)
	testutil.Equal(t, "cross-node-sub", reply.Ref)

	uniqueName := fmt.Sprintf("CrossNodeWS%d", time.Now().UnixNano())
	created := decodeAPIRecord(t, apiJSONRequest(t, http.MethodPost, tsA.URL+"/api/collections/users/",
		map[string]any{"name": uniqueName, "email": strings.ToLower(uniqueName) + "@example.com"},
		http.StatusCreated))
	userID := fmt.Sprint(created["id"])

	event := readWSUntilType(t, wsConn, ws.MsgTypeEvent)
	testutil.Equal(t, "create", event.Action)
	testutil.Equal(t, "users", event.Table)
	testutil.True(t, event.Record != nil, "expected record in cross-node WebSocket create event")
	assertRealtimeRecordValue(t, event.Record, "id", created["id"])
	assertRealtimeRecordValue(t, event.Record, "name", uniqueName)
	assertRealtimeRecordValue(t, event.Record, "email", strings.ToLower(uniqueName)+"@example.com")

	updatedName := uniqueName + "_Updated"
	updated := decodeAPIRecord(t, apiJSONRequest(t, http.MethodPatch, tsA.URL+"/api/collections/users/"+userID,
		map[string]any{"name": updatedName, "email": strings.ToLower(uniqueName) + "@example.com"},
		http.StatusOK))

	event = readWSUntilType(t, wsConn, ws.MsgTypeEvent)
	testutil.Equal(t, "update", event.Action)
	testutil.Equal(t, "users", event.Table)
	testutil.True(t, event.Record != nil, "expected record in cross-node WebSocket update event")
	assertRealtimeRecordValue(t, event.Record, "id", updated["id"])
	assertRealtimeRecordValue(t, event.Record, "name", updatedName)
	assertRealtimeRecordValue(t, event.Record, "email", strings.ToLower(uniqueName)+"@example.com")

	apiJSONRequest(t, http.MethodDelete, tsA.URL+"/api/collections/users/"+userID, nil, http.StatusNoContent)

	event = readWSUntilType(t, wsConn, ws.MsgTypeEvent)
	testutil.Equal(t, "delete", event.Action)
	testutil.Equal(t, "users", event.Table)
	testutil.True(t, event.Record != nil, "expected record in cross-node WebSocket delete event")
	assertRealtimeRecordValue(t, event.Record, "id", userID)
}

// TestRealtimeWSFilteredSubscriptionDelivery verifies that filtered subscriptions
// only receive events matching the filter criteria.
func TestRealtimeWSFilteredSubscriptionDelivery(t *testing.T) {
	ts := newWSTestServer(t)

	// Connect two clients — one with filter, one without.
	wsFiltered := dialWS(t, ts)
	defer wsFiltered.Close()
	readWSJSON(t, wsFiltered) // connected

	wsAll := dialWS(t, ts)
	defer wsAll.Close()
	readWSJSON(t, wsAll) // connected

	// Filtered client subscribes only to records where name='Bob'.
	// Filter syntax: column=operator.value (parsed by realtime.ParseFilters).
	writeWSJSON(t, wsFiltered, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{"users"},
		Filter: "name=eq.Bob",
		Ref:    "sub-filtered",
	})
	reply := readWSJSON(t, wsFiltered)
	testutil.Equal(t, "ok", reply.Status)

	// Unfiltered client subscribes to all users.
	writeWSJSON(t, wsAll, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{"users"},
		Ref:    "sub-all",
	})
	reply = readWSJSON(t, wsAll)
	testutil.Equal(t, "ok", reply.Status)

	// Create a record with name='Alice' — should only go to unfiltered client.
	body, _ := json.Marshal(map[string]any{"name": "Alice", "email": "alice@example.com"})
	resp, _ := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	// Unfiltered client should receive the event.
	allCh := make(chan ws.ServerMessage, 1)
	go func() {
		msg := readWSJSON(t, wsAll)
		allCh <- msg
	}()

	// Unfiltered client gets the event.
	select {
	case event := <-allCh:
		testutil.Equal(t, ws.MsgTypeEvent, event.Type)
		testutil.Equal(t, "Alice", event.Record["name"])
	case <-time.After(2 * time.Second):
		t.Fatal("unfiltered client should have received event")
	}

	// Now create a record with name='Bob' — both clients should receive it.
	body, _ = json.Marshal(map[string]any{"name": "Bob", "email": "bob@example.com"})
	resp, _ = http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	// Both clients should receive this event. If the filtered subscription
	// incorrectly received Alice, it would be dequeued first here and fail.
	allCh = make(chan ws.ServerMessage, 1)
	go func() {
		msg := readWSJSON(t, wsAll)
		allCh <- msg
	}()

	filteredCh := make(chan ws.ServerMessage, 1)
	go func() {
		msg := readWSJSON(t, wsFiltered)
		filteredCh <- msg
	}()

	select {
	case event := <-allCh:
		testutil.Equal(t, "Bob", event.Record["name"])
	case <-time.After(2 * time.Second):
		t.Fatal("unfiltered client should have received Bob event")
	}

	select {
	case event := <-filteredCh:
		testutil.Equal(t, "Bob", event.Record["name"])
	case <-time.After(2 * time.Second):
		t.Fatal("filtered client should have received Bob event")
	}
}

// TestRealtimeWSBroadcastRelayBetweenClients verifies that broadcast messages
// are relayed between different WebSocket clients subscribed to the same channel,
// and that self-receive works when Self=true.
func TestRealtimeWSBroadcastRelayBetweenClients(t *testing.T) {
	ts := newWSTestServer(t)

	// Connect two clients.
	ws1 := dialWS(t, ts)
	defer ws1.Close()
	readWSJSON(t, ws1) // connected

	ws2 := dialWS(t, ts)
	defer ws2.Close()
	readWSJSON(t, ws2) // connected

	// Both subscribe to the same channel.
	// handleChannelSubscribe sends replyOK then auto presence_sync.
	for _, wsConn := range []*websocket.Conn{ws1, ws2} {
		writeWSJSON(t, wsConn, ws.ClientMessage{
			Type:    ws.MsgTypeChannelSubscribe,
			Channel: "chat-room",
			Ref:     "sub-chat",
		})
		reply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
		testutil.Equal(t, "ok", reply.Status)
	}

	// Client 1 sends a broadcast with self-receive (Self:true).
	// In handleBroadcast, Relay enqueues messages before replyOK.
	writeWSJSON(t, ws1, ws.ClientMessage{
		Type:    ws.MsgTypeBroadcast,
		Channel: "chat-room",
		Event:   "chat-message",
		Payload: map[string]any{"from": "client1", "text": "hello!"},
		Self:    true,
		Ref:     "bc-1",
	})

	// Client 1 self-relay arrives before reply (Relay enqueued first).
	selfRelay := readWSUntilType(t, ws1, ws.MsgTypeBroadcast)
	testutil.Equal(t, "chat-room", selfRelay.Channel)
	testutil.Equal(t, "hello!", selfRelay.Payload["text"])

	reply := readWSUntilType(t, ws1, ws.MsgTypeReply)
	testutil.Equal(t, "ok", reply.Status)

	// Client 2 receives the cross-client relay.
	crossRelay := readWSUntilType(t, ws2, ws.MsgTypeBroadcast)
	testutil.Equal(t, ws.MsgTypeBroadcast, crossRelay.Type)
	testutil.Equal(t, "chat-room", crossRelay.Channel)
	testutil.Equal(t, "chat-message", crossRelay.Event)
	testutil.Equal(t, "client1", crossRelay.Payload["from"])
	testutil.Equal(t, "hello!", crossRelay.Payload["text"])
}

// TestRealtimeWSPresenceTrackSyncAndUntrack verifies presence tracking,
// sync, and untrack functionality across multiple clients.
func TestRealtimeWSPresenceTrackSyncAndUntrack(t *testing.T) {
	ts := newWSTestServer(t)

	// Connect two clients.
	ws1 := dialWS(t, ts)
	defer ws1.Close()
	connected1 := readWSJSON(t, ws1)
	clientID1 := connected1.ClientID

	ws2 := dialWS(t, ts)
	defer ws2.Close()
	connected2 := readWSJSON(t, ws2)
	clientID2 := connected2.ClientID

	// Both clients must subscribe to the channel before tracking presence.
	// handleChannelSubscribe sends replyOK then auto presence_sync.
	for _, wsConn := range []*websocket.Conn{ws1, ws2} {
		writeWSJSON(t, wsConn, ws.ClientMessage{
			Type:    ws.MsgTypeChannelSubscribe,
			Channel: "lobby",
			Ref:     "chan-sub",
		})
		reply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
		testutil.Equal(t, "ok", reply.Status)
	}

	// Both clients track presence on the channel.
	// sendPresenceDiff (join) is broadcast to all channel subscribers before replyOK.
	for i, wsConn := range []*websocket.Conn{ws1, ws2} {
		name := "user1"
		if i == 1 {
			name = "user2"
		}
		writeWSJSON(t, wsConn, ws.ClientMessage{
			Type:     ws.MsgTypePresenceTrack,
			Channel:  "lobby",
			Presence: map[string]any{"name": name, "status": "online"},
			Ref:      "track-" + name,
		})
		reply := readWSUntilType(t, wsConn, ws.MsgTypeReply)
		testutil.Equal(t, "ok", reply.Status)
	}

	// Client 1 requests sync — should see both presences.
	// There may be buffered join diffs; readWSPresenceSync skips them.
	writeWSJSON(t, ws1, ws.ClientMessage{
		Type:    ws.MsgTypePresenceSync,
		Channel: "lobby",
		Ref:     "sync-1",
	})
	sync := readWSPresenceSync(t, ws1)
	testutil.Equal(t, "lobby", sync.Channel)
	testutil.True(t, len(sync.Presences) == 2, "expected 2 presences in sync")
	_, hasClient1 := sync.Presences[clientID1]
	_, hasClient2 := sync.Presences[clientID2]
	testutil.True(t, hasClient1, "expected client1 presence")
	testutil.True(t, hasClient2, "expected client2 presence")

	// Client 2 untracks. sendPresenceDiff (leave) is broadcast before replyOK.
	writeWSJSON(t, ws2, ws.ClientMessage{
		Type:    ws.MsgTypePresenceUntrack,
		Channel: "lobby",
		Ref:     "untrack-2",
	})
	reply := readWSUntilType(t, ws2, ws.MsgTypeReply)
	testutil.Equal(t, "ok", reply.Status)

	// Client 1 requests sync again — should see only its own presence.
	// There may be a buffered leave diff; readWSPresenceSync skips it.
	writeWSJSON(t, ws1, ws.ClientMessage{
		Type:    ws.MsgTypePresenceSync,
		Channel: "lobby",
		Ref:     "sync-2",
	})
	sync = readWSPresenceSync(t, ws1)
	testutil.True(t, len(sync.Presences) == 1, "expected 1 presence after untrack")
	_, hasClient1 = sync.Presences[clientID1]
	testutil.True(t, hasClient1, "expected only client1 presence")
}

// TestRealtimeWSNonDefaultConfig verifies that non-default realtime config
// values are properly applied in the WebSocket handler.
func TestRealtimeWSNonDefaultConfig(t *testing.T) {
	ts := newWSTestServer(t, func(cfg *config.Config) {
		cfg.Realtime.HeartbeatIntervalSeconds = 45
		cfg.Realtime.BroadcastRateLimitPerSecond = 50
		cfg.Realtime.BroadcastMaxMessageBytes = 16384
		cfg.Realtime.PresenceLeaveTimeoutSeconds = 30
	})

	wsConn := dialWS(t, ts)
	defer wsConn.Close()

	// Read connected message.
	connected := readWSJSON(t, wsConn)
	testutil.Equal(t, ws.MsgTypeConnected, connected.Type)

	// Subscribe and verify functionality works with non-default config.
	writeWSJSON(t, wsConn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{"users"},
		Ref:    "sub-1",
	})
	reply := readWSJSON(t, wsConn)
	testutil.Equal(t, "ok", reply.Status)

	// Create a record and verify event delivery still works.
	body, _ := json.Marshal(map[string]any{"name": "ConfigTest", "email": "config@example.com"})
	resp, _ := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	eventCh := make(chan ws.ServerMessage, 1)
	go func() {
		msg := readWSJSON(t, wsConn)
		eventCh <- msg
	}()

	select {
	case event := <-eventCh:
		testutil.Equal(t, ws.MsgTypeEvent, event.Type)
		testutil.Equal(t, "ConfigTest", event.Record["name"])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event with non-default config")
	}
}
