package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

type samlSignatureFixture struct {
	privateKey *rsa.PrivateKey
	cert       *x509.Certificate
	certDER    []byte
}

type samlSignedCallbackHarness struct {
	service    *SAMLService
	provider   string
	loginCalls int
	gotInfo    *OAuthUserInfo
}

type samlAssertionFixture struct {
	id                   string
	issuer               string
	subject              string
	email                string
	name                 string
	requestID            string
	recipient            string
	audiences            []string
	audienceRestrictions [][]string
}

func newSAMLSignatureFixture(t *testing.T) samlSignatureFixture {
	t.Helper()

	return newSAMLSignatureFixtureWithRSAKeyBits(t, 2048)
}

func newSAMLSignatureFixtureWithRSAKeyBits(t *testing.T, bits int) samlSignatureFixture {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	testutil.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	testutil.NoError(t, err)

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "AYB test SAML IdP",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	testutil.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	testutil.NoError(t, err)

	return samlSignatureFixture{
		privateKey: privateKey,
		cert:       cert,
		certDER:    certDER,
	}
}

func newSAMLSignedCallbackHarness(t *testing.T, idp samlSignatureFixture) *samlSignedCallbackHarness {
	t.Helper()

	return newSAMLSignedCallbackHarnessWithMetadata(t, samlSignatureIDPMetadataXML(idp.certDER))
}

func newSAMLSignedCallbackHarnessWithMetadata(t *testing.T, metadata string) *samlSignedCallbackHarness {
	t.Helper()

	h := &samlSignedCallbackHarness{
		service:  newTestSAMLService(t),
		provider: "signed_okta",
	}
	err := h.service.UpsertProvider(context.Background(), config.SAMLProvider{
		Enabled:        true,
		Name:           h.provider,
		EntityID:       "https://sp.example.com/" + h.provider,
		IDPMetadataXML: metadata,
		AttributeMapping: map[string]string{
			"email": "email",
			"name":  "name",
		},
	})
	testutil.NoError(t, err)

	h.service.oauthLoginFn = func(_ context.Context, _ string, info *OAuthUserInfo) (*User, string, string, error) {
		h.loginCalls++
		copied := *info
		h.gotInfo = &copied
		return &User{ID: "saml-signed-user", Email: info.Email}, "access-token", "refresh-token", nil
	}
	return h
}

func samlSignatureIDPMetadataXML(certDER []byte) string {
	return samlSignatureIDPMetadataXMLWithDescriptors(samlSignatureKeyDescriptor("signing", certDER))
}

func samlSignatureKeyDescriptor(use string, certDER []byte) string {
	useAttr := ""
	if use != "" {
		useAttr = ` use="` + use + `"`
	}
	cert := base64.StdEncoding.EncodeToString(certDER)
	if len(cert) > 96 {
		cert = cert[:64] + "\n    " + cert[64:96] + "\n    " + cert[96:]
	}
	return `<KeyDescriptor` + useAttr + `>
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>` + cert + `</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>`
}

func samlSignatureIDPMetadataXMLWithDescriptors(descriptors string) string {
	return `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    ` + descriptors + `
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.com/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`
}

func samlSignatureIDPMetadataWithoutSigningCert() string {
	return `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.com/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`
}

