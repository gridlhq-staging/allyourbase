package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/vector"
	"github.com/jackc/pgx/v5/pgxpool"
)

type providerTokenRefreshPayload struct {
	WindowSeconds int `json:"window_seconds"`
}
type aiUsageAggregationPayload struct {
	Day string `json:"day"` // optional YYYY-MM-DD UTC; defaults to yesterday UTC
}

// webhookPrunePayload is the expected payload for webhook_delivery_prune jobs.
type webhookPrunePayload struct {
	RetentionHours int `json:"retention_hours"`
}
type auditRetentionPayload struct {
	RetentionDays int `json:"retention_days"`
}
type jobRunsRetentionPayload struct {
	RetentionDays int `json:"retention_days"`
}
type requestLogRetentionPayload struct {
	RetentionDays int `json:"retention_days"`
}

func moviesReembedUpdateSQL() string {
	return `
				UPDATE movies
				SET embedding = $2::vector,
				    updated_at = NOW()
				WHERE slug = $1
				  AND embedding IS DISTINCT FROM $2::vector`
}

func hasMoviesDemoSchemaContract(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	const contractQuery = `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'movies'
			GROUP BY table_schema, table_name
			HAVING
				bool_or(column_name = 'id' AND data_type = 'uuid') AND
				bool_or(column_name = 'slug' AND data_type = 'text') AND
				bool_or(column_name = 'title' AND data_type = 'text') AND
				bool_or(column_name = 'overview' AND data_type = 'text') AND
				bool_or(column_name = 'release_year' AND data_type = 'integer') AND
				bool_or(column_name = 'genres' AND data_type = 'ARRAY' AND udt_name = '_text') AND
				bool_or(column_name = 'embedding' AND data_type = 'USER-DEFINED' AND udt_name = 'vector') AND
				bool_or(column_name = 'created_at' AND data_type = 'timestamp with time zone') AND
				bool_or(column_name = 'updated_at' AND data_type = 'timestamp with time zone')
		)`
	var hasContract bool
	if err := pool.QueryRow(ctx, contractQuery).Scan(&hasContract); err != nil {
		return false, err
	}
	return hasContract, nil
}

// hasMoviesDemoCorpusIdentity verifies that the live `movies` rows still match
// the committed demo corpus identities exactly. The reembed job is demo-owned;
// if a user created or repurposed a same-shaped `public.movies` table, the job
// must skip rather than silently rewriting that table's embeddings.
func hasMoviesDemoCorpusIdentity(ctx context.Context, pool *pgxpool.Pool, artifact vector.MoviesEmbeddingArtifact) (bool, error) {
	rows, err := pool.Query(ctx, `SELECT id::text, slug, title FROM movies ORDER BY slug`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	idx := 0
	for rows.Next() {
		if idx >= len(artifact.Records) {
			return false, nil
		}

		var id, slug, title string
		if err := rows.Scan(&id, &slug, &title); err != nil {
			return false, err
		}

		rec := artifact.Records[idx]
		if id != rec.ID || slug != rec.Slug || title != rec.Title {
			return false, nil
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return idx == len(artifact.Records), nil
}

func AIUsageAggregationJobHandler(aggregator AIUsageAggregator) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		targetDay := time.Now().UTC().AddDate(0, 0, -1)
		targetDay = time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), 0, 0, 0, 0, time.UTC)
		if hasJobPayload(payload) {
			var p aiUsageAggregationPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("ai_usage_aggregate_daily: invalid payload: %w", err)
			}
			if p.Day != "" {
				parsed, err := time.Parse("2006-01-02", p.Day)
				if err != nil {
					return fmt.Errorf("ai_usage_aggregate_daily: invalid day %q: %w", p.Day, err)
				}
				targetDay = parsed.UTC()
			}
		}
		if _, err := aggregator.AggregateDailyUsage(ctx, targetDay); err != nil {
			return fmt.Errorf("ai_usage_aggregate_daily: %w", err)
		}
		return nil
	}
}

// BillingUsageSyncJobHandler reports metered usage deltas for billable tenants.
func BillingUsageSyncJobHandler(billingSvc billing.BillingService, ds billingUsageSyncDataSource) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		_ = payload
		if billingSvc == nil {
			return fmt.Errorf("billing service is nil")
		}
		if ds == nil {
			return fmt.Errorf("billing usage sync data source is nil")
		}
		targetDate := usageSyncNow().UTC().Truncate(24 * time.Hour)
		tenantIDs, err := ds.ListBillableTenants(ctx)
		if err != nil {
			return fmt.Errorf("list billable tenants: %w", err)
		}
		successes := 0
		failures := 0
		for _, tenantID := range tenantIDs {
			report, found, err := ds.GetUsageReport(ctx, tenantID, targetDate)
			if err != nil {
				failures++
				slog.Default().Error("failed to query tenant usage", "tenant_id", tenantID, "error", err)
				continue
			}
			if !found {
				yesterday := targetDate.AddDate(0, 0, -1)
				report, found, err = ds.GetUsageReport(ctx, tenantID, yesterday)
				if err != nil {
					failures++
					slog.Default().Error("failed to query tenant usage", "tenant_id", tenantID, "error", err)
					continue
				}
				if !found {
					slog.Default().Debug("no usage row for tenant in sync window", "tenant_id", tenantID)
					continue
				}
			}
			if err := billingSvc.ReportUsage(ctx, tenantID, report); err != nil {
				failures++
				slog.Default().Error("billing usage report failed", "tenant_id", tenantID, "error", err)
				continue
			}
			successes++
		}
		slog.Default().Info("billing usage sync summary",
			"tenants", len(tenantIDs),
			"success", successes,
			"failed", failures)
		return nil
	}
}

