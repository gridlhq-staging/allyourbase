// Package server Provides HTTP middleware for request logging, CORS, security headers, rate limiting, IP allowlisting, and serves the embedded admin SPA with path rewriting.
package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
)

// corsMiddleware returns middleware that sets CORS headers.
// Per the spec, Access-Control-Allow-Origin must be either "*" or a single
// origin. When multiple origins are configured, the middleware echoes back
// only the matching origin and adds Vary: Origin so caches key correctly.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	wildcard := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				if _, ok := originSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions && !passesThroughCORSPreflight(r) {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func passesThroughCORSPreflight(r *http.Request) bool {
	return r != nil && strings.TrimSpace(r.Header.Get("Tus-Resumable")) != ""
}

// securityHeaders adds standard security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")

		// HSTS: only advertise when the request arrived over TLS (direct or
		// via a trusted reverse proxy that sets X-Forwarded-Proto).
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// --- Rate limiting and IP allowlist middleware ---

func newIPAllowlist(section string, entries []string, logger *slog.Logger) *httputil.IPAllowlist {
	allowlist, err := httputil.NewIPAllowlist(entries)
	if err != nil {
		logger.Error("invalid ip allowlist configuration", "section", section, "error", err)
		return nil
	}
	return allowlist
}

// apiRouteAllowlistMiddleware returns HTTP middleware that applies IP allowlists
// to API routes, using adminAllowlist for paths under /api/admin and
// serverAllowlist for all other API paths.
func apiRouteAllowlistMiddleware(serverAllowlist, adminAllowlist *httputil.IPAllowlist) func(http.Handler) http.Handler {
	serverMiddleware := apiRouteMiddleware(serverAllowlist)
	adminMiddleware := apiRouteMiddleware(adminAllowlist)

	return func(next http.Handler) http.Handler {
		serverHandler := serverMiddleware(next)
		adminHandler := adminMiddleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAdminAPIPath(r.URL.Path) {
				adminHandler.ServeHTTP(w, r)
				return
			}
			serverHandler.ServeHTTP(w, r)
		})
	}
}

func isAdminAPIPath(path string) bool {
	return path == "/api/admin" || strings.HasPrefix(path, "/api/admin/")
}

func apiRouteMiddleware(allowlist *httputil.IPAllowlist) func(http.Handler) http.Handler {
	if allowlist == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return allowlist.Middleware
}

// authRouteRateLimitMiddleware returns HTTP middleware that applies rate limiting
// to authentication routes, using a stricter limiter for sensitive endpoints
// like login and register, and a more lenient limiter for other auth paths.
func authRouteRateLimitMiddleware(general, sensitive *auth.RateLimiter) func(http.Handler) http.Handler {
	if general == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	generalMiddleware := general.Middleware
	sensitiveMiddleware := generalMiddleware
	if sensitive != nil {
		sensitiveMiddleware = sensitive.Middleware
	}

	return func(next http.Handler) http.Handler {
		sensitiveHandler := sensitiveMiddleware(next)
		generalHandler := generalMiddleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSensitiveAuthPath(r.URL.Path) {
				sensitiveHandler.ServeHTTP(w, r)
				return
			}
			generalHandler.ServeHTTP(w, r)
		})
	}
}

func isSensitiveAuthPath(path string) bool {
	switch path {
	case "/api/auth/register", "/api/auth/magic-link", "/api/auth/sms", "/api/auth/sms/confirm":
		return true
	}
	if strings.HasPrefix(path, "/api/auth/mfa/") && strings.HasSuffix(path, "/verify") {
		return true
	}
	return false
}

// APIRouteRateLimitMiddleware returns HTTP middleware that applies per-request
// rate limiting based on authentication status, using the authenticated limiter
// for JWT or API-key bearers and the anonymous limiter for unauthenticated
// requests. Sets X-RateLimit headers in all responses.
func APIRouteRateLimitMiddleware(authenticated, anonymous *auth.RateLimiter, authLimit, anonymousLimit int) func(http.Handler) http.Handler {
	if authenticated == nil && anonymous == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rl := anonymous
			limit := anonymousLimit
			key := httputil.ClientIP(r)
			if claims := auth.ClaimsFromContext(r.Context()); claims != nil && claims.Subject != "" && authenticated != nil {
				rl = authenticated
				limit = authLimit
				key = claims.Subject
			}
			if rl == nil {
				next.ServeHTTP(w, r)
				return
			}

			allowed, remaining, resetTime := rl.Allow(key)
			if !handleRateLimitDecision(w, limit, allowed, remaining, resetTime) {
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// handleRateLimitDecision sets X-RateLimit response headers and, if the request is not allowed, writes a 429 response with a Retry-After header and returns false.
func handleRateLimitDecision(w http.ResponseWriter, limit int, allowed bool, remaining int, resetTime time.Time) bool {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	if allowed {
		return true
	}

	retryAfter := int(time.Until(resetTime).Seconds()) + 1
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	httputil.WriteError(w, http.StatusTooManyRequests, "too many requests")
	return false
}
