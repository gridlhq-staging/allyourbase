//go:build integration

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

const instantsearchSmokeTimeout = 90 * time.Second

type instantsearchSmokePayload struct {
	Items      []map[string]any `json:"items"`
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

type instantsearchSmokePorts struct {
	server   int
	postgres int
	app      int
}

type instantsearchSmokePortReservation struct {
	ports     instantsearchSmokePorts
	listeners []net.Listener
}

type instantsearchSmokeAuthPayload struct {
	Token string `json:"token"`
}

func testDemoInstantsearchOneCommandSmoke(t *testing.T) {
	t.Helper()

	binary := buildTestBinary(t)
	homeDir := instantsearchSmokeShortTempDir(t, "ayb-is-home-*")
	dataDir := instantsearchSmokeShortTempDir(t, "ayb-is-pg-*")
	ports := instantsearchSmokeReservePorts(t)
	env := append(os.Environ(),
		"HOME="+homeDir,
		"AYB_DATABASE_EMBEDDED_DATA_DIR="+dataDir,
		"AYB_SERVER_PORT="+strconv.Itoa(ports.ports.server),
		"AYB_DATABASE_EMBEDDED_PORT="+strconv.Itoa(ports.ports.postgres),
		"AYB_DEMO_APP_PORT="+strconv.Itoa(ports.ports.app),
	)

	var output bytes.Buffer
	cmd := exec.Command(binary, "demo", "instantsearch")
	cmd.Env = env
	cmd.Stdout = &output
	cmd.Stderr = &output

	ports.release(t)
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting ayb demo instantsearch: %v", err)
	}
	processExit, processDone := observeProcessExit(cmd)
	t.Cleanup(func() {
		instantsearchSmokeCleanup(t, binary, env, cmd, processDone, ports.ports, &output)
	})

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", ports.ports.server)
	appURL := fmt.Sprintf("http://127.0.0.1:%d", ports.ports.app)
	if err := instantsearchSmokeWaitReady(t, serverURL, appURL, processExit, &output, homeDir, dataDir); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	page := instantsearchSmokeGET(t, client, appURL+"/")
	if !strings.Contains(page, "<title>AYB InstantSearch Demo</title>") {
		t.Fatalf("demo page missing title; body excerpt: %s", instantsearchSmokeExcerpt(page))
	}
	if !strings.Contains(page, `<div id="root"></div>`) {
		t.Fatalf("demo page missing root div; body excerpt: %s", instantsearchSmokeExcerpt(page))
	}

	token := instantsearchSmokeLogin(t, client, appURL)
	instantsearchSmokeAssertList(t, client, token, appURL+"/api/collections/instantsearch_products?perPage=20", 20, 20, 1, "")
	instantsearchSmokeAssertList(t, client, token, appURL+"/api/collections/instantsearch_products?filter=category%3D%27Stationery%27&perPage=20", 20, 4, 1, "")
	instantsearchSmokeAssertList(t, client, token, appURL+"/api/collections/instantsearch_products?search=crimson+ledger&perPage=5", 5, 1, 1, "red-notebook")
}

func instantsearchSmokeShortTempDir(t *testing.T, pattern string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", pattern)
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func instantsearchSmokeReservePorts(t *testing.T) instantsearchSmokePortReservation {
	t.Helper()
	listeners := make([]net.Listener, 0, 3)
	ports := make([]int, 0, 3)
	seen := map[int]bool{}
	for range 3 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		testutil.NoError(t, err)
		listeners = append(listeners, listener)

		tcpAddr, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			t.Fatalf("listener address %T is not a TCP address", listener.Addr())
		}
		port := tcpAddr.Port
		if seen[port] {
			t.Fatalf("duplicate free port allocated: %d", port)
		}
		seen[port] = true
		ports = append(ports, port)
	}
	return instantsearchSmokePortReservation{
		ports: instantsearchSmokePorts{
			server:   ports[0],
			postgres: ports[1],
			app:      ports[2],
		},
		listeners: listeners,
	}
}

func (reservation instantsearchSmokePortReservation) release(t *testing.T) {
	t.Helper()
	for _, listener := range reservation.listeners {
		testutil.NoError(t, listener.Close())
	}
}

func instantsearchSmokeWaitReady(t *testing.T, serverURL, appURL string, processExit <-chan error, output *bytes.Buffer, homeDir, dataDir string) error {
	t.Helper()
	serverPort := mustPortFromURL(t, serverURL)
	healthReady := make(chan bool, 1)
	appReady := make(chan bool, 1)
	go func() {
		healthReady <- waitForHealthPort(serverPort, instantsearchSmokeTimeout)
	}()
	go func() {
		appReady <- instantsearchSmokeWaitForApp(appURL, instantsearchSmokeTimeout)
	}()

	healthOK, appOK := false, false
	deadline := time.After(instantsearchSmokeTimeout + 5*time.Second)
	for !healthOK || !appOK {
		select {
		case err := <-processExit:
			return fmt.Errorf("ayb demo instantsearch exited before readiness: %v\n%s", err, instantsearchSmokeOutput(output, homeDir, dataDir))
		case healthOK = <-healthReady:
			if !healthOK {
				return fmt.Errorf("timed out waiting for %s/health\n%s", serverURL, instantsearchSmokeOutput(output, homeDir, dataDir))
			}
		case appOK = <-appReady:
			if !appOK {
				return fmt.Errorf("timed out waiting for %s/\n%s", appURL, instantsearchSmokeOutput(output, homeDir, dataDir))
			}
		case <-deadline:
			return fmt.Errorf("timed out waiting for instantsearch demo readiness\n%s", instantsearchSmokeOutput(output, homeDir, dataDir))
		}
	}
	return nil
}

