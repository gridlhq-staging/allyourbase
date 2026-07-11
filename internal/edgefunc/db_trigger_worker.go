package edgefunc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/pgnotify"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

const (
	// dbTriggerNotifyChannel is the PG NOTIFY channel used for low-latency wakeup.
	dbTriggerNotifyChannel = "ayb_edge_trigger"

	// dbTriggerPollInterval is the fallback polling interval when LISTEN is unavailable.
	dbTriggerPollInterval = 5 * time.Second

	// dbTriggerListenTimeout is the max wait time per raw listener notification read.
	dbTriggerListenTimeout = 30 * time.Second

	// dbTriggerBatchSize is the number of events claimed per poll cycle.
	dbTriggerBatchSize = 10
)

// DBTriggerWorker is a background worker that processes DB trigger events
// from the queue table. It uses LISTEN/NOTIFY for low-latency wakeup with
// a fallback poll timer for reliability.
type DBTriggerWorker struct {
	eventStore    DBTriggerEventStore
	dispatcher    *DBTriggerDispatcher
	bus           *pgnotify.Bus
	logger        *slog.Logger
	listenTimeout time.Duration
}

// DBTriggerWorkerOption customizes DB trigger worker behavior.
type DBTriggerWorkerOption func(*DBTriggerWorker)

// WithDBTriggerWorkerTiming overrides the listener wait timeout (the interval
// after which an idle LISTEN drains the durable queue).
//
// It deliberately takes ONLY the listen timeout. Reconnect backoff is not a
// worker concern: since the pgnotify consolidation the worker listens through
// pgnotify.Bus.ListenRaw, which owns reconnect-with-backoff. An earlier
// signature accepted a second reconnect-delay argument and silently discarded
// it -- a knob that looks configurable but does nothing is worse than no knob.
func WithDBTriggerWorkerTiming(listenTimeout time.Duration) DBTriggerWorkerOption {
	return func(w *DBTriggerWorker) {
		if listenTimeout > 0 {
			w.listenTimeout = listenTimeout
		}
	}
}

// NewDBTriggerWorker creates a new worker that processes DB trigger events.
func NewDBTriggerWorker(
	eventStore DBTriggerEventStore,
	dispatcher *DBTriggerDispatcher,
	bus *pgnotify.Bus,
	logger *slog.Logger,
	opts ...DBTriggerWorkerOption,
) *DBTriggerWorker {
	w := &DBTriggerWorker{
		eventStore:    eventStore,
		dispatcher:    dispatcher,
		bus:           bus,
		logger:        logger,
		listenTimeout: dbTriggerListenTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(w)
		}
	}
	return w
}

// Start runs the worker loop. It attempts to establish a LISTEN connection
// for low-latency event processing, falling back to periodic polling if
// the connection fails. Blocks until ctx is cancelled.
func (w *DBTriggerWorker) Start(ctx context.Context) error {
	if w.bus == nil {
		w.logger.Warn("db trigger worker: listener bus unavailable, falling back to polling")
		return w.runPoller(ctx)
	}

	w.logger.Info("db trigger worker: listening for events",
		"channel", dbTriggerNotifyChannel)

	err := w.bus.ListenRaw(ctx, dbTriggerNotifyChannel, func(string) {
		w.processAvailableEvents(ctx)
	},
		pgnotify.WithRawListenStartHandler(w.processAvailableEvents),
		pgnotify.WithRawListenTimeout(w.listenTimeout),
		pgnotify.WithRawListenTimeoutHandler(w.processAvailableEvents),
		pgnotify.WithRawListenReconnectFailurePolicy(pgnotify.RawListenReconnectReturnFailure),
	)
	if err != nil && ctx.Err() == nil {
		w.logger.Warn("db trigger worker: listener failed, falling back to polling",
			"error", err)
		return w.runPoller(ctx)
	}
	return err
}

// runPoller periodically processes events when LISTEN is unavailable.
func (w *DBTriggerWorker) runPoller(ctx context.Context) error {
	ticker := time.NewTicker(dbTriggerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.processAvailableEvents(ctx)
		}
	}
}

