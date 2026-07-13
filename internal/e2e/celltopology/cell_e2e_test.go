//go:build cell

package celltopology

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/e2e/crossnode"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
)

// TestCellTopologyLBRoutedProofs boots the compose cell once and drives all
// three minimum proofs with every client request entering through the nginx
// LB. It then asserts the proof set actually reached both AYB upstreams.
func TestCellTopologyLBRoutedProofs(t *testing.T) {
	cell := bootCell(t)
	upstreams := &upstreamSet{seen: map[string]struct{}{}}

	proveRealtimeFanoutAcrossNodes(t, cell, upstreams)
	proveSessionRevocationAcrossNodes(t, cell, upstreams)
	proveStorageIsolationAcrossNodes(t, cell, upstreams)

	if distinct := upstreams.distinct(); distinct < 2 {
		t.Fatalf("expected LB-routed traffic to span >=2 distinct AYB upstreams, saw %d: %v",
			distinct, upstreams.list())
	}
}

// proveRealtimeFanoutAcrossNodes subscribes to a realtime table through the LB,
// writes through the LB onto a *different* upstream than the subscriber holds,
// and observes the resulting event — proving cross-node fanout under the LB's
// round-robin routing.
func proveRealtimeFanoutAcrossNodes(t *testing.T, cell *cell, upstreams *upstreamSet) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	crossnode.SeedRealtimeProofTable(ctx, t, cell.pgURL, realtimeProofTable)
	auth := crossnode.RegisterUser(t, cell.lbURL, "cell-realtime@example.com")
	crossnode.ConfigureUsersAsSharedTenants(ctx, t, cell.pgURL, auth.UserID)

	// Unlike the multinode harness (which seeds before booting the nodes), the
	// compose AYB containers are already running when we create the proof
	// table, so both must reflect the new table into their schema cache before
	// the collection endpoint accepts writes. Wait until the LB serves it.
	waitForCollectionReflected(t, cell.lbURL, realtimeProofTable, auth.AccessToken, 30*time.Second)

	conn, dialResp := crossnode.DialRealtimeWS(t, cell.lbURL, auth.AccessToken)
	defer conn.Close()
	subscribeUpstream := dialResp.Header.Get(upstreamHeader)
	upstreams.add(subscribeUpstream)

	crossnode.WriteWSJSON(t, conn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{realtimeProofTable},
		Ref:    "sub-realtime-proof",
	})
	reply, err := crossnode.ReadWSUntil(t, conn, ws.MsgTypeReply, 5*time.Second)
	if err != nil {
		t.Fatalf("realtime subscribe reply failed: %v", err)
	}
	if reply.Status != "ok" || reply.Ref != "sub-realtime-proof" {
		t.Fatalf("unexpected subscribe reply: %#v", reply)
	}

	// Write repeatedly until a write lands on a different upstream than the one
	// holding the WS subscription, so the observed event is genuinely a
	// cross-node fanout (write node != subscribe node). The LB health probe can
	// perturb round-robin ordering, so a fixed single write would be flaky.
	sentinel, publishUpstream := writeUntilDifferentUpstream(t, cell, auth.AccessToken, subscribeUpstream, upstreams)
	if publishUpstream == subscribeUpstream {
		t.Fatalf("could not route a realtime write to a different upstream than the WS subscribe (both %q)", subscribeUpstream)
	}

	if !observeSentinelEvent(t, conn, sentinel, 15*time.Second) {
		t.Fatalf("did not observe realtime event for sentinel %q (subscribe=%s publish=%s)",
			sentinel, subscribeUpstream, publishUpstream)
	}
}

