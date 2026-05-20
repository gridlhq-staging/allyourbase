package tenant

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// fetchLegacyAppRows loads all apps joined with their owner's email.
func (ms *MigrationService) fetchLegacyAppRows(ctx context.Context) ([]legacyAppRow, error) {
	rows, err := ms.pool.Query(ctx,
		`SELECT a.id, a.tenant_id, a.owner_user_id, COALESCE(u.email, '')
		 FROM _ayb_apps a
		 LEFT JOIN _ayb_users u ON u.id = a.owner_user_id
		 ORDER BY a.owner_user_id, a.created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying legacy apps: %w", err)
	}
	defer rows.Close()

	var out []legacyAppRow
	for rows.Next() {
		var r legacyAppRow
		if err := rows.Scan(&r.AppID, &r.AppTenantID, &r.OwnerUserID, &r.OwnerEmail); err != nil {
			return nil, fmt.Errorf("scanning legacy app row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// fetchExistingSlugs returns a set of all slug values currently in _ayb_tenants.
func (ms *MigrationService) fetchExistingSlugs(ctx context.Context) (map[string]bool, error) {
	rows, err := ms.pool.Query(ctx, `SELECT slug FROM _ayb_tenants`)
	if err != nil {
		return nil, fmt.Errorf("querying existing slugs: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("scanning slug: %w", err)
		}
		out[slug] = true
	}
	return out, rows.Err()
}

// fetchMigrationTenantsByOwner returns a map of owner_user_id -> tenant_id for
// tenants already created by a previous migration run.
func (ms *MigrationService) fetchMigrationTenantsByOwner(ctx context.Context) (map[string]string, error) {
	rows, err := ms.pool.Query(ctx,
		`SELECT idempotency_key, id FROM _ayb_tenants WHERE idempotency_key LIKE 'miglegacy:%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying migration tenants: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var idemKey, tenantID string
		if err := rows.Scan(&idemKey, &tenantID); err != nil {
			return nil, fmt.Errorf("scanning migration tenant: %w", err)
		}
		ownerID := strings.TrimPrefix(idemKey, "miglegacy:")
		out[ownerID] = tenantID
	}
	return out, rows.Err()
}

// migrationIdempotencyKey returns the stable key used to identify migration-created tenants.
func migrationIdempotencyKey(ownerUserID string) string {
	return "miglegacy:" + ownerUserID
}

// groupAppsByOwner groups legacy app rows by owner_user_id, returning groups
// sorted deterministically by owner_user_id. This is the canonical grouping
// algorithm used by both dry-run and apply modes.
func groupAppsByOwner(rows []legacyAppRow) []ownerGroup {
	if len(rows) == 0 {
		return nil
	}

	ownerOrder := make([]string, 0)
	byOwner := make(map[string]*ownerGroup)

	for _, r := range rows {
		if _, seen := byOwner[r.OwnerUserID]; !seen {
			ownerOrder = append(ownerOrder, r.OwnerUserID)
			byOwner[r.OwnerUserID] = &ownerGroup{
				OwnerUserID: r.OwnerUserID,
				OwnerEmail:  r.OwnerEmail,
			}
		}
		g := byOwner[r.OwnerUserID]
		if r.AppTenantID != nil {
			g.AlreadyMigrated = append(g.AlreadyMigrated, r.AppID)
			continue
		}
		g.AppIDs = append(g.AppIDs, r.AppID)
	}

	sort.Strings(ownerOrder)

	groups := make([]ownerGroup, len(ownerOrder))
	for i, ownerID := range ownerOrder {
		groups[i] = *byOwner[ownerID]
	}
	return groups
}

// slugRe matches sequences of non-slug characters (anything that's not
// lowercase alphanumeric).
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// deriveSlug computes a URL-safe slug from an owner's email address.
func deriveSlug(email string) string {
	local := email
	if idx := strings.Index(email, "@"); idx >= 0 {
		local = email[:idx]
	}
	local = strings.ToLower(local)
	slug := slugRe.ReplaceAllString(local, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "tenant"
	}
	return slug
}

// resolveSlugWithCollisions appends numeric suffixes (-1, -2, ...) until it
// finds a slug not present in existing. The caller must add the returned slug
// to existing to prevent the same value from being reused.
func resolveSlugWithCollisions(base string, existing map[string]bool) string {
	if !existing[base] {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

// classifyGroup determines the dry-run/apply action for an owner group.
func classifyGroup(g ownerGroup, existingSlugs map[string]bool) (action, conflict string) {
	if g.ExistingTenantID != "" {
		return dryRunActionReuse, ""
	}
	if g.OwnerEmail == "" {
		return dryRunActionSkip, dryRunConflictMissingOwner
	}
	if len(g.AppIDs) == 0 {
		return dryRunActionSkip, dryRunConflictNoApps
	}
	base := deriveSlug(g.OwnerEmail)
	if base == "" {
		return dryRunActionSkip, dryRunConflictInvalidSlug
	}
	resolved := resolveSlugWithCollisions(base, existingSlugs)
	if resolved != base {
		return dryRunActionCreate, dryRunConflictSlugCollision
	}
	return dryRunActionCreate, ""
}

// proposeDryRunSlug derives a preview slug and claims it only for create actions.
// Reuse actions can display a derived slug for visibility but must not reserve it.
func proposeDryRunSlug(g ownerGroup, action string, slugsSeen map[string]bool) string {
	if action == dryRunActionSkip {
		return ""
	}
	base := deriveSlug(g.OwnerEmail)
	slug := resolveSlugWithCollisions(base, slugsSeen)
	if action == dryRunActionCreate {
		slugsSeen[slug] = true
	}
	return slug
}

// Ensure pgx is used (Row scanning in fetchLegacyAppRows).
var _ pgx.Row
