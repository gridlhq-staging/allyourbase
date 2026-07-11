//go:build integration

package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

type twoNodeServers struct {
	nodeA, nodeB *server.Server
	tsA, tsB     *httptest.Server
}

func newOAuthTwoNodeTestServers(t *testing.T, ctx context.Context) *twoNodeServers {
	t.Helper()

	logger := testutil.DiscardLogger()
	chA := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, chA.Load(ctx))
	chB := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, chB.Load(ctx))

	cfg := config.Default()
	cfg.Database.URL = sharedPG.ConnString

	nodeA := server.New(cfg, logger, chA, sharedPG.Pool, nil, nil)
	nodeB := server.New(cfg, logger, chB, sharedPG.Pool, nil, nil)
	t.Cleanup(func() {
		_ = nodeA.Shutdown(context.Background())
		_ = nodeB.Shutdown(context.Background())
	})

	tsA := httptest.NewServer(nodeA.Router())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(nodeB.Router())
	t.Cleanup(tsB.Close)

	return &twoNodeServers{nodeA: nodeA, nodeB: nodeB, tsA: tsA, tsB: tsB}
}

func TestRealtimeSSECrossNodeOAuthDelivery(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	nodes := newOAuthTwoNodeTestServers(t, ctx)

	resp, err := http.Get(nodes.tsA.URL + "/api/realtime?oauth=true")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for OAuth SSE connected event on node A")
	testutil.Equal(t, "event: connected", connected[0])

	connData := decodeSSEEventPayload(t, connected)
	clientID, ok := connData["clientId"].(string)
	testutil.True(t, ok && clientID != "", "expected non-empty clientId in connected event")

	hubB := nodes.nodeB.RealtimeHub()
	hubB.PublishOAuth(clientID, &auth.OAuthEvent{Token: "cross-node-oauth-probe"})

	lines := readNextSSEEvent(t, scanner, 3*time.Second, "timed out waiting for cross-node OAuth event on node A")
	testutil.True(t, len(lines) >= 2, "expected event+data lines")
	testutil.Equal(t, "event: oauth", lines[0])

	var oauthPayload auth.OAuthEvent
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			testutil.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &oauthPayload))
			break
		}
	}
	testutil.Equal(t, "cross-node-oauth-probe", oauthPayload.Token)

	// OAuth SSE is one-shot — the handler closes the stream after delivering
	// one event. Verify no second event arrives before the stream ends.
	extraCh := make(chan []string, 1)
	go func() {
		var extra []string
		for scanner.Scan() {
			line := scanner.Text()
			extra = append(extra, line)
			if line == "" && len(extra) > 1 {
				break
			}
		}
		extraCh <- extra
	}()
	select {
	case extra := <-extraCh:
		if len(extra) > 0 {
			t.Fatalf("unexpected duplicate OAuth event on node A: %#v", extra)
		}
	case <-time.After(time.Second):
		t.Fatal("SSE stream did not close after one-shot OAuth event")
	}
}
