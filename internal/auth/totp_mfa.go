// Package auth Provides TOTP multi-factor authentication (RFC 6238) with enrollment, verification, and utilities for managing MFA factors using AES-256-GCM encryption and replay protection.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// TOTP configuration constants (RFC 6238, Google Authenticator compatible).
const (
	totpDigits = 6
	totpPeriod = 30 // seconds
	totpSkew   = 1  // accept ±1 adjacent time window
	totpKeyLen = 20 // bytes (160-bit HMAC-SHA1 key)
)

// TOTP sentinel errors.
var (
	ErrTOTPAlreadyEnrolled   = errors.New("TOTP MFA already enrolled")
	ErrTOTPNotEnrolled       = errors.New("no TOTP factor found")
	ErrTOTPInvalidCode       = errors.New("invalid TOTP code")
	ErrTOTPReplay            = errors.New("TOTP code already used")
	ErrTOTPChallengeNotFound = errors.New("MFA challenge not found or expired")
	ErrTOTPChallengeUsed     = errors.New("MFA challenge already verified")
	ErrEncryptionKeyNotSet   = errors.New("encryption key not configured")
)

// TOTPEnrollment holds the data returned to the client during TOTP enrollment.
type TOTPEnrollment struct {
	FactorID string `json:"factor_id"`
	URI      string `json:"uri"`
	Secret   string `json:"secret"` // base32-encoded, shown once
}

// EnrollTOTP starts TOTP enrollment for a user. Generates a secret, encrypts
// and stores it as an unverified factor. Returns enrollment data for the client.
func (s *Service) EnrollTOTP(ctx context.Context, userID, email, issuer string) (*TOTPEnrollment, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}
	if len(s.encryptionKey) == 0 {
		return nil, ErrEncryptionKeyNotSet
	}

	// Check for existing verified TOTP factor.
	var existing bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'totp' AND enabled = true)`,
		userID,
	).Scan(&existing)
	if err != nil {
		return nil, fmt.Errorf("checking TOTP enrollment: %w", err)
	}
	if existing {
		return nil, ErrTOTPAlreadyEnrolled
	}

	// Generate random TOTP secret.
	secret := make([]byte, totpKeyLen)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return nil, fmt.Errorf("generating TOTP secret: %w", err)
	}

	// Encrypt the secret for storage.
	encrypted, err := s.encryptAESGCM(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting TOTP secret: %w", err)
	}

	// Upsert: replace any existing unverified enrollment.
	var factorID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_user_mfa (user_id, method, totp_secret_enc, enabled)
		 VALUES ($1, 'totp', $2, false)
		 ON CONFLICT (user_id, method) DO UPDATE
		 SET totp_secret_enc = $2, enabled = false, totp_enrolled_at = NULL, last_used_step = 0
		 RETURNING id`,
		userID, encrypted,
	).Scan(&factorID)
	if err != nil {
		return nil, fmt.Errorf("inserting TOTP enrollment: %w", err)
	}

	b32Secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	uri := buildOTPAuthURI(secret, email, issuer)

	s.logger.Info("TOTP enrollment started", "user_id", userID, "factor_id", factorID)
	return &TOTPEnrollment{
		FactorID: factorID,
		URI:      uri,
		Secret:   b32Secret,
	}, nil
}

// ConfirmTOTPEnrollment verifies the user's first TOTP code and activates the factor.
func (s *Service) ConfirmTOTPEnrollment(ctx context.Context, userID, code string) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	// Load the unverified factor.
	var factorID string
	var secretEnc []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, totp_secret_enc FROM _ayb_user_mfa
		 WHERE user_id = $1 AND method = 'totp' AND enabled = false`,
		userID,
	).Scan(&factorID, &secretEnc)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTOTPNotEnrolled
		}
		return fmt.Errorf("loading TOTP factor: %w", err)
	}

	secret, err := s.decryptAESGCM(secretEnc)
	if err != nil {
		return fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	// Validate the code.
	_, ok := validateTOTPCode(secret, code, time.Now())
	if !ok {
		return ErrTOTPInvalidCode
	}

	// Activate the factor.
	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_user_mfa SET enabled = true, enrolled_at = NOW(), totp_enrolled_at = NOW()
		 WHERE id = $1`,
		factorID,
	)
	if err != nil {
		return fmt.Errorf("confirming TOTP enrollment: %w", err)
	}

	s.logger.Info("TOTP enrollment confirmed", "user_id", userID, "factor_id", factorID)
	return nil
}

