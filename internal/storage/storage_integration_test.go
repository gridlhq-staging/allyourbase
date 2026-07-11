//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

func applyStorageTemplate(t *testing.T, baseURL, adminJWT, template, payload string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/admin/rls/templates/storage-objects/"+template, strings.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

var (
	sharedPG      *testutil.PGContainer
	sharedCleanup func()
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	sharedCleanup = cleanup
	code := m.Run()
	sharedCleanup()
	os.Exit(code)
}

func setupServer(t *testing.T) *httptest.Server {
	t.Helper()

	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	// Run migrations.
	runner := migrations.NewRunner(pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create local storage backend.
	dir := t.TempDir()
	backend, err := storage.NewLocalBackend(dir)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}

	storageSvc := storage.NewService(pool, backend, "test-sign-key-at-least-32-chars!!", logger, 0)

	cfg := config.Default()
	cfg.Storage.Enabled = true
	ch := schema.NewCacheHolder(pool, logger)

	srv := server.New(cfg, logger, ch, pool, nil, storageSvc)
	return httptest.NewServer(srv.Router())
}

func setupServerWithAuthAndStorageAdmin(t *testing.T) (*httptest.Server, *storage.Service, *auth.Service) {
	t.Helper()
	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	runner := migrations.NewRunner(pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	dir := t.TempDir()
	backend, err := storage.NewLocalBackend(dir)
	testutil.NoError(t, err)
	storageSvc := storage.NewService(pool, backend, "test-sign-key-at-least-32-chars!!", logger, 0)

	cfg := config.Default()
	cfg.Storage.Enabled = true
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "jwt-secret-test-at-least-32-chars!!"
	authSvc := auth.NewService(pool, cfg.Auth.JWTSecret, time.Hour, 7*24*time.Hour, 8, logger)

	ch := schema.NewCacheHolder(pool, logger)
	srv := server.New(cfg, logger, ch, pool, authSvc, storageSvc)
	return httptest.NewServer(srv.Router()), storageSvc, authSvc
}

func adminToken(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/admin/auth", "application/json", strings.NewReader(`{"password":"admin-pass"}`))
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	var payload map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	token := payload["token"]
	testutil.True(t, token != "", "expected admin token")
	return token
}

func userToken(t *testing.T, authSvc *auth.Service, userID, email string) string {
	t.Helper()
	token, err := authSvc.IssueTestToken(userID, email)
	testutil.NoError(t, err)
	testutil.True(t, token != "", "expected user token")
	return token
}

func tenantContextJWT(t *testing.T, tenantID, subject string) string {
	t.Helper()
	now := time.Now().UTC()
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:    subject + "@example.com",
		TenantID: tenantID,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("jwt-secret-test-at-least-32-chars!!"))
	testutil.NoError(t, err)
	return signed
}

func clearStorageData(t *testing.T) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(context.Background(), "TRUNCATE _ayb_storage_uploads, _ayb_storage_objects, _ayb_storage_buckets")
	testutil.NoError(t, err)
}

func assertStorageDownloadBody(t *testing.T, ctx context.Context, svc *storage.Service, bucket, name, want string) *storage.Object {
	t.Helper()
	rc, obj, err := svc.Download(ctx, bucket, name)
	testutil.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	testutil.NoError(t, err)
	testutil.Equal(t, want, string(got))
	return obj
}

func TestStorageUploadAndServe(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload a file.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "hello.txt")
	fw.Write([]byte("Hello, Storage!"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/storage/testbucket", w.FormDataContentType(), body)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)

	var obj map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&obj))
	resp.Body.Close()

	testutil.Equal(t, "testbucket", obj["bucket"])
	testutil.Equal(t, "hello.txt", obj["name"])
	testutil.Equal(t, float64(15), obj["size"].(float64))

	// Serve the file.
	resp, err = http.Get(ts.URL + "/api/storage/testbucket/hello.txt")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.Equal(t, "Hello, Storage!", string(got))
}

