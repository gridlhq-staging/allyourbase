package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/vault"
	"github.com/go-chi/chi/v5"
)

// withChiURLParam attaches a chi URL parameter onto an existing request's context.
func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// sliceReadCloser yields predefined chunks across successive Read calls so a
// streaming-provider stub can deliver multiple SSE chunks deterministically.
type sliceReadCloser struct {
	chunks [][]byte
	idx    int
}

func newSliceReadCloser(chunks []string) io.ReadCloser {
	bs := make([][]byte, len(chunks))
	for i, c := range chunks {
		bs[i] = []byte(c)
	}
	return &sliceReadCloser{chunks: bs}
}

func (s *sliceReadCloser) Read(p []byte) (int, error) {
	if s.idx >= len(s.chunks) {
		return 0, io.EOF
	}
	n := copy(p, s.chunks[s.idx])
	s.idx++
	return n, nil
}

func (s *sliceReadCloser) Close() error { return nil }

// --- fakes used by movies handler unit tests ---

// fakeEmbeddingProvider satisfies ai.EmbeddingProvider with a deterministic stub vector.
type fakeEmbeddingProvider struct {
	calls int
	resp  ai.EmbeddingResponse
	err   error
}

func (f *fakeEmbeddingProvider) GenerateText(_ context.Context, _ ai.GenerateTextRequest) (ai.GenerateTextResponse, error) {
	return ai.GenerateTextResponse{}, nil
}

func (f *fakeEmbeddingProvider) GenerateEmbedding(_ context.Context, _ ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	f.calls++
	if f.err != nil {
		return ai.EmbeddingResponse{}, f.err
	}
	if len(f.resp.Embeddings) == 0 {
		return ai.EmbeddingResponse{Embeddings: [][]float64{{0.1, 0.2, 0.3}}, Model: "stub"}, nil
	}
	return f.resp, nil
}

// moviesTestServer constructs a minimal Server with optional registry and vault store wired.
// pool is left nil so handlers exercise their 503 paths unless the caller intends DB access.
func moviesTestServer(reg *ai.Registry, vaultStore VaultSecretStore) *Server {
	cfg := config.Default()
	cfg.AI.DefaultProvider = "openai"
	cfg.AI.EmbeddingProvider = "openai"
	cfg.AI.EmbeddingModel = "text-embedding-3-small"
	cfg.AI.Providers = map[string]config.ProviderConfig{
		"openai": {APIKey: "config-key", DefaultModel: "gpt-4o-mini"},
	}
	s := &Server{
		cfg:        cfg,
		aiRegistry: reg,
		vaultStore: vaultStore,
		moviesBYOK: make(map[string]string),
	}
	return s
}

// --- movies search handler ---

