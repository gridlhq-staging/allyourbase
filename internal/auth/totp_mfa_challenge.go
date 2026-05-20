package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type totpChallengeRecord struct {
	factorID string
}

type totpSecretState struct {
	secret       []byte
	lastUsedStep int64
}

// CreateTOTPChallenge creates a challenge record for the TOTP factor.
// Returns the challenge ID.
func (s *Service) CreateTOTPChallenge(ctx context.Context, userID, ipAddress string) (string, error) {
	if s.pool == nil {
		return "", errors.New("database pool is not configured")
	}

	var factorID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'totp' AND enabled = true`,
		userID,
	).Scan(&factorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrTOTPNotEnrolled
		}
		return "", fmt.Errorf("looking up TOTP factor: %w", err)
	}

	var challengeID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_mfa_challenges (factor_id, ip_address)
		 VALUES ($1, $2::inet)
		 RETURNING id`,
		factorID, ipAddress,
	).Scan(&challengeID)
	if err != nil {
		return "", fmt.Errorf("creating MFA challenge: %w", err)
	}

	s.logger.Info("TOTP challenge created", "user_id", userID, "challenge_id", challengeID)
	return challengeID, nil
}

// VerifyTOTPChallenge verifies a TOTP code against a specific challenge.
// On success, issues AAL2 tokens.
func (s *Service) VerifyTOTPChallenge(ctx context.Context, userID, challengeID, code, firstFactorMethod string) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	challenge, err := s.loadTOTPChallenge(ctx, userID, challengeID, time.Now())
	if err != nil {
		return nil, "", "", err
	}
	secretState, err := s.loadTOTPSecretState(ctx, challenge.factorID)
	if err != nil {
		return nil, "", "", err
	}
	matchedStep, err := validateTOTPChallengeCode(secretState, code, time.Now())
	if err != nil {
		return nil, "", "", err
	}
	if err := s.markTOTPChallengeVerified(ctx, challengeID, challenge.factorID, matchedStep); err != nil {
		return nil, "", "", err
	}
	user, token, refreshToken, err := s.issueTOTPSession(ctx, userID, firstFactorMethod)
	if err != nil {
		return nil, "", "", err
	}

	s.logger.Info("TOTP MFA verified", "user_id", userID, "challenge_id", challengeID)
	return user, token, refreshToken, nil
}

func (s *Service) loadTOTPChallenge(ctx context.Context, userID, challengeID string, now time.Time) (totpChallengeRecord, error) {
	var (
		record     totpChallengeRecord
		verifiedAt *time.Time
		expiresAt  time.Time
	)

	err := s.pool.QueryRow(ctx,
		`SELECT c.factor_id, c.verified_at, c.expires_at
		 FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE c.id = $1 AND f.user_id = $2`,
		challengeID, userID,
	).Scan(&record.factorID, &verifiedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return totpChallengeRecord{}, ErrTOTPChallengeNotFound
		}
		return totpChallengeRecord{}, fmt.Errorf("loading MFA challenge: %w", err)
	}
	if verifiedAt != nil {
		return totpChallengeRecord{}, ErrTOTPChallengeUsed
	}
	if now.After(expiresAt) {
		return totpChallengeRecord{}, ErrTOTPChallengeNotFound
	}
	return record, nil
}

func (s *Service) loadTOTPSecretState(ctx context.Context, factorID string) (totpSecretState, error) {
	var (
		secretEnc []byte
		state     totpSecretState
	)

	err := s.pool.QueryRow(ctx,
		`SELECT totp_secret_enc, COALESCE(last_used_step, 0) FROM _ayb_user_mfa WHERE id = $1`,
		factorID,
	).Scan(&secretEnc, &state.lastUsedStep)
	if err != nil {
		return totpSecretState{}, fmt.Errorf("loading TOTP secret: %w", err)
	}

	state.secret, err = s.decryptAESGCM(secretEnc)
	if err != nil {
		return totpSecretState{}, fmt.Errorf("decrypting TOTP secret: %w", err)
	}
	return state, nil
}

func validateTOTPChallengeCode(state totpSecretState, code string, now time.Time) (int64, error) {
	matchedStep, ok := validateTOTPCode(state.secret, code, now)
	if !ok {
		return 0, ErrTOTPInvalidCode
	}
	if matchedStep <= state.lastUsedStep {
		return 0, ErrTOTPReplay
	}
	return matchedStep, nil
}

func (s *Service) markTOTPChallengeVerified(ctx context.Context, challengeID, factorID string, matchedStep int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_mfa_challenges SET verified_at = NOW() WHERE id = $1`,
		challengeID,
	)
	if err != nil {
		return fmt.Errorf("marking challenge verified: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_user_mfa SET last_used_step = $1 WHERE id = $2`,
		matchedStep, factorID,
	)
	if err != nil {
		return fmt.Errorf("updating last used step: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing MFA verification: %w", err)
	}
	return nil
}

func (s *Service) issueTOTPSession(ctx context.Context, userID, firstFactorMethod string) (*User, string, string, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	sessionOpts := mfaSessionOptions(firstFactorMethod, "totp")
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
