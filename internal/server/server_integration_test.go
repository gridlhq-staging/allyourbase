//go:build integration

package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func createIntegrationTestSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email VARCHAR(255) UNIQUE
		)
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func setupServerWithTenantForRateLimit(t *testing.T, reqRateHard, reqRateSoft *int) (*httptest.Server, *tenant.TenantRateLimiter, string) {
	t.Helper()

	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	tenantSvc := tenant.NewService(sharedPG.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(sharedPG.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})

	rl := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(rl)

	connCounter := tenant.NewTenantConnCounter()
	srv.SetTenantConnCounter(connCounter)

	tenantEnt, err := tenantSvc.CreateTenant(ctx, "rate-limit-tenant", fmt.Sprintf("rate-limit-tenant-%d", time.Now().UnixNano()), "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	_, err = tenantSvc.SetQuotas(ctx, tenantEnt.ID, tenant.TenantQuotas{
		RequestRateRPSHard:      reqRateHard,
		RequestRateRPSSoft:      reqRateSoft,
		DBSizeBytesHard:         nil,
		DBSizeBytesSoft:         nil,
		RealtimeConnectionsHard: nil,
		RealtimeConnectionsSoft: nil,
	})
	testutil.NoError(t, err)

	return httptest.NewServer(srv.Router()), rl, tenantEnt.ID
}

func setupServerWithTenantForRealtimeLimit(t *testing.T, realtimeHard, realtimeSoft *int) (*httptest.Server, string) {
	t.Helper()

	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	tenantSvc := tenant.NewService(sharedPG.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(sharedPG.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())

	tenantEnt, err := tenantSvc.CreateTenant(ctx, "realtime-tenant", fmt.Sprintf("realtime-tenant-%d", time.Now().UnixNano()), "schema", "free", "default", nil, "")
	testutil.NoError(t, err)

	_, err = tenantSvc.SetQuotas(ctx, tenantEnt.ID, tenant.TenantQuotas{
		RealtimeConnectionsHard: realtimeHard,
		RealtimeConnectionsSoft: realtimeSoft,
		RequestRateRPSHard:      nil,
		RequestRateRPSSoft:      nil,
		DBSizeBytesHard:         nil,
		DBSizeBytesSoft:         nil,
	})
	testutil.NoError(t, err)

	return httptest.NewServer(srv.Router()), tenantEnt.ID
}

func dialTenantRealtimeWS(t *testing.T, baseURL, tenantID string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/realtime/ws"
	header := http.Header{}
	header.Set("X-Tenant-ID", tenantID)

	return websocket.DefaultDialer.Dial(wsURL, header)
}

func ensureIntegrationMigrations(t *testing.T, ctx context.Context) {
	t.Helper()

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func readNextSSEEvent(t *testing.T, scanner *bufio.Scanner, timeout time.Duration, timeoutMessage string) []string {
	t.Helper()
	eventCh := make(chan []string, 1)
	go func() {
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" && len(lines) > 1 {
				break
			}
		}
		eventCh <- lines
	}()

	select {
	case lines := <-eventCh:
		return lines
	case <-time.After(timeout):
		t.Fatal(timeoutMessage)
		return nil
	}
}

func decodeCommittedRPCRow(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var payload any
	testutil.NoError(t, json.Unmarshal(body, &payload))

	switch value := payload.(type) {
	case map[string]any:
		return value
	case []any:
		testutil.True(t, len(value) == 1, "expected one row from RPC notify function")
		row, ok := value[0].(map[string]any)
		testutil.True(t, ok, "expected first RPC row to be an object")
		return row
	default:
		t.Fatalf("unexpected RPC payload shape %T: %s", payload, string(body))
		return nil
	}
}

func decodeSSEEventPayload(t *testing.T, lines []string) map[string]any {
	t.Helper()

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		testutil.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		return event
	}
	t.Fatalf("expected SSE data frame in %#v", lines)
	return nil
}

func apiJSONRequest(t *testing.T, method, url string, body any, wantStatus int) []byte {
	t.Helper()
	return apiJSONRequestWithToken(t, method, url, body, "", wantStatus)
}

func apiJSONRequestWithToken(t *testing.T, method, url string, body any, token string, wantStatus int) []byte {
	t.Helper()
	return apiJSONRequestWithTokenAndTenant(t, method, url, body, token, "", wantStatus)
}

func apiJSONRequestWithTokenAndTenant(t *testing.T, method, url string, body any, token, tenantID string, wantStatus int) []byte {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		testutil.NoError(t, err)
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reader)
	testutil.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, wantStatus, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	testutil.NoError(t, err)
	return respBody
}

func decodeAPIRecord(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var record map[string]any
	testutil.NoError(t, json.Unmarshal(body, &record))
	return record
}

func assertRealtimeRecordValue(t *testing.T, record map[string]any, key string, want any) {
	t.Helper()
	testutil.Equal(t, fmt.Sprint(want), fmt.Sprint(record[key]))
}

func assertSSETableEvent(t *testing.T, lines []string, action, table string) map[string]any {
	t.Helper()

	event := decodeSSEEventPayload(t, lines)
	testutil.Equal(t, action, fmt.Sprint(event["action"]))
	testutil.Equal(t, table, fmt.Sprint(event["table"]))
	record, ok := event["record"].(map[string]any)
	testutil.True(t, ok, "expected record object in SSE event")
	return record
}

