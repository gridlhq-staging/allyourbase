// Package cli.
package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/examples"
)

var (
	demoServerURLFunc          = serverURL
	demoServerStartCommandFunc = demoServerStartCommand
	demoResolveJWTSecretFunc   = resolveDemoJWTSecret
	demoReadAYBPIDFunc         = readAYBPID
	demoPIDAliveFunc           = demoPIDAlive
	demoPIDMatchesAYBFunc      = pidMatchesAYBBinary
	demoStopOwnedServerFunc    = func(pid int) error {
		_, err := stopAYBServerFromPID(pid)
		return err
	}
	demoWaitForPortFreeFunc = waitForDemoPortFree
)

// ensureDemoServer returns the configured server URL and starts an auth-enabled
// local AYB server when one is not already running.
func ensureDemoServer(demoName string) (string, bool, error) {
	base := demoServerURLFunc()
	client := &http.Client{Timeout: 2 * time.Second}

	// Check if already running.
	resp, err := client.Get(base + "/health")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			authEnabled, err := demoAuthEnabled(base)
			if err != nil {
				return "", false, fmt.Errorf("checking auth status: %w", err)
			}
			if authEnabled {
				return base, false, nil
			}
			return recoverOwnedAuthDisabledDemoServer(base, demoName)
		}
	}

	return startAuthEnabledDemoServer(base, demoName)
}

func recoverOwnedAuthDisabledDemoServer(baseURL, demoName string) (string, bool, error) {
	pid, err := demoOwnedPIDForPort(baseURL)
	if err != nil {
		return "", false, demoAuthDisabledOwnershipError(baseURL, err)
	}
	if err := demoStopOwnedServerFunc(pid); err != nil {
		return "", false, fmt.Errorf("stopping owned auth-disabled AYB server: %w", err)
	}
	port, err := strconv.Atoi(demoServerPort(baseURL))
	if err != nil {
		return "", false, fmt.Errorf("parsing server port for restart: %w", err)
	}
	if err := demoWaitForPortFreeFunc(port, 10*time.Second); err != nil {
		return "", false, fmt.Errorf("waiting for AYB server port %d to be free after stop: %w", port, err)
	}
	return startAuthEnabledDemoServer(baseURL, demoName)
}

func startAuthEnabledDemoServer(baseURL, demoName string) (string, bool, error) {
	// Not running — auto-start via `ayb start`.
	// cmd.Run() blocks until the parent `ayb start` exits (after readiness).
	aybBin, err := os.Executable()
	if err != nil {
		aybBin = os.Args[0]
	}
	jwtSecret, err := demoResolveJWTSecretFunc()
	if err != nil {
		return "", false, fmt.Errorf("generating demo auth secret: %w", err)
	}

	startCmd, cleanup, err := demoServerStartCommandFunc(aybBin, demoName)
	if err != nil {
		return "", false, err
	}
	defer cleanup()
	startCmd.Env = demoServerStartEnv(jwtSecret, demoName)
	startCmd.Stdout = io.Discard
	var startErr strings.Builder
	startCmd.Stderr = &startErr

	if err := startCmd.Run(); err != nil {
		detail := strings.TrimSpace(startErr.String())
		if detail != "" {
			return "", false, fmt.Errorf("failed to start AYB server:\n  %s", detail)
		}
		return "", false, fmt.Errorf("failed to start AYB server: %w", err)
	}
	return baseURL, true, nil
}

func demoOwnedPIDForPort(baseURL string) (int, error) {
	pid, pidPort, err := demoReadAYBPIDFunc()
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("no AYB PID file proves ownership of the listener on port %s", demoServerPort(baseURL))
		}
		return 0, fmt.Errorf("reading AYB PID file: %w", err)
	}
	targetPort, err := strconv.Atoi(demoServerPort(baseURL))
	if err != nil {
		return 0, fmt.Errorf("parsing target server port: %w", err)
	}
	if pidPort != targetPort {
		return 0, fmt.Errorf("AYB PID file port %d does not match target port %d", pidPort, targetPort)
	}
	if !demoPIDAliveFunc(pid) {
		return 0, fmt.Errorf("AYB PID file process %d is not live", pid)
	}
	if !demoPIDMatchesAYBFunc(pid) {
		return 0, fmt.Errorf("AYB PID file process %d is not an AYB server", pid)
	}
	return pid, nil
}

func demoPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func demoAuthDisabledOwnershipError(baseURL string, cause error) error {
	port := demoServerPort(baseURL)
	return fmt.Errorf("the running AYB server has auth disabled, but AYB will not stop a foreign or manually-started process without a matching live AYB PID file: %w\n\n  Stop the server yourself and retry:\n    ayb stop --port %s && ayb demo <name>\n\n  Or run the demo against an auth-enabled server, a proxy, or an alternate AYB server port.", cause, port)
}

func waitForDemoPortFree(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !portInUse(port) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("port %d is still in use", port)
}

func demoServerStartCommand(aybBin, demoName string) (*exec.Cmd, func(), error) {
	if demoName != "movies" {
		return exec.Command(aybBin, "start"), func() {}, nil
	}
	configPath, cleanup, err := materializeEmbeddedDemoConfig(demoName)
	if err != nil {
		return nil, nil, err
	}
	return exec.Command(aybBin, "start", "--config", configPath), cleanup, nil
}

func materializeEmbeddedDemoConfig(demoName string) (string, func(), error) {
	data, err := fs.ReadFile(examples.FS, demoName+"/ayb.toml")
	if err != nil {
		return "", nil, fmt.Errorf("reading embedded %s/ayb.toml: %w", demoName, err)
	}
	file, err := os.CreateTemp("", "ayb-demo-"+demoName+"-*.toml")
	if err != nil {
		return "", nil, fmt.Errorf("creating demo config: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("writing demo config: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("hardening demo config permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("closing demo config: %w", err)
	}
	return file.Name(), cleanup, nil
}

func demoServerStartEnv(jwtSecret, demoName string) []string {
	siteURL := "http://localhost:" + demoDefaultServerPort
	if demo, ok := demoRegistry[demoName]; ok {
		// WebAuthn verifies the browser origin, so demo-started servers
		// advertise the app origin rather than the backend API target.
		siteURL = fmt.Sprintf("http://localhost:%d", demo.Port)
	}
	return append(
		os.Environ(),
		"AYB_AUTH_ENABLED=true",
		"AYB_AUTH_JWT_SECRET="+jwtSecret,
		"AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true",
		"AYB_SERVER_SITE_URL="+siteURL,
	)
}

func resolveDemoJWTSecret() (string, error) {
	if secret := strings.TrimSpace(os.Getenv("AYB_AUTH_JWT_SECRET")); secret != "" {
		return secret, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// demoServerPort extracts the port from a server URL, falling back to scheme defaults or the default demo port.
func demoServerPort(baseURL string) string {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return demoDefaultServerPort
	}
	if port := strings.TrimSpace(parsedURL.Port()); port != "" {
		return port
	}
	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return demoDefaultServerPort
	}
}

func demoRestartCommand() string {
	return "ayb stop && ayb demo <name>"
}

func demoKillCommand() string {
	return "lsof -ti :8090 | xargs kill && ayb demo <name>"
}

func demoCustomPortNote(baseURL, command string) string {
	port := demoServerPort(baseURL)
	if port == demoDefaultServerPort {
		return ""
	}
	return fmt.Sprintf("\n\n  If the server is using port %s, use instead:\n    %s", port, command)
}

// requireDemoAuthEnabled checks that the running server has auth enabled, returning an actionable error with restart instructions if it does not.
func requireDemoAuthEnabled(baseURL string, useColor bool) error {
	enabled, err := demoAuthEnabled(baseURL)
	if err != nil {
		return fmt.Errorf("checking auth status: %w", err)
	}
	if enabled {
		return nil
	}
	return fmt.Errorf("%s %s\n\n  %s\n    [auth]\n    enabled = true\n\n  %s\n\n    %s%s\n\n  %s",
		yellow("⚠", useColor),
		yellow("The running AYB server has auth disabled. Demos require auth for registration and login.", useColor),
		dim("Enable auth in ayb.toml:", useColor),
		dim("Or stop the running server and let the demo start its own auth-enabled server:", useColor),
		demoRestartCommand(),
		demoCustomPortNote(baseURL, fmt.Sprintf("ayb stop --port %s && ayb demo <name>", demoServerPort(baseURL))),
		dim("Then restart your usual server config after the demo if needed.", useColor),
	)
}

// demoAuthEnabled probes the server to determine whether the public auth
// routes are available. /api/auth/me returns 404 when auth is disabled and
// 401/200 when the route exists.
func demoAuthEnabled(baseURL string) (bool, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/auth/me")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return false, fmt.Errorf("auth probe returned %d and the response body could not be read: %w", resp.StatusCode, readErr)
		}
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			return false, fmt.Errorf("auth probe returned unexpected status %d", resp.StatusCode)
		}
		return false, fmt.Errorf("auth probe returned unexpected status %d: %s", resp.StatusCode, detail)
	}
}
