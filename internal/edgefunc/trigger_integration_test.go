//go:build integration

package edgefunc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/pgnotify"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// --- DB Trigger Integration Tests ---
// Verifies: INSERT/UPDATE/DELETE on target table → event queued → dispatcher invokes function → log persisted

func TestDBTriggerIntegration_InsertQueuesEventAndInvokesFunction(t *testing.T) {
	ctx := context.Background()

	// Deploy a simple function that echoes the payload
	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: req.body };
	}`)

	// Create a target table
	table := createTriggerTargetTable(t)

	// Create DB trigger for INSERT events
	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	// INSERT a row into the target table — this fires the PG trigger function
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (value) VALUES ('hello-insert')`, sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	// Verify an event was queued in _ayb_edge_trigger_events
	eventStore := edgefunc.NewDBTriggerEventPostgresStore(testPool)
	events := claimEventsWithRetry(t, eventStore, 5, 200*time.Millisecond)
	testutil.True(t, len(events) >= 1, "expected at least 1 queued event, got %d", len(events))

	// Find the event for our trigger
	var matchedEvent *edgefunc.DBTriggerQueueEvent
	for _, e := range events {
		if e.TriggerID == trigger.ID {
			matchedEvent = e
			break
		}
	}
	testutil.True(t, matchedEvent != nil, "expected to find event for trigger %s", trigger.ID)
	testutil.Equal(t, "INSERT", matchedEvent.Operation)
	testutil.Equal(t, table, matchedEvent.TableName)
	testutil.Equal(t, "public", matchedEvent.SchemaName)

	// Verify payload contains the inserted row data
	var rowData map[string]interface{}
	testutil.NoError(t, json.Unmarshal(matchedEvent.Payload, &rowData))
	testutil.Equal(t, "hello-insert", rowData["value"])

	// Dispatch the event through the full pipeline
	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)
	dispatcher := edgefunc.NewDBTriggerDispatcher(dbTriggerStore, svc)

	err = dispatcher.DispatchEvent(ctx, matchedEvent)
	testutil.NoError(t, err)

	// Verify execution log was persisted with trigger metadata
	logs, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.True(t, len(logs) >= 1, "expected at least 1 log entry, got %d", len(logs))
	testutil.Equal(t, "success", logs[0].Status)
	testutil.Equal(t, "db", logs[0].TriggerType)
	testutil.Equal(t, trigger.ID, logs[0].TriggerID)

	// Mark event completed
	testutil.NoError(t, eventStore.MarkCompleted(ctx, matchedEvent.ID))
}

func TestDBTriggerIntegration_UpdateQueuesEvent(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: "updated" };
	}`)

	table := createTriggerTargetTable(t)

	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventUpdate},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	// Insert a row first (trigger is UPDATE-only, so this shouldn't queue)
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (value) VALUES ('before')`, sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	// Drain any unrelated events
	drainEvents(t)

	// UPDATE the row — this should queue an event
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s SET value = 'after' WHERE value = 'before'`, sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	eventStore := edgefunc.NewDBTriggerEventPostgresStore(testPool)
	events := claimEventsWithRetry(t, eventStore, 5, 200*time.Millisecond)

	var matchedEvent *edgefunc.DBTriggerQueueEvent
	for _, e := range events {
		if e.TriggerID == trigger.ID {
			matchedEvent = e
			break
		}
	}
	testutil.True(t, matchedEvent != nil, "expected to find UPDATE event for trigger %s", trigger.ID)
	testutil.Equal(t, "UPDATE", matchedEvent.Operation)

	// Verify the payload contains the updated row
	var rowData map[string]interface{}
	testutil.NoError(t, json.Unmarshal(matchedEvent.Payload, &rowData))
	testutil.Equal(t, "after", rowData["value"])

	testutil.NoError(t, eventStore.MarkCompleted(ctx, matchedEvent.ID))
}

func TestDBTriggerIntegration_DeleteQueuesEvent(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: "deleted" };
	}`)

	table := createTriggerTargetTable(t)

	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventDelete},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	// Insert a row
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (value) VALUES ('doomed')`, sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	// Drain any unrelated events
	drainEvents(t)

	// DELETE the row
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE value = 'doomed'`, sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	eventStore := edgefunc.NewDBTriggerEventPostgresStore(testPool)
	events := claimEventsWithRetry(t, eventStore, 5, 200*time.Millisecond)

	var matchedEvent *edgefunc.DBTriggerQueueEvent
	for _, e := range events {
		if e.TriggerID == trigger.ID {
			matchedEvent = e
			break
		}
	}
	testutil.True(t, matchedEvent != nil, "expected to find DELETE event for trigger %s", trigger.ID)
	testutil.Equal(t, "DELETE", matchedEvent.Operation)

	// Verify the payload contains the deleted row (OLD record)
	var rowData map[string]interface{}
	testutil.NoError(t, json.Unmarshal(matchedEvent.Payload, &rowData))
	testutil.Equal(t, "doomed", rowData["value"])

	testutil.NoError(t, eventStore.MarkCompleted(ctx, matchedEvent.ID))
}

