package cli

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

type failingSQLInputReader struct {
	read bool
}

func (r *failingSQLInputReader) Read(_ []byte) (int, error) {
	r.read = true
	return 0, errors.New("stdin should not be read")
}

func TestResolveSQLInputArgsWinWithoutReadingStdin(t *testing.T) {
	stdin := &failingSQLInputReader{}

	query, err := resolveSQLInput([]string{"SELECT", "1"}, stdin, false)
	if err != nil {
		t.Fatalf("resolveSQLInput returned error: %v", err)
	}
	if query != "SELECT 1" {
		t.Fatalf("expected joined query args, got %q", query)
	}
	if stdin.read {
		t.Fatal("expected explicit query args to avoid reading stdin")
	}
}

func TestResolveSQLInputReadsTrimmedPipedStdin(t *testing.T) {
	query, err := resolveSQLInput(nil, strings.NewReader("\n SELECT 1; \n"), false)
	if err != nil {
		t.Fatalf("resolveSQLInput returned error: %v", err)
	}
	if query != "SELECT 1;" {
		t.Fatalf("expected trimmed piped SQL, got %q", query)
	}
}

func TestResolveSQLInputTerminalNoQueryReturnsGuidanceWithoutReadingStdin(t *testing.T) {
	stdin := &failingSQLInputReader{}

	_, err := resolveSQLInput(nil, stdin, true)
	if err == nil {
		t.Fatal("expected terminal stdin without query to return an error")
	}
	assertSQLInputGuidance(t, err)
	if stdin.read {
		t.Fatal("expected terminal stdin without query to avoid reading stdin")
	}
}

func TestSQLInputNonLoopbackWithoutTokenKeepsAuthGuardBeforeSQLRequest(t *testing.T) {
	t.Setenv("AYB_ADMIN_TOKEN", "")

	oldClient := cliHTTPClient
	requested := false
	cliHTTPClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requested = true
		return nil, errors.New("unexpected SQL HTTP request")
	})}
	t.Cleanup(func() { cliHTTPClient = oldClient })

	cmd := sqlCmd
	oldToken, _ := cmd.Flags().GetString("admin-token")
	oldURL, _ := cmd.Flags().GetString("url")
	t.Cleanup(func() {
		_ = cmd.Flags().Set("admin-token", oldToken)
		_ = cmd.Flags().Set("url", oldURL)
	})
	if err := cmd.Flags().Set("admin-token", ""); err != nil {
		t.Fatalf("resetting admin-token flag: %v", err)
	}
	if err := cmd.Flags().Set("url", "http://example.test"); err != nil {
		t.Fatalf("setting url flag: %v", err)
	}

	err := runSQL(cmd, []string{"SELECT 1"})
	if err == nil {
		t.Fatal("expected non-loopback SQL without token to fail")
	}
	if !strings.Contains(err.Error(), "pass --admin-token or set AYB_ADMIN_TOKEN") {
		t.Fatalf("expected auth guidance, got: %v", err)
	}
	if requested {
		t.Fatal("expected auth guard to return before SQL HTTP request")
	}
}

func assertSQLInputGuidance(t *testing.T, err error) {
	t.Helper()
	message := err.Error()
	for _, want := range []string{`ayb sql "..."`, "echo ... | ayb sql"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error %q to contain %q", message, want)
		}
	}
}