func (h *samlSignedCallbackHarness) postResponse(t *testing.T, responseXML []byte) (*User, error) {
	t.Helper()

	_, requestID, err := h.service.InitiateLogin(h.provider, "")
	testutil.NoError(t, err)

	form := url.Values{
		"SAMLResponse": {base64.StdEncoding.EncodeToString(responseXML)},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/saml/"+h.provider+"/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	user, _, _, _, err := h.service.HandleCallback(req, h.provider, requestID)
	return user, err
}

func (h *samlSignedCallbackHarness) postBuiltResponse(t *testing.T, build func(requestID string) []byte) (*User, error) {
	t.Helper()

	_, requestID, err := h.service.InitiateLogin(h.provider, "")
	testutil.NoError(t, err)

	responseXML := build(requestID)
	form := url.Values{
		"SAMLResponse": {base64.StdEncoding.EncodeToString(responseXML)},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/saml/"+h.provider+"/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	user, _, _, _, err := h.service.HandleCallback(req, h.provider, requestID)
	return user, err
}

func assertRejectedSAMLCallback(t *testing.T, h *samlSignedCallbackHarness, responseXML []byte) {
	t.Helper()

	user, err := h.postResponse(t, responseXML)
	testutil.Error(t, err)
	testutil.Nil(t, user)
	testutil.Equal(t, 0, h.loginCalls)
}

func assertAcceptedBuiltSAMLCallback(t *testing.T, h *samlSignedCallbackHarness, build func(requestID string) []byte, want samlAssertionFixture) {
	t.Helper()

	user, err := h.postBuiltResponse(t, build)
	testutil.NoError(t, err)
	testutil.NotNil(t, user)
	testutil.NotNil(t, h.gotInfo)
	testutil.Equal(t, want.subject, h.gotInfo.ProviderUserID)
	testutil.Equal(t, want.email, h.gotInfo.Email)
	testutil.Equal(t, want.name, h.gotInfo.Name)
}

func assertRejectedBuiltSAMLCallback(t *testing.T, h *samlSignedCallbackHarness, build func(requestID string) []byte) {
	t.Helper()

	user, err := h.postBuiltResponse(t, build)
	testutil.Error(t, err)
	testutil.Nil(t, user)
	testutil.Equal(t, 0, h.loginCalls)
}

func assertRejectedBuiltSAMLCallbackErrorContains(t *testing.T, h *samlSignedCallbackHarness, build func(requestID string) []byte, wantErr string) {
	t.Helper()

	user, err := h.postBuiltResponse(t, build)
	testutil.ErrorContains(t, err, wantErr)
	testutil.Nil(t, user)
	testutil.Equal(t, 0, h.loginCalls)
}

func signedSAMLAssertion(t *testing.T, idp samlSignatureFixture, assertion samlAssertionFixture) *etree.Element {
	t.Helper()

	return signedSAMLAssertionWithMethod(t, idp, assertion, dsig.RSASHA256SignatureMethod)
}

func signedSAMLAssertionWithMethod(t *testing.T, idp samlSignatureFixture, assertion samlAssertionFixture, method string) *etree.Element {
	t.Helper()

	ctx, err := dsig.NewSigningContext(idp.privateKey, [][]byte{idp.certDER})
	testutil.NoError(t, err)
	testutil.NoError(t, ctx.SetSignatureMethod(method))
	signed, err := ctx.SignEnveloped(samlAssertionElement(assertion))
	testutil.NoError(t, err)
	return signed
}

func signedSAMLAssertionWithoutKeyInfo(t *testing.T, idp samlSignatureFixture, assertion samlAssertionFixture) *etree.Element {
	t.Helper()

	signed := signedSAMLAssertion(t, idp, assertion)
	signature := signed.FindElement(".//Signature")
	testutil.NotNil(t, signature)
	for _, child := range signature.ChildElements() {
		if child.Tag == "KeyInfo" && child.NamespaceURI() == dsig.Namespace {
			signature.RemoveChild(child)
			return signed
		}
	}
	t.Fatalf("signed assertion did not contain ds:KeyInfo")
	return nil
}

func samlAssertionElement(assertion samlAssertionFixture) *etree.Element {
	el := etree.NewElement("saml2:Assertion")
	el.CreateAttr("xmlns:saml2", "urn:oasis:names:tc:SAML:2.0:assertion")
	el.CreateAttr("ID", assertion.id)
	el.CreateAttr("Version", "2.0")
	el.CreateAttr("IssueInstant", time.Now().UTC().Format(time.RFC3339))

	issuer := el.CreateElement("saml2:Issuer")
	issuerValue := assertion.issuer
	if issuerValue == "" {
		issuerValue = "https://idp.example.com/metadata"
	}
	issuer.SetText(issuerValue)
	subject := el.CreateElement("saml2:Subject")
	nameID := subject.CreateElement("saml2:NameID")
	nameID.SetText(assertion.subject)
	if assertion.requestID != "" || assertion.recipient != "" {
		confirmation := subject.CreateElement("saml2:SubjectConfirmation")
		confirmation.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
		data := confirmation.CreateElement("saml2:SubjectConfirmationData")
		if assertion.requestID != "" {
			data.CreateAttr("InResponseTo", assertion.requestID)
		}
		if assertion.recipient != "" {
			data.CreateAttr("Recipient", assertion.recipient)
		}
	}
	conditions := el.CreateElement("saml2:Conditions")
	conditions.CreateAttr("NotBefore", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
	conditions.CreateAttr("NotOnOrAfter", time.Now().UTC().Add(time.Hour).Format(time.RFC3339))
	restrictions := assertion.audienceRestrictions
	if len(restrictions) == 0 && len(assertion.audiences) > 0 {
		restrictions = [][]string{assertion.audiences}
	}
	for _, audiences := range restrictions {
		restriction := conditions.CreateElement("saml2:AudienceRestriction")
		for _, audience := range audiences {
			audienceEl := restriction.CreateElement("saml2:Audience")
			audienceEl.SetText(audience)
		}
	}
	statement := el.CreateElement("saml2:AttributeStatement")
	addSAMLAttribute(statement, "email", assertion.email)
	addSAMLAttribute(statement, "name", assertion.name)
	return el
}

func addSAMLAttribute(statement *etree.Element, name, value string) {
	attr := statement.CreateElement("saml2:Attribute")
	attr.CreateAttr("Name", name)
	attrValue := attr.CreateElement("saml2:AttributeValue")
	attrValue.SetText(value)
}

func samlResponseXML(t *testing.T, assertions ...*etree.Element) []byte {
	t.Helper()

	doc := etree.NewDocument()
	response := doc.CreateElement("saml2p:Response")
	response.CreateAttr("xmlns:saml2p", "urn:oasis:names:tc:SAML:2.0:protocol")
	response.CreateAttr("ID", "response-"+time.Now().UTC().Format("20060102150405.000000000"))
	response.CreateAttr("Version", "2.0")
	response.CreateAttr("IssueInstant", time.Now().UTC().Format(time.RFC3339))
	issuer := response.CreateElement("saml2:Issuer")
	issuer.CreateAttr("xmlns:saml2", "urn:oasis:names:tc:SAML:2.0:assertion")
	issuer.SetText("https://idp.example.com/metadata")
	status := response.CreateElement("saml2p:Status")
	statusCode := status.CreateElement("saml2p:StatusCode")
	statusCode.CreateAttr("Value", "urn:oasis:names:tc:SAML:2.0:status:Success")
	for _, assertion := range assertions {
		response.AddChild(assertion)
	}

	xmlBytes, err := doc.WriteToBytes()
	testutil.NoError(t, err)
	return xmlBytes
}

func tamperSignedSAMLAssertionText(t *testing.T, signedResponse []byte, oldText, newText string) []byte {
	t.Helper()

	tampered := strings.Replace(string(signedResponse), oldText, newText, 1)
	testutil.True(t, tampered != string(signedResponse), "expected signed XML text replacement to change the response")
	return []byte(tampered)
}

func corruptSAMLSignatureValue(t *testing.T, signedResponse []byte) []byte {
	t.Helper()

	doc := etree.NewDocument()
	testutil.NoError(t, doc.ReadFromBytes(signedResponse))
	signatureValue := doc.FindElement("//SignatureValue")
	testutil.NotNil(t, signatureValue)
	value := strings.TrimSpace(signatureValue.Text())
	testutil.True(t, len(value) > 8, "SignatureValue should be populated")
	replacement := "A"
	if strings.HasPrefix(value, replacement) {
		replacement = "B"
	}
	signatureValue.SetText(replacement + value[1:])
	xmlBytes, err := doc.WriteToBytes()
	testutil.NoError(t, err)
	return xmlBytes
}

func TestSAMLAssertionSignatureValidAccepted(t *testing.T) {
	t.Parallel()

	idp := newSAMLSignatureFixture(t)
	h := newSAMLSignedCallbackHarness(t, idp)
	signed := samlAssertionFixture{
		id:      "signed-control-assertion",
		subject: "signed-control-subject",
		email:   "signed-control@example.com",
		name:    "Signed Control",
	}

	assertAcceptedBuiltSAMLCallback(t, h, func(requestID string) []byte {
		assertion := boundSAMLAssertionForHarness(h, signed, requestID)
		return samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
	}, signed)
}

func TestSAMLAssertionSignatureRejectsSHA1Algorithms(t *testing.T) {
	t.Parallel()

	idp := newSAMLSignatureFixture(t)
	h := newSAMLSignedCallbackHarness(t, idp)
	signed := samlAssertionFixture{
		id:      "sha1-signed-assertion",
		subject: "sha1-signed-subject",
		email:   "sha1-signed@example.com",
		name:    "SHA-1 Signed",
	}

	assertRejectedBuiltSAMLCallbackErrorContains(t, h, func(requestID string) []byte {
		assertion := boundSAMLAssertionForHarness(h, signed, requestID)
		return samlResponseXML(t, signedSAMLAssertionWithMethod(t, idp, assertion, dsig.RSASHA1SignatureMethod))
	}, "no SAML assertion validated")
}

func TestSAMLAssertionSignatureWithoutKeyInfoUsesRotationCertificates(t *testing.T) {
	t.Parallel()

	firstIDP := newSAMLSignatureFixture(t)
	secondIDP := newSAMLSignatureFixture(t)
	metadata := samlSignatureIDPMetadataXMLWithDescriptors(
		samlSignatureKeyDescriptor("signing", firstIDP.certDER) +
			samlSignatureKeyDescriptor("signing", secondIDP.certDER),
	)

	tests := []struct {
		name string
		idp  samlSignatureFixture
	}{
		{name: "first metadata certificate", idp: firstIDP},
		{name: "second metadata certificate", idp: secondIDP},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newSAMLSignedCallbackHarnessWithMetadata(t, metadata)
			h.service.mu.RLock()
			certCount := len(h.service.providers[h.provider].idpSigningCerts)
			h.service.mu.RUnlock()
			testutil.Equal(t, 2, certCount)
			signed := samlAssertionFixture{
				id:      "no-keyinfo-" + strings.ReplaceAll(tt.name, " ", "-"),
				subject: "no-keyinfo-subject",
				email:   "no-keyinfo@example.com",
				name:    "No KeyInfo",
			}

			assertAcceptedBuiltSAMLCallback(t, h, func(requestID string) []byte {
				assertion := boundSAMLAssertionForHarness(h, signed, requestID)
				return samlResponseXML(t, signedSAMLAssertionWithoutKeyInfo(t, tt.idp, assertion))
			}, signed)
		})
	}
}

func TestSAMLAssertionSignatureRequiresRequestAndAudienceBinding(t *testing.T) {
	t.Parallel()

	idp := newSAMLSignatureFixture(t)
	h := newSAMLSignedCallbackHarness(t, idp)
	control := samlAssertionFixture{
		id:      "bound-control-assertion",
		subject: "bound-control-subject",
		email:   "bound-control@example.com",
		name:    "Bound Control",
	}

	assertAcceptedBuiltSAMLCallback(t, h, func(requestID string) []byte {
		assertion := boundSAMLAssertionForHarness(h, control, requestID)
		return samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
	}, control)
}

func TestSAMLAssertionSignatureRejectsRequestAndAudienceReplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*samlAssertionFixture)
	}{
		{
			name: "missing request id",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.requestID = ""
			},
		},
		{
			name: "wrong request id",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.requestID = "replayed-request-id"
			},
		},
		{
			name: "missing recipient",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.recipient = ""
			},
		},
		{
			name: "wrong recipient",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.recipient = "https://attacker.example.com/acs"
			},
		},
		{
			name: "missing audience",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.audiences = nil
			},
		},
		{
			name: "wrong audience",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.audiences = []string{"https://sp.example.com/other-provider"}
			},
		},
		{
			name: "split audience restrictions require every restriction to match",
			mutate: func(assertion *samlAssertionFixture) {
				assertion.audiences = nil
				assertion.audienceRestrictions = [][]string{
					{"https://sp.example.com/signed_okta"},
					{"https://sp.example.com/other-provider"},
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idp := newSAMLSignatureFixture(t)
			h := newSAMLSignedCallbackHarness(t, idp)
			replayed := samlAssertionFixture{
				id:      "replay-" + strings.ReplaceAll(tt.name, " ", "-"),
				subject: "replay-subject",
				email:   "replay@example.com",
				name:    "Replay Attempt",
			}

			assertRejectedBuiltSAMLCallback(t, h, func(requestID string) []byte {
				assertion := boundSAMLAssertionForHarness(h, replayed, requestID)
				tt.mutate(&assertion)
				return samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
			})
		})
	}
}

