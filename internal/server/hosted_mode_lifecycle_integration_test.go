//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const hostedModeLifecycleJWTSecret = "hosted-mode-lifecycle-secret-32-chars"

type hostedModeLifecycleHarness struct {
	cfg       *config.Config
	pool      *pgxpool.Pool
	srv       *Server
	tenantSvc *tenant.Service
}

func setupHostedModeLifecycleHarness(t *testing.T) *hostedModeLifecycleHarness {
	t.Helper()
	return setupHostedModeLifecycleHarnessWithConfig(t, nil)
}

func setupHostedModeLifecycleHarnessWithConfig(t *testing.T, configure func(*config.Config)) *hostedModeLifecycleHarness {
	t.Helper()

	ctx := context.Background()
	pg := newRequestLoggerTestDB(t)
	ensureIntegrationMigrations(t, ctx, pg.Pool)

	logger := testutil.DiscardLogger()
	cache := schema.NewCacheHolder(pg.Pool, logger)
	testutil.NoError(t, cache.Load(ctx))

	cfg := hostedModeLifecycleTestConfig()
	if configure != nil {
		configure(cfg)
	}
	authSvc := auth.NewService(
		pg.Pool,
		cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.TokenDuration)*time.Second,
		time.Duration(cfg.Auth.RefreshTokenDuration)*time.Second,
		cfg.Auth.MinPasswordLength,
		logger,
	)
	backend, err := storage.NewLocalBackend(t.TempDir())
	testutil.NoError(t, err)
	storageSvc := storage.NewService(pg.Pool, backend, "hosted-mode-storage-sign-key-32-chars", logger, 0)
	srv := New(cfg, logger, cache, pg.Pool, authSvc, storageSvc)

	tenantSvc := tenant.NewService(pg.Pool, logger)
	usageAcc := tenant.NewUsageAccumulator(pg.Pool, logger)
	srv.SetTenantService(tenantSvc)
	srv.SetUsageAccumulator(usageAcc)
	srv.SetQuotaChecker(tenant.DefaultQuotaChecker{})
	rateLimiter := tenant.NewTenantRateLimiter(time.Minute)
	srv.SetTenantRateLimiter(rateLimiter)
	t.Cleanup(rateLimiter.Stop)
	srv.SetTenantConnCounter(tenant.NewTenantConnCounter())

	return &hostedModeLifecycleHarness{
		cfg:       cfg,
		pool:      pg.Pool,
		srv:       srv,
		tenantSvc: tenantSvc,
	}
}

func hostedModeLifecycleTestConfig() *config.Config {
	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	cfg.Auth.JWTSecret = hostedModeLifecycleJWTSecret
	cfg.Storage.Enabled = true
	return cfg
}

func (h *hostedModeLifecycleHarness) adminLogin(t *testing.T) string {
	t.Helper()
	return stage5AdminLogin(t, h.srv)
}

func (h *hostedModeLifecycleHarness) adminRequest(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	return stage5TenantAdminRequest(t, h.srv, method, path, token, "", body)
}

func (h *hostedModeLifecycleHarness) ensureUser(t *testing.T, userID string) {
	t.Helper()
	stage5EnsureUser(t, h.srv, userID)
}

func (h *hostedModeLifecycleHarness) activateTenant(t *testing.T, tenantID string) tenant.Tenant {
	t.Helper()
	return stage5ActivateTenant(t, h.srv, tenantID)
}

