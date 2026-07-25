//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

// newOrgAdminFixtureServer boots a test server wired with the real DB-backed
// org, team, and membership stores plus the tenant service, mirroring the
// branch integration bootstrap. server.New already initializes the private
// DB-backed org usage/audit queriers when constructed with sharedPG.Pool, so no
// fixture-local usage/audit stores or new server setters are introduced here.
func newOrgAdminFixtureServer(t *testing.T, ctx context.Context) *httptest.Server {
	t.Helper()

	createIntegrationTestSchema(t, ctx)
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "test-admin-pass"

	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	srv.SetTenantService(tenant.NewService(sharedPG.Pool, logger))
	srv.SetOrgStore(tenant.NewPostgresOrgStore(sharedPG.Pool, logger))
	srv.SetTeamStore(tenant.NewPostgresTeamStore(sharedPG.Pool, logger))
	srv.SetOrgMembershipStore(tenant.NewPostgresOrgMembershipStore(sharedPG.Pool, logger))
	srv.SetTeamMembershipStore(tenant.NewPostgresTeamMembershipStore(sharedPG.Pool, logger))

	return httptest.NewServer(srv.Router())
}

// seedOrgAdminUser inserts one deterministic user directly through the pool and
// returns its server-generated UUID for membership prerequisites.
func seedOrgAdminUser(t *testing.T, ctx context.Context) string {
	t.Helper()
	var userID string
	testutil.NoError(t, sharedPG.Pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		"org-admin-fixture@example.com", "integration-password-hash",
	).Scan(&userID))
	return userID
}

// seedOrgAdminTenant creates one shared-isolation tenant via the tenant service
// and returns its UUID for the tenant assignment flow.
func seedOrgAdminTenant(t *testing.T, ctx context.Context) string {
	t.Helper()
	svc := tenant.NewService(sharedPG.Pool, testutil.DiscardLogger())
	created, err := svc.CreateTenant(ctx, "Fixture Tenant", "fixture-tenant", "shared", "free", "us-east-1", nil, "")
	testutil.NoError(t, err)
	return created.ID
}

// TestAdminOrgsSDKContractFixtures drives one dependency-ordered admin-token
// flow through the real /api/admin/orgs* router and asserts every captured
// request/response payload against a committed sanitized fixture. Missing or
// drifted fixtures fail red and print the sanitized real-server payload so the
// canonical JSON can be committed by hand.
func TestAdminOrgsSDKContractFixtures(t *testing.T) {
	ctx := context.Background()
	ts := newOrgAdminFixtureServer(t, ctx)
	defer ts.Close()

	userID := seedOrgAdminUser(t, ctx)
	tenantID := seedOrgAdminTenant(t, ctx)
	token := branchAdminLogin(t, ts, "test-admin-pass")

	rec := &orgAdminFixtureRecorder{t: t, ts: ts, token: token}
	orgID := rec.orgLifecycle()
	teamID := rec.teamLifecycle(orgID)
	rec.orgMembership(orgID, userID)
	rec.teamMembership(orgID, teamID, userID)
	rec.tenantAssignment(orgID, tenantID)
	rec.cleanup(orgID, teamID, userID, tenantID)

	rec.assertAll()
}

// orgAdminFixtureCapture pairs a fixture basename with its sanitized payload.
type orgAdminFixtureCapture struct {
	name    string
	payload any
}

// orgAdminFixtureRecorder issues admin requests, asserts status codes, and
// accumulates sanitized request/response payloads for end-of-flow comparison.
type orgAdminFixtureRecorder struct {
	t        *testing.T
	ts       *httptest.Server
	token    string
	captures []orgAdminFixtureCapture
}

// call sends one admin request, asserting the expected status. When reqName is
// non-empty the sanitized request body is recorded; when respName is non-empty
// and the response carries a body the sanitized response is recorded. The
// unsanitized decoded response is returned so callers can read real UUIDs.
func (r *orgAdminFixtureRecorder) call(method, path string, body []byte, wantStatus int, reqName, respName string) map[string]any {
	r.t.Helper()
	if reqName != "" {
		r.captures = append(r.captures, orgAdminFixtureCapture{name: reqName, payload: sanitizeOrgAdminFixture(mustDecodeJSON(r.t, body))})
	}

	req := adminRequest(r.t, method, r.ts.URL+path, r.token, body)
	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(r.t, err)
	defer resp.Body.Close()
	testutil.StatusCode(r.t, wantStatus, resp.StatusCode)
	if wantStatus == http.StatusNoContent {
		return nil
	}

	rawBody, err := io.ReadAll(resp.Body)
	testutil.NoError(r.t, err)
	if respName != "" {
		r.captures = append(r.captures, orgAdminFixtureCapture{name: respName, payload: sanitizeOrgAdminFixture(mustDecodeJSON(r.t, rawBody))})
	}
	var decoded map[string]any
	testutil.NoError(r.t, json.Unmarshal(rawBody, &decoded))
	return decoded
}