func TestSAMLAssertionSignatureRejectsWrongIssuer(t *testing.T) {
	t.Parallel()

	idp := newSAMLSignatureFixture(t)
	h := newSAMLSignedCallbackHarness(t, idp)
	wrongIssuer := samlAssertionFixture{
		id:      "wrong-issuer-assertion",
		issuer:  "https://attacker.example.com/metadata",
		subject: "wrong-issuer-subject",
		email:   "wrong-issuer@example.com",
		name:    "Wrong Issuer",
	}

	assertRejectedBuiltSAMLCallback(t, h, func(requestID string) []byte {
		assertion := boundSAMLAssertionForHarness(h, wrongIssuer, requestID)
		return samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
	})
}

func TestSAMLAssertionSignatureWrappingUsesSignedAssertion(t *testing.T) {
	t.Parallel()

	idp := newSAMLSignatureFixture(t)
	h := newSAMLSignedCallbackHarness(t, idp)
	unsignedB := samlAssertionFixture{
		id:      "unsigned-wrapper-b",
		subject: "unsigned-wrapper-subject",
		email:   "unsigned-wrapper@example.com",
		name:    "Unsigned Wrapper",
	}
	signedA := samlAssertionFixture{
		id:      "signed-wrapper-a",
		subject: "signed-wrapper-subject",
		email:   "signed-wrapper@example.com",
		name:    "Signed Wrapper",
	}

	assertAcceptedBuiltSAMLCallback(t, h, func(requestID string) []byte {
		assertion := boundSAMLAssertionForHarness(h, signedA, requestID)
		return samlResponseXML(t, samlAssertionElement(unsignedB), signedSAMLAssertion(t, idp, assertion))
	}, signedA)
}

