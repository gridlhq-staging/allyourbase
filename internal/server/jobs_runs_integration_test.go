//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminJobsRunsAPI(t *testing.T) {
	ctx := context.Background()
	ensureIntegrationMigrations(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)

	store := jobs.NewStore(sharedPG.Pool)
	jobSvc := jobs.NewService(store, logger, jobs.DefaultServiceConfig())
	srv.SetJobService(jobSvc)

	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_jobs (type, payload, run_at, max_attempts)
		 VALUES ('unrelated_preexisting', '{"scope":"shared-queue-noise"}', NOW() + INTERVAL '2 minutes', 3)`,
	)
	testutil.NoError(t, err)

	jobWithRuns, err := store.Enqueue(ctx, "stale_session_cleanup", json.RawMessage(`{"scope":"runs-api-itest"}`), jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	claimOwnedJob := func(expectedID string) *jobs.Job {
		t.Helper()
		claimedJob, claimErr := store.Claim(ctx, "itest-worker", time.Minute)
		testutil.NoError(t, claimErr)
		testutil.NotNil(t, claimedJob)
		testutil.Equal(t, expectedID, claimedJob.ID)
		return claimedJob
	}

	startedAtAttemptOne := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	finishedAtAttemptOne := time.Date(2026, 5, 31, 12, 0, 42, 0, time.UTC)
	firstAttempt := claimOwnedJob(jobWithRuns.ID)
	_, err = store.Fail(ctx, firstAttempt.ID, "worker timeout", 0, jobs.RunTiming{
		StartedAt:  startedAtAttemptOne,
		FinishedAt: finishedAtAttemptOne,
		DurationMs: 42000,
	})
	testutil.NoError(t, err)

	startedAtAttemptTwo := time.Date(2026, 5, 31, 12, 1, 0, 0, time.UTC)
	finishedAtAttemptTwo := time.Date(2026, 5, 31, 12, 1, 15, 0, time.UTC)
	secondAttempt := claimOwnedJob(jobWithRuns.ID)
	_, err = store.Complete(ctx, secondAttempt.ID, jobs.RunTiming{
		StartedAt:  startedAtAttemptTwo,
		FinishedAt: finishedAtAttemptTwo,
		DurationMs: 15000,
	})
	testutil.NoError(t, err)

	jobWithoutRuns, err := store.Enqueue(ctx, "webhook_delivery_prune", json.RawMessage(`{"retention_hours":24}`), jobs.EnqueueOpts{})
	testutil.NoError(t, err)

	token := adminLogin(t, srv)
	callRunsEndpoint := func(jobID string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/admin/jobs/"+jobID+"/runs", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		return w
	}

	withRunsResponse := callRunsEndpoint(jobWithRuns.ID)
	testutil.StatusCode(t, http.StatusOK, withRunsResponse.Code)
	var withRunsBody struct {
		Items []jobs.JobRun `json:"items"`
		Count int           `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(withRunsResponse.Body.Bytes(), &withRunsBody))
	testutil.Equal(t, 2, withRunsBody.Count)
	testutil.Equal(t, 2, len(withRunsBody.Items))

	testutil.Equal(t, 1, withRunsBody.Items[0].Attempt)
	testutil.Equal(t, jobs.StateFailed, withRunsBody.Items[0].Status)
	testutil.Equal(t, startedAtAttemptOne.UnixMilli(), withRunsBody.Items[0].StartedAt.UTC().UnixMilli())
	testutil.Equal(t, finishedAtAttemptOne.UnixMilli(), withRunsBody.Items[0].FinishedAt.UTC().UnixMilli())
	testutil.Equal(t, 42000, withRunsBody.Items[0].DurationMs)
	testutil.NotNil(t, withRunsBody.Items[0].Error)
	testutil.Equal(t, "worker timeout", *withRunsBody.Items[0].Error)

	testutil.Equal(t, 2, withRunsBody.Items[1].Attempt)
	testutil.Equal(t, jobs.StateCompleted, withRunsBody.Items[1].Status)
	testutil.Equal(t, startedAtAttemptTwo.UnixMilli(), withRunsBody.Items[1].StartedAt.UTC().UnixMilli())
	testutil.Equal(t, finishedAtAttemptTwo.UnixMilli(), withRunsBody.Items[1].FinishedAt.UTC().UnixMilli())
	testutil.Equal(t, 15000, withRunsBody.Items[1].DurationMs)
	testutil.Nil(t, withRunsBody.Items[1].Error)

	withoutRunsResponse := callRunsEndpoint(jobWithoutRuns.ID)
	testutil.StatusCode(t, http.StatusOK, withoutRunsResponse.Code)
	var withoutRunsBody struct {
		Items []jobs.JobRun `json:"items"`
		Count int           `json:"count"`
	}
	testutil.NoError(t, json.Unmarshal(withoutRunsResponse.Body.Bytes(), &withoutRunsBody))
	testutil.Equal(t, 0, withoutRunsBody.Count)
	testutil.Equal(t, 0, len(withoutRunsBody.Items))
}
