package crossnode

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
)

// realtimeWSURL derives the realtime websocket URL from an HTTP base URL.
func realtimeWSURL(baseURL string) string {
	wsBase := strings.Replace(baseURL, "http://", "ws://", 1)
	wsBase = strings.Replace(wsBase, "https://", "wss://", 1)
	return wsBase + "/api/realtime/ws"
}

// DialRealtimeWS opens an authenticated realtime websocket against baseURL and
// waits for the initial "connected" frame. It returns the connection and the
// HTTP upgrade response so callers can inspect load-balancer attribution
// headers (e.g. X-AYB-Upstream) on the upgrade. The caller owns closing conn.
func DialRealtimeWS(t *testing.T, baseURL, token string) (*websocket.Conn, *http.Response) {
	t.Helper()

	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, resp, err := websocket.DefaultDialer.Dial(realtimeWSURL(baseURL), headers)
	if err != nil {
		t.Fatalf("dial realtime websocket at %s: %v", baseURL, err)
	}
	msg, err := ReadWSUntil(t, conn, ws.MsgTypeConnected, 5*time.Second)
	if err != nil {
		conn.Close()
		t.Fatalf("websocket connected message failed: %v", err)
	}
	if msg.ClientID == "" {
		conn.Close()
		t.Fatalf("websocket connected message missing client id: %#v", msg)
	}
	return conn, resp
}

// WriteWSJSON writes a JSON message to a websocket connection.
func WriteWSJSON(t *testing.T, conn *websocket.Conn, msg any) {
	t.Helper()
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("write websocket JSON: %v", err)
	}
}

// ReadWSUntil reads server messages until one of msgType arrives or the timeout
// elapses. On timeout it reports the last mismatching message for diagnostics.
func ReadWSUntil(t *testing.T, conn *websocket.Conn, msgType string, timeout time.Duration) (ws.ServerMessage, error) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var lastMismatch string
	for {
		var msg ws.ServerMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if lastMismatch != "" {
				return ws.ServerMessage{}, fmt.Errorf("%w (last message: %s)", err, lastMismatch)
			}
			return ws.ServerMessage{}, err
		}
		if msg.Type == msgType {
			return msg, nil
		}
		lastMismatch = fmt.Sprintf("type=%s status=%s action=%s table=%s", msg.Type, msg.Status, msg.Action, msg.Table)
	}
}