func assertNoSSEEvent(t *testing.T, scanner *bufio.Scanner, body io.Closer, duration time.Duration, message string) {
	t.Helper()

	eventCh := make(chan []string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if line == "" && len(lines) > 1 {
				break
			}
		}
		eventCh <- lines
	}()

	select {
	case lines := <-eventCh:
		t.Fatalf("%s: %#v", message, lines)
	case <-time.After(duration):
	}
	testutil.NoError(t, body.Close())
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s: SSE scanner did not stop after closing response body", message)
	}
}

func newRealtimeTwoNodeTestServers(t *testing.T, ctx context.Context, authSvc *auth.Service) (*httptest.Server, *httptest.Server) {
	t.Helper()

	logger := testutil.DiscardLogger()
	chA := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, chA.Load(ctx))
	chB := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, chB.Load(ctx))

	cfg := config.Default()
	cfg.Database.URL = sharedPG.ConnString
	if authSvc != nil {
		cfg.Auth.Enabled = true
		cfg.Auth.JWTSecret = realtimeTestJWTSecret
	}

	nodeA := server.New(cfg, logger, chA, sharedPG.Pool, authSvc, nil)
	nodeB := server.New(cfg, logger, chB, sharedPG.Pool, authSvc, nil)
	tenantSvcA := tenant.NewService(sharedPG.Pool, logger)
	tenantSvcB := tenant.NewService(sharedPG.Pool, logger)
	nodeA.SetTenantService(tenantSvcA)
	nodeB.SetTenantService(tenantSvcB)
	t.Cleanup(func() {
		_ = nodeA.Shutdown(context.Background())
		_ = nodeB.Shutdown(context.Background())
	})

	tsA := httptest.NewServer(nodeA.Router())
	t.Cleanup(tsA.Close)
	tsB := httptest.NewServer(nodeB.Router())
	t.Cleanup(tsB.Close)
	return tsA, tsB
}

const realtimeTestJWTSecret = "realtime-test-secret-that-is-at-least-32-chars"

func issueRealtimeTestToken(t *testing.T, authSvc *auth.Service, userID string) string {
	t.Helper()

	token, err := authSvc.IssueTestToken(userID, userID+"@example.com")
	testutil.NoError(t, err)
	return token
}

func issueRealtimeTenantTestToken(t *testing.T, userID, tenantID string) string {
	t.Helper()

	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email:    userID + "@example.com",
		TenantID: tenantID,
		AAL:      "aal1",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(realtimeTestJWTSecret))
	testutil.NoError(t, err)
	return token
}

func setupSecureDocsRealtimeFixture(t *testing.T, ctx context.Context) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	ensureIntegrationMigrations(t, ctx)

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		);

		CREATE TABLE project_memberships (
			user_id TEXT NOT NULL,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			PRIMARY KEY (user_id, project_id)
		);

		CREATE TABLE secure_docs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL
		);

		ALTER TABLE secure_docs ENABLE ROW LEVEL SECURITY;
		ALTER TABLE secure_docs FORCE ROW LEVEL SECURITY;

		CREATE POLICY secure_docs_membership_select
			ON secure_docs
			FOR SELECT
			USING (
				EXISTS (
					SELECT 1
					FROM project_memberships pm
					WHERE pm.project_id = secure_docs.project_id
					  AND pm.user_id = current_setting('ayb.user_id', true)
				)
			);

		CREATE POLICY secure_docs_membership_insert
			ON secure_docs
			FOR INSERT
			WITH CHECK (
				EXISTS (
					SELECT 1
					FROM project_memberships pm
					WHERE pm.project_id = secure_docs.project_id
					  AND pm.user_id = current_setting('ayb.user_id', true)
				)
			);

		CREATE POLICY secure_docs_membership_update
			ON secure_docs
			FOR UPDATE
			USING (
				EXISTS (
					SELECT 1
					FROM project_memberships pm
					WHERE pm.project_id = secure_docs.project_id
					  AND pm.user_id = current_setting('ayb.user_id', true)
				)
			)
			WITH CHECK (
				EXISTS (
					SELECT 1
					FROM project_memberships pm
					WHERE pm.project_id = secure_docs.project_id
					  AND pm.user_id = current_setting('ayb.user_id', true)
				)
			);

		CREATE POLICY secure_docs_membership_delete
			ON secure_docs
			FOR DELETE
			USING (
				EXISTS (
					SELECT 1
					FROM project_memberships pm
					WHERE pm.project_id = secure_docs.project_id
					  AND pm.user_id = current_setting('ayb.user_id', true)
				)
			);

		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT SELECT ON projects, project_memberships TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON secure_docs TO ayb_authenticated;

		INSERT INTO projects (id, name) VALUES ('project-1', 'Project One');
		INSERT INTO project_memberships (user_id, project_id) VALUES ('user-allowed', 'project-1');
	`)
	testutil.NoError(t, err)
}

func TestSchemaEndpointReturnsValidJSON(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	err := ch.Load(ctx)
	testutil.NoError(t, err)

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/schema", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Should be valid JSON with tables.
	var result schema.SchemaCache
	err = json.Unmarshal(w.Body.Bytes(), &result)
	testutil.NoError(t, err)
	testutil.True(t, len(result.Tables) >= 1, "expected at least 1 table")
	testutil.NotNil(t, result.Tables["public.users"])
}

func TestTenantRequestRateQuotaRejectsHardLimit(t *testing.T) {
	hard := 1
	ts, rl, tenantID := setupServerWithTenantForRateLimit(t, &hard, nil)
	defer ts.Close()
	defer rl.Stop()

	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
		req.Header.Set("X-Tenant-ID", tenantID)
		ts.Config.Handler.ServeHTTP(w, req)
		testutil.StatusCode(t, http.StatusOK, w.Code)
	}

	over := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.Header.Set("X-Tenant-ID", tenantID)
	ts.Config.Handler.ServeHTTP(over, req)
	testutil.Equal(t, http.StatusTooManyRequests, over.Code)
	retryAfter := over.Header().Get("Retry-After")
	testutil.True(t, retryAfter != "")
	_, parseErr := strconv.Atoi(retryAfter)
	testutil.NoError(t, parseErr)
}

func TestTenantRealtimeConnectionsQuotaRejectsOverHardLimit(t *testing.T) {
	hard := 1
	ts, tenantID := setupServerWithTenantForRealtimeLimit(t, &hard, nil)
	defer ts.Close()

	c1, _, err := dialTenantRealtimeWS(t, ts.URL, tenantID)
	testutil.NoError(t, err)
	defer c1.Close()

	_, cResp, err := dialTenantRealtimeWS(t, ts.URL, tenantID)
	testutil.True(t, err != nil)
	testutil.NotNil(t, cResp)
	testutil.Equal(t, http.StatusTooManyRequests, cResp.StatusCode)

	testutil.NoError(t, c1.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second)))
	time.Sleep(100 * time.Millisecond)

	c2, _, err := dialTenantRealtimeWS(t, ts.URL, tenantID)
	testutil.NoError(t, err)
	_ = c2.Close()
}

// TestRealtimeSSEReceivesCreateEvent verifies the full end-to-end flow:
// connect SSE → create record via API → receive the realtime event.
func TestRealtimeSSEReceivesCreateEvent(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	// Start a real HTTP server so SSE streaming works.
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Connect to SSE endpoint.
	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)

	// Read and verify connected event.
	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")
	testutil.Equal(t, "event: connected", connected[0])

	// Create a record via the API.
	body, _ := json.Marshal(map[string]any{"name": "Charlie", "email": "charlie@example.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	createResp.Body.Close()

	// Read the create event from SSE.
	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE create event")
	testutil.True(t, len(lines) >= 1, "should have event lines")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"create"`)
	testutil.Contains(t, joined, `"table":"users"`)
	testutil.Contains(t, joined, `"Charlie"`)
}