func TestStorageDelete(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload.
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, _ := w.CreateFormFile("file", "delete-me.txt")
	fw.Write([]byte("bye"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/storage/testbucket", w.FormDataContentType(), body)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Delete.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/testbucket/delete-me.txt", nil)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Serve should 404.
	resp, _ = http.Get(ts.URL + "/api/storage/testbucket/delete-me.txt")
	testutil.StatusCode(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestStorageList(t *testing.T) {
	ts := setupServer(t)
	defer ts.Close()

	// Upload 3 files.
	for i := 0; i < 3; i++ {
		body := &bytes.Buffer{}
		w := multipart.NewWriter(body)
		fw, err := w.CreateFormFile("file", fmt.Sprintf("file%d.txt", i))
		testutil.NoError(t, err)
		_, err = fw.Write([]byte(fmt.Sprintf("content %d", i)))
		testutil.NoError(t, err)
		w.Close()
		resp, err := http.Post(ts.URL+"/api/storage/listbucket", w.FormDataContentType(), body)
		testutil.NoError(t, err)
		testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/api/storage/listbucket")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var list map[string]any
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	resp.Body.Close()

	testutil.Equal(t, float64(3), list["totalItems"].(float64))
	items := list["items"].([]any)
	testutil.Equal(t, 3, len(items))
}

func TestStorageSignedURL(t *testing.T) {
	ts, _, _, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	resp, err := uploadFile(t, ts.URL, "signbucket", "signed.txt", "signed content", requestHeaders{token: adminJWT, tenantID: tenantID})
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Generate signed URL.
	signBody := bytes.NewReader([]byte(`{"expiresIn": 3600}`))
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/signbucket/signed.txt/sign", signBody)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var signResp map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&signResp))
	resp.Body.Close()

	signedURL := signResp["url"]
	testutil.True(t, signedURL != "", "should have a URL")
	parsedSignedURL, err := url.Parse(signedURL)
	testutil.NoError(t, err)
	testutil.Equal(t, tenantID, parsedSignedURL.Query().Get("tenant"))

	// Fetch via signed URL without depending on a second tenant resolver.
	resp, err = http.Get(ts.URL + signedURL)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	testutil.Equal(t, "signed content", string(got))
}

func TestStorageSignedURLTenantIsolation(t *testing.T) {
	ts, storageSvc, _, tenantA := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	ctx := context.Background()
	tenantSvc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	tenantB := createQuotaTestTenant(t, ctx, tenantSvc, "signed-url-isolation").ID
	ctxA := tenant.ContextWithTenantID(ctx, tenantA)
	ctxB := tenant.ContextWithTenantID(ctx, tenantB)

	const (
		bucket      = "signedshared"
		objectName  = "same.txt"
		bodyTenantA = "tenant-a-signed-body"
		bodyTenantB = "tenant-b-signed-body"
	)
	_, err := storageSvc.CreateBucket(ctxA, bucket, false)
	testutil.NoError(t, err)
	_, err = storageSvc.CreateBucket(ctxB, bucket, false)
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctxA, bucket, objectName, "text/plain", nil, strings.NewReader(bodyTenantA))
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctxB, bucket, objectName, "text/plain", nil, strings.NewReader(bodyTenantB))
	testutil.NoError(t, err)

	signBody := bytes.NewReader([]byte(`{"expiresIn":3600}`))
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/"+bucket+"/"+objectName+"/sign", signBody)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantA)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	var signResp map[string]string
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&signResp))
	resp.Body.Close()

	signedURL := signResp["url"]
	parsedSignedURL, err := url.Parse(signedURL)
	testutil.NoError(t, err)
	testutil.Equal(t, tenantA, parsedSignedURL.Query().Get("tenant"))

	reqGet, err := http.NewRequest(http.MethodGet, ts.URL+signedURL, nil)
	testutil.NoError(t, err)
	reqGet.Header.Set("X-Tenant-ID", tenantB)
	resp, err = http.DefaultClient.Do(reqGet)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	got, err := io.ReadAll(resp.Body)
	testutil.NoError(t, err)
	resp.Body.Close()
	testutil.Equal(t, bodyTenantA, string(got))
}

func TestStoragePublicServeTenantIsolation(t *testing.T) {
	ts, storageSvc, _, tenantA := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	ctx := context.Background()
	tenantSvc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	tenantB := createQuotaTestTenant(t, ctx, tenantSvc, "public-serve-isolation").ID
	ctxA := tenant.ContextWithTenantID(ctx, tenantA)
	ctxB := tenant.ContextWithTenantID(ctx, tenantB)

	const (
		bucket      = "publicshared"
		objectName  = "same.txt"
		bodyTenantA = "tenant-a-public-body"
		bodyTenantB = "tenant-b-public-body"
	)
	_, err := storageSvc.CreateBucket(ctxA, bucket, true)
	testutil.NoError(t, err)
	_, err = storageSvc.CreateBucket(ctxB, bucket, true)
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctxA, bucket, objectName, "text/plain", nil, strings.NewReader(bodyTenantA))
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctxB, bucket, objectName, "text/plain", nil, strings.NewReader(bodyTenantB))
	testutil.NoError(t, err)

	testCases := []struct {
		name     string
		tenantID string
		token    string
		wantBody string
	}{
		{name: "anonymous header tenant a", tenantID: tenantA, wantBody: bodyTenantA},
		{name: "anonymous header tenant b", tenantID: tenantB, wantBody: bodyTenantB},
		{
			name:     "jwt tenant a",
			token:    tenantContextJWT(t, tenantA, "public-serve-user-a"),
			wantBody: bodyTenantA,
		},
		{
			name:     "jwt tenant b",
			token:    tenantContextJWT(t, tenantB, "public-serve-user-b"),
			wantBody: bodyTenantB,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/"+objectName, nil)
			testutil.NoError(t, err)
			if tc.tenantID != "" {
				req.Header.Set("X-Tenant-ID", tc.tenantID)
			}
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}

			resp, err := http.DefaultClient.Do(req)
			testutil.NoError(t, err)
			defer resp.Body.Close()
			testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
			testutil.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
			got, err := io.ReadAll(resp.Body)
			testutil.NoError(t, err)
			testutil.Equal(t, tc.wantBody, string(got))
		})
	}
}

