// Package server movies_byok.go owns the movies demo's Bring-Your-Own-Key
// (BYOK) flow: a single in-memory provider→vault-secret-name mapping on the
// Server struct plus admin handlers to set and clear it. The vault store is
// the durable home for the secret itself; only the binding from provider to
// secret-name is held in memory and is intentionally non-durable across
// restarts. resolveMoviesBYOKKey is the sole reader the demo handlers use.
package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/vault"
	"github.com/go-chi/chi/v5"
)

// knownMoviesProviders enumerates the provider names the demo BYOK flow
// accepts. Anything outside this set is rejected before any vault lookup
// happens to keep operator typos from registering bindings that can never
// resolve to a usable provider.
var knownMoviesProviders = map[string]struct{}{
	"openai":    {},
	"anthropic": {},
	"ollama":    {},
}

type moviesBYOKSetRequest struct {
	Provider   string `json:"provider"`
	SecretName string `json:"secret_name"`
}

// setMoviesBYOK installs (or replaces) the BYOK binding for a provider.
// This is the only writer for s.moviesBYOK so the lock scope stays
// localized; callers must validate provider/secret name first.
func (s *Server) setMoviesBYOK(provider, secretName string) {
	s.moviesBYOKMu.Lock()
	defer s.moviesBYOKMu.Unlock()
	if s.moviesBYOK == nil {
		s.moviesBYOK = make(map[string]string)
	}
	s.moviesBYOK[provider] = secretName
}

// clearMoviesBYOK removes the BYOK binding for a provider. Subsequent
// resolutions for that provider fall back to the registered singleton.
func (s *Server) clearMoviesBYOK(provider string) {
	s.moviesBYOKMu.Lock()
	defer s.moviesBYOKMu.Unlock()
	delete(s.moviesBYOK, provider)
}

// resolveMoviesBYOKKey returns the vault-resolved API key for a BYOK
// provider binding, or ("", nil) when no binding exists. An existing
// binding whose vault secret is unreachable surfaces as a wrapped error
// so the caller can decide whether to fall back or fail the request.
func (s *Server) resolveMoviesBYOKKey(ctx context.Context, provider string) (string, error) {
	s.moviesBYOKMu.RLock()
	secretName, ok := s.moviesBYOK[provider]
	s.moviesBYOKMu.RUnlock()
	if !ok || secretName == "" {
		return "", nil
	}
	if s.vaultStore == nil {
		return "", errors.New("vault store unavailable for BYOK resolution")
	}
	value, err := s.vaultStore.GetSecret(ctx, secretName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

// resolveMoviesProvider resolves a chat/generation provider with BYOK
// applied. It is the sole path for movies demo Provider lookups so the
// BYOK precedence rule (vault secret → registered singleton) is
// centralized rather than duplicated across the search and chat handlers.
func (s *Server) resolveMoviesProvider(ctx context.Context, providerName, model string) (ai.Provider, string, error) {
	byokKey, err := s.resolveMoviesBYOKKey(ctx, providerName)
	if err != nil {
		// Vault error while a binding exists — fail the request rather
		// than silently falling back, so the operator can see the misconfig.
		return nil, "", err
	}
	return ai.ResolveProvider(s.aiRegistry, providerName, model, byokKey, s.cfg.AI)
}

// handleMoviesBYOKSet installs a vault-backed BYOK binding for a provider.
// Validation order: vault available → secret name shape → provider name →
// secret exists. Each step keeps the failure mode obvious to the operator.
func (s *Server) handleMoviesBYOKSet(w http.ResponseWriter, r *http.Request) {
	if s.vaultStore == nil {
		serviceUnavailable(w, serviceUnavailableMovies)
		return
	}
	var body moviesBYOKSetRequest
	if !httputil.DecodeJSON(w, r, &body) {
		return
	}
	secretName, err := vault.NormalizeSecretName(body.SecretName)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid secret_name")
		return
	}
	provider := strings.TrimSpace(strings.ToLower(body.Provider))
	if _, ok := knownMoviesProviders[provider]; !ok {
		httputil.WriteError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	if _, err := s.vaultStore.GetSecret(r.Context(), secretName); err != nil {
		if errors.Is(err, vault.ErrSecretNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "vault secret not found")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "vault lookup failed")
		return
	}
	s.setMoviesBYOK(provider, secretName)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"provider":    provider,
		"secret_name": secretName,
	})
}

// handleMoviesBYOKClear removes a BYOK binding. It is idempotent — clearing
// an unbound provider returns 204 just like clearing a bound one — so
// scripted teardown does not have to special-case missing entries.
func (s *Server) handleMoviesBYOKClear(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(strings.ToLower(chi.URLParam(r, "provider")))
	if _, ok := knownMoviesProviders[provider]; !ok {
		httputil.WriteError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	s.clearMoviesBYOK(provider)
	w.WriteHeader(http.StatusNoContent)
}