func TestRealtimeSSECrossNodeTableDelivery(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	tsA, tsB := newRealtimeTwoNodeTestServers(t, ctx, nil)

	resp, err := http.Get(tsB.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for cross-node SSE connected event on node B")
	testutil.Equal(t, "event: connected", connected[0])

	uniqueName := fmt.Sprintf("CrossNodeSSE%d", time.Now().UnixNano())
	created := decodeAPIRecord(t, apiJSONRequest(t, http.MethodPost, tsA.URL+"/api/collections/users/",
		map[string]any{"name": uniqueName, "email": strings.ToLower(uniqueName) + "@example.com"},
		http.StatusCreated))
	userID := fmt.Sprint(created["id"])

	lines := readNextSSEEvent(t, scanner, 2*time.Second, "timed out waiting for cross-node SSE create event from node A on node B")
	createRecord := assertSSETableEvent(t, lines, "create", "users")
	assertRealtimeRecordValue(t, createRecord, "id", created["id"])
	assertRealtimeRecordValue(t, createRecord, "name", uniqueName)
	assertRealtimeRecordValue(t, createRecord, "email", strings.ToLower(uniqueName)+"@example.com")

	updatedName := uniqueName + "_Updated"
	updated := decodeAPIRecord(t, apiJSONRequest(t, http.MethodPatch, tsA.URL+"/api/collections/users/"+userID,
		map[string]any{"name": updatedName, "email": strings.ToLower(uniqueName) + "@example.com"},
		http.StatusOK))

	lines = readNextSSEEvent(t, scanner, 2*time.Second, "timed out waiting for cross-node SSE update event from node A on node B")
	updateRecord := assertSSETableEvent(t, lines, "update", "users")
	assertRealtimeRecordValue(t, updateRecord, "id", updated["id"])
	assertRealtimeRecordValue(t, updateRecord, "name", updatedName)
	assertRealtimeRecordValue(t, updateRecord, "email", strings.ToLower(uniqueName)+"@example.com")

	apiJSONRequest(t, http.MethodDelete, tsA.URL+"/api/collections/users/"+userID, nil, http.StatusNoContent)

	lines = readNextSSEEvent(t, scanner, 2*time.Second, "timed out waiting for cross-node SSE delete event from node A on node B")
	deleteRecord := assertSSETableEvent(t, lines, "delete", "users")
	assertRealtimeRecordValue(t, deleteRecord, "id", userID)
}

func TestRealtimeSSECrossNodeRLSFiltersClaimVisibilityAndDelete(t *testing.T) {
	ctx := context.Background()
	setupSecureDocsRealtimeFixture(t, ctx)

	authSvc := auth.NewService(sharedPG.Pool, realtimeTestJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
	tsA, tsB := newRealtimeTwoNodeTestServers(t, ctx, authSvc)
	allowedToken := issueRealtimeTestToken(t, authSvc, "user-allowed")

	allowedReq, err := http.NewRequest(http.MethodGet, tsB.URL+"/api/realtime?tables=secure_docs", nil)
	testutil.NoError(t, err)
	allowedReq.Header.Set("Authorization", "Bearer "+allowedToken)
	allowedResp, err := http.DefaultClient.Do(allowedReq)
	testutil.NoError(t, err)
	defer allowedResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, allowedResp.StatusCode)

	deniedReq, err := http.NewRequest(http.MethodGet, tsB.URL+"/api/realtime?tables=secure_docs", nil)
	testutil.NoError(t, err)
	deniedReq.Header.Set("Authorization", "Bearer "+issueRealtimeTestToken(t, authSvc, "user-denied"))
	deniedResp, err := http.DefaultClient.Do(deniedReq)
	testutil.NoError(t, err)
	defer deniedResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, deniedResp.StatusCode)

	allowedScanner := bufio.NewScanner(allowedResp.Body)
	deniedScanner := bufio.NewScanner(deniedResp.Body)
	readNextSSEEvent(t, allowedScanner, 5*time.Second, "timed out waiting for allowed SSE connected event")
	readNextSSEEvent(t, deniedScanner, 5*time.Second, "timed out waiting for denied SSE connected event")

	created := decodeAPIRecord(t, apiJSONRequestWithToken(t, http.MethodPost, tsA.URL+"/api/collections/secure_docs/",
		map[string]any{"id": "doc-route-1", "project_id": "project-1", "title": "Visible Draft"},
		allowedToken,
		http.StatusCreated))

	lines := readNextSSEEvent(t, allowedScanner, 3*time.Second, "allowed user should receive secure_docs create event")
	createRecord := assertSSETableEvent(t, lines, "create", "secure_docs")
	assertRealtimeRecordValue(t, createRecord, "id", created["id"])
	assertRealtimeRecordValue(t, createRecord, "project_id", "project-1")
	assertRealtimeRecordValue(t, createRecord, "title", "Visible Draft")

	updated := decodeAPIRecord(t, apiJSONRequestWithToken(t, http.MethodPatch, tsA.URL+"/api/collections/secure_docs/doc-route-1",
		map[string]any{"title": "Visible Final"},
		allowedToken,
		http.StatusOK))

	lines = readNextSSEEvent(t, allowedScanner, 3*time.Second, "allowed user should receive secure_docs update event")
	updateRecord := assertSSETableEvent(t, lines, "update", "secure_docs")
	assertRealtimeRecordValue(t, updateRecord, "id", updated["id"])
	assertRealtimeRecordValue(t, updateRecord, "title", "Visible Final")

	apiJSONRequestWithToken(t, http.MethodDelete, tsA.URL+"/api/collections/secure_docs/doc-route-1", nil, allowedToken, http.StatusNoContent)

	lines = readNextSSEEvent(t, allowedScanner, 3*time.Second, "allowed user should receive secure_docs delete event")
	deleteRecord := assertSSETableEvent(t, lines, "delete", "secure_docs")
	assertRealtimeRecordValue(t, deleteRecord, "id", "doc-route-1")
	assertNoSSEEvent(t, deniedScanner, deniedResp.Body, 250*time.Millisecond, "denied user received secure_docs delete event")
}

