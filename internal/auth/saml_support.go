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

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	samlBearerConfirmationMethod = "urn:oasis:names:tc:SAML:2.0:cm:bearer"
	samlSHA256DigestMethod       = "http://www.w3.org/2001/04/xmlenc#sha256"
	samlSHA384DigestMethod       = "http://www.w3.org/2001/04/xmldsig-more#sha384"
	samlSHA512DigestMethod       = "http://www.w3.org/2001/04/xmlenc#sha512"
)

// Parses a SAML assertion from an HTTP request. Extracts the SAMLResponse form parameter, base64-decodes it, and parses the XML. If a test seam is configured, uses that instead. Returns the parsed SAMLAssertion or an error if parsing fails.
func (s *SAMLService) parseAssertion(r *http.Request, state *samlProviderState, requestID string) (*SAMLAssertion, error) {
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
	return decodeVerifiedSAMLAssertion(xmlBytes, samlAssertionSelection{
		roots:     state.idpSigningCerts,
		requestID: requestID,
		recipient: s.assertionConsumerServiceURL(state),
		entityID:  state.entityID,
		issuer:    state.idpEntityID,
		now:       s.now(),
	})
}

type samlAssertionSelection struct {
	roots     []*x509.Certificate
	requestID string
	recipient string
	entityID  string
	issuer    string
	now       time.Time
}