func TestSAMLAssertionSignatureSelectsLaterBoundAssertion(t *testing.T) {
	t.Parallel()

	idp := newSAMLSignatureFixture(t)
	h := newSAMLSignedCallbackHarness(t, idp)
	unbound := samlAssertionFixture{
		id:      "earlier-valid-unbound",
		subject: "unbound-subject",
		email:   "unbound@example.com",
		name:    "Unbound Assertion",
	}
	bound := samlAssertionFixture{
		id:      "later-valid-bound",
		subject: "bound-subject",
		email:   "bound@example.com",
		name:    "Bound Assertion",
	}

	assertAcceptedBuiltSAMLCallback(t, h, func(requestID string) []byte {
		earlier := boundSAMLAssertionForHarness(h, unbound, requestID)
		earlier.requestID = "other-request-id"
		later := boundSAMLAssertionForHarness(h, bound, requestID)
		return samlResponseXML(t, signedSAMLAssertion(t, idp, earlier), signedSAMLAssertion(t, idp, later))
	}, bound)
}

func boundSAMLAssertionForHarness(h *samlSignedCallbackHarness, assertion samlAssertionFixture, requestID string) samlAssertionFixture {
	assertion.requestID = requestID
	assertion.recipient = h.service.baseURL + "/api/auth/saml/" + url.PathEscape(h.provider) + "/acs"
	assertion.audiences = []string{"https://sp.example.com/" + h.provider}
	return assertion
}