func TestRealtimeSSESchemaIsolatedTenantUsesActiveSchemaForDelivery(t *testing.T) {
	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	tenantSvc := tenant.NewService(sharedPG.Pool, logger)
	slugBase := fmt.Sprintf("rt-schema-%d", time.Now().UnixNano())
	tenantA, err := tenantSvc.CreateTenant(ctx, "Realtime Schema A", slugBase+"-a", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	tenantB, err := tenantSvc.CreateTenant(ctx, "Realtime Schema B", slugBase+"-b", "schema", "free", "default", nil, "")
	testutil.NoError(t, err)
	createRealtimeSchemaIsolationTables(t, ctx, tenantA.Slug, tenantB.Slug)

	authSvc := auth.NewService(sharedPG.Pool, realtimeTestJWTSecret, time.Hour, 7*24*time.Hour, 8, logger)
	tsA, tsB := newRealtimeTwoNodeTestServers(t, ctx, authSvc)
	tokenA := issueRealtimeTenantTestToken(t, "schema-rt-user-a", tenantA.ID)
	tokenB := issueRealtimeTenantTestToken(t, "schema-rt-user-b", tenantB.ID)

	reqA, err := http.NewRequest(http.MethodGet, tsB.URL+"/api/realtime?tables=live_items", nil)
	testutil.NoError(t, err)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	respA, err := http.DefaultClient.Do(reqA)
	testutil.NoError(t, err)
	defer respA.Body.Close()
	testutil.StatusCode(t, http.StatusOK, respA.StatusCode)

	reqB, err := http.NewRequest(http.MethodGet, tsB.URL+"/api/realtime?tables=live_items", nil)
	testutil.NoError(t, err)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	respB, err := http.DefaultClient.Do(reqB)
	testutil.NoError(t, err)
	defer respB.Body.Close()
	testutil.StatusCode(t, http.StatusOK, respB.StatusCode)

	scannerA := bufio.NewScanner(respA.Body)
	scannerB := bufio.NewScanner(respB.Body)
	readNextSSEEvent(t, scannerA, 5*time.Second, "timed out waiting for tenant A SSE connected event")
	readNextSSEEvent(t, scannerB, 5*time.Second, "timed out waiting for tenant B SSE connected event")

	created := decodeAPIRecord(t, apiJSONRequestWithToken(t, http.MethodPost, tsA.URL+"/api/collections/live_items/",
		map[string]any{"id": "tenant-a-row", "tenant_note": "tenant-a-visible"},
		tokenA,
		http.StatusCreated))

	lines := readNextSSEEvent(t, scannerA, 3*time.Second, "tenant A should receive schema-isolated live_items create event")
	record := assertSSETableEvent(t, lines, "create", "live_items")
	assertRealtimeRecordValue(t, record, "id", created["id"])
	assertRealtimeRecordValue(t, record, "tenant_note", "tenant-a-visible")
	testutil.True(t, record["tenant_b_note"] == nil, "tenant A event must not use tenant B metadata")
	assertNoSSEEvent(t, scannerB, respB.Body, 250*time.Millisecond, "tenant B received tenant A schema-isolated event")

	peerReq, err := http.NewRequest(http.MethodGet, tsB.URL+"/api/realtime?tables=peer_only", nil)
	testutil.NoError(t, err)
	peerReq.Header.Set("Authorization", "Bearer "+tokenA)
	peerResp, err := http.DefaultClient.Do(peerReq)
	testutil.NoError(t, err)
	defer peerResp.Body.Close()
	testutil.StatusCode(t, http.StatusBadRequest, peerResp.StatusCode)
}

func createRealtimeSchemaIsolationTables(t *testing.T, ctx context.Context, slugA, slugB string) {
	t.Helper()

	schemaA := pgx.Identifier{slugA}.Sanitize()
	schemaB := pgx.Identifier{slugB}.Sanitize()
	_, err := sharedPG.Pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE public.live_items (id TEXT PRIMARY KEY, public_note TEXT NOT NULL);
		CREATE TABLE %s.live_items (id TEXT PRIMARY KEY, tenant_note TEXT NOT NULL);
		CREATE TABLE %s.live_items (id TEXT PRIMARY KEY, tenant_b_note TEXT NOT NULL);
		CREATE TABLE %s.peer_only (id TEXT PRIMARY KEY, tenant_b_note TEXT NOT NULL);
		GRANT USAGE ON SCHEMA public, %s, %s TO ayb_authenticated;
		GRANT SELECT ON public.live_items TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON %s.live_items TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON %s.live_items, %s.peer_only TO ayb_authenticated;
	`, schemaA, schemaB, schemaB, schemaA, schemaB, schemaA, schemaB, schemaB))
	testutil.NoError(t, err)
}

func TestRealtimeSSEOriginNodeDoesNotDuplicateBusEcho(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	tsA, _ := newRealtimeTwoNodeTestServers(t, ctx, nil)

	resp, err := http.Get(tsA.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for origin-node SSE connected event")

	uniqueName := fmt.Sprintf("OriginNodeSSE%d", time.Now().UnixNano())
	created := decodeAPIRecord(t, apiJSONRequest(t, http.MethodPost, tsA.URL+"/api/collections/users/",
		map[string]any{"name": uniqueName, "email": strings.ToLower(uniqueName) + "@example.com"},
		http.StatusCreated))

	lines := readNextSSEEvent(t, scanner, 3*time.Second, "timed out waiting for local origin-node SSE create event")
	createRecord := assertSSETableEvent(t, lines, "create", "users")
	assertRealtimeRecordValue(t, createRecord, "id", created["id"])
	assertRealtimeRecordValue(t, createRecord, "name", uniqueName)
	assertNoSSEEvent(t, scanner, resp.Body, 3*time.Second, "origin-node client received duplicate bus echo")
}

// TestAdminStatsWithDBPool verifies that the stats endpoint includes DB pool
// fields when a real database pool is available.
func TestAdminStatsWithDBPool(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	token := adminLogin(t, srv)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)

	testutil.StatusCode(t, http.StatusOK, w.Code)
	var stats map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))

	// With a real pool, DB stats should be present.
	testutil.NotNil(t, stats["db_pool_total"])
	testutil.NotNil(t, stats["db_pool_idle"])
	testutil.NotNil(t, stats["db_pool_in_use"])
	testutil.NotNil(t, stats["db_pool_max"])

	// Pool max should be positive.
	maxConns := stats["db_pool_max"].(float64)
	testutil.True(t, maxConns > 0, "db_pool_max should be positive")

	// Standard fields should also be present.
	testutil.NotNil(t, stats["go_version"])
	testutil.NotNil(t, stats["goroutines"])
}

// TestRealtimeSSEDoesNotReceiveUnsubscribedTable verifies that SSE clients
// only receive events for tables they subscribed to.
func TestRealtimeSSEDoesNotReceiveUnsubscribedTable(t *testing.T) {
	ctx := context.Background()

	// Reset schema with two tables.
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE logs (id SERIAL PRIMARY KEY, message TEXT NOT NULL);
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// Subscribe only to "users".
	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// Skip connected event.
	readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")

	// Create a log record (not subscribed).
	body, err := json.Marshal(map[string]any{"message": "hello"})
	testutil.NoError(t, err)
	cr, err := http.Post(ts.URL+"/api/collections/logs/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, cr.StatusCode)
	cr.Body.Close()

	// Create a user record (subscribed).
	body, err = json.Marshal(map[string]any{"name": "Dave"})
	testutil.NoError(t, err)
	cr, err = http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, cr.StatusCode)
	cr.Body.Close()

	// The next event should be for users, not logs.
	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE event")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"table":"users"`)
	testutil.Contains(t, joined, `"Dave"`)
	// Should NOT contain logs data.
	testutil.False(t, strings.Contains(joined, `"logs"`), "should not receive logs events")
}

