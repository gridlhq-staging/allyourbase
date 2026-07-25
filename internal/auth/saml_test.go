package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

const samlIDPMetadataTemplate = `<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>%s</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="%s"/>
  </IDPSSODescriptor>
</EntityDescriptor>`

const testSAMLSigningCertB64 = "MIIDJzCCAg+gAwIBAgIUdF2pLOFwCZ0wgaHe9ZKu8MN6tZ8wDQYJKoZIhvcNAQELBQAwIzEhMB8GA1UEAwwYQVlCIHNoYXJlZCB0ZXN0IFNBTUwgSWRQMB4XDTI2MDcyNTAyNDUzNFoXDTM2MDcyMjAyNDUzNFowIzEhMB8GA1UEAwwYQVlCIHNoYXJlZCB0ZXN0IFNBTUwgSWRQMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqf+jAc+fabuM61lmIL+WgqWHYR7wCFNJULBJ/48qWN/rx3SSWpgXdByyP7ur20PMK+Sm8yiZvp2QGUCCv/3jzB1R/AH1S6R3PPfy9fabDTEuWrWz/HNXZ+GzY36UeB+TAcKZycJgptCvZKkEncuQcgu2d7G3uFE6NB6KDbASNhM9Fci54kQshdhUOe0D28p+Pjni4OBEgngf2BOln/xBUl5K1djcSUHP9QygNn95ESy4deynHwnejmO47MLktr7oIi1LGv+nNFSdIxIfZCuuDStgZEJEz1TL8tinOhxQ/o936vXQ/ied6hTuMECrz+cOI4T2G9RfiCrf0rFsb+EOzwIDAQABo1MwUTAdBgNVHQ4EFgQURkeI2vHjEjkTN54WT0EB5y7islkwHwYDVR0jBBgwFoAURkeI2vHjEjkTN54WT0EB5y7islkwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAUPxeVZTPA8yXPENGR+41YyWXzR2VzaXPU0C+rtrXw2T4La0E7JhGUl/iEIY3U4xyC9fwb/W7hh3MjeaSayisN62EYfCf4aPLq6YROwFJco2d5e8E7D83RFE8EApwLbYhl8xT/CUUmz5yRQaYcnwtNIiiwkeNTjl+WoFsEHHWFML3EWMdme2eVOd8J06SfA0iBbmoXgr5TZKWr73Juo4pPFGe3DVIjbXfKm01dz9rMXgllHgU5xgRg0RLk7M/aUUNWYnM2jUENCFhsxbpbVezIlJY/rASMifuY9ktnB4hfEjR1+G3AfaM0tBahhK5rK1Oz74GkfH9V1twim5acnSGjQ=="

func newTestSAMLService(t *testing.T) *SAMLService {
	t.Helper()
	authSvc := newTestService()
	samlSvc, err := NewSAMLService("http://localhost:8090", t.TempDir(), authSvc, testutil.DiscardLogger())
	testutil.NoError(t, err)
	return samlSvc
}

func registerSAMLProvider(t *testing.T, samlSvc *SAMLService, name string) {
	t.Helper()
	err := samlSvc.UpsertProvider(context.Background(), config.SAMLProvider{
		Enabled:        true,
		Name:           name,
		EntityID:       "https://sp.example.com/" + name,
		IDPMetadataXML: testSAMLIDPMetadataXML("https://idp.example.com/sso"),
		AttributeMapping: map[string]string{
			"email": "email",
			"name":  "name",
		},
	})
	testutil.NoError(t, err)
}

func testSAMLIDPMetadataXML(ssoURL string) string {
	return fmt.Sprintf(samlIDPMetadataTemplate, testSAMLSigningCertB64, ssoURL)
}

func TestSAMLServiceInitiateLoginRedirectsToIDP(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	redirectURL, requestID, err := samlSvc.InitiateLogin("okta", "https://app.example.com/post-login")
	testutil.NoError(t, err)
	testutil.True(t, requestID != "", "request ID should be populated")
	testutil.Equal(t, "idp.example.com", redirectURL.Host)
	testutil.Contains(t, redirectURL.String(), "RelayState=")
	testutil.Contains(t, redirectURL.String(), url.QueryEscape("https://app.example.com/post-login"))
}

