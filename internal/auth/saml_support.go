package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Parses a SAML assertion from an HTTP request. Extracts the SAMLResponse form parameter, base64-decodes it, and parses the XML. If a test seam is configured, uses that instead. Returns the parsed SAMLAssertion or an error if parsing fails.
func (s *SAMLService) parseAssertion(r *http.Request) (*SAMLAssertion, error) {
	if s.parseAssertionFn != nil {
		return s.parseAssertionFn(r)
	}
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parsing form: %w", err)
	}
	raw := strings.TrimSpace(r.FormValue("SAMLResponse"))
	if raw == "" {
		return nil, fmt.Errorf("missing SAMLResponse")
	}
	xmlBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding SAMLResponse: %w", err)
	}
	return decodeSAMLAssertion(xmlBytes)
}

// Decodes and parses a SAML assertion from XML bytes. Unmarshals the assertion structure, extracts the subject name ID, attributes, and validity conditions. Returns a SAMLAssertion struct or an error if parsing fails.
func decodeSAMLAssertion(xmlBytes []byte) (*SAMLAssertion, error) {
	type xmlAttributeValue struct {
		Value string `xml:",chardata"`
	}
	type xmlAttribute struct {
		Name   string              `xml:"Name,attr"`
		Values []xmlAttributeValue `xml:"AttributeValue"`
	}
	type xmlAttributeStatement struct {
		Attributes []xmlAttribute `xml:"Attribute"`
	}
	type xmlNameID struct {
		Value string `xml:",chardata"`
	}
	type xmlSubject struct {
		NameID xmlNameID `xml:"NameID"`
	}
	type xmlConditions struct {
		NotBefore    string `xml:"NotBefore,attr"`
		NotOnOrAfter string `xml:"NotOnOrAfter,attr"`
	}
	type xmlAssertion struct {
		Subject            xmlSubject              `xml:"Subject"`
		Conditions         xmlConditions           `xml:"Conditions"`
		AttributeStatement []xmlAttributeStatement `xml:"AttributeStatement"`
	}
	type xmlResponse struct {
		Assertions []xmlAssertion `xml:"Assertion"`
	}

	var resp xmlResponse
	if err := xml.Unmarshal(xmlBytes, &resp); err != nil {
		return nil, fmt.Errorf("parsing assertion XML: %w", err)
	}
	if len(resp.Assertions) == 0 {
		return nil, fmt.Errorf("no assertion in response")
	}
	a := resp.Assertions[0]
	out := &SAMLAssertion{
		SubjectNameID: strings.TrimSpace(a.Subject.NameID.Value),
		Attributes:    map[string]string{},
	}
	for _, stmt := range a.AttributeStatement {
		for _, attr := range stmt.Attributes {
			if len(attr.Values) == 0 {
				continue
			}
			out.Attributes[strings.TrimSpace(attr.Name)] = strings.TrimSpace(attr.Values[0].Value)
		}
	}
	if ts := strings.TrimSpace(a.Conditions.NotBefore); ts != "" {
		parsed, err := parseSAMLTime(ts)
		if err != nil {
			return nil, fmt.Errorf("invalid NotBefore: %w", err)
		}
		out.NotBefore = &parsed
	}
	if ts := strings.TrimSpace(a.Conditions.NotOnOrAfter); ts != "" {
		parsed, err := parseSAMLTime(ts)
		if err != nil {
			return nil, fmt.Errorf("invalid NotOnOrAfter: %w", err)
		}
		out.NotOnOrAfter = &parsed
	}
	return out, nil
}

func parseSAMLTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02T15:04:05Z", raw)
}

// Validates a SAML request by provider name and request ID. Checks that the provider exists, the request ID is valid and not expired, and the request is associated with the correct provider. Consumes the request upon successful validation. Returns the provider state or an error if validation fails.
func (s *SAMLService) validateRequest(providerName, requestID string) (*samlProviderState, error) {
	providerName = strings.TrimSpace(providerName)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("missing request ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.providers[providerName]
	if !ok {
		return nil, errSAMLProviderNotFound
	}
	reqState, ok := s.requests[requestID]
	if !ok {
		return nil, fmt.Errorf("invalid or expired SAML request")
	}
	delete(s.requests, requestID)
	if s.now().After(reqState.expiresAt) {
		return nil, fmt.Errorf("invalid or expired SAML request")
	}
	if reqState.provider != providerName {
		return nil, fmt.Errorf("provider mismatch for request")
	}
	return state, nil
}

func (s *SAMLService) pruneExpiredLocked() {
	now := s.now()
	for id, req := range s.requests {
		if now.After(req.expiresAt) {
			delete(s.requests, id)
		}
	}
}

func newSAMLRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Extracts the SingleSignOnService URL from IdP metadata XML. Parses the SAML metadata and returns the first available location attribute. Returns an error if the metadata is invalid or no SingleSignOnService is found.
func extractSSOURL(metadataXML string) (string, error) {
	type mdSSO struct {
		Binding  string `xml:"Binding,attr"`
		Location string `xml:"Location,attr"`
	}
	type mdIDPDescriptor struct {
		Services []mdSSO `xml:"SingleSignOnService"`
	}
	type mdEntity struct {
		IDP []mdIDPDescriptor `xml:"IDPSSODescriptor"`
	}
	var md mdEntity
	if err := xml.Unmarshal([]byte(metadataXML), &md); err != nil {
		return "", err
	}
	for _, desc := range md.IDP {
		for _, svc := range desc.Services {
			if strings.TrimSpace(svc.Location) != "" {
				return strings.TrimSpace(svc.Location), nil
			}
		}
	}
	return "", fmt.Errorf("no SingleSignOnService found")
}

// Ensures that SP certificate and private key files exist for a provider. Reads existing files if present, otherwise generates a new self-signed certificate and RSA key pair. Writes files to the specified paths (or defaults to the data directory). Returns the certificate PEM, key PEM, and any error encountered.
func (s *SAMLService) ensureSPCertKey(providerName, certPath, keyPath string) (string, string, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" {
		certPath = filepath.Join(s.dataDir, providerName+".crt")
	}
	if keyPath == "" {
		keyPath = filepath.Join(s.dataDir, providerName+".key")
	}
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		return string(certPEM), string(keyPEM), nil
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generating SP private key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", "", fmt.Errorf("generating cert serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "AYB SAML SP " + providerName,
		},
		NotBefore:             s.now().Add(-5 * time.Minute),
		NotAfter:              s.now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("creating SP certificate: %w", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return "", "", fmt.Errorf("creating certificate directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return "", "", fmt.Errorf("creating key directory: %w", err)
	}
	if err := os.WriteFile(certPath, certBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("writing SP certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, keyBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("writing SP private key: %w", err)
	}
	return string(certBytes), string(keyBytes), nil
}

func xmlEscape(v string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(v)); err != nil {
		return v
	}
	return b.String()
}
