// Package auth Stub summary for /Users/stuart/parallel_development/allyourbase_dev/may31_pm_9_webauthn_passkeys_backend/allyourbase_dev/internal/auth/webauthn_mfa.go.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

var (
	ErrWebAuthnAlreadyEnrolled      = errors.New("WebAuthn MFA already enrolled")
	ErrWebAuthnNotEnrolled          = errors.New("no WebAuthn factor found")
	ErrWebAuthnEnrollmentNotPending = errors.New("no pending WebAuthn enrollment found")
	ErrWebAuthnInvalidAttestation   = errors.New("WebAuthn enrollment verification failed")
)

func (s *Service) EnrollWebAuthn(ctx context.Context, userID, publicBaseURL string) (*protocol.CredentialCreation, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}

	var existing bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'webauthn' AND enabled = true)`,
		userID,
	).Scan(&existing)
	if err != nil {
		return nil, fmt.Errorf("checking WebAuthn enrollment: %w", err)
	}
	if existing {
		return nil, ErrWebAuthnAlreadyEnrolled
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("looking up user for WebAuthn enroll: %w", err)
	}

	wUser := &webauthnUser{
		id:   []byte(user.ID),
		name: user.Email,
	}

	creation, session, err := wa.BeginRegistration(wUser,
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		return nil, fmt.Errorf("beginning WebAuthn registration: %w", err)
	}

	sessionBytes, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("serializing WebAuthn session: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_user_mfa (user_id, method, enabled, webauthn_session_data)
		 VALUES ($1, 'webauthn', false, $2)
		 ON CONFLICT (user_id, method) DO UPDATE
		 SET enabled = false,
		     webauthn_credential_id = NULL,
		     webauthn_public_key = NULL,
		     webauthn_sign_count = 0,
		     webauthn_display_name = NULL,
		     webauthn_session_data = $2`,
		userID, sessionBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("persisting WebAuthn enrollment: %w", err)
	}

	s.logger.Info("WebAuthn enrollment started", "user_id", userID)
	return creation, nil
}

func (s *Service) ConfirmWebAuthnEnrollment(
	ctx context.Context,
	userID,
	publicBaseURL,
	displayName string,
	attestationResponse *protocol.ParsedCredentialCreationData,
) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	var sessionBytes []byte
	err := s.pool.QueryRow(ctx,
		`SELECT webauthn_session_data FROM _ayb_user_mfa
		 WHERE user_id = $1 AND method = 'webauthn' AND enabled = false`,
		userID,
	).Scan(&sessionBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWebAuthnEnrollmentNotPending
		}
		return fmt.Errorf("loading WebAuthn enrollment session: %w", err)
	}
	if len(sessionBytes) == 0 {
		return ErrWebAuthnEnrollmentNotPending
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		return fmt.Errorf("deserializing WebAuthn session: %w", err)
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	wUser := &webauthnUser{
		id:   []byte(userID),
		name: "",
	}

	credential, err := wa.CreateCredential(wUser, session, attestationResponse)
	if err != nil {
		return ErrWebAuthnInvalidAttestation
	}

	trimmedDisplayName := strings.TrimSpace(displayName)

	// Compare-and-swap the exact pending session bytes so a superseded
	// enrollment challenge cannot still activate a credential.
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_user_mfa
		 SET enabled = true,
		     webauthn_credential_id = $2,
		     webauthn_public_key = $3,
		     webauthn_sign_count = $4,
		     webauthn_display_name = $5,
		     webauthn_session_data = NULL,
		     enrolled_at = NOW()
		 WHERE user_id = $1 AND method = 'webauthn'
		   AND enabled = false
		   AND webauthn_session_data = $6`,
		userID,
		credential.ID,
		credential.PublicKey,
		int64(credential.Authenticator.SignCount),
		trimmedDisplayName,
		sessionBytes,
	)
	if err != nil {
		return fmt.Errorf("persisting WebAuthn credential: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrWebAuthnEnrollmentNotPending
	}

	s.logger.Info("WebAuthn enrollment confirmed", "user_id", userID)
	return nil
}

// DeleteWebAuthn removes the user's enrolled passkey so the dashboard can
// re-enroll or clear stale credentials without touching unrelated MFA factors.
func (s *Service) DeleteWebAuthn(ctx context.Context, userID string) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	result, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_user_mfa
		 WHERE user_id = $1 AND method = 'webauthn' AND enabled = true`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("deleting WebAuthn factor: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrWebAuthnNotEnrolled
	}
	return nil
}