func (h *hostedModeLifecycleHarness) tenantCreateBody(name, slug, ownerUserID string) string {
	body, err := json.Marshal(map[string]string{
		"name":          name,
		"slug":          slug,
		"ownerUserId":   ownerUserID,
		"isolationMode": "shared",
		"planTier":      "free",
		"region":        "default",
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func (h *hostedModeLifecycleHarness) tenantAuthToken(t *testing.T, userID, email, tenantID string) string {
	t.Helper()
	return signedTenantTestToken(t, h.cfg.Auth.JWTSecret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email:    email,
		TenantID: tenantID,
	})
}

func (h *hostedModeLifecycleHarness) tenantAuthHeaders(t *testing.T, userID, email, tenantID string) map[string]string {
	t.Helper()
	token := h.tenantAuthToken(t, userID, email, tenantID)
	return map[string]string{
		"Authorization": "Bearer " + token,
		"X-Tenant-ID":   tenantID,
	}
}

func (h *hostedModeLifecycleHarness) jsonRequest(
	t *testing.T,
	method,
	path string,
	body any,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		encoded, err := json.Marshal(body)
		testutil.NoError(t, err)
		reader = strings.NewReader(string(encoded))
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp := httptest.NewRecorder()
	h.srv.Router().ServeHTTP(resp, req)
	return resp
}

func (h *hostedModeLifecycleHarness) storageUploadRequest(
	t *testing.T,
	bucket,
	filename,
	bodyText string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", filename)
	testutil.NoError(t, err)
	_, err = fileWriter.Write([]byte(bodyText))
	testutil.NoError(t, err)
	testutil.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/storage/"+bucket, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp := httptest.NewRecorder()
	h.srv.Router().ServeHTTP(resp, req)
	return resp
}

func (h *hostedModeLifecycleHarness) storageListRequest(t *testing.T, bucket string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return h.rawStorageRequest(t, http.MethodGet, "/api/storage/"+bucket, headers)
}

func (h *hostedModeLifecycleHarness) storageReadRequest(t *testing.T, bucket, name string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return h.rawStorageRequest(t, http.MethodGet, "/api/storage/"+bucket+"/"+name, headers)
}

func (h *hostedModeLifecycleHarness) anonymousStorageReadRequest(
	t *testing.T,
	bucket,
	name,
	tenantID string,
) *httptest.ResponseRecorder {
	t.Helper()
	headers := map[string]string{}
	if tenantID != "" {
		headers["X-Tenant-ID"] = tenantID
	}
	return h.rawStorageRequest(t, http.MethodGet, "/api/storage/"+bucket+"/"+name, headers)
}

func (h *hostedModeLifecycleHarness) anonymousStorageListRequest(
	t *testing.T,
	bucket,
	tenantID string,
) *httptest.ResponseRecorder {
	t.Helper()
	headers := map[string]string{}
	if tenantID != "" {
		headers["X-Tenant-ID"] = tenantID
	}
	return h.rawStorageRequest(t, http.MethodGet, "/api/storage/"+bucket, headers)
}

func (h *hostedModeLifecycleHarness) rawStorageRequest(
	t *testing.T,
	method,
	path string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp := httptest.NewRecorder()
	h.srv.Router().ServeHTTP(resp, req)
	return resp
}

func (h *hostedModeLifecycleHarness) collectionRequest(
	t *testing.T,
	method,
	collectionPath string,
	body any,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/" + strings.TrimPrefix(collectionPath, "/")
	return h.jsonRequest(t, method, path, body, headers)
}

func decodeHostedModeTenant(t *testing.T, resp *httptest.ResponseRecorder) tenant.Tenant {
	t.Helper()

	var result tenant.Tenant
	testutil.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	testutil.True(t, result.ID != "", "tenant response should include id")
	return result
}

func assertLifecycleItemCreated(t *testing.T, resp *httptest.ResponseRecorder, wantName string) {
	t.Helper()
	testutil.StatusCode(t, http.StatusCreated, resp.Code)

	var body map[string]any
	testutil.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	gotName, ok := body["name"].(string)
	testutil.True(t, ok, "created item name should be a string")
	testutil.Equal(t, wantName, gotName)
	testutil.True(t, body["id"] != nil, "created item should include id")
}

func assertLifecycleCollectionContainsName(t *testing.T, resp *httptest.ResponseRecorder, wantName string) {
	t.Helper()
	testutil.StatusCode(t, http.StatusOK, resp.Code)

	var body struct {
		TotalItems int              `json:"totalItems"`
		Items      []map[string]any `json:"items"`
	}
	testutil.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	testutil.True(t, body.TotalItems >= 1, "collection list should include at least one item")
	for _, item := range body.Items {
		if item["name"] == wantName {
			return
		}
	}
	t.Fatalf("collection list did not include item named %q: %#v", wantName, body.Items)
}

func assertLifecycleStorageRead(t *testing.T, resp *httptest.ResponseRecorder, wantBody string) {
	t.Helper()
	testutil.StatusCode(t, http.StatusOK, resp.Code)
	testutil.Equal(t, wantBody, resp.Body.String())
}

func assertLifecycleStorageNotFound(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	testutil.StatusCode(t, http.StatusNotFound, resp.Code)

	var body struct {
		Message string `json:"message"`
	}
	testutil.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	testutil.Equal(t, "file not found", body.Message)
}

func assertLifecyclePublicStorageNotFound(t *testing.T, resp *httptest.ResponseRecorder, forbiddenBodies ...string) {
	t.Helper()
	testutil.StatusCode(t, http.StatusNotFound, resp.Code)
	body := resp.Body.String()
	for _, forbidden := range forbiddenBodies {
		testutil.True(t, !strings.Contains(body, forbidden), "not found response leaked body %q: %q", forbidden, body)
	}
}

func assertLifecycleStorageListNames(t *testing.T, resp *httptest.ResponseRecorder, wantNames ...string) {
	t.Helper()
	testutil.StatusCode(t, http.StatusOK, resp.Code)

	var body struct {
		Items      []storage.Object `json:"items"`
		TotalItems int              `json:"totalItems"`
	}
	testutil.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	testutil.Equal(t, len(wantNames), body.TotalItems)
	testutil.Equal(t, len(wantNames), len(body.Items))

	gotCounts := make(map[string]int, len(body.Items))
	for _, item := range body.Items {
		gotCounts[item.Name]++
	}
	testutil.Equal(t, len(wantNames), len(gotCounts))
	for _, wantName := range wantNames {
		testutil.Equal(t, 1, gotCounts[wantName])
	}
}

type anonymousPublicServeFixture struct {
	tenantAID string
	tenantBID string
}

func setupAnonymousPublicServeFixture(t *testing.T, h *hostedModeLifecycleHarness) anonymousPublicServeFixture {
	t.Helper()

	adminToken := h.adminLogin(t)
	h.ensureUser(t, stageIntegrationOwnerUserID)
	h.ensureUser(t, stageIntegrationMemberUserID)

	slugBase := fmt.Sprintf("anonymous-public-serve-%d", time.Now().UnixNano())
	tenantA := createHostedModeLifecycleTenant(
		t,
		h,
		adminToken,
		"anonymous public tenant a",
		slugBase+"-a",
		stageIntegrationOwnerUserID,
	)
	tenantB := createHostedModeLifecycleTenant(
		t,
		h,
		adminToken,
		"anonymous public tenant b",
		slugBase+"-b",
		stageIntegrationMemberUserID,
	)

	activatedTenantA := h.activateTenant(t, tenantA.ID)
	testutil.Equal(t, tenant.TenantStateActive, activatedTenantA.State)
	activatedTenantB := h.activateTenant(t, tenantB.ID)
	testutil.Equal(t, tenant.TenantStateActive, activatedTenantB.State)

	createPublicBucket(t, h, tenantA.ID)
	createPublicBucket(t, h, tenantB.ID)
	createPublicBucket(t, h, "")

	uploadPublicObject(t, h, tenantA.ID, "A-bytes")
	uploadPublicObject(t, h, tenantB.ID, "B-bytes")
	uploadPublicObject(t, h, "", "ambient-bytes")

	return anonymousPublicServeFixture{
		tenantAID: tenantA.ID,
		tenantBID: tenantB.ID,
	}
}

func createHostedModeLifecycleTenant(
	t *testing.T,
	h *hostedModeLifecycleHarness,
	adminToken,
	name,
	slug,
	ownerUserID string,
) tenant.Tenant {
	t.Helper()

	createResp := h.adminRequest(
		t,
		http.MethodPost,
		"/api/admin/tenants",
		adminToken,
		h.tenantCreateBody(name, slug, ownerUserID),
	)
	testutil.StatusCode(t, http.StatusCreated, createResp.Code)
	created := decodeHostedModeTenant(t, createResp)
	testutil.Equal(t, "shared", created.IsolationMode)
	return created
}

func createPublicBucket(t *testing.T, h *hostedModeLifecycleHarness, tenantID string) {
	t.Helper()
	_, err := h.srv.storageSvc.CreateBucket(tenant.ContextWithTenantID(context.Background(), tenantID), "pub", true)
	testutil.NoError(t, err)
}

func uploadPublicObject(t *testing.T, h *hostedModeLifecycleHarness, tenantID, body string) {
	t.Helper()
	ctx := tenant.ContextWithTenantID(context.Background(), tenantID)
	_, err := h.srv.storageSvc.Upload(ctx, "pub", "same.txt", "text/plain", nil, strings.NewReader(body))
	testutil.NoError(t, err)
}

func assertLifecycleDataPath(
	t *testing.T,
	h *hostedModeLifecycleHarness,
	headers map[string]string,
	wantItemName,
	wantObjectBody string,
) {
	t.Helper()

	collectionResp := h.collectionRequest(t, http.MethodGet, "collections/lifecycle_test_items", nil, headers)
	assertLifecycleCollectionContainsName(t, collectionResp, wantItemName)

	storageResp := h.storageReadRequest(t, "testbucket", "hello.txt", headers)
	assertLifecycleStorageRead(t, storageResp, wantObjectBody)
}

func TestHostedModeLifecycleHarnessScaffold(t *testing.T) {
	h := setupHostedModeLifecycleHarness(t)

	adminToken := h.adminLogin(t)
	h.ensureUser(t, stageIntegrationOwnerUserID)

	slug := fmt.Sprintf("hosted-mode-%d", time.Now().UnixNano())
	createBody := h.tenantCreateBody("hosted mode scaffold", slug, stageIntegrationOwnerUserID)
	createResp := h.adminRequest(t, http.MethodPost, "/api/admin/tenants", adminToken, createBody)
	testutil.StatusCode(t, http.StatusCreated, createResp.Code)

	var created tenant.Tenant
	testutil.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	testutil.True(t, created.ID != "", "created tenant should include id")
	testutil.Equal(t, slug, created.Slug)
	testutil.Equal(t, "shared", created.IsolationMode)

	activated := h.activateTenant(t, created.ID)
	testutil.Equal(t, tenant.TenantStateActive, activated.State)

	headers := h.tenantAuthHeaders(
		t,
		stageIntegrationOwnerUserID,
		"integration-owner@example.com",
		created.ID,
	)
	testutil.True(t, strings.HasPrefix(headers["Authorization"], "Bearer "), "tenant auth should include bearer token")
	testutil.Equal(t, created.ID, headers["X-Tenant-ID"])

	schemaResp := h.jsonRequest(t, http.MethodGet, "/api/schema", nil, headers)
	testutil.StatusCode(t, http.StatusOK, schemaResp.Code)

	var schemaBody map[string]any
	testutil.NoError(t, json.Unmarshal(schemaResp.Body.Bytes(), &schemaBody))
	_, hasTables := schemaBody["tables"].(map[string]any)
	testutil.True(t, hasTables, "schema response should include tables")
}

func TestHostedModeLifecycle(t *testing.T) {
	h := setupHostedModeLifecycleHarness(t)
	ctx := context.Background()

	adminToken := h.adminLogin(t)
	h.ensureUser(t, stageIntegrationOwnerUserID)

	_, err := h.pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS lifecycle_test_items (id serial PRIMARY KEY, name text)")
	testutil.NoError(t, err)
	testutil.NoError(t, h.srv.schema.Reload(ctx))

	slug := fmt.Sprintf("hosted-mode-lifecycle-%d", time.Now().UnixNano())
	createResp := h.adminRequest(
		t,
		http.MethodPost,
		"/api/admin/tenants",
		adminToken,
		h.tenantCreateBody("hosted mode lifecycle", slug, stageIntegrationOwnerUserID),
	)
	testutil.StatusCode(t, http.StatusCreated, createResp.Code)
	created := decodeHostedModeTenant(t, createResp)
	testutil.Equal(t, "shared", created.IsolationMode)
	testutil.True(t,
		created.State == tenant.TenantStateProvisioning || created.State == tenant.TenantStateActive,
		"unexpected created tenant state %s",
		created.State,
	)

	tenantCtx := tenant.ContextWithTenantID(ctx, created.ID)
	_, err = h.srv.storageSvc.CreateBucket(tenantCtx, "testbucket", false)
	testutil.NoError(t, err)

	activated := h.activateTenant(t, created.ID)
	testutil.Equal(t, tenant.TenantStateActive, activated.State)

	headers := h.tenantAuthHeaders(
		t,
		stageIntegrationOwnerUserID,
		"integration-owner@example.com",
		created.ID,
	)

	itemName := "test-item"
	itemCreateResp := h.collectionRequest(t, http.MethodPost, "collections/lifecycle_test_items", map[string]string{
		"name": itemName,
	}, headers)
	assertLifecycleItemCreated(t, itemCreateResp, itemName)
	assertLifecycleCollectionContainsName(
		t,
		h.collectionRequest(t, http.MethodGet, "collections/lifecycle_test_items", nil, headers),
		itemName,
	)

	objectBody := "hello world"
	uploadResp := h.storageUploadRequest(t, "testbucket", "hello.txt", objectBody, headers)
	testutil.StatusCode(t, http.StatusCreated, uploadResp.Code)
	assertLifecycleStorageRead(t, h.storageReadRequest(t, "testbucket", "hello.txt", headers), objectBody)

	suspendResp := h.adminRequest(t, http.MethodPost, "/api/admin/tenants/"+created.ID+"/suspend", adminToken, "")
	testutil.StatusCode(t, http.StatusOK, suspendResp.Code)
	suspended := decodeHostedModeTenant(t, suspendResp)
	testutil.Equal(t, tenant.TenantStateSuspended, suspended.State)

	// GAP: suspension changes state without blocking the data path - enforceTenantContext/enforceTenantMatch have no state check.
	// Ship decision: acceptable for MVP, add enforceTenantActive middleware guard when suspension must be enforced.
	assertLifecycleDataPath(t, h, headers, itemName, objectBody)

	resumeResp := h.adminRequest(t, http.MethodPost, "/api/admin/tenants/"+created.ID+"/resume", adminToken, "")
	testutil.StatusCode(t, http.StatusOK, resumeResp.Code)
	resumed := decodeHostedModeTenant(t, resumeResp)
	testutil.Equal(t, tenant.TenantStateActive, resumed.State)
	assertLifecycleDataPath(t, h, headers, itemName, objectBody)

	deleteResp := h.adminRequest(t, http.MethodDelete, "/api/admin/tenants/"+created.ID, adminToken, "")
	testutil.StatusCode(t, http.StatusOK, deleteResp.Code)
	deleting := decodeHostedModeTenant(t, deleteResp)
	testutil.Equal(t, tenant.TenantStateDeleting, deleting.State)
}

func TestTenantStorageIsolation(t *testing.T) {
	h := setupHostedModeLifecycleHarness(t)
	ctx := context.Background()

	adminToken := h.adminLogin(t)
	h.ensureUser(t, stageIntegrationOwnerUserID)
	h.ensureUser(t, stageIntegrationMemberUserID)

	slugBase := fmt.Sprintf("hosted-mode-storage-%d", time.Now().UnixNano())
	createTenantAResp := h.adminRequest(
		t,
		http.MethodPost,
		"/api/admin/tenants",
		adminToken,
		h.tenantCreateBody("hosted mode storage tenant a", slugBase+"-a", stageIntegrationOwnerUserID),
	)
	testutil.StatusCode(t, http.StatusCreated, createTenantAResp.Code)
	tenantA := decodeHostedModeTenant(t, createTenantAResp)
	testutil.Equal(t, "shared", tenantA.IsolationMode)

	createTenantBResp := h.adminRequest(
		t,
		http.MethodPost,
		"/api/admin/tenants",
		adminToken,
		h.tenantCreateBody("hosted mode storage tenant b", slugBase+"-b", stageIntegrationMemberUserID),
	)
	testutil.StatusCode(t, http.StatusCreated, createTenantBResp.Code)
	tenantB := decodeHostedModeTenant(t, createTenantBResp)
	testutil.Equal(t, "shared", tenantB.IsolationMode)

	activatedTenantA := h.activateTenant(t, tenantA.ID)
	testutil.Equal(t, tenant.TenantStateActive, activatedTenantA.State)
	activatedTenantB := h.activateTenant(t, tenantB.ID)
	testutil.Equal(t, tenant.TenantStateActive, activatedTenantB.State)

	_, err := h.srv.storageSvc.CreateBucket(tenant.ContextWithTenantID(ctx, tenantA.ID), "sharedbucket", false)
	testutil.NoError(t, err)
	_, err = h.srv.storageSvc.CreateBucket(tenant.ContextWithTenantID(ctx, tenantB.ID), "sharedbucket", false)
	testutil.NoError(t, err)

	headersA := h.tenantAuthHeaders(
		t,
		stageIntegrationOwnerUserID,
		"tenant-a-storage@example.com",
		tenantA.ID,
	)
	headersB := h.tenantAuthHeaders(
		t,
		stageIntegrationMemberUserID,
		"tenant-b-storage@example.com",
		tenantB.ID,
	)
	testutil.True(t, headersA["Authorization"] != headersB["Authorization"], "tenant tokens should be distinct")
	testutil.Equal(t, tenantA.ID, headersA["X-Tenant-ID"])
	testutil.Equal(t, tenantB.ID, headersB["X-Tenant-ID"])

	const (
		sameBodyA       = "tenant-a-same-bytes"
		sameBodyB       = "tenant-b-same-bytes"
		tenantAOnlyName = "tenant-a-only.txt"
		tenantBOnlyName = "tenant-b-only.txt"
		tenantAOnlyBody = "tenant-a-private-bytes"
		tenantBOnlyBody = "tenant-b-private-bytes"
	)

	uploadSameA := h.storageUploadRequest(t, "sharedbucket", "same.txt", sameBodyA, headersA)
	testutil.StatusCode(t, http.StatusCreated, uploadSameA.Code)
	uploadSameB := h.storageUploadRequest(t, "sharedbucket", "same.txt", sameBodyB, headersB)
	testutil.StatusCode(t, http.StatusCreated, uploadSameB.Code)

	uploadTenantAOnly := h.storageUploadRequest(t, "sharedbucket", tenantAOnlyName, tenantAOnlyBody, headersA)
	testutil.StatusCode(t, http.StatusCreated, uploadTenantAOnly.Code)
	uploadTenantBOnly := h.storageUploadRequest(t, "sharedbucket", tenantBOnlyName, tenantBOnlyBody, headersB)
	testutil.StatusCode(t, http.StatusCreated, uploadTenantBOnly.Code)

	assertLifecycleStorageRead(t, h.storageReadRequest(t, "sharedbucket", "same.txt", headersA), sameBodyA)
	assertLifecycleStorageRead(t, h.storageReadRequest(t, "sharedbucket", "same.txt", headersB), sameBodyB)

	assertLifecycleStorageRead(t, h.storageReadRequest(t, "sharedbucket", tenantAOnlyName, headersA), tenantAOnlyBody)
	assertLifecycleStorageRead(t, h.storageReadRequest(t, "sharedbucket", tenantBOnlyName, headersB), tenantBOnlyBody)
	assertLifecycleStorageNotFound(t, h.storageReadRequest(t, "sharedbucket", tenantAOnlyName, headersB))
	assertLifecycleStorageNotFound(t, h.storageReadRequest(t, "sharedbucket", tenantBOnlyName, headersA))

	assertLifecycleStorageListNames(
		t,
		h.storageListRequest(t, "sharedbucket", headersA),
		"same.txt",
		tenantAOnlyName,
	)
	assertLifecycleStorageListNames(
		t,
		h.storageListRequest(t, "sharedbucket", headersB),
		"same.txt",
		tenantBOnlyName,
	)
}

func TestAnonymousPublicServeTenantCorrect(t *testing.T) {
	t.Run("pooled mode requires resolved tenant for anonymous public reads", func(t *testing.T) {
		h := setupHostedModeLifecycleHarnessWithConfig(t, func(cfg *config.Config) {
			cfg.Server.RequireResolvedTenant = true
		})
		fixture := setupAnonymousPublicServeFixture(t, h)

		assertLifecycleStorageRead(
			t,
			h.anonymousStorageReadRequest(t, "pub", "same.txt", fixture.tenantAID),
			"A-bytes",
		)
		assertLifecycleStorageRead(
			t,
			h.anonymousStorageReadRequest(t, "pub", "same.txt", fixture.tenantBID),
			"B-bytes",
		)
		assertLifecycleStorageListNames(
			t,
			h.anonymousStorageListRequest(t, "pub", fixture.tenantAID),
			"same.txt",
		)
		assertLifecycleStorageListNames(
			t,
			h.anonymousStorageListRequest(t, "pub", fixture.tenantBID),
			"same.txt",
		)
		assertLifecyclePublicStorageNotFound(
			t,
			h.anonymousStorageReadRequest(t, "pub", "same.txt", ""),
			"ambient-bytes",
			"A-bytes",
			"B-bytes",
		)
		assertLifecyclePublicStorageNotFound(
			t,
			h.anonymousStorageListRequest(t, "pub", ""),
			"same.txt",
		)
	})

	t.Run("self hosted default keeps ambient anonymous public reads", func(t *testing.T) {
		h := setupHostedModeLifecycleHarness(t)
		setupAnonymousPublicServeFixture(t, h)

		assertLifecycleStorageRead(
			t,
			h.anonymousStorageReadRequest(t, "pub", "same.txt", ""),
			"ambient-bytes",
		)
		assertLifecycleStorageListNames(
			t,
			h.anonymousStorageListRequest(t, "pub", ""),
			"same.txt",
		)
	})
}
