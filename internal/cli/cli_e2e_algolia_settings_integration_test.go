//go:build integration

package cli

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/searchsettings"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCLI_E2E_MigrateAlgoliaSettings_PersistsSearchSettings(t *testing.T) {
	if cliE2EHarnessBinaryPath == "" {
		t.Fatal("expected cliE2EHarnessBinaryPath to be initialized by TestMain")
	}
	if cliE2EHarnessDatabaseURL == "" {
		t.Fatal("expected cliE2EHarnessDatabaseURL to be initialized by TestMain")
	}

	tableName := "products_import"
	dropTableCleanup(t, tableName)

	fakeAlgolia := newFakeAlgoliaServer(t)
	defer fakeAlgolia.Close()
	t.Setenv("ALGOLIA_ENDPOINT", fakeAlgolia.URL)

	stdout, stderr, exitCode := runCLIE2E(t,
		"migrate", "algolia",
		"--app-id", "FAKEAPPID",
		"--api-key", "FAKEAPIKEY",
		"--index", "products",
		"--database-url", cliE2EHarnessDatabaseURL,
		"--table", tableName,
		"--include-settings",
		"--include-synonyms",
		"-y",
	)
	if exitCode != 0 {
		t.Fatalf("migrate algolia failed with exit %d\nstdout=%s\nstderr=%s", exitCode, stdout, stderr)
	}

	waitForCollectionVisible(t, tableName)

	req, err := http.NewRequest(http.MethodGet,
		cliE2EHarnessBaseURL+"/api/collections/"+tableName+"/search-settings", nil)
	if err != nil {
		t.Fatalf("building search-settings request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cliE2EHarnessBearerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET search-settings: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET search-settings: expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var got searchsettings.Settings
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decoding search-settings response: %v\n%s", err, string(body))
	}

	wantAttrs := []searchsettings.Attribute{
		{Column: "title", Weight: searchsettings.WeightHigh},
		{Column: "subtitle", Weight: searchsettings.WeightLowest},
	}
	if !reflect.DeepEqual(got.Attributes, wantAttrs) {
		t.Fatalf("attributes = %#v, want %#v", got.Attributes, wantAttrs)
	}

	wantRanking := []searchsettings.CustomRanking{
		{Column: "inventory_count", Order: searchsettings.RankingOrderDesc},
		{Column: "price", Order: searchsettings.RankingOrderAsc},
	}
	if !reflect.DeepEqual(got.CustomRanking, wantRanking) {
		t.Fatalf("customRanking = %#v, want %#v", got.CustomRanking, wantRanking)
	}
}

func TestCLI_E2E_MigrateAlgoliaDryRunFailsWhenTargetTableExists(t *testing.T) {
	if cliE2EHarnessBinaryPath == "" {
		t.Fatal("expected cliE2EHarnessBinaryPath to be initialized by TestMain")
	}
	if cliE2EHarnessDatabaseURL == "" {
		t.Fatal("expected cliE2EHarnessDatabaseURL to be initialized by TestMain")
	}

	tableName := "products_dryrun_exists"
	dropTableCleanup(t, tableName)

	db, err := sql.Open("pgx", cliE2EHarnessDatabaseURL)
	if err != nil {
		t.Fatalf("open harness database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE "` + tableName + `" (id bigint)`); err != nil {
		t.Fatalf("create existing target table: %v", err)
	}

	fakeAlgolia := newFakeAlgoliaServer(t)
	defer fakeAlgolia.Close()
	t.Setenv("ALGOLIA_ENDPOINT", fakeAlgolia.URL)

	stdout, stderr, exitCode := runCLIE2E(t,
		"migrate", "algolia",
		"--app-id", "FAKEAPPID",
		"--api-key", "FAKEAPIKEY",
		"--index", "products",
		"--database-url", cliE2EHarnessDatabaseURL,
		"--table", tableName,
		"--include-settings",
		"--dry-run",
	)
	if exitCode == 0 {
		t.Fatalf("dry-run unexpectedly succeeded\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "target table "+tableName+" already exists") {
		t.Fatalf("dry-run stderr = %q, want existing-target message", stderr)
	}
}

func waitForCollectionVisible(t *testing.T, tableName string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if collectionVisible(t, tableName) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("collection %q was not visible in /api/schema before timeout", tableName)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func collectionVisible(t *testing.T, tableName string) bool {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cliE2EHarnessBaseURL+"/api/schema", nil)
	if err != nil {
		t.Fatalf("building schema request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cliE2EHarnessBearerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET schema: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var schema struct {
		Tables map[string]json.RawMessage `json:"tables"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		t.Fatalf("decoding schema response: %v", err)
	}
	for _, raw := range schema.Tables {
		var table struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &table); err != nil {
			t.Fatalf("decoding schema table: %v", err)
		}
		if table.Name == tableName {
			return true
		}
	}
	return false
}

func newFakeAlgoliaServer(t *testing.T) *httptest.Server {
	t.Helper()
	responses := map[string][]byte{
		"/browse":          readAlgoliaFixture(t, "algolia_browse_sample.json"),
		"/settings":        readAlgoliaFixture(t, "algolia_settings_sample.json"),
		"/synonyms/search": readAlgoliaFixture(t, "algolia_synonyms_sample.json"),
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/browse") && browseRequestHasCursor(t, r) {
			_, _ = w.Write([]byte(`{"hits":[]}`))
			return
		}
		for suffix, body := range responses {
			if strings.HasSuffix(r.URL.Path, suffix) {
				_, _ = w.Write(body)
				return
			}
		}
		http.NotFound(w, r)
	}))
}

func browseRequestHasCursor(t *testing.T, r *http.Request) bool {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decoding fake Algolia browse request: %v", err)
	}
	return body["cursor"] != ""
}

func readAlgoliaFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("../algoliamigrate/testdata/" + name)
	if err != nil {
		t.Fatalf("reading Algolia fixture %s: %v", name, err)
	}
	return body
}