func TestSAMLAssertionSignatureRejectsMutatedOrWrongKey(t *testing.T) {
	t.Parallel()

	t.Run("rejects subject text replaced after signing", func(t *testing.T) {
		idp := newSAMLSignatureFixture(t)
		h := newSAMLSignedCallbackHarness(t, idp)
		signed := samlAssertionFixture{
			id:      "mutated-subject-assertion",
			subject: "original-signed-subject",
			email:   "mutated-subject@example.com",
			name:    "Mutated Subject",
		}
		assertRejectedBuiltSAMLCallbackErrorContains(t, h, func(requestID string) []byte {
			assertion := boundSAMLAssertionForHarness(h, signed, requestID)
			responseXML := samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
			return tamperSignedSAMLAssertionText(t, responseXML, signed.subject, "forged-subject")
		}, "no SAML assertion validated")
	})

	t.Run("rejects email text replaced after signing", func(t *testing.T) {
		idp := newSAMLSignatureFixture(t)
		h := newSAMLSignedCallbackHarness(t, idp)
		signed := samlAssertionFixture{
			id:      "mutated-email-assertion",
			subject: "mutated-email-subject",
			email:   "original-signed-email@example.com",
			name:    "Mutated Email",
		}
		assertRejectedBuiltSAMLCallbackErrorContains(t, h, func(requestID string) []byte {
			assertion := boundSAMLAssertionForHarness(h, signed, requestID)
			responseXML := samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
			return tamperSignedSAMLAssertionText(t, responseXML, signed.email, "forged-email@example.com")
		}, "no SAML assertion validated")
	})

	t.Run("rejects assertion signed by untrusted key", func(t *testing.T) {
		trustedIDP := newSAMLSignatureFixture(t)
		untrustedIDP := newSAMLSignatureFixture(t)
		h := newSAMLSignedCallbackHarness(t, trustedIDP)
		signed := samlAssertionFixture{
			id:      "wrong-key-assertion",
			subject: "wrong-key-subject",
			email:   "wrong-key@example.com",
			name:    "Wrong Key",
		}
		assertRejectedBuiltSAMLCallbackErrorContains(t, h, func(requestID string) []byte {
			assertion := boundSAMLAssertionForHarness(h, signed, requestID)
			return samlResponseXML(t, signedSAMLAssertion(t, untrustedIDP, assertion))
		}, "no SAML assertion validated")
	})
}

