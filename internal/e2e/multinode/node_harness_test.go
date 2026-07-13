//go:build multinode

package multinode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/e2e/crossnode"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	nodeAApplicationName = "ayb_multinode_node_a"
	nodeBApplicationName = "ayb_multinode_node_b"
	tableEventsChannel   = "ayb_pgnotify_realtime_table_events"
	oauthEventsChannel   = "ayb_pgnotify_realtime_oauth_events"
	realtimeProofTable   = "multinode_realtime_events"
)

func TestMultiNodeHarnessBoots(t *testing.T) {
	requireMultinodeDatabaseURL(t)
	_ = bootTwoNodeHarness(t)
}

func TestMultiNodeRealtimeFanoutAcrossNodes(t *testing.T) {
	testDatabaseURL := requireMultinodeDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	crossnode.SeedRealtimeProofTable(ctx, t, testDatabaseURL, realtimeProofTable)
	harness := bootTwoNodeHarness(t)
	auth := registerHarnessUser(t, harness.nodeA.baseURL(), "multinode-realtime@example.com")
	crossnode.ConfigureUsersAsSharedTenants(ctx, t, testDatabaseURL, auth.userID)
	conn := dialHarnessRealtimeWS(t, harness.nodeB, auth.accessToken)
	defer conn.Close()

	writeHarnessWSJSON(t, conn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{realtimeProofTable},
		Ref:    "sub-realtime-proof",
	})
	reply, err := readHarnessWSUntil(t, conn, ws.MsgTypeReply, 5*time.Second)
	if err != nil {
		t.Fatalf("realtime subscribe reply failed: %v\nnodes:\n%s", err, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	if reply.Status != "ok" || reply.Ref != "sub-realtime-proof" {
		t.Fatalf("unexpected subscribe reply: %#v\nnodes:\n%s", reply, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	sentinel := "node-a-write-node-b-observe-" + randomHex(t, 6)
	status, body := harnessHTTPJSON(t, http.MethodPost, harness.nodeA.baseURL()+"/api/collections/"+realtimeProofTable,
		map[string]any{"sentinel": sentinel}, auth.accessToken)
	if status != http.StatusCreated {
		t.Fatalf("create realtime sentinel status=%d body=%s\nnodes:\n%s", status, body, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	event, err := readHarnessWSUntil(t, conn, ws.MsgTypeEvent, 10*time.Second)
	if err != nil {
		t.Fatalf("node B did not receive realtime event: %v\nnodes:\n%s", err, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	if event.Action != "create" || event.Table != realtimeProofTable || event.Record["sentinel"] != sentinel {
		t.Fatalf("unexpected realtime event: type=%s action=%s table=%s record=%v sentinel=%s\nnodes:\n%s",
			event.Type, event.Action, event.Table, event.Record, sentinel, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
}

func TestMultiNodeSessionRevocationAcrossNodes(t *testing.T) {
	requireMultinodeDatabaseURL(t)
	harness := bootTwoNodeHarness(t)
	auth := registerHarnessUser(t, harness.nodeA.baseURL(), "multinode-revoke@example.com")

	initialStatus, initialBody := harnessRaw(t, http.MethodGet, harness.nodeB.baseURL()+"/api/auth/me", nil, auth.accessToken)
	if initialStatus != http.StatusOK || !strings.Contains(initialBody, auth.userID) || !strings.Contains(initialBody, auth.email) {
		t.Fatalf("baseline node B /api/auth/me status=%d body=%s user=%s email=%s\nnodes:\n%s",
			initialStatus, initialBody, auth.userID, auth.email, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	sessions := crossnode.ListSessions(t, harness.nodeA.baseURL(), auth.accessToken)
	if len(sessions) != 1 || sessions[0].ID == "" {
		t.Fatalf("expected one current session before revoke, got %#v\nnodes:\n%s", sessions, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	revokedSessionID := sessions[0].ID
	deleteStatus, deleteBody := harnessRaw(t, http.MethodDelete, harness.nodeA.baseURL()+"/api/auth/sessions/"+revokedSessionID, nil, auth.accessToken)
	if deleteStatus != http.StatusNoContent {
		t.Fatalf("revoke session %s status=%d body=%s\nnodes:\n%s",
			revokedSessionID, deleteStatus, deleteBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	finalResp := crossnode.WaitForAuthMeStatus(t, harness.nodeB.baseURL(), auth.accessToken, http.StatusUnauthorized, 10*time.Second)
	finalStatus, finalBody := finalResp.Status, finalResp.Body
	if finalStatus != http.StatusUnauthorized {
		t.Fatalf("node B access token was not revoked: initial=%d final=%d session=%s body=%s\nnodes:\n%s",
			initialStatus, finalStatus, revokedSessionID, finalBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	refreshStatus, refreshBody := harnessHTTPJSON(t, http.MethodPost, harness.nodeB.baseURL()+"/api/auth/refresh",
		map[string]string{"refreshToken": auth.refreshToken}, "")
	if refreshStatus != http.StatusUnauthorized {
		t.Fatalf("node B refresh token was not invalidated: status=%d session=%s body=%s\nnodes:\n%s",
			refreshStatus, revokedSessionID, refreshBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
}

func TestMultiNodeStorageReadTenantIsolationAcrossNodes(t *testing.T) {
	requireMultinodeDatabaseURL(t)
	harness := bootTwoNodeHarness(t)
	userA := registerHarnessUser(t, harness.nodeA.baseURL(), "multinode-storage-a@example.com")
	userB := registerHarnessUser(t, harness.nodeA.baseURL(), "multinode-storage-b@example.com")
	bucket := "multinode-storage-" + randomHex(t, 4)
	filename := "tenant-proof-" + randomHex(t, 4) + ".txt"
	body := "node-a-storage-write-node-b-read-" + randomHex(t, 8)

	uploadStatus, uploadBody := uploadHarnessFile(t, harness.nodeA.baseURL(), bucket, filename, body, userA.accessToken)
	if uploadStatus != http.StatusCreated {
		t.Fatalf("node A upload status=%d body=%s\nnodes:\n%s", uploadStatus, uploadBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	statusA, bodyA := waitForHarnessStorageBody(t, harness.nodeB.baseURL(), bucket, filename, userA.accessToken, http.StatusOK, body, 10*time.Second)
	if statusA != http.StatusOK || bodyA != body {
		t.Fatalf("node B same-tenant read mismatch: status=%d body=%q want=%q\nnodes:\n%s",
			statusA, bodyA, body, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	statusB, bodyB := harnessRaw(t, http.MethodGet, harness.nodeB.baseURL()+"/api/storage/"+bucket+"/"+url.PathEscape(filename), nil, userB.accessToken)
	if statusB != http.StatusNotFound {
		t.Fatalf("node B cross-tenant read status=%d body=%s want=%d\nnodes:\n%s",
			statusB, bodyB, http.StatusNotFound, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
}

func requireMultinodeDatabaseURL(t *testing.T) string {
	t.Helper()
	testDatabaseURL := os.Getenv("TEST_DATABASE_URL")
	if testDatabaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for multinode harness; run through internal/testutil/cmd/testpg")
	}
	return testDatabaseURL
}

type twoNodeHarness struct {
	nodeA *harnessNode
	nodeB *harnessNode
}

func bootTwoNodeHarness(t *testing.T, envOverrides ...map[string]string) *twoNodeHarness {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	testDatabaseURL := os.Getenv("TEST_DATABASE_URL")
	minio := StartMinIOHarness(ctx, t)
	aybBin := buildAYBBinary(t)
	jwtSecret := randomHex(t, 32)
	portA := freePort(t)
	portB := freePort(t)
	modRoot := moduleRoot(t)

	nodeA := newHarnessNode(t, "node-a", aybBin, portA, deriveNodeDatabaseURL(t, testDatabaseURL, nodeAApplicationName), jwtSecret, minio)
	nodeB := newHarnessNode(t, "node-b", aybBin, portB, deriveNodeDatabaseURL(t, testDatabaseURL, nodeBApplicationName), jwtSecret, minio)
	assertNodeRoots(t, modRoot, nodeA, nodeB)
	applyTwoNodeEnvOverrides(envOverrideFromArgs(envOverrides), nodeA, nodeB)

	startTwoNodeHarness(ctx, t, testDatabaseURL, nodeA, nodeB)
	return &twoNodeHarness{nodeA: nodeA, nodeB: nodeB}
}

func startTwoNodeHarness(ctx context.Context, t *testing.T, testDatabaseURL string, nodeA, nodeB *harnessNode) {
	t.Helper()
	startHarnessNode(t, nodeA)
	waitForNodeHealth(t, nodeA, 90*time.Second)

	startHarnessNode(t, nodeB)
	waitForNodeHealth(t, nodeA, 30*time.Second)
	waitForNodeHealth(t, nodeB, 90*time.Second)

	assertPgNotifyListeners(ctx, t, testDatabaseURL, nodeA, nodeB)
}

func restartTwoNodeHarness(t *testing.T, harness *twoNodeHarness, envOverrides map[string]string) {
	t.Helper()

	stopHarnessNode(harness.nodeB)
	stopHarnessNode(harness.nodeA)
	applyTwoNodeEnvOverrides(envOverrides, harness.nodeA, harness.nodeB)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	startTwoNodeHarness(ctx, t, os.Getenv("TEST_DATABASE_URL"), harness.nodeA, harness.nodeB)
}

func envOverrideFromArgs(envOverrides []map[string]string) map[string]string {
	if len(envOverrides) == 0 {
		return nil
	}
	return envOverrides[0]
}

func applyTwoNodeEnvOverrides(envOverrides map[string]string, nodes ...*harnessNode) {
	for _, node := range nodes {
		node.envOverrides = copyStringMap(envOverrides)
	}
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

type harnessNode struct {
	name         string
	binPath      string
	port         int
	databaseURL  string
	jwtSecret    string
	minio        *MinIOHarness
	homeDir      string
	workDir      string
	envOverrides map[string]string
	output       bytes.Buffer
	cmd          *exec.Cmd
	exit         <-chan error
	done         <-chan struct{}
}

func (node *harnessNode) baseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", node.port)
}

func newHarnessNode(
	t *testing.T,
	name string,
	aybBin string,
	port int,
	databaseURL string,
	jwtSecret string,
	minio *MinIOHarness,
) *harnessNode {
	t.Helper()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("%s: create home dir: %v", name, err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("%s: create work dir: %v", name, err)
	}

	return &harnessNode{
		name:        name,
		binPath:     aybBin,
		port:        port,
		databaseURL: databaseURL,
		jwtSecret:   jwtSecret,
		minio:       minio,
		homeDir:     homeDir,
		workDir:     workDir,
	}
}

func startHarnessNode(t *testing.T, node *harnessNode) {
	t.Helper()

	writeHarnessNodeConfig(t, node)
	cmd := exec.Command(node.binPath, "start", "--foreground") //nolint:gosec
	cmd.Dir = node.workDir
	cmd.Stdout = &node.output
	cmd.Stderr = &node.output
	cmd.Env = nodeEnv(node)
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start ayb foreground process: %v", node.name, err)
	}

	node.cmd = cmd
	node.exit, node.done = observeHarnessProcessExit(cmd)
	t.Cleanup(func() {
		stopHarnessNode(node)
	})
}

func writeHarnessNodeConfig(t *testing.T, node *harnessNode) {
	t.Helper()
	if node.envOverrides["AYB_GRAPHQL_ENABLED"] != "true" {
		return
	}
	configPath := filepath.Join(node.workDir, "ayb.toml")
	if err := os.WriteFile(configPath, []byte("[graphql]\nenabled = true\n"), 0o600); err != nil {
		t.Fatalf("%s: write harness ayb.toml: %v", node.name, err)
	}
}

func nodeEnv(node *harnessNode) []string {
	env := filteredProcessEnv()
	env = append(env,
		"HOME="+node.homeDir,
		"AYB_DATABASE_URL="+node.databaseURL,
		"AYB_SERVER_HOST=127.0.0.1",
		fmt.Sprintf("AYB_SERVER_PORT=%d", node.port),
		"AYB_AUTH_ENABLED=true",
		"AYB_AUTH_JWT_SECRET="+node.jwtSecret,
		"AYB_STORAGE_ENABLED=true",
		"AYB_STORAGE_BACKEND=s3",
		"AYB_STORAGE_DEFAULT_QUOTA_MB=1",
		"AYB_STORAGE_S3_ENDPOINT="+node.minio.Endpoint,
		"AYB_STORAGE_S3_BUCKET="+node.minio.Bucket,
		"AYB_STORAGE_S3_REGION=us-east-1",
		"AYB_STORAGE_S3_ACCESS_KEY="+node.minio.AccessKey,
		"AYB_STORAGE_S3_SECRET_KEY="+node.minio.SecretKey,
		"AYB_STORAGE_S3_USE_SSL=false",
	)
	for _, key := range sortedMapKeys(node.envOverrides) {
		env = append(env, key+"="+node.envOverrides[key])
	}
	return env
}

func filteredProcessEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if ok && (strings.HasPrefix(key, "AYB_") || key == "TEST_DATABASE_URL") {
			continue
		}
		env = append(env, value)
	}
	return env
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func observeHarnessProcessExit(cmd *exec.Cmd) (<-chan error, <-chan struct{}) {
	serverExit := make(chan error, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		serverExit <- cmd.Wait()
	}()
	return serverExit, serverDone
}

func stopHarnessNode(node *harnessNode) {
	if node == nil || node.cmd == nil || node.cmd.Process == nil {
		return
	}

	select {
	case <-node.done:
		return
	default:
	}

	_ = node.cmd.Process.Signal(os.Interrupt)
	select {
	case <-node.done:
		return
	case <-time.After(10 * time.Second):
	}

	_ = node.cmd.Process.Kill()
	select {
	case <-node.done:
	case <-time.After(2 * time.Second):
	}
}

func waitForNodeHealth(t *testing.T, node *harnessNode, timeout time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", node.port)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-node.exit:
			t.Fatalf("%s: exited before /health was ready: %v\noutput:\n%s", node.name, err, nodeOutput(node))
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s: /health did not return HTTP 200 at %s before deadline: %v\noutput:\n%s",
		node.name, healthURL, lastErr, nodeOutput(node))
}

func buildAYBBinary(t *testing.T) string {
	t.Helper()

	binPath := filepath.Join(t.TempDir(), "ayb")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ayb") //nolint:gosec
	cmd.Dir = moduleRoot(t)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build ayb binary: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return binPath
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func deriveNodeDatabaseURL(t *testing.T, rawURL, applicationName string) string {
	t.Helper()

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("application_name", applicationName)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func randomHex(t *testing.T, byteCount int) string {
	t.Helper()
	return crossnode.RandomHex(t, byteCount)
}

func freePort(t *testing.T) int {
	t.Helper()

	port, err := testutil.FreePort()
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	return port
}

func assertNodeRoots(t *testing.T, modRoot string, nodeA, nodeB *harnessNode) {
	t.Helper()

	if nodeA.homeDir == nodeB.homeDir || nodeA.workDir == nodeB.workDir {
		t.Fatalf("node runtime roots must be distinct: nodeA=%s/%s nodeB=%s/%s",
			nodeA.homeDir, nodeA.workDir, nodeB.homeDir, nodeB.workDir)
	}
	for _, root := range []string{nodeA.homeDir, nodeA.workDir, nodeB.homeDir, nodeB.workDir} {
		if isWithinDir(root, modRoot) {
			t.Fatalf("node runtime root %s must not be inside repository worktree %s", root, modRoot)
		}
	}
}

func isWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

type pgNotifyListenerRow struct {
	pid             int32
	applicationName string
	query           string
}

func assertPgNotifyListeners(ctx context.Context, t *testing.T, databaseURL string, nodes ...*harnessNode) {
	t.Helper()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for pgnotify proof: %v", err)
	}
	defer pool.Close()

	deadline := time.Now().Add(30 * time.Second)
	var rows []pgNotifyListenerRow
	for time.Now().Before(deadline) {
		rows = fetchPgNotifyListenerRows(ctx, t, pool)
		if hasExpectedPgNotifyListeners(rows) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("pgnotify listeners missing; observed rows=%v\nnode output:\n%s",
		rows, combinedNodeOutput(nodes...))
}

func fetchPgNotifyListenerRows(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []pgNotifyListenerRow {
	t.Helper()

	rows, err := pool.Query(ctx, `
		select pid, application_name, query
		from pg_stat_activity
		where datname = current_database()
			and application_name in ($1, $2)
			and query in ($3, $4)
		order by application_name, query, pid`,
		nodeAApplicationName, nodeBApplicationName, tableEventsChannelListen(), oauthEventsChannelListen())
	if err != nil {
		t.Fatalf("query pg_stat_activity for pgnotify listeners: %v", err)
	}
	defer rows.Close()

	var listeners []pgNotifyListenerRow
	for rows.Next() {
		var row pgNotifyListenerRow
		if err := rows.Scan(&row.pid, &row.applicationName, &row.query); err != nil {
			t.Fatalf("scan pgnotify listener row: %v", err)
		}
		listeners = append(listeners, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pgnotify listener rows: %v", err)
	}
	return listeners
}

func hasExpectedPgNotifyListeners(rows []pgNotifyListenerRow) bool {
	seen := map[string]map[string]int32{}
	distinctPIDs := map[int32]struct{}{}
	for _, row := range rows {
		if seen[row.applicationName] == nil {
			seen[row.applicationName] = map[string]int32{}
		}
		seen[row.applicationName][row.query] = row.pid
		distinctPIDs[row.pid] = struct{}{}
	}

	for _, appName := range []string{nodeAApplicationName, nodeBApplicationName} {
		if seen[appName][tableEventsChannelListen()] == 0 || seen[appName][oauthEventsChannelListen()] == 0 {
			return false
		}
	}
	return len(distinctPIDs) == 4
}

func tableEventsChannelListen() string { return "LISTEN " + tableEventsChannel }

func oauthEventsChannelListen() string { return "LISTEN " + oauthEventsChannel }

// harnessAuthTokens adapts crossnode.AuthTokens to the lower-cased field names
// the multinode _test.go files reference. The registration logic itself lives
// in the importable crossnode package.
type harnessAuthTokens struct {
	accessToken  string
	refreshToken string
	userID       string
	email        string
}

func registerHarnessUser(t *testing.T, baseURL, email string) harnessAuthTokens {
	t.Helper()
	tokens := crossnode.RegisterUser(t, baseURL, email)
	return harnessAuthTokens{
		accessToken:  tokens.AccessToken,
		refreshToken: tokens.RefreshToken,
		userID:       tokens.UserID,
		email:        tokens.Email,
	}
}

// harnessJSONBody adapts crossnode.JSONBody to the lower-cased field names the
// multinode _test.go files reference (body.object / body.raw).
type harnessJSONBody struct {
	object map[string]any
	raw    string
}

func (b harnessJSONBody) String() string {
	return b.raw
}

func adaptJSONBody(body crossnode.JSONBody) harnessJSONBody {
	return harnessJSONBody{object: body.Object, raw: body.Raw}
}

func harnessHTTPJSON(t *testing.T, method, url string, body any, token string) (int, harnessJSONBody) {
	t.Helper()
	status, decoded := crossnode.HTTPJSON(t, method, url, body, token)
	return status, adaptJSONBody(decoded)
}

func harnessRaw(t *testing.T, method, url string, body any, token string) (int, string) {
	t.Helper()
	return crossnode.Raw(t, method, url, body, token)
}

func harnessAnonymousStorageRead(t *testing.T, node *harnessNode, bucket, filename, tenantID string) (int, string) {
	t.Helper()
	headers := map[string]string{}
	if tenantID != "" {
		headers["X-Tenant-ID"] = tenantID
	}
	resp := crossnode.Do(t, crossnode.RawRequest{
		Method:  http.MethodGet,
		URL:     crossnode.StorageObjectURL(node.baseURL(), bucket, filename),
		Headers: headers,
	})
	return resp.Status, resp.Body
}

func uploadHarnessFile(t *testing.T, baseURL, bucket, filename, fileBody, token string) (int, harnessJSONBody) {
	t.Helper()
	resp, decoded := crossnode.Upload(t, baseURL, bucket, filename, fileBody, token)
	return resp.Status, adaptJSONBody(decoded)
}

func waitForHarnessStorageBody(
	t *testing.T,
	baseURL string,
	bucket string,
	filename string,
	token string,
	wantStatus int,
	wantBody string,
	timeout time.Duration,
) (int, string) {
	t.Helper()
	resp := crossnode.WaitForStorageBody(t, baseURL, bucket, filename, token, wantStatus, wantBody, timeout)
	return resp.Status, resp.Body
}

func waitForHarnessSchemaTableColumn(
	t *testing.T,
	node *harnessNode,
	token string,
	schemaName string,
	tableName string,
	columnName string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var status int
	var body harnessJSONBody
	for time.Now().Before(deadline) {
		status, body = harnessHTTPJSON(t, http.MethodGet, node.baseURL()+"/api/schema", nil, token)
		if status == http.StatusOK && schemaBodyHasColumn(body.object, schemaName, tableName, columnName) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s schema cache did not expose %s.%s.%s before deadline: status=%d body=%s\noutput:\n%s",
		node.name, schemaName, tableName, columnName, status, body, nodeOutput(node))
}

func schemaBodyHasColumn(body map[string]any, schemaName, tableName, columnName string) bool {
	tables, ok := body["tables"].(map[string]any)
	if !ok {
		return false
	}
	rawTable, ok := tables[schemaName+"."+tableName].(map[string]any)
	if !ok {
		return false
	}
	columns, ok := rawTable["columns"].([]any)
	if !ok {
		return false
	}
	for _, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if ok && column["name"] == columnName {
			return true
		}
	}
	return false
}

func dialHarnessRealtimeWS(t *testing.T, node *harnessNode, token string) *websocket.Conn {
	t.Helper()
	conn, _ := crossnode.DialRealtimeWS(t, node.baseURL(), token)
	return conn
}

type harnessGQLWSMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func dialHarnessGraphQLWS(t *testing.T, node *harnessNode, token string) *websocket.Conn {
	t.Helper()

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/api/graphql", node.port)
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}
	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("%s: dial GraphQL websocket: %v\noutput:\n%s", node.name, err, nodeOutput(node))
	}
	if conn.Subprotocol() != "graphql-transport-ws" {
		conn.Close()
		t.Fatalf("%s: GraphQL websocket subprotocol=%q", node.name, conn.Subprotocol())
	}

	payload, err := json.Marshal(map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		conn.Close()
		t.Fatalf("marshal GraphQL connection_init payload: %v", err)
	}
	writeHarnessWSJSON(t, conn, harnessGQLWSMessage{Type: "connection_init", Payload: payload})
	msg, err := readHarnessGraphQLWSUntil(t, conn, "connection_ack", 5*time.Second)
	if err != nil {
		conn.Close()
		t.Fatalf("%s: GraphQL connection_ack failed: %v\noutput:\n%s", node.name, err, nodeOutput(node))
	}
	if msg.Type != "connection_ack" {
		conn.Close()
		t.Fatalf("%s: unexpected GraphQL ack message: %#v", node.name, msg)
	}
	return conn
}

func subscribeHarnessGraphQLWS(t *testing.T, conn *websocket.Conn, ref string, query string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal GraphQL subscribe payload: %v", err)
	}
	writeHarnessWSJSON(t, conn, harnessGQLWSMessage{ID: ref, Type: "subscribe", Payload: payload})
}

func readHarnessGraphQLWSUntil(
	t *testing.T,
	conn *websocket.Conn,
	msgType string,
	timeout time.Duration,
) (harnessGQLWSMessage, error) {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var lastMismatch string
	for {
		var msg harnessGQLWSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if lastMismatch != "" {
				return harnessGQLWSMessage{}, fmt.Errorf("%w (last message: %s)", err, lastMismatch)
			}
			return harnessGQLWSMessage{}, err
		}
		if msg.Type == msgType {
			return msg, nil
		}
		lastMismatch = fmt.Sprintf("type=%s id=%s payload=%s", msg.Type, msg.ID, string(msg.Payload))
	}
}

func writeHarnessWSJSON(t *testing.T, conn *websocket.Conn, msg any) {
	t.Helper()
	crossnode.WriteWSJSON(t, conn, msg)
}

func readHarnessWSUntil(t *testing.T, conn *websocket.Conn, msgType string, timeout time.Duration) (ws.ServerMessage, error) {
	t.Helper()
	return crossnode.ReadWSUntil(t, conn, msgType, timeout)
}

func nodeOutput(node *harnessNode) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.output.String())
}

func combinedNodeOutput(nodes ...*harnessNode) string {
	var b strings.Builder
	for _, node := range nodes {
		fmt.Fprintf(&b, "== %s ==\n%s\n", node.name, nodeOutput(node))
	}
	return strings.TrimSpace(b.String())
}