// ProviderTokenRefreshJobHandler refreshes OAuth provider tokens nearing expiration.
func ProviderTokenRefreshJobHandler(refresher ProviderTokenRefreshService) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		window := providerTokenRefreshDefaultWindow
		if hasJobPayload(payload) {
			var p providerTokenRefreshPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("oauth_provider_tokens_refresh: invalid payload: %w", err)
			}
			if p.WindowSeconds > 0 {
				window = time.Duration(p.WindowSeconds) * time.Second
			}
		}
		return refresher.RefreshExpiringProviderTokens(ctx, window)
	}
}

// StaleSessionCleanupHandler deletes expired refresh-token sessions.
func StaleSessionCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		tag, err := pool.Exec(ctx,
			`DELETE FROM _ayb_sessions WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("stale_session_cleanup: %w", err)
		}
		logger.Info("stale_session_cleanup completed", "deleted", tag.RowsAffected())
		return nil
	}
}

func WebhookDeliveryPruneHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p webhookPrunePayload
		if hasJobPayload(payload) {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("webhook_delivery_prune: invalid payload: %w", err)
			}
		}
		if p.RetentionHours <= 0 {
			p.RetentionHours = 168 // 7 days default
		}
		tag, err := pool.Exec(ctx,
			`DELETE FROM _ayb_webhook_deliveries
			 WHERE delivered_at < NOW() - make_interval(hours => $1)`,
			p.RetentionHours)
		if err != nil {
			return fmt.Errorf("webhook_delivery_prune: %w", err)
		}
		logger.Info("webhook_delivery_prune completed",
			"deleted", tag.RowsAffected(), "retention_hours", p.RetentionHours)
		return nil
	}
}

// ExpiredOAuthCleanupHandler deletes expired/revoked OAuth tokens and used auth codes.
func ExpiredOAuthCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		// Delete expired OAuth tokens (expired > 1 day ago).
		tagTokens, err := pool.Exec(ctx,
			`DELETE FROM _ayb_oauth_tokens
			 WHERE (expires_at < NOW() - interval '1 day')
			    OR (revoked_at IS NOT NULL AND revoked_at < NOW() - interval '1 day')`)
		if err != nil {
			return fmt.Errorf("expired_oauth_cleanup tokens: %w", err)
		}
		// Delete expired authorization codes.
		tagCodes, err := pool.Exec(ctx,
			`DELETE FROM _ayb_oauth_authorization_codes
			 WHERE expires_at < NOW()
			    OR (used_at IS NOT NULL AND used_at < NOW() - interval '1 day')`)
		if err != nil {
			return fmt.Errorf("expired_oauth_cleanup codes: %w", err)
		}
		logger.Info("expired_oauth_cleanup completed",
			"tokens_deleted", tagTokens.RowsAffected(),
			"codes_deleted", tagCodes.RowsAffected())
		return nil
	}
}

// ExpiredAuthCleanupHandler deletes expired magic links and password resets.
func ExpiredAuthCleanupHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		tagLinks, err := pool.Exec(ctx,
			`DELETE FROM _ayb_magic_links WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("expired_auth_cleanup magic_links: %w", err)
		}
		tagResets, err := pool.Exec(ctx,
			`DELETE FROM _ayb_password_resets WHERE expires_at < NOW()`)
		if err != nil {
			return fmt.Errorf("expired_auth_cleanup password_resets: %w", err)
		}
		logger.Info("expired_auth_cleanup completed",
			"magic_links_deleted", tagLinks.RowsAffected(),
			"password_resets_deleted", tagResets.RowsAffected())
		return nil
	}
}

func ResumableUploadCleanupHandler(storageSvc *storage.Service, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		if storageSvc == nil {
			return fmt.Errorf("cleanup resumable uploads: storage service is nil")
		}
		deleted, err := storageSvc.CleanupExpiredResumableUploads(ctx)
		if err != nil {
			return fmt.Errorf("cleanup resumable uploads: %w", err)
		}
		if logger != nil {
			logger.Info("cleanup resumable uploads completed", "deleted", deleted)
		}
		return nil
	}
}

func AuditLogRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = auditLogRetentionDefaultDays
		}
		var p auditRetentionPayload
		if hasJobPayload(payload) {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("audit_log_retention: invalid payload: %w", err)
			}
			if p.RetentionDays > 0 {
				retentionDays = p.RetentionDays
			}
		}
		tag, err := pool.Exec(ctx, `
			DELETE FROM _ayb_audit_log
			 WHERE timestamp < NOW() - make_interval(days => $1)`,
			retentionDays)
		if err != nil {
			return fmt.Errorf("audit_log_retention: %w", err)
		}
		if logger != nil {
			logger.Info("audit_log_retention completed",
				"deleted", tag.RowsAffected(),
				"retention_days", retentionDays)
		}
		return nil
	}
}

func JobRunsRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = jobRunsRetentionDefaultDays
		}
		var p jobRunsRetentionPayload
		if hasJobPayload(payload) {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("job_runs_retention: invalid payload: %w", err)
			}
			if p.RetentionDays > 0 {
				retentionDays = p.RetentionDays
			}
		}
		// finished_at is the stable terminal timestamp for persisted run history rows.
		tag, err := pool.Exec(ctx, `
			DELETE FROM _ayb_job_runs
			 WHERE finished_at < NOW() - make_interval(days => $1)`,
			retentionDays)
		if err != nil {
			return fmt.Errorf("job_runs_retention: %w", err)
		}
		if logger != nil {
			logger.Info("job_runs_retention completed",
				"deleted", tag.RowsAffected(),
				"retention_days", retentionDays)
		}
		return nil
	}
}

func RequestLogRetentionHandler(pool *pgxpool.Pool, defaultRetentionDays int, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		retentionDays := defaultRetentionDays
		if retentionDays <= 0 {
			retentionDays = requestLogRetentionDefaultDays
		}
		var p requestLogRetentionPayload
		if hasJobPayload(payload) {
			if err := json.Unmarshal(payload, &p); err != nil {
				return fmt.Errorf("request_log_retention: invalid payload: %w", err)
			}
			if p.RetentionDays > 0 {
				retentionDays = p.RetentionDays
			}
		}
		tag, err := pool.Exec(ctx, `
			DELETE FROM _ayb_request_logs
			 WHERE timestamp < NOW() - make_interval(days => $1)`,
			retentionDays)
		if err != nil {
			return fmt.Errorf("request_log_retention: %w", err)
		}
		if logger != nil {
			logger.Info("request_log_retention completed",
				"deleted", tag.RowsAffected(),
				"retention_days", retentionDays)
		}
		return nil
	}
}

// MoviesReembedHandler repairs movie embeddings from the committed demo artifact.
func MoviesReembedHandler(pool *pgxpool.Pool, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		if pool == nil {
			return fmt.Errorf("movies_reembed: pool is nil")
		}

		hasContract, err := hasMoviesDemoSchemaContract(ctx, pool)
		if err != nil {
			return fmt.Errorf("movies_reembed: verify movies demo schema contract: %w", err)
		}
		if !hasContract {
			if logger != nil {
				logger.Info("movies_reembed skipped: movies table does not match demo schema contract")
			}
			return nil
		}

		seedBytes, err := fs.ReadFile(examples.FS, "movies/seed.sql")
		if err != nil {
			return fmt.Errorf("movies_reembed: read embedded seed.sql: %w", err)
		}
		artifactBytes, err := fs.ReadFile(examples.FS, "movies/embeddings.json")
		if err != nil {
			return fmt.Errorf("movies_reembed: read embedded embeddings.json: %w", err)
		}
		artifact, err := vector.LoadCommittedMoviesEmbeddingArtifact(seedBytes, artifactBytes)
		if err != nil {
			return fmt.Errorf("movies_reembed: load committed embedding artifact: %w", err)
		}
		hasCorpusIdentity, err := hasMoviesDemoCorpusIdentity(ctx, pool, artifact)
		if err != nil {
			return fmt.Errorf("movies_reembed: verify movies demo corpus identity: %w", err)
		}
		if !hasCorpusIdentity {
			if logger != nil {
				logger.Info("movies_reembed skipped: movies rows do not match committed demo corpus")
			}
			return nil
		}

		repairedRows := int64(0)
		for _, rec := range artifact.Records {
			tag, err := pool.Exec(ctx, moviesReembedUpdateSQL(), rec.Slug, vector.FormatVectorLiteral(rec.Embedding))
			if err != nil {
				return fmt.Errorf("movies_reembed: update slug %q: %w", rec.Slug, err)
			}
			repairedRows += tag.RowsAffected()
		}

		if logger != nil {
			logger.Info("movies_reembed completed",
				"artifact_rows", len(artifact.Records),
				"repaired_rows", repairedRows)
		}
		return nil
	}
}

type AnonymousUserCleaner interface {
	CleanupAnonymousUsers(ctx context.Context, ttl time.Duration) (int64, error)
}

func AnonymousUserCleanupHandler(cleaner AnonymousUserCleaner, logger *slog.Logger) JobHandler {
	return func(ctx context.Context, _ json.RawMessage) error {
		if cleaner == nil {
			return fmt.Errorf("anonymous_user_cleanup: cleaner is nil")
		}
		deleted, err := cleaner.CleanupAnonymousUsers(ctx, auth.DefaultAnonymousTTL)
		if err != nil {
			return fmt.Errorf("anonymous_user_cleanup: %w", err)
		}
		if logger != nil {
			logger.Info("anonymous_user_cleanup completed", "deleted", deleted)
		}
		return nil
	}
}
