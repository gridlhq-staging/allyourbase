//go:build integration

package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/version"
)

func TestHealthEndpointIncludesVersionWithDatabase(t *testing.T) {
	prev := version.Get()
	version.Set("9.9.9-test")
	t.Cleanup(func() { version.Set(prev) })

	logger := testutil.DiscardLogger()
	cache := schema.NewCacheHolder(sharedPG.Pool, logger)
	srv := server.New(config.Default(), logger, cache, sharedPG.Pool, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	testutil.Equal(t, "ok", body["status"])
	testutil.Equal(t, "ok", body["database"])
	testutil.Equal(t, "9.9.9-test", body["version"])
}
