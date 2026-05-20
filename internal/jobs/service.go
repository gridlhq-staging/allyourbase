package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/adhocore/gronx"
)

// ServiceConfig holds runtime parameters for the job service.
type ServiceConfig struct {
	WorkerConcurrency int
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	SchedulerEnabled  bool
	SchedulerTick     time.Duration
	ShutdownTimeout   time.Duration
	WorkerID          string // unique identifier for this instance
}

// DefaultServiceConfig returns production defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		WorkerConcurrency: 4,
		PollInterval:      1 * time.Second,
		LeaseDuration:     5 * time.Minute,
		SchedulerEnabled:  true,
		SchedulerTick:     15 * time.Second,
		ShutdownTimeout:   30 * time.Second,
		WorkerID:          fmt.Sprintf("worker-%d", time.Now().UnixNano()),
	}
}

// Service orchestrates the job queue: worker loop, scheduler, handler dispatch.
type Service struct {
	store    *Store
	logger   *slog.Logger
	cfg      ServiceConfig
	handlers map[string]JobHandler
	mu       sync.RWMutex // protects handlers

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewService creates a new job Service.
func NewService(store *Store, logger *slog.Logger, cfg ServiceConfig) *Service {
	return &Service{
		store:    store,
		logger:   logger,
		cfg:      cfg,
		handlers: make(map[string]JobHandler),
	}
}

// RegisterHandler registers a handler for a job type.
func (s *Service) RegisterHandler(jobType string, handler JobHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[jobType] = handler
}

// Start launches worker goroutines and the scheduler loop.
func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	// Start worker goroutines.
	for i := 0; i < s.cfg.WorkerConcurrency; i++ {
		s.wg.Add(1)
		go s.workerLoop(ctx, i)
	}

	// Start scheduler goroutine when enabled.
	if s.cfg.SchedulerEnabled {
		s.wg.Add(1)
		go s.schedulerLoop(ctx)
	}

	// Start crash recovery goroutine.
	s.wg.Add(1)
	go s.recoveryLoop(ctx)

	s.logger.Info("job service started",
		"workers", s.cfg.WorkerConcurrency,
		"poll_interval", s.cfg.PollInterval,
		"scheduler_enabled", s.cfg.SchedulerEnabled,
		"scheduler_tick", s.cfg.SchedulerTick,
	)
}

// Stop signals all goroutines to stop and waits for in-progress jobs to finish.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Info("job service stopped")
}

func (s *Service) workerLoop(ctx context.Context, workerNum int) {
	defer s.wg.Done()
	workerID := fmt.Sprintf("%s-%d", s.cfg.WorkerID, workerNum)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollAndProcess(ctx, workerID)
		}
	}
}

// pollAndProcess claims the next available job from the queue, dispatches it to the registered handler with lease renewal, and records the success or failure result.
func (s *Service) pollAndProcess(ctx context.Context, workerID string) {
	job, err := s.store.Claim(ctx, workerID, s.cfg.LeaseDuration)
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down
		}
		s.logger.Error("failed to claim job", "error", err, "worker", workerID)
		return
	}
	if job == nil {
		return // no jobs available
	}

	s.logger.Info("claimed job", "job_id", job.ID, "type", job.Type,
		"attempt", job.Attempts, "worker", workerID)

	s.mu.RLock()
	handler, ok := s.handlers[job.Type]
	s.mu.RUnlock()

	// Use a separate context for handler execution so that in-flight jobs
	// can finish their DB operations during graceful shutdown. The poll loop's
	// ctx may already be cancelled, but the handler needs a live context to
	// complete or fail the job cleanly. With lease renewal the handler is no
	// longer hard-capped at the lease duration — the shutdown timeout bounds
	// total in-flight execution instead.
	handlerCtx, handlerCancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer handlerCancel()

	// Start lease renewal goroutine. It extends the lease every half-period
	// so crash recovery won't reclaim the job while the handler is still running.
	renewCtx, renewCancel := context.WithCancel(handlerCtx)
	defer renewCancel()
	go s.renewLease(renewCtx, job.ID)

	var jobErr error
	if !ok {
		jobErr = fmt.Errorf("no handler registered for job type %q", job.Type)
	} else {
		jobErr = handler(handlerCtx, job.Payload)
	}

	// Stop lease renewal before updating final state.
	renewCancel()

	// If the handler context expired (ShutdownTimeout) but the handler did not
	// propagate the cancellation error, treat the job as timed-out rather than
	// completed — the handler's nil return is unreliable when its context has
	// been cancelled.
	if jobErr == nil && handlerCtx.Err() != nil {
		jobErr = handlerCtx.Err()
	}

	// If the handler timed out, the handler context is already cancelled and
	// cannot be used for terminal state persistence. Use a short-lived fresh
	// context so failed/completed state is still durably recorded.
	persistCtx := handlerCtx
	persistCancel := func() {}
	if handlerCtx.Err() != nil {
		persistCtx, persistCancel = context.WithTimeout(context.Background(), 5*time.Second)
	}
	defer persistCancel()

	if jobErr != nil {
		backoff := ComputeBackoff(job.Attempts)
		_, failErr := s.store.Fail(persistCtx, job.ID, jobErr.Error(), backoff)
		if failErr != nil {
			s.logger.Error("failed to record job failure",
				"job_id", job.ID, "error", failErr)
		} else {
			s.logger.Warn("job failed", "job_id", job.ID, "type", job.Type,
				"attempt", job.Attempts, "error", jobErr.Error())
		}
		return
	}

	_, completeErr := s.store.Complete(persistCtx, job.ID)
	if completeErr != nil {
		s.logger.Error("failed to complete job",
			"job_id", job.ID, "error", completeErr)
	} else {
		s.logger.Info("job completed", "job_id", job.ID, "type", job.Type)
	}
}

