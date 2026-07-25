// Package auth Implements SAML service provider integration for federated authentication, handling request creation, assertion parsing and validation, provider configuration, and certificate management.
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/config"
)

const samlRequestTTL = 5 * time.Minute

var errSAMLProviderNotFound = errors.New("saml provider not configured")

var samlProviderNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type SAMLAssertion struct {
	Issuer               string
	SubjectNameID        string
	Attributes           map[string]string
	NotBefore            *time.Time
	NotOnOrAfter         *time.Time
	SubjectConfirmations []SAMLSubjectConfirmation
	AudienceRestrictions [][]string
}

type SAMLSubjectConfirmation struct {
	Method    string
	RequestID string
	Recipient string
}

func validateIDPSigningPublicKey(cert *x509.Certificate) error {
	switch publicKey := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if publicKey.N.BitLen() < 2048 {
			return fmt.Errorf("IdP RSA public key must be at least 2048 bits")
		}
	case *ecdsa.PublicKey:
		if publicKey.Curve.Params().BitSize < 256 {
			return fmt.Errorf("IdP ECDSA public key must be at least 256 bits")
		}
	default:
		return fmt.Errorf("unsupported IdP signing public key type")
	}
	return nil
}

type samlProviderState struct {
	name             string
	entityID         string
	idpEntityID      string
	idpMetadataXML   string
	ssoURL           string
	attributeMapping map[string]string
	certPEM          string
	idpSigningCerts  []*x509.Certificate
}

type samlRequestState struct {
	provider  string
	expiresAt time.Time
}

type SAMLService struct {
	baseURL   string
	dataDir   string
	authSvc   *Service
	logger    *slog.Logger
	now       func() time.Time
	mu        sync.RWMutex
	providers map[string]*samlProviderState
	requests  map[string]samlRequestState

	// Test-only seams.
	parseAssertionFn func(*http.Request) (*SAMLAssertion, error)
	oauthLoginFn     func(ctx context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error)
}

// Creates and initializes a new SAMLService with the provided base URL, data directory, and auth service reference. Ensures the data directory is writable, falling back to a temporary directory if necessary. Returns an error if required parameters are empty or directory creation fails.
func NewSAMLService(baseURL, dataDir string, authSvc *Service, logger *slog.Logger) (*SAMLService, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("data directory is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fallbackDir := filepath.Join(os.TempDir(), "ayb", "saml")
		if mkErr := os.MkdirAll(fallbackDir, 0o700); mkErr != nil {
			return nil, fmt.Errorf("creating SAML data directory: %w", err)
		}
		logger.Warn("SAML data directory not writable, falling back to temp directory", "path", dataDir, "fallback", fallbackDir, "error", err)
		dataDir = fallbackDir
	}

	return &SAMLService{
		baseURL:   strings.TrimRight(baseURL, "/"),
		dataDir:   dataDir,
		authSvc:   authSvc,
		logger:    logger,
		now:       time.Now,
		providers: make(map[string]*samlProviderState),
		requests:  make(map[string]samlRequestState),
	}, nil
}

// DefaultSAMLDataDir resolves the on-disk directory for generated SP certs/keys.
func DefaultSAMLDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".ayb/saml"
	}
	return filepath.Join(home, ".ayb", "saml")
}

// ValidateSAMLProviderName enforces a filesystem-safe provider identifier.
// Allowed characters: letters, digits, underscore, hyphen; max length 64.
func ValidateSAMLProviderName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("provider name is required")
	}
	if !samlProviderNameRE.MatchString(name) {
		return fmt.Errorf("invalid provider name")
	}
	return nil
}