func TestEdgeFuncAdminDeployInvokeAndPersistLogs(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"

	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	execPool := edgefunc.NewPool(cfg.EdgeFunctions.PoolSize)
	defer execPool.Close()

	store := edgefunc.NewPostgresStore(sharedPG.Pool)
	logStore := edgefunc.NewPostgresLogStore(sharedPG.Pool)
	svc := edgefunc.NewService(store, execPool, logStore,
		edgefunc.WithDefaultTimeout(time.Duration(cfg.EdgeFunctions.DefaultTimeoutMs)*time.Millisecond),
	)
	srv.SetEdgeFuncService(svc)

	adminToken := adminLogin(t, srv)

	fnName := fmt.Sprintf("itest-fn-%d", time.Now().UnixNano())
	source := `function handler(req) { return { statusCode: 201, body: req.method + "|" + req.path + "|" + req.query + "|" + req.body }; }`
	deployPayload := map[string]any{
		"name":   fnName,
		"source": source,
		"public": true,
	}
	deployBody, err := json.Marshal(deployPayload)
	testutil.NoError(t, err)

	deployReq := httptest.NewRequest(http.MethodPost, "/api/admin/functions", bytes.NewReader(deployBody))
	deployReq.Header.Set("Content-Type", "application/json")
	deployReq.Header.Set("Authorization", "Bearer "+adminToken)
	deployW := httptest.NewRecorder()
	srv.Router().ServeHTTP(deployW, deployReq)

	testutil.StatusCode(t, http.StatusCreated, deployW.Code)
	var deployed edgefunc.EdgeFunction
	testutil.NoError(t, json.Unmarshal(deployW.Body.Bytes(), &deployed))
	testutil.Equal(t, fnName, deployed.Name)

	invokeReq := httptest.NewRequest(http.MethodPost, "/functions/v1/"+fnName+"/nested/path?q=1", strings.NewReader("payload"))
	invokeReq.Header.Set("Content-Type", "text/plain")
	invokeW := httptest.NewRecorder()
	srv.Router().ServeHTTP(invokeW, invokeReq)

	testutil.StatusCode(t, http.StatusCreated, invokeW.Code)
	testutil.Equal(t, "POST|/"+fnName+"/nested/path|q=1|payload", invokeW.Body.String())

	logsReq := httptest.NewRequest(http.MethodGet, "/api/admin/functions/"+deployed.ID.String()+"/logs?page=1&perPage=10", nil)
	logsReq.Header.Set("Authorization", "Bearer "+adminToken)
	logsW := httptest.NewRecorder()
	srv.Router().ServeHTTP(logsW, logsReq)

	testutil.StatusCode(t, http.StatusOK, logsW.Code)
	var logs []edgefunc.LogEntry
	testutil.NoError(t, json.Unmarshal(logsW.Body.Bytes(), &logs))
	testutil.SliceLen(t, logs, 1)
	testutil.Equal(t, "success", logs[0].Status)
	testutil.True(t, logs[0].DurationMs >= 0, "duration must be non-negative")
	testutil.Equal(t, "POST", logs[0].RequestMethod)
	testutil.Equal(t, "/"+fnName+"/nested/path", logs[0].RequestPath)
}

