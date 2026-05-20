package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"time"
)

// SetEncryptionKey sets the AES-256-GCM key used for encrypting TOTP secrets.
// Key must be exactly 32 bytes.
func (s *Service) SetEncryptionKey(key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	s.encryptionKey = make([]byte, 32)
	copy(s.encryptionKey, key)
	return nil
}

// encryptAESGCM encrypts plaintext using AES-256-GCM with the service encryption key.
func (s *Service) encryptAESGCM(plaintext []byte) ([]byte, error) {
	if len(s.encryptionKey) == 0 {
		return nil, ErrEncryptionKeyNotSet
	}
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decryptAESGCM decrypts ciphertext using AES-256-GCM with the service encryption key.
func (s *Service) decryptAESGCM(ciphertext []byte) ([]byte, error) {
	if len(s.encryptionKey) == 0 {
		return nil, ErrEncryptionKeyNotSet
	}
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}

// generateTOTPCode computes the TOTP value for a secret at a given time step.
// Implements RFC 6238 (TOTP) using HMAC-SHA1, 6 digits, 30-second period.
func generateTOTPCode(secret []byte, timeStep int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(timeStep))

	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 §5.4).
	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code = code % uint32(math.Pow10(totpDigits))

	return fmt.Sprintf("%0*d", totpDigits, code)
}

// validateTOTPCode checks if the provided code matches the secret within the
// allowed time skew. Returns the matched time step, or -1 if no match.
func validateTOTPCode(secret []byte, code string, now time.Time) (int64, bool) {
	currentStep := now.Unix() / totpPeriod
	for offset := -int64(totpSkew); offset <= int64(totpSkew); offset++ {
		step := currentStep + offset
		expected := generateTOTPCode(secret, step)
		if subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return step, true
		}
	}
	return -1, false
}

// buildOTPAuthURI builds a standard otpauth:// URI for authenticator app enrollment.
func buildOTPAuthURI(secret []byte, email, issuer string) string {
	label := url.PathEscape(issuer + ":" + email)
	params := url.Values{}
	params.Set("secret", base32NoPadding(secret))
	params.Set("issuer", issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", totpPeriod))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, params.Encode())
}

func base32NoPadding(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}