// Adds or updates a SAML provider configuration. Validates the provider name, extracts the SSO URL from the metadata, ensures the SP certificate and key exist, and stores the provider state. Returns an error if any required fields are missing or invalid.
func (s *SAMLService) UpsertProvider(_ context.Context, p config.SAMLProvider) error {
	name := strings.TrimSpace(p.Name)
	if err := ValidateSAMLProviderName(name); err != nil {
		return err
	}
	entityID := strings.TrimSpace(p.EntityID)
	if entityID == "" {
		return fmt.Errorf("provider entity_id is required")
	}
	metadata := strings.TrimSpace(p.IDPMetadataXML)
	if metadata == "" {
		return fmt.Errorf("idp_metadata_xml is required")
	}
	ssoURL, idpEntityID, err := extractIDPMetadata(metadata)
	if err != nil {
		return fmt.Errorf("invalid idp metadata: %w", err)
	}
	idpSigningCerts, err := extractIDPSigningCertificates(metadata)
	if err != nil {
		return fmt.Errorf("invalid idp signing certificate material: %w", err)
	}
	certPEM, _, err := s.ensureSPCertKey(name, p.SPCertFile, p.SPKeyFile)
	if err != nil {
		return err
	}

	mapping := map[string]string{}
	for k, v := range p.AttributeMapping {
		mapping[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[name] = &samlProviderState{
		name:             name,
		entityID:         entityID,
		idpEntityID:      idpEntityID,
		idpMetadataXML:   metadata,
		ssoURL:           ssoURL,
		attributeMapping: mapping,
		certPEM:          certPEM,
		idpSigningCerts:  idpSigningCerts,
	}
	return nil
}

func (s *SAMLService) DeleteProvider(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.providers, strings.TrimSpace(name))
}

// Initiates a SAML login flow by creating an AuthnRequest. Generates a request ID, constructs the SAML request XML, base64-encodes it, and builds a redirect URL to the IdP's SSO endpoint. Returns the redirect URL, request ID, and an error if any step fails.
func (s *SAMLService) InitiateLogin(providerName, relayState string) (*url.URL, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.providers[strings.TrimSpace(providerName)]
	if !ok {
		return nil, "", errSAMLProviderNotFound
	}

	requestID, err := newSAMLRequestID()
	if err != nil {
		return nil, "", fmt.Errorf("generating SAML request ID: %w", err)
	}
	s.requests[requestID] = samlRequestState{
		provider:  state.name,
		expiresAt: s.now().Add(samlRequestTTL),
	}
	s.pruneExpiredLocked()

	acsURL := s.assertionConsumerServiceURL(state)
	requestXML := fmt.Sprintf(`<AuthnRequest ID="%s" Version="2.0" IssueInstant="%s" AssertionConsumerServiceURL="%s"><Issuer>%s</Issuer></AuthnRequest>`,
		requestID,
		s.now().UTC().Format(time.RFC3339),
		xmlEscape(acsURL),
		xmlEscape(state.entityID),
	)
	reqB64 := base64.StdEncoding.EncodeToString([]byte(requestXML))

	redirectURL, err := url.Parse(state.ssoURL)
	if err != nil {
		return nil, "", fmt.Errorf("parsing IdP SSO URL: %w", err)
	}
	q := redirectURL.Query()
	q.Set("SAMLRequest", reqB64)
	if strings.TrimSpace(relayState) != "" {
		q.Set("RelayState", relayState)
	}
	redirectURL.RawQuery = q.Encode()
	return redirectURL, requestID, nil
}

// Processes the SAML assertion callback from the IdP after user authentication. Validates the request, parses and validates the assertion, maps attributes to email and name, then delegates to the auth service to create or link a user account. Returns the authenticated user, access token, refresh token, and relay state, or an error if validation fails.
func (s *SAMLService) HandleCallback(r *http.Request, providerName, requestID string) (*User, string, string, string, error) {
	requestID = strings.TrimSpace(requestID)
	state, err := s.validateRequest(providerName, requestID)
	if err != nil {
		return nil, "", "", "", err
	}
	assertion, err := s.parseAssertion(r, state, requestID)
	if err != nil {
		return nil, "", "", "", err
	}
	if assertion == nil {
		return nil, "", "", "", fmt.Errorf("missing SAML assertion")
	}
	if err := s.validateAssertionBinding(assertion, state, requestID); err != nil {
		return nil, "", "", "", err
	}
	if err := validateSAMLAssertionTime(assertion, s.now()); err != nil {
		return nil, "", "", "", err
	}

	emailKey := "email"
	nameKey := "name"
	if v := strings.TrimSpace(state.attributeMapping["email"]); v != "" {
		emailKey = v
	}
	if v := strings.TrimSpace(state.attributeMapping["name"]); v != "" {
		nameKey = v
	}
	email := strings.TrimSpace(assertion.Attributes[emailKey])
	displayName := strings.TrimSpace(assertion.Attributes[nameKey])
	subject := strings.TrimSpace(assertion.SubjectNameID)
	if subject == "" {
		subject = email
	}
	if subject == "" {
		return nil, "", "", "", fmt.Errorf("assertion subject is required")
	}

	login := s.oauthLoginFn
	if login == nil {
		login = s.authSvc.OAuthLogin
	}
	if login == nil {
		return nil, "", "", "", fmt.Errorf("auth service is not configured")
	}
	user, accessToken, refreshToken, err := login(r.Context(), "saml:"+state.name, &OAuthUserInfo{
		ProviderUserID: subject,
		Email:          email,
		Name:           displayName,
	})
	if err != nil {
		return nil, "", "", "", err
	}
	return user, accessToken, refreshToken, r.FormValue("RelayState"), nil
}

func (s *SAMLService) validateAssertionBinding(assertion *SAMLAssertion, state *samlProviderState, requestID string) error {
	expectedRecipient := s.assertionConsumerServiceURL(state)
	// Request and audience binding are signed assertion data; validate them
	// before consuming subject or attributes to prevent assertion replay.
	if !hasMatchingSAMLSubjectConfirmation(assertion.SubjectConfirmations, requestID, expectedRecipient) {
		return fmt.Errorf("assertion subject confirmation does not match SAML request")
	}
	if !hasSAMLAudienceRestrictions(assertion.AudienceRestrictions, state.entityID) {
		return fmt.Errorf("assertion audience does not match service provider")
	}
	if strings.TrimSpace(assertion.Issuer) != state.idpEntityID {
		return fmt.Errorf("assertion issuer does not match identity provider")
	}
	return nil
}

func validateSAMLAssertionTime(assertion *SAMLAssertion, now time.Time) error {
	if assertion.NotBefore != nil && now.Before(assertion.NotBefore.Add(-30*time.Second)) {
		return fmt.Errorf("assertion is not yet valid")
	}
	if assertion.NotOnOrAfter != nil && !now.Before(*assertion.NotOnOrAfter) {
		return fmt.Errorf("assertion is expired")
	}
	return nil
}

func (s *SAMLService) assertionConsumerServiceURL(state *samlProviderState) string {
	return s.baseURL + "/api/auth/saml/" + url.PathEscape(state.name) + "/acs"
}

func hasMatchingSAMLSubjectConfirmation(confirmations []SAMLSubjectConfirmation, requestID, recipient string) bool {
	for _, confirmation := range confirmations {
		if strings.TrimSpace(confirmation.Method) != samlBearerConfirmationMethod {
			continue
		}
		if strings.TrimSpace(confirmation.RequestID) == requestID && strings.TrimSpace(confirmation.Recipient) == recipient {
			return true
		}
	}
	return false
}

func hasSAMLAudienceRestrictions(restrictions [][]string, entityID string) bool {
	if len(restrictions) == 0 {
		return false
	}
	for _, restriction := range restrictions {
		if !hasSAMLAudience(restriction, entityID) {
			return false
		}
	}
	return true
}

func hasSAMLAudience(audiences []string, entityID string) bool {
	for _, audience := range audiences {
		if strings.TrimSpace(audience) == entityID {
			return true
		}
	}
	return false
}

// Generates SAML Service Provider metadata XML. Returns the SP's entity descriptor including the signing certificate and assertion consumer service endpoint. Returns an error if the provider is not found or the certificate is invalid.
func (s *SAMLService) SPMetadataXML(providerName string) ([]byte, error) {
	s.mu.RLock()
	state, ok := s.providers[strings.TrimSpace(providerName)]
	s.mu.RUnlock()
	if !ok {
		return nil, errSAMLProviderNotFound
	}

	certBlock, _ := pem.Decode([]byte(state.certPEM))
	if certBlock == nil {
		return nil, fmt.Errorf("invalid SP certificate")
	}
	certB64 := base64.StdEncoding.EncodeToString(certBlock.Bytes)
	acsURL := s.assertionConsumerServiceURL(state)
	metadata := fmt.Sprintf(`<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol" WantAssertionsSigned="true">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>%s</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s" index="1"/>
  </SPSSODescriptor>
</EntityDescriptor>`, xmlEscape(state.entityID), certB64, xmlEscape(acsURL))
	return []byte(metadata), nil
}
