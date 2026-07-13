package crossnode

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// RawRequest describes a single HTTP request against a base URL endpoint.
type RawRequest struct {
	Method  string
	URL     string
	Body    any
	Token   string
	Headers map[string]string
}

// Response is the outcome of an HTTP request. Header is retained so callers can
// assert on load-balancer attribution headers such as X-AYB-Upstream.
type Response struct {
	Status int
	Body   string
	Header http.Header
}

// Do executes an HTTP request, JSON-encoding a non-nil Body, and returns the
// full response including headers.
func Do(t *testing.T, rawReq RawRequest) Response {
	t.Helper()
	var reqBody io.Reader
	if rawReq.Body != nil {
		payload, err := json.Marshal(rawReq.Body)
		if err != nil {
			t.Fatalf("marshal %s %s body: %v", rawReq.Method, rawReq.URL, err)
		}
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(rawReq.Method, rawReq.URL, reqBody)
	if err != nil {
		t.Fatalf("build %s %s request: %v", rawReq.Method, rawReq.URL, err)
	}
	if rawReq.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if rawReq.Token != "" {
		req.Header.Set("Authorization", "Bearer "+rawReq.Token)
	}
	for name, value := range rawReq.Headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", rawReq.Method, rawReq.URL, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s response: %v", rawReq.Method, rawReq.URL, err)
	}
	return Response{Status: resp.StatusCode, Body: string(raw), Header: resp.Header}
}

// Raw executes a request and returns just the status and raw body.
func Raw(t *testing.T, method, url string, body any, token string) (int, string) {
	t.Helper()
	resp := Do(t, RawRequest{Method: method, URL: url, Body: body, Token: token})
	return resp.Status, resp.Body
}

// HTTPJSON executes a request and decodes a JSON object response.
func HTTPJSON(t *testing.T, method, url string, body any, token string) (int, JSONBody) {
	t.Helper()
	status, raw := Raw(t, method, url, body, token)
	return status, decodeJSONBody(t, method, url, raw)
}

func decodeJSONBody(t *testing.T, method, url, raw string) JSONBody {
	t.Helper()
	result := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			t.Fatalf("decode %s %s JSON: %v\nbody: %s", method, url, err, raw)
		}
	}
	return JSONBody{Object: result, Raw: raw}
}

// RegisterUser creates a new user at baseURL and returns its auth tokens.
func RegisterUser(t *testing.T, baseURL, email string) AuthTokens {
	t.Helper()
	status, body := HTTPJSON(t, http.MethodPost, baseURL+"/api/auth/register",
		map[string]string{"email": email, "password": "password123"}, "")
	if status != http.StatusCreated {
		t.Fatalf("register user status=%d body=%s", status, body)
	}
	user, ok := body.Object["user"].(map[string]any)
	if !ok {
		t.Fatalf("register response missing user object: %s", body)
	}
	return AuthTokens{
		AccessToken:  body.StringValue(t, "token"),
		RefreshToken: body.StringValue(t, "refreshToken"),
		UserID:       stringFromMap(t, user, "id"),
		Email:        stringFromMap(t, user, "email"),
	}
}

// ListSessions returns the caller's active auth sessions.
func ListSessions(t *testing.T, baseURL, token string) []Session {
	t.Helper()
	status, raw := Raw(t, http.MethodGet, baseURL+"/api/auth/sessions", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list sessions status=%d body=%s", status, raw)
	}
	var sessions []Session
	if err := json.Unmarshal([]byte(raw), &sessions); err != nil {
		t.Fatalf("decode sessions response: %v\nbody=%s", err, raw)
	}
	return sessions
}

// WaitForAuthMeStatus polls GET /api/auth/me until it returns wantStatus or the
// timeout elapses, returning the final response.
func WaitForAuthMeStatus(t *testing.T, baseURL, token string, wantStatus int, timeout time.Duration) Response {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var resp Response
	for time.Now().Before(deadline) {
		resp = Do(t, RawRequest{Method: http.MethodGet, URL: baseURL + "/api/auth/me", Token: token})
		if resp.Status == wantStatus {
			return resp
		}
		time.Sleep(100 * time.Millisecond)
	}
	return resp
}

// RandomHex returns byteCount random bytes hex-encoded, for unique test names.
func RandomHex(t *testing.T, byteCount int) string {
	t.Helper()
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate random bytes: %v", err)
	}
	return hex.EncodeToString(buf)
}
