//go:build integration && aicontract

package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/examples"
	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/vault"
)

type contractVaultStore struct {
	secrets map[string][]byte
}

func newContractVaultStore() *contractVaultStore {
	return &contractVaultStore{secrets: map[string][]byte{}}
}

func (s *contractVaultStore) ListSecrets(context.Context) ([]vault.SecretMetadata, error) {
	return nil, nil
}

func (s *contractVaultStore) GetSecret(_ context.Context, name string) ([]byte, error) {
	value, ok := s.secrets[name]
	if !ok {
		return nil, vault.ErrSecretNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *contractVaultStore) CreateSecret(_ context.Context, name string, value []byte) error {
	s.secrets[name] = append([]byte(nil), value...)
	return nil
}

func (s *contractVaultStore) UpdateSecret(_ context.Context, name string, value []byte) error {
	s.secrets[name] = append([]byte(nil), value...)
	return nil
}

func (s *contractVaultStore) DeleteSecret(_ context.Context, name string) error {
	delete(s.secrets, name)
	return nil
}

func TestMoviesChatContractStreamWithRealBYOKProviders(t *testing.T) {
	cases := []struct {
		name         string
		provider     string
		apiKeyEnv    string
		secretName   string
		sessionID    string
		sentinel     string
		resolveModel func(t *testing.T, apiKey string) string
	}{
		{
			name:       "anthropic",
			provider:   "anthropic",
			apiKeyEnv:  "ANTHROPIC_API_KEY",
			secretName: "ANTHROPIC_MOVIES_TEST_KEY",
			sessionID:  "4dfdb3fe-650c-4db8-8278-dd5943f54f8a",
			sentinel:   "AYB_MOVIES_ANTHROPIC_BYOK_OK",
			resolveModel: func(t *testing.T, _ string) string {
				t.Helper()
				return requireStage3Env(t, "AYB_AICONTRACT_ANTHROPIC_MODEL")
			},
		},
		{
			name:       "openai",
			provider:   "openai",
			apiKeyEnv:  "OPENAI_API_KEY",
			secretName: "OPENAI_MOVIES_TEST_KEY",
			sessionID:  "7a9af7e2-7e53-4f4d-a5a2-3d0ec7bc1fd7",
			sentinel:   "AYB_MOVIES_OPENAI_BYOK_OK",
			resolveModel: func(t *testing.T, apiKey string) string {
				t.Helper()
				return testutil.ResolveOpenAIContractModel(t, apiKey)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiKey := requireStage3Env(t, tc.apiKeyEnv)
			model := tc.resolveModel(t, apiKey)
			userPrompt := "Reply with exactly " + tc.sentinel + " and no other text."

			ctx := context.Background()
			ensureIntegrationMigrations(t, ctx)

			logger := testutil.DiscardLogger()
			ch := schema.NewCacheHolder(sharedPG.Pool, logger)
			testutil.NoError(t, ch.Load(ctx))

			cfg := config.Default()
			cfg.Admin.Password = "testpass"
			cfg.AI.DefaultProvider = tc.provider
			cfg.AI.DefaultModel = model
			cfg.AI.Providers = map[string]config.ProviderConfig{
				tc.provider: {
					APIKey:       apiKey,
					DefaultModel: model,
				},
			}

			reg, err := ai.BuildRegistry(cfg.AI, nil)
			testutil.NoError(t, err)
			resolvedModel := requireStreamingBYOKProvider(t, reg, tc.provider, apiKey, cfg.AI)
			testutil.Equal(t, model, resolvedModel)

			srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
			srv.SetAIRegistry(reg)
			vaultStore := newContractVaultStore()
			testutil.NoError(t, vaultStore.CreateSecret(ctx, tc.secretName, []byte(apiKey)))
			srv.SetVaultStore(vaultStore)
			token := adminLogin(t, srv)

			execSQL := func(t *testing.T, query string) {
				t.Helper()
				body := `{"query":` + jsonString(query) + `}`
				req := httptest.NewRequest(http.MethodPost, "/api/admin/sql/", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				srv.Router().ServeHTTP(w, req)
				testutil.Equal(t, http.StatusOK, w.Code)
			}

			moviesSchemaSQL, err := fs.ReadFile(examples.FS, "movies/schema.sql")
			testutil.NoError(t, err)
			execSQL(t, string(moviesSchemaSQL))

			moviesSeedSQL, err := fs.ReadFile(examples.FS, "movies/seed.sql")
			testutil.NoError(t, err)
			execSQL(t, string(moviesSeedSQL))

			ts := httptest.NewServer(srv.Router())
			defer ts.Close()

			byokBody, err := json.Marshal(map[string]string{
				"provider":    tc.provider,
				"secret_name": tc.secretName,
			})
			testutil.NoError(t, err)

			byokReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/movies/byok", bytes.NewReader(byokBody))
			testutil.NoError(t, err)
			byokReq.Header.Set("Authorization", "Bearer "+token)
			byokReq.Header.Set("Content-Type", "application/json")
			byokResp, err := ts.Client().Do(byokReq)
			testutil.NoError(t, err)
			defer byokResp.Body.Close()
			testutil.Equal(t, http.StatusOK, byokResp.StatusCode)

			var byokPayload map[string]string
			testutil.NoError(t, json.NewDecoder(byokResp.Body).Decode(&byokPayload))
			testutil.Equal(t, tc.provider, byokPayload["provider"])
			testutil.Equal(t, tc.secretName, byokPayload["secret_name"])

			streamBody, err := json.Marshal(map[string]any{
				"provider":   tc.provider,
				"session_id": tc.sessionID,
				"messages": []map[string]string{
					{"role": "user", "content": userPrompt},
				},
			})
			testutil.NoError(t, err)

			streamReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/movies/chat/stream", bytes.NewReader(streamBody))
			testutil.NoError(t, err)
			streamReq.Header.Set("Authorization", "Bearer "+token)
			streamReq.Header.Set("Content-Type", "application/json")
			streamResp, err := ts.Client().Do(streamReq)
			testutil.NoError(t, err)
			defer streamResp.Body.Close()

			testutil.Equal(t, http.StatusOK, streamResp.StatusCode)
			testutil.Equal(t, "text/event-stream", streamResp.Header.Get("Content-Type"))

			scanner := bufio.NewScanner(streamResp.Body)
			scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

			startLines := readNextSSEEvent(t, scanner, 2*time.Minute, "timed out waiting for movies SSE start event")
			startEvent, startPayload := decodeMoviesSSEEvent(t, startLines)
			testutil.Equal(t, "start", startEvent)
			testutil.Equal(t, tc.provider, stringField(t, startPayload, "provider"))
			testutil.Equal(t, model, stringField(t, startPayload, "model"))
			testutil.Equal(t, tc.sessionID, stringField(t, startPayload, "session_id"))

			var (
				chunkCount int
				collected  strings.Builder
				doneText   string
			)
			for {
				lines := readNextSSEEvent(t, scanner, 2*time.Minute, "timed out waiting for movies SSE follow-up event")
				eventName, payload := decodeMoviesSSEEvent(t, lines)
				switch eventName {
				case "chunk":
					chunk := stringField(t, payload, "text")
					testutil.True(t, chunk != "", "chunk text should be non-empty")
					chunkCount++
					collected.WriteString(chunk)
				case "done":
					doneText = stringField(t, payload, "text")
					testutil.Equal(t, tc.sessionID, stringField(t, payload, "session_id"))
					if strings.TrimSpace(doneText) != tc.sentinel {
						t.Fatalf("done text = %q; want exactly %q after trimming whitespace", doneText, tc.sentinel)
					}
					testutil.Equal(t, doneText, collected.String())
					goto persistedAssertions
				case "error":
					t.Fatalf("received SSE error payload: %#v", payload)
				default:
					t.Fatalf("unexpected SSE event %q with payload %#v", eventName, payload)
				}
			}

		persistedAssertions:
			// >1, not >0. The synchronous fallback in movies_chat.go
			// (streamMoviesChatProvider's non-streaming branch) calls onChunk exactly
			// once with the entire response, so it emits exactly ONE "chunk" event no
			// matter how long the reply is. A `> 0` assertion therefore still passes
			// when real token streaming has silently degraded to a single blob --
			// which is precisely how the missing GenerateTextStream backends went
			// unnoticed across four sessions. Only a real streaming provider (one
			// onChunk per reader.Read that returns n>0) can exceed 1.
			testutil.True(t, chunkCount > 1, "expected >1 chunk events; exactly 1 chunk means the provider fell back to non-streaming GenerateText")
			testutil.NoError(t, scanner.Err())

			var persistedCount int
			testutil.NoError(t, sharedPG.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM movies_chat_history WHERE session_id = $1`,
				tc.sessionID,
			).Scan(&persistedCount))
			testutil.Equal(t, 2, persistedCount)

			var persistedUser string
			testutil.NoError(t, sharedPG.Pool.QueryRow(ctx,
				`SELECT content FROM movies_chat_history
				 WHERE session_id = $1 AND role = 'user'
				 ORDER BY created_at DESC, id DESC
				 LIMIT 1`,
				tc.sessionID,
			).Scan(&persistedUser))
			testutil.Equal(t, userPrompt, persistedUser)

			var (
				persistedAssistant string
				partial            bool
			)
			testutil.NoError(t, sharedPG.Pool.QueryRow(ctx,
				`SELECT content, partial FROM movies_chat_history
				 WHERE session_id = $1 AND role = 'assistant'
				 ORDER BY created_at DESC, id DESC
				 LIMIT 1`,
				tc.sessionID,
			).Scan(&persistedAssistant, &partial))
			testutil.Equal(t, doneText, persistedAssistant)
			testutil.True(t, !partial, "assistant transcript should not be partial")
		})
	}
}

func requireStage3Env(t *testing.T, name string) string {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s must be set for Stage 3 aicontract integration tests", name)
	}
	return value
}

func requireStreamingBYOKProvider(t *testing.T, reg *ai.Registry, providerName, byokKey string, cfg config.AIConfig) string {
	t.Helper()

	provider, model, err := ai.ResolveProvider(reg, providerName, "", byokKey, cfg)
	testutil.NoError(t, err)
	_, streaming := provider.(ai.StreamingProvider)
	testutil.True(t, streaming, "expected BYOK-resolved %s provider to implement StreamingProvider", providerName)
	return model
}

func decodeMoviesSSEEvent(t *testing.T, lines []string) (string, map[string]any) {
	t.Helper()

	var (
		eventName string
		dataLines []string
	)
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	testutil.True(t, eventName != "", "expected SSE event name in %v", lines)
	testutil.True(t, len(dataLines) > 0, "expected SSE data in %v", lines)

	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &payload))
	return eventName, payload
}

func stringField(t *testing.T, payload map[string]any, field string) string {
	t.Helper()

	value, ok := payload[field].(string)
	testutil.True(t, ok, "expected %s to be a string in %#v", field, payload)
	return value
}
