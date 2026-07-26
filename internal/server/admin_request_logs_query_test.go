package server

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestBuildPathLikeClauseEscapesLiteralLikeChars(t *testing.T) {
	t.Parallel()

	clause, arg, next := buildPathLikeClause("/api/user_profiles%v1", 3)
	if clause != "path LIKE $3 ESCAPE '\\'" {
		t.Fatalf("unexpected clause: %q", clause)
	}
	if arg != "/api/user\\_profiles\\%v1" {
		t.Fatalf("unexpected arg: %q", arg)
	}
	if next != 4 {
		t.Fatalf("unexpected next arg pos: %d", next)
	}
}

func TestBuildPathLikeClauseSupportsStarWildcard(t *testing.T) {
	t.Parallel()

	clause, arg, next := buildPathLikeClause("/api/collections/*", 1)
	if clause != "path LIKE $1 ESCAPE '\\'" {
		t.Fatalf("unexpected clause: %q", clause)
	}
	if arg != "/api/collections/%" {
		t.Fatalf("unexpected arg: %q", arg)
	}
	if next != 2 {
		t.Fatalf("unexpected next arg pos: %d", next)
	}
}

func TestBuildAdminRequestLogsQueriesShareOrderedFilterPredicate(t *testing.T) {
	t.Parallel()

	minDuration := int64(25)
	maxDuration := int64(250)
	fromTime := time.Date(2026, 3, 1, 1, 2, 3, 0, time.UTC)
	toTime := time.Date(2026, 3, 2, 4, 5, 6, 0, time.UTC)
	filters := adminRequestLogFilters{
		method:         "POST",
		path:           "/api/orders_*",
		statusCode:     201,
		statusClassMin: 200,
		statusClassMax: 299,
		minDurationMS:  &minDuration,
		maxDurationMS:  &maxDuration,
		limit:          25,
		offset:         50,
		fromTime:       fromTime,
		toTime:         toTime,
	}
	wantPredicate := `WHERE method = $1 AND path LIKE $2 ESCAPE '\' AND status_code = $3 AND status_code >= $4 AND status_code <= $5 AND duration_ms >= $6 AND duration_ms <= $7 AND timestamp >= $8 AND timestamp <= $9`
	wantFilterArgs := []any{
		"POST",
		"/api/orders\\_%",
		201,
		200,
		299,
		int64(25),
		int64(250),
		fromTime,
		toTime,
	}

	pageSQL, pageArgs := buildAdminRequestLogsQuery(filters)
	countSQL, countArgs := buildAdminRequestLogsCountQuery(filters)

	if !strings.Contains(pageSQL, wantPredicate) {
		t.Fatalf("page query missing shared predicate:\n%s", pageSQL)
	}
	if !strings.Contains(pageSQL, "ORDER BY timestamp DESC, id DESC LIMIT $10 OFFSET $11") {
		t.Fatalf("page query missing stable pagination placeholders:\n%s", pageSQL)
	}
	if !reflect.DeepEqual(pageArgs, append(append([]any{}, wantFilterArgs...), 25, 50)) {
		t.Fatalf("unexpected page args: %#v", pageArgs)
	}
	if !strings.Contains(countSQL, "SELECT COUNT(*) FROM _ayb_request_logs") {
		t.Fatalf("unexpected count query projection:\n%s", countSQL)
	}
	if !strings.Contains(countSQL, wantPredicate) {
		t.Fatalf("count query missing shared predicate:\n%s", countSQL)
	}
	for _, forbidden := range []string{"ORDER BY", "LIMIT", "OFFSET"} {
		if strings.Contains(countSQL, forbidden) {
			t.Fatalf("count query contains %q:\n%s", forbidden, countSQL)
		}
	}
	if !reflect.DeepEqual(countArgs, wantFilterArgs) {
		t.Fatalf("unexpected count args: %#v", countArgs)
	}
}

func TestBuildAdminRequestLogsQueryUsesStableCursorAfterSharedTimestamp(t *testing.T) {
	t.Parallel()

	cursorTimestamp := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	filters := adminRequestLogFilters{
		limit:           500,
		cursorTimestamp: cursorTimestamp,
		cursorID:        "00000000-0000-0000-0000-000000000002",
	}

	pageSQL, pageArgs := buildAdminRequestLogsQuery(filters)

	if !strings.Contains(pageSQL, "WHERE (timestamp, id) < ($1, $2)") {
		t.Fatalf("page query missing stable cursor predicate:\n%s", pageSQL)
	}
	if !strings.Contains(pageSQL, "ORDER BY timestamp DESC, id DESC LIMIT $3 OFFSET $4") {
		t.Fatalf("page query missing stable cursor ordering:\n%s", pageSQL)
	}
	wantArgs := []any{
		cursorTimestamp,
		"00000000-0000-0000-0000-000000000002",
		500,
		0,
	}
	if !reflect.DeepEqual(pageArgs, wantArgs) {
		t.Fatalf("unexpected page args: %#v", pageArgs)
	}
}

func TestBuildAdminRequestLogsCountQueryUsesDateOnlyExclusiveUpperBound(t *testing.T) {
	t.Parallel()

	toTime := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	countSQL, countArgs := buildAdminRequestLogsCountQuery(adminRequestLogFilters{
		toTime:     toTime,
		toDateOnly: true,
	})

	if !strings.Contains(countSQL, "WHERE timestamp < $1") {
		t.Fatalf("date-only count query should use an exclusive upper bound:\n%s", countSQL)
	}
	wantArgs := []any{toTime.Add(24 * time.Hour)}
	if !reflect.DeepEqual(countArgs, wantArgs) {
		t.Fatalf("unexpected count args: %#v", countArgs)
	}
}

func TestAdminRequestLogsReadTransactionOptionsUseOneSnapshot(t *testing.T) {
	t.Parallel()

	options := adminRequestLogsReadTransactionOptions()

	if options.IsoLevel != pgx.RepeatableRead {
		t.Fatalf("request-log reads must use one transaction snapshot: got isolation %q", options.IsoLevel)
	}
	if options.AccessMode != pgx.ReadOnly {
		t.Fatalf("request-log reads should not open a write transaction: got access mode %q", options.AccessMode)
	}
}
