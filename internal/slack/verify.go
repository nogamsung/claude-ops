// Package slack provides Slack notification and webhook handling.
package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	slackTimestampHeader = "X-Slack-Request-Timestamp"
	slackSignatureHeader = "X-Slack-Signature"
	signatureVersion     = "v0"
	replayWindow         = 5 * time.Minute
)

// ErrInvalidSignature is returned when the Slack request signature does not match.
var ErrInvalidSignature = errors.New("invalid slack signature")

// ErrExpiredTimestamp is returned when the request timestamp is outside the replay window.
var ErrExpiredTimestamp = errors.New("slack request timestamp expired")

// VerifySignature verifies a Slack request signature.
// timestamp is the X-Slack-Request-Timestamp header value.
// body is the raw request body bytes.
// signature is the X-Slack-Signature header value (v0=<hex>).
// secret is the Slack signing secret.
func VerifySignature(timestamp, signature string, body []byte, secret string) error {
	// Parse and validate timestamp.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("parse timestamp %q: %w", timestamp, err)
	}

	age := time.Since(time.Unix(ts, 0)).Abs()
	if age > replayWindow {
		return ErrExpiredTimestamp
	}

	// Compute expected signature.
	baseString := fmt.Sprintf("%s:%s:%s", signatureVersion, timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	expected := signatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrInvalidSignature
	}
	return nil
}
