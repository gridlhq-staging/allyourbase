//go:build multinode

package multinode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
)

func createSchemaPost(t *testing.T, nodes *twoNodeHarness, token, columnName, sentinel string) string {
	t.Helper()

	status, body := harnessHTTPJSON(t, http.MethodPost, nodes.nodeA.baseURL()+"/api/collections/"+pooledTenantPostsTable,
		map[string]any{columnName: sentinel}, token)
	if status != http.StatusCreated {
		t.Fatalf("create schema post status=%d body=%s\nnodes:\n%s", status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	assertSchemaRecordShape(t, body.object, columnName, sentinel)
	return fmt.Sprint(body.object["id"])
}

func createPeerOnlyPost(t *testing.T, nodes *twoNodeHarness, token, sentinel string) {
	t.Helper()
	status, body := harnessHTTPJSON(t, http.MethodPost, nodes.nodeA.baseURL()+"/api/collections/peer_only",
		map[string]any{"tenant_b_secret": sentinel}, token)
	if status != http.StatusCreated {
		t.Fatalf("create tenant B peer_only status=%d body=%s\nnodes:\n%s", status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	assertSchemaRecordShape(t, body.object, "tenant_b_secret", sentinel)
}

func deleteSchemaPostFromNodeB(t *testing.T, nodes *twoNodeHarness, token, id string) {
	t.Helper()

	status, body := harnessHTTPJSON(t, http.MethodDelete, nodes.nodeB.baseURL()+"/api/collections/"+pooledTenantPostsTable+"/"+id, nil, token)
	if status != http.StatusNoContent {
		t.Fatalf("delete schema post from node B id=%s status=%d body=%s\nnodes:\n%s",
			id, status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
}

func assertSchemaRecordShape(t *testing.T, record map[string]any, columnName, sentinel string) {
	t.Helper()
	if record[columnName] != sentinel {
		t.Fatalf("schema record %s=%v want %s in %#v", columnName, record[columnName], sentinel, record)
	}
	for _, forbidden := range []string{"public_note", "tenant_b_note", "tenant_b_secret"} {
		if forbidden == columnName {
			continue
		}
		if _, ok := record[forbidden]; ok {
			t.Fatalf("schema record exposed wrong-schema column %s in %#v", forbidden, record)
		}
	}
}

func assertSchemaPostRead(t *testing.T, nodes *twoNodeHarness, token, id, sentinel string) {
	t.Helper()
	status, body := harnessHTTPJSON(t, http.MethodGet, nodes.nodeB.baseURL()+"/api/collections/"+pooledTenantPostsTable+"/"+id, nil, token)
	if status != http.StatusOK {
		t.Fatalf("read schema post id=%s status=%d body=%s\nnodes:\n%s", id, status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	assertSchemaRecordShape(t, body.object, "tenant_a_note", sentinel)
}

func assertSchemaPostListContains(t *testing.T, nodes *twoNodeHarness, token, sentinel string) {
	t.Helper()
	status, body := harnessHTTPJSON(t, http.MethodGet, nodes.nodeB.baseURL()+"/api/collections/"+pooledTenantPostsTable+"?perPage=50", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list schema posts status=%d body=%s\nnodes:\n%s", status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	items, ok := body.object["items"].([]any)
	if !ok {
		t.Fatalf("list schema posts missing items array: %s", body)
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if ok && item["tenant_a_note"] == sentinel {
			assertSchemaRecordShape(t, item, "tenant_a_note", sentinel)
			return
		}
	}
	t.Fatalf("list schema posts did not include %q: %s", sentinel, body)
}

func assertSchemaCreateEvent(t *testing.T, nodes *twoNodeHarness, conn *websocket.Conn, columnName, sentinel string) {
	t.Helper()

	event, err := readHarnessWSUntil(t, conn, ws.MsgTypeEvent, 10*time.Second)
	if err != nil {
		t.Fatalf("schema create event %q not received: %v\nnodes:\n%s", sentinel, err, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if event.Action != "create" || event.Table != pooledTenantPostsTable {
		t.Fatalf("unexpected schema create event metadata: %#v sentinel=%s\nnodes:\n%s",
			event, sentinel, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	assertSchemaRecordShape(t, event.Record, columnName, sentinel)
}

func assertSchemaPostDeleted(t *testing.T, nodes *twoNodeHarness, token, id, sentinel string) {
	t.Helper()
	status, body := harnessHTTPJSON(t, http.MethodGet, nodes.nodeA.baseURL()+"/api/collections/"+pooledTenantPostsTable+"/"+id, nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("deleted schema post id=%s status=%d body=%s\nnodes:\n%s", id, status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	status, body = harnessHTTPJSON(t, http.MethodGet, nodes.nodeB.baseURL()+"/api/collections/"+pooledTenantPostsTable+"?perPage=50", nil, token)
	if status != http.StatusOK {
		t.Fatalf("list after schema delete status=%d body=%s\nnodes:\n%s", status, body, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if schemaListContains(body.object, "tenant_a_note", sentinel) {
		t.Fatalf("list after schema delete still contains %q: %s", sentinel, body)
	}
}

func schemaListContains(body map[string]any, columnName, sentinel string) bool {
	items, ok := body["items"].([]any)
	if !ok {
		return false
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if ok && item[columnName] == sentinel {
			return true
		}
	}
	return false
}

func assertHarnessGraphQLNext(
	t *testing.T,
	nodes *twoNodeHarness,
	conn *websocket.Conn,
	ref string,
	fieldName string,
	columnName string,
	sentinel string,
) {
	t.Helper()
	msg, err := readHarnessGraphQLWSUntil(t, conn, "next", 10*time.Second)
	if err != nil {
		t.Fatalf("GraphQL next %q not received: %v\nnodes:\n%s", sentinel, err, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	if msg.ID != ref {
		t.Fatalf("GraphQL next id=%q want %q payload=%s\nnodes:\n%s", msg.ID, ref, msg.Payload, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
	}
	row := graphQLPayloadRow(t, msg.Payload, fieldName)
	assertSchemaRecordShape(t, row, columnName, sentinel)
}

func assertNoHarnessGraphQLNext(
	t *testing.T,
	nodes *twoNodeHarness,
	conn *websocket.Conn,
	timeout time.Duration,
	message string,
) {
	t.Helper()
	msg, err := readHarnessGraphQLWSUntil(t, conn, "next", timeout)
	if err != nil {
		return
	}
	t.Fatalf("%s: id=%s payload=%s\nnodes:\n%s", message, msg.ID, msg.Payload, combinedNodeOutput(nodes.nodeA, nodes.nodeB))
}

func graphQLPayloadRow(t *testing.T, payload json.RawMessage, fieldName string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode GraphQL payload: %v payload=%s", err, payload)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("GraphQL payload missing data object: %s", payload)
	}
	row, ok := data[fieldName].(map[string]any)
	if !ok {
		t.Fatalf("GraphQL payload missing %s row: %s", fieldName, payload)
	}
	return row
}

func assertSchemaPeerOnlyFailsClosed(t *testing.T, fixture pooledTenantPostsHarness) {
	t.Helper()
	assertTenantAPeerOnlyCRUDFailsClosed(t, fixture)

	tenantAConn := dialHarnessRealtimeWS(t, fixture.nodes.nodeB, fixture.tenantA.accessToken)
	defer tenantAConn.Close()
	wsAccepted := subscribeHarnessTableIfAccepted(t, fixture.nodes, tenantAConn, "peer_only", "tenant-a-peer-only")

	tenantAGQL := dialHarnessGraphQLWS(t, fixture.nodes.nodeB, fixture.tenantA.accessToken)
	defer tenantAGQL.Close()
	subscribeHarnessGraphQLWS(t, tenantAGQL, "tenant-a-peer-gql", `subscription { peer_only { id tenant_b_secret } }`)
	msg, err := readHarnessGraphQLWSUntil(t, tenantAGQL, "error", 5*time.Second)
	if err != nil {
		t.Fatalf("tenant A peer_only GraphQL subscription was not rejected: %v\nnodes:\n%s",
			err, combinedNodeOutput(fixture.nodes.nodeA, fixture.nodes.nodeB))
	}
	if msg.ID != "tenant-a-peer-gql" {
		t.Fatalf("tenant A peer_only GraphQL error id=%q payload=%s", msg.ID, msg.Payload)
	}
	assertGraphQLErrorContains(t, msg.Payload, "unknown subscription table")

	sentinel := "tenant-b-peer-only-" + randomHex(t, 6)
	createPeerOnlyPost(t, fixture.nodes, fixture.tenantB.accessToken, sentinel)
	if wsAccepted {
		assertNoPooledEvent(t, fixture.nodes, tenantAConn, time.Second, "tenant A received tenant B peer_only event")
	}
}

func assertTenantAPeerOnlyCRUDFailsClosed(t *testing.T, fixture pooledTenantPostsHarness) {
	t.Helper()
	status, body := harnessHTTPJSON(t, http.MethodPost, fixture.nodes.nodeA.baseURL()+"/api/collections/peer_only",
		map[string]any{"tenant_b_secret": "tenant-a-peer-only-probe-" + randomHex(t, 6)}, fixture.tenantA.accessToken)
	if status != http.StatusNotFound {
		t.Fatalf("tenant A peer_only create status=%d body=%s want=%d\nnodes:\n%s",
			status, body, http.StatusNotFound, combinedNodeOutput(fixture.nodes.nodeA, fixture.nodes.nodeB))
	}
	status, body = harnessHTTPJSON(t, http.MethodGet, fixture.nodes.nodeB.baseURL()+"/api/collections/peer_only?perPage=50",
		nil, fixture.tenantA.accessToken)
	if status != http.StatusNotFound {
		t.Fatalf("tenant A peer_only list status=%d body=%s want=%d\nnodes:\n%s",
			status, body, http.StatusNotFound, combinedNodeOutput(fixture.nodes.nodeA, fixture.nodes.nodeB))
	}
}

func assertGraphQLErrorContains(t *testing.T, payload json.RawMessage, want string) {
	t.Helper()
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode GraphQL error payload: %v payload=%s", err, payload)
	}
	for _, gqlErr := range envelope.Errors {
		if strings.Contains(gqlErr.Message, want) {
			return
		}
	}
	t.Fatalf("GraphQL error payload %s did not contain %q", payload, want)
}