// TestRealtimeSSEConfigRegression verifies that SSE realtime behavior is not
// regressed by config/metrics changes. This test ensures:
// 1. SSE connections work with default realtime config
// 2. SSE events (create/update/delete) are delivered correctly
// 3. Metrics correctly count SSE connections
func TestRealtimeSSEConfigRegression(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Metrics.Enabled = true
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	sseURL := ts.URL + "/api/realtime?tables=users"
	resp, err := http.Get(sseURL)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)

	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")
	testutil.Equal(t, "event: connected", connected[0])

	body, _ := json.Marshal(map[string]any{"name": "SSE_Test", "email": "sse@test.com"})
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(body))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	createResp.Body.Close()

	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE create event")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"create"`)
	testutil.Contains(t, joined, `"table":"users"`)

	client := &http.Client{}

	updateReq, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/collections/users/1", bytes.NewReader(body))
	testutil.NoError(t, err)
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := client.Do(updateReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, updateResp.StatusCode)
	updateResp.Body.Close()

	lines = readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE update event")
	joined = strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"update"`)
	testutil.Contains(t, joined, `"table":"users"`)

	delReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/collections/users/1", nil)
	testutil.NoError(t, err)
	delResp, err := client.Do(delReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()

	lines = readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE delete event")
	joined = strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"delete"`)
	testutil.Contains(t, joined, `"table":"users"`)

	adminToken := adminLogin(t, srv)
	statsReq := httptest.NewRequest(http.MethodGet, "/api/admin/realtime/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+adminToken)
	statsW := httptest.NewRecorder()
	srv.Router().ServeHTTP(statsW, statsReq)

	testutil.StatusCode(t, http.StatusOK, statsW.Code)
	var stats map[string]any
	testutil.NoError(t, json.Unmarshal(statsW.Body.Bytes(), &stats))

	conns := stats["connections"].(map[string]any)
	testutil.True(t, conns["sse"].(float64) >= 1, "expected at least 1 SSE connection in stats")
}

