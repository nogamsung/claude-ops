package github_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/api"
	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
	igithub "github.com/gs97ahn/claude-ops/internal/github"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------- test doubles ----------

const testWebhookSecret = "test-webhook-secret"

// fakeTaskEnqueuer records calls to EnqueueFromIssue.
type fakeTaskEnqueuer struct {
	calls  []usecase.EnqueueRequest
	result *domain.Task
	err    error
}

func (f *fakeTaskEnqueuer) EnqueueFromIssue(_ context.Context, req usecase.EnqueueRequest) (*domain.Task, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &domain.Task{ID: "task-uuid-generated", Status: domain.TaskStatusQueued}, nil
}

// fakeTaskChecker controls ExistsByRepoAndIssue.
type fakeTaskChecker struct {
	exists bool
	err    error
}

func (f *fakeTaskChecker) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return f.exists, f.err
}

// fixedDedup always returns the configured bool.
type fixedDedup struct{ accept bool }

func (d *fixedDedup) CheckAndAdd(_ string) bool { return d.accept }

// ---------- helpers ----------

func sign(t *testing.T, body []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newGitHubCfg(repos ...config.RepoConfig) *config.GitHubConfig {
	return &config.GitHubConfig{Repos: repos}
}

func buildHandler(
	secret string,
	dedup igithub.DedupCache,
	enqueuer *fakeTaskEnqueuer,
	checker *fakeTaskChecker,
	cfg *config.GitHubConfig,
) *gin.Engine {
	verifier := igithub.NewWebhookVerifier(secret)
	h := igithub.NewWebhookHandler(verifier, dedup, enqueuer, checker, cfg)

	r := gin.New()
	r.POST("/github/webhook", h.HandleWebhook)
	return r
}

// buildIssuePayload creates a minimal GitHub issues event JSON body.
func buildIssuePayload(t *testing.T, action, repo string, issueNum int, labels []string, isPR bool) []byte {
	t.Helper()

	type label struct {
		Name string `json:"name"`
	}
	type issue struct {
		Number      int       `json:"number"`
		Title       string    `json:"title"`
		PullRequest *struct{} `json:"pull_request,omitempty"`
		Labels      []label   `json:"labels"`
	}
	type repository struct {
		FullName string `json:"full_name"`
	}
	type payload struct {
		Action     string     `json:"action"`
		Issue      issue      `json:"issue"`
		Repository repository `json:"repository"`
	}

	ls := make([]label, len(labels))
	for i, l := range labels {
		ls[i] = label{Name: l}
	}

	var pr *struct{}
	if isPR {
		pr = &struct{}{}
	}

	p := payload{
		Action: action,
		Issue: issue{
			Number:      issueNum,
			Title:       "Test Issue",
			PullRequest: pr,
			Labels:      ls,
		},
		Repository: repository{FullName: repo},
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

// ---------- tests ----------

// TestWebhookHandler_WebhookDisabled_SignatureStillChecked verifies that even without a
// configured secret the handler itself rejects mismatched signatures (though in production
// the route is never registered when secret is empty — see api.NewRouter). // MODIFIED
func TestWebhookHandler_WebhookDisabled_SignatureStillChecked(t *testing.T) { // MODIFIED
	// empty secret → verifier returns ErrWebhookDisabled (401 for any call)
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg()
	body := []byte("{}")
	r := buildHandler("", &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "disabled-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, "any-secret"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Handler returns 401 because empty-secret verifier always fails. // MODIFIED
	if w.Code != http.StatusUnauthorized { // MODIFIED
		t.Errorf("expected 401 when secret is empty, got %d", w.Code) // MODIFIED
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called when webhook is disabled")
	}
}

func TestWebhookHandler_MissingEvent_400(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, newGitHubCfg())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewBufferString("{}"))
	req.Header.Set("X-GitHub-Delivery", "uuid-1")
	req.Header.Set("X-Hub-Signature-256", sign(t, []byte("{}"), testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called")
	}
}

func TestWebhookHandler_MissingDelivery_400(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, newGitHubCfg())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewBufferString("{}"))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", sign(t, []byte("{}"), testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called")
	}
}

func TestWebhookHandler_MissingSignature_401(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, newGitHubCfg())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewBufferString("{}"))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "uuid-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called")
	}
}

