package auth

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-webauthn/webauthn/webauthn"
)

const webauthnRPDisplayName = "Allyourbase"

func newWebAuthnVerifier(publicBaseURL string) (*webauthn.WebAuthn, error) {
	rpID, err := deriveRPID(publicBaseURL)
	if err != nil {
		return nil, err
	}

	origin := strings.TrimRight(publicBaseURL, "/")

	return webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         webauthnRPDisplayName,
		RPOrigins:             []string{origin},
		AttestationPreference: "none",
	})
}

func deriveRPID(publicBaseURL string) (string, error) {
	u, err := url.Parse(publicBaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing public base URL for WebAuthn RP ID: %w", err)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("public base URL has no hostname: %s", publicBaseURL)
	}
	return strings.ToLower(host), nil
}
