package realtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseRealtimeTableSubscriptionsUsesActiveSchema(t *testing.T) {
	t.Parallel()

	h := &Handler{schemaCache: activeSchemaRealtimeCache()}
	w := httptest.NewRecorder()

	tables, ok := h.parseRealtimeTableSubscriptions(w, "tenant_a", "items")

	testutil.True(t, ok, "tenant_a.items should be accepted")
	testutil.True(t, tables["items"], "items subscription should be returned")
	testutil.Equal(t, http.StatusOK, w.Code)
}

func TestParseRealtimeTableSubscriptionsRejectsPeerOnlyTable(t *testing.T) {
	t.Parallel()

	h := &Handler{schemaCache: activeSchemaRealtimeCache()}
	w := httptest.NewRecorder()

	_, ok := h.parseRealtimeTableSubscriptions(w, "tenant_a", "peer_only")

	testutil.False(t, ok, "tenant_a must not accept tenant_b-only tables")
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "unknown table")
}

func activeSchemaRealtimeCache() *schema.CacheHolder {
	ch := schema.NewCacheHolder(nil, testutil.DiscardLogger())
	ch.SetForTesting(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"tenant_a.items":     {Schema: "tenant_a", Name: "items", Kind: "table"},
			"tenant_b.items":     {Schema: "tenant_b", Name: "items", Kind: "table"},
			"tenant_b.peer_only": {Schema: "tenant_b", Name: "peer_only", Kind: "table"},
		},
	})
	return ch
}