func TestStorageBucketAPICreateUpdateDelete(t *testing.T) {
	ts, storageSvc, _ := setupServerWithAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	ctx := context.Background()

	name := fmt.Sprintf("api-bucket-%d", time.Now().UnixNano())
	payload := fmt.Sprintf(`{"name":"%s","public":false}`, name)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets", strings.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusCreated, resp.StatusCode)

	var bucket storage.Bucket
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&bucket))
	resp.Body.Close()
	testutil.Equal(t, name, bucket.Name)
	testutil.False(t, bucket.Public)

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/buckets", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var listed struct {
		Items []storage.Bucket `json:"items"`
	}
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	resp.Body.Close()
	testutil.True(t, len(listed.Items) >= 1, "expected at least one bucket")

	payload = `{"public":true}`
	req, err = http.NewRequest(http.MethodPut, ts.URL+"/api/storage/buckets/"+name, strings.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)

	var updated storage.Bucket
	testutil.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	resp.Body.Close()
	testutil.True(t, updated.Public)

	_, err = storageSvc.Upload(ctx, name, "bucket-object.txt", "text/plain", nil, strings.NewReader("abc"))
	testutil.NoError(t, err)

	req, err = http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/buckets/"+name, nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/buckets/"+name+"?force=true", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	_, err = storageSvc.GetBucket(ctx, name)
	testutil.ErrorContains(t, err, "bucket not found")
}

func TestStorageBucketACLAndCacheBehavior(t *testing.T) {
	ts, storageSvc, authSvc := setupServerWithAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userJWT := userToken(t, authSvc, "user-1", "user-1@example.com")
	ctx := context.Background()

	publicName := fmt.Sprintf("public-%d", time.Now().UnixNano())
	privateName := fmt.Sprintf("private-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(ctx, publicName, true)
	testutil.NoError(t, err)
	_, err = storageSvc.CreateBucket(ctx, privateName, false)
	testutil.NoError(t, err)

	_, err = storageSvc.Upload(ctx, publicName, "public.txt", "text/plain", nil, strings.NewReader("public-data"))
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctx, privateName, "private.txt", "text/plain", nil, strings.NewReader("private-data"))
	testutil.NoError(t, err)

	resp, err := http.Get(ts.URL + "/api/storage/" + publicName + "/public.txt")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/storage/" + privateName + "/private.txt")
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+privateName+"/private.txt", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+privateName+"/private.txt", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	resp, err = http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	resp.Body.Close()

	sign := storageSvc.SignURL(context.Background(), privateName, "private.txt", time.Hour)
	resp, err = http.Get(ts.URL + "/api/storage/" + privateName + "/private.txt?" + sign)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	resp.Body.Close()
}

func TestStorageBucketServiceLifecycle(t *testing.T) {
	_, storageSvc, _ := setupServerWithAuthAndStorageAdmin(t)
	clearStorageData(t)
	ctx := context.Background()

	publicName := fmt.Sprintf("lifecycle-%d", time.Now().UnixNano())
	privateName := fmt.Sprintf("lifecycle-private-%d", time.Now().UnixNano())

	pub, err := storageSvc.CreateBucket(ctx, publicName, true)
	testutil.NoError(t, err)
	testutil.Equal(t, publicName, pub.Name)
	testutil.True(t, pub.Public)

	_, err = storageSvc.CreateBucket(ctx, publicName, false)
	testutil.ErrorContains(t, err, "object already exists")

	pubBucket, err := storageSvc.GetBucket(ctx, publicName)
	testutil.NoError(t, err)
	testutil.Equal(t, publicName, pubBucket.Name)

	_, err = storageSvc.GetBucket(ctx, "missing-bucket")
	testutil.ErrorContains(t, err, "bucket not found")

	_, err = storageSvc.CreateBucket(ctx, privateName, false)
	testutil.NoError(t, err)

	buckets, err := storageSvc.ListBuckets(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(buckets) >= 2, "expected at least two buckets")

	private, err := storageSvc.UpdateBucket(ctx, privateName, true)
	testutil.NoError(t, err)
	testutil.True(t, private.Public)

	_, err = storageSvc.Upload(ctx, privateName, "file.txt", "text/plain", nil, strings.NewReader("hello"))
	testutil.NoError(t, err)

	err = storageSvc.DeleteBucket(ctx, privateName, false)
	testutil.ErrorContains(t, err, "bucket has objects")

	err = storageSvc.DeleteBucket(ctx, privateName, true)
	testutil.NoError(t, err)

	_, err = storageSvc.GetBucket(ctx, privateName)
	testutil.ErrorContains(t, err, "bucket not found")
}