func TestDBTriggerIntegration_DisabledTriggerDoesNotQueue(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200 };
	}`)

	table := createTriggerTargetTable(t)

	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	// Disable the trigger
	_, err = dbTriggerSvc.SetEnabled(ctx, trigger.ID, false)
	testutil.NoError(t, err)

	// Drain any events
	drainEvents(t)

	// INSERT a row — PG trigger is disabled, no event should be queued
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (value) VALUES ('should-not-trigger')`, sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	eventStore := edgefunc.NewDBTriggerEventPostgresStore(testPool)
	events, err := eventStore.ClaimPendingEvents(ctx, 10)
	testutil.NoError(t, err)

	for _, e := range events {
		testutil.True(t, e.TriggerID != trigger.ID,
			"disabled trigger should not queue events, found event %s", e.ID)
	}
}

func TestDBTriggerWorkerStart_ProcessesNotifyBeforePollFallback(t *testing.T) {
	ctx := context.Background()
	drainEvents(t)

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: req.body };
	}`)
	table := createTriggerTargetTable(t)
	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_ = dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	applicationName := "ayb_test_db_trigger_worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	workerDone, cancelWorker := startDBTriggerWorker(t, withApplicationName(t, testConnString, applicationName))
	defer stopDBTriggerWorker(t, cancelWorker, workerDone)
	waitForDBTriggerWorkerListenerPID(t, applicationName)

	insertedAt := time.Now()
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (value) VALUES ('worker-notify')`,
		sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	waitForDBTriggerSuccessLog(t, fn.ID, trigger.ID, 2*time.Second)
	elapsed := time.Since(insertedAt)
	testutil.True(t, elapsed < 5*time.Second,
		"worker should process NOTIFY before 5s poll fallback; elapsed=%s", elapsed)
	assertDBTriggerQueueCompleted(t, trigger.ID)
}

