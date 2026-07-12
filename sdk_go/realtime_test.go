package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRealtimeEventFixtureDecodesExactRowEvent(t *testing.T) {
	data := mustLoadContractFixture(t, "realtime_event.json")
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode realtime_event fixture: %v", err)
	}

	if event.Action != "UPDATE" {
		t.Fatalf("unexpected action %q", event.Action)
	}
	if event.Table != "posts" {
		t.Fatalf("unexpected table %q", event.Table)
	}
	if event.Record["id"] != "rec_1" {
		t.Fatalf("unexpected record id %#v", event.Record["id"])
	}
	if event.Record["title"] != "after" {
		t.Fatalf("unexpected record title %#v", event.Record["title"])
	}
	expectedOldRecord := map[string]any{"id": "rec_1", "title": "before"}
	if !reflect.DeepEqual(event.OldRecord, expectedOldRecord) {
		t.Fatalf("unexpected old record %#v", event.OldRecord)
	}
}

func TestRealtimeEventOldRecordAliasesNormalizeToOneOwner(t *testing.T) {
	tests := []struct {
		name string
		body string
		want map[string]any
	}{
		{
			name: "camel case",
			body: `{"action":"UPDATE","table":"posts","record":{"id":"rec_1"},"oldRecord":{"id":"camel"}}`,
			want: map[string]any{"id": "camel"},
		},
		{
			name: "snake case",
			body: `{"action":"UPDATE","table":"posts","record":{"id":"rec_1"},"old_record":{"id":"snake"}}`,
			want: map[string]any{"id": "snake"},
		},
		{
			name: "camel case takes precedence",
			body: `{"action":"UPDATE","table":"posts","record":{"id":"rec_1"},"oldRecord":{"id":"camel"},"old_record":{"id":"snake"}}`,
			want: map[string]any{"id": "camel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event Event
			if err := json.Unmarshal([]byte(tt.body), &event); err != nil {
				t.Fatalf("decode event: %v", err)
			}
			if !reflect.DeepEqual(event.OldRecord, tt.want) {
				t.Fatalf("unexpected old record %#v", event.OldRecord)
			}
		})
	}
}

func TestRealtimeServerRowEventEnvelopeMatchesWSOwner(t *testing.T) {
	assertRealtimeWSOwnerSource(t)

	frames := []string{
		`{"type":"connected","client_id":"ws-1"}`,
		`{"type":"event","action":"UPDATE","table":"posts","record":{"id":"rec_1","title":"after"}}`,
	}

	var connected realtimeServerFrame
	if err := json.Unmarshal([]byte(frames[0]), &connected); err != nil {
		t.Fatalf("decode connected frame: %v", err)
	}
	if connected.Type != "connected" || connected.ClientID != "ws-1" {
		t.Fatalf("unexpected connected frame %+v", connected)
	}

	var event realtimeServerFrame
	if err := json.Unmarshal([]byte(frames[1]), &event); err != nil {
		t.Fatalf("decode event frame: %v", err)
	}
	if event.Type != "event" {
		t.Fatalf("unexpected frame type %q", event.Type)
	}
	if event.Action != "UPDATE" || event.Table != "posts" {
		t.Fatalf("unexpected row event identity %+v", event)
	}
	if event.Record["id"] != "rec_1" || event.Record["title"] != "after" {
		t.Fatalf("unexpected row event record %#v", event.Record)
	}
	if event.OldRecord != nil {
		t.Fatalf("websocket row event should not carry oldRecord today: %#v", event.OldRecord)
	}
}

func assertRealtimeWSOwnerSource(t *testing.T) {
	t.Helper()
	sourcePath := filepath.Join("..", "internal", "ws", "message.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read websocket message owner: %v", err)
	}
	text := string(source)
	ownerFacts := []string{
		`MsgTypeConnected = "connected"`,
		`MsgTypeEvent     = "event"`,
		"ClientID       string",
		"`json:\"client_id,omitempty\"`",
		"Action         string",
		"`json:\"action,omitempty\"`",
		"Table          string",
		"`json:\"table,omitempty\"`",
		"Record         map[string]any",
		"`json:\"record,omitempty\"`",
		"func connectedMsg(clientID string) ServerMessage",
		"return ServerMessage{Type: MsgTypeConnected, ClientID: clientID}",
		"func EventMsg(action, table string, record map[string]any) ServerMessage",
		"return ServerMessage{Type: MsgTypeEvent, Action: action, Table: table, Record: record}",
	}
	for _, fact := range ownerFacts {
		if !strings.Contains(text, fact) {
			t.Fatalf("websocket message owner %s no longer contains %q", sourcePath, fact)
		}
	}
	if strings.Contains(text, "OldRecord") {
		t.Fatalf("websocket message owner now contains OldRecord; update Stage 1 event-frame expectations")
	}
}