func TestStorageTenantIsolationMetadata(t *testing.T) {
	_, storageSvc, _, tenantA := setupServerWithTenantAuthAndStorageAdmin(t)
	clearStorageData(t)

	ctx := context.Background()
	tenantSvc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	tenantB := createQuotaTestTenant(t, ctx, tenantSvc, "storage-isolation").ID
	ctxA := tenant.ContextWithTenantID(ctx, tenantA)
	ctxB := tenant.ContextWithTenantID(ctx, tenantB)

	userA := "abababab-1111-1111-1111-111111111111"
	userB := "abababab-2222-2222-2222-222222222222"
	ensureStorageTestUser(t, userA, "tenant-a-storage@example.com")
	ensureStorageTestUser(t, userB, "tenant-b-storage@example.com")

	bucketA, err := storageSvc.CreateBucket(ctxA, "sharedbucket", false)
	testutil.NoError(t, err)
	bucketB, err := storageSvc.CreateBucket(ctxB, "sharedbucket", true)
	testutil.NoError(t, err)
	testutil.Equal(t, "sharedbucket", bucketA.Name)
	testutil.Equal(t, "sharedbucket", bucketB.Name)
	testutil.False(t, bucketA.Public)
	testutil.True(t, bucketB.Public)

	gotBucketA, err := storageSvc.GetBucket(ctxA, "sharedbucket")
	testutil.NoError(t, err)
	gotBucketB, err := storageSvc.GetBucket(ctxB, "sharedbucket")
	testutil.NoError(t, err)
	testutil.False(t, gotBucketA.Public)
	testutil.True(t, gotBucketB.Public)

	bucketsA, err := storageSvc.ListBuckets(ctxA)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(bucketsA))
	testutil.Equal(t, "sharedbucket", bucketsA[0].Name)
	testutil.False(t, bucketsA[0].Public)
	bucketsB, err := storageSvc.ListBuckets(ctxB)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(bucketsB))
	testutil.Equal(t, "sharedbucket", bucketsB[0].Name)
	testutil.True(t, bucketsB[0].Public)

	const (
		directBodyA = "tenant-a-bytes"
		directBodyB = "tenant-b-bytes"
	)
	_, err = storageSvc.Upload(ctxA, "sharedbucket", "same.txt", "text/plain", &userA, strings.NewReader(directBodyA))
	testutil.NoError(t, err)
	_, err = storageSvc.Upload(ctxB, "sharedbucket", "same.txt", "text/plain", &userB, strings.NewReader(directBodyB))
	testutil.NoError(t, err)

	objA, err := storageSvc.GetObject(ctxA, "sharedbucket", "same.txt")
	testutil.NoError(t, err)
	objB, err := storageSvc.GetObject(ctxB, "sharedbucket", "same.txt")
	testutil.NoError(t, err)
	testutil.Equal(t, userA, *objA.UserID)
	testutil.Equal(t, userB, *objB.UserID)
	testutil.Equal(t, int64(len(directBodyA)), objA.Size)
	testutil.Equal(t, int64(len(directBodyB)), objB.Size)

	downloadedA := assertStorageDownloadBody(t, ctxA, storageSvc, "sharedbucket", "same.txt", directBodyA)
	downloadedB := assertStorageDownloadBody(t, ctxB, storageSvc, "sharedbucket", "same.txt", directBodyB)
	testutil.Equal(t, userA, *downloadedA.UserID)
	testutil.Equal(t, userB, *downloadedB.UserID)

	listA, totalA, err := storageSvc.ListObjects(ctxA, "sharedbucket", "", 100, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, totalA)
	testutil.Equal(t, 1, len(listA))
	testutil.Equal(t, userA, *listA[0].UserID)
	listB, totalB, err := storageSvc.ListObjects(ctxB, "sharedbucket", "", 100, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, totalB)
	testutil.Equal(t, 1, len(listB))
	testutil.Equal(t, userB, *listB[0].UserID)

	err = storageSvc.DeleteObject(ctxA, "sharedbucket", "same.txt")
	testutil.NoError(t, err)
	_, err = storageSvc.GetObject(ctxA, "sharedbucket", "same.txt")
	testutil.ErrorContains(t, err, "object not found")
	objB, err = storageSvc.GetObject(ctxB, "sharedbucket", "same.txt")
	testutil.NoError(t, err)
	testutil.Equal(t, userB, *objB.UserID)
	downloadedB = assertStorageDownloadBody(t, ctxB, storageSvc, "sharedbucket", "same.txt", directBodyB)
	testutil.Equal(t, userB, *downloadedB.UserID)

	err = storageSvc.DeleteBucket(ctxA, "sharedbucket", false)
	testutil.NoError(t, err)
	_, err = storageSvc.GetBucket(ctxA, "sharedbucket")
	testutil.ErrorContains(t, err, "bucket not found")
	gotBucketB, err = storageSvc.GetBucket(ctxB, "sharedbucket")
	testutil.NoError(t, err)
	testutil.True(t, gotBucketB.Public)
}