func instantsearchSmokeWaitForApp(appURL string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(appURL + "/")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func instantsearchSmokeCleanup(t *testing.T, binary string, env []string, cmd *exec.Cmd, done <-chan struct{}, ports instantsearchSmokePorts, output *bytes.Buffer) {
	t.Helper()
	select {
	case <-done:
	default:
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
		}
	}

	stopCmd := exec.Command(binary, "stop", "--port", strconv.Itoa(ports.server))
	stopCmd.Env = env
	_ = stopCmd.Run()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}

	if waitForHealthPort(ports.server, 2*time.Second) {
		t.Fatalf("server health still responds on port %d after cleanup\n%s", ports.server, strings.TrimSpace(output.String()))
	}
	if instantsearchSmokeWaitForApp(fmt.Sprintf("http://127.0.0.1:%d", ports.app), 2*time.Second) {
		t.Fatalf("demo app still responds on port %d after cleanup\n%s", ports.app, strings.TrimSpace(output.String()))
	}
}

func instantsearchSmokeGET(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	return instantsearchSmokeDo(t, client, http.MethodGet, url, "", nil)
}

func instantsearchSmokeDo(t *testing.T, client *http.Client, method, url, token string, body io.Reader) string {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	testutil.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	testutil.NoError(t, err)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s returned %d; body excerpt: %s", method, url, resp.StatusCode, instantsearchSmokeExcerpt(string(raw)))
	}
	return string(raw)
}

func instantsearchSmokeLogin(t *testing.T, client *http.Client, appURL string) string {
	t.Helper()
	payload := fmt.Sprintf(`{"email":%q,"password":%q}`, demoSeedUsers[0].Email, demoSeedUsers[0].Password)
	body := instantsearchSmokeDo(t, client, http.MethodPost, appURL+"/api/auth/login", "", strings.NewReader(payload))
	var got instantsearchSmokeAuthPayload
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decoding login response: %v; body excerpt: %s", err, instantsearchSmokeExcerpt(body))
	}
	if got.Token == "" {
		t.Fatalf("login response missing token; body excerpt: %s", instantsearchSmokeExcerpt(body))
	}
	return got.Token
}

func instantsearchSmokeAssertList(t *testing.T, client *http.Client, token, url string, wantPerPage, wantTotalItems, wantTotalPages int, wantSlug string) {
	t.Helper()
	body := instantsearchSmokeDo(t, client, http.MethodGet, url, token, nil)
	var got instantsearchSmokePayload
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decoding %s: %v; body excerpt: %s", url, err, instantsearchSmokeExcerpt(body))
	}
	if got.Page != 1 || got.PerPage != wantPerPage || got.TotalItems != wantTotalItems || got.TotalPages != wantTotalPages {
		t.Fatalf("%s pagination = page:%d perPage:%d totalItems:%d totalPages:%d, want 1/%d/%d/%d; body excerpt: %s",
			url, got.Page, got.PerPage, got.TotalItems, got.TotalPages, wantPerPage, wantTotalItems, wantTotalPages, instantsearchSmokeExcerpt(body))
	}
	if wantSlug == "" {
		return
	}
	for _, item := range got.Items {
		if item["slug"] == wantSlug {
			if !strings.Contains(fmt.Sprint(item["description"]), "crimson ledger") {
				t.Fatalf("%s item %q description = %q, want crimson ledger", url, wantSlug, item["description"])
			}
			if item["title"] != "Red Notebook" || item["category"] != "Stationery" || item["brand"] != "Apex" {
				t.Fatalf("%s item %q fields = title:%q category:%q brand:%q, want Red Notebook/Stationery/Apex",
					url, wantSlug, item["title"], item["category"], item["brand"])
			}
			return
		}
	}
	t.Fatalf("%s missing slug %q; body excerpt: %s", url, wantSlug, instantsearchSmokeExcerpt(body))
}

func instantsearchSmokeExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 512 {
		return body[:512]
	}
	return body
}

func instantsearchSmokeOutput(output *bytes.Buffer, homeDir, dataDir string) string {
	sections := []string{strings.TrimSpace(output.String())}
	sections = append(sections, instantsearchSmokeLogFiles(filepath.Join(homeDir, ".ayb", "logs"))...)
	sections = append(sections, instantsearchSmokeLogFiles(dataDir)...)
	return strings.Join(sections, "\n")
}

func instantsearchSmokeLogFiles(logDir string) []string {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}
	sections := []string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		path := filepath.Join(logDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			sections = append(sections, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		sections = append(sections, fmt.Sprintf("%s:\n%s", path, instantsearchSmokeLogExcerpt(string(data))))
	}
	return sections
}

func instantsearchSmokeLogExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 8192 {
		return body[len(body)-8192:]
	}
	return body
}

func mustPortFromURL(t *testing.T, rawURL string) int {
	t.Helper()
	_, port, ok := strings.Cut(rawURL, "http://127.0.0.1:")
	if !ok {
		t.Fatalf("unexpected smoke URL %q", rawURL)
	}
	parsed, err := strconv.Atoi(strings.Trim(port, "/"))
	testutil.NoError(t, err)
	return parsed
}