// DefaultUnverifiedTOTPTTL is the default time-to-live for unverified TOTP
// enrollments. Enrollments older than this are cleaned up to prevent bloat.
const DefaultUnverifiedTOTPTTL = 10 * time.Minute

// CleanupUnverifiedTOTPEnrollments deletes unverified TOTP enrollments older
// than the specified TTL. This prevents stale factor bloat from abandoned enrollments.
func (s *Service) CleanupUnverifiedTOTPEnrollments(ctx context.Context, ttl time.Duration) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}
	cutoff := time.Now().Add(-ttl)
	result, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_user_mfa WHERE method = 'totp' AND enabled = false AND created_at < $1`,
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("cleaning up unverified TOTP enrollments: %w", err)
	}
	if result.RowsAffected() > 0 {
		s.logger.Info("cleaned up unverified TOTP enrollments", "count", result.RowsAffected())
	}
	return nil
}

// HasTOTPMFA checks whether a user has an enabled TOTP MFA enrollment.
func (s *Service) HasTOTPMFA(ctx context.Context, userID string) (bool, error) {
	if s.pool == nil {
		return false, errors.New("database pool is not configured")
	}
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'totp' AND enabled = true)`,
		userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking TOTP MFA enrollment: %w", err)
	}
	return exists, nil
}

// HasAnyMFA checks whether a user has any enabled MFA factor (SMS, TOTP, email).
func (s *Service) HasAnyMFA(ctx context.Context, userID string) (bool, string, error) {
	if s.pool == nil {
		return false, "", errors.New("database pool is not configured")
	}
	var method string
	err := s.pool.QueryRow(ctx,
		`SELECT method FROM _ayb_user_mfa WHERE user_id = $1 AND enabled = true ORDER BY enrolled_at LIMIT 1`,
		userID,
	).Scan(&method)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	return true, method, nil
}

// GetUserMFAFactors returns all enabled MFA factors for a user.
type MFAFactor struct {
	ID          string `json:"id"`
	Method      string `json:"method"`
	Label       string `json:"label"`                  // human-readable: "Authenticator app", masked phone/email
	Phone       string `json:"phone,omitempty"`        // e.g. "***1234"
	Email       string `json:"email,omitempty"`        // e.g. "t***@example.com"
	DisplayName string `json:"display_name,omitempty"` // raw user-entered label for passkeys
}

// maskEmail masks an email for display, e.g. "test@example.com" → "t***@example.com".
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	return string(email[0]) + "***" + email[at:]
}

// GetUserMFAFactors returns all enabled MFA factors for a user in enrollment order, masking sensitive data for display: SMS numbers show only the last 4 digits, and emails show the first letter and domain.
func (s *Service) GetUserMFAFactors(ctx context.Context, userID string) ([]MFAFactor, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}
	rows, err := s.pool.Query(ctx,
		`SELECT f.id, f.method, COALESCE(f.phone, ''), COALESCE(u.email, ''), COALESCE(f.webauthn_display_name, '')
		 FROM _ayb_user_mfa f
		 JOIN _ayb_users u ON u.id = f.user_id
		 WHERE f.user_id = $1 AND f.enabled = true
		 ORDER BY f.enrolled_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying MFA factors: %w", err)
	}
	defer rows.Close()

	var factors []MFAFactor
	var userEmail string
	for rows.Next() {
		var f MFAFactor
		if err := rows.Scan(&f.ID, &f.Method, &f.Phone, &userEmail, &f.DisplayName); err != nil {
			return nil, fmt.Errorf("scanning MFA factor: %w", err)
		}
		// Set method-specific display fields.
		switch f.Method {
		case "sms":
			if f.Phone != "" && len(f.Phone) > 4 {
				f.Phone = strings.Repeat("*", len(f.Phone)-4) + f.Phone[len(f.Phone)-4:]
			}
			f.Label = "SMS (" + f.Phone + ")"
		case "email":
			if userEmail != "" {
				f.Email = maskEmail(userEmail)
			}
			f.Label = "Email (" + f.Email + ")"
		case "totp":
			f.Label = "Authenticator app"
		case "webauthn":
			if f.DisplayName != "" {
				f.Label = f.DisplayName
			} else {
				f.Label = "Passkey"
			}
		default:
			f.Label = f.Method
		}
		factors = append(factors, f)
	}
	return factors, rows.Err()
}