func TestStorageTenantIsolationResumableMetadata(t *testing.T) {
	_, storageSvc, _, tenantA := setupServerWithTenantAuthAndStorageAdmin(t)
	clearStorageData(t)

	ctx := context.Background()
	tenantSvc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	tenantB := createQuotaTestTenant(t, ctx, tenantSvc, "storage-resumable-isolation").ID
	ctxA := tenant.ContextWithTenantID(ctx, tenantA)
	ctxB := tenant.ContextWithTenantID(ctx, tenantB)

	userA := "cdcdcdcd-1111-1111-1111-111111111111"
	userB := "cdcdcdcd-2222-2222-2222-222222222222"
	ensureStorageTestUser(t, userA, "tenant-a-resumable@example.com")
	ensureStorageTestUser(t, userB, "tenant-b-resumable@example.com")

	_, err := storageSvc.CreateBucket(ctxA, "sharedbucket", false)
	testutil.NoError(t, err)
	_, err = storageSvc.CreateBucket(ctxB, "sharedbucket", true)
	testutil.NoError(t, err)

	const (
		bodyA = "tenant-a-resumable"
		bodyB = "tenant-b-resumable-content"
	)
	uploadA, err := storageSvc.CreateResumableUpload(ctxA, "sharedbucket", "same.txt", "text/plain", &userA, int64(len(bodyA)))
	testutil.NoError(t, err)
	uploadB, err := storageSvc.CreateResumableUpload(ctxB, "sharedbucket", "same.txt", "text/plain", &userB, int64(len(bodyB)))
	testutil.NoError(t, err)

	_, err = storageSvc.GetResumableUpload(ctxB, uploadA.ID, nil)
	testutil.ErrorContains(t, err, "resumable upload not found")

	_, shouldFinalize, err := storageSvc.AppendResumableUpload(ctxA, uploadA.ID, 0, &userA, strings.NewReader(bodyA))
	testutil.NoError(t, err)
	testutil.True(t, shouldFinalize)
	_, shouldFinalize, err = storageSvc.AppendResumableUpload(ctxB, uploadB.ID, 0, &userB, strings.NewReader(bodyB))
	testutil.NoError(t, err)
	testutil.True(t, shouldFinalize)

	objA, err := storageSvc.FinalizeResumableUpload(ctxA, uploadA.ID, &userA)
	testutil.NoError(t, err)
	objB, err := storageSvc.FinalizeResumableUpload(ctxB, uploadB.ID, &userB)
	testutil.NoError(t, err)
	testutil.Equal(t, userA, *objA.UserID)
	testutil.Equal(t, userB, *objB.UserID)
	testutil.Equal(t, int64(len(bodyA)), objA.Size)
	testutil.Equal(t, int64(len(bodyB)), objB.Size)

	gotA, err := storageSvc.GetObject(ctxA, "sharedbucket", "same.txt")
	testutil.NoError(t, err)
	gotB, err := storageSvc.GetObject(ctxB, "sharedbucket", "same.txt")
	testutil.NoError(t, err)
	testutil.Equal(t, userA, *gotA.UserID)
	testutil.Equal(t, userB, *gotB.UserID)

	downloadedA := assertStorageDownloadBody(t, ctxA, storageSvc, "sharedbucket", "same.txt", bodyA)
	downloadedB := assertStorageDownloadBody(t, ctxB, storageSvc, "sharedbucket", "same.txt", bodyB)
	testutil.Equal(t, userA, *downloadedA.UserID)
	testutil.Equal(t, userB, *downloadedB.UserID)

	listA, totalA, err := storageSvc.ListObjects(ctxA, "sharedbucket", "", 100, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, totalA)
	testutil.Equal(t, 1, len(listA))
	testutil.Equal(t, userA, *listA[0].UserID)
	listB, totalB, err := storageSvc.ListObjects(ctxB, "sharedbucket", "", 100, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, totalB)
	testutil.Equal(t, 1, len(listB))
	testutil.Equal(t, userB, *listB[0].UserID)
}

func TestStorageRLSUserIsolationAdminBypassAndPolicyUpdate(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userAID := "11111111-1111-1111-1111-111111111111"
	userBID := "22222222-2222-2222-2222-222222222222"
	ensureStorageTestUser(t, userAID, "a@example.com")
	ensureStorageTestUser(t, userBID, "b@example.com")
	addStorageTestMembership(t, tenantID, userAID)
	addStorageTestMembership(t, tenantID, userBID)
	userA := userToken(t, authSvc, userAID, "a@example.com")
	userB := userToken(t, authSvc, userBID, "b@example.com")

	bucket := fmt.Sprintf("rls-private-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	status := applyStorageTemplate(t, ts.URL, adminJWT, "user-own-files", `{"prefix":"storage_owner"}`)
	testutil.Equal(t, http.StatusCreated, status)

	testutil.Equal(t, http.StatusCreated, uploadStatus(t, ts.URL, bucket, "a.txt", "owner-a", requestHeaders{token: userA, tenantID: tenantID}))
	testutil.Equal(t, http.StatusCreated, uploadStatus(t, ts.URL, bucket, "b.txt", "owner-b", requestHeaders{token: userB, tenantID: tenantID}))

	reqA, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket, nil)
	testutil.NoError(t, err)
	reqA.Header.Set("Authorization", "Bearer "+userA)
	reqA.Header.Set("X-Tenant-ID", tenantID)
	respA, err := http.DefaultClient.Do(reqA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respA.StatusCode)
	var listA struct {
		Items []storage.Object `json:"items"`
	}
	testutil.NoError(t, json.NewDecoder(respA.Body).Decode(&listA))
	respA.Body.Close()
	testutil.Equal(t, 1, len(listA.Items))
	testutil.Equal(t, "a.txt", listA.Items[0].Name)

	reqUserBReadA, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	reqUserBReadA.Header.Set("Authorization", "Bearer "+userB)
	reqUserBReadA.Header.Set("X-Tenant-ID", tenantID)
	respUserBReadA, err := http.DefaultClient.Do(reqUserBReadA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNotFound, respUserBReadA.StatusCode)
	respUserBReadA.Body.Close()

	// Unauthenticated serve on private bucket must return 401.
	reqUnauth, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	respUnauth, err := http.DefaultClient.Do(reqUnauth)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusUnauthorized, respUnauth.StatusCode)
	respUnauth.Body.Close()

	reqAdminReadA, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	reqAdminReadA.Header.Set("Authorization", "Bearer "+adminJWT)
	reqAdminReadA.Header.Set("X-Tenant-ID", tenantID)
	respAdminReadA, err := http.DefaultClient.Do(reqAdminReadA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respAdminReadA.StatusCode)
	respAdminReadA.Body.Close()

	// Owner sign success: userA signs a.txt, gets a signed URL, roundtrip serves the file.
	signBodyA := bytes.NewReader([]byte(`{"expiresIn":3600}`))
	reqSignA, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/"+bucket+"/a.txt/sign", signBodyA)
	testutil.NoError(t, err)
	reqSignA.Header.Set("Authorization", "Bearer "+userA)
	reqSignA.Header.Set("X-Tenant-ID", tenantID)
	reqSignA.Header.Set("Content-Type", "application/json")
	respSignA, err := http.DefaultClient.Do(reqSignA)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respSignA.StatusCode)
	var signRespA map[string]string
	testutil.NoError(t, json.NewDecoder(respSignA.Body).Decode(&signRespA))
	respSignA.Body.Close()
	signedURL := signRespA["url"]
	testutil.True(t, signedURL != "", "expected signed URL in response")

	// Fetch via signed URL without auth — withRLS returns raw pool for nil claims,
	// so the signed URL bypasses RLS and serves the file.
	reqSignedGet, err := http.NewRequest(http.MethodGet, ts.URL+signedURL, nil)
	testutil.NoError(t, err)
	reqSignedGet.Header.Set("X-Tenant-ID", tenantID)
	respSignedGet, err := http.DefaultClient.Do(reqSignedGet)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respSignedGet.StatusCode)
	signedBody, err := io.ReadAll(respSignedGet.Body)
	testutil.NoError(t, err)
	respSignedGet.Body.Close()
	testutil.Equal(t, "owner-a", string(signedBody))

	// Cross-user sign 404: userB cannot sign a.txt owned by userA under user-own-files RLS.
	signBodyB := bytes.NewReader([]byte(`{"expiresIn":3600}`))
	reqSignB, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/"+bucket+"/a.txt/sign", signBodyB)
	testutil.NoError(t, err)
	reqSignB.Header.Set("Authorization", "Bearer "+userB)
	reqSignB.Header.Set("X-Tenant-ID", tenantID)
	reqSignB.Header.Set("Content-Type", "application/json")
	respSignB, err := http.DefaultClient.Do(reqSignB)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNotFound, respSignB.StatusCode)
	respSignB.Body.Close()

	status = applyStorageTemplate(t, ts.URL, adminJWT, "public-read-auth-write", `{"prefix":"storage_public_auth"}`)
	testutil.Equal(t, http.StatusCreated, status)

	reqUserBReadA2, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/a.txt", nil)
	testutil.NoError(t, err)
	reqUserBReadA2.Header.Set("Authorization", "Bearer "+userB)
	reqUserBReadA2.Header.Set("X-Tenant-ID", tenantID)
	respUserBReadA2, err := http.DefaultClient.Do(reqUserBReadA2)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, respUserBReadA2.StatusCode)
	respUserBReadA2.Body.Close()
}

