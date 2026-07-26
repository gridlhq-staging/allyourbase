//go:build integration

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminRequestLogsAggregateEndpointBucketsByMinuteAndStatusClass(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 60)

	seedRequestLogs(t, db.Pool, []seededRequestLog{
		{method: http.MethodGet, path: "/aggregate/orders", status: 200, timestamp: time.Date(2026, 5, 4, 10, 15, 3, 0, time.UTC), requestID: "aggregate-match-2xx"},
		{method: http.MethodGet, path: "/aggregate/orders/1", status: 500, timestamp: time.Date(2026, 5, 4, 10, 15, 20, 0, time.UTC), requestID: "aggregate-match-500"},
		{method: http.MethodGet, path: "/aggregate/orders/2", status: 503, timestamp: time.Date(2026, 5, 4, 10, 15, 55, 0, time.UTC), requestID: "aggregate-match-503"},
		{method: http.MethodGet, path: "/aggregate/orders/3", status: 404, timestamp: time.Date(2026, 5, 4, 10, 16, 10, 0, time.UTC), requestID: "aggregate-match-404"},
		{method: http.MethodPost, path: "/aggregate/orders", status: 200, timestamp: time.Date(2026, 5, 4, 10, 15, 30, 0, time.UTC), requestID: "aggregate-filter-method"},
		{method: http.MethodGet, path: "/aggregate/customers", status: 500, timestamp: time.Date(2026, 5, 4, 10, 16, 30, 0, time.UTC), requestID: "aggregate-filter-path"},
	})
	token := requestAdminToken(t, srv)

	query := requestLogAggregateFixtureQuery()
	response := requestAdminRequestLogsAggregate(t, srv, token, query)

	testutil.Equal(t, 2, response.Count)
	if len(response.Items) != 2 {
		t.Fatalf("aggregate bucket count: got %d, want 2", len(response.Items))
	}
	assertRequestLogAggregateBucket(t, response.Items[0], time.Date(2026, 5, 4, 10, 15, 0, 0, time.UTC), 3, 1, 0, 0, 2)
	assertRequestLogAggregateBucket(t, response.Items[1], time.Date(2026, 5, 4, 10, 16, 0, 0, time.UTC), 1, 0, 0, 1, 0)

	query.Set("status_class", "5xx")
	response = requestAdminRequestLogsAggregate(t, srv, token, query)

	testutil.Equal(t, 1, response.Count)
	if len(response.Items) != 1 {
		t.Fatalf("5xx aggregate bucket count: got %d, want 1", len(response.Items))
	}
	assertRequestLogAggregateBucket(t, response.Items[0], time.Date(2026, 5, 4, 10, 15, 0, 0, time.UTC), 2, 0, 0, 0, 2)
}

func TestAdminRequestLogsAggregateEndpointFiltersByTenantID(t *testing.T) {
	db := newRequestLoggerTestDB(t)
	srv := newRequestLoggerServerForIntegration(t, db.Pool, 100, 60)

	baseTime := time.Date(2026, 5, 4, 10, 15, 0, 0, time.UTC)
	seedRequestLogs(t, db.Pool, []seededRequestLog{
		{method: http.MethodGet, path: "/aggregate-tenants", status: 200, timestamp: baseTime, requestID: "aggregate-tenant-a-1", tenantID: "tenant_a"},
		{method: http.MethodGet, path: "/aggregate-tenants", status: 500, timestamp: baseTime, requestID: "aggregate-tenant-a-2", tenantID: "tenant_a"},
		{method: http.MethodGet, path: "/aggregate-tenants", status: 404, timestamp: baseTime.Add(time.Minute), requestID: "aggregate-tenant-default"},
	})
	token := requestAdminToken(t, srv)

	tenantAResponse := requestAdminRequestLogsAggregate(t, srv, token, url.Values{
		"path":      {"/aggregate-tenants"},
		"tenant_id": {"tenant_a"},
	})
	testutil.Equal(t, 1, tenantAResponse.Count)
	if len(tenantAResponse.Items) != 1 {
		t.Fatalf("tenant aggregate bucket count: got %d, want 1", len(tenantAResponse.Items))
	}
	assertRequestLogAggregateBucket(
		t,
		tenantAResponse.Items[0],
		baseTime,
		2,
		1,
		0,
		0,
		1,
	)

	defaultTenantResponse := requestAdminRequestLogsAggregate(t, srv, token, url.Values{
		"path":      {"/aggregate-tenants"},
		"tenant_id": {""},
	})
	testutil.Equal(t, 1, defaultTenantResponse.Count)
	if len(defaultTenantResponse.Items) != 1 {
		t.Fatalf("default-tenant aggregate bucket count: got %d, want 1", len(defaultTenantResponse.Items))
	}
	assertRequestLogAggregateBucket(
		t,
		defaultTenantResponse.Items[0],
		baseTime.Add(time.Minute),
		1,
		0,
		0,
		1,
		0,
	)

	unfilteredResponse := requestAdminRequestLogsAggregate(t, srv, token, url.Values{
		"path": {"/aggregate-tenants"},
	})
	testutil.Equal(t, 2, unfilteredResponse.Count)
}

func requestLogAggregateFixtureQuery() url.Values {
	query := url.Values{}
	query.Set("method", http.MethodGet)
	query.Set("path", "/aggregate/orders*")
	query.Set("from", "2026-05-04T10:15:00Z")
	query.Set("to", "2026-05-04T10:16:59Z")
	return query
}

func requestAdminRequestLogsAggregate(
	t *testing.T,
	srv *Server,
	token string,
	query url.Values,
) testRequestLogAggregateResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics/requests/aggregate?"+query.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var response testRequestLogAggregateResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}

type testRequestLogAggregateResponse struct {
	Items []testRequestLogAggregateBucket `json:"items"`
	Count int                             `json:"count"`
}

type testRequestLogAggregateBucket struct {
	Bucket    time.Time `json:"bucket"`
	Count     int       `json:"count"`
	Status2xx int       `json:"status_2xx"`
	Status3xx int       `json:"status_3xx"`
	Status4xx int       `json:"status_4xx"`
	Status5xx int       `json:"status_5xx"`
}

func assertRequestLogAggregateBucket(
	t *testing.T,
	item testRequestLogAggregateBucket,
	bucket time.Time,
	count,
	status2xx,
	status3xx,
	status4xx,
	status5xx int,
) {
	t.Helper()
	testutil.Equal(t, bucket, item.Bucket)
	testutil.Equal(t, count, item.Count)
	testutil.Equal(t, status2xx, item.Status2xx)
	testutil.Equal(t, status3xx, item.Status3xx)
	testutil.Equal(t, status4xx, item.Status4xx)
	testutil.Equal(t, status5xx, item.Status5xx)
}
