// Package auth Stub summary for /Users/stuart/parallel_development/allyourbase_dev/may31_pm_9_webauthn_passkeys_backend/allyourbase_dev/internal/auth/webauthn_mfa_handlers.go.
package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-webauthn/webauthn/protocol"
)

func (h *Handler) SetWebAuthnEnabled(enabled bool) {
	h.webauthnEnabled = enabled
}

func (h *Handler) SetWebAuthnPublicBaseURL(baseURL string) {
	h.webauthnPublicBaseURL = baseURL
}

func (h *Handler) requireWebAuthnEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.webauthnEnabled {
			httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "WebAuthn MFA is not enabled",
				"https://allyourbase.io/guide/authentication#webauthn")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) handleWebAuthnEnroll(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, ErrAnonymousMFABlock.Error())
		return
	}

	if h.enforceAAL2ForExistingMFA(w, r, claims) {
		return
	}

	creation, err := h.auth.EnrollWebAuthn(r.Context(), claims.Subject, h.webauthnPublicBaseURL)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebAuthnAlreadyEnrolled):
			httputil.WriteError(w, http.StatusConflict, "WebAuthn MFA already enrolled")
		default:
			h.logger.Error("WebAuthn enroll error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, creation.Response)
}

type webauthnEnrollConfirmRequest struct {
	DisplayName         string          `json:"display_name"`
	AttestationResponse json.RawMessage `json:"attestation_response"`
}

func (h *Handler) handleWebAuthnEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, ErrAnonymousMFABlock.Error())
		return
	}

	var req webauthnEnrollConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.AttestationResponse) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "attestation_response is required")
		return
	}

	parsed, err := protocol.ParseCredentialCreationResponseBytes(req.AttestationResponse)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid attestation response")
		return
	}

	err = h.auth.ConfirmWebAuthnEnrollment(
		r.Context(),
		claims.Subject,
		h.webauthnPublicBaseURL,
		strings.TrimSpace(req.DisplayName),
		parsed,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebAuthnEnrollmentNotPending), errors.Is(err, ErrWebAuthnInvalidAttestation):
			httputil.WriteError(w, http.StatusBadRequest, "WebAuthn enrollment verification failed")
		default:
			h.logger.Error("WebAuthn enroll confirm error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "WebAuthn MFA enrollment confirmed",
	})
}

func (h *Handler) handleWebAuthnDelete(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	err := h.auth.DeleteWebAuthn(r.Context(), claims.Subject)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebAuthnNotEnrolled):
			httputil.WriteError(w, http.StatusNotFound, "no WebAuthn factor enrolled")
		default:
			h.logger.Error("WebAuthn delete error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleWebAuthnChallenge(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	challengeID, assertion, err := h.auth.CreateWebAuthnChallenge(
		r.Context(), claims.Subject, mfaChallengeIP(r), h.webauthnPublicBaseURL,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebAuthnNotEnrolled):
			httputil.WriteError(w, http.StatusNotFound, "no WebAuthn factor enrolled")
		default:
			h.logger.Error("WebAuthn challenge error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"challenge_id": challengeID,
		"options":      assertion.Response,
	})
}

type webauthnFirstFactorBeginRequest struct {
	Email string `json:"email"`
}

func (h *Handler) handleWebAuthnFirstFactorBegin(w http.ResponseWriter, r *http.Request) {
	var req webauthnFirstFactorBeginRequest
	if !decodeBody(w, r, &req) {
		return
	}

	email := normalizeAuthEmail(req.Email)
	if email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}
	if err := validateAuthEmail(email); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid email format")
		return
	}

	challengeID, assertion, err := h.auth.CreateWebAuthnFirstFactorChallenge(
		r.Context(), email, mfaChallengeIP(r), h.webauthnPublicBaseURL,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrWebAuthnNotEnrolled):
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
				"invalid email or passkey",
				"https://allyourbase.io/guide/authentication")
		default:
			h.logger.Error("WebAuthn first-factor challenge error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"challenge_id": challengeID,
		"options":      assertion.Response,
	})
}

type webauthnVerifyRequest struct {
	ChallengeID       string          `json:"challenge_id"`
	AssertionResponse json.RawMessage `json:"assertion_response"`
}

func (h *Handler) handleWebAuthnFirstFactorFinish(w http.ResponseWriter, r *http.Request) {
	var req webauthnVerifyRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ChallengeID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "challenge_id is required")
		return
	}
	if len(req.AssertionResponse) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "assertion_response is required")
		return
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(req.AssertionResponse)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid assertion response")
		return
	}

	user, accessToken, refreshToken, err := h.auth.VerifyWebAuthnFirstFactorChallenge(
		r.Context(), req.ChallengeID, h.webauthnPublicBaseURL, parsed,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebAuthnChallengeNotFound):
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
				"invalid email or passkey",
				"https://allyourbase.io/guide/authentication")
		case errors.Is(err, ErrWebAuthnChallengeUsed):
			httputil.WriteError(w, http.StatusConflict, "challenge already verified")
		case errors.Is(err, ErrWebAuthnClonedKey):
			httputil.WriteError(w, http.StatusUnauthorized, "cloned authenticator detected")
		case errors.Is(err, ErrWebAuthnInvalidAssertion):
			httputil.WriteError(w, http.StatusUnauthorized, "WebAuthn assertion failed")
		default:
			h.logger.Error("WebAuthn first-factor verify error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

func (h *Handler) handleWebAuthnVerify(w http.ResponseWriter, r *http.Request) {
	claims := mfaPendingClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "no MFA challenge pending")
		return
	}

	var req webauthnVerifyRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ChallengeID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "challenge_id is required")
		return
	}
	if len(req.AssertionResponse) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "assertion_response is required")
		return
	}

	if h.auth.IsMFALocked(claims.Subject) {
		httputil.WriteError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(req.AssertionResponse)
	if err != nil {
		h.auth.RecordMFAFailure(claims.Subject)
		httputil.WriteError(w, http.StatusBadRequest, "invalid assertion response")
		return
	}

	user, accessToken, refreshToken, err := h.auth.VerifyWebAuthnChallenge(
		r.Context(), claims.Subject, req.ChallengeID, h.webauthnPublicBaseURL,
		parsed, firstFactorMethodFromPendingClaims(claims),
	)
	if err != nil {
		if errors.Is(err, ErrWebAuthnInvalidAssertion) || errors.Is(err, ErrWebAuthnClonedKey) {
			h.auth.RecordMFAFailure(claims.Subject)
		}
		switch {
		case errors.Is(err, ErrWebAuthnChallengeNotFound):
			httputil.WriteError(w, http.StatusNotFound, "challenge not found or expired")
		case errors.Is(err, ErrWebAuthnChallengeUsed):
			httputil.WriteError(w, http.StatusConflict, "challenge already verified")
		case errors.Is(err, ErrWebAuthnClonedKey):
			httputil.WriteError(w, http.StatusUnauthorized, "cloned authenticator detected")
		case errors.Is(err, ErrWebAuthnInvalidAssertion):
			httputil.WriteError(w, http.StatusUnauthorized, "WebAuthn assertion failed")
		default:
			h.logger.Error("WebAuthn verify error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	h.auth.ResetMFAFailures(claims.Subject)

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}