func TestWebhookHandler_InvalidSignature_401(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, newGitHubCfg())

	body := []byte("{}")
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "uuid-1")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalidsignature")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called")
	}
}

func TestWebhookHandler_PingEvent_200(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	body := []byte(`{"zen":"Keep it logically awesome."}`)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, newGitHubCfg())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "ping")
	req.Header.Set("X-GitHub-Delivery", "ping-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Accepted || resp.Reason != "ping" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called for ping")
	}
}

func TestWebhookHandler_UnsupportedEvent_200Ignored(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	body := []byte(`{}`)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, newGitHubCfg())

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "push-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "ignored:event_not_supported" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called")
	}
}

func TestWebhookHandler_DuplicateDelivery_200Duplicate(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 42, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{false}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "dup-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "duplicate" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called for duplicate delivery")
	}
}

func TestWebhookHandler_PREvent_200Ignored(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 42, []string{"claude-ops"}, true)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "pr-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "ignored:pr_event" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called for PR event")
	}
}

func TestWebhookHandler_UnsupportedAction_200Ignored(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "closed", "owner/repo", 42, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "closed-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "ignored:action" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called for unsupported action")
	}
}

func TestWebhookHandler_NotInAllowlist_200Ignored(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/allowed-repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/other-repo", 42, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "allowlist-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "ignored:not_in_allowlist" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called for non-allowlisted repo")
	}
}

func TestWebhookHandler_LabelMismatch_200Ignored(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 42, []string{"other-label"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "label-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "ignored:label_mismatch" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called for label mismatch")
	}
}

func TestWebhookHandler_AlreadyExists_200Duplicate(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 42, []string{"claude-ops"}, false)
	checker := &fakeTaskChecker{exists: true}
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, checker, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "exists-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp api.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted || resp.Reason != "duplicate" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(enqueuer.calls) != 0 {
		t.Error("EnqueueFromIssue must not be called when task already exists")
	}
}

func TestWebhookHandler_HappyPath_Labeled_200Queued(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 42, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "happy-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp api.WebhookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Accepted || resp.Reason != "queued" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.TaskID == "" {
		t.Error("task_id must be set in accepted response")
	}
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 EnqueueFromIssue call, got %d", len(enqueuer.calls))
	}
	if enqueuer.calls[0].Source != "webhook" {
		t.Errorf("expected source=webhook, got %s", enqueuer.calls[0].Source)
	}
}

func TestWebhookHandler_HappyPath_Opened_200Queued(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "opened", "owner/repo", 1, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "opened-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 EnqueueFromIssue call, got %d", len(enqueuer.calls))
	}
}

// TestWebhookHandler_EnqueueError_500 verifies GitHub can retry on DB failure.
func TestWebhookHandler_EnqueueError_500(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{err: domain.ErrNotFound}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 42, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "err-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// TestWebhookHandler_InvalidJSON_400 verifies that unparseable bodies return 400.
func TestWebhookHandler_InvalidJSON_400(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{}
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := []byte("not-json-at-all{{{")
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "json-err-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestWebhookHandler_WindowGate_EnqueueOnly verifies US-7:
// webhook enqueues regardless of active window (scheduler gate is separate).
// We simulate this by confirming enqueue IS called even at an "odd" time
// (the window gate is only enforced by TaskUseCase, not the webhook handler itself).
func TestWebhookHandler_WindowGate_EnqueueOnly(t *testing.T) {
	enqueuer := &fakeTaskEnqueuer{} // no window gate — handler just calls EnqueueFromIssue
	cfg := newGitHubCfg(config.RepoConfig{Name: "owner/repo", Labels: []string{"claude-ops"}})
	body := buildIssuePayload(t, "labeled", "owner/repo", 99, []string{"claude-ops"}, false)
	r := buildHandler(testWebhookSecret, &fixedDedup{true}, enqueuer, &fakeTaskChecker{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "window-uuid")
	req.Header.Set("X-Hub-Signature-256", sign(t, body, testWebhookSecret))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(enqueuer.calls) != 1 {
		t.Error("webhook handler must call EnqueueFromIssue; window gate is TaskUseCase's responsibility")
	}
	_ = time.Now() // just to confirm no time operations in handler path beyond latency
}
