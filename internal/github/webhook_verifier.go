// Package github provides GitHub webhook verification and delivery deduplication.
//
// Replay protection note:
// GitHub webhook does not provide an official timestamp header
// (unlike Slack X-Slack-Request-Timestamp). Replay window is
// implemented at the dedup layer via delivery-ID TTL (default 5min).
// See docs/specs/github-webhook.md §6.1 for the design decision. // ADDED
package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrWebhookDisabled is returned when the webhook secret is not configured.
var ErrWebhookDisabled = errors.New("github webhook disabled: secret not set")

// ErrInvalidWebhookSignature is returned when the HMAC-SHA256 signature does not match.
var ErrInvalidWebhookSignature = errors.New("invalid github webhook signature")

// ErrMissingSignatureHeader is returned when the X-Hub-Signature-256 header is absent.
var ErrMissingSignatureHeader = errors.New("missing X-Hub-Signature-256 header")

// WebhookVerifier verifies GitHub webhook HMAC-SHA256 signatures.
type WebhookVerifier struct {
	secret []byte
}

// NewWebhookVerifier creates a WebhookVerifier with the given secret.
// If secret is empty, all Verify calls will return ErrWebhookDisabled.
func NewWebhookVerifier(secret string) *WebhookVerifier {
	return &WebhookVerifier{secret: []byte(secret)}
}

// Verify checks that signatureHeader matches the HMAC-SHA256 of rawBody.
// signatureHeader must be in the form "sha256=<hex>".
// Returns ErrWebhookDisabled if secret is empty, ErrMissingSignatureHeader if header is empty,
// or ErrInvalidWebhookSignature if the signature does not match.
func (v *WebhookVerifier) Verify(rawBody []byte, signatureHeader string) error {
	if len(v.secret) == 0 {
		return ErrWebhookDisabled
	}
	if signatureHeader == "" {
		return ErrMissingSignatureHeader
	}

	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return fmt.Errorf("%w: expected sha256= prefix", ErrInvalidWebhookSignature)
	}

	hexPart := signatureHeader[len(prefix):]
	gotBytes, err := hex.DecodeString(hexPart)
	if err != nil {
		return fmt.Errorf("%w: hex decode error: %w", ErrInvalidWebhookSignature, err)
	}

	mac := hmac.New(sha256.New, v.secret)
	mac.Write(rawBody)
	expectedBytes := mac.Sum(nil)

	if !hmac.Equal(expectedBytes, gotBytes) {
		return ErrInvalidWebhookSignature
	}
	return nil
}
