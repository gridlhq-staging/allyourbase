package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEnsureDemoServerReusesHealthyAuthEnabledServer(t *testing.T) {
	ts := newDemoLifecycleServer(t, http.StatusUnauthorized)
	resetDemoServerLifecycleSeams(t)
	demoServerURLFunc = func() string { return ts.URL }

	baseURL, weStarted, err := ensureDemoServer("live-polls")
	if err != nil {
		t.Fatalf("ensureDemoServer returned error: %v", err)
	}
	if baseURL != ts.URL {
		t.Fatalf("baseURL = %q, want %q", baseURL, ts.URL)
	}
	if weStarted {
		t.Fatal("weStarted = true, want false for reused auth-enabled server")
	}
}

func TestEnsureDemoServerRestartsOwnedAuthDisabledServer(t *testing.T) {
	ts := newDemoLifecycleServer(t, http.StatusNotFound)
	port := mustDemoLifecyclePort(t, ts.URL)
	state := resetDemoServerLifecycleSeams(t)
	demoServerURLFunc = func() string { return ts.URL }
	demoReadAYBPIDFunc = func() (int, int, error) { return 4321, port, nil }
	demoPIDAliveFunc = func(pid int) bool { return pid == 4321 }

	baseURL, weStarted, err := ensureDemoServer("live-polls")
	if err != nil {
		t.Fatalf("ensureDemoServer returned error: %v", err)
	}
	if baseURL != ts.URL {
		t.Fatalf("baseURL = %q, want %q", baseURL, ts.URL)
	}
	if !weStarted {
		t.Fatal("weStarted = false, want true after demo-owned restart")
	}
	if state.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", state.stopCalls)
	}
	if state.startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", state.startCalls)
	}
	if state.waitForPortFreeCalls != 1 {
		t.Fatalf("wait-for-port-free calls = %d, want 1", state.waitForPortFreeCalls)
	}
	if state.startCmd == nil {
		t.Fatal("start command was not captured")
	}
	if !envContains(state.startCmd.Env, "AYB_AUTH_ENABLED=true") {
		t.Fatalf("start env missing AYB_AUTH_ENABLED=true: %#v", state.startCmd.Env)
	}
	if !envContains(state.startCmd.Env, "AYB_AUTH_JWT_SECRET=test-demo-secret") {
		t.Fatalf("start env missing resolved JWT secret: %#v", state.startCmd.Env)
	}
}

func TestEnsureDemoServerRejectsAuthDisabledPIDPortMismatch(t *testing.T) {
	ts := newDemoLifecycleServer(t, http.StatusNotFound)
	port := mustDemoLifecyclePort(t, ts.URL)
	state := resetDemoServerLifecycleSeams(t)
	demoServerURLFunc = func() string { return ts.URL }
	demoReadAYBPIDFunc = func() (int, int, error) { return 4321, port + 1, nil }
	demoPIDAliveFunc = func(pid int) bool { return pid == 4321 }

	baseURL, weStarted, err := ensureDemoServer("live-polls")
	if err == nil {
		t.Fatal("expected PID port mismatch to block demo startup")
	}
	if baseURL != "" {
		t.Fatalf("baseURL = %q, want empty on blocked startup", baseURL)
	}
	if weStarted {
		t.Fatal("weStarted = true, want false on blocked startup")
	}
	assertDemoBlockedWithoutProcessMutation(t, state, err.Error())
	if !strings.Contains(err.Error(), "PID file port") || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error should preserve PID port mismatch detail, got: %v", err)
	}
}

