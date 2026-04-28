package github_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	igithub "github.com/gs97ahn/claude-ops/internal/github"
)

// makeGitHubSignature computes a valid sha256=<hex> signature for the given body and secret.
func makeGitHubSignature(t *testing.T, body []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookVerifier_Verify(t *testing.T) {
	const secret = "github-webhook-secret-1234"
	body := []byte(`{"action":"labeled","issue":{"number":42}}`)

	tests := []struct {
		name      string
		secret    string
		header    string
		body      []byte
		wantErr   error
		wantErrIs bool
	}{
		{
			name:   "valid signature",
			secret: secret,
			header: makeGitHubSignature(t, body, secret),
			body:   body,
		},
		{
			name:      "disabled: empty secret",
			secret:    "",
			header:    makeGitHubSignature(t, body, secret),
			body:      body,
			wantErr:   igithub.ErrWebhookDisabled,
			wantErrIs: true,
		},
		{
			name:      "missing header",
			secret:    secret,
			header:    "",
			body:      body,
			wantErr:   igithub.ErrMissingSignatureHeader,
			wantErrIs: true,
		},
		{
			name:      "prefix mismatch: uses v0= instead of sha256=",
			secret:    secret,
			header:    "v0=" + hex.EncodeToString([]byte("deadbeef")),
			body:      body,
			wantErr:   igithub.ErrInvalidWebhookSignature,
			wantErrIs: true,
		},
		{
			name:      "hex decode error: non-hex chars after sha256=",
			secret:    secret,
			header:    "sha256=ZZZNOTVALIDHEX",
			body:      body,
			wantErr:   igithub.ErrInvalidWebhookSignature,
			wantErrIs: true,
		},
		{
			name:      "wrong secret",
			secret:    secret,
			header:    makeGitHubSignature(t, body, "wrong-secret"),
			body:      body,
			wantErr:   igithub.ErrInvalidWebhookSignature,
			wantErrIs: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := igithub.NewWebhookVerifier(tc.secret)
			err := v.Verify(tc.body, tc.header)
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if !tc.wantErrIs {
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("expected errors.Is(err, %v), got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestWebhookVerifier_GitHubOfficialFixture tests with a known GitHub official-style fixture.
func TestWebhookVerifier_GitHubOfficialFixture(t *testing.T) {
	// Values derived from GitHub docs example pattern.
	secret := "It's a Secret to Everybody"
	body := []byte("Hello, World!")

	sig := makeGitHubSignature(t, body, secret)

	v := igithub.NewWebhookVerifier(secret)
	if err := v.Verify(body, sig); err != nil {
		t.Fatalf("official fixture verification failed: %v", err)
	}
}
