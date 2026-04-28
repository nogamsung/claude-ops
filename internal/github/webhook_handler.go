package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/gs97ahn/scheduled-dev-agent/internal/api"
	"github.com/gs97ahn/scheduled-dev-agent/internal/config"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
	"github.com/gs97ahn/scheduled-dev-agent/internal/usecase"
)

const maxBodyBytes = 1 << 20 // 1 MB

// webhookTaskEnqueuer is the subset of TaskUseCase used by the webhook handler.
type webhookTaskEnqueuer interface {
	EnqueueFromIssue(ctx context.Context, req usecase.EnqueueRequest) (*domain.Task, error)
}

// webhookTaskChecker is the subset of TaskRepository used by the webhook handler.
type webhookTaskChecker interface {
	ExistsByRepoAndIssue(ctx context.Context, repoFullName string, issueNumber int) (bool, error)
}

// issueWebhookPayload is the subset of the GitHub IssueEvent JSON we care about.
type issueWebhookPayload struct {
	Action string `json:"action"`
	Issue  struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request,omitempty"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Label *struct {
		Name string `json:"name"`
	} `json:"label,omitempty"`
}

// WebhookHandler handles POST /github/webhook.
type WebhookHandler struct {
	verifier  *WebhookVerifier
	dedup     DedupCache
	taskUC    webhookTaskEnqueuer
	taskRepo  webhookTaskChecker
	githubCfg *config.GitHubConfig
}

// NewWebhookHandler creates a WebhookHandler.
func NewWebhookHandler(
	verifier *WebhookVerifier,
	dedup DedupCache,
	taskUC webhookTaskEnqueuer,
	taskRepo webhookTaskChecker,
	githubCfg *config.GitHubConfig,
) *WebhookHandler {
	return &WebhookHandler{
		verifier:  verifier,
		dedup:     dedup,
		taskUC:    taskUC,
		taskRepo:  taskRepo,
		githubCfg: githubCfg,
	}
}