func TestEnsureDemoServerRejectsAuthDisabledMissingOrStalePID(t *testing.T) {
	tests := []struct {
		name      string
		readPID   func(int) func() (int, int, error)
		pidAlive  func(int) bool
		wantError string
	}{
		{
			name:      "missing PID file",
			readPID:   func(int) func() (int, int, error) { return func() (int, int, error) { return 0, 0, os.ErrNotExist } },
			pidAlive:  func(int) bool { return true },
			wantError: "no AYB PID file",
		},
		{
			name:      "stale PID file",
			readPID:   func(port int) func() (int, int, error) { return func() (int, int, error) { return 4321, port, nil } },
			pidAlive:  func(int) bool { return false },
			wantError: "is not live",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newDemoLifecycleServer(t, http.StatusNotFound)
			state := resetDemoServerLifecycleSeams(t)
			demoServerURLFunc = func() string { return ts.URL }
			demoReadAYBPIDFunc = tt.readPID(mustDemoLifecyclePort(t, ts.URL))
			demoPIDAliveFunc = tt.pidAlive

			_, weStarted, err := ensureDemoServer("live-polls")
			if err == nil {
				t.Fatal("expected missing/stale PID ownership to block demo startup")
			}
			if weStarted {
				t.Fatal("weStarted = true, want false on blocked startup")
			}
			assertDemoBlockedWithoutProcessMutation(t, state, err.Error())
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %q, want detail %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestEnsureDemoServerNeverStopsForeignAuthDisabledListener(t *testing.T) {
	ts := newDemoLifecycleServer(t, http.StatusNotFound)
	state := resetDemoServerLifecycleSeams(t)
	demoServerURLFunc = func() string { return ts.URL }
	demoReadAYBPIDFunc = func() (int, int, error) { return 0, 0, os.ErrNotExist }

	_, weStarted, err := ensureDemoServer("live-polls")
	if err == nil {
		t.Fatal("expected foreign auth-disabled listener to block demo startup")
	}
	if weStarted {
		t.Fatal("weStarted = true, want false for foreign listener")
	}
	assertDemoBlockedWithoutProcessMutation(t, state, err.Error())
	if !strings.Contains(err.Error(), "foreign or manually-started process") {
		t.Fatalf("error should identify anti-stop gap, got: %v", err)
	}
}

type demoLifecycleState struct {
	startCalls           int
	stopCalls            int
	waitForPortFreeCalls int
	startCmd             *exec.Cmd
}

func resetDemoServerLifecycleSeams(t *testing.T) *demoLifecycleState {
	t.Helper()
	state := &demoLifecycleState{}

	origServerURL := demoServerURLFunc
	origStartCommand := demoServerStartCommandFunc
	origResolveSecret := demoResolveJWTSecretFunc
	origReadPID := demoReadAYBPIDFunc
	origPIDAlive := demoPIDAliveFunc
	origStop := demoStopOwnedServerFunc
	origWait := demoWaitForPortFreeFunc
	t.Cleanup(func() {
		demoServerURLFunc = origServerURL
		demoServerStartCommandFunc = origStartCommand
		demoResolveJWTSecretFunc = origResolveSecret
		demoReadAYBPIDFunc = origReadPID
		demoPIDAliveFunc = origPIDAlive
		demoStopOwnedServerFunc = origStop
		demoWaitForPortFreeFunc = origWait
	})

	demoServerStartCommandFunc = func(aybBin, demoName string) (*exec.Cmd, func(), error) {
		state.startCalls++
		cmd := exec.Command("true")
		state.startCmd = cmd
		return cmd, func() {}, nil
	}
	demoResolveJWTSecretFunc = func() (string, error) {
		return "test-demo-secret", nil
	}
	demoStopOwnedServerFunc = func(pid int) error {
		state.stopCalls++
		return nil
	}
	demoWaitForPortFreeFunc = func(port int, timeout time.Duration) error {
		state.waitForPortFreeCalls++
		return nil
	}
	return state
}

func newDemoLifecycleServer(t *testing.T, authStatus int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/auth/me":
			w.WriteHeader(authStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func mustDemoLifecyclePort(t *testing.T, baseURL string) int {
	t.Helper()
	port, err := strconv.Atoi(demoServerPort(baseURL))
	if err != nil {
		t.Fatalf("parsing server port from %q: %v", baseURL, err)
	}
	return port
}

func assertDemoBlockedWithoutProcessMutation(t *testing.T, state *demoLifecycleState, msg string) {
	t.Helper()
	if state.stopCalls != 0 {
		t.Fatalf("stop calls = %d, want 0 for blocked startup", state.stopCalls)
	}
	if state.startCalls != 0 {
		t.Fatalf("start calls = %d, want 0 for blocked startup", state.startCalls)
	}
	if state.waitForPortFreeCalls != 0 {
		t.Fatalf("wait-for-port-free calls = %d, want 0 for blocked startup", state.waitForPortFreeCalls)
	}
	if !strings.Contains(msg, "ayb stop") || !strings.Contains(msg, "ayb demo") {
		t.Fatalf("blocked error should include manual stop/retry guidance, got: %s", msg)
	}
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
