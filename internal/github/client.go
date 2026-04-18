// Package github provides GitHub issue polling and PR creation.
package github

import (
	"context"
	"os"

	gh "github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

// NewClient creates an authenticated GitHub client using GITHUB_TOKEN.
func NewClient(token string) *gh.Client {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return gh.NewClient(tc)
}
