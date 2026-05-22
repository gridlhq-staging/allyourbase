// Package server movies_chat.go owns the movies demo's SSE chat endpoint.
// The handler reuses writeSSEJSON / isCanceledStreamRequest from
// ai_assistant_handler.go for SSE framing and disconnect detection, and
// resolveMoviesProvider for BYOK-aware provider resolution. Persistence
// into movies_chat_history is best-effort: when the pool is nil (e.g.
// during unit tests) the streaming still completes; only the trailing
// transaction is skipped.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/google/uuid"
)

const (
	moviesChatMaxMessages      = 20
	moviesChatMaxMessageRunes  = 4000
	moviesChatStreamReadBuf    = 4096
	serviceUnavailableMoviesAI = "movies AI provider not configured"
)

type moviesChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type moviesChatStreamRequest struct {
	Messages  []moviesChatMessage `json:"messages"`
	Provider  string              `json:"provider"`
	Model     string              `json:"model"`
	SessionID string              `json:"session_id,omitempty"`
}

// handleMoviesChatStream streams an AI chat completion as SSE. Flow:
//  1. decode + validate request — abort with 4xx before SSE framing.
//  2. resolve provider (BYOK-aware) — 503 if unavailable, also pre-SSE.
//  3. begin SSE (start event), forward provider chunks, end with done.
//  4. persist user + assistant messages to movies_chat_history; if the
//     client disconnected mid-stream the assistant row is marked partial.
//
// Returning 4xx/5xx before SSE starts keeps client error handling simple;
// once SSE has begun we surface errors as an "error" SSE event instead so
// the connection stays a single coherent response.
func (s *Server) handleMoviesChatStream(w http.ResponseWriter, r *http.Request) {
	var body moviesChatStreamRequest
	if !httputil.DecodeJSON(w, r, &body) {
		return
	}
	if err := validateMoviesChatRequest(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.aiRegistry == nil {
		serviceUnavailable(w, serviceUnavailableMoviesAI)
		return
	}
	provider, model, err := s.resolveMoviesProvider(r.Context(), strings.TrimSpace(body.Provider), strings.TrimSpace(body.Model))
	if err != nil {
		// Pre-SSE failure → respond with structured JSON error.
		httputil.WriteError(w, http.StatusServiceUnavailable, "provider resolution failed: "+err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "streaming is not supported by this server")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sessionID := normalizeSessionID(body.SessionID)
	if err := writeSSEJSON(w, flusher, "start", map[string]any{
		"provider":   body.Provider,
		"model":      model,
		"session_id": sessionID,
	}); err != nil {
		return
	}

	req := buildMoviesChatGenerateRequest(body.Messages, model)
	var collected strings.Builder
	streamErr := streamMoviesChatProvider(r.Context(), provider, req, &collected, func(chunk string) error {
		return writeSSEJSON(w, flusher, "chunk", map[string]any{"text": chunk})
	})

	partial := false
	if streamErr != nil {
		if isCanceledStreamRequest(r.Context()) {
			partial = true
		} else {
			_ = writeSSEJSON(w, flusher, "error", map[string]any{
				"code":    http.StatusInternalServerError,
				"message": streamErr.Error(),
			})
			partial = true
		}
	}

	s.persistMoviesChatExchange(r.Context(), sessionID, body.Messages, collected.String(), partial)

	if streamErr == nil {
		_ = writeSSEJSON(w, flusher, "done", map[string]any{
			"session_id": sessionID,
			"text":       collected.String(),
		})
	}
}

// validateMoviesChatRequest enforces the message-array / per-message length
// bounds described in the stage checklist. Returning errors keeps the
// handler's main path linear.
func validateMoviesChatRequest(body *moviesChatStreamRequest) error {
	if len(body.Messages) == 0 {
		return errors.New("messages must contain at least one entry")
	}
	if len(body.Messages) > moviesChatMaxMessages {
		return errors.New("messages exceeds maximum length of 20")
	}
	for i, m := range body.Messages {
		role := strings.TrimSpace(m.Role)
		if role != "user" && role != "assistant" && role != "system" {
			return errInvalidRole(i)
		}
		if utf8.RuneCountInString(m.Content) > moviesChatMaxMessageRunes {
			return errors.New("message content exceeds 4000 characters")
		}
	}
	return nil
}

func errInvalidRole(i int) error {
	return fmt.Errorf("message role must be one of system|user|assistant at index %d", i)
}

// buildMoviesChatGenerateRequest converts the wire-shape messages into the
// ai.GenerateTextRequest understood by Provider/StreamingProvider. The
// first system message is hoisted into SystemPrompt; subsequent ones are
// kept in the message list so providers that prefer the explicit field
// (Anthropic) and those that don't (OpenAI) both work.
func buildMoviesChatGenerateRequest(in []moviesChatMessage, model string) ai.GenerateTextRequest {
	req := ai.GenerateTextRequest{Model: model}
	systemHoisted := false
	for _, m := range in {
		role := strings.TrimSpace(m.Role)
		if role == "system" && !systemHoisted {
			req.SystemPrompt = m.Content
			systemHoisted = true
			continue
		}
		req.Messages = append(req.Messages, ai.Message{
			Role:    role,
			Content: ai.TextContent(m.Content),
		})
	}
	return req
}

// streamMoviesChatProvider mirrors ai.AssistantService.ExecuteStream's
// shape: prefer StreamingProvider when available, otherwise call the
// synchronous GenerateText and emit the full response as a single chunk.
// We can't reuse AssistantService directly because it persists into
// assistant history; the movies demo persists into a different table.
func streamMoviesChatProvider(ctx context.Context, provider ai.Provider, req ai.GenerateTextRequest, collected *strings.Builder, onChunk func(string) error) error {
	if sp, ok := provider.(ai.StreamingProvider); ok {
		return streamFromStreamingProvider(ctx, sp, req, collected, onChunk)
	}
	resp, err := provider.GenerateText(ctx, req)
	if err != nil {
		return err
	}
	if resp.Text == "" {
		return nil
	}
	collected.WriteString(resp.Text)
	return onChunk(resp.Text)
}

func streamFromStreamingProvider(ctx context.Context, provider ai.StreamingProvider, req ai.GenerateTextRequest, collected *strings.Builder, onChunk func(string) error) error {
	reader, err := provider.GenerateTextStream(ctx, req)
	if err != nil {
		return err
	}
	defer reader.Close()
	buf := make([]byte, moviesChatStreamReadBuf)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			collected.WriteString(chunk)
			if err := onChunk(chunk); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// normalizeSessionID accepts a client-provided session_id when valid; on
// empty or invalid input a new UUID is minted. Persistence keys on this
// value so reusing the same id appends turns to the same conversation.
func normalizeSessionID(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return uuid.New().String()
	}
	if _, err := uuid.Parse(in); err != nil {
		return uuid.New().String()
	}
	return in
}

// persistMoviesChatExchange writes the user/assistant turns of a single
// streamed exchange into movies_chat_history. With s.pool nil (tests, or
// the demo running schema-less) this is a no-op so SSE responses still
// work end-to-end.
func (s *Server) persistMoviesChatExchange(ctx context.Context, sessionID string, inputs []moviesChatMessage, assistantText string, partial bool) {
	if s.pool == nil {
		return
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, m := range inputs {
		role := strings.TrimSpace(m.Role)
		// Only persist user/assistant; system prompts are configuration,
		// not conversational state, and storing them here would conflate
		// the two concerns when retrieving session history later.
		if role != "user" && role != "assistant" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO movies_chat_history (id, session_id, role, content, partial)
			 VALUES (gen_random_uuid(), $1, $2, $3, false)`,
			sessionID, role, m.Content,
		); err != nil {
			return
		}
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO movies_chat_history (id, session_id, role, content, partial)
		 VALUES (gen_random_uuid(), $1, 'assistant', $2, $3)`,
		sessionID, assistantText, partial,
	); err != nil {
		return
	}
	_ = tx.Commit(ctx)
}
