package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

const docURLFileStorage = "https://allyourbase.io/guide/file-storage"

// assertDocURLError drives one handler error branch and asserts the exact HTTP
// status and the exact doc_url value the operator is pointed at. Both are
// asserted because a doc_url on the wrong status is as broken as a missing one.
func assertDocURLError(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantMessage, wantDocURL string) {
	t.Helper()
	testutil.Equal(t, wantStatus, w.Code)

	var resp httputil.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding error envelope: %v", err)
	}
	testutil.Equal(t, wantStatus, resp.Code)
	// The message pins which branch ran: neighbouring branches can share a
	// status and page, so status+doc_url alone can pass on the wrong one.
	testutil.Equal(t, wantMessage, resp.Message)
	testutil.Equal(t, wantDocURL, resp.DocURL)
}

func docURLChiRequest(method, target string, body []byte, params map[string]string) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	}
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// --- bucket_handler.go (3 mapped branches) ---

func TestDocURLBucketCreateMissingName(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "", false)
	w := httptest.NewRecorder()
	h.HandleBucketCreate(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`))))
	assertDocURLError(t, w, http.StatusBadRequest, "name is required", docURLFileStorage)
}

func TestDocURLBucketDeleteInvalidForceValue(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "", false)
	req := docURLChiRequest(http.MethodDelete, "/?force=notabool", nil, map[string]string{"name": "images"})
	w := httptest.NewRecorder()
	h.HandleBucketDelete(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "invalid force value", docURLFileStorage)
}

func TestDocURLBucketDeleteNonEmptyConflict(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "", false)
	w := httptest.NewRecorder()
	// writeBucketError owns this branch; drive it with the sentinel directly so
	// the assertion does not need a database-backed non-empty bucket.
	h.writeBucketError(w, ErrBucketNotEmpty)
	assertDocURLError(t, w, http.StatusConflict, "bucket has objects; use force=true to delete", docURLFileStorage)
}

// --- handler_upload.go (2 mapped branches) ---

func TestDocURLUploadTenantQuotaExceeded(t *testing.T) {
	t.Parallel()
	h := newTestUploadHandler(&countBackend{})
	h.SetTenantQuota(&tenantQuotaReaderMock{
		quotas: &tenant.TenantQuotas{DBSizeBytesHard: ptrInt64(10)},
	}, tenant.DefaultQuotaChecker{}, tenant.NewUsageAccumulator(nil, nil))

	req := newMultipartUploadRequest(t, bytes.Repeat([]byte("x"), 20))
	req = req.WithContext(tenant.ContextWithTenantID(req.Context(), "tenant-1"))
	req = withUserClaims(req, "cbf722d5-d03e-43ac-becf-f4dca1764f36")
	rec := performUpload(t, h, req)

	assertDocURLError(t, rec, http.StatusRequestEntityTooLarge, "tenant storage quota exceeded", docURLFileStorage)
}

func TestDocURLUploadUserQuotaExceeded(t *testing.T) {
	t.Parallel()
	h := newTestUploadHandler(&countBackend{})
	h.mutations.reserveQuota = func(context.Context, string, int64) error { return ErrQuotaExceeded }

	req := newMultipartUploadRequest(t, []byte("small"))
	req = withUserClaims(req, "cbf722d5-d03e-43ac-becf-f4dca1764f36")
	rec := performUpload(t, h, req)

	assertDocURLError(t, rec, http.StatusRequestEntityTooLarge, "storage quota exceeded", docURLFileStorage)
}

// --- handler_resumable.go (4 mapped branches) ---

func newResumableRequest(method string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(method, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestDocURLResumableCreateMissingBucketOrName(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "", false)
	req := newResumableRequest(http.MethodPost, map[string]string{
		tusResumableHeader:    tusResumableVersion,
		tusUploadLengthHeader: "10",
	})
	w := httptest.NewRecorder()
	h.HandleResumableCreate(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "bucket and name query params are required", docURLFileStorage)
}

func TestDocURLResumableCreateExceedsMaxFileSize(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 1024, "", false)
	req := httptest.NewRequest(http.MethodPost, "/?bucket=images&name=big.bin", nil)
	req.Header.Set(tusResumableHeader, tusResumableVersion)
	req.Header.Set(tusUploadLengthHeader, "99999")
	w := httptest.NewRecorder()
	h.HandleResumableCreate(w, req)
	assertDocURLError(t, w, http.StatusRequestEntityTooLarge, "upload exceeds maximum file size", docURLFileStorage)
}

func TestDocURLResumablePatchWrongContentType(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger(), 10<<20, "", false)
	req := newResumableRequest(http.MethodPatch, map[string]string{
		tusResumableHeader: tusResumableVersion,
		"Content-Type":     "application/json",
	})
	w := httptest.NewRecorder()
	h.HandleResumablePatch(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "Content-Type must be application/offset+octet-stream", docURLFileStorage)
}

func TestDocURLResumableMissingTusVersion(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	testutil.False(t, requireTusVersion(w, httptest.NewRequest(http.MethodPost, "/", nil)),
		"missing Tus-Resumable header must be rejected")
	assertDocURLError(t, w, http.StatusPreconditionFailed, "invalid Tus-Resumable header", docURLFileStorage)
}
