package realtime

import (
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
)

func realtimeHubTenantScope(claims *auth.Claims, schemaCache *schema.CacheHolder, activeSchema, tenantID string, tables map[string]bool, rlsFilteringAvailable bool) string {
	if subscriptionUsesRLSCandidateFanout(claims, schemaCache, activeSchema, tables, rlsFilteringAvailable) {
		return RLSFilteredTenantScope
	}
	return tenantID
}

func subscriptionUsesRLSCandidateFanout(claims *auth.Claims, schemaCache *schema.CacheHolder, activeSchema string, tables map[string]bool, rlsFilteringAvailable bool) bool {
	if claims == nil || !rlsFilteringAvailable || schemaCache == nil || len(tables) == 0 {
		return false
	}
	sc := schemaCache.Get()
	if sc == nil {
		return false
	}
	for table := range tables {
		if table == internalNotificationsTable {
			return false
		}
		tbl := sc.TableByNameInSchema(activeSchema, table)
		if tbl == nil || !tbl.RLSEnabled {
			return false
		}
	}
	return true
}