func (r *orgAdminFixtureRecorder) orgLifecycle() string {
	base := "/api/admin/orgs"
	created := r.call(http.MethodPost, base,
		mustJSON(r.t, map[string]any{"name": "Fixture Org", "slug": "fixture-org", "planTier": "pro"}),
		http.StatusCreated, "org_admin_org_create_request.json", "org_admin_org_create_response.json")
	orgID, _ := created["id"].(string)
	if orgID == "" {
		r.t.Fatalf("org create returned empty id: %v", created)
	}

	r.call(http.MethodGet, base, nil, http.StatusOK, "", "org_admin_org_list_response.json")
	r.call(http.MethodGet, base+"/"+orgID, nil, http.StatusOK, "", "org_admin_org_get_response.json")
	r.call(http.MethodPut, base+"/"+orgID,
		mustJSON(r.t, map[string]any{"name": "Fixture Org Renamed", "slug": "fixture-org-renamed"}),
		http.StatusOK, "org_admin_org_update_request.json", "org_admin_org_update_response.json")
	r.call(http.MethodGet, base+"/"+orgID+"/usage", nil, http.StatusOK, "", "org_admin_org_usage_response.json")
	r.call(http.MethodGet, base+"/"+orgID+"/audit", nil, http.StatusOK, "", "org_admin_org_audit_response.json")
	return orgID
}

func (r *orgAdminFixtureRecorder) teamLifecycle(orgID string) string {
	base := "/api/admin/orgs/" + orgID + "/teams"
	created := r.call(http.MethodPost, base,
		mustJSON(r.t, map[string]any{"name": "Fixture Team", "slug": "fixture-team"}),
		http.StatusCreated, "org_admin_team_create_request.json", "org_admin_team_create_response.json")
	teamID, _ := created["id"].(string)
	if teamID == "" {
		r.t.Fatalf("team create returned empty id: %v", created)
	}

	r.call(http.MethodGet, base, nil, http.StatusOK, "", "org_admin_team_list_response.json")
	r.call(http.MethodGet, base+"/"+teamID, nil, http.StatusOK, "", "org_admin_team_get_response.json")
	r.call(http.MethodPut, base+"/"+teamID,
		mustJSON(r.t, map[string]any{"name": "Fixture Team Renamed", "slug": "fixture-team-renamed"}),
		http.StatusOK, "org_admin_team_update_request.json", "org_admin_team_update_response.json")
	return teamID
}

func (r *orgAdminFixtureRecorder) orgMembership(orgID, userID string) {
	base := "/api/admin/orgs/" + orgID + "/members"
	r.call(http.MethodPost, base,
		mustJSON(r.t, map[string]any{"userId": userID, "role": "admin"}),
		http.StatusCreated, "org_admin_org_member_add_request.json", "org_admin_org_member_add_response.json")
	r.call(http.MethodGet, base, nil, http.StatusOK, "", "org_admin_org_member_list_response.json")
	r.call(http.MethodPut, base+"/"+userID+"/role",
		mustJSON(r.t, map[string]any{"role": "member"}),
		http.StatusOK, "org_admin_org_member_role_update_request.json", "org_admin_org_member_role_update_response.json")
}

func (r *orgAdminFixtureRecorder) teamMembership(orgID, teamID, userID string) {
	base := "/api/admin/orgs/" + orgID + "/teams/" + teamID + "/members"
	r.call(http.MethodPost, base,
		mustJSON(r.t, map[string]any{"userId": userID, "role": "member"}),
		http.StatusCreated, "org_admin_team_member_add_request.json", "org_admin_team_member_add_response.json")
	r.call(http.MethodGet, base, nil, http.StatusOK, "", "org_admin_team_member_list_response.json")
	r.call(http.MethodPut, base+"/"+userID+"/role",
		mustJSON(r.t, map[string]any{"role": "lead"}),
		http.StatusOK, "org_admin_team_member_role_update_request.json", "org_admin_team_member_role_update_response.json")
}

func (r *orgAdminFixtureRecorder) tenantAssignment(orgID, tenantID string) {
	base := "/api/admin/orgs/" + orgID + "/tenants"
	r.call(http.MethodPost, base,
		mustJSON(r.t, map[string]any{"tenantId": tenantID}),
		http.StatusOK, "org_admin_tenant_assign_request.json", "org_admin_tenant_assign_response.json")
	r.call(http.MethodGet, base, nil, http.StatusOK, "", "org_admin_tenant_list_response.json")
}

