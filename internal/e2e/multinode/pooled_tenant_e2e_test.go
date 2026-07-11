//go:build multinode

package multinode

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pooledTenantPostsTable = "posts"

type pooledTenantPostsHarness struct {
	nodes   *twoNodeHarness
	tenantA harnessAuthTokens
	tenantB harnessAuthTokens
}

func TestCrossNodeSchemaIsolatedRealtimeAndCRUDIsolation(t *testing.T) {
	fixture := setupSchemaTenantPostsHarness(t)
	assertSchemaPeerOnlyFailsClosed(t, fixture)

	tenantBConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantB.accessToken)
	defer tenantBConn.Close()
	subscribeHarnessPosts(t, fixture.nodes, tenantBConn, "tenant-b-sub")

	tenantAConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantA.accessToken)
	defer tenantAConn.Close()
	subscribeHarnessPosts(t, fixture.nodes, tenantAConn, "tenant-a-sub")

	sentinel := "tenant-a-schema-create-" + randomHex(t, 6)
	rowID := createSchemaPost(t, fixture.nodes, fixture.tenantA.accessToken, "tenant_a_note", sentinel)
	assertSchemaPostRead(t, fixture.nodes, fixture.tenantA.accessToken, rowID, sentinel)
	assertSchemaPostListContains(t, fixture.nodes, fixture.tenantA.accessToken, sentinel)
	assertSchemaCreateEvent(t, fixture.nodes, tenantAConn, "tenant_a_note", sentinel)
	assertNoPooledEvent(t, fixture.nodes, tenantBConn, time.Second, "tenant B received tenant A create event")

	deleteSchemaPostFromNodeB(t, fixture.nodes, fixture.tenantA.accessToken, rowID)
	assertSchemaPostDeleted(t, fixture.nodes, fixture.tenantA.accessToken, rowID, sentinel)

	tenantAGQL := dialHarnessGraphQLWS(t, fixture.nodes.nodeB, fixture.tenantA.accessToken)
	defer tenantAGQL.Close()
	subscribeHarnessGraphQLWS(t, tenantAGQL, "tenant-a-gql", `subscription { posts { id tenant_a_note } }`)

	tenantBGQL := dialHarnessGraphQLWS(t, fixture.nodes.nodeB, fixture.tenantB.accessToken)
	defer tenantBGQL.Close()
	subscribeHarnessGraphQLWS(t, tenantBGQL, "tenant-b-gql", `subscription { posts { id tenant_b_note } }`)

	gqlSentinel := "tenant-a-schema-gql-" + randomHex(t, 6)
	createSchemaPost(t, fixture.nodes, fixture.tenantA.accessToken, "tenant_a_note", gqlSentinel)
	assertNoPooledEvent(t, fixture.nodes, tenantBConn, time.Second, "tenant B received tenant A GraphQL probe event")
	assertNoHarnessGraphQLNext(t, fixture.nodes, tenantBGQL, time.Second, "tenant B received tenant A GraphQL event")
	assertHarnessGraphQLNext(t, fixture.nodes, tenantAGQL, "tenant-a-gql", "posts", "tenant_a_note", gqlSentinel)
}

func TestCrossNodePooledDeleteVisibilityRespectsRLS(t *testing.T) {
	fixture := setupPooledTenantPostsHarness(t)

	assertDeleteIsolationCheckIsSensitive(t, fixture)

	rowID := createPooledPost(t, fixture.nodes, fixture.tenantA.accessToken, "tenant-a-delete-"+randomHex(t, 6))

	tenantAConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantA.accessToken)
	defer tenantAConn.Close()
	subscribeHarnessPosts(t, fixture.nodes, tenantAConn, "tenant-a-delete-sub")

	tenantBConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantB.accessToken)
	defer tenantBConn.Close()
	subscribeHarnessPosts(t, fixture.nodes, tenantBConn, "tenant-b-delete-sub")

	assertDeleteAssertionRejectsBogusID(t)
	deletePooledPost(t, fixture.nodes, fixture.tenantA.accessToken, rowID)
	assertPooledDeleteEvent(t, fixture.nodes, tenantAConn, rowID)
	assertNoPooledEvent(t, fixture.nodes, tenantBConn, time.Second, "tenant B received tenant A delete event")
}

