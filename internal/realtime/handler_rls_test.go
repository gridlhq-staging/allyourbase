package realtime_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSSERLSFiltersBeforeSubscriptionFilter verifies that the SSE transport
// applies CanSeeRecord before subscription filters. Both events match the
// status filter; publishing the denied tenant-b row first proves a permissive
// filter cannot bypass the per-record RLS gate.
func TestSSERLSFiltersBeforeSubscriptionFilter(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	h := realtime.NewHandler(hub, &pgxpool.Pool{}, testAuthService(), testSchemaCache("orders"), testutil.DiscardLogger())

	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"?tables=orders&filter=status=eq.pending", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+validTokenForTenant("tenant-a"))

	resp, err := (&http.Client{}).Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.Equal(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	connected := readNextSSEData(t, scanner)
	testutil.NotNil(t, connected["clientId"])

	hub.Publish(&realtime.Event{
		Action:   "create",
		Table:    "orders",
		TenantID: "tenant-b",
		Record: map[string]any{
			"id":     201,
			"status": "pending",
			"title":  "denied-tenant-b-sse",
		},
	})
	hub.Publish(&realtime.Event{
		Action:   "create",
		Table:    "orders",
		TenantID: "tenant-a",
		Record: map[string]any{
			"id":     202,
			"status": "pending",
			"title":  "allowed-tenant-a-sse",
		},
	})

	evData := readNextSSEData(t, scanner)
	testutil.Equal(t, "create", evData["action"])
	testutil.Equal(t, "orders", evData["table"])
	testutil.Equal(t, "tenant-a", evData["tenant_id"])
	record, ok := evData["record"].(map[string]any)
	testutil.True(t, ok, "event should contain a record object")
	id, ok := record["id"].(float64)
	testutil.True(t, ok, "record id should decode as a JSON number")
	testutil.Equal(t, float64(202), id)
	testutil.Equal(t, "pending", record["status"])
	testutil.Equal(t, "allowed-tenant-a-sse", record["title"])
	payload, err := json.Marshal(evData)
	testutil.NoError(t, err)
	testutil.False(t, strings.Contains(string(payload), "denied-tenant-b-sse"), "denied tenant-b SSE payload must be absent")
}
