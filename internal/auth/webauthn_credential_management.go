package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type WebAuthnCredentialMetadata struct {
	CredentialID string     `json:"credential_id"`
	DisplayName  string     `json:"display_name"`
	Transports   []string   `json:"transports"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// ListWebAuthnCredentials returns metadata for the user's enabled WebAuthn
// credentials, ordered by creation time.
func (s *Service) ListWebAuthnCredentials(ctx context.Context, userID string) ([]WebAuthnCredentialMetadata, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}

	rows, err := s.pool.Query(ctx,
		`SELECT c.credential_id, c.display_name, c.transports, c.created_at, c.last_used_at
		   FROM _ayb_user_mfa f
		   JOIN _ayb_webauthn_credentials c ON c.factor_id = f.id
		  WHERE f.user_id = $1 AND f.method = 'webauthn' AND f.enabled = true
		  ORDER BY c.created_at, c.id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing WebAuthn credentials: %w", err)
	}
	defer rows.Close()

	credentials := []WebAuthnCredentialMetadata{}
	for rows.Next() {
		credential, err := scanWebAuthnCredentialMetadata(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating WebAuthn credentials: %w", err)
	}
	return credentials, nil
}

// RenameWebAuthnCredential sets the display name of a WebAuthn credential owned by
// the user, returning the updated metadata and sending a credential-changed
// notification email. It returns ErrWebAuthnCredentialNotFound when the user has
// no matching enabled credential.
func (s *Service) RenameWebAuthnCredential(
	ctx context.Context,
	userID string,
	credentialID []byte,
	displayName string,
) (*WebAuthnCredentialMetadata, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}

	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_webauthn_credentials c
		    SET display_name = $3
		   FROM _ayb_user_mfa f
		  WHERE c.factor_id = f.id
		    AND f.user_id = $1
		    AND f.method = 'webauthn'
		    AND f.enabled = true
		    AND c.credential_id = $2
		  RETURNING c.credential_id, c.display_name, c.transports, c.created_at, c.last_used_at`,
		userID, credentialID, strings.TrimSpace(displayName),
	)

	credential, err := scanWebAuthnCredentialMetadata(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWebAuthnCredentialNotFound
		}
		return nil, err
	}
	s.sendWebAuthnCredentialChangedEmail(ctx, userID, "renamed", credential.DisplayName)
	return &credential, nil
}

// DeleteWebAuthnCredential removes a WebAuthn credential owned by the user. It
// refuses to delete the user's last remaining passkey with ErrWebAuthnLastCredential
// so the factor cannot be silently disabled, and sends a credential-changed
// notification on success.
func (s *Service) DeleteWebAuthnCredential(ctx context.Context, userID string, credentialID []byte) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning WebAuthn credential delete transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	credentialCount, err := webAuthnCredentialCountForDelete(ctx, tx, userID, credentialID)
	if err != nil {
		return err
	}
	if credentialCount <= 1 {
		if _, _, err := s.HasAnyMFA(ctx, userID); err != nil {
			return err
		}
		return ErrWebAuthnLastCredential
	}

	displayName, err := webAuthnCredentialDisplayNameForDelete(ctx, tx, userID, credentialID)
	if err != nil {
		return err
	}

	result, err := tx.Exec(ctx,
		`DELETE FROM _ayb_webauthn_credentials c
		  USING _ayb_user_mfa f
		  WHERE c.factor_id = f.id
		    AND f.user_id = $1
		    AND f.method = 'webauthn'
		    AND f.enabled = true
		    AND c.credential_id = $2`,
		userID, credentialID,
	)
	if err != nil {
		return fmt.Errorf("deleting WebAuthn credential: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrWebAuthnCredentialNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing WebAuthn credential delete: %w", err)
	}
	s.sendWebAuthnCredentialChangedEmail(ctx, userID, "deleted", displayName)
	return nil
}

type webAuthnCredentialMetadataScanner interface {
	Scan(dest ...any) error
}

func scanWebAuthnCredentialMetadata(row webAuthnCredentialMetadataScanner) (WebAuthnCredentialMetadata, error) {
	var (
		rawID      []byte
		metadata   WebAuthnCredentialMetadata
		lastUsedAt *time.Time
	)
	err := row.Scan(&rawID, &metadata.DisplayName, &metadata.Transports, &metadata.CreatedAt, &lastUsedAt)
	if err != nil {
		return WebAuthnCredentialMetadata{}, fmt.Errorf("scanning WebAuthn credential metadata: %w", err)
	}
	metadata.CredentialID = base64.RawURLEncoding.EncodeToString(rawID)
	metadata.LastUsedAt = lastUsedAt
	return metadata, nil
}

type webAuthnCredentialDeleteTx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func webAuthnCredentialCountForDelete(
	ctx context.Context,
	tx webAuthnCredentialDeleteTx,
	userID string,
	credentialID []byte,
) (int, error) {
	var credentialCount int
	err := tx.QueryRow(ctx,
		`WITH target AS (
		     SELECT c.factor_id
		       FROM _ayb_webauthn_credentials c
		       JOIN _ayb_user_mfa f ON f.id = c.factor_id
		      WHERE f.user_id = $1
		        AND f.method = 'webauthn'
		        AND f.enabled = true
		        AND c.credential_id = $2
		 ),
		 locked_credentials AS (
		     SELECT c.id
		       FROM _ayb_webauthn_credentials c
		       JOIN target t ON t.factor_id = c.factor_id
		      FOR UPDATE OF c
		 )
		 SELECT COUNT(id)
		   FROM locked_credentials`,
		userID, credentialID,
	).Scan(&credentialCount)
	if err != nil {
		return 0, fmt.Errorf("counting WebAuthn credentials: %w", err)
	}
	if credentialCount == 0 {
		return 0, ErrWebAuthnCredentialNotFound
	}
	return credentialCount, nil
}

func webAuthnCredentialDisplayNameForDelete(
	ctx context.Context,
	tx webAuthnCredentialDeleteTx,
	userID string,
	credentialID []byte,
) (string, error) {
	var displayName string
	err := tx.QueryRow(ctx,
		`SELECT c.display_name
		   FROM _ayb_webauthn_credentials c
		   JOIN _ayb_user_mfa f ON f.id = c.factor_id
		  WHERE f.user_id = $1
		    AND f.method = 'webauthn'
		    AND f.enabled = true
		    AND c.credential_id = $2`,
		userID, credentialID,
	).Scan(&displayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrWebAuthnCredentialNotFound
		}
		return "", fmt.Errorf("loading WebAuthn credential display name: %w", err)
	}
	return displayName, nil
}
