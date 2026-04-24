package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// Client sends Slack messages via the chat.postMessage API.
type Client struct {
	token     string
	channelID string
	http      *http.Client
}

// NewClient creates a Slack Client.
// token and channelID must not be logged.
func NewClient(token, channelID string) *Client {
	return &Client{
		token:     token,
		channelID: channelID,
		http:      &http.Client{},
	}
}

// NotifyStarted sends a "Task started" message.
func (c *Client) NotifyStarted(ctx context.Context, task *domain.Task) error {
	issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", task.RepoFullName, task.IssueNumber)
	msg := BuildStarted(task.ID, task.RepoFullName, task.IssueNumber, issueURL)
	return c.postMessage(ctx, msg)
}

// NotifyDone sends a "Task done" message.
func (c *Client) NotifyDone(ctx context.Context, task *domain.Task) error {
	msg := BuildDone(task.RepoFullName, task.IssueNumber, task.PRURL, task.PRNumber)
	return c.postMessage(ctx, msg)
}

// NotifyFailed sends a "Task failed" message.
func (c *Client) NotifyFailed(ctx context.Context, task *domain.Task, errMsg string) error {
	msg := BuildFailed(task.RepoFullName, task.IssueNumber, errMsg)
	return c.postMessage(ctx, msg)
}

// NotifyCancelled sends a "Task cancelled" message.
func (c *Client) NotifyCancelled(ctx context.Context, task *domain.Task) error {
	msg := BuildCancelled(task.RepoFullName, task.IssueNumber)
	return c.postMessage(ctx, msg)
}

// NotifyOrphaned sends a "Task orphaned (manual review required)" message so
// operators notice unfinished work whose worktree has uncommitted changes.
// Reuses the failed-message template with an "orphaned" prefix — keeping
// Slack surface small while still distinguishing the state.
func (c *Client) NotifyOrphaned(ctx context.Context, task *domain.Task) error {
	detail := fmt.Sprintf("orphaned — worktree %q has unsaved changes; manual review required", task.WorktreePath)
	msg := BuildFailed(task.RepoFullName, task.IssueNumber, detail)
	return c.postMessage(ctx, msg)
}

// NotifyModeChange sends a full-mode change notification.
func (c *Client) NotifyModeChange(ctx context.Context, enabled bool) error {
	msg := BuildModeChange(enabled)
	return c.postMessage(ctx, msg)
}

func (c *Client) postMessage(ctx context.Context, msg Message) error {
	type postBody struct {
		Channel string  `json:"channel"`
		Blocks  []Block `json:"blocks"`
	}
	body := postBody{Channel: c.channelID, Blocks: msg.Blocks}

	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	// Token must never be logged.
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("post to slack: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("slack client: close response body", "err", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned %d", resp.StatusCode)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode slack response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("slack API error: %s", result.Error)
	}
	return nil
}
