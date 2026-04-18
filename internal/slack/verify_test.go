package slack_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/slack"
)

const testSecret = "test_signing_secret_1234"

func makeValidSignature(t *testing.T, ts int64, body []byte, secret string) (string, string) {
	t.Helper()
	timestamp := strconv.FormatInt(ts, 10)
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return timestamp, sig
}

func TestVerifySignature_Valid(t *testing.T) {
	body := []byte(`payload=test_payload`)
	ts := time.Now().Unix()
	timestamp, sig := makeValidSignature(t, ts, body, testSecret)

	if err := slack.VerifySignature(timestamp, sig, body, testSecret); err != nil {
		t.Fatalf("expected valid signature, got: %v", err)
	}
}

func TestVerifySignature_Expired(t *testing.T) {
	body := []byte(`payload=old`)
	// 6 minutes ago — outside replay window
	ts := time.Now().Add(-6 * time.Minute).Unix()
	timestamp, sig := makeValidSignature(t, ts, body, testSecret)

	err := slack.VerifySignature(timestamp, sig, body, testSecret)
	if !errors.Is(err, slack.ErrExpiredTimestamp) {
		t.Fatalf("expected ErrExpiredTimestamp, got: %v", err)
	}
}

func TestVerifySignature_WrongSecret(t *testing.T) {
	body := []byte(`payload=test`)
	ts := time.Now().Unix()
	timestamp, sig := makeValidSignature(t, ts, body, "wrong_secret")

	err := slack.VerifySignature(timestamp, sig, body, testSecret)
	if !errors.Is(err, slack.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestVerifySignature_TamperedBody(t *testing.T) {
	body := []byte(`payload=original`)
	ts := time.Now().Unix()
	timestamp, sig := makeValidSignature(t, ts, body, testSecret)

	tampered := []byte(`payload=tampered`)
	err := slack.VerifySignature(timestamp, sig, tampered, testSecret)
	if !errors.Is(err, slack.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for tampered body, got: %v", err)
	}
}

func TestVerifySignature_InvalidTimestamp(t *testing.T) {
	err := slack.VerifySignature("not-a-number", "v0=sig", []byte("body"), testSecret)
	if err == nil {
		t.Fatal("expected error for invalid timestamp")
	}
}

func TestVerifySignature_FutureTimestamp(t *testing.T) {
	// 6 minutes in the future — also outside replay window
	body := []byte(`payload=future`)
	ts := time.Now().Add(6 * time.Minute).Unix()
	timestamp, sig := makeValidSignature(t, ts, body, testSecret)

	err := slack.VerifySignature(timestamp, sig, body, testSecret)
	if !errors.Is(err, slack.ErrExpiredTimestamp) {
		t.Fatalf("expected ErrExpiredTimestamp for future timestamp, got: %v", err)
	}
}