// HandleWebhook ingests GitHub issue webhooks.
//
// @Summary  Receive GitHub webhook
// @Tags     github
// @Accept   json
// @Param    X-Hub-Signature-256  header  string  true  "HMAC-SHA256 signature (sha256=<hex>)"
// @Param    X-GitHub-Delivery    header  string  true  "Delivery UUID"
// @Param    X-GitHub-Event       header  string  true  "Event type (issues, ping)"
// @Success  200  {object}  api.WebhookResponse
// @Failure  400  {object}  api.ErrorResponse
// @Failure  401  {object}  api.ErrorResponse
// @Failure  500  {object}  api.ErrorResponse
// @Router   /github/webhook [post]
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	// Step 2: Read body with size cap.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.Warn("webhook: body read error", "err", err)
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "request body too large or unreadable"})
		return
	}

	// Step 3: Validate required headers.
	event := c.GetHeader("X-GitHub-Event")
	if event == "" {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing X-GitHub-Event header"})
		return
	}

	// Step 4: Delivery ID.
	delivery := c.GetHeader("X-GitHub-Delivery")
	if delivery == "" {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing X-GitHub-Delivery header"})
		return
	}

	// Step 5: Signature header.
	sig := c.GetHeader("X-Hub-Signature-256")
	if sig == "" {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "missing X-Hub-Signature-256 header"})
		return
	}

	// Step 6: Verify HMAC-SHA256.
	if verifyErr := h.verifier.Verify(body, sig); verifyErr != nil {
		if errors.Is(verifyErr, ErrWebhookDisabled) {
			// Webhook disabled but endpoint still called — return 503.
			c.JSON(http.StatusServiceUnavailable, api.ErrorResponse{Error: "github webhook not configured"})
			return
		}
		slog.Warn("webhook: signature verification failed", "delivery", delivery, "event", event)
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "signature verification failed"})
		return
	}

	// Step 7: Handle ping.
	if event == "ping" {
		slog.Info("webhook: ping received", "delivery", delivery)
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: true, Reason: "ping"})
		return
	}

	// Step 8: Only handle "issues" events.
	if event != "issues" {
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "ignored:event_not_supported"})
		return
	}

	// Step 9: Dedup by delivery ID.
	if !h.dedup.CheckAndAdd(delivery) {
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "duplicate"})
		return
	}

	// Step 10: Unmarshal payload.
	var payload issueWebhookPayload
	if jsonErr := json.Unmarshal(body, &payload); jsonErr != nil {
		slog.Error("webhook: json unmarshal failed", "delivery", delivery, "err", jsonErr)
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid JSON payload"})
		return
	}

	// Step 11: Skip PR events.
	if payload.Issue.PullRequest != nil {
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "ignored:pr_event"})
		return
	}

	// Step 12: Only handle opened / labeled actions.
	if payload.Action != "opened" && payload.Action != "labeled" {
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "ignored:action"})
		return
	}

	// Step 13: Allowlist check.
	repoFullName := payload.Repository.FullName
	repoCfg, found := h.findRepoCfg(repoFullName)
	if !found {
		slog.Info("webhook: repo not in allowlist", "delivery", delivery, "repo", repoFullName)
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "ignored:not_in_allowlist"})
		return
	}

	// Step 14: Label match.
	issueLabels := make([]string, 0, len(payload.Issue.Labels))
	for _, l := range payload.Issue.Labels {
		issueLabels = append(issueLabels, l.Name)
	}
	if !hasAllLabelNames(issueLabels, repoCfg.Labels) {
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "ignored:label_mismatch"})
		return
	}

	issueNumber := payload.Issue.Number

	// Step 15: Check for existing task (polling race).
	exists, err := h.taskRepo.ExistsByRepoAndIssue(ctx, repoFullName, issueNumber)
	if err != nil {
		slog.Error("webhook: ExistsByRepoAndIssue failed", "delivery", delivery, "repo", repoFullName, "issue", issueNumber, "err", err)
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "internal server error"})
		return
	}
	if exists {
		c.JSON(http.StatusOK, api.WebhookResponse{Accepted: false, Reason: "duplicate"})
		return
	}

	// Step 16: Enqueue task.
	taskID := uuid.New().String()
	task, enqErr := h.taskUC.EnqueueFromIssue(ctx, usecase.EnqueueRequest{
		RepoFullName: repoFullName,
		IssueNumber:  issueNumber,
		IssueTitle:   payload.Issue.Title,
		Source:       "webhook",
	})
	if enqErr != nil {
		// Step 17: INSERT failure → 500 so GitHub will retry.
		slog.Error("webhook: enqueue failed", "delivery", delivery, "repo", repoFullName, "issue", issueNumber, "err", enqErr)
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "internal server error"})
		return
	}
	if task != nil {
		taskID = task.ID
	}

	// Step 18: Success.
	latencyMS := time.Since(start).Milliseconds()
	slog.Info("webhook: accepted",
		"event", event,
		"action", payload.Action,
		"delivery_id", delivery,
		"repo", repoFullName,
		"issue", issueNumber,
		"accepted", true,
		"reason", "queued",
		"latency_ms", latencyMS,
	)

	c.JSON(http.StatusOK, api.WebhookResponse{
		Accepted: true,
		Reason:   "queued",
		TaskID:   taskID,
	})
}

// findRepoCfg searches the configured repos for a matching full name.
func (h *WebhookHandler) findRepoCfg(fullName string) (config.RepoConfig, bool) {
	for _, r := range h.githubCfg.Repos {
		if r.Name == fullName {
			return r, true
		}
	}
	return config.RepoConfig{}, false
}

// hasAllLabelNames reports whether labelNames contains all of required.
func hasAllLabelNames(labelNames, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(labelNames))
	for _, l := range labelNames {
		set[l] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}