func TestRealtimeSubscribeAuthenticatedHandshakeAfterConnected(t *testing.T) {
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		writeRealtimeFrame(t, ws, map[string]any{"type": "connected", "client_id": "ws-1"})
		auth := readRealtimeFrame(t, ws)
		assertRealtimeFrame(t, auth, map[string]any{"type": "auth", "token": "test-jwt"})
		subscribe := readRealtimeFrameAfterNoFrame(t, ws, 50*time.Millisecond, func() {
			writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": auth["ref"], "status": "ok"})
		})
		assertRealtimeFrame(t, subscribe, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": subscribe["ref"], "status": "ok"})
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.SetTokens("test-jwt", "refresh")
	events, cancel, err := client.Realtime().Subscribe(context.Background(), "posts", SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if events == nil {
		t.Fatalf("events channel is nil")
	}
	if cancel == nil {
		t.Fatalf("cancel func is nil")
	}
}

func TestRealtimeSubscribeBeginsOnOpenWithoutConnectedFrame(t *testing.T) {
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		subscribe := readRealtimeFrame(t, ws)
		assertRealtimeFrame(t, subscribe, map[string]any{"type": "subscribe", "tables": []string{"tasks"}})
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": subscribe["ref"], "status": "ok"})
	})
	defer server.Close()

	client := NewClient(server.URL)
	_, _, err := client.Realtime().Subscribe(context.Background(), "tasks", SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
}

func TestRealtimeSubscribeAuthenticatedHandshakeBeginsOnOpenWithoutConnectedFrame(t *testing.T) {
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		auth := readRealtimeFrame(t, ws)
		assertRealtimeFrame(t, auth, map[string]any{"type": "auth", "token": "test-jwt"})
		subscribe := readRealtimeFrameAfterNoFrame(t, ws, 50*time.Millisecond, func() {
			writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": auth["ref"], "status": "ok"})
		})
		assertRealtimeFrame(t, subscribe, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": subscribe["ref"], "status": "ok"})
	})
	defer server.Close()

	client := NewClient(server.URL)
	client.SetTokens("test-jwt", "refresh")
	_, _, err := client.Realtime().Subscribe(context.Background(), "posts", SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
}

func TestRealtimeSubscribeUsesFilterAndSurfacesReplyError(t *testing.T) {
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		subscribe := readRealtimeFrame(t, ws)
		assertRealtimeFrame(t, subscribe, map[string]any{
			"type":   "subscribe",
			"tables": []string{"posts"},
			"filter": "status=eq.open",
		})
		writeRealtimeFrame(t, ws, map[string]any{
			"type":    "reply",
			"ref":     subscribe["ref"],
			"status":  "error",
			"message": "subscription rejected",
		})
	})
	defer server.Close()

	client := NewClient(server.URL)
	_, _, err := client.Realtime().Subscribe(context.Background(), "posts", SubscribeOptions{Filter: "status=eq.open"})
	if err == nil {
		t.Fatalf("expected subscribe error")
	}
	if !strings.Contains(err.Error(), "subscription rejected") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRealtimeSubscribeDeliversRowEvents(t *testing.T) {
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		subscribe := readRealtimeFrame(t, ws)
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": subscribe["ref"], "status": "ok"})
		writeRealtimeFrame(t, ws, map[string]any{
			"type":   "event",
			"action": "UPDATE",
			"table":  "posts",
			"record": map[string]any{"id": "rec_1", "title": "after"},
		})
	})
	defer server.Close()

	client := NewClient(server.URL)
	events, cancel, err := client.Realtime().Subscribe(context.Background(), "posts", SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()

	select {
	case event := <-events:
		if event.Action != "UPDATE" || event.Table != "posts" {
			t.Fatalf("unexpected event identity %+v", event)
		}
		if event.Record["id"] != "rec_1" || event.Record["title"] != "after" {
			t.Fatalf("unexpected event record %#v", event.Record)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for realtime event")
	}
}

func TestRealtimeCancelUnsubscribesAndClosesEventChannel(t *testing.T) {
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		subscribe := readRealtimeFrame(t, ws)
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": subscribe["ref"], "status": "ok"})
		unsubscribe := readRealtimeFrame(t, ws)
		assertRealtimeFrame(t, unsubscribe, map[string]any{"type": "unsubscribe", "tables": []string{"posts"}})
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": unsubscribe["ref"], "status": "ok"})
	})
	defer server.Close()

	client := NewClient(server.URL)
	events, cancel, err := client.Realtime().Subscribe(context.Background(), "posts", SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := cancel(); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	select {
	case _, ok := <-events:
		if ok {
			t.Fatalf("events channel remained open after cancel")
		}
	case <-time.After(time.Second):
		t.Fatalf("events channel did not close after cancel")
	}
}