// cleanup exercises the delete/unassign flows. These return 204 with no body,
// so no fixtures are captured, but exact status codes are still asserted.
func (r *orgAdminFixtureRecorder) cleanup(orgID, teamID, userID, tenantID string) {
	orgBase := "/api/admin/orgs/" + orgID
	r.call(http.MethodDelete, orgBase+"/teams/"+teamID+"/members/"+userID, nil, http.StatusNoContent, "", "")
	r.call(http.MethodDelete, orgBase+"/members/"+userID, nil, http.StatusNoContent, "", "")
	r.call(http.MethodDelete, orgBase+"/teams/"+teamID, nil, http.StatusNoContent, "", "")
	r.call(http.MethodDelete, orgBase+"/tenants/"+tenantID, nil, http.StatusNoContent, "", "")
	r.call(http.MethodDelete, orgBase+"?confirm=true", nil, http.StatusNoContent, "", "")
}

// assertAll compares every captured payload against its committed fixture,
// collecting all missing/drifted results so a single red run prints every
// sanitized payload needed to bootstrap or repair the fixtures.
func (r *orgAdminFixtureRecorder) assertAll() {
	r.t.Helper()
	var failures []string
	for _, capture := range r.captures {
		fixture, found := loadOrgAdminFixture(r.t, capture.name)
		if !found {
			failures = append(failures, fmt.Sprintf("missing fixture %s; sanitized payload:\n%s",
				capture.name, indentedOrgAdminJSON(r.t, capture.payload)))
			continue
		}
		if !reflect.DeepEqual(fixture, capture.payload) {
			failures = append(failures, fmt.Sprintf("fixture %s drifted\nwant:\n%s\ngot:\n%s",
				capture.name, indentedOrgAdminJSON(r.t, fixture), indentedOrgAdminJSON(r.t, capture.payload)))
		}
	}
	if len(failures) > 0 {
		r.t.Fatalf("org-admin contract fixtures failed (%d):\n%s", len(failures), strings.Join(failures, "\n\n"))
	}
}

// volatileOrgAdminFixtureKeys maps run-varying JSON keys to stable semantic
// placeholders. UUID linkages and timestamps are normalized; every other field
// (names, slugs, plan/role/status values, list envelopes, counts) is preserved.
var volatileOrgAdminFixtureKeys = map[string]string{
	"id":          "<id>",
	"orgId":       "<orgId>",
	"teamId":      "<teamId>",
	"userId":      "<userId>",
	"tenantId":    "<tenantId>",
	"parentOrgId": "<parentOrgId>",
	"createdAt":   "<timestamp>",
	"updatedAt":   "<timestamp>",
	"date":        "<date>",
}

// sanitizeOrgAdminFixture recursively replaces the string value of every
// volatile key with its placeholder, leaving structure and all other values
// intact. It mutates and returns the passed tree.
func sanitizeOrgAdminFixture(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if placeholder, ok := volatileOrgAdminFixtureKeys[key]; ok {
				if _, isString := child.(string); isString {
					typed[key] = placeholder
					continue
				}
			}
			typed[key] = sanitizeOrgAdminFixture(child)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = sanitizeOrgAdminFixture(child)
		}
		return typed
	default:
		return value
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	encoded, err := json.Marshal(v)
	testutil.NoError(t, err)
	return encoded
}

func mustDecodeJSON(t *testing.T, raw []byte) any {
	t.Helper()
	var decoded any
	testutil.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded
}

func indentedOrgAdminJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.MarshalIndent(v, "", "  ")
	testutil.NoError(t, err)
	return string(encoded)
}

// loadOrgAdminFixture reads and decodes a committed fixture, reporting absence
// distinctly from read/parse errors. It never writes.
func loadOrgAdminFixture(t *testing.T, name string) (any, bool) {
	t.Helper()
	body, err := os.ReadFile(orgAdminFixturePath(t, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		testutil.NoError(t, err)
	}
	var fixture any
	testutil.NoError(t, json.Unmarshal(body, &fixture))
	return fixture, true
}

// orgAdminFixturePath resolves a single fixture basename to its path under
// tests/contract/fixtures/sdk_contract, walking up from the working directory.
func orgAdminFixturePath(t *testing.T, name string) string {
	t.Helper()
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		t.Fatalf("fixture name must be a single filename: %q", name)
	}
	dir, err := os.Getwd()
	testutil.NoError(t, err)
	for {
		candidate := filepath.Join(dir, "tests", "contract", "fixtures", "sdk_contract", name)
		if _, err := os.Stat(filepath.Dir(candidate)); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate sdk contract fixture directory from %s", dir)
		}
		dir = parent
	}
}