func setupPooledTenantPostsHarness(t *testing.T) pooledTenantPostsHarness {
	t.Helper()

	databaseURL := requirePooledTenantDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	seedPooledTenantPostsTable(ctx, t, databaseURL)

	nodes := bootTwoNodeHarness(t)
	tenantA := registerHarnessUser(t, nodes.nodeA.baseURL(), "pooled-tenant-a-"+randomHex(t, 4)+"@example.com")
	tenantB := registerHarnessUser(t, nodes.nodeA.baseURL(), "pooled-tenant-b-"+randomHex(t, 4)+"@example.com")
	configureHarnessUsersAsSharedTenants(ctx, t, databaseURL, tenantA.userID, tenantB.userID)
	createPooledPost(t, nodes, tenantA.accessToken, "tenant-a-setup-"+randomHex(t, 6))
	createPooledPost(t, nodes, tenantB.accessToken, "tenant-b-setup-"+randomHex(t, 6))

	return pooledTenantPostsHarness{nodes: nodes, tenantA: tenantA, tenantB: tenantB}
}

func setupSchemaTenantPostsHarness(t *testing.T) pooledTenantPostsHarness {
	t.Helper()

	databaseURL := requirePooledTenantDatabaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	seedPooledTenantPublicPostsTable(ctx, t, databaseURL)

	nodes := bootTwoNodeHarness(t, map[string]string{"AYB_GRAPHQL_ENABLED": "true"})
	tenantA := registerHarnessUser(t, nodes.nodeA.baseURL(), "schema-tenant-a-"+randomHex(t, 4)+"@example.com")
	tenantB := registerHarnessUser(t, nodes.nodeA.baseURL(), "schema-tenant-b-"+randomHex(t, 4)+"@example.com")
	tenantARecord, tenantBRecord := configureHarnessUsersAsSchemaTenants(ctx, t, databaseURL, tenantA.userID, tenantB.userID)
	seedSchemaTenantPostsTables(ctx, t, databaseURL, tenantARecord.slug, tenantBRecord.slug)
	for _, node := range []*harnessNode{nodes.nodeA, nodes.nodeB} {
		waitForHarnessSchemaTableColumn(t, node, tenantA.accessToken, tenantARecord.slug, "posts", "tenant_a_note", 30*time.Second)
		waitForHarnessSchemaTableColumn(t, node, tenantB.accessToken, tenantBRecord.slug, "peer_only", "tenant_b_secret", 30*time.Second)
	}

	return pooledTenantPostsHarness{nodes: nodes, tenantA: tenantA, tenantB: tenantB}
}

func requirePooledTenantDatabaseURL(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for pooled multinode tenant tests; run through internal/testutil/cmd/testpg")
	}
	return databaseURL
}

func seedPooledTenantPostsTable(ctx context.Context, t *testing.T, databaseURL string) {
	t.Helper()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for pooled posts seed: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ayb_authenticated') THEN
				CREATE ROLE ayb_authenticated NOLOGIN;
			END IF;
		END $$;
		DROP TABLE IF EXISTS posts;
		CREATE TABLE posts (
			id BIGSERIAL PRIMARY KEY,
			tenant_id UUID NOT NULL DEFAULT current_setting('ayb.tenant_id', true)::uuid,
			sentinel TEXT NOT NULL
		);
		ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
		CREATE POLICY posts_tenant_isolation ON posts
			USING (tenant_id = current_setting('ayb.tenant_id', true)::uuid)
			WITH CHECK (tenant_id = current_setting('ayb.tenant_id', true)::uuid);
		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON posts TO ayb_authenticated;
		GRANT USAGE, SELECT ON SEQUENCE posts_id_seq TO ayb_authenticated;
	`)
	if err != nil {
		t.Fatalf("seed pooled tenant posts table: %v", err)
	}
}

func seedPooledTenantPublicPostsTable(ctx context.Context, t *testing.T, databaseURL string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for public posts seed: %v", err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ayb_authenticated') THEN
				CREATE ROLE ayb_authenticated NOLOGIN;
			END IF;
		END $$;
		DROP TABLE IF EXISTS public.posts CASCADE;
		CREATE TABLE public.posts (id BIGSERIAL PRIMARY KEY, public_note TEXT NOT NULL);
		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON public.posts TO ayb_authenticated;
		GRANT USAGE, SELECT ON SEQUENCE public.posts_id_seq TO ayb_authenticated;
	`)
	if err != nil {
		t.Fatalf("seed public divergent posts table: %v", err)
	}
}

func configureHarnessUsersAsSharedTenants(ctx context.Context, t *testing.T, databaseURL string, userIDs ...string) {
	t.Helper()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for tenant mode setup: %v", err)
	}
	defer pool.Close()

	for _, userID := range userIDs {
		tenantRecord := configureHarnessUserTenantMode(ctx, t, pool, userID, "shared")
		if tenantRecord.id == "" {
			t.Fatalf("user %s did not resolve to a tenant", userID)
		}
	}
}