func TestRealtimeSSERPCNotifyPublishesCRUDShape(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION public.rpc_insert_user_notify(p_name text, p_email text)
		RETURNS TABLE (id integer, name text, email varchar(255))
		LANGUAGE sql
		AS $$
			INSERT INTO public.users (name, email)
			VALUES (p_name, p_email)
			RETURNING users.id, users.name, users.email;
		$$;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	connected := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")
	testutil.True(t, len(connected) >= 1, "expected SSE connected event frame")
	testutil.Equal(t, "event: connected", connected[0])

	rpcArgs, err := json.Marshal(map[string]any{
		"p_name":  "RPC Notify User",
		"p_email": "rpc-notify@example.com",
	})
	testutil.NoError(t, err)

	rpcReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/rpc/rpc_insert_user_notify", bytes.NewReader(rpcArgs))
	testutil.NoError(t, err)
	rpcReq.Header.Set("Content-Type", "application/json")
	rpcReq.Header.Set("X-Notify-Table", "users")
	rpcReq.Header.Set("X-Notify-Action", "create")

	rpcResp, err := http.DefaultClient.Do(rpcReq)
	testutil.NoError(t, err)
	defer rpcResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, rpcResp.StatusCode)

	rpcBody, err := io.ReadAll(rpcResp.Body)
	testutil.NoError(t, err)

	committedRow := decodeCommittedRPCRow(t, rpcBody)
	testutil.NotNil(t, committedRow)

	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE RPC notify event")
	var dataFrame string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataFrame = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	testutil.True(t, dataFrame != "", "expected SSE data frame in event")

	var event map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(dataFrame), &event))

	action, ok := event["action"].(string)
	testutil.True(t, ok, "expected action to be a string")
	table, ok := event["table"].(string)
	testutil.True(t, ok, "expected table to be a string")
	testutil.Equal(t, "create", action)
	testutil.Equal(t, "users", table)

	record, ok := event["record"].(map[string]any)
	testutil.True(t, ok, "expected record object in SSE payload")
	if !reflect.DeepEqual(committedRow, record) {
		t.Fatalf("SSE record mismatch: got %#v, want %#v", record, committedRow)
	}

	_, hasOldRecord := event["old_record"]
	testutil.False(t, hasOldRecord, "old_record should not be present in SSE payload")
}

func TestRealtimeSSERPCNotifyFilteredDelivery(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION public.rpc_insert_user_notify_filtered(p_name text, p_email text)
		RETURNS TABLE (id integer, name text, email varchar(255))
		LANGUAGE sql
		AS $$
			INSERT INTO public.users (name, email)
			VALUES (p_name, p_email)
			RETURNING users.id, users.name, users.email;
		$$;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	filteredResp, err := http.Get(ts.URL + "/api/realtime?tables=users&filter=name=eq.Bob")
	testutil.NoError(t, err)
	defer filteredResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, filteredResp.StatusCode)

	allResp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer allResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, allResp.StatusCode)

	filteredScanner := bufio.NewScanner(filteredResp.Body)
	allScanner := bufio.NewScanner(allResp.Body)
	readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE connected event")
	readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE connected event")

	decodeSSEPayload := func(lines []string) map[string]any {
		var dataFrame string
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				dataFrame = strings.TrimPrefix(line, "data: ")
				break
			}
		}
		testutil.True(t, dataFrame != "", "expected SSE data frame in event")

		var event map[string]any
		testutil.NoError(t, json.Unmarshal([]byte(dataFrame), &event))
		return event
	}

	callRPC := func(name, email string) {
		t.Helper()

		rpcArgs, marshalErr := json.Marshal(map[string]any{
			"p_name":  name,
			"p_email": email,
		})
		testutil.NoError(t, marshalErr)

		rpcReq, reqErr := http.NewRequest(http.MethodPost, ts.URL+"/api/rpc/rpc_insert_user_notify_filtered", bytes.NewReader(rpcArgs))
		testutil.NoError(t, reqErr)
		rpcReq.Header.Set("Content-Type", "application/json")
		rpcReq.Header.Set("X-Notify-Table", "users")
		rpcReq.Header.Set("X-Notify-Action", "create")

		rpcResp, doErr := http.DefaultClient.Do(rpcReq)
		testutil.NoError(t, doErr)
		defer rpcResp.Body.Close()
		testutil.StatusCode(t, http.StatusOK, rpcResp.StatusCode)
	}

	callRPC("Alice", "alice-rpc-filter@example.com")

	allAliceLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE RPC Alice event")
	allAliceEvent := decodeSSEPayload(allAliceLines)
	testutil.Equal(t, "create", allAliceEvent["action"])
	testutil.Equal(t, "users", allAliceEvent["table"])
	allAliceRecord, ok := allAliceEvent["record"].(map[string]any)
	testutil.True(t, ok, "expected unfiltered SSE record payload")
	testutil.Equal(t, "Alice", allAliceRecord["name"])

	callRPC("Bob", "bob-rpc-filter@example.com")

	// If the filtered client incorrectly received Alice, that stale frame is read first here and fails this assertion.
	allBobLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE RPC Bob event")
	filteredBobLines := readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE RPC Bob event")

	allBobEvent := decodeSSEPayload(allBobLines)
	testutil.Equal(t, "create", allBobEvent["action"])
	testutil.Equal(t, "users", allBobEvent["table"])
	allBobRecord, ok := allBobEvent["record"].(map[string]any)
	testutil.True(t, ok, "expected unfiltered SSE Bob record payload")
	testutil.Equal(t, "Bob", allBobRecord["name"])

	filteredBobEvent := decodeSSEPayload(filteredBobLines)
	testutil.Equal(t, "create", filteredBobEvent["action"])
	testutil.Equal(t, "users", filteredBobEvent["table"])
	filteredBobRecord, ok := filteredBobEvent["record"].(map[string]any)
	testutil.True(t, ok, "expected filtered SSE Bob record payload")
	testutil.Equal(t, "Bob", filteredBobRecord["name"])
}

