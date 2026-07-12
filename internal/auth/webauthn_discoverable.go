package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

// CreateWebAuthnDiscoverableChallenge starts a username-less passkey login
// ceremony and persists the challenge session until finish.
func (s *Service) CreateWebAuthnDiscoverableChallenge(ctx context.Context, ipAddress, publicBaseURL string) (string, *protocol.CredentialAssertion, error) {
	if s.pool == nil {
		return "", nil, errors.New("database pool is not configured")
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return "", nil, fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	assertion, session, err := wa.BeginDiscoverableLogin()
	if err != nil {
		return "", nil, fmt.Errorf("beginning discoverable WebAuthn login: %w", err)
	}

	sessionBytes, err := json.Marshal(session)
	if err != nil {
		return "", nil, fmt.Errorf("serializing discoverable WebAuthn login session: %w", err)
	}

	var challengeID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_webauthn_discoverable_challenges (ip_address, webauthn_session_data)
		 VALUES ($1::inet, $2)
		 RETURNING id`,
		ipAddress, sessionBytes,
	).Scan(&challengeID)
	if err != nil {
		return "", nil, fmt.Errorf("creating discoverable WebAuthn challenge: %w", err)
	}

	s.logger.Info("WebAuthn discoverable challenge created", "challenge_id", challengeID)
	return challengeID, assertion, nil
}

// VerifyWebAuthnDiscoverableChallenge validates a username-less passkey
// assertion, resolves the user from the WebAuthn user handle, and issues an
// AAL1 first-factor WebAuthn session.
func (s *Service) VerifyWebAuthnDiscoverableChallenge(ctx context.Context, challengeID, publicBaseURL string, parsedAssertion *protocol.ParsedCredentialAssertionData) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	session, err := s.loadWebAuthnDiscoverableChallenge(ctx, challengeID)
	if err != nil {
		return nil, "", "", err
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	var resolved *discoverableWebAuthnUser
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		user, err := s.loadDiscoverableWebAuthnUser(ctx, rawID, userHandle)
		if err != nil {
			return nil, err
		}
		resolved = user
		return &user.webauthnUser, nil
	}

	_, credential, err := wa.ValidatePasskeyLogin(handler, session, parsedAssertion)
	if err != nil {
		return nil, "", "", ErrWebAuthnInvalidAssertion
	}
	if credential.Authenticator.CloneWarning {
		return nil, "", "", ErrWebAuthnClonedKey
	}
	if resolved == nil {
		return nil, "", "", ErrWebAuthnInvalidAssertion
	}

	if err := s.commitWebAuthnDiscoverableVerification(ctx, challengeID, resolved.factorID, credential.ID, int64(credential.Authenticator.SignCount)); err != nil {
		return nil, "", "", err
	}

	user, token, refreshToken, err := s.issueWebAuthnFirstFactorSession(ctx, resolved.userID)
	if err != nil {
		return nil, "", "", err
	}

	s.logger.Info("WebAuthn discoverable first-factor verified", "user_id", resolved.userID, "challenge_id", challengeID)
	return user, token, refreshToken, nil
}

func (s *Service) loadWebAuthnDiscoverableChallenge(ctx context.Context, challengeID string) (webauthn.SessionData, error) {
	var (
		verifiedAt   *time.Time
		expiresAt    time.Time
		sessionBytes []byte
	)
	err := s.pool.QueryRow(ctx,
		`SELECT verified_at, expires_at, webauthn_session_data
		   FROM _ayb_webauthn_discoverable_challenges
		  WHERE id = $1`,
		challengeID,
	).Scan(&verifiedAt, &expiresAt, &sessionBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return webauthn.SessionData{}, ErrWebAuthnChallengeNotFound
		}
		return webauthn.SessionData{}, fmt.Errorf("loading discoverable WebAuthn challenge: %w", err)
	}
	if verifiedAt != nil {
		return webauthn.SessionData{}, ErrWebAuthnChallengeUsed
	}
	if time.Now().After(expiresAt) {
		return webauthn.SessionData{}, ErrWebAuthnChallengeNotFound
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		return webauthn.SessionData{}, fmt.Errorf("deserializing discoverable WebAuthn session: %w", err)
	}
	return session, nil
}

type discoverableWebAuthnUser struct {
	webauthnUser
	userID   string
	factorID string
}

func (s *Service) loadDiscoverableWebAuthnUser(ctx context.Context, rawID, userHandle []byte) (*discoverableWebAuthnUser, error) {
	if len(rawID) == 0 || len(userHandle) == 0 {
		return nil, ErrInvalidCredentials
	}

	userID := string(userHandle)
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	var factorID string
	err = s.pool.QueryRow(ctx,
		`SELECT f.id::text
		   FROM _ayb_user_mfa f
		   JOIN _ayb_webauthn_credentials c ON c.factor_id = f.id
		  WHERE f.user_id = $1
		    AND f.method = 'webauthn'
		    AND f.enabled = true
		    AND c.credential_id = $2
		  LIMIT 1`,
		user.ID, rawID,
	).Scan(&factorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("looking up discoverable WebAuthn credential: %w", err)
	}

	credentialRows, err := s.loadWebAuthnCredentialRowsForFactor(ctx, factorID)
	if err != nil {
		return nil, err
	}
	return &discoverableWebAuthnUser{
		webauthnUser: webauthnUser{
			id:          []byte(user.ID),
			name:        user.Email,
			credentials: webAuthnCredentialsFromRows(credentialRows),
		},
		userID:   user.ID,
		factorID: factorID,
	}, nil
}

func (s *Service) commitWebAuthnDiscoverableVerification(ctx context.Context, challengeID, factorID string, credentialID []byte, newSignCount int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	challengeResult, err := tx.Exec(ctx,
		`UPDATE _ayb_webauthn_discoverable_challenges
		    SET verified_at = NOW()
		  WHERE id = $1 AND verified_at IS NULL`,
		challengeID,
	)
	if err != nil {
		return fmt.Errorf("marking discoverable challenge verified: %w", err)
	}
	if challengeResult.RowsAffected() != 1 {
		return ErrWebAuthnChallengeUsed
	}

	if err := updateWebAuthnCredentialUsage(ctx, tx, factorID, credentialID, newSignCount); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing discoverable verification: %w", err)
	}
	return nil
}
