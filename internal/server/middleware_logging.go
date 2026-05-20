// Package server contains request logging middleware and drain fanout helpers.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger returns middleware that logs each request as structured JSON.
func requestLogger(loggerProvider func() *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := loggerProvider()
			if logger == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}
				fields := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"duration_ms", time.Since(start).Milliseconds(),
					"bytes", ww.BytesWritten(),
					"request_id", middleware.GetReqID(r.Context()),
					"remote", r.RemoteAddr,
				}
				// Include tenant_id in logs when tenant context or request tenant source is present.
				if tenantID := tenantIDFromContextOrRequest(r); tenantID != "" {
					fields = append(fields, "tenant_id", tenantID)
				}
				for k, v := range observability.TraceLogFields(r.Context()) {
					fields = append(fields, k, v)
				}
				logger.Info("request", fields...)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// wrapLoggerForDrainFanout returns a logger that sends every record to the log drain manager,
// while preserving the existing handler behavior.
func wrapLoggerForDrainFanout(base *slog.Logger, manager *logging.DrainManager) *slog.Logger {
	if base == nil || manager == nil {
		return base
	}
	return slog.New(&drainSlogHandler{next: base.Handler(), drainManager: manager})
}

// drainSlogHandler forwards slog records to an external drain manager.
type drainSlogHandler struct {
	next         slog.Handler
	drainManager *logging.DrainManager
	preAttrs     []slog.Attr
	groupPrefix  string
}

func (h *drainSlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle collects attributes from the slog record, applies group prefix namespacing, enqueues the resulting entry to the drain manager, and forwards the record to the next handler in the chain.
func (h *drainSlogHandler) Handle(ctx context.Context, record slog.Record) error {
	fields := make(map[string]any)
	// Include pre-set attrs from WithAttrs calls (e.g. slog.With("component", "auth")).
	for _, a := range h.preAttrs {
		key := a.Key
		if h.groupPrefix != "" {
			key = h.groupPrefix + "." + key
		}
		fields[key] = a.Value.Resolve().Any()
	}
	record.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if h.groupPrefix != "" {
			key = h.groupPrefix + "." + key
		}
		fields[key] = a.Value.Resolve().Any()
		return true
	})

	h.drainManager.Enqueue(logging.LogEntry{
		Timestamp: record.Time,
		Level:     strings.ToLower(record.Level.String()),
		Message:   record.Message,
		Source:    "app",
		Fields:    fields,
	})

	return h.next.Handle(ctx, record)
}

func (h *drainSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.preAttrs)+len(attrs))
	copy(combined, h.preAttrs)
	copy(combined[len(h.preAttrs):], attrs)
	return &drainSlogHandler{
		next:         h.next.WithAttrs(attrs),
		drainManager: h.drainManager,
		preAttrs:     combined,
		groupPrefix:  h.groupPrefix,
	}
}

func (h *drainSlogHandler) WithGroup(name string) slog.Handler {
	prefix := name
	if h.groupPrefix != "" {
		prefix = h.groupPrefix + "." + name
	}
	return &drainSlogHandler{
		next:         h.next.WithGroup(name),
		drainManager: h.drainManager,
		preAttrs:     h.preAttrs,
		groupPrefix:  prefix,
	}
}

// requestLogMiddleware records each request as a RequestLogEntry via the async RequestLogger.
func requestLogMiddleware(rl *RequestLogger, drainManagerProvider func() *logging.DrainManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			entry := RequestLogEntry{
				Method:       r.Method,
				Path:         r.URL.Path,
				StatusCode:   status,
				DurationMS:   time.Since(start).Milliseconds(),
				RequestSize:  normalizedRequestSize(r.ContentLength),
				ResponseSize: int64(ww.BytesWritten()),
				RequestID:    middleware.GetReqID(r.Context()),
			}

			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				entry.IPAddress = host
			} else {
				entry.IPAddress = r.RemoteAddr
			}

			if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
				entry.UserID = claims.Subject
				entry.APIKeyID = claims.APIKeyID
			}

			// Include tenant_id in request logs when tenant context or request tenant source is present.
			if tenantID := tenantIDFromContextOrRequest(r); tenantID != "" {
				entry.TenantID = tenantID
			}

			if rl != nil {
				rl.Log(entry)
			}
			if dm := drainManagerProvider(); dm != nil {
				drainEntry := logEntryToDrain(entry)
				for k, v := range observability.TraceLogFields(r.Context()) {
					drainEntry.Fields[k] = v
				}
				dm.Enqueue(drainEntry)
			}
		})
	}
}

func normalizedRequestSize(contentLength int64) int64 {
	if contentLength < 0 {
		return 0
	}
	return contentLength
}

// logEntryToDrain converts a RequestLogEntry to a logging.LogEntry, mapping HTTP request metadata and excluding empty optional fields.
func logEntryToDrain(entry RequestLogEntry) logging.LogEntry {
	fields := map[string]any{
		"method":        entry.Method,
		"path":          entry.Path,
		"status":        entry.StatusCode,
		"duration_ms":   entry.DurationMS,
		"request_size":  entry.RequestSize,
		"response_size": entry.ResponseSize,
	}
	if entry.UserID != "" {
		fields["user_id"] = entry.UserID
	}
	if entry.APIKeyID != "" {
		fields["api_key_id"] = entry.APIKeyID
	}
	if entry.RequestID != "" {
		fields["request_id"] = entry.RequestID
	}
	if entry.IPAddress != "" {
		fields["ip_address"] = entry.IPAddress
	}
	if entry.TenantID != "" {
		fields["tenant_id"] = entry.TenantID
	}

	return logging.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     "info",
		Message:   fmt.Sprintf("%s %s", entry.Method, entry.Path),
		Source:    "request",
		Fields:    fields,
	}
}
