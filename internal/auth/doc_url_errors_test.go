package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

const docURLAuth = "https://allyourbase.io/guide/authentication"

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
// chi.URLParam resolve them without mounting a full router. Routes cannot match
// an empty path segment, so the empty-param branches require this.
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

func boolPtr(b bool) *bool { return &b }

// --- middleware.go (3 mapped branches) ---

func TestDocURLRequireAuthMFAPendingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token, err := svc.generateMFAPendingToken(&User{ID: "u1", Email: "u@example.com"})
	testutil.NoError(t, err)

	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("next handler must not run for an MFA-pending token")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assertDocURLError(t, w, http.StatusUnauthorized, "MFA verification required", docURLAuth)
}

func TestDocURLRequireMFAPendingNoBearerToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	handler := RequireMFAPending(svc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("next handler must not run without a bearer token")
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assertDocURLError(t, w, http.StatusUnauthorized, "no MFA challenge pending", docURLAuth)
}

func TestDocURLRequireMFAPendingNonPendingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token := generateTestToken(t, svc, "u1", "u@example.com") // not MFA-pending

	handler := RequireMFAPending(svc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("next handler must not run for a non-MFA-pending token")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assertDocURLError(t, w, http.StatusUnauthorized, "no MFA challenge pending", docURLAuth)
}

// --- handler.go (3 mapped branches) ---

func TestDocURLRefreshMissingToken(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	w := httptest.NewRecorder()
	h.handleRefresh(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`))))
	assertDocURLError(t, w, http.StatusBadRequest, "refreshToken is required", docURLAuth)
}

func TestDocURLDeleteSessionMissingID(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	req := chiRequest(http.MethodDelete, "/", nil, map[string]string{"id": ""})
	req = req.WithContext(ContextWithClaims(req.Context(), &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleDeleteSession(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "session id is required", docURLAuth)
}

func TestDocURLDeleteSessionInvalidUUID(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	req := chiRequest(http.MethodDelete, "/", nil, map[string]string{"id": "not-a-uuid"})
	req = req.WithContext(ContextWithClaims(req.Context(), &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleDeleteSession(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "invalid session id format", docURLAuth)
}

// --- handler_config.go (1 mapped branch) ---

func TestDocURLEnrollRequiresAAL2WhenMFAExists(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	h.existingMFAOverride = boolPtr(true)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	blocked := h.enforceAAL2ForExistingMFA(w, req, &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}})

	testutil.True(t, blocked, "existing MFA without AAL2 must be blocked")
	assertDocURLError(t, w, http.StatusForbidden, "AAL2 session required to enroll additional MFA factors", docURLAuth)
}

// --- sms_mfa_handlers.go (3 unit-reachable mapped branches) ---

func newDocURLSMSHandler(t *testing.T) *Handler {
	t.Helper()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	h.smsEnabled = true
	h.existingMFAOverride = boolPtr(false)
	return h
}

func TestDocURLSMSEnrollMissingPhone(t *testing.T) {
	t.Parallel()
	h := newDocURLSMSHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	req = req.WithContext(ContextWithClaims(req.Context(), &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleMFAEnroll(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "phone is required", docURLAuth)
}

func TestDocURLSMSEnrollInvalidPhone(t *testing.T) {
	t.Parallel()
	// EnrollSMSMFA normalizes the phone before touching the pool, so the
	// invalid-format branch is reachable with a nil pool.
	h := newDocURLSMSHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"phone":"not-a-phone"}`)))
	req = req.WithContext(ContextWithClaims(req.Context(), &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleMFAEnroll(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "invalid phone number format", docURLAuth)
}

func TestDocURLSMSVerifyLockedOut(t *testing.T) {
	t.Parallel()
	h := newDocURLSMSHandler(t)
	// Trip the in-memory lockout tracker rather than relying on a DB.
	for i := 0; i < 20; i++ {
		h.auth.RecordMFAFailure("u1")
	}
	testutil.True(t, h.auth.IsMFALocked("u1"), "user must be locked out for this branch")

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"code":"123456"}`)))
	req = req.WithContext(context.WithValue(req.Context(), mfaPendingCtxKey{}, &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleMFAVerify(w, req)
	assertDocURLError(t, w, http.StatusTooManyRequests, "too many failed attempts, try again later", docURLAuth)
}

// --- totp_mfa_handlers.go (1 unit-reachable mapped branch) ---

func TestDocURLTOTPEnrollConfirmMissingCode(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	req = req.WithContext(ContextWithClaims(req.Context(), &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleTOTPEnrollConfirm(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "code is required", docURLAuth)
}

// --- webauthn_credential_management_handlers.go (2 mapped branches) ---

func TestDocURLWebAuthnRenameMissingDisplayName(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	req := chiRequest(http.MethodPatch, "/", []byte(`{"display_name":"   "}`),
		map[string]string{"credential_id": "Y3JlZA"})
	req = req.WithContext(ContextWithClaims(req.Context(), &Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"}}))
	w := httptest.NewRecorder()
	h.handleWebAuthnCredentialRename(w, req)
	assertDocURLError(t, w, http.StatusBadRequest, "display_name is required", docURLAuth)
}

func TestDocURLWebAuthnDeleteLastCredential(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	w := httptest.NewRecorder()
	// The error mapper is the owner of this branch; drive it with the sentinel
	// directly so the assertion does not need a database-backed credential set.
	h.writeWebAuthnCredentialManagementError(w, ErrWebAuthnLastCredential, "delete credential")
	assertDocURLError(t, w, http.StatusForbidden, "cannot delete final WebAuthn credential", docURLAuth)
}