func configureHarnessUserAsSharedTenant(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	return configureHarnessUserTenantMode(ctx, t, pool, userID, "shared").id
}

type harnessTenantRecord struct {
	id   string
	slug string
}

func configureHarnessUsersAsSchemaTenants(
	ctx context.Context,
	t *testing.T,
	databaseURL string,
	userAID string,
	userBID string,
) (harnessTenantRecord, harnessTenantRecord) {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for schema tenant setup: %v", err)
	}
	defer pool.Close()
	return configureHarnessUserTenantMode(ctx, t, pool, userAID, "schema"),
		configureHarnessUserTenantMode(ctx, t, pool, userBID, "schema")
}

func configureHarnessUserTenantMode(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	userID string,
	isolationMode string,
) harnessTenantRecord {
	t.Helper()
	var record harnessTenantRecord
	err := pool.QueryRow(ctx, `
		UPDATE _ayb_tenants AS tenants
		   SET isolation_mode = $2, state = 'active', updated_at = NOW()
		  FROM _ayb_tenant_memberships AS memberships
		 WHERE memberships.tenant_id = tenants.id
		   AND memberships.user_id = $1
		 RETURNING tenants.id::text, tenants.slug
	`, userID, isolationMode).Scan(&record.id, &record.slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return harnessTenantRecord{}
		}
		t.Fatalf("configure user %s tenant as %s: %v", userID, isolationMode, err)
	}
	return record
}

func seedSchemaTenantPostsTables(ctx context.Context, t *testing.T, databaseURL, tenantASlug, tenantBSlug string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL for schema posts seed: %v", err)
	}
	defer pool.Close()
	for _, slug := range []string{tenantASlug, tenantBSlug} {
		schemaIdent := pgx.Identifier{slug}.Sanitize()
		if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, schemaIdent)); err != nil {
			t.Fatalf("create tenant schema %s: %v", slug, err)
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO ayb_authenticated`, schemaIdent)); err != nil {
			t.Fatalf("grant tenant schema %s: %v", slug, err)
		}
	}
	seedDivergentSchemaTable(ctx, t, pool, tenantASlug, "tenant_a_note")
	seedDivergentSchemaTable(ctx, t, pool, tenantBSlug, "tenant_b_note")
	seedDivergentSchemaTable(ctx, t, pool, tenantBSlug, "tenant_b_secret", "peer_only")
}

func seedDivergentSchemaTable(ctx context.Context, t *testing.T, pool *pgxpool.Pool, slug, noteColumn string, names ...string) {
	t.Helper()
	tableName := "posts"
	if len(names) > 0 {
		tableName = names[0]
	}
	schemaIdent := pgx.Identifier{slug}.Sanitize()
	tableIdent := pgx.Identifier{tableName}.Sanitize()
	qualified := schemaIdent + "." + tableIdent
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		DROP TABLE IF EXISTS %s CASCADE;
		CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, %s TEXT NOT NULL);
		GRANT SELECT, INSERT, UPDATE, DELETE ON %s TO ayb_authenticated;
		GRANT USAGE, SELECT ON SEQUENCE %s.%s TO ayb_authenticated;
	`, qualified, qualified, pgx.Identifier{noteColumn}.Sanitize(), qualified, schemaIdent, tableName+"_id_seq"))
	if err != nil {
		t.Fatalf("seed schema table %s.%s: %v", slug, tableName, err)
	}
}

func assertCreateIsolationCheckIsSensitive(t *testing.T, fixture pooledTenantPostsHarness) {
	t.Helper()

	wrongTenantBConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantA.accessToken)
	defer wrongTenantBConn.Close()
	subscribeHarnessPosts(t, fixture.nodes, wrongTenantBConn, "tenant-b-wrong-token-sub")

	probeSentinel := "wrong-token-create-probe-" + randomHex(t, 6)
	createPooledPost(t, fixture.nodes, fixture.tenantA.accessToken, probeSentinel)
	assertPooledCreateEvent(t, fixture.nodes, wrongTenantBConn, probeSentinel)
}

func assertDeleteIsolationCheckIsSensitive(t *testing.T, fixture pooledTenantPostsHarness) {
	t.Helper()

	probeID := createPooledPost(t, fixture.nodes, fixture.tenantA.accessToken, "wrong-token-delete-probe-"+randomHex(t, 6))
	wrongTenantAConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantB.accessToken)
	defer wrongTenantAConn.Close()
	subscribeHarnessPosts(t, fixture.nodes, wrongTenantAConn, "tenant-a-wrong-token-delete-sub")

	deletePooledPost(t, fixture.nodes, fixture.tenantA.accessToken, probeID)
	assertNoPooledEvent(t, fixture.nodes, wrongTenantAConn, time.Second, "wrong-token tenant A delete assertion received an event")
}

