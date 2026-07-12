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
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrWebAuthnAlreadyEnrolled      = errors.New("WebAuthn MFA already enrolled")
	ErrWebAuthnNotEnrolled          = errors.New("no WebAuthn factor found")
	ErrWebAuthnEnrollmentNotPending = errors.New("no pending WebAuthn enrollment found")
	ErrWebAuthnInvalidAttestation   = errors.New("WebAuthn enrollment verification failed")
	ErrWebAuthnCredentialNotFound   = errors.New("WebAuthn credential not found")
	ErrWebAuthnLastCredential       = errors.New("cannot delete final WebAuthn credential")
)

func (s *Service) EnrollWebAuthn(ctx context.Context, userID, publicBaseURL string) (*protocol.CredentialCreation, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}

	wa, err := newWebAuthnVerifier(publicBaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn verifier: %w", err)
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("looking up user for WebAuthn enroll: %w", err)
	}

	existingCredentials, err := s.loadWebAuthnCredentialsForEnrollment(ctx, userID)
	if err != nil {
		return nil, err
	}

	wUser := &webauthnUser{
		id:   []byte(user.ID),
		name: user.Email,
	}

	creation, session, err := wa.BeginRegistration(wUser,
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		webauthn.WithExclusions(webauthn.Credentials(existingCredentials).CredentialDescriptors()),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementPreferred,
			RequireResidentKey: protocol.ResidentKeyNotRequired(),
			UserVerification:   protocol.VerificationPreferred,
		}),
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
		 SET webauthn_session_data = $2`,
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

	var (
		factorID     string
		sessionBytes []byte
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, webauthn_session_data FROM _ayb_user_mfa
		 WHERE user_id = $1 AND method = 'webauthn'
		   AND webauthn_session_data IS NOT NULL`,
		userID,
	).Scan(&factorID, &sessionBytes)
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning WebAuthn enrollment transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx,
		`UPDATE _ayb_user_mfa
		 SET enabled = true,
		     webauthn_session_data = NULL,
		     enrolled_at = COALESCE(enrolled_at, NOW())
		 WHERE id = $1 AND user_id = $2 AND method = 'webauthn'
		   AND webauthn_session_data = $3`,
		factorID, userID, sessionBytes,
	)
	if err != nil {
		return fmt.Errorf("activating WebAuthn factor: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrWebAuthnEnrollmentNotPending
	}

	transports := webAuthnTransportStrings(credential.Transport)
	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_webauthn_credentials (
		     factor_id, credential_id, public_key, transports, sign_count, display_name
		 )
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		factorID,
		credential.ID,
		credential.PublicKey,
		transports,
		int64(credential.Authenticator.SignCount),
		trimmedDisplayName,
	)
	if err != nil {
		if isWebAuthnCredentialConflict(err) {
			return ErrWebAuthnInvalidAttestation
		}
		return fmt.Errorf("persisting WebAuthn credential: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing WebAuthn enrollment: %w", err)
	}

	s.logger.Info("WebAuthn enrollment confirmed", "user_id", userID)
	return nil
}

type webAuthnStoredCredential struct {
	credential webauthn.Credential
}

func (s *Service) loadWebAuthnCredentialsForEnrollment(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	_, rows, err := s.loadEnabledWebAuthnCredentialRows(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrWebAuthnNotEnrolled) {
			return nil, nil
		}
		return nil, err
	}
	return webAuthnCredentialsFromRows(rows), nil
}

func (s *Service) loadEnabledWebAuthnCredentialRows(ctx context.Context, userID string) (string, []webAuthnStoredCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT f.id::text, c.credential_id, c.public_key, c.transports, c.sign_count
		   FROM _ayb_user_mfa f
		   LEFT JOIN _ayb_webauthn_credentials c ON c.factor_id = f.id
		  WHERE f.user_id = $1 AND f.method = 'webauthn' AND f.enabled = true
		  ORDER BY c.created_at, c.id`,
		userID,
	)
	if err != nil {
		return "", nil, fmt.Errorf("loading WebAuthn credentials: %w", err)
	}
	defer rows.Close()

	return scanWebAuthnCredentialRows(rows)
}

func (s *Service) loadWebAuthnCredentialRowsForFactor(ctx context.Context, factorID string) ([]webAuthnStoredCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT f.id::text, c.credential_id, c.public_key, c.transports, c.sign_count
		   FROM _ayb_user_mfa f
		   JOIN _ayb_webauthn_credentials c ON c.factor_id = f.id
		  WHERE f.id = $1 AND f.method = 'webauthn' AND f.enabled = true
		  ORDER BY c.created_at, c.id`,
		factorID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading WebAuthn credentials: %w", err)
	}
	defer rows.Close()

	_, credentialRows, err := scanWebAuthnCredentialRows(rows)
	return credentialRows, err
}

type webAuthnCredentialScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanWebAuthnCredentialRows(rows webAuthnCredentialScanner) (string, []webAuthnStoredCredential, error) {
	var (
		factorID       string
		credentialRows []webAuthnStoredCredential
	)
	for rows.Next() {
		var (
			rowFactorID  string
			credentialID []byte
			publicKey    []byte
			transports   []string
			signCount    *int64
		)
		if err := rows.Scan(&rowFactorID, &credentialID, &publicKey, &transports, &signCount); err != nil {
			return "", nil, fmt.Errorf("scanning WebAuthn credential: %w", err)
		}
		if factorID == "" {
			factorID = rowFactorID
		}
		if credentialID == nil {
			continue
		}
		credentialRows = append(credentialRows, webAuthnStoredCredential{
			credential: webauthn.Credential{
				ID:        credentialID,
				PublicKey: publicKey,
				Authenticator: webauthn.Authenticator{
					SignCount: uint32(*signCount),
				},
			},
		})
	}
	if err := rows.Err(); err != nil {
		return "", nil, fmt.Errorf("iterating WebAuthn credentials: %w", err)
	}
	if factorID == "" || len(credentialRows) == 0 {
		return "", nil, ErrWebAuthnNotEnrolled
	}
	return factorID, credentialRows, nil
}

func webAuthnCredentialsFromRows(rows []webAuthnStoredCredential) []webauthn.Credential {
	credentials := make([]webauthn.Credential, 0, len(rows))
	for _, row := range rows {
		credentials = append(credentials, row.credential)
	}
	return credentials
}

func webAuthnTransportStrings(transports []protocol.AuthenticatorTransport) []string {
	result := make([]string, 0, len(transports))
	for _, transport := range transports {
		result = append(result, string(transport))
	}
	return result
}

func isWebAuthnCredentialConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "uq_ayb_webauthn_credentials_credential_id"
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