// processAvailableEvents claims and dispatches all pending events in batches.
func (w *DBTriggerWorker) processAvailableEvents(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		events, err := w.eventStore.ClaimPendingEvents(ctx, dbTriggerBatchSize)
		if err != nil {
			w.logger.Error("db trigger worker: claiming events failed", "error", err)
			return
		}
		if len(events) == 0 {
			return
		}

		for _, event := range events {
			if err := w.dispatcher.DispatchEvent(ctx, event); err != nil {
				attempt := event.Attempts
				retryable := attempt < MaxDBTriggerRetries
				w.logger.Error("db trigger worker: dispatch failed",
					"event_id", event.ID,
					"trigger_id", event.TriggerID,
					"attempt", attempt,
					"max_attempts", MaxDBTriggerRetries,
					"will_retry", retryable,
					"error", err,
				)
				if markErr := w.eventStore.MarkFailed(ctx, event.ID); markErr != nil {
					w.logger.Error("db trigger worker: marking event failed",
						"event_id", event.ID, "error", markErr)
				}
			} else {
				if markErr := w.eventStore.MarkCompleted(ctx, event.ID); markErr != nil {
					w.logger.Error("db trigger worker: marking event completed failed",
						"event_id", event.ID, "error", markErr)
				}
			}
		}
	}
}

// TriggerName returns the PG trigger name for a given DB trigger record.
// Used for CREATE/DROP TRIGGER on the target table.
func TriggerName(triggerID string) string {
	return fmt.Sprintf("_ayb_edge_trig_%s", triggerID)
}

// InstallTriggerSQL returns the SQL to CREATE TRIGGER on the target table.
// It defensively validates events at the DDL layer (not just the service layer)
// to prevent SQL injection if this function is ever called with unchecked input.
func InstallTriggerSQL(trigger *DBTrigger) string {
	var eventParts []string
	for _, ev := range trigger.Events {
		// Defense-in-depth: only allow known SQL keywords, reject anything else.
		switch ev {
		case DBEventInsert, DBEventUpdate, DBEventDelete:
			eventParts = append(eventParts, string(ev))
		default:
			// Skip unknown events rather than interpolating raw user input into SQL.
			continue
		}
	}
	events := strings.Join(eventParts, " OR ")

	return fmt.Sprintf(
		`CREATE TRIGGER %s AFTER %s ON %s FOR EACH ROW EXECUTE FUNCTION _ayb_edge_notify('%s')`,
		sqlutil.QuoteIdent(TriggerName(trigger.ID)),
		events,
		sqlutil.QuoteQualifiedName(trigger.Schema, trigger.TableName),
		escapeSQLStringLiteral(trigger.ID),
	)
}

// RemoveTriggerSQL returns the SQL to DROP TRIGGER from the target table.
func RemoveTriggerSQL(trigger *DBTrigger) string {
	return fmt.Sprintf(
		`DROP TRIGGER IF EXISTS %s ON %s`,
		sqlutil.QuoteIdent(TriggerName(trigger.ID)),
		sqlutil.QuoteQualifiedName(trigger.Schema, trigger.TableName),
	)
}

// EnableTriggerSQL returns the SQL to enable a trigger on the target table.
func EnableTriggerSQL(trigger *DBTrigger) string {
	return fmt.Sprintf(
		`ALTER TABLE %s ENABLE TRIGGER %s`,
		sqlutil.QuoteQualifiedName(trigger.Schema, trigger.TableName),
		sqlutil.QuoteIdent(TriggerName(trigger.ID)),
	)
}

// DisableTriggerSQL returns the SQL to disable a trigger on the target table.
func DisableTriggerSQL(trigger *DBTrigger) string {
	return fmt.Sprintf(
		`ALTER TABLE %s DISABLE TRIGGER %s`,
		sqlutil.QuoteQualifiedName(trigger.Schema, trigger.TableName),
		sqlutil.QuoteIdent(TriggerName(trigger.ID)),
	)
}

// escapeSQLStringLiteral escapes a value for embedding in a single-quoted SQL
// string literal by doubling any single quotes.
func escapeSQLStringLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
