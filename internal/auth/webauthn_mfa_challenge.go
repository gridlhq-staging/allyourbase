package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) CreateWebAuthnFirstFactorChallenge(ctx context.Context, email, ipAddress, publicBaseURL string) (string, *protocol.CredentialAssertion, error) {
	if s.pool == nil {
		return "", nil, errors.New("database pool is not configured")
	}

	normalizedEmail := normalizeAuthEmail(email)
	userID, err := s.lookupWebAuthnFirstFactorUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrWebAuthnNotEnrolled) {
			return s.createWebAuthnFirstFactorDecoyChallenge(normalizedEmail, publicBaseURL)
		}
		return "", nil, err
	}

	return s.createWebAuthnChallengeForScope(ctx, userID, ipAddress, publicBaseURL, webAuthnChallengeScopeFirstFactor)
}

var (
	ErrWebAuthnChallengeNotFound = errors.New("WebAuthn challenge not found or expired")
	ErrWebAuthnChallengeUsed     = errors.New("WebAuthn challenge already verified")
	ErrWebAuthnClonedKey         = errors.New("cloned authenticator detected")
	ErrWebAuthnInvalidAssertion  = errors.New("WebAuthn assertion verification failed")
)

const (
	webAuthnChallengeScopeMFA         = "mfa"
	webAuthnChallengeScopeFirstFactor = "webauthn_first_factor"
)

func (s *Service) CreateWebAuthnChallenge(ctx context.Context, userID, ipAddress, publicBaseURL string) (string, *protocol.CredentialAssertion, error) {
	return s.createWebAuthnChallengeForScope(ctx, userID, ipAddress, publicBaseURL, webAuthnChallengeScopeMFA)
}