func TestStorageResumableUploadCreateResumeComplete(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)
	_, err := sharedPG.Pool.Exec(context.Background(), "TRUNCATE _ayb_storage_usage")
	testutil.NoError(t, err)

	adminJWT := adminToken(t, ts.URL)
	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	ensureStorageTestUser(t, userID, "resumable-user@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user@example.com")
	_, _ = adminJWT, userJWT
	_ = storageSvc

	bucket := fmt.Sprintf("resumable-%d", time.Now().UnixNano())
	ctx := context.Background()
	_, err = storageSvc.CreateBucket(ctx, bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "hello.txt", 12, requestHeaders{token: userJWT, tenantID: tenantID})

	var bytesUsed int64
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT bytes_used FROM _ayb_storage_usage WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	).Scan(&bytesUsed)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(12), bytesUsed)

	resp := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("hello"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	testutil.Equal(t, "5", resp.Header.Get("Upload-Offset"))
	testutil.Equal(t, "12", resp.Header.Get("Upload-Length"))
	resp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 5, []byte(" world!"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	testutil.Equal(t, "12", resp.Header.Get("Upload-Offset"))
	testutil.Equal(t, "12", resp.Header.Get("Upload-Length"))
	resp.Body.Close()

	getReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/hello.txt", nil)
	testutil.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+userJWT)
	getReq.Header.Set("X-Tenant-ID", tenantID)
	getResp, err := http.DefaultClient.Do(getReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, getResp.StatusCode)
	body, err := io.ReadAll(getResp.Body)
	testutil.NoError(t, err)
	getResp.Body.Close()
	testutil.Equal(t, "hello world!", string(body))

	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT bytes_used FROM _ayb_storage_usage WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	).Scan(&bytesUsed)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(12), bytesUsed)
}

