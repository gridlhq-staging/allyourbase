//go:build pushcontract

// APNS contract gap: missing `AYB_PUSH_CONTRACT_APNS_KEY_FILE` (intended source `.secret/apns_sandbox_auth_key.p8`), `AYB_PUSH_CONTRACT_APNS_KEY_ID`, `AYB_PUSH_CONTRACT_APNS_TEAM_ID`, `AYB_PUSH_CONTRACT_APNS_BUNDLE_ID`; smallest unblock = create an APNS auth key in the Apple Developer portal and drop the `.p8` + three identifiers into `.secret/` and the staging/prod GitHub Actions secrets.
package push

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

const apnsContractGap = "APNS contract gap: missing `AYB_PUSH_CONTRACT_APNS_KEY_FILE` (intended source `.secret/apns_sandbox_auth_key.p8`), `AYB_PUSH_CONTRACT_APNS_KEY_ID`, `AYB_PUSH_CONTRACT_APNS_TEAM_ID`, `AYB_PUSH_CONTRACT_APNS_BUNDLE_ID`; smallest unblock = create an APNS auth key in the Apple Developer portal and drop the `.p8` + three identifiers into `.secret/` and the staging/prod GitHub Actions secrets."

func TestFCMSendContract(t *testing.T) {
	credentialsFile := os.Getenv("AYB_PUSH_CONTRACT_FCM_CREDENTIALS")
	if credentialsFile == "" {
		t.Skip("FCM contract requires AYB_PUSH_CONTRACT_FCM_CREDENTIALS; refusing to substitute a mock provider")
	}

	provider, err := NewFCMProvider(credentialsFile, "")
	if err != nil {
		t.Fatalf("construct live FCM provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, err = provider.Send(ctx, "push-contract-bogus-token", &Message{Title: "contract", Body: "probe"})
	if err == nil {
		t.Fatal("live FCM accepted the deliberately bogus token; want Google's token rejection")
	}

	if strings.Contains(err.Error(), "PERMISSION_DENIED") &&
		strings.Contains(err.Error(), "Firebase Cloud Messaging API") {
		if !errors.Is(err, ErrProviderError) {
			t.Fatalf("disabled FCM API response = %v, want ErrProviderError (not ErrProviderAuth)", err)
		}
		accessToken, tokenErr := provider.getAccessToken(ctx)
		if tokenErr != nil {
			t.Fatalf("OAuth sub-contract failed after disabled FCM API response: %v", tokenErr)
		}
		if accessToken == "" {
			t.Fatal("OAuth sub-contract returned an empty access token")
		}
		t.Log("FCM API-disabled branch: OAuth succeeded; enable Cloud Messaging API v1 on project coffee-spark-ai-barista-6534c, or provision a dedicated push-test Firebase project")
		return
	}

	if errors.Is(err, ErrProviderAuth) {
		t.Fatalf("live FCM OAuth/provider authentication failed: %v", err)
	}
	if !errors.Is(err, ErrInvalidToken) && !errors.Is(err, ErrUnregistered) {
		t.Fatalf("Google rejected the bogus FCM token with %v; want ErrInvalidToken or ErrUnregistered", err)
	}
	t.Logf("FCM token-rejected branch: Google rejected the bogus token with %v", err)
}

func TestAPNSSendContract(t *testing.T) {
	config := APNSConfig{
		KeyFile:     os.Getenv("AYB_PUSH_CONTRACT_APNS_KEY_FILE"),
		KeyID:       os.Getenv("AYB_PUSH_CONTRACT_APNS_KEY_ID"),
		TeamID:      os.Getenv("AYB_PUSH_CONTRACT_APNS_TEAM_ID"),
		BundleID:    os.Getenv("AYB_PUSH_CONTRACT_APNS_BUNDLE_ID"),
		Environment: "sandbox",
	}
	if config.KeyFile == "" || config.KeyID == "" || config.TeamID == "" || config.BundleID == "" {
		t.Skip(apnsContractGap)
	}

	provider, err := NewAPNSProvider(config)
	if err != nil {
		t.Fatalf("construct live APNS provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, err = provider.Send(ctx, strings.Repeat("0", 64), &Message{Title: "contract", Body: "probe"})
	if errors.Is(err, ErrProviderAuth) {
		t.Fatalf("live APNS provider authentication failed: %v", err)
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("APNS sandbox response = %v, want ErrInvalidToken for the unregistered token", err)
	}
	t.Logf("APNS token-rejected branch: Apple rejected the unregistered token with %v", err)
}