func TestHandleMoviesSearchEmptyQuery(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`{"query":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesSearch(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "query is required")
}

func TestHandleMoviesSearchWhitespaceQuery(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`{"query":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesSearch(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "query is required")
}

func TestHandleMoviesSearchInvalidJSON(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesSearch(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMoviesSearchLimitOutOfBounds(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)

	tooHigh := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`{"query":"x","limit":51}`))
	tooHigh.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesSearch(w, tooHigh)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "limit")

	negative := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`{"query":"x","limit":-1}`))
	negative.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	s.handleMoviesSearch(w2, negative)
	testutil.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestHandleMoviesSearchNoBackingServices(t *testing.T) {
	t.Parallel()
	// No registry, no pool — should 503.
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`{"query":"sci-fi","limit":5}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesSearch(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleMoviesSearchNoEmbeddingProvider(t *testing.T) {
	t.Parallel()
	// Registry exists but its provider does not implement EmbeddingProvider.
	reg := ai.NewRegistry()
	reg.Register("openai", &nonEmbeddingProvider{})
	s := moviesTestServer(reg, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/search", strings.NewReader(`{"query":"sci-fi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesSearch(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// --- movies notes embed handler ---

func TestHandleMoviesNotesEmbedEmptyText(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/notes/embed", strings.NewReader(`{"text":"","movie_slug":"the-matrix"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesNotesEmbed(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "text is required")
}

func TestHandleMoviesNotesEmbedTextTooLong(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	body := `{"text":"` + strings.Repeat("a", 2001) + `","movie_slug":"the-matrix"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/notes/embed", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesNotesEmbed(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "at most")
}

func TestHandleMoviesNotesEmbedMissingSlug(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/notes/embed", strings.NewReader(`{"text":"loved it","movie_slug":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesNotesEmbed(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "slug")
}

func TestHandleMoviesNotesEmbedInvalidSlug(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	// Slug with characters outside [a-z0-9-] should be rejected before any pool/embedder call.
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/notes/embed", strings.NewReader(`{"text":"good","movie_slug":"Bad Slug!"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesNotesEmbed(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "slug")
}

func TestHandleMoviesNotesEmbedNoBackingServices(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/notes/embed", strings.NewReader(`{"text":"good","movie_slug":"the-matrix"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesNotesEmbed(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// --- BYOK lifecycle handlers ---

// fakeVaultForBYOK is a minimal VaultSecretStore for BYOK tests.
type fakeVaultForBYOK struct {
	mu      sync.RWMutex
	secrets map[string][]byte
}

func newFakeVaultForBYOK() *fakeVaultForBYOK {
	return &fakeVaultForBYOK{secrets: map[string][]byte{}}
}

func (f *fakeVaultForBYOK) ListSecrets(_ context.Context) ([]vault.SecretMetadata, error) {
	return nil, nil
}

func (f *fakeVaultForBYOK) GetSecret(_ context.Context, name string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.secrets[name]
	if !ok {
		return nil, vault.ErrSecretNotFound
	}
	return v, nil
}

func (f *fakeVaultForBYOK) CreateSecret(_ context.Context, name string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[name] = append([]byte(nil), value...)
	return nil
}

func (f *fakeVaultForBYOK) UpdateSecret(_ context.Context, name string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[name] = append([]byte(nil), value...)
	return nil
}

func (f *fakeVaultForBYOK) DeleteSecret(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.secrets, name)
	return nil
}

func TestHandleMoviesBYOKSetMissingVault(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/byok", strings.NewReader(`{"provider":"openai","secret_name":"MY_KEY"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesBYOKSet(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleMoviesBYOKSetUnknownProvider(t *testing.T) {
	t.Parallel()
	vs := newFakeVaultForBYOK()
	s := moviesTestServer(nil, vs)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/byok", strings.NewReader(`{"provider":"made-up","secret_name":"MY_KEY"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesBYOKSet(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "provider")
}

func TestHandleMoviesBYOKSetMissingSecret(t *testing.T) {
	t.Parallel()
	vs := newFakeVaultForBYOK()
	s := moviesTestServer(nil, vs)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/byok", strings.NewReader(`{"provider":"openai","secret_name":"NO_SUCH_KEY"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesBYOKSet(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleMoviesBYOKSetInvalidSecretName(t *testing.T) {
	t.Parallel()
	vs := newFakeVaultForBYOK()
	s := moviesTestServer(nil, vs)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/byok", strings.NewReader(`{"provider":"openai","secret_name":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesBYOKSet(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMoviesBYOKSetSuccessAndClear(t *testing.T) {
	t.Parallel()
	vs := newFakeVaultForBYOK()
	_ = vs.CreateSecret(context.Background(), "MY_KEY", []byte("byok-key-value"))
	s := moviesTestServer(nil, vs)

	req := httptest.NewRequest(http.MethodPost, "/admin/movies/byok", strings.NewReader(`{"provider":"openai","secret_name":"MY_KEY"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesBYOKSet(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	s.moviesBYOKMu.RLock()
	got := s.moviesBYOK["openai"]
	s.moviesBYOKMu.RUnlock()
	testutil.Equal(t, "MY_KEY", got)

	// Now resolve the key via the helper.
	key, err := s.resolveMoviesBYOKKey(context.Background(), "openai")
	testutil.NoError(t, err)
	testutil.Equal(t, "byok-key-value", key)

	// Clear via DELETE handler.
	delReq := httptest.NewRequest(http.MethodDelete, "/admin/movies/byok/openai", nil)
	delReq = withChiURLParam(delReq, "provider", "openai")
	delW := httptest.NewRecorder()
	s.handleMoviesBYOKClear(delW, delReq)
	testutil.Equal(t, http.StatusNoContent, delW.Code)

	s.moviesBYOKMu.RLock()
	_, exists := s.moviesBYOK["openai"]
	s.moviesBYOKMu.RUnlock()
	testutil.True(t, !exists, "BYOK entry should be cleared")
}

func TestResolveMoviesBYOKKeyUnset(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, newFakeVaultForBYOK())
	key, err := s.resolveMoviesBYOKKey(context.Background(), "openai")
	testutil.NoError(t, err)
	testutil.Equal(t, "", key)
}

// --- chat stream handler ---

// streamingProviderStub satisfies StreamingProvider with canned chunks.
type streamingProviderStub struct {
	chunks []string
}

func (s *streamingProviderStub) GenerateText(_ context.Context, _ ai.GenerateTextRequest) (ai.GenerateTextResponse, error) {
	return ai.GenerateTextResponse{Text: strings.Join(s.chunks, "")}, nil
}

func (s *streamingProviderStub) GenerateTextStream(_ context.Context, _ ai.GenerateTextRequest) (io.ReadCloser, error) {
	return newSliceReadCloser(s.chunks), nil
}

// nonEmbeddingProvider satisfies Provider but not EmbeddingProvider.
type nonEmbeddingProvider struct{}

func (nonEmbeddingProvider) GenerateText(_ context.Context, _ ai.GenerateTextRequest) (ai.GenerateTextResponse, error) {
	return ai.GenerateTextResponse{}, errors.New("not implemented")
}

func TestHandleMoviesChatStreamEmptyBody(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/chat/stream", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesChatStream(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMoviesChatStreamNoMessages(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/chat/stream", strings.NewReader(`{"messages":[],"provider":"openai","model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesChatStream(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "messages")
}

func TestHandleMoviesChatStreamTooManyMessages(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	msgs := make([]map[string]string, 21)
	for i := range msgs {
		msgs[i] = map[string]string{"role": "user", "content": "x"}
	}
	body, _ := json.Marshal(map[string]any{"messages": msgs, "provider": "openai", "model": "gpt-4o"})
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/chat/stream", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesChatStream(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMoviesChatStreamMessageTooLong(t *testing.T) {
	t.Parallel()
	s := moviesTestServer(nil, nil)
	body := `{"messages":[{"role":"user","content":"` + strings.Repeat("x", 4001) + `"}],"provider":"openai","model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesChatStream(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMoviesChatStreamUnavailable(t *testing.T) {
	t.Parallel()
	// No registry → 503.
	s := moviesTestServer(nil, nil)
	body := `{"messages":[{"role":"user","content":"hi"}],"provider":"openai","model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesChatStream(w, req)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleMoviesChatStreamSSEEvents(t *testing.T) {
	t.Parallel()
	reg := ai.NewRegistry()
	reg.Register("openai", &streamingProviderStub{chunks: []string{"hel", "lo"}})
	s := moviesTestServer(reg, nil)

	body := `{"messages":[{"role":"user","content":"hi"}],"provider":"openai","model":"gpt-4o"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/movies/chat/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMoviesChatStream(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	out := w.Body.String()
	testutil.Contains(t, out, "event: start")
	testutil.Contains(t, out, "event: chunk")
	testutil.Contains(t, out, "event: done")
	testutil.Contains(t, out, "hel")
	testutil.Contains(t, out, "lo")
}

// TestResolveMoviesProviderBYOKBypassesRegistered proves that when BYOK is
// configured for a provider, resolution constructs a fresh provider instance
// from config (via ai.NewProviderFromConfig) rather than returning the
// registered singleton. The key-precedence behavior of NewProviderFromConfig
// itself is covered in package ai's unit tests.
func TestResolveMoviesProviderBYOKBypassesRegistered(t *testing.T) {
	t.Parallel()
	vs := newFakeVaultForBYOK()
	_ = vs.CreateSecret(context.Background(), "MY_KEY", []byte("byok-secret-value"))

	reg := ai.NewRegistry()
	registered := &nonEmbeddingProvider{}
	reg.Register("openai", registered)

	s := moviesTestServer(reg, vs)

	// Without BYOK, resolution returns the registered singleton.
	gotProv, _, err := s.resolveMoviesProvider(context.Background(), "openai", "gpt-4o")
	testutil.NoError(t, err)
	if gotProv != ai.Provider(registered) {
		t.Fatal("expected registered singleton when BYOK is unset")
	}

	// With BYOK, a fresh provider is constructed (different pointer).
	s.moviesBYOKMu.Lock()
	s.moviesBYOK["openai"] = "MY_KEY"
	s.moviesBYOKMu.Unlock()

	byokProv, _, err := s.resolveMoviesProvider(context.Background(), "openai", "gpt-4o")
	testutil.NoError(t, err)
	if byokProv == ai.Provider(registered) {
		t.Fatal("BYOK should bypass registered singleton")
	}

	// Clearing the mapping restores singleton resolution.
	s.moviesBYOKMu.Lock()
	delete(s.moviesBYOK, "openai")
	s.moviesBYOKMu.Unlock()

	restoredProv, _, err := s.resolveMoviesProvider(context.Background(), "openai", "gpt-4o")
	testutil.NoError(t, err)
	if restoredProv != ai.Provider(registered) {
		t.Fatal("expected registered singleton after BYOK cleared")
	}
}
