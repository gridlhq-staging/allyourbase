package server

import (
	"net/url"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseAdminRequestLogFiltersValid(t *testing.T) {
	t.Parallel()

	query := url.Values{
		"method": {" post "},
		"path":   {" /api/* "},
		"status": {"201"},
		"limit":  {"600"},
		"offset": {"-1"},
		"from":   {"2026-03-01"},
		"to":     {"2026-03-02"},
	}

	filters, badRequestMessage := parseAdminRequestLogFilters(query)
	testutil.Equal(t, "", badRequestMessage)
	testutil.Equal(t, "POST", filters.method)
	testutil.Equal(t, "/api/*", filters.path)
	testutil.Equal(t, 201, filters.statusCode)
	testutil.Equal(t, 500, filters.limit)
	testutil.Equal(t, 0, filters.offset)
	testutil.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), filters.fromTime)
	testutil.Equal(t, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), filters.toTime)
	testutil.True(t, filters.toDateOnly, "date-only to filter should be tracked for exclusive upper bound")
}

func TestParseAdminRequestLogFiltersAcceptsStableCursor(t *testing.T) {
	t.Parallel()

	filters, badRequestMessage := parseAdminRequestLogFilters(url.Values{
		"cursor_timestamp": {"2026-07-26T11:00:00.123Z"},
		"cursor_id":        {"00000000-0000-0000-0000-000000000002"},
	})

	testutil.Equal(t, "", badRequestMessage)
	testutil.Equal(
		t,
		time.Date(2026, 7, 26, 11, 0, 0, 123000000, time.UTC),
		filters.cursorTimestamp,
	)
	testutil.Equal(t, "00000000-0000-0000-0000-000000000002", filters.cursorID)
}

func TestParseAdminRequestLogFiltersRejectsIncompleteOrInvalidCursor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   url.Values
		wantErr string
	}{
		{
			name:    "timestamp without id",
			query:   url.Values{"cursor_timestamp": {"2026-07-26T11:00:00Z"}},
			wantErr: "cursor_timestamp and cursor_id must be provided together",
		},
		{
			name:    "id without timestamp",
			query:   url.Values{"cursor_id": {"00000000-0000-0000-0000-000000000002"}},
			wantErr: "cursor_timestamp and cursor_id must be provided together",
		},
		{
			name: "invalid timestamp",
			query: url.Values{
				"cursor_timestamp": {"yesterday"},
				"cursor_id":        {"00000000-0000-0000-0000-000000000002"},
			},
			wantErr: "invalid cursor_timestamp; expected RFC3339",
		},
		{
			name: "invalid id",
			query: url.Values{
				"cursor_timestamp": {"2026-07-26T11:00:00Z"},
				"cursor_id":        {"not-a-uuid"},
			},
			wantErr: "invalid cursor_id; expected UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, badRequestMessage := parseAdminRequestLogFilters(tt.query)

			testutil.Equal(t, tt.wantErr, badRequestMessage)
		})
	}
}

func TestParseAdminRequestLogFiltersRejectsStatusOutsideRange(t *testing.T) {
	t.Parallel()

	filters, badRequestMessage := parseAdminRequestLogFilters(url.Values{"status": {"99"}})
	testutil.Equal(t, "", filters.method)
	testutil.Equal(t, "invalid status; must be an integer 100–599", badRequestMessage)
}

func TestParseAdminRequestLogFiltersAcceptsStatusClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusClass string
		wantMin     int
		wantMax     int
	}{
		{name: "success", statusClass: "2xx", wantMin: 200, wantMax: 299},
		{name: "redirect", statusClass: "3xx", wantMin: 300, wantMax: 399},
		{name: "client error", statusClass: "4xx", wantMin: 400, wantMax: 499},
		{name: "server error", statusClass: "5xx", wantMin: 500, wantMax: 599},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters, badRequestMessage := parseAdminRequestLogFilters(url.Values{
				"status_class": {tt.statusClass},
			})

			testutil.Equal(t, "", badRequestMessage)
			testutil.Equal(t, tt.wantMin, filters.statusClassMin)
			testutil.Equal(t, tt.wantMax, filters.statusClassMax)
		})
	}
}

func TestParseAdminRequestLogFiltersAcceptsInclusiveDurationBounds(t *testing.T) {
	t.Parallel()

	filters, badRequestMessage := parseAdminRequestLogFilters(url.Values{
		"min_duration_ms": {"0"},
		"max_duration_ms": {"250"},
	})

	testutil.Equal(t, "", badRequestMessage)
	testutil.True(t, filters.minDurationMS != nil, "zero minimum duration should remain active")
	testutil.Equal(t, int64(0), *filters.minDurationMS)
	testutil.True(t, filters.maxDurationMS != nil, "maximum duration should remain active")
	testutil.Equal(t, int64(250), *filters.maxDurationMS)
}

func TestParseAdminRequestLogFiltersLeavesOmittedDurationBoundsInactive(t *testing.T) {
	t.Parallel()

	filters, badRequestMessage := parseAdminRequestLogFilters(nil)

	testutil.Equal(t, "", badRequestMessage)
	testutil.True(t, filters.minDurationMS == nil, "omitted minimum duration should be inactive")
	testutil.True(t, filters.maxDurationMS == nil, "omitted maximum duration should be inactive")
}

func TestParseAdminRequestLogFiltersRejectsInvalidNewFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   url.Values
		wantErr string
	}{
		{
			name:    "unknown status class",
			query:   url.Values{"status_class": {"1xx"}},
			wantErr: "invalid status_class; must be one of 2xx, 3xx, 4xx, 5xx",
		},
		{
			name:    "malformed minimum duration",
			query:   url.Values{"min_duration_ms": {"fast"}},
			wantErr: "invalid min_duration_ms; must be a non-negative integer",
		},
		{
			name:    "negative minimum duration",
			query:   url.Values{"min_duration_ms": {"-1"}},
			wantErr: "invalid min_duration_ms; must be a non-negative integer",
		},
		{
			name:    "malformed maximum duration",
			query:   url.Values{"max_duration_ms": {"slow"}},
			wantErr: "invalid max_duration_ms; must be a non-negative integer",
		},
		{
			name:    "negative maximum duration",
			query:   url.Values{"max_duration_ms": {"-1"}},
			wantErr: "invalid max_duration_ms; must be a non-negative integer",
		},
		{
			name: "minimum exceeds maximum",
			query: url.Values{
				"min_duration_ms": {"251"},
				"max_duration_ms": {"250"},
			},
			wantErr: "min_duration_ms must be less than or equal to max_duration_ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, badRequestMessage := parseAdminRequestLogFilters(tt.query)

			testutil.Equal(t, tt.wantErr, badRequestMessage)
		})
	}
}