func TestDBTriggerWorkerStart_ProcessesQueuedEventOnListenTimeoutWithoutNotify(t *testing.T) {
	ctx := context.Background()
	drainEvents(t)

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: req.body };
	}`)
	table := createTriggerTargetTable(t)
	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_ = dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	applicationName := "ayb_test_db_trigger_worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	workerDone, cancelWorker := startDBTriggerWorker(
		t,
		withApplicationName(t, testConnString, applicationName),
		edgefunc.WithDBTriggerWorkerTiming(100*time.Millisecond),
	)
	defer stopDBTriggerWorker(t, cancelWorker, workerDone)
	waitForDBTriggerWorkerListenerPID(t, applicationName)

	insertedAt := time.Now()
	_, err = testPool.Exec(ctx,
		`INSERT INTO _ayb_edge_trigger_events (trigger_id, table_name, schema_name, operation, row_id, payload)
		 VALUES ($1, $2, 'public', 'INSERT', 'direct-row-1', '{"value":"worker-timeout"}'::jsonb)`,
		trigger.ID,
		table,
	)
	testutil.NoError(t, err)

	waitForDBTriggerSuccessLog(t, fn.ID, trigger.ID, 2*time.Second)
	elapsed := time.Since(insertedAt)
	testutil.True(t, elapsed < 5*time.Second,
		"worker should process queued event on LISTEN timeout before 5s poll fallback; elapsed=%s", elapsed)
	assertDBTriggerQueueCompleted(t, trigger.ID)
}

func TestDBTriggerWorkerStart_ReconnectsAfterListenerBackendTermination(t *testing.T) {
	ctx := context.Background()
	drainEvents(t)

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: req.body };
	}`)
	table := createTriggerTargetTable(t)
	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	dbTriggerSvc := edgefunc.NewDBTriggerService(dbTriggerStore)

	trigger, err := dbTriggerSvc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: fn.ID.String(),
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_ = dbTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	applicationName := "ayb_test_db_trigger_worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	connString := withApplicationName(t, testConnString, applicationName)
	workerDone, cancelWorker := startDBTriggerWorker(
		t,
		connString,
		edgefunc.WithDBTriggerWorkerTiming(30*time.Second),
	)
	defer stopDBTriggerWorker(t, cancelWorker, workerDone)

	initialPID := waitForDBTriggerWorkerListenerPID(t, applicationName)
	var terminated bool
	err = testPool.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, initialPID).Scan(&terminated)
	testutil.NoError(t, err)
	testutil.True(t, terminated, "expected pg_terminate_backend to terminate listener PID %d", initialPID)
	waitForDBTriggerWorkerListenerPIDChange(t, applicationName, initialPID)

	insertedAt := time.Now()
	_, err = testPool.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (value) VALUES ('worker-reconnect')`,
		sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	waitForDBTriggerSuccessLog(t, fn.ID, trigger.ID, 2*time.Second)
	elapsed := time.Since(insertedAt)
	testutil.True(t, elapsed < 5*time.Second,
		"reconnected worker should process NOTIFY before 5s poll fallback; elapsed=%s", elapsed)
	assertDBTriggerQueueCompleted(t, trigger.ID)
}

// --- Cron Trigger Integration Tests ---
// Verifies: create cron trigger → job handler fires → function invoked → log persisted

func TestCronTriggerIntegration_JobHandlerInvokesFunction(t *testing.T) {
	ctx := context.Background()

	// Deploy a function that reads the cron payload
	fn := deployTriggerTestFunction(t, `function handler(req) {
		var body = JSON.parse(req.body);
		return { statusCode: 200, body: "cron-key:" + (body.key || "none") };
	}`)

	// Set up service stack
	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Build the cron job handler using the real service as the invoker
	handler := edgefunc.NewCronJobHandler(svc)

	// Construct a CronJobPayload as the jobs scheduler would
	cronTriggerID := uuid.New().String()
	jobPayload := edgefunc.CronJobPayload{
		FunctionID:    fn.ID.String(),
		CronTriggerID: cronTriggerID,
		Payload:       json.RawMessage(`{"key":"cron-value"}`),
	}
	payloadBytes, err := json.Marshal(jobPayload)
	testutil.NoError(t, err)

	// Execute the handler as the scheduler would
	err = handler(ctx, payloadBytes)
	testutil.NoError(t, err)

	// Verify log entry was persisted with cron trigger metadata
	logs, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.True(t, len(logs) >= 1, "expected at least 1 log entry, got %d", len(logs))
	testutil.Equal(t, "success", logs[0].Status)
	testutil.Equal(t, "cron", logs[0].TriggerType)
	testutil.Equal(t, cronTriggerID, logs[0].TriggerID)
}

func TestCronTriggerIntegration_FullCRUDWithPostgresStores(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200 };
	}`)

	cronStore := edgefunc.NewCronTriggerPostgresStore(testPool)
	jobStore := jobs.NewStore(testPool)
	jobSvc := jobs.NewService(jobStore, testutil.DiscardLogger(), jobs.ServiceConfig{})
	adapter := &testJobsSchedulerAdapter{svc: jobSvc}
	cronSvc := edgefunc.NewCronTriggerService(cronStore, adapter)

	// Create
	cronTrigger, err := cronSvc.Create(ctx, edgefunc.CreateCronTriggerInput{
		FunctionID: fn.ID.String(),
		CronExpr:   "*/10 * * * *",
		Timezone:   "America/New_York",
		Payload:    json.RawMessage(`{"task":"cleanup"}`),
	})
	testutil.NoError(t, err)
	testutil.True(t, cronTrigger.ID != "", "should have an ID")
	testutil.Equal(t, "*/10 * * * *", cronTrigger.CronExpr)
	testutil.Equal(t, "America/New_York", cronTrigger.Timezone)

	// Get
	got, err := cronSvc.Get(ctx, cronTrigger.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, cronTrigger.ID, got.ID)

	// List
	cronTriggers, err := cronSvc.List(ctx, fn.ID.String())
	testutil.NoError(t, err)
	testutil.SliceLen(t, cronTriggers, 1)

	// Disable
	disabled, err := cronSvc.SetEnabled(ctx, cronTrigger.ID, false)
	testutil.NoError(t, err)
	testutil.False(t, disabled.Enabled)

	// Re-enable
	reenabled, err := cronSvc.SetEnabled(ctx, cronTrigger.ID, true)
	testutil.NoError(t, err)
	testutil.True(t, reenabled.Enabled)

	// Delete
	err = cronSvc.Delete(ctx, cronTrigger.ID)
	testutil.NoError(t, err)

	_, err = cronSvc.Get(ctx, cronTrigger.ID)
	testutil.True(t, errors.Is(err, edgefunc.ErrCronTriggerNotFound), "expected cron trigger not found, got: %v", err)
}

