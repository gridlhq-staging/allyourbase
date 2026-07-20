package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	statuspkg "github.com/allyourbase/ayb/internal/status"
)

// TestPublicStatusWithNoHistoryIsMajorOutage drives the real handlePublicStatus
// with a nil incident store and no status snapshots. When there is no
// trustworthy status evidence — a nil history, or an empty history with no
// snapshots — the public endpoint must not claim health. The honest fallback is
// MajorOutage.
func TestPublicStatusWithNoHistoryIsMajorOutage(t *testing.T) {
	cases := []struct {
		name    string
		history *statuspkg.StatusHistory
	}{
		{name: "nil history", history: nil},
		{name: "empty history", history: statuspkg.NewStatusHistory(1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := handlePublicStatus(tc.history, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status code = %d, want 200", w.Code)
			}
			var got statusResponse
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.Status != statuspkg.MajorOutage {
				t.Fatalf("public status with %s = %q, want %q (no evidence must not report healthy)",
					tc.name, got.Status, statuspkg.MajorOutage)
			}
			if len(got.Services) != 0 {
				t.Fatalf("public status with %s services len = %d, want 0", tc.name, len(got.Services))
			}
			if got.Services == nil {
				t.Fatalf("public status with %s services = nil, want empty non-nil array", tc.name)
			}
		})
	}
}
