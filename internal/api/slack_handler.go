package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/slack"
)

// TaskCancellerAPI is the interface used by the Slack handler to stop tasks.
type TaskCancellerAPI interface {
	CancelTask(ctx context.Context, taskID string) error
}

// SlackHandler handles Slack interactive component payloads.
type SlackHandler struct {
	signingSecret string
	canceller     TaskCancellerAPI
}

// NewSlackHandler creates a SlackHandler.
func NewSlackHandler(signingSecret string, canceller TaskCancellerAPI) *SlackHandler {
	return &SlackHandler{signingSecret: signingSecret, canceller: canceller}
}

// HandleInteractions godoc
//
//	@Summary      Handle Slack interactive component payload
//	@Tags         slack
//	@Accept       application/x-www-form-urlencoded
//	@Produce      json
//	@Success      200
//	@Failure      401  {object}  ErrorResponse
//	@Router       /slack/interactions [post]
func (h *SlackHandler) HandleInteractions(c *gin.Context) {
	// Read raw body for signature verification (must be before form parsing).
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to read body"})
		return
	}

	timestamp := c.GetHeader("X-Slack-Request-Timestamp")
	signature := c.GetHeader("X-Slack-Signature")

	if err = slack.VerifySignature(timestamp, signature, body, h.signingSecret); err != nil {
		slog.Warn("slack: signature verification failed", "err", err)
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid signature"})
		return
	}

	// Parse URL-encoded form.
	values, err := url.ParseQuery(string(body))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid payload"})
		return
	}
	payloadJSON := values.Get("payload")
	if payloadJSON == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "missing payload"})
		return
	}

	if err = slack.HandleInteraction(c.Request.Context(), payloadJSON, h.canceller); err != nil {
		slog.Error("slack: handle interaction error", "err", err)
		c.JSON(http.StatusOK, gin.H{}) // Slack expects 200 even on errors.
		return
	}

	c.JSON(http.StatusOK, gin.H{})
}
