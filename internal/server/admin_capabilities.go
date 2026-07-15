package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
)

const (
	adminCapabilityAuth              = "auth"
	adminCapabilityAuthAnonymous     = "auth_anonymous"
	adminCapabilityAuthEmailMFA      = "auth_email_mfa"
	adminCapabilityAuthMagicLink     = "auth_magic_link"
	adminCapabilityAuthOAuthProvider = "auth_oauth_provider"
	adminCapabilityAuthSMS           = "auth_sms"
	adminCapabilityAuthTOTP          = "auth_totp"
	adminCapabilityAuthWebAuthn      = "auth_webauthn"
	adminCapabilityBilling           = "billing"
	adminCapabilityEdgeFunctions     = "edge_functions"
	adminCapabilityJobs              = "jobs"
	adminCapabilityPush              = "push"
	adminCapabilityStatus            = "status"
	adminCapabilityStorage           = "storage"
	adminCapabilitySupport           = "support"
)

// handleAdminCapabilities returns server-owned capability flags for the admin console.
func (s *Server) handleAdminCapabilities(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, s.adminCapabilities())
}

func (s *Server) adminCapabilities() map[string]bool {
	capabilities := map[string]bool{
		adminCapabilityAuth:              false,
		adminCapabilityAuthAnonymous:     false,
		adminCapabilityAuthEmailMFA:      false,
		adminCapabilityAuthMagicLink:     false,
		adminCapabilityAuthOAuthProvider: false,
		adminCapabilityAuthSMS:           false,
		adminCapabilityAuthTOTP:          false,
		adminCapabilityAuthWebAuthn:      false,
		adminCapabilityBilling:           false,
		adminCapabilityEdgeFunctions:     s.edgeFuncSvc != nil,
		adminCapabilityJobs:              false,
		adminCapabilityPush:              false,
		adminCapabilityStatus:            false,
		adminCapabilityStorage:           false,
		adminCapabilitySupport:           false,
	}
	if s.cfg == nil {
		return capabilities
	}

	capabilities[adminCapabilityAuth] = s.cfg.Auth.Enabled
	capabilities[adminCapabilityAuthAnonymous] = s.cfg.Auth.AnonymousAuthEnabled
	capabilities[adminCapabilityAuthEmailMFA] = s.cfg.Auth.EmailMFAEnabled
	capabilities[adminCapabilityAuthMagicLink] = s.cfg.Auth.MagicLinkEnabled
	capabilities[adminCapabilityAuthOAuthProvider] = s.cfg.Auth.OAuthProviderMode.Enabled
	capabilities[adminCapabilityAuthSMS] = s.cfg.Auth.SMSEnabled
	capabilities[adminCapabilityAuthTOTP] = s.cfg.Auth.TOTPEnabled
	capabilities[adminCapabilityAuthWebAuthn] = s.cfg.Auth.WebAuthnEnabled
	capabilities[adminCapabilityBilling] = s.cfg.Billing.Provider == "stripe"
	capabilities[adminCapabilityJobs] = s.cfg.Jobs.Enabled
	capabilities[adminCapabilityPush] = s.cfg.Push.Enabled
	capabilities[adminCapabilityStatus] = s.cfg.Status.Enabled
	capabilities[adminCapabilityStorage] = s.cfg.Storage.Enabled
	capabilities[adminCapabilitySupport] = s.cfg.Support.Enabled
	return capabilities
}
