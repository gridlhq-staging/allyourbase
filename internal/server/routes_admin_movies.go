package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// registerAdminMoviesRoutes mounts admin-gated routes for the movies demo:
// vector search, notes embedding, BYOK lifecycle, and SSE chat streaming.
// All endpoints sit under /admin/movies and reuse requireAdminToken plus
// the existing assistantRateLimitMiddleware so per-user rate limiting
// remains a single source of truth across AI-facing demo routes.
func (s *Server) registerAdminMoviesRoutes(r chi.Router) {
	r.Route("/admin/movies", func(r chi.Router) {
		r.Use(s.requireAdminToken)

		jsonOnly := middleware.AllowContentType("application/json")
		assistantLimiter := s.assistantRateLimitMiddleware()

		r.With(jsonOnly).Post("/search", s.handleMoviesSearch)
		r.With(jsonOnly).Post("/notes/embed", s.handleMoviesNotesEmbed)

		r.With(jsonOnly).Post("/byok", s.handleMoviesBYOKSet)
		r.Delete("/byok/{provider}", s.handleMoviesBYOKClear)

		r.With(jsonOnly, assistantLimiter).Post("/chat/stream", s.handleMoviesChatStream)
	})
}