func (s *Service) createWebAuthnChallengeForScope(ctx context.Context, userID, ipAddress, publicBaseURL, challengeScope string) (string, *protocol.CredentialAssertion, error) {
	if s.pool == nil {
		return "", nil, errors.New("database pool is not configured")
	}

	factorID, credentialRows, err := s.loadEnabledWebAuthnCredentialRows(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrWebAuthnNotEnrolled) {
			return "", nil, ErrWebAuthnNotEnrolled
		}
		return "", nil, err
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return "", nil, fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	wUser := &webauthnUser{
		id:          []byte(userID),
		name:        "",
		credentials: webAuthnCredentialsFromRows(credentialRows),
	}

	assertion, session, err := wa.BeginLogin(wUser)
	if err != nil {
		return "", nil, fmt.Errorf("beginning WebAuthn login: %w", err)
	}

	sessionBytes, err := json.Marshal(session)
	if err != nil {
		return "", nil, fmt.Errorf("serializing WebAuthn login session: %w", err)
	}

	var challengeID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_mfa_challenges (factor_id, ip_address, webauthn_session_data, challenge_scope)
		 VALUES ($1, $2::inet, $3, $4)
		 RETURNING id`,
		factorID, ipAddress, sessionBytes, challengeScope,
	).Scan(&challengeID)
	if err != nil {
		return "", nil, fmt.Errorf("creating WebAuthn challenge: %w", err)
	}

	s.logger.Info("WebAuthn challenge created", "user_id", userID, "challenge_id", challengeID)
	return challengeID, assertion, nil
}

func (s *Service) VerifyWebAuthnChallenge(ctx context.Context, userID, challengeID, publicBaseURL string, parsedAssertion *protocol.ParsedCredentialAssertionData, firstFactorMethod string) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	factorID, session, err := s.loadWebAuthnChallenge(ctx, userID, challengeID, webAuthnChallengeScopeMFA)
	if err != nil {
		return nil, "", "", err
	}

	credential, err := s.validateWebAuthnAssertion(ctx, userID, factorID, publicBaseURL, session, parsedAssertion)
	if err != nil {
		return nil, "", "", err
	}

	if err := s.commitWebAuthnVerification(ctx, factorID, challengeID, credential.ID, int64(credential.Authenticator.SignCount)); err != nil {
		return nil, "", "", err
	}

	user, token, refreshToken, err := s.issueWebAuthnSession(ctx, userID, firstFactorMethod)
	if err != nil {
		return nil, "", "", err
	}

	s.logger.Info("WebAuthn MFA verified", "user_id", userID, "challenge_id", challengeID)
	return user, token, refreshToken, nil
}

func (s *Service) VerifyWebAuthnFirstFactorChallenge(ctx context.Context, challengeID, publicBaseURL string, parsedAssertion *protocol.ParsedCredentialAssertionData) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	userID, err := s.lookupWebAuthnChallengeUserID(ctx, challengeID, webAuthnChallengeScopeFirstFactor)
	if err != nil {
		return nil, "", "", err
	}

	factorID, session, err := s.loadWebAuthnChallenge(ctx, userID, challengeID, webAuthnChallengeScopeFirstFactor)
	if err != nil {
		return nil, "", "", err
	}

	credential, err := s.validateWebAuthnAssertion(ctx, userID, factorID, publicBaseURL, session, parsedAssertion)
	if err != nil {
		return nil, "", "", err
	}

	if err := s.commitWebAuthnVerification(ctx, factorID, challengeID, credential.ID, int64(credential.Authenticator.SignCount)); err != nil {
		return nil, "", "", err
	}

	user, token, refreshToken, err := s.issueWebAuthnFirstFactorSession(ctx, userID)
	if err != nil {
		return nil, "", "", err
	}

	s.logger.Info("WebAuthn first-factor verified", "user_id", userID, "challenge_id", challengeID)
	return user, token, refreshToken, nil
}

// loadWebAuthnChallenge fetches the challenge row, enforces single-use and
// expiry semantics, and returns the bound factor ID plus the deserialized
// go-webauthn session needed by ValidateLogin.
func (s *Service) loadWebAuthnChallenge(ctx context.Context, userID, challengeID, challengeScope string) (string, webauthn.SessionData, error) {
	var (
		factorID     string
		verifiedAt   *time.Time
		expiresAt    time.Time
		sessionBytes []byte
	)
	err := s.pool.QueryRow(ctx,
		`SELECT c.factor_id, c.verified_at, c.expires_at, c.webauthn_session_data
		 FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE c.id = $1
		   AND f.user_id = $2
		   AND c.challenge_scope = $3
		   AND f.method = 'webauthn'
		   AND f.enabled = true`,
		challengeID, userID, challengeScope,
	).Scan(&factorID, &verifiedAt, &expiresAt, &sessionBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", webauthn.SessionData{}, ErrWebAuthnChallengeNotFound
		}
		return "", webauthn.SessionData{}, fmt.Errorf("loading WebAuthn challenge: %w", err)
	}
	if verifiedAt != nil {
		return "", webauthn.SessionData{}, ErrWebAuthnChallengeUsed
	}
	if time.Now().After(expiresAt) {
		return "", webauthn.SessionData{}, ErrWebAuthnChallengeNotFound
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		return "", webauthn.SessionData{}, fmt.Errorf("deserializing WebAuthn session: %w", err)
	}
	return factorID, session, nil
}

// validateWebAuthnAssertion loads the stored credential for factorID and
// runs the go-webauthn protocol check, returning ErrWebAuthnInvalidAssertion
// or ErrWebAuthnClonedKey without mutating any stored counters.
func (s *Service) validateWebAuthnAssertion(ctx context.Context, userID, factorID, publicBaseURL string, session webauthn.SessionData, parsedAssertion *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	credentialRows, err := s.loadWebAuthnCredentialRowsForFactor(ctx, factorID)
	if err != nil {
		if errors.Is(err, ErrWebAuthnNotEnrolled) {
			return nil, ErrWebAuthnChallengeNotFound
		}
		return nil, err
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	wUser := &webauthnUser{
		id:          []byte(userID),
		name:        "",
		credentials: webAuthnCredentialsFromRows(credentialRows),
	}

	credential, err := wa.ValidateLogin(wUser, session, parsedAssertion)
	if err != nil {
		return nil, ErrWebAuthnInvalidAssertion
	}
	if credential.Authenticator.CloneWarning {
		return nil, ErrWebAuthnClonedKey
	}
	return credential, nil
}

// commitWebAuthnVerification persists the new sign count and marks the
// challenge consumed atomically so retries can't re-use the same proof.
func (s *Service) commitWebAuthnVerification(ctx context.Context, factorID, challengeID string, credentialID []byte, newSignCount int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	challengeResult, err := tx.Exec(ctx,
		`UPDATE _ayb_mfa_challenges
		 SET verified_at = NOW()
		 WHERE id = $1 AND verified_at IS NULL`,
		challengeID,
	)
	if err != nil {
		return fmt.Errorf("marking challenge verified: %w", err)
	}
	if challengeResult.RowsAffected() != 1 {
		return ErrWebAuthnChallengeUsed
	}

	if err := updateWebAuthnCredentialUsage(ctx, tx, factorID, credentialID, newSignCount); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing verification: %w", err)
	}
	return nil
}

type webAuthnCredentialUsageTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func updateWebAuthnCredentialUsage(ctx context.Context, tx webAuthnCredentialUsageTx, factorID string, credentialID []byte, newSignCount int64) error {
	credentialResult, err := tx.Exec(ctx,
		`UPDATE _ayb_webauthn_credentials c
		    SET sign_count = GREATEST(c.sign_count, $3),
		        last_used_at = NOW()
		   FROM _ayb_user_mfa f
		  WHERE c.factor_id = f.id
		    AND c.factor_id = $1
		    AND c.credential_id = $2
		    AND f.method = 'webauthn'
		    AND f.enabled = true`,
		factorID, credentialID, newSignCount,
	)
	if err != nil {
		return fmt.Errorf("updating sign count: %w", err)
	}
	if credentialResult.RowsAffected() != 1 {
		return ErrWebAuthnChallengeNotFound
	}
	return nil
}

func (s *Service) issueWebAuthnSession(ctx context.Context, userID, firstFactorMethod string) (*User, string, string, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	sessionOpts := mfaSessionOptions(firstFactorMethod, "webauthn")
	sessionID, refreshToken, err := s.createSession(ctx, user.ID, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating session: %w", err)
	}
	sessionOpts.SessionID = sessionID
	sessionOpts, err = s.sessionTokenOptions(ctx, user, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolving session tenant: %w", err)
	}

	token, err := s.generateTokenWithOpts(ctx, user, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating AAL2 token: %w", err)
	}
	return user, token, refreshToken, nil
}

func (s *Service) issueWebAuthnFirstFactorSession(ctx context.Context, userID string) (*User, string, string, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}
	return s.issueTokensWithFirstFactorMethod(ctx, user, "webauthn")
}

func (s *Service) lookupWebAuthnFirstFactorUserByEmail(ctx context.Context, email string) (string, error) {
	normalizedEmail := normalizeAuthEmail(email)
	user, _, err := s.lookupUserByNormalizedEmail(ctx, normalizedEmail)
	if err != nil {
		return "", err
	}

	var factorID string
	err = s.pool.QueryRow(ctx,
		`SELECT id
		 FROM _ayb_user_mfa
		 WHERE user_id = $1
		   AND method = 'webauthn'
		   AND enabled = true
		 LIMIT 1`,
		user.ID,
	).Scan(&factorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrWebAuthnNotEnrolled
		}
		return "", fmt.Errorf("looking up WebAuthn first-factor user: %w", err)
	}
	return user.ID, nil
}

// createWebAuthnFirstFactorDecoyChallenge returns a non-persisted challenge for
// unknown or non-passkey accounts. The credential identifier is derived from a
// secret plus the normalized email so callers cannot group all decoy responses
// by a shared constant ID.
func (s *Service) createWebAuthnFirstFactorDecoyChallenge(normalizedEmail, publicBaseURL string) (string, *protocol.CredentialAssertion, error) {
	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return "", nil, fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	decoyCredentialID := s.deriveWebAuthnFirstFactorDecoyID(normalizedEmail)
	decoyUser := &webauthnUser{
		id:   decoyCredentialID,
		name: "",
		credentials: []webauthn.Credential{
			{
				ID:        decoyCredentialID,
				PublicKey: []byte{1},
				Authenticator: webauthn.Authenticator{
					SignCount: 0,
				},
			},
		},
	}

	assertion, _, err := wa.BeginLogin(decoyUser)
	if err != nil {
		return "", nil, fmt.Errorf("building decoy WebAuthn challenge: %w", err)
	}

	return uuid.NewString(), assertion, nil
}

func (s *Service) deriveWebAuthnFirstFactorDecoyID(normalizedEmail string) []byte {
	s.jwtSecretMu.RLock()
	secret := append([]byte(nil), s.jwtSecret...)
	s.jwtSecretMu.RUnlock()

	mac := hmac.New(sha256.New, secret)
	if len(secret) == 0 {
		// Tests sometimes construct a zero-value Service directly; still derive
		// per-email bytes instead of reusing a global constant marker.
		mac.Write([]byte("no-secret"))
	}
	mac.Write([]byte("ayb-webauthn-first-factor-decoy"))
	mac.Write([]byte{0})
	mac.Write([]byte(normalizedEmail))
	return mac.Sum(nil)
}

func (s *Service) lookupWebAuthnChallengeUserID(ctx context.Context, challengeID, challengeScope string) (string, error) {
	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT f.user_id
		 FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE c.id = $1
		   AND c.challenge_scope = $2
		   AND f.method = 'webauthn'
		   AND f.enabled = true`,
		challengeID, challengeScope,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrWebAuthnChallengeNotFound
		}
		return "", fmt.Errorf("looking up WebAuthn challenge user: %w", err)
	}
	return userID, nil
}