func TestSAMLAssertionSignatureRejectsUnsignedOrCorruptSignature(t *testing.T) {
	t.Parallel()

	t.Run("rejects unsigned assertion", func(t *testing.T) {
		idp := newSAMLSignatureFixture(t)
		h := newSAMLSignedCallbackHarness(t, idp)
		unsigned := samlAssertionFixture{
			id:      "unsigned-assertion",
			subject: "unsigned-subject",
			email:   "unsigned@example.com",
			name:    "Unsigned User",
		}
		assertRejectedBuiltSAMLCallbackErrorContains(t, h, func(requestID string) []byte {
			assertion := boundSAMLAssertionForHarness(h, unsigned, requestID)
			return samlResponseXML(t, samlAssertionElement(assertion))
		}, "no SAML assertion validated")
	})

	t.Run("rejects corrupted signature value", func(t *testing.T) {
		idp := newSAMLSignatureFixture(t)
		h := newSAMLSignedCallbackHarness(t, idp)
		signed := samlAssertionFixture{
			id:      "corrupt-signature-assertion",
			subject: "corrupt-signature-subject",
			email:   "corrupt-signature@example.com",
			name:    "Corrupt Signature",
		}
		assertRejectedBuiltSAMLCallbackErrorContains(t, h, func(requestID string) []byte {
			assertion := boundSAMLAssertionForHarness(h, signed, requestID)
			responseXML := samlResponseXML(t, signedSAMLAssertion(t, idp, assertion))
			return corruptSAMLSignatureValue(t, responseXML)
		}, "no SAML assertion validated")
	})
}

func TestSAMLProviderSignatureRequiresSigningCertificate(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	err := samlSvc.UpsertProvider(context.Background(), config.SAMLProvider{
		Enabled:        true,
		Name:           "certless",
		EntityID:       "https://sp.example.com/certless",
		IDPMetadataXML: samlSignatureIDPMetadataWithoutSigningCert(),
		AttributeMapping: map[string]string{
			"email": "email",
			"name":  "name",
		},
	})
	if err == nil {
		t.Errorf("expected provider registration to reject metadata without a signing certificate")
	}

	_, _, loginErr := samlSvc.InitiateLogin("certless", "")
	if !errors.Is(loginErr, errSAMLProviderNotFound) {
		t.Fatalf("InitiateLogin error = %v, want %v", loginErr, errSAMLProviderNotFound)
	}
}

func TestValidateIDPSigningPublicKey(t *testing.T) {
	t.Parallel()

	p224, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	testutil.NoError(t, err)
	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testutil.NoError(t, err)

	tests := []struct {
		name        string
		publicKey   any
		wantErrText string
	}{
		{name: "accepts P-256 ECDSA", publicKey: &p256.PublicKey},
		{name: "rejects P-224 ECDSA", publicKey: &p224.PublicKey, wantErrText: "ECDSA public key must be at least 256 bits"},
		{name: "rejects unsupported key type", publicKey: struct{}{}, wantErrText: "unsupported IdP signing public key type"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := validateIDPSigningPublicKey(&x509.Certificate{PublicKey: tt.publicKey})
			if tt.wantErrText == "" {
				testutil.NoError(t, err)
				return
			}
			testutil.ErrorContains(t, err, tt.wantErrText)
		})
	}
}

func TestSAMLProviderSignatureCertificateSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		metadata    func(t *testing.T) string
		wantCerts   int
		wantErrText string
	}{
		{
			name: "accepts signing certificate",
			metadata: func(t *testing.T) string {
				return samlSignatureIDPMetadataXML(newSAMLSignatureFixture(t).certDER)
			},
			wantCerts: 1,
		},
		{
			name: "accepts omitted use certificate",
			metadata: func(t *testing.T) string {
				return samlSignatureIDPMetadataXMLWithDescriptors(samlSignatureKeyDescriptor("", newSAMLSignatureFixture(t).certDER))
			},
			wantCerts: 1,
		},
		{
			name: "retains every signing certificate for rotation and ignores encryption",
			metadata: func(t *testing.T) string {
				first := newSAMLSignatureFixture(t)
				second := newSAMLSignatureFixture(t)
				encryption := newSAMLSignatureFixture(t)
				return samlSignatureIDPMetadataXMLWithDescriptors(
					samlSignatureKeyDescriptor("signing", first.certDER) +
						samlSignatureKeyDescriptor("", second.certDER) +
						samlSignatureKeyDescriptor("encryption", encryption.certDER),
				)
			},
			wantCerts: 2,
		},
		{
			name: "ignores encryption only metadata",
			metadata: func(t *testing.T) string {
				return samlSignatureIDPMetadataXMLWithDescriptors(samlSignatureKeyDescriptor("encryption", newSAMLSignatureFixture(t).certDER))
			},
			wantErrText: "idp signing certificate",
		},
		{
			name: "rejects malformed eligible certificate data",
			metadata: func(t *testing.T) string {
				return samlSignatureIDPMetadataXMLWithDescriptors(`<KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>not base64</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>`)
			},
			wantErrText: "idp signing certificate",
		},
		{
			name: "rejects metadata without usable signing certificate",
			metadata: func(t *testing.T) string {
				return samlSignatureIDPMetadataWithoutSigningCert()
			},
			wantErrText: "idp signing certificate",
		},
		{
			name: "rejects metadata without identity provider entity id",
			metadata: func(t *testing.T) string {
				metadata := samlSignatureIDPMetadataXML(newSAMLSignatureFixture(t).certDER)
				return strings.Replace(metadata, ` entityID="https://idp.example.com/metadata"`, "", 1)
			},
			wantErrText: "metadata entityID is required",
		},
		{
			name: "rejects undersized rsa signing key",
			metadata: func(t *testing.T) string {
				return samlSignatureIDPMetadataXML(newSAMLSignatureFixtureWithRSAKeyBits(t, 1024).certDER)
			},
			wantErrText: "RSA public key must be at least 2048 bits",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			samlSvc := newTestSAMLService(t)
			certPath := filepath.Join(t.TempDir(), "sp.crt")
			keyPath := filepath.Join(t.TempDir(), "sp.key")
			err := samlSvc.UpsertProvider(context.Background(), config.SAMLProvider{
				Enabled:        true,
				Name:           "selection",
				EntityID:       "https://sp.example.com/selection",
				IDPMetadataXML: tt.metadata(t),
				SPCertFile:     certPath,
				SPKeyFile:      keyPath,
			})

			if tt.wantErrText != "" {
				testutil.ErrorContains(t, err, tt.wantErrText)
				assertNoSAMLProviderOrSPFiles(t, samlSvc, "selection", certPath, keyPath)
				return
			}

			testutil.NoError(t, err)
			samlSvc.mu.RLock()
			state := samlSvc.providers["selection"]
			samlSvc.mu.RUnlock()
			testutil.NotNil(t, state)
			testutil.Equal(t, tt.wantCerts, len(state.idpSigningCerts))
		})
	}
}

func assertNoSAMLProviderOrSPFiles(t *testing.T, samlSvc *SAMLService, providerName, certPath, keyPath string) {
	t.Helper()

	samlSvc.mu.RLock()
	_, installed := samlSvc.providers[providerName]
	samlSvc.mu.RUnlock()
	testutil.False(t, installed, "provider should not be installed")

	_, certErr := os.Stat(certPath)
	testutil.True(t, errors.Is(certErr, os.ErrNotExist), "SP certificate file should not exist, got %v", certErr)
	_, keyErr := os.Stat(keyPath)
	testutil.True(t, errors.Is(keyErr, os.ErrNotExist), "SP key file should not exist, got %v", keyErr)
}