func TestStorageResumableUploadOffsetMismatch(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	ensureStorageTestUser(t, userID, "resumable-user2@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user2@example.com")
	bucket := fmt.Sprintf("resumable-offset-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "bad-offset.txt", 6, requestHeaders{token: userJWT, tenantID: tenantID})

	resp := patchResumableChunkWithHeaders(t, ts.URL, id, 2, []byte("abc"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	headResp := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusOK, headResp.StatusCode)
	testutil.Equal(t, int64(0), parseOffsetHeader(t, headResp.Header.Get("Upload-Offset")))
	headResp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("abc"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 3, []byte("def"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()
}

func TestStorageResumableUploadInterruptedResume(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	ensureStorageTestUser(t, userID, "resumable-user3@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user3@example.com")
	bucket := fmt.Sprintf("resumable-reconnect-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "resume.txt", 11, requestHeaders{token: userJWT, tenantID: tenantID})

	resp := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("part"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	head := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusOK, head.StatusCode)
	testutil.Equal(t, int64(4), parseOffsetHeader(t, head.Header.Get("Upload-Offset")))
	head.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 1, []byte("wrong"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	resp = patchResumableChunkWithHeaders(t, ts.URL, id, 4, []byte("resume!"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	getReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/resume.txt", nil)
	testutil.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+userJWT)
	getReq.Header.Set("X-Tenant-ID", tenantID)
	getResp, err := http.DefaultClient.Do(getReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, getResp.StatusCode)
	got, err := io.ReadAll(getResp.Body)
	testutil.NoError(t, err)
	getResp.Body.Close()
	testutil.Equal(t, "partresume!", string(got))
}

func TestStorageResumableUploadOversizedChunkRejected(t *testing.T) {
	ts, storageSvc, _, tenantID := setupServerWithTenantAuthAndStorageAdminAndMaxFile(t, "1KB")
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	bucket := fmt.Sprintf("resumable-small-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	reqOpts, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/storage/upload/resumable", nil)
	testutil.NoError(t, err)
	reqOpts.Header.Set("Authorization", "Bearer "+adminJWT)
	reqOpts.Header.Set("X-Tenant-ID", tenantID)
	reqOpts.Header.Set("Tus-Resumable", "1.0.0")
	optionsResp, err := http.DefaultClient.Do(reqOpts)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusNoContent, optionsResp.StatusCode)
	testutil.Equal(t, "1.0.0", optionsResp.Header.Get("Tus-Resumable"))
	testutil.Equal(t, "creation", optionsResp.Header.Get("Tus-Extension"))
	testutil.Equal(t, "1024", optionsResp.Header.Get("Tus-Max-Size"))
	optionsResp.Body.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/upload/resumable?bucket="+bucket+"&name=big.txt", nil)
	testutil.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	req.Header.Set("X-Tenant-ID", tenantID)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.Itoa(2048))
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	resp.Body.Close()
}

func TestStorageResumableUploadExpiration(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)
	_, err := sharedPG.Pool.Exec(context.Background(), "TRUNCATE _ayb_storage_usage")
	testutil.NoError(t, err)

	userID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	ensureStorageTestUser(t, userID, "resumable-user4@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user4@example.com")
	bucket := fmt.Sprintf("resumable-expire-%d", time.Now().UnixNano())
	_, err = storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	const (
		expiredSize = int64(4)
		activeSize  = int64(6)
	)
	_, expiredID := createResumableSessionWithHeaders(t, ts.URL, bucket, "expire.txt", expiredSize, requestHeaders{token: userJWT, tenantID: tenantID})
	_, activeID := createResumableSessionWithHeaders(t, ts.URL, bucket, "keep.txt", activeSize, requestHeaders{token: userJWT, tenantID: tenantID})

	// Stage real backend bytes for each session so cleanup exercises staging-blob
	// removal for the expired upload while the active upload's staged bytes must
	// survive. Partial chunks keep both sessions in the active (resumable) state.
	expiredPartial := patchResumableChunkWithHeaders(t, ts.URL, expiredID, 0, []byte("ab"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, expiredPartial.StatusCode)
	expiredPartial.Body.Close()
	activePartial := patchResumableChunkWithHeaders(t, ts.URL, activeID, 0, []byte("abc"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, activePartial.StatusCode)
	activePartial.Body.Close()

	var reservedBytes int64
	err = sharedPG.Pool.QueryRow(context.Background(),
		`SELECT bytes_used FROM _ayb_storage_usage WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	).Scan(&reservedBytes)
	testutil.NoError(t, err)
	testutil.Equal(t, expiredSize+activeSize, reservedBytes)

	_, err = sharedPG.Pool.Exec(context.Background(),
		`UPDATE _ayb_storage_uploads SET expires_at = NOW() - interval '1 hour' WHERE id = $1`, expiredID)
	testutil.NoError(t, err)
	_, err = sharedPG.Pool.Exec(context.Background(),
		`UPDATE _ayb_storage_uploads SET expires_at = NOW() + interval '1 day' WHERE id = $1`, activeID)
	testutil.NoError(t, err)

	head := headResumableSessionWithHeaders(t, ts.URL, expiredID, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusGone, head.StatusCode)
	head.Body.Close()

	deleted, err := storageSvc.CleanupExpiredResumableUploads(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, deleted)

	var remainingUploads int
	err = sharedPG.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM _ayb_storage_uploads`).Scan(&remainingUploads)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, remainingUploads)

	var remainingActive int
	err = sharedPG.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM _ayb_storage_uploads WHERE id = $1`, activeID).Scan(&remainingActive)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, remainingActive)

	var remainingExpired int
	err = sharedPG.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM _ayb_storage_uploads WHERE id = $1`, expiredID).Scan(&remainingExpired)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, remainingExpired)

	var bytesUsed int64
	err = sharedPG.Pool.QueryRow(context.Background(),
		`SELECT bytes_used FROM _ayb_storage_usage WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	).Scan(&bytesUsed)
	testutil.NoError(t, err)
	testutil.Equal(t, activeSize, bytesUsed)

	activeHead := headResumableSessionWithHeaders(t, ts.URL, activeID, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusOK, activeHead.StatusCode)
	activeHead.Body.Close()

	// The active upload's staged bytes must have survived cleanup: resuming with
	// the remaining bytes finalizes to the exact content.
	activeResume := patchResumableChunkWithHeaders(t, ts.URL, activeID, 3, []byte("def"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, activeResume.StatusCode)
	testutil.Equal(t, "6", activeResume.Header.Get("Upload-Offset"))
	activeResume.Body.Close()

	getReq, err := http.NewRequest(http.MethodGet, ts.URL+"/api/storage/"+bucket+"/keep.txt", nil)
	testutil.NoError(t, err)
	getReq.Header.Set("Authorization", "Bearer "+userJWT)
	getReq.Header.Set("X-Tenant-ID", tenantID)
	getResp, err := http.DefaultClient.Do(getReq)
	testutil.NoError(t, err)
	testutil.StatusCode(t, http.StatusOK, getResp.StatusCode)
	body, err := io.ReadAll(getResp.Body)
	testutil.NoError(t, err)
	getResp.Body.Close()
	testutil.Equal(t, "abcdef", string(body))
}

func TestStorageResumableUploadExpirationOwnerlessNoQuotaMutation(t *testing.T) {
	ts, storageSvc, _, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)
	_, err := sharedPG.Pool.Exec(context.Background(), "TRUNCATE _ayb_storage_usage")
	testutil.NoError(t, err)

	userID := "abababab-abab-abab-abab-abababababab"
	ensureStorageTestUser(t, userID, "resumable-ownerless@example.com")
	_, err = sharedPG.Pool.Exec(context.Background(),
		`INSERT INTO _ayb_storage_usage (tenant_id, user_id, bytes_used, updated_at)
		 VALUES ($1, $2, $3, NOW())`,
		tenantID, userID, int64(99))
	testutil.NoError(t, err)

	adminJWT := adminToken(t, ts.URL)
	bucket := fmt.Sprintf("resumable-ownerless-expire-%d", time.Now().UnixNano())
	_, err = storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	_, uploadID := createResumableSessionWithHeaders(t, ts.URL, bucket, "ownerless.txt", 7, requestHeaders{token: adminJWT, tenantID: tenantID})

	var uploadUserID *string
	err = sharedPG.Pool.QueryRow(context.Background(), `SELECT user_id FROM _ayb_storage_uploads WHERE id = $1`, uploadID).Scan(&uploadUserID)
	testutil.NoError(t, err)
	testutil.True(t, uploadUserID == nil, "expected ownerless upload")

	// Stage real backend bytes (admin owns the ownerless session) so cleanup
	// exercises staging-blob removal, not just the DB row delete.
	partial := patchResumableChunkWithHeaders(t, ts.URL, uploadID, 0, []byte("abc"), requestHeaders{token: adminJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, partial.StatusCode)
	partial.Body.Close()

	_, err = sharedPG.Pool.Exec(context.Background(),
		`UPDATE _ayb_storage_uploads SET expires_at = NOW() - interval '1 hour' WHERE id = $1`, uploadID)
	testutil.NoError(t, err)

	deleted, err := storageSvc.CleanupExpiredResumableUploads(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, deleted)

	var remainingUpload int
	err = sharedPG.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM _ayb_storage_uploads WHERE id = $1`, uploadID).Scan(&remainingUpload)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, remainingUpload)

	var bytesUsed int64
	err = sharedPG.Pool.QueryRow(context.Background(),
		`SELECT bytes_used FROM _ayb_storage_usage WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID,
	).Scan(&bytesUsed)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(99), bytesUsed)

	// The session (and its staged blob) is fully removed: resuming it is a 404.
	head := headResumableSessionWithHeaders(t, ts.URL, uploadID, requestHeaders{token: adminJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNotFound, head.StatusCode)
	head.Body.Close()
}

func TestStorageResumableUploadConcurrentIDs(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	userID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	ensureStorageTestUser(t, userID, "resumable-user5@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user5@example.com")
	bucket := fmt.Sprintf("resumable-concurrent-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	ids := map[string]struct{}{}
	for i := 0; i < 8; i++ {
		_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, fmt.Sprintf("c%d.txt", i), 1, requestHeaders{token: userJWT, tenantID: tenantID})
		_, exists := ids[id]
		testutil.False(t, exists)
		ids[id] = struct{}{}
	}
	testutil.Equal(t, 8, len(ids))
}

func TestStorageResumableUploadOwnerlessSessionAdminOnly(t *testing.T) {
	ts, storageSvc, authSvc, tenantID := setupServerWithTenantAuthAndStorageAdmin(t)
	defer ts.Close()
	clearStorageData(t)

	adminJWT := adminToken(t, ts.URL)
	userID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	ensureStorageTestUser(t, userID, "resumable-user6@example.com")
	addStorageTestMembership(t, tenantID, userID)
	userJWT := userToken(t, authSvc, userID, "resumable-user6@example.com")
	bucket := fmt.Sprintf("resumable-admin-%d", time.Now().UnixNano())
	_, err := storageSvc.CreateBucket(context.Background(), bucket, false)
	testutil.NoError(t, err)

	// Admin-created sessions have no user_id, so they must remain admin-only.
	_, id := createResumableSessionWithHeaders(t, ts.URL, bucket, "admin-owned.txt", 5, requestHeaders{token: adminJWT, tenantID: tenantID})

	headAsUser := headResumableSessionWithHeaders(t, ts.URL, id, requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusForbidden, headAsUser.StatusCode)
	headAsUser.Body.Close()

	patchAsUser := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("hello"), requestHeaders{token: userJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusForbidden, patchAsUser.StatusCode)
	patchAsUser.Body.Close()

	// Admin can still continue the upload.
	patchAsAdmin := patchResumableChunkWithHeaders(t, ts.URL, id, 0, []byte("hello"), requestHeaders{token: adminJWT, tenantID: tenantID})
	testutil.StatusCode(t, http.StatusNoContent, patchAsAdmin.StatusCode)
	patchAsAdmin.Body.Close()
}
