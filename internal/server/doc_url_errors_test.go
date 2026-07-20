package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
)

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

// chiRequest builds a request carrying chi URL params so handlers that read
// chi.URLParam resolve them without mounting a full router.
func chiRequest(method, target string, body []byte, params map[string]string) *http.Request {
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

const (
	docURLPush      = "https://allyourbase.io/guide/push-notifications"
	docURLJobQueue  = "https://allyourbase.io/guide/job-queue"
	docURLBackups   = "https://allyourbase.io/guide/backups"
	docURLDatabase  = "https://allyourbase.io/guide/database-rpc"
	docURLSecurity  = "https://allyourbase.io/guide/security"
	docURLEmail     = "https://allyourbase.io/guide/email"
	docURLEdgeFuncs = "https://allyourbase.io/guide/edge-functions"
	docURLSAML      = "https://allyourbase.io/guide/saml"
)

// --- push_handler.go (4 mapped branches) ---

func TestDocURLPushRegisterUnauthenticated(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	handleUserPushRegister(newFakePushAdmin())(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`))))
	assertDocURLError(t, w, http.StatusUnauthorized, "authentication required", docURLPush)
}

func TestDocURLPushRegisterMissingAppID(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"token":"t"}`)))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{}))
	w := httptest.NewRecorder()
	handleUserPushRegister(newFakePushAdmin())(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "app_id is required", docURLPush)
}

func TestDocURLPushRegisterMissingToken(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"app_id":"a"}`)))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{}))
	w := httptest.NewRecorder()
	handleUserPushRegister(newFakePushAdmin())(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "token is required", docURLPush)
}

func TestDocURLPushAdminSendMissingTitle(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	body := []byte(`{"app_id":"a","user_id":"u"}`)
	handleAdminPushSend(newFakePushAdmin())(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	assertDocURLError(t, w, http.StatusBadRequest, "title is required", docURLPush)
}

// --- jobs_handler_schedules.go (4 mapped branches) ---

func TestDocURLScheduleInvalidCronExpr(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	testutil.False(t, validateScheduleCronExpr(w, "not-a-cron"), "invalid cron must be rejected")
	assertDocURLError(t, w, http.StatusBadRequest, "invalid cron expression", docURLJobQueue)
}

func TestDocURLScheduleInvalidTimezone(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	testutil.False(t, validateScheduleTimezone(w, "Not/AZone"), "invalid timezone must be rejected")
	assertDocURLError(t, w, http.StatusBadRequest, "invalid timezone", docURLJobQueue)
}

func TestDocURLCreateScheduleMissingName(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	body := []byte(`{"jobType":"j","cronExpr":"* * * * *"}`)
	handleAdminCreateSchedule(nil)(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	assertDocURLError(t, w, http.StatusBadRequest, "name is required", docURLJobQueue)
}

func TestDocURLCreateScheduleMissingCronExpr(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	body := []byte(`{"name":"n","jobType":"j"}`)
	handleAdminCreateSchedule(nil)(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	assertDocURLError(t, w, http.StatusBadRequest, "cronExpr is required", docURLJobQueue)
}

// --- backup_admin_handler.go / pitr_admin_handler.go (5 mapped branches) ---

func TestDocURLBackupTriggerServiceUnconfigured(t *testing.T) {
	t.Parallel()
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleAdminBackupTrigger(w, httptest.NewRequest(http.MethodPost, "/", nil))
	assertDocURLError(t, w, http.StatusServiceUnavailable, "backup service not configured", docURLBackups)
}

func TestDocURLPITRValidateServiceUnconfigured(t *testing.T) {
	t.Parallel()
	s := &Server{}
	w := httptest.NewRecorder()
	s.handlePITRValidate(w, httptest.NewRequest(http.MethodPost, "/", nil))
	assertDocURLError(t, w, http.StatusServiceUnavailable, "PITR not configured", docURLBackups)
}

func TestDocURLPITRValidateMissingTargetTime(t *testing.T) {
	t.Parallel()
	s := &Server{pitrService: &fakePITRService{}}
	w := httptest.NewRecorder()
	req := chiRequest(http.MethodPost, "/", []byte(`{"database_id":"db"}`), map[string]string{"projectId": "p"})
	s.handlePITRValidate(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "target_time is required", docURLBackups)
}

func TestDocURLPITRValidateMissingDatabaseID(t *testing.T) {
	t.Parallel()
	s := &Server{pitrService: &fakePITRService{}}
	w := httptest.NewRecorder()
	req := chiRequest(http.MethodPost, "/", []byte(`{"target_time":"2026-01-01T00:00:00Z"}`), map[string]string{"projectId": "p"})
	s.handlePITRValidate(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "database_id is required", docURLBackups)
}

func TestDocURLPITRRestoreShadowModeConflict(t *testing.T) {
	t.Parallel()
	s := &Server{pitrService: &fakePITRService{restoreErr: errors.New("restore blocked: shadow mode active")}}
	w := httptest.NewRecorder()
	req := chiRequest(http.MethodPost, "/", []byte(`{"target_time":"2026-01-01T00:00:00Z","database_id":"db"}`), map[string]string{"projectId": "p"})
	s.handlePITRRestore(w, req)
	assertDocURLError(t, w, http.StatusConflict, "restore blocked: shadow mode active", docURLBackups)
}

// --- sql_handler.go (2 mapped branches) ---

func TestDocURLAdminSQLEmptyQuery(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	handleAdminSQL(nil, nil)(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"query":"   "}`))))
	assertDocURLError(t, w, http.StatusBadRequest, "query is required", docURLDatabase)
}

func TestDocURLAdminSQLNoDatabase(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	handleAdminSQL(nil, nil)(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"query":"SELECT 1"}`))))
	assertDocURLError(t, w, http.StatusServiceUnavailable, "database not available", docURLDatabase)
}

// --- secrets_handler.go (1 mapped branch) ---

func TestDocURLGetSecretInvalidName(t *testing.T) {
	t.Parallel()
	s := &Server{vaultStore: newFakeVaultSecretStore()}
	w := httptest.NewRecorder()
	req := chiRequest(http.MethodGet, "/", nil, map[string]string{"name": "../etc/passwd"})
	s.handleGetSecret(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "invalid secret name", docURLSecurity)
}

// --- email_send_handler.go (1 mapped branch) ---

func TestDocURLPublicEmailSendNotConfigured(t *testing.T) {
	t.Parallel()
	s := &Server{}
	w := httptest.NewRecorder()
	s.handlePublicEmailSend(w, httptest.NewRequest(http.MethodPost, "/", nil))
	assertDocURLError(t, w, http.StatusNotImplemented, "email sending is not configured", docURLEmail)
}

// --- edgefunc_handler.go (1 mapped branch) ---

func TestDocURLEdgeFuncInvokeMissingName(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	req := chiRequest(http.MethodPost, "/", nil, map[string]string{"name": ""})
	handleEdgeFuncInvoke(nil, 0, nil, nil)(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "function name is required", docURLEdgeFuncs)
}

// --- saml_handler.go (1 mapped branch) ---

func TestDocURLSAMLAdminListNotEnabled(t *testing.T) {
	t.Parallel()
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleAdminSAMLList(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assertDocURLError(t, w, http.StatusNotFound, "auth SAML is not enabled", docURLSAML)
}