func subscribeHarnessPosts(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, ref string) {
	t.Helper()
	subscribeHarnessTable(t, nodes, conn, pooledTenantPostsTable, ref)
}

func subscribeHarnessTable(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, tableName, ref string) {
	t.Helper()
	writeHarnessWSJSON(t, conn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{tableName},
		Ref:    ref,
	})
	reply, err := readHarnessWSUntil(t, conn, ws.MsgTypeReply, 5*time.Second)
	if err != nil {
		t.Fatalf("subscribe %s failed: %v\nnodes:\n%s", ref, err, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if reply.Status != "ok" || reply.Ref != ref {
		t.Fatalf("unexpected subscribe reply for %s: %#v\nnodes:\n%s", ref, reply, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
}

func subscribeHarnessTableIfAccepted(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, tableName, ref string) bool {
	t.Helper()
	writeHarnessWSJSON(t, conn, ws.ClientMessage{
		Type:   ws.MsgTypeSubscribe,
		Tables: []string{tableName},
		Ref:    ref,
	})
	reply, err := readHarnessWSUntil(t, conn, ws.MsgTypeReply, 5*time.Second)
	if err != nil {
		t.Fatalf("subscribe %s failed: %v\nnodes:\n%s", ref, err, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if reply.Ref != ref {
		t.Fatalf("unexpected subscribe reply ref for %s: %#v\nnodes:\n%s", ref, reply, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	switch reply.Status {
	case "ok":
		return true
	case "error":
		return false
	default:
		t.Fatalf("unexpected subscribe status for %s: %#v\nnodes:\n%s", ref, reply, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
		return false
	}
}

func createPooledPost(t *testing.T, nodes *twoNodeHarness, token, sentinel string) string {
	t.Helper()

	status, body := harnessHTTPJSON(t, http.MethodPost, nodes.nodeA.baseURL()+"/api/collections/"+pooledTenantPostsTable,
		map[string]any{"sentinel": sentinel}, token)
	if status != http.StatusCreated {
		t.Fatalf("create pooled post status=%d body=%s\nnodes:\n%s", status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	return fmt.Sprint(body.object["id"])
}

func deletePooledPost(t *testing.T, nodes *twoNodeHarness, token, id string) {
	t.Helper()

	status, body := harnessHTTPJSON(t, http.MethodDelete, nodes.nodeA.baseURL()+"/api/collections/"+pooledTenantPostsTable+"/"+id, nil, token)
	if status != http.StatusNoContent {
		t.Fatalf("delete pooled post id=%s status=%d body=%s\nnodes:\n%s", id, status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
}

func assertPooledCreateEvent(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, sentinel string) {
	t.Helper()

	event, err := readHarnessWSUntil(t, conn, ws.MsgTypeEvent, 10*time.Second)
	if err != nil {
		t.Fatalf("pooled create event %q not received: %v\nnodes:\n%s", sentinel, err, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if event.Action != "create" || event.Table != pooledTenantPostsTable || event.Record["sentinel"] != sentinel {
		t.Fatalf("unexpected pooled create event: %#v sentinel=%s\nnodes:\n%s", event, sentinel, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
}

func assertPooledDeleteEvent(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, id string) {
	t.Helper()

	event, err := readHarnessWSUntil(t, conn, ws.MsgTypeEvent, 10*time.Second)
	if err != nil {
		t.Fatalf("pooled delete event %q not received: %v\nnodes:\n%s", id, err, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if !pooledDeleteEventMatches(event, id) {
		t.Fatalf("unexpected pooled delete event: %#v id=%s\nnodes:\n%s", event, id, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
}

func assertNoPooledEvent(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, timeout time.Duration, message string) {
	t.Helper()

	event, err := readHarnessWSUntil(t, conn, ws.MsgTypeEvent, timeout)
	if err != nil {
		return
	}
	t.Fatalf("%s: %#v\nnodes:\n%s", message, event, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
}

func assertDeleteAssertionRejectsBogusID(t *testing.T) {
	t.Helper()

	event := ws.ServerMessage{
		Type:   ws.MsgTypeEvent,
		Action: "delete",
		Table:  pooledTenantPostsTable,
		Record: map[string]any{"id": "actual-id"},
	}
	if pooledDeleteEventMatches(event, "bogus-id") {
		t.Fatal("delete event assertion accepted a bogus id")
	}
}

func pooledDeleteEventMatches(event ws.ServerMessage, id string) bool {
	return event.Action == "delete" &&
		event.Table == pooledTenantPostsTable &&
		event.Record["id"] == id
}