func TestRealtimeReconnectResubscribesWithPythonDefaultBackoff(t *testing.T) {
	attempts := make(chan []map[string]any, 2)
	server := newRealtimeScriptServer(t, func(t *testing.T, ws *websocket.Conn) {
		subscribe := readRealtimeFrame(t, ws)
		writeRealtimeFrame(t, ws, map[string]any{"type": "reply", "ref": subscribe["ref"], "status": "ok"})
		attempts <- []map[string]any{subscribe}
		_ = ws.Close()
	})
	defer server.Close()

	client := NewClient(server.URL)
	options := SubscribeOptions{
		Reconnect: RealtimeReconnectOptions{
			MaxAttempts: 2,
			Delays:      []time.Duration{time.Millisecond, time.Millisecond},
			Jitter:      0,
		},
	}
	_, cancel, err := client.Realtime().Subscribe(context.Background(), "posts", options)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()

	first := receiveRealtimeAttempt(t, attempts)
	second := receiveRealtimeAttempt(t, attempts)
	assertRealtimeFrame(t, first[0], map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	assertRealtimeFrame(t, second[0], map[string]any{"type": "subscribe", "tables": []string{"posts"}})

	defaults := DefaultRealtimeReconnectOptions()
	if defaults.MaxAttempts != 5 {
		t.Fatalf("max reconnect attempts = %d, want 5", defaults.MaxAttempts)
	}
	wantDelays := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}
	if !reflect.DeepEqual(defaults.Delays, wantDelays) {
		t.Fatalf("reconnect delays = %v, want %v", defaults.Delays, wantDelays)
	}
	if defaults.Jitter != 100*time.Millisecond {
		t.Fatalf("jitter = %v, want 100ms", defaults.Jitter)
	}
}

type realtimeServerFrame struct {
	Type      string         `json:"type"`
	Ref       string         `json:"ref,omitempty"`
	ClientID  string         `json:"client_id,omitempty"`
	Status    string         `json:"status,omitempty"`
	Message   string         `json:"message,omitempty"`
	Action    string         `json:"action,omitempty"`
	Table     string         `json:"table,omitempty"`
	Record    map[string]any `json:"record,omitempty"`
	OldRecord map[string]any `json:"oldRecord,omitempty"`
}

func newRealtimeScriptServer(t *testing.T, script func(*testing.T, *websocket.Conn)) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/realtime/ws" {
			http.NotFound(w, r)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()
		script(t, ws)
	}))
}

func readRealtimeFrame(t *testing.T, ws *websocket.Conn) map[string]any {
	t.Helper()
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var frame map[string]any
	if err := ws.ReadJSON(&frame); err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}
	return frame
}

func writeRealtimeFrame(t *testing.T, ws *websocket.Conn, frame map[string]any) {
	t.Helper()
	if err := ws.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set write deadline: %v", err)
	}
	if err := ws.WriteJSON(frame); err != nil {
		t.Fatalf("write websocket frame: %v", err)
	}
}

func readRealtimeFrameAfterNoFrame(t *testing.T, ws *websocket.Conn, wait time.Duration, unblock func()) map[string]any {
	t.Helper()

	results := make(chan realtimeReadResult, 1)
	go func() {
		frame, err := readRealtimeFrameResult(ws)
		results <- realtimeReadResult{frame: frame, err: err}
	}()

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("read websocket frame: %v", result.err)
		}
		t.Fatalf("unexpected websocket frame before auth reply: %#v", result.frame)
	case <-time.After(wait):
	}

	unblock()

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("read websocket frame: %v", result.err)
		}
		return result.frame
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for websocket frame after auth reply")
		return nil
	}
}

type realtimeReadResult struct {
	frame map[string]any
	err   error
}

func readRealtimeFrameResult(ws *websocket.Conn) (map[string]any, error) {
	var frame map[string]any
	err := ws.ReadJSON(&frame)
	return frame, err
}

func assertRealtimeFrame(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		gotValue := got[key]
		if wantSlice, ok := wantValue.([]string); ok {
			if !reflect.DeepEqual(stringSliceFromFrame(gotValue), wantSlice) {
				t.Fatalf("%s = %#v, want %#v in frame %#v", key, gotValue, wantValue, got)
			}
			continue
		}
		if gotValue != wantValue {
			t.Fatalf("%s = %#v, want %#v in frame %#v", key, gotValue, wantValue, got)
		}
	}
	if _, ok := want["ref"]; !ok && got["ref"] == "" {
		t.Fatalf("frame missing ref: %#v", got)
	}
}

func stringSliceFromFrame(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil
		}
		out = append(out, text)
	}
	return out
}

func receiveRealtimeAttempt(t *testing.T, attempts <-chan []map[string]any) []map[string]any {
	t.Helper()
	select {
	case attempt := <-attempts:
		return attempt
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for reconnect attempt")
		return nil
	}
}