// --- Storage Trigger Integration Tests ---
// Verifies: storage event → dispatcher matches trigger → function invoked → log persisted

func TestStorageTriggerIntegration_UploadFiresTrigger(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		var body = JSON.parse(req.body);
		return { statusCode: 200, body: "bucket:" + body.bucket + ",name:" + body.name };
	}`)

	// Create storage trigger in Postgres
	storageTriggerStore := edgefunc.NewStorageTriggerPostgresStore(testPool)
	storageTriggerSvc := edgefunc.NewStorageTriggerService(storageTriggerStore)

	trigger, err := storageTriggerSvc.Create(ctx, edgefunc.CreateStorageTriggerInput{
		FunctionID: fn.ID.String(),
		Bucket:     "test-uploads",
		EventTypes: []string{"upload"},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		storageTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	// Set up service stack for invocation
	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Create dispatcher with real Postgres store and service
	dispatcher := edgefunc.NewStorageTriggerDispatcher(storageTriggerStore, svc)

	// Simulate a storage upload event
	err = dispatcher.OnStorageEvent(ctx, storage.StorageEvent{
		Bucket:      "test-uploads",
		Name:        "photos/vacation.jpg",
		Operation:   storage.OperationUpload,
		Size:        4096,
		ContentType: "image/jpeg",
	})
	testutil.NoError(t, err)

	// Verify log entry was persisted with storage trigger metadata
	logs, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.True(t, len(logs) >= 1, "expected at least 1 log entry, got %d", len(logs))
	testutil.Equal(t, "success", logs[0].Status)
	testutil.Equal(t, "storage", logs[0].TriggerType)
	testutil.Equal(t, trigger.ID, logs[0].TriggerID)
}

func TestStorageTriggerIntegration_DeleteFiresTrigger(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		var body = JSON.parse(req.body);
		return { statusCode: 200, body: body.operation };
	}`)

	storageTriggerStore := edgefunc.NewStorageTriggerPostgresStore(testPool)
	storageTriggerSvc := edgefunc.NewStorageTriggerService(storageTriggerStore)

	trigger, err := storageTriggerSvc.Create(ctx, edgefunc.CreateStorageTriggerInput{
		FunctionID: fn.ID.String(),
		Bucket:     "test-docs",
		EventTypes: []string{"delete"},
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		storageTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)
	dispatcher := edgefunc.NewStorageTriggerDispatcher(storageTriggerStore, svc)

	err = dispatcher.OnStorageEvent(ctx, storage.StorageEvent{
		Bucket:    "test-docs",
		Name:      "reports/old-report.pdf",
		Operation: storage.OperationDelete,
	})
	testutil.NoError(t, err)

	logs, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.True(t, len(logs) >= 1, "expected at least 1 log entry, got %d", len(logs))
	testutil.Equal(t, "success", logs[0].Status)
	testutil.Equal(t, "storage", logs[0].TriggerType)
	testutil.Equal(t, trigger.ID, logs[0].TriggerID)
}

