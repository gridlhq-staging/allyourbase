// Package crossnode holds the importable, endpoint-parameterized assertion
// helpers shared by the multinode (two separate AYB processes) and cell
// topology (two AYB containers behind an nginx load balancer) E2E drivers.
//
// The helpers here deliberately take base-URL strings rather than a concrete
// node handle so the same proof logic can drive either topology: the multinode
// harness passes distinct per-node URLs, while the cell driver passes the LB
// URL for both write and observe endpoints. These files intentionally import
// "testing" in non-test code (precedent: internal/testutil) so the logic is
// importable by other packages' test binaries.
package crossnode

import "testing"

// Cluster is the endpoint seam a proof runs against. In the multinode topology
// WriteBaseURL and ObserveBaseURL point at two distinct node processes; in the
// cell topology both point at the single nginx LB URL, which round-robins
// across the two upstream containers.
type Cluster struct {
	WriteBaseURL   string
	ObserveBaseURL string
}

// AuthTokens holds the credentials returned by registering a user.
type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	UserID       string
	Email        string
}

// Session is a single row from the auth sessions listing endpoint.
type Session struct {
	ID string `json:"id"`
}

// JSONBody is a decoded JSON response paired with its raw text for diagnostics.
type JSONBody struct {
	Object map[string]any
	Raw    string
}

// String returns the raw response text so JSONBody formats usefully in %s/%v.
func (b JSONBody) String() string {
	return b.Raw
}

// StringValue returns a required non-empty string field from the decoded body.
func (b JSONBody) StringValue(t *testing.T, key string) string {
	t.Helper()
	return stringFromMap(t, b.Object, key)
}

func stringFromMap(t *testing.T, values map[string]any, key string) string {
	t.Helper()
	value, ok := values[key].(string)
	if !ok || value == "" {
		t.Fatalf("expected non-empty string field %q in %v", key, values)
	}
	return value
}
