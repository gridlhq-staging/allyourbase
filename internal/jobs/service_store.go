// Package jobs Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun01_pm_6_release_readiness_closeout/allyourbase_dev/internal/jobs/service_store.go.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Enqueue delegates to the underlying store.
func (s *Service) Enqueue(ctx context.Context, jobType string, payload json.RawMessage, opts EnqueueOpts) (*Job, error) {
	return s.store.Enqueue(ctx, jobType, payload, opts)
}

// Get delegates to the underlying store.
func (s *Service) Get(ctx context.Context, jobID string) (*Job, error) {
	return s.store.Get(ctx, jobID)
}

// ListRuns delegates to the underlying store.
func (s *Service) ListRuns(ctx context.Context, jobID string) ([]JobRun, error) {
	return s.store.ListRuns(ctx, jobID)
}

// List delegates to the underlying store.
func (s *Service) List(ctx context.Context, state, jobType string, limit, offset int) ([]Job, error) {
	return s.store.List(ctx, state, jobType, limit, offset)
}

// Stats delegates to the underlying store.
func (s *Service) Stats(ctx context.Context) (*QueueStats, error) {
	return s.store.Stats(ctx)
}

// Cancel delegates to the underlying store.
func (s *Service) Cancel(ctx context.Context, jobID string) (*Job, error) {
	return s.store.Cancel(ctx, jobID)
}

// RetryNow delegates to the underlying store.
func (s *Service) RetryNow(ctx context.Context, jobID string) (*Job, error) {
	return s.store.RetryNow(ctx, jobID)
}

// CreateSchedule delegates to the underlying store.
func (s *Service) CreateSchedule(ctx context.Context, sched *Schedule) (*Schedule, error) {
	return s.store.CreateSchedule(ctx, sched)
}

// UpsertSchedule inserts or updates a schedule by name (idempotent).
// Use this for registering recurring schedules that should exist exactly once.
func (s *Service) UpsertSchedule(ctx context.Context, sched *Schedule) (*Schedule, error) {
	return s.store.UpsertSchedule(ctx, sched)
}

// GetSchedule delegates to the underlying store.
func (s *Service) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	return s.store.GetSchedule(ctx, id)
}

// GetScheduleByName delegates to the underlying store.
func (s *Service) GetScheduleByName(ctx context.Context, name string) (*Schedule, error) {
	return s.store.GetScheduleByName(ctx, name)
}

// ListSchedules delegates to the underlying store.
func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return s.store.ListSchedules(ctx)
}

// UpdateSchedule delegates to the underlying store.
func (s *Service) UpdateSchedule(ctx context.Context, id string, cronExpr, timezone string, payload json.RawMessage, enabled bool, nextRunAt *time.Time) (*Schedule, error) {
	return s.store.UpdateSchedule(ctx, id, cronExpr, timezone, payload, enabled, nextRunAt)
}

// DeleteSchedule delegates to the underlying store.
func (s *Service) DeleteSchedule(ctx context.Context, id string) error {
	return s.store.DeleteSchedule(ctx, id)
}

// SetScheduleEnabled delegates to the underlying store.
func (s *Service) SetScheduleEnabled(ctx context.Context, id string, enabled bool) (*Schedule, error) {
	var nextRunAt *time.Time
	if enabled {
		sched, err := s.store.GetSchedule(ctx, id)
		if err != nil {
			return nil, err
		}
		t, err := CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			return nil, err
		}
		nextRunAt = &t
	}
	return s.store.SetScheduleEnabled(ctx, id, enabled, nextRunAt)
}

// RegisterDefaultSchedules inserts the built-in schedule definitions (idempotent).
func (s *Service) RegisterDefaultSchedules(ctx context.Context) error {
	return s.RegisterDefaultSchedulesWithAuditRetention(ctx, auditLogRetentionDefaultDays, jobRunsRetentionDefaultDays)
}

// RegisterDefaultSchedulesWithAuditRetention inserts built-in schedules and uses
// the provided retention days in the built-in retention schedule payloads.
// request_log_retention keeps its optional variadic override so the single
// startup caller can thread its non-jobs config owner without parallel seams.
func (s *Service) RegisterDefaultSchedulesWithAuditRetention(ctx context.Context, auditRetentionDays, jobRunsRetentionDays int, requestLogRetentionDays ...int) error {
	if auditRetentionDays <= 0 {
		auditRetentionDays = auditLogRetentionDefaultDays
	}
	if jobRunsRetentionDays <= 0 {
		jobRunsRetentionDays = jobRunsRetentionDefaultDays
	}
	requestLogRetention := requestLogRetentionDefaultDays
	if len(requestLogRetentionDays) > 0 && requestLogRetentionDays[0] > 0 {
		requestLogRetention = requestLogRetentionDays[0]
	}

	defaults := defaultSchedules(auditRetentionDays, jobRunsRetentionDays, requestLogRetention)
	for i := range defaults {
		sched := &defaults[i]
		next, err := CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			return fmt.Errorf("compute next_run_at for %s: %w", sched.Name, err)
		}
		sched.NextRunAt = &next

		if _, err := s.store.UpsertSchedule(ctx, sched); err != nil {
			return fmt.Errorf("upsert default schedule %s: %w", sched.Name, err)
		}
	}
	return nil
}

func defaultSchedules(auditDays, jobRunsDays, requestLogDays int) []Schedule {
	return []Schedule{
		{
			Name:        "session_cleanup_hourly",
			JobType:     "stale_session_cleanup",
			CronExpr:    "0 * * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "webhook_delivery_prune_daily",
			JobType:     "webhook_delivery_prune",
			Payload:     json.RawMessage(`{"retention_hours": 168}`),
			CronExpr:    "0 3 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "expired_oauth_cleanup_daily",
			JobType:     "expired_oauth_cleanup",
			CronExpr:    "0 4 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "expired_auth_cleanup_daily",
			JobType:     "expired_auth_cleanup",
			CronExpr:    "0 5 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        resumableUploadCleanupScheduleName,
			JobType:     resumableUploadCleanupJobType,
			CronExpr:    resumableUploadCleanupCronExpr,
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "audit_log_retention_daily",
			JobType:     "audit_log_retention",
			Payload:     json.RawMessage(fmt.Sprintf(`{"retention_days":%d}`, auditDays)),
			CronExpr:    "0 2 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        jobRunsRetentionScheduleName,
			JobType:     jobRunsRetentionJobType,
			Payload:     json.RawMessage(fmt.Sprintf(`{"retention_days":%d}`, jobRunsDays)),
			CronExpr:    jobRunsRetentionCronExpr,
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        "request_log_retention_daily",
			JobType:     "request_log_retention",
			Payload:     json.RawMessage(fmt.Sprintf(`{"retention_days":%d}`, requestLogDays)),
			CronExpr:    "0 6 * * *",
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
		{
			Name:        moviesReembedScheduleName,
			JobType:     moviesReembedJobType,
			Payload:     json.RawMessage(`{}`),
			CronExpr:    moviesReembedCronExpr,
			Timezone:    "UTC",
			Enabled:     true,
			MaxAttempts: 3,
		},
	}
}