func TestStorageTriggerIntegration_PrefixFilterSkipsNonMatching(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200, body: "matched" };
	}`)

	storageTriggerStore := edgefunc.NewStorageTriggerPostgresStore(testPool)
	storageTriggerSvc := edgefunc.NewStorageTriggerService(storageTriggerStore)

	trigger, err := storageTriggerSvc.Create(ctx, edgefunc.CreateStorageTriggerInput{
		FunctionID:   fn.ID.String(),
		Bucket:       "test-filtered",
		EventTypes:   []string{"upload"},
		PrefixFilter: "images/",
		SuffixFilter: ".png",
	})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		storageTriggerSvc.Delete(context.Background(), trigger.ID)
	})

	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)
	dispatcher := edgefunc.NewStorageTriggerDispatcher(storageTriggerStore, svc)

	// Non-matching prefix: should not invoke
	err = dispatcher.OnStorageEvent(ctx, storage.StorageEvent{
		Bucket:    "test-filtered",
		Name:      "docs/report.png",
		Operation: storage.OperationUpload,
	})
	testutil.NoError(t, err)

	// Non-matching suffix: should not invoke
	err = dispatcher.OnStorageEvent(ctx, storage.StorageEvent{
		Bucket:    "test-filtered",
		Name:      "images/photo.jpg",
		Operation: storage.OperationUpload,
	})
	testutil.NoError(t, err)

	// Matching both: should invoke
	err = dispatcher.OnStorageEvent(ctx, storage.StorageEvent{
		Bucket:    "test-filtered",
		Name:      "images/avatar.png",
		Operation: storage.OperationUpload,
	})
	testutil.NoError(t, err)

	logs, err := logStore.ListByFunction(ctx, fn.ID, edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.SliceLen(t, logs, 1)
	testutil.Equal(t, "success", logs[0].Status)
}

func TestStorageTriggerIntegration_FullCRUDWithPostgresStore(t *testing.T) {
	ctx := context.Background()

	fn := deployTriggerTestFunction(t, `function handler(req) {
		return { statusCode: 200 };
	}`)

	storageTriggerStore := edgefunc.NewStorageTriggerPostgresStore(testPool)
	svc := edgefunc.NewStorageTriggerService(storageTriggerStore)

	// Create
	trigger, err := svc.Create(ctx, edgefunc.CreateStorageTriggerInput{
		FunctionID:   fn.ID.String(),
		Bucket:       "crud-test",
		EventTypes:   []string{"upload", "delete"},
		PrefixFilter: "prefix/",
		SuffixFilter: ".txt",
	})
	testutil.NoError(t, err)
	testutil.True(t, trigger.ID != "", "should have an ID")
	testutil.Equal(t, "crud-test", trigger.Bucket)
	testutil.Equal(t, 2, len(trigger.EventTypes))
	testutil.Equal(t, "prefix/", trigger.PrefixFilter)
	testutil.Equal(t, ".txt", trigger.SuffixFilter)

	// Get
	got, err := svc.Get(ctx, trigger.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, trigger.ID, got.ID)

	// List
	triggers, err := svc.List(ctx, fn.ID.String())
	testutil.NoError(t, err)
	testutil.SliceLen(t, triggers, 1)

	// Toggle
	disabled, err := svc.SetEnabled(ctx, trigger.ID, false)
	testutil.NoError(t, err)
	testutil.False(t, disabled.Enabled)

	// Delete
	err = svc.Delete(ctx, trigger.ID)
	testutil.NoError(t, err)

	_, err = svc.Get(ctx, trigger.ID)
	testutil.True(t, errors.Is(err, edgefunc.ErrStorageTriggerNotFound), "expected storage trigger not found, got: %v", err)
}

// --- Helpers ---

// deployTriggerTestFunction deploys a simple edge function and returns it.
func deployTriggerTestFunction(t *testing.T, source string) *edgefunc.EdgeFunction {
	t.Helper()

	ctx := context.Background()
	name := "test-trigger-fn-" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(1)

	svc := edgefunc.NewService(store, pool, logStore)
	fn, err := svc.Deploy(ctx, name, source, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		store.Delete(context.Background(), fn.ID)
	})
	return fn
}

// createTriggerTargetTable creates a test table for DB trigger integration tests.
func createTriggerTargetTable(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	table := "test_trigger_int_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	_, err := testPool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, value TEXT)`,
		sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s CASCADE`, sqlutil.QuoteIdent(table)))
	})
	return table
}

// claimEventsWithRetry polls for pending events with retries (events may not be immediately visible).
func claimEventsWithRetry(t *testing.T, store *edgefunc.DBTriggerEventPostgresStore, maxRetries int, interval time.Duration) []*edgefunc.DBTriggerQueueEvent {
	t.Helper()

	for i := 0; i < maxRetries; i++ {
		events, err := store.ClaimPendingEvents(context.Background(), 50)
		testutil.NoError(t, err)
		if len(events) > 0 {
			return events
		}
		time.Sleep(interval)
	}
	t.Fatalf("no events found after %d retries", maxRetries)
	return nil
}

// drainEvents claims and discards all pending events to clean slate for next assertion.
func drainEvents(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	eventStore := edgefunc.NewDBTriggerEventPostgresStore(testPool)
	for {
		events, err := eventStore.ClaimPendingEvents(ctx, 100)
		testutil.NoError(t, err)
		if len(events) == 0 {
			return
		}
		for _, e := range events {
			_ = eventStore.MarkCompleted(ctx, e.ID)
		}
	}
}

func startDBTriggerWorker(
	t *testing.T,
	connString string,
	opts ...edgefunc.DBTriggerWorkerOption,
) (<-chan error, context.CancelFunc) {
	t.Helper()

	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	t.Cleanup(pool.Close)

	svc := edgefunc.NewService(store, pool, logStore)
	dbTriggerStore := edgefunc.NewDBTriggerPostgresStore(testPool)
	bus := pgnotify.NewBus(testPool, connString, testutil.DiscardLogger())
	worker := edgefunc.NewDBTriggerWorker(
		edgefunc.NewDBTriggerEventPostgresStore(testPool),
		edgefunc.NewDBTriggerDispatcher(dbTriggerStore, svc),
		bus,
		testutil.DiscardLogger(),
		opts...,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()
	return done, cancel
}

func stopDBTriggerWorker(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("DBTriggerWorker.Start returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DBTriggerWorker.Start did not exit after context cancellation")
	}
	drainEvents(t)
}

func waitForDBTriggerSuccessLog(
	t *testing.T,
	functionID uuid.UUID,
	triggerID string,
	timeout time.Duration,
) *edgefunc.LogEntry {
	t.Helper()

	ctx := context.Background()
	logStore := edgefunc.NewPostgresLogStore(testPool)
	deadline := time.After(timeout)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		logs, err := logStore.ListByFunction(ctx, functionID, edgefunc.LogListOptions{
			Page:        1,
			PerPage:     20,
			Status:      "success",
			TriggerType: string(edgefunc.TriggerDB),
		})
		testutil.NoError(t, err)
		for _, log := range logs {
			if log.TriggerID == triggerID {
				return log
			}
		}

		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for success db trigger log for trigger %s", triggerID)
		}
	}
}

func assertDBTriggerQueueCompleted(t *testing.T, triggerID string) {
	t.Helper()

	var status string
	var attempts int
	err := testPool.QueryRow(context.Background(),
		`SELECT status, attempts
		 FROM _ayb_edge_trigger_events
		 WHERE trigger_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		triggerID,
	).Scan(&status, &attempts)
	testutil.NoError(t, err)
	testutil.Equal(t, string(edgefunc.DBEventStatusCompleted), status)
	testutil.Equal(t, 1, attempts)
}