func decodeSAMLAssertionElement(assertion *etree.Element) (*SAMLAssertion, error) {
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
	type xmlSubjectConfirmationData struct {
		RequestID string `xml:"InResponseTo,attr"`
		Recipient string `xml:"Recipient,attr"`
	}
	type xmlSubjectConfirmation struct {
		Method string                     `xml:"Method,attr"`
		Data   xmlSubjectConfirmationData `xml:"SubjectConfirmationData"`
	}
	type xmlSubject struct {
		NameID        xmlNameID                `xml:"NameID"`
		Confirmations []xmlSubjectConfirmation `xml:"SubjectConfirmation"`
	}
	type xmlAudience struct {
		Value string `xml:",chardata"`
	}
	type xmlAudienceRestriction struct {
		Audiences []xmlAudience `xml:"Audience"`
	}
	type xmlConditions struct {
		NotBefore            string                   `xml:"NotBefore,attr"`
		NotOnOrAfter         string                   `xml:"NotOnOrAfter,attr"`
		AudienceRestrictions []xmlAudienceRestriction `xml:"AudienceRestriction"`
	}
	type xmlAssertion struct {
		Issuer             xmlNameID               `xml:"Issuer"`
		Subject            xmlSubject              `xml:"Subject"`
		Conditions         xmlConditions           `xml:"Conditions"`
		AttributeStatement []xmlAttributeStatement `xml:"AttributeStatement"`
	}
	doc := etree.NewDocument()
	doc.SetRoot(assertion.Copy())
	xmlBytes, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serializing assertion XML: %w", err)
	}
	var a xmlAssertion
	if err := xml.Unmarshal(xmlBytes, &a); err != nil {
		return nil, fmt.Errorf("parsing assertion XML: %w", err)
	}
	out := &SAMLAssertion{
		Issuer:        strings.TrimSpace(a.Issuer.Value),
		SubjectNameID: strings.TrimSpace(a.Subject.NameID.Value),
		Attributes:    map[string]string{},
	}
	for _, confirmation := range a.Subject.Confirmations {
		out.SubjectConfirmations = append(out.SubjectConfirmations, SAMLSubjectConfirmation{
			Method:    strings.TrimSpace(confirmation.Method),
			RequestID: strings.TrimSpace(confirmation.Data.RequestID),
			Recipient: strings.TrimSpace(confirmation.Data.Recipient),
		})
	}
	for _, restriction := range a.Conditions.AudienceRestrictions {
		group := make([]string, 0, len(restriction.Audiences))
		for _, audience := range restriction.Audiences {
			group = append(group, strings.TrimSpace(audience.Value))
		}
		out.AudienceRestrictions = append(out.AudienceRestrictions, group)
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

func decodeVerifiedSAMLAssertion(responseXML []byte, selection samlAssertionSelection) (*SAMLAssertion, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(responseXML); err != nil {
		return nil, fmt.Errorf("parsing SAMLResponse XML: %w", err)
	}
	response := doc.Root()
	if response == nil || !isSAMLResponseElement(response) {
		return nil, fmt.Errorf("SAMLResponse root must be a SAML Response")
	}
	if len(selection.roots) == 0 {
		return nil, fmt.Errorf("missing IdP signing certificates")
	}
	var sawVerifiedAssertion bool
	for _, assertion := range response.ChildElements() {
		if !isSAMLAssertionElement(assertion) {
			continue
		}
		// Assertion signatures are always required; identity is decoded only
		// from the signature-stripped element returned by the validator.
		validated, err := validateSAMLAssertionSignature(assertion, selection.roots)
		if err != nil {
			continue
		}
		sawVerifiedAssertion = true
		decoded, err := decodeSAMLAssertionElement(validated)
		if err != nil {
			continue
		}
		if err := validateSAMLAssertionSelection(decoded, selection); err != nil {
			continue
		}
		return decoded, nil
	}
	if sawVerifiedAssertion {
		return nil, fmt.Errorf("no SAML assertion satisfied request binding")
	}
	return nil, fmt.Errorf("no SAML assertion validated")
}

func validateSAMLAssertionSignature(assertion *etree.Element, roots []*x509.Certificate) (*etree.Element, error) {
	if err := validateSAMLSignatureAlgorithms(assertion); err != nil {
		return nil, err
	}
	var lastErr error
	for _, root := range roots {
		store := &dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{root}}
		validated, err := dsig.NewDefaultValidationContext(store).Validate(assertion)
		if err == nil {
			return validated, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("missing IdP signing certificates")
}

func validateSAMLSignatureAlgorithms(assertion *etree.Element) error {
	for _, el := range assertion.FindElements(".//*") {
		if el.NamespaceURI() != dsig.Namespace {
			continue
		}
		algorithm := strings.TrimSpace(el.SelectAttrValue("Algorithm", ""))
		switch el.Tag {
		case dsig.SignatureMethodTag:
			if !isAllowedSAMLSignatureMethod(algorithm) {
				return fmt.Errorf("disallowed SAML signature algorithm")
			}
		case dsig.DigestMethodTag:
			if !isAllowedSAMLDigestMethod(algorithm) {
				return fmt.Errorf("disallowed SAML digest algorithm")
			}
		}
	}
	return nil
}

func isAllowedSAMLSignatureMethod(method string) bool {
	switch method {
	case dsig.RSASHA256SignatureMethod, dsig.RSASHA384SignatureMethod, dsig.RSASHA512SignatureMethod,
		dsig.ECDSASHA256SignatureMethod, dsig.ECDSASHA384SignatureMethod, dsig.ECDSASHA512SignatureMethod:
		return true
	default:
		return false
	}
}

func isAllowedSAMLDigestMethod(method string) bool {
	switch method {
	case samlSHA256DigestMethod, samlSHA384DigestMethod, samlSHA512DigestMethod:
		return true
	default:
		return false
	}
}

func validateSAMLAssertionSelection(assertion *SAMLAssertion, selection samlAssertionSelection) error {
	if !hasMatchingSAMLSubjectConfirmation(assertion.SubjectConfirmations, selection.requestID, selection.recipient) {
		return fmt.Errorf("assertion subject confirmation does not match SAML request")
	}
	if !hasSAMLAudienceRestrictions(assertion.AudienceRestrictions, selection.entityID) {
		return fmt.Errorf("assertion audience does not match service provider")
	}
	if strings.TrimSpace(assertion.Issuer) != selection.issuer {
		return fmt.Errorf("assertion issuer does not match identity provider")
	}
	return validateSAMLAssertionTime(assertion, selection.now)
}

func isSAMLResponseElement(el *etree.Element) bool {
	return el.Tag == "Response" && el.NamespaceURI() == "urn:oasis:names:tc:SAML:2.0:protocol"
}

func isSAMLAssertionElement(el *etree.Element) bool {
	return el.Tag == "Assertion" && el.NamespaceURI() == "urn:oasis:names:tc:SAML:2.0:assertion"
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

// Extracts the IdP entity ID and SingleSignOnService URL from metadata XML.
func extractIDPMetadata(metadataXML string) (string, string, error) {
	type mdSSO struct {
		Binding  string `xml:"Binding,attr"`
		Location string `xml:"Location,attr"`
	}
	type mdIDPDescriptor struct {
		Services []mdSSO `xml:"SingleSignOnService"`
	}
	type mdEntity struct {
		EntityID string            `xml:"entityID,attr"`
		IDP      []mdIDPDescriptor `xml:"IDPSSODescriptor"`
	}
	var md mdEntity
	if err := xml.Unmarshal([]byte(metadataXML), &md); err != nil {
		return "", "", err
	}
	entityID := strings.TrimSpace(md.EntityID)
	if entityID == "" {
		return "", "", fmt.Errorf("metadata entityID is required")
	}
	for _, desc := range md.IDP {
		for _, svc := range desc.Services {
			if strings.TrimSpace(svc.Location) != "" {
				return strings.TrimSpace(svc.Location), entityID, nil
			}
		}
	}
	return "", "", fmt.Errorf("no SingleSignOnService found")
}

func extractIDPSigningCertificates(metadataXML string) ([]*x509.Certificate, error) {
	type mdCertificate struct {
		Value string `xml:",chardata"`
	}
	type mdX509Data struct {
		Certificates []mdCertificate `xml:"X509Certificate"`
	}
	type mdKeyInfo struct {
		X509Data []mdX509Data `xml:"X509Data"`
	}
	type mdKeyDescriptor struct {
		Use      string      `xml:"use,attr"`
		KeyInfos []mdKeyInfo `xml:"KeyInfo"`
	}
	type mdIDPDescriptor struct {
		KeyDescriptors []mdKeyDescriptor `xml:"KeyDescriptor"`
	}
	type mdEntity struct {
		IDP []mdIDPDescriptor `xml:"IDPSSODescriptor"`
	}
	var md mdEntity
	if err := xml.Unmarshal([]byte(metadataXML), &md); err != nil {
		return nil, err
	}
	var certs []*x509.Certificate
	for _, desc := range md.IDP {
		for _, key := range desc.KeyDescriptors {
			use := strings.TrimSpace(key.Use)
			if use != "" && use != "signing" {
				continue
			}
			for _, keyInfo := range key.KeyInfos {
				for _, x509Data := range keyInfo.X509Data {
					for _, certData := range x509Data.Certificates {
						cert, err := parseIDPSigningCertificate(certData.Value)
						if err != nil {
							return nil, err
						}
						certs = append(certs, cert)
					}
				}
			}
		}
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no usable idp signing certificate found")
	}
	return certs, nil
}

func parseIDPSigningCertificate(raw string) (*x509.Certificate, error) {
	// IdP metadata is the only trust source; parse DER now so invalid
	// registration fails before any SP certificate/key files are created.
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(raw), ""))
	if err != nil {
		return nil, fmt.Errorf("decoding idp signing certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parsing idp signing certificate: %w", err)
	}
	if err := validateIDPSigningPublicKey(cert); err != nil {
		return nil, err
	}
	return cert, nil
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