func TestSAMLServiceHandleCallbackCallsOAuthLogin(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	var gotProvider string
	var gotInfo *OAuthUserInfo
	var reqID string
	samlSvc.oauthLoginFn = func(_ context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error) {
		gotProvider = provider
		gotInfo = info
		return &User{ID: "u_123", Email: "saml-user@example.com"}, "access-token", "refresh-token", nil
	}
	samlSvc.parseAssertionFn = func(_ *http.Request) (*SAMLAssertion, error) {
		return testBoundSAMLAssertion(samlSvc, "okta", reqID, &SAMLAssertion{
			SubjectNameID: "idp-user-1",
			Attributes: map[string]string{
				"email": "saml-user@example.com",
				"name":  "SAML User",
			},
		}), nil
	}

	_, reqID, err := samlSvc.InitiateLogin("okta", "https://app.example.com/home")
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/saml/okta/acs", strings.NewReader("RelayState=https%3A%2F%2Fapp.example.com%2Fhome"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	testutil.NoError(t, req.ParseForm())

	user, accessToken, refreshToken, relayState, err := samlSvc.HandleCallback(req, "okta", reqID)
	testutil.NoError(t, err)
	testutil.Equal(t, "u_123", user.ID)
	testutil.Equal(t, "access-token", accessToken)
	testutil.Equal(t, "refresh-token", refreshToken)
	testutil.Equal(t, "https://app.example.com/home", relayState)
	testutil.Equal(t, "saml:okta", gotProvider)
	testutil.NotNil(t, gotInfo)
	testutil.Equal(t, "idp-user-1", gotInfo.ProviderUserID)
	testutil.Equal(t, "saml-user@example.com", gotInfo.Email)
	testutil.Equal(t, "SAML User", gotInfo.Name)
}

func TestSAMLServiceHandleCallbackRejectsInvalidAssertion(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	_, reqID, err := samlSvc.InitiateLogin("okta", "")
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/saml/okta/acs", strings.NewReader("SAMLResponse=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	testutil.NoError(t, req.ParseForm())

	_, _, _, _, err = samlSvc.HandleCallback(req, "okta", reqID)
	testutil.NotNil(t, err)
}

func TestSAMLServiceSPMetadataContainsEntityDescriptor(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")

	b, err := samlSvc.SPMetadataXML("okta")
	testutil.NoError(t, err)
	testutil.Contains(t, string(b), "EntityDescriptor")
	testutil.Contains(t, string(b), "okta")
}

func TestSAMLAuthRoutesLoginMetadataAndACS(t *testing.T) {
	t.Parallel()

	authSvc := newTestService()
	h := NewHandler(authSvc, testutil.DiscardLogger())
	samlSvc := newTestSAMLService(t)
	registerSAMLProvider(t, samlSvc, "okta")
	h.SetSAMLService(samlSvc)

	samlSvc.oauthLoginFn = func(_ context.Context, _ string, _ *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "u_123", Email: "saml-route@example.com"}, "route-access", "route-refresh", nil
	}
	var requestID string
	samlSvc.parseAssertionFn = func(_ *http.Request) (*SAMLAssertion, error) {
		return testBoundSAMLAssertion(samlSvc, "okta", requestID, &SAMLAssertion{
			SubjectNameID: "sub-1",
			Attributes:    map[string]string{"email": "saml-route@example.com"},
		}), nil
	}

	routes := h.Routes()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/saml/okta/login?redirect_to=https://app.example.com/post-login", nil)
	req.Host = "localhost:8090"
	routes.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
	testutil.Contains(t, w.Header().Get("Location"), "RelayState=")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/saml/okta/metadata", nil)
	routes.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "EntityDescriptor")

	_, requestID, err := samlSvc.InitiateLogin("okta", "https://app.example.com/post-login")
	testutil.NoError(t, err)

	form := url.Values{}
	form.Set("RelayState", "https://app.example.com/post-login")
	form.Set("request_id", requestID)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/saml/okta/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	routes.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "route-access")
	testutil.Contains(t, w.Body.String(), "route-refresh")
}

func testBoundSAMLAssertion(samlSvc *SAMLService, providerName, requestID string, assertion *SAMLAssertion) *SAMLAssertion {
	state := &samlProviderState{
		name:        providerName,
		entityID:    "https://sp.example.com/" + providerName,
		idpEntityID: "https://idp.example.com/metadata",
	}
	assertion.Issuer = state.idpEntityID
	assertion.SubjectConfirmations = []SAMLSubjectConfirmation{{
		Method:    samlBearerConfirmationMethod,
		RequestID: requestID,
		Recipient: samlSvc.assertionConsumerServiceURL(state),
	}}
	assertion.AudienceRestrictions = [][]string{{state.entityID}}
	return assertion
}

func TestValidateSAMLProviderName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple", input: "okta", wantErr: false},
		{name: "with_hyphen", input: "azure-ad", wantErr: false},
		{name: "with_underscore", input: "google_oidc", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "path_traversal", input: "../escape", wantErr: true},
		{name: "slash", input: "tenant/idp", wantErr: true},
		{name: "dot", input: "okta.prod", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSAMLProviderName(tt.input)
			if tt.wantErr {
				testutil.NotNil(t, err)
				return
			}
			testutil.NoError(t, err)
		})
	}
}

func TestSAMLServiceUpsertProviderRejectsInvalidProviderName(t *testing.T) {
	t.Parallel()

	samlSvc := newTestSAMLService(t)
	err := samlSvc.UpsertProvider(context.Background(), config.SAMLProvider{
		Enabled:        true,
		Name:           "../escape",
		EntityID:       "https://sp.example.com/escape",
		IDPMetadataXML: testSAMLIDPMetadataXML("https://idp.example.com/sso"),
	})
	testutil.ErrorContains(t, err, "invalid provider name")
}