func withApplicationName(t *testing.T, connString, applicationName string) string {
	t.Helper()

	u, err := url.Parse(connString)
	testutil.NoError(t, err)
	values := u.Query()
	values.Set("application_name", applicationName)
	u.RawQuery = values.Encode()
	return u.String()
}

func waitForDBTriggerWorkerListenerPID(t *testing.T, applicationName string) int {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if pid := dbTriggerWorkerListenerPID(t, applicationName); pid != 0 {
			return pid
		}

		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for DB trigger worker listener PID for application_name %q", applicationName)
		}
	}
}

func waitForDBTriggerWorkerListenerPIDChange(t *testing.T, applicationName string, previous int) int {
	t.Helper()

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if pid := dbTriggerWorkerListenerPID(t, applicationName); pid != 0 && pid != previous {
			return pid
		}

		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for DB trigger worker listener PID for %q to change from %d",
				applicationName, previous)
		}
	}
}

func dbTriggerWorkerListenerPID(t *testing.T, applicationName string) int {
	t.Helper()

	var pid int
	err := testPool.QueryRow(context.Background(),
		`SELECT pid
		 FROM pg_stat_activity
		 WHERE datname = current_database()
		   AND application_name = $1
		   AND query = 'LISTEN ayb_edge_trigger'
		 ORDER BY backend_start DESC
		 LIMIT 1`,
		applicationName,
	).Scan(&pid)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0
	}
	testutil.NoError(t, err)
	return pid
}

// testJobsSchedulerAdapter adapts jobs.Service to edgefunc.JobsScheduler for integration tests.
type testJobsSchedulerAdapter struct {
	svc *jobs.Service
}

func (a *testJobsSchedulerAdapter) CreateSchedule(
	ctx context.Context,
	name, jobType string,
	payload json.RawMessage,
	cronExpr, timezone string,
	maxAttempts int,
	nextRunAt *time.Time,
) (string, error) {
	sched, err := a.svc.CreateSchedule(ctx, &jobs.Schedule{
		Name:        name,
		JobType:     jobType,
		Payload:     payload,
		CronExpr:    cronExpr,
		Timezone:    timezone,
		Enabled:     true,
		MaxAttempts: maxAttempts,
		NextRunAt:   nextRunAt,
	})
	if err != nil {
		return "", err
	}
	return sched.ID, nil
}

func (a *testJobsSchedulerAdapter) DeleteSchedule(ctx context.Context, id string) error {
	return a.svc.DeleteSchedule(ctx, id)
}

func (a *testJobsSchedulerAdapter) SetScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := a.svc.SetScheduleEnabled(ctx, id, enabled)
	return err
}
