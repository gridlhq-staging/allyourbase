//go:build multinode

package multinode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pooledStorageQuotaBytes = 1024 * 1024

const (
	anonymousPublicBucket      = "pub"
	anonymousPublicObject      = "same.txt"
	anonymousPublicTenantABody = "T1-bytes"
	anonymousPublicTenantBBody = "T2-bytes"
	anonymousPublicAmbientBody = "ambient-bytes"
)

func TestCrossNodePooledStorageQuotaPerTenant(t *testing.T) {
	databaseURL := requirePooledTenantDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	harness := bootTwoNodeHarness(t)
	userT1 := registerHarnessUser(t, harness.nodeA.baseURL(), "quota-t1-"+randomHex(t, 4)+"@example.com")
	userT2 := registerHarnessUser(t, harness.nodeA.baseURL(), "quota-t2-"+randomHex(t, 4)+"@example.com")
	configureHarnessUsersAsSharedTenants(ctx, t, databaseURL, userT1.userID, userT2.userID)

	pool := openPooledStoragePool(ctx, t, databaseURL)
	defer pool.Close()
	t1ID := resolveHarnessUserTenantID(ctx, t, pool, userT1.userID)
	t2ID := resolveHarnessUserTenantID(ctx, t, pool, userT2.userID)

	bucket := "pooled-quota-" + randomHex(t, 4)
	fillBody := strings.Repeat("f", 700*1024)
	secondBody := strings.Repeat("s", 700*1024)
	if len(fillBody) > pooledStorageQuotaBytes || len(secondBody) > pooledStorageQuotaBytes {
		t.Fatalf("test bodies must each fit within the 1 MiB quota: fill=%d second=%d", len(fillBody), len(secondBody))
	}
	if len(fillBody)+len(secondBody) <= pooledStorageQuotaBytes {
		t.Fatalf("test bodies must exceed quota together: fill=%d second=%d", len(fillBody), len(secondBody))
	}

	probeName := "probe-" + randomHex(t, 4) + ".bin"
	probeStatus, probeBody := uploadHarnessFile(t, harness.nodeB.baseURL(), bucket, probeName, secondBody, userT1.accessToken)
	if probeStatus != http.StatusCreated {
		t.Fatalf("node B single-upload quota probe status=%d body=%s\nnodes:\n%s",
			probeStatus, probeBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	deleteStatus, deleteBody := harnessRaw(t, http.MethodDelete,
		harness.nodeB.baseURL()+"/api/storage/"+bucket+"/"+url.PathEscape(probeName), nil, userT1.accessToken)
	if deleteStatus != http.StatusNoContent {
		t.Fatalf("node B quota probe cleanup status=%d body=%s\nnodes:\n%s",
			deleteStatus, deleteBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	assertPooledStorageUsage(t, ctx, pool, t1ID, userT1.userID, 0)

	fillName := "fill-" + randomHex(t, 4) + ".bin"
	fillStatus, fillResp := uploadHarnessFile(t, harness.nodeA.baseURL(), bucket, fillName, fillBody, userT1.accessToken)
	if fillStatus != http.StatusCreated {
		t.Fatalf("node A fill upload status=%d body=%s\nnodes:\n%s",
			fillStatus, fillResp, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	assertPooledStorageUsage(t, ctx, pool, t1ID, userT1.userID, int64(len(fillBody)))

	overflowName := "overflow-" + randomHex(t, 4) + ".bin"
	overflowStatus, overflowResp := uploadHarnessFile(t, harness.nodeB.baseURL(), bucket, overflowName, secondBody, userT1.accessToken)
	if overflowStatus != http.StatusRequestEntityTooLarge || !strings.Contains(overflowResp.raw, "tenant storage quota exceeded") {
		t.Fatalf("node B overflow status=%d body=%s want status=%d tenant quota error\nnodes:\n%s",
			overflowStatus, overflowResp, http.StatusRequestEntityTooLarge, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	t2Name := "tenant-two-" + randomHex(t, 4) + ".bin"
	t2Status, t2Resp := uploadHarnessFile(t, harness.nodeB.baseURL(), bucket, t2Name, secondBody, userT2.accessToken)
	if t2Status != http.StatusCreated {
		t.Fatalf("node B tenant two upload status=%d body=%s\nnodes:\n%s",
			t2Status, t2Resp, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	assertPooledStorageUsage(t, ctx, pool, t1ID, userT1.userID, int64(len(fillBody)))
	assertPooledStorageUsage(t, ctx, pool, t2ID, userT2.userID, int64(len(secondBody)))
}

func TestCrossNodeResumableUploadSpansNodes(t *testing.T) {
	requireMultinodeDatabaseURL(t)
	harness := bootTwoNodeHarness(t)
	auth := registerHarnessUser(t, harness.nodeA.baseURL(), "resumable-cross-node-"+randomHex(t, 4)+"@example.com")
	bucket := "pooled-resumable-" + randomHex(t, 4)
	name := "cross-node-" + randomHex(t, 4) + ".txt"

	seedName := "bucket-seed-" + randomHex(t, 4) + ".txt"
	seedStatus, seedBody := uploadHarnessFile(t, harness.nodeA.baseURL(), bucket, seedName, "seed", auth.accessToken)
	if seedStatus != http.StatusCreated {
		t.Fatalf("seed resumable bucket status=%d body=%s\nnodes:\n%s", seedStatus, seedBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	deleteStatus, deleteBody := harnessRaw(t, http.MethodDelete,
		harness.nodeA.baseURL()+"/api/storage/"+bucket+"/"+url.PathEscape(seedName), nil, auth.accessToken)
	if deleteStatus != http.StatusNoContent {
		t.Fatalf("cleanup resumable bucket seed status=%d body=%s\nnodes:\n%s",
			deleteStatus, deleteBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	part1 := "hello "
	part2 := "multinode resumable"
	full := part1 + part2
	uploadID := tusResumableCreate(t, harness.nodeA.baseURL(), bucket, name, auth.accessToken, int64(len(full)), harness)

	probe := tusResumablePatch(t, harness.nodeB.baseURL(), uploadID, auth.accessToken, 1, part1)
	if probe.status != http.StatusConflict {
		t.Fatalf("mismatched offset probe status=%d body=%s want=%d\nnodes:\n%s",
			probe.status, probe.body, http.StatusConflict, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	first := tusResumablePatch(t, harness.nodeB.baseURL(), uploadID, auth.accessToken, 0, part1)
	if first.status != http.StatusNoContent || first.offset != int64(len(part1)) {
		t.Fatalf("first PATCH status=%d offset=%d body=%s\nnodes:\n%s",
			first.status, first.offset, first.body, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	final := tusResumablePatch(t, harness.nodeB.baseURL(), uploadID, auth.accessToken, int64(len(part1)), part2)
	if final.status != http.StatusNoContent || final.offset != int64(len(full)) {
		t.Fatalf("final PATCH status=%d offset=%d body=%s\nnodes:\n%s",
			final.status, final.offset, final.body, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	readStatus, readBody := waitForHarnessStorageBody(t, harness.nodeA.baseURL(), bucket, name, auth.accessToken, http.StatusOK, full, 10*time.Second)
	if readStatus != http.StatusOK || readBody != full {
		t.Fatalf("node A finalized resumable body status=%d body=%q want=%q\nnodes:\n%s",
			readStatus, readBody, full, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
}

func TestCrossNodeAnonymousPublicServeFailsClosed(t *testing.T) {
	databaseURL := requirePooledTenantDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	harness := bootTwoNodeHarness(t, map[string]string{"AYB_SERVER_REQUIRE_RESOLVED_TENANT": "true"})
	fixture := setupCrossNodeAnonymousPublicServeFixture(ctx, t, databaseURL, harness)

	for _, node := range []*harnessNode{harness.nodeA, harness.nodeB} {
		assertAnonymousPublicServeBody(t, harness, node, fixture.tenantAID, anonymousPublicTenantABody)
		assertAnonymousPublicServeBody(t, harness, node, fixture.tenantBID, anonymousPublicTenantBBody)
		assertAnonymousPublicServeNotFound(t, harness, node, "", anonymousPublicAmbientBody, anonymousPublicTenantABody, anonymousPublicTenantBBody)
	}
}

func TestCrossNodeAnonymousPublicServeSelfHostDefault(t *testing.T) {
	databaseURL := requirePooledTenantDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	harness := bootTwoNodeHarness(t, map[string]string{"AYB_SERVER_REQUIRE_RESOLVED_TENANT": "true"})
	setupCrossNodeAnonymousPublicServeFixture(ctx, t, databaseURL, harness)
	assertAnonymousPublicServeNotFound(t, harness, harness.nodeA, "", anonymousPublicAmbientBody)

	restartTwoNodeHarness(t, harness, map[string]string{"AYB_SERVER_REQUIRE_RESOLVED_TENANT": "false"})

	for _, node := range []*harnessNode{harness.nodeA, harness.nodeB} {
		assertAnonymousPublicServeBody(t, harness, node, "", anonymousPublicAmbientBody)
	}
}

type crossNodeAnonymousPublicServeFixture struct {
	tenantAID string
	tenantBID string
}

func setupCrossNodeAnonymousPublicServeFixture(
	ctx context.Context,
	t *testing.T,
	databaseURL string,
	harness *twoNodeHarness,
) crossNodeAnonymousPublicServeFixture {
	t.Helper()

	userT1 := registerHarnessUser(t, harness.nodeA.baseURL(), "anon-public-t1-"+randomHex(t, 4)+"@example.com")
	userT2 := registerHarnessUser(t, harness.nodeA.baseURL(), "anon-public-t2-"+randomHex(t, 4)+"@example.com")
	configureHarnessUsersAsSharedTenants(ctx, t, databaseURL, userT1.userID, userT2.userID)

	pool := openPooledStoragePool(ctx, t, databaseURL)
	defer pool.Close()
	fixture := crossNodeAnonymousPublicServeFixture{
		tenantAID: resolveHarnessUserTenantID(ctx, t, pool, userT1.userID),
		tenantBID: resolveHarnessUserTenantID(ctx, t, pool, userT2.userID),
	}
	seedAnonymousPublicObject(ctx, t, pool, harness, fixture.tenantAID, anonymousPublicTenantABody)
	seedAnonymousPublicObject(ctx, t, pool, harness, fixture.tenantBID, anonymousPublicTenantBBody)
	seedAnonymousPublicObject(ctx, t, pool, harness, "", anonymousPublicAmbientBody)
	return fixture
}

func seedAnonymousPublicObject(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	harness *twoNodeHarness,
	tenantID string,
	body string,
) {
	t.Helper()

	backend, err := storage.NewS3Backend(ctx, harness.nodeA.minio.S3Config())
	if err != nil {
		t.Fatalf("create S3 backend for anonymous public fixture: %v", err)
	}
	size, err := backend.Put(ctx, tenantID, anonymousPublicBucket, anonymousPublicObject, strings.NewReader(body))
	if err != nil {
		t.Fatalf("seed S3 object tenant=%q: %v", tenantID, err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO _ayb_storage_buckets (tenant_id, name, public)
		VALUES ($1, $2, true)
		ON CONFLICT (tenant_id, name) DO UPDATE
		SET public = EXCLUDED.public, updated_at = NOW()
	`, tenantID, anonymousPublicBucket)
	if err != nil {
		t.Fatalf("seed public bucket tenant=%q: %v", tenantID, err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO _ayb_storage_objects (tenant_id, bucket, name, size, content_type, user_id)
		VALUES ($1, $2, $3, $4, 'text/plain', NULL)
		ON CONFLICT (tenant_id, bucket, name) DO UPDATE
		SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, user_id = NULL, updated_at = NOW()
	`, tenantID, anonymousPublicBucket, anonymousPublicObject, size)
	if err != nil {
		t.Fatalf("seed public object metadata tenant=%q: %v", tenantID, err)
	}
}

func assertAnonymousPublicServeBody(t *testing.T, harness *twoNodeHarness, node *harnessNode, tenantID, wantBody string) {
	t.Helper()

	status, body := harnessAnonymousStorageRead(t, node, anonymousPublicBucket, anonymousPublicObject, tenantID)
	if status != http.StatusOK || body != wantBody {
		t.Fatalf("%s anonymous public read tenant=%q status=%d body=%q want status=%d body=%q\nnodes:\n%s",
			node.name, tenantID, status, body, http.StatusOK, wantBody, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
}

func assertAnonymousPublicServeNotFound(
	t *testing.T,
	harness *twoNodeHarness,
	node *harnessNode,
	tenantID string,
	forbiddenBodies ...string,
) {
	t.Helper()

	status, body := harnessAnonymousStorageRead(t, node, anonymousPublicBucket, anonymousPublicObject, tenantID)
	if status != http.StatusNotFound {
		t.Fatalf("%s anonymous public read tenant=%q status=%d body=%q want status=%d\nnodes:\n%s",
			node.name, tenantID, status, body, http.StatusNotFound, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	for _, forbidden := range forbiddenBodies {
		if strings.Contains(body, forbidden) {
			t.Fatalf("%s anonymous public not-found tenant=%q leaked body %q in %q\nnodes:\n%s",
				node.name, tenantID, forbidden, body, combinedNodeOutput(harness.nodeA, harness.nodeB))
		}
	}
}

func openPooledStoragePool(ctx context.Context, t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for pooled storage assertions: %v", err)
	}
	return pool
}

func resolveHarnessUserTenantID(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	var tenantID string
	err := pool.QueryRow(ctx, `
		SELECT tenants.id::text
		  FROM _ayb_tenants AS tenants
		  JOIN _ayb_tenant_memberships AS memberships ON memberships.tenant_id = tenants.id
		 WHERE memberships.user_id = $1
		 ORDER BY memberships.created_at ASC
		 LIMIT 1
	`, userID).Scan(&tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			t.Fatalf("user %s did not resolve to a tenant", userID)
		}
		t.Fatalf("resolve tenant for user %s: %v", userID, err)
	}
	return tenantID
}

func assertPooledStorageUsage(ctxTest *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, userID string, want int64) {
	ctxTest.Helper()
	var got int64
	err := pool.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT bytes_used
			  FROM _ayb_storage_usage
			 WHERE tenant_id = $1 AND user_id = $2
		), 0)
	`, tenantID, userID).Scan(&got)
	if err != nil {
		ctxTest.Fatalf("read storage usage tenant=%s user=%s: %v", tenantID, userID, err)
	}
	if got != want {
		ctxTest.Fatalf("storage usage tenant=%s user=%s bytes=%d want=%d", tenantID, userID, got, want)
	}
}

func tusResumableCreate(
	t *testing.T,
	baseURL string,
	bucket string,
	name string,
	token string,
	length int64,
	harness *twoNodeHarness,
) string {
	t.Helper()
	endpoint := baseURL + "/api/storage/upload/resumable/?bucket=" + url.QueryEscape(bucket) + "&name=" + url.QueryEscape(name)
	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		t.Fatalf("build TUS create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.FormatInt(length, 10))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("TUS create failed: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read TUS create response: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("TUS create status=%d body=%s\nnodes:\n%s", resp.StatusCode, raw, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}
	if resp.Header.Get("Location") == "" {
		t.Fatalf("TUS create response missing Location header body=%s\nnodes:\n%s", raw, combinedNodeOutput(harness.nodeA, harness.nodeB))
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode TUS create response: %v\nbody=%s", err, raw)
	}
	if payload.ID == "" {
		t.Fatalf("TUS create response missing id: %s", raw)
	}
	return payload.ID
}

type tusPatchResult struct {
	status int
	offset int64
	body   string
}

func tusResumablePatch(t *testing.T, baseURL, id, token string, offset int64, body string) tusPatchResult {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, baseURL+"/api/storage/upload/resumable/"+url.PathEscape(id), bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("build TUS PATCH request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	req.Header.Set("Content-Type", "application/offset+octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("TUS PATCH failed: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read TUS PATCH response: %v", err)
	}
	gotOffset, _ := strconv.ParseInt(resp.Header.Get("Upload-Offset"), 10, 64)
	return tusPatchResult{status: resp.StatusCode, offset: gotOffset, body: string(raw)}
}