// writeUntilDifferentUpstream posts fresh sentinels through the LB until one
// lands on an upstream other than avoidUpstream (or attempts are exhausted),
// returning the last sentinel written and the upstream that served it.
func writeUntilDifferentUpstream(
	t *testing.T,
	cell *cell,
	token string,
	avoidUpstream string,
	upstreams *upstreamSet,
) (string, string) {
	t.Helper()
	var sentinel, upstream string
	for attempt := 0; attempt < 12; attempt++ {
		sentinel = "cell-realtime-" + crossnode.RandomHex(t, 6)
		resp := crossnode.Do(t, crossnode.RawRequest{
			Method: http.MethodPost,
			URL:    cell.lbURL + "/api/collections/" + realtimeProofTable,
			Body:   map[string]any{"sentinel": sentinel},
			Token:  token,
		})
		// A 404 means the upstream this write landed on has not yet reflected
		// the freshly-seeded table; retry so a slower node can catch up.
		if resp.Status == http.StatusNotFound {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.Status != http.StatusCreated {
			t.Fatalf("create realtime sentinel status=%d body=%s", resp.Status, resp.Body)
		}
		upstream = resp.Header.Get(upstreamHeader)
		upstreams.add(upstream)
		if upstream != "" && upstream != avoidUpstream {
			break
		}
	}
	return sentinel, upstream
}

// waitForCollectionReflected polls the LB until the collection list endpoint
// serves HTTP 200 for the table, i.e. both upstreams have picked up the seeded
// table into their schema cache.
func waitForCollectionReflected(t *testing.T, lbURL, table, token string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	listURL := lbURL + "/api/collections/" + table + "?perPage=1"
	var status int
	var body string
	consecutiveOK := 0
	for time.Now().Before(deadline) {
		status, body = crossnode.Raw(t, http.MethodGet, listURL, nil, token)
		if status == http.StatusOK {
			// Require several consecutive 200s so both round-robin upstreams
			// have reflected the table, not just the one that served the probe.
			consecutiveOK++
			if consecutiveOK >= 4 {
				return
			}
		} else {
			consecutiveOK = 0
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("collection %q not reflected via LB before deadline: status=%d body=%s", table, status, body)
}

// observeSentinelEvent reads realtime events until it sees a create event whose
// record matches sentinel, or the timeout elapses.
func observeSentinelEvent(t *testing.T, conn *websocket.Conn, sentinel string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event, err := crossnode.ReadWSUntil(t, conn, ws.MsgTypeEvent, time.Until(deadline))
		if err != nil {
			return false
		}
		if event.Action == "create" && event.Table == realtimeProofTable && event.Record["sentinel"] == sentinel {
			return true
		}
	}
	return false
}

// proveSessionRevocationAcrossNodes registers, lists, and revokes a session
// through the LB, then asserts the access and refresh tokens are rejected
// through the LB — cross-node because revocation state lives in shared PG.
func proveSessionRevocationAcrossNodes(t *testing.T, cell *cell, upstreams *upstreamSet) {
	t.Helper()
	auth := crossnode.RegisterUser(t, cell.lbURL, "cell-revoke@example.com")

	baseline := crossnode.Do(t, crossnode.RawRequest{
		Method: http.MethodGet, URL: cell.lbURL + "/api/auth/me", Token: auth.AccessToken,
	})
	upstreams.add(baseline.Header.Get(upstreamHeader))
	if baseline.Status != http.StatusOK || !strings.Contains(baseline.Body, auth.UserID) || !strings.Contains(baseline.Body, auth.Email) {
		t.Fatalf("baseline LB /api/auth/me status=%d body=%s", baseline.Status, baseline.Body)
	}

	sessions := crossnode.ListSessions(t, cell.lbURL, auth.AccessToken)
	if len(sessions) != 1 || sessions[0].ID == "" {
		t.Fatalf("expected one current session before revoke, got %#v", sessions)
	}

	deleteResp := crossnode.Do(t, crossnode.RawRequest{
		Method: http.MethodDelete, URL: cell.lbURL + "/api/auth/sessions/" + sessions[0].ID, Token: auth.AccessToken,
	})
	upstreams.add(deleteResp.Header.Get(upstreamHeader))
	if deleteResp.Status != http.StatusNoContent {
		t.Fatalf("revoke session %s status=%d body=%s", sessions[0].ID, deleteResp.Status, deleteResp.Body)
	}

	final := crossnode.WaitForAuthMeStatus(t, cell.lbURL, auth.AccessToken, http.StatusUnauthorized, 15*time.Second)
	upstreams.add(final.Header.Get(upstreamHeader))
	if final.Status != http.StatusUnauthorized {
		t.Fatalf("LB access token was not revoked: final=%d body=%s", final.Status, final.Body)
	}

	refresh := crossnode.Do(t, crossnode.RawRequest{
		Method: http.MethodPost, URL: cell.lbURL + "/api/auth/refresh",
		Body: map[string]string{"refreshToken": auth.RefreshToken},
	})
	upstreams.add(refresh.Header.Get(upstreamHeader))
	if refresh.Status != http.StatusUnauthorized {
		t.Fatalf("LB refresh token was not invalidated: status=%d body=%s", refresh.Status, refresh.Body)
	}
}

// proveStorageIsolationAcrossNodes uploads as tenant A through the LB, reads it
// back same-tenant (200) and cross-tenant (404) through the LB — the shared
// MinIO backend plus PG tenant scoping is what makes this cross-node.
func proveStorageIsolationAcrossNodes(t *testing.T, cell *cell, upstreams *upstreamSet) {
	t.Helper()
	userA := crossnode.RegisterUser(t, cell.lbURL, "cell-storage-a@example.com")
	userB := crossnode.RegisterUser(t, cell.lbURL, "cell-storage-b@example.com")
	bucket := "cell-storage-" + crossnode.RandomHex(t, 4)
	filename := "tenant-proof-" + crossnode.RandomHex(t, 4) + ".txt"
	body := "cell-storage-write-read-" + crossnode.RandomHex(t, 8)

	uploadResp, _ := crossnode.Upload(t, cell.lbURL, bucket, filename, body, userA.AccessToken)
	upstreams.add(uploadResp.Header.Get(upstreamHeader))
	if uploadResp.Status != http.StatusCreated {
		t.Fatalf("tenant A upload status=%d body=%s", uploadResp.Status, uploadResp.Body)
	}

	sameTenant := crossnode.WaitForStorageBody(t, cell.lbURL, bucket, filename, userA.AccessToken, http.StatusOK, body, 15*time.Second)
	upstreams.add(sameTenant.Header.Get(upstreamHeader))
	if sameTenant.Status != http.StatusOK || sameTenant.Body != body {
		t.Fatalf("same-tenant read mismatch: status=%d body=%q want=%q", sameTenant.Status, sameTenant.Body, body)
	}

	crossTenant := crossnode.Do(t, crossnode.RawRequest{
		Method: http.MethodGet,
		URL:    crossnode.StorageObjectURL(cell.lbURL, bucket, filename),
		Token:  userB.AccessToken,
	})
	upstreams.add(crossTenant.Header.Get(upstreamHeader))
	if crossTenant.Status != http.StatusNotFound {
		t.Fatalf("cross-tenant read status=%d body=%s want=%d", crossTenant.Status, crossTenant.Body, http.StatusNotFound)
	}
}