func TestRealtimeSSEVoidRPCNotifyProducesNoEvent(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION public.rpc_void_notify_no_event()
		RETURNS void
		LANGUAGE sql
		AS $$
			SELECT;
		$$;
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE connected event")

	voidRPCReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/rpc/rpc_void_notify_no_event", nil)
	testutil.NoError(t, err)
	voidRPCReq.Header.Set("X-Notify-Table", "users")
	voidRPCReq.Header.Set("X-Notify-Action", "delete")

	voidRPCResp, err := http.DefaultClient.Do(voidRPCReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, voidRPCResp.StatusCode)
	testutil.NoError(t, voidRPCResp.Body.Close())

	createBody, err := json.Marshal(map[string]any{
		"name":  "PostVoidCreate",
		"email": "post-void-create@example.com",
	})
	testutil.NoError(t, err)
	createResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(createBody))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, createResp.StatusCode)
	testutil.NoError(t, createResp.Body.Close())

	lines := readNextSSEEvent(t, scanner, 5*time.Second, "timed out waiting for SSE create event after void RPC")
	joined := strings.Join(lines, "\n")
	testutil.Contains(t, joined, `"action":"create"`)
	testutil.Contains(t, joined, `"table":"users"`)
	testutil.Contains(t, joined, `"PostVoidCreate"`)
	testutil.False(t, strings.Contains(joined, `"action":"delete"`), "void RPC should not produce notify event")
}

func TestRealtimeSSEFilteredDeliveryParity(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	filteredResp, err := http.Get(ts.URL + "/api/realtime?tables=users&filter=name=eq.Bob")
	testutil.NoError(t, err)
	defer filteredResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, filteredResp.StatusCode)

	allResp, err := http.Get(ts.URL + "/api/realtime?tables=users")
	testutil.NoError(t, err)
	defer allResp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, allResp.StatusCode)

	filteredScanner := bufio.NewScanner(filteredResp.Body)
	allScanner := bufio.NewScanner(allResp.Body)
	readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE connected event")
	readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE connected event")

	aliceBody, err := json.Marshal(map[string]any{"name": "Alice", "email": "alice-filter-parity@example.com"})
	testutil.NoError(t, err)
	aliceResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(aliceBody))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, aliceResp.StatusCode)
	testutil.NoError(t, aliceResp.Body.Close())

	allAliceLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE Alice event")
	allAliceJoined := strings.Join(allAliceLines, "\n")
	testutil.Contains(t, allAliceJoined, `"action":"create"`)
	testutil.Contains(t, allAliceJoined, `"table":"users"`)
	testutil.Contains(t, allAliceJoined, `"Alice"`)

	bobBody, err := json.Marshal(map[string]any{"name": "Bob", "email": "bob-filter-parity@example.com"})
	testutil.NoError(t, err)
	bobResp, err := http.Post(ts.URL+"/api/collections/users/", "application/json", bytes.NewReader(bobBody))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, bobResp.StatusCode)
	testutil.NoError(t, bobResp.Body.Close())

	// Mirrors the WS pattern: if filtered SSE incorrectly got Alice, that stale event is read first and fails this assertion.
	allBobLines := readNextSSEEvent(t, allScanner, 5*time.Second, "timed out waiting for unfiltered SSE Bob event")
	filteredBobLines := readNextSSEEvent(t, filteredScanner, 5*time.Second, "timed out waiting for filtered SSE Bob event")

	allBobJoined := strings.Join(allBobLines, "\n")
	testutil.Contains(t, allBobJoined, `"action":"create"`)
	testutil.Contains(t, allBobJoined, `"table":"users"`)
	testutil.Contains(t, allBobJoined, `"Bob"`)

	filteredBobJoined := strings.Join(filteredBobLines, "\n")
	testutil.Contains(t, filteredBobJoined, `"action":"create"`)
	testutil.Contains(t, filteredBobJoined, `"table":"users"`)
	testutil.Contains(t, filteredBobJoined, `"Bob"`)
}