// renewLease periodically extends a running job's lease until the context is cancelled.
// The renewal interval is half the configured lease duration, ensuring the lease is
// always refreshed well before expiry.
func (s *Service) renewLease(ctx context.Context, jobID string) {
	interval := s.cfg.LeaseDuration / 2
	if interval < 1*time.Second {
		interval = 1 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := s.store.ExtendLease(ctx, jobID, s.cfg.LeaseDuration)
			if err != nil {
				if ctx.Err() != nil {
					return // cancelled, expected during completion
				}
				s.logger.Error("failed to extend lease",
					"job_id", jobID, "error", err)
			}
		}
	}
}

func (s *Service) schedulerLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.SchedulerTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.schedulerTick(ctx)
		}
	}
}

// Processes all due schedules by computing their next run times from cron expressions and enqueuing corresponding jobs.
func (s *Service) schedulerTick(ctx context.Context) {
	schedules, err := s.store.DueSchedules(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger.Error("failed to fetch due schedules", "error", err)
		return
	}

	for i := range schedules {
		sched := &schedules[i]
		nextRunAt, err := CronNextTime(sched.CronExpr, sched.Timezone, time.Now())
		if err != nil {
			s.logger.Error("failed to compute next run time",
				"schedule", sched.Name, "cron", sched.CronExpr, "error", err)
			continue
		}

		advanced, err := s.store.AdvanceScheduleAndEnqueue(
			ctx, sched.ID, nextRunAt, sched.JobType, sched.Payload, sched.MaxAttempts,
		)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("failed to advance schedule and enqueue job",
				"schedule", sched.Name, "error", err)
			continue
		}
		if !advanced {
			continue // another instance handled this tick
		}

		s.logger.Info("enqueued scheduled job",
			"schedule", sched.Name, "type", sched.JobType, "next_run", nextRunAt)
	}
}

// Periodically recovers stalled jobs whose leases have expired, moving them back to the ready state for retry.
func (s *Service) recoveryLoop(ctx context.Context) {
	defer s.wg.Done()
	// Run recovery at the lease duration interval (minimum 30s).
	interval := s.cfg.LeaseDuration
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := s.store.RecoverStalledJobs(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				s.logger.Error("failed to recover stalled jobs", "error", err)
				continue
			}
			if recovered > 0 {
				s.logger.Info("recovered stalled jobs", "count", recovered)
			}
		}
	}
}

// CronNextTime computes the next run time for a cron expression after refTime in the given timezone.
func CronNextTime(cronExpr, tz string, refTime time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}

	gron := gronx.New()
	if !gron.IsValid(cronExpr) {
		return time.Time{}, fmt.Errorf("invalid cron expression %q", cronExpr)
	}

	// Convert ref to the target timezone for computation.
	refInTZ := refTime.In(loc)
	next, err := gronx.NextTickAfter(cronExpr, refInTZ, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to compute next tick for %q: %w", cronExpr, err)
	}

	return next.UTC(), nil
}
