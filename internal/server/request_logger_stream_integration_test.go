//go:build integration

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testSSEEvent struct {
	Name string
	Data json.RawMessage
}

func TestAdminRequestLogsStreamEmitsNewMatchingRealRequest(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 1, 60)
	token := requestAdminToken(t, srv)
	httpSrv := httptest.NewServer(srv.Router())
	defer httpSrv.Close()

	runID := time.Now().UnixNano()
	matchingPath := fmt.Sprintf("/stage3-stream/%d/match", runID)
	nonmatchingPath := fmt.Sprintf("/stage3-stream/%d/nope", runID)
	matchingRequestID := fmt.Sprintf("stage3-stream-match-%d", runID)
	nonmatchingRequestID := fmt.Sprintf("stage3-stream-filter-%d", runID)

	query := url.Values{
		"method":       {http.MethodGet},
		"path":         {matchingPath},
		"status_class": {"4xx"},
	}
	streamReq, err := http.NewRequest(
		http.MethodGet,
		httpSrv.URL+"/api/admin/analytics/requests/stream?"+query.Encode(),
		nil,
	)
	testutil.NoError(t, err)
	streamReq.Header.Set("Authorization", "Bearer "+token)
	streamResp, err := http.DefaultClient.Do(streamReq)
	testutil.NoError(t, err)
	defer streamResp.Body.Close()
	testutil.Equal(t, http.StatusOK, streamResp.StatusCode)
	testutil.Equal(t, "text/event-stream", streamResp.Header.Get("Content-Type"))

	events := readTestSSEEvents(streamResp.Body)
	waitForTestSSEEvent(t, events, "ready")

	sendLoggedRequest(t, httpSrv.URL+nonmatchingPath, nonmatchingRequestID)
	sendLoggedRequest(t, httpSrv.URL+matchingPath, matchingRequestID)

	event := waitForTestSSEEvent(t, events, "request_log")
	var payload adminRequestLogEntry
	testutil.NoError(t, json.Unmarshal(event.Data, &payload))
	wantID := waitForRequestLogIDByRequestID(t, db.Pool, matchingRequestID)
	testutil.Equal(t, wantID, payload.ID)
	testutil.Equal(t, http.MethodGet, payload.Method)
	testutil.Equal(t, matchingPath, payload.Path)
	testutil.Equal(t, http.StatusNotFound, payload.StatusCode)
	if payload.RequestID == nil {
		t.Fatal("streamed request log should include request_id")
	}
	testutil.Equal(t, matchingRequestID, *payload.RequestID)
	waitForRequestLogCount(t, db.Pool, []string{nonmatchingRequestID}, 1)

	testutil.NoError(t, streamResp.Body.Close())
	select {
	case _, ok := <-events:
		testutil.False(t, ok, "stream reader should exit after response body close")
	case <-time.After(2 * time.Second):
		t.Fatal("stream reader did not exit after response body close")
	}
}

func TestAdminRequestLogsStreamFiltersByTenantID(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 1, 60)
	token := requestAdminToken(t, srv)
	httpSrv := httptest.NewServer(srv.Router())
	defer httpSrv.Close()

	streamReq, err := http.NewRequest(
		http.MethodGet,
		httpSrv.URL+"/api/admin/analytics/requests/stream?path=%2Ftenant-stream&tenant_id=tenant_a",
		nil,
	)
	testutil.NoError(t, err)
	streamReq.Header.Set("Authorization", "Bearer "+token)
	streamResp, err := http.DefaultClient.Do(streamReq)
	testutil.NoError(t, err)
	defer streamResp.Body.Close()
	testutil.Equal(t, http.StatusOK, streamResp.StatusCode)

	events := readTestSSEEvents(streamResp.Body)
	waitForTestSSEEvent(t, events, "ready")

	baseTime := time.Now().UTC().Truncate(time.Second)
	seedRequestLogs(t, db.Pool, []seededRequestLog{
		{method: http.MethodGet, path: "/tenant-stream", status: 200, timestamp: baseTime, requestID: "tenant-stream-default"},
		{method: http.MethodGet, path: "/tenant-stream", status: 404, timestamp: baseTime.Add(time.Second), requestID: "tenant-stream-match", tenantID: "tenant_a"},
	})

	event := waitForTestSSEEvent(t, events, "request_log")
	var payload adminRequestLogEntry
	testutil.NoError(t, json.Unmarshal(event.Data, &payload))
	if payload.RequestID == nil {
		t.Fatal("streamed tenant-filtered request log should include request_id")
	}
	testutil.Equal(t, "tenant-stream-match", *payload.RequestID)
	testutil.Equal(t, http.StatusNotFound, payload.StatusCode)

	select {
	case extraEvent, ok := <-events:
		if ok && extraEvent.Name == "request_log" {
			t.Fatalf("tenant-filtered stream should ignore nonmatching rows: %s", extraEvent.Data)
		}
	case <-time.After(500 * time.Millisecond):
	}
}

func sendLoggedRequest(t *testing.T, url, requestID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	testutil.NoError(t, err)
	req.Header.Set("X-Request-Id", requestID)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func waitForRequestLogIDByRequestID(t *testing.T, pool *pgxpool.Pool, requestID string) string {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var id string
		err := pool.QueryRow(ctx, `SELECT id FROM _ayb_request_logs WHERE request_id = $1`, requestID).Scan(&id)
		if err == nil {
			return id
		}
		if err != pgx.ErrNoRows {
			t.Fatalf("query request log ID by request_id: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for request log ID %q", requestID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readTestSSEEvents(body io.Reader) <-chan testSSEEvent {
	events := make(chan testSSEEvent)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(body)
		var eventName string
		var dataLines []string
		flush := func() {
			if eventName == "" && len(dataLines) == 0 {
				return
			}
			name := eventName
			if name == "" {
				name = "message"
			}
			events <- testSSEEvent{Name: name, Data: json.RawMessage(strings.Join(dataLines, "\n"))}
			eventName = ""
			dataLines = nil
		}
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				flush()
			case strings.HasPrefix(line, "event:"):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		flush()
	}()
	return events
}

func waitForTestSSEEvent(t *testing.T, events <-chan testSSEEvent, name string) testSSEEvent {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("stream closed before %q event", name)
			}
			if event.Name == name {
				return event
			}
			if event.Name == "error" {
				t.Fatalf("stream returned error event while waiting for %q: %s", name, event.Data)
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %q event", name)
		}
	}
}
