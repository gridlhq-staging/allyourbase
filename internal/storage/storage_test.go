package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateBucket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		bucket  string
		wantErr string
	}{
		{"valid simple", "images", ""},
		{"valid with hyphens", "my-bucket", ""},
		{"valid with underscores", "my_bucket", ""},
		{"valid with digits", "bucket123", ""},
		{"empty", "", "bucket name is required"},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "too long"}, // 66 chars > 63 max
		{"uppercase", "Images", "lowercase letters"},
		{"spaces", "my bucket", "lowercase letters"},
		{"dots", "my.bucket", "lowercase letters"},
		{"slashes", "my/bucket", "lowercase letters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateBucket(tt.bucket)
			if tt.wantErr != "" {
				testutil.ErrorContains(t, err, tt.wantErr)
			} else {
				testutil.NoError(t, err)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		objName string
		wantErr string
	}{
		{"valid simple", "photo.jpg", ""},
		{"valid nested", "a/b/c/file.txt", ""},
		{"empty", "", "object name is required"},
		{"dot dot", "a/../b", "must not contain"},
		{"leading slash", "/a/b", "must not start with"},
		{"too long", string(make([]byte, 1025)), "too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateName(tt.objName)
			if tt.wantErr != "" {
				testutil.ErrorContains(t, err, tt.wantErr)
			} else {
				testutil.NoError(t, err)
			}
		})
	}
}

func TestSignAndValidateURL(t *testing.T) {
	t.Parallel()
	svc := &Service{signKey: []byte("test-secret-key-for-signing-urls")}

	token := svc.SignURL(context.Background(), "images", "photo.jpg", time.Hour)
	testutil.True(t, token != "", "token should not be empty")

	values := parseSignedURLQuery(t, token)
	exp := values.Get("exp")
	sig := values.Get("sig")

	testutil.True(t, exp != "", "exp should be present")
	testutil.True(t, sig != "", "sig should be present")

	// Valid.
	validation := svc.ValidateSignedURL("images", "photo.jpg", values)
	testutil.True(t, validation.Valid, "should be valid")
	testutil.Equal(t, "", validation.TenantID)

	// Wrong bucket.
	testutil.False(t, svc.ValidateSignedURL("wrong", "photo.jpg", values).Valid, "wrong bucket should fail")

	// Wrong name.
	testutil.False(t, svc.ValidateSignedURL("images", "wrong.jpg", values).Valid, "wrong name should fail")

	// Wrong sig.
	wrongSig := cloneSignedURLValues(values)
	wrongSig.Set("sig", "badsig")
	testutil.False(t, svc.ValidateSignedURL("images", "photo.jpg", wrongSig).Valid, "wrong sig should fail")

	// Invalid exp.
	invalidExp := cloneSignedURLValues(values)
	invalidExp.Set("exp", "notanumber")
	testutil.False(t, svc.ValidateSignedURL("images", "photo.jpg", invalidExp).Valid, "invalid exp should fail")

	tenantCtx := tenant.ContextWithTenantID(context.Background(), "tenant-a")
	tenantToken := svc.SignURL(tenantCtx, "images", "photo.jpg", time.Hour)
	tenantValues := parseSignedURLQuery(t, tenantToken)
	testutil.Equal(t, "tenant-a", tenantValues.Get("tenant"))

	tenantValidation := svc.ValidateSignedURL("images", "photo.jpg", tenantValues)
	testutil.True(t, tenantValidation.Valid, "tenant token should be valid")
	testutil.Equal(t, "tenant-a", tenantValidation.TenantID)

	mismatchedTenant := cloneSignedURLValues(tenantValues)
	mismatchedTenant.Set("tenant", "tenant-b")
	testutil.False(t, svc.ValidateSignedURL("images", "photo.jpg", mismatchedTenant).Valid, "mismatched tenant should fail")

	missingTenant := cloneSignedURLValues(tenantValues)
	missingTenant.Del("tenant")
	testutil.False(t, svc.ValidateSignedURL("images", "photo.jpg", missingTenant).Valid, "missing tenant field should fail")
}

func TestSignURLExpired(t *testing.T) {
	t.Parallel()
	svc := &Service{signKey: []byte("test-secret-key-for-signing-urls")}

	// Generate a token that expires immediately.
	token := svc.SignURL(context.Background(), "b", "f", -time.Second)
	values := parseSignedURLQuery(t, token)
	testutil.Equal(t, 2, len(values))
	testutil.True(t, values.Get("exp") != "", "exp should be present")
	testutil.True(t, values.Get("sig") != "", "sig should be present")
	testutil.Equal(t, "", values.Get("tenant"))
	testutil.Equal(t, legacySignedURLSignature([]byte("test-secret-key-for-signing-urls"), "b/f:"+values.Get("exp")), values.Get("sig"))
	testutil.False(t, svc.ValidateSignedURL("b", "f", values).Valid, "expired token should fail")
}

func parseSignedURLQuery(t *testing.T, token string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(token)
	testutil.NoError(t, err)
	return values
}

func cloneSignedURLValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, vals := range values {
		cloned[key] = append([]string(nil), vals...)
	}
	return cloned
}

func legacySignedURLSignature(signKey []byte, payload string) string {
	mac := hmac.New(sha256.New, signKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
