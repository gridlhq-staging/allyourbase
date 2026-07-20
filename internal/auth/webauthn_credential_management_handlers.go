// Package auth exposes HTTP handlers for authentication and MFA workflows.
package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// handleWebAuthnCredentialsList returns the current user's registered WebAuthn credentials.
func (h *Handler) handleWebAuthnCredentialsList(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	credentials, err := h.auth.ListWebAuthnCredentials(r.Context(), claims.Subject)
	if err != nil {
		h.logger.Error("WebAuthn credential list error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"credentials": credentials,
	})
}

type webauthnCredentialRenameRequest struct {
	DisplayName string `json:"display_name"`
}

// handleWebAuthnCredentialRename updates a credential display name after ownership and AAL checks.
func (h *Handler) handleWebAuthnCredentialRename(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	credentialID, ok := decodeWebAuthnCredentialIDParam(w, r)
	if !ok {
		return
	}

	var req webauthnCredentialRenameRequest
	if !decodeBody(w, r, &req) {
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "display_name is required", httputil.DocURL("/guide/authentication"))
		return
	}
	if blocked := h.blockSelfServiceWebAuthnCredentialWrite(w, r, claims, credentialID); blocked {
		return
	}

	credential, err := h.auth.RenameWebAuthnCredential(r.Context(), claims.Subject, credentialID, displayName)
	if err != nil {
		h.writeWebAuthnCredentialManagementError(w, err, "WebAuthn credential rename error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, credential)
}

// handleWebAuthnCredentialDelete removes a credential after ownership and AAL checks.
func (h *Handler) handleWebAuthnCredentialDelete(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	credentialID, ok := decodeWebAuthnCredentialIDParam(w, r)
	if !ok {
		return
	}
	if blocked := h.blockSelfServiceWebAuthnCredentialWrite(w, r, claims, credentialID); blocked {
		return
	}

	err := h.auth.DeleteWebAuthnCredential(r.Context(), claims.Subject, credentialID)
	if err != nil {
		h.writeWebAuthnCredentialManagementError(w, err, "WebAuthn credential delete error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeWebAuthnCredentialIDParam(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	encoded := chi.URLParam(r, "credential_id")
	if encoded == "" || strings.Contains(encoded, "=") {
		httputil.WriteError(w, http.StatusBadRequest, "invalid credential_id")
		return nil, false
	}
	credentialID, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(credentialID) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "invalid credential_id")
		return nil, false
	}
	return credentialID, true
}

// blockSelfServiceWebAuthnCredentialWrite requires aal2 for writes to the user's own credentials.
func (h *Handler) blockSelfServiceWebAuthnCredentialWrite(
	w http.ResponseWriter,
	r *http.Request,
	claims *Claims,
	credentialID []byte,
) bool {
	if claims.AAL == "aal2" {
		return false
	}

	ownsCredential, err := h.userOwnsWebAuthnCredential(r.Context(), claims.Subject, credentialID)
	if err != nil {
		h.logger.Error("WebAuthn credential ownership lookup error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return true
	}
	if !ownsCredential {
		return false
	}

	httputil.WriteJSON(w, http.StatusForbidden, map[string]string{
		"error":   "insufficient_aal",
		"message": "MFA verification is required for this action",
	})
	return true
}

func (h *Handler) userOwnsWebAuthnCredential(ctx context.Context, userID string, credentialID []byte) (bool, error) {
	credentials, err := h.auth.ListWebAuthnCredentials(ctx, userID)
	if err != nil {
		return false, err
	}

	encodedID := base64.RawURLEncoding.EncodeToString(credentialID)
	for _, credential := range credentials {
		if credential.CredentialID == encodedID {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) writeWebAuthnCredentialManagementError(w http.ResponseWriter, err error, logMessage string) {
	switch {
	case errors.Is(err, ErrWebAuthnCredentialNotFound):
		httputil.WriteError(w, http.StatusNotFound, "WebAuthn credential not found")
	case errors.Is(err, ErrWebAuthnLastCredential):
		httputil.WriteErrorWithDocURL(w, http.StatusForbidden, "cannot delete final WebAuthn credential", httputil.DocURL("/guide/authentication"))
	default:
		h.logger.Error(logMessage, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
