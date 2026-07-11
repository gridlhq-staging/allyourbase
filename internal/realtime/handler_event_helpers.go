package realtime

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

// extractToken gets the bearer token from the Authorization header or the token
// query parameter. EventSource (browser SSE API) does not support custom
// headers, so the query parameter remains available for short-lived JWTs; API
// keys must stay in the Authorization header to avoid leaking durable
// credentials into URL logs and browser history.
func extractToken(r *http.Request) (string, bool) {
	if token, ok := httputil.ExtractBearerToken(r); ok {
		return token, false
	}
	return r.URL.Query().Get("token"), true
}

// shouldDeliverEvent applies column-level filters to determine if an event should
// be delivered. Returns true for unfiltered subscriptions. For UPDATE events,
// evaluates both old and new row values to handle enter/leave filter transitions.
func shouldDeliverEvent(event *Event, filters Filters) bool {
	if len(filters) == 0 {
		return true
	}

	match := filters.Matches(event.OldRecord, event.Record)
	return ShouldDeliver(event.Action, match)
}

func sanitizeEventForClient(event *Event) *Event {
	if event == nil {
		return nil
	}
	clean := *event
	clean.OldRecord = nil
	return &clean
}
