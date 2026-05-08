package ci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

const (
	// LogTailLines caps how many trailing lines of `gh run view --log-failed`
	// reach the ci-fix prompt (PRD US-3 step 3).
	LogTailLines = 200
)

// GhRunner is the gh-cli adapter the watcher uses. The github package's
// GhRunner satisfies it; the interface is repeated here so this package
// stays import-light and easy to fake in tests.
type GhRunner interface {
	RunGh(ctx context.Context, args ...string) (string, error)
}

// FixEnqueuer is satisfied by usecase.TaskUseCase.EnqueueFixTask. Defined
// as an interface so the watcher can be unit-tested without spinning up
// a TaskUseCase.
type FixEnqueuer interface {
	EnqueueFixTask(ctx context.Context, in FixEnqueueInput) (*domain.Task, error)
}

// FixEnqueueInput mirrors usecase.FixTaskInput on this side of the
// dependency boundary so internal/ci does not import internal/usecase.
type FixEnqueueInput struct {
	Parent         *domain.Task
	HeadSHA        string
	FailedStep     string
	PromptTemplate string
}

// SlackNotifier is the subset of Slack interactions the watcher needs.
// Methods may be no-ops in tests.
type SlackNotifier interface {
	NotifyCIPassed(ctx context.Context, task *domain.Task) error
	NotifyCIFixEnqueued(ctx context.Context, task *domain.Task, attempt, maxAttempts int) error
	NotifyCIExhausted(ctx context.Context, task *domain.Task, attempts int) error
	NotifyCIStuck(ctx context.Context, task *domain.Task) error
	NotifyCITimeout(ctx context.Context, task *domain.Task) error
}

// PromptRenderer renders the ci-fix template with the watcher-supplied
// data. Defined here so the template implementation can live outside the
// hot path.
type PromptRenderer interface {
	RenderCIFix(data PromptData) (string, error)
}

// PromptData is the binding for prompts/ci-fix.tmpl (PRD US-3).
type PromptData struct {
	PRNumber         int
	FailedStep       string
	LogTail          string
	PreviousAttempts int
	HeadSHA          string
}

// Clock is the interface every time-aware component in the project uses.
type Clock interface{ Now() time.Time }

// Config bundles the runtime knobs ci.Watcher needs.
type Config struct {
	Enabled             bool
	MaxAttempts         int
	PollInterval        time.Duration
	PollTimeout         time.Duration
	CommentOnExhaustion bool
}

// Watcher polls open PRs created by claude-ops tasks until their CI checks
// reach a terminal state, then either marks the task passed or enqueues a
// ci-fix child task. See PRD §6.1 for the decision flow.
type Watcher struct {
	cfg      Config
	repo     domain.TaskRepository
	events   domain.TaskEventRepository
	enqueuer FixEnqueuer
	gh       GhRunner
	slack    SlackNotifier
	prompts  PromptRenderer
	clock    Clock
}

// NewWatcher constructs a Watcher. Any nil dependency disables the
// corresponding code path (e.g. nil slack means no notifications), which
// is convenient for staged rollout and tests.
func NewWatcher(cfg Config, repo domain.TaskRepository, events domain.TaskEventRepository,
	enqueuer FixEnqueuer, gh GhRunner, slack SlackNotifier, prompts PromptRenderer, clock Clock) *Watcher {
	return &Watcher{
		cfg: cfg, repo: repo, events: events, enqueuer: enqueuer,
		gh: gh, slack: slack, prompts: prompts, clock: clock,
	}
}

// Start blocks until ctx is cancelled, ticking every cfg.PollInterval.
// Disabled or mis-configured watchers return immediately.
func (w *Watcher) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.cfg.PollInterval <= 0 || w.repo == nil {
		slog.Info("ci-watcher: disabled (cfg.enabled=false or missing deps)")
		return
	}
	slog.Info("ci-watcher: started",
		"poll_interval", w.cfg.PollInterval, "poll_timeout", w.cfg.PollTimeout, "max_attempts", w.cfg.MaxAttempts)

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("ci-watcher: stopping")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// tick walks every watching task once. Errors on a single task log and
// skip; the remaining tasks still get processed so one bad PR cannot
// stall the whole loop.
func (w *Watcher) tick(ctx context.Context) {
	ids, err := w.repo.ListWatchingIDs(ctx)
	if err != nil {
		slog.Error("ci-watcher: list watching", "err", err)
		return
	}
	for _, id := range ids {
		if err := w.processTask(ctx, id); err != nil {
			slog.Error("ci-watcher: process", "task_id", id, "err", err)
		}
	}
}

func (w *Watcher) processTask(ctx context.Context, id string) error {
	task, err := w.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("load task: %w", err)
	}
	if task.PRNumber == 0 {
		// Defensive: the worker hook should never set ci_status='watching'
		// without a PR number, but legacy rows might.
		_ = w.repo.UpdateCIStatus(ctx, id, domain.CIStatusEmpty, task.HeadSHA, w.clock.Now())
		return nil
	}

	// Poll-timeout guard.
	if !task.UpdatedAt.IsZero() && w.cfg.PollTimeout > 0 &&
		w.clock.Now().Sub(task.UpdatedAt) > w.cfg.PollTimeout {
		_ = w.repo.UpdateCIStatus(ctx, id, domain.CIStatusTimeout, task.HeadSHA, w.clock.Now())
		w.notify("timeout", task, 0)
		return nil
	}

	view, err := w.fetchPRView(ctx, task)
	if err != nil {
		return fmt.Errorf("gh pr view: %w", err)
	}
	if view.State != "" && view.State != "OPEN" {
		_ = w.repo.UpdateCIStatus(ctx, id, domain.CIStatusClosed, view.HeadRefOid, w.clock.Now())
		return nil
	}

	checks, err := w.fetchChecks(ctx, task)
	if err != nil {
		return fmt.Errorf("gh pr checks: %w", err)
	}
	w.recordEvent(ctx, id, domain.EventKindCICheckPolled, map[string]any{
		"checks":   len(checks),
		"head_sha": view.HeadRefOid,
	})

	status, failedRuns := AggregateConclusion(checks)
	switch status {
	case "":
		// still in_progress / queued — touch ci_last_polled_at and continue
		return w.repo.UpdateCIStatus(ctx, id, domain.CIStatusWatching, view.HeadRefOid, w.clock.Now())
	case "passed":
		_ = w.repo.UpdateCIStatus(ctx, id, domain.CIStatusPassed, view.HeadRefOid, w.clock.Now())
		w.notify("passed", task, 0)
		return nil
	case "stuck":
		_ = w.repo.UpdateCIStatus(ctx, id, domain.CIStatusStuck, view.HeadRefOid, w.clock.Now())
		w.notify("stuck", task, 0)
		return nil
	case "failed":
		return w.handleFailure(ctx, task, view.HeadRefOid, failedRuns)
	}
	return nil
}

func (w *Watcher) handleFailure(ctx context.Context, task *domain.Task, headSHA string, failed []CheckRun) error {
	w.recordEvent(ctx, task.ID, domain.EventKindCIFailureDetected, map[string]any{
		"failed_checks": len(failed),
		"head_sha":      headSHA,
	})

	// Dedup: if a fix task already exists for (parent, head_sha) we leave
	// the parent's ci_status as 'watching' — the fix task itself will run
	// through the worker and produce a new commit on this same branch.
	if existing, err := w.repo.FindFixChild(ctx, task.ID, headSHA); err == nil && existing != nil {
		return nil
	}

	if task.FixAttemptCount >= w.cfg.MaxAttempts {
		_ = w.repo.UpdateCIStatus(ctx, task.ID, domain.CIStatusExhausted, headSHA, w.clock.Now())
		w.recordEvent(ctx, task.ID, domain.EventKindCIFixExhausted, map[string]any{
			"attempts": task.FixAttemptCount,
		})
		w.notify("exhausted", task, task.FixAttemptCount)
		if w.cfg.CommentOnExhaustion && w.gh != nil && task.PRNumber > 0 {
			body := fmt.Sprintf("CI still failing after %d auto-fix attempts — needs human review.", task.FixAttemptCount)
			_, _ = w.gh.RunGh(ctx, "pr", "comment", strconv.Itoa(task.PRNumber),
				"--repo", task.RepoFullName, "--body", body)
		}
		return nil
	}

	// Pull failed-step + log tail for the prompt.
	failedStep, logTail := w.collectFailureContext(ctx, task, failed)
	prompt := ""
	if w.prompts != nil {
		rendered, perr := w.prompts.RenderCIFix(PromptData{
			PRNumber:         task.PRNumber,
			FailedStep:       failedStep,
			LogTail:          logTail,
			PreviousAttempts: task.FixAttemptCount,
			HeadSHA:          headSHA,
		})
		if perr != nil {
			return fmt.Errorf("render prompt: %w", perr)
		}
		prompt = rendered
	}

	child, err := w.enqueuer.EnqueueFixTask(ctx, FixEnqueueInput{
		Parent: task, HeadSHA: headSHA, FailedStep: failedStep, PromptTemplate: prompt,
	})
	if err != nil {
		return fmt.Errorf("enqueue fix task: %w", err)
	}

	w.recordEvent(ctx, task.ID, domain.EventKindCIFixEnqueued, map[string]any{
		"child_task_id": child.ID,
		"attempt":       child.FixAttemptCount,
		"max_attempts":  w.cfg.MaxAttempts,
	})
	// Parent's ci_status stays 'watching' so we can re-evaluate after the
	// child pushes — the new HEAD will land here on the next tick.
	_ = w.repo.UpdateCIStatus(ctx, task.ID, domain.CIStatusWatching, headSHA, w.clock.Now())
	if w.slack != nil {
		_ = w.slack.NotifyCIFixEnqueued(ctx, task, child.FixAttemptCount, w.cfg.MaxAttempts)
	}
	return nil
}

func (w *Watcher) collectFailureContext(ctx context.Context, task *domain.Task, failed []CheckRun) (string, string) {
	if len(failed) == 0 {
		return "", ""
	}
	first := failed[0]
	if w.gh == nil || first.WorkflowRun == 0 {
		return first.Name, ""
	}
	raw, err := w.gh.RunGh(ctx, "run", "view", strconv.FormatInt(first.WorkflowRun, 10),
		"--repo", task.RepoFullName, "--log-failed")
	if err != nil {
		slog.Warn("ci-watcher: log-failed fetch", "task_id", task.ID, "err", err)
		return first.Name, ""
	}
	return first.Name, MaskSecrets(TruncateLogTail(raw, LogTailLines))
}

type prView struct {
	HeadRefOid string `json:"headRefOid"`
	State      string `json:"state"`
}

func (w *Watcher) fetchPRView(ctx context.Context, task *domain.Task) (prView, error) {
	var v prView
	if w.gh == nil {
		return v, errors.New("gh runner not wired")
	}
	out, err := w.gh.RunGh(ctx, "pr", "view", strconv.Itoa(task.PRNumber),
		"--repo", task.RepoFullName, "--json", "headRefOid,state")
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal([]byte(out), &v)
}

func (w *Watcher) fetchChecks(ctx context.Context, task *domain.Task) ([]CheckRun, error) {
	if w.gh == nil {
		return nil, errors.New("gh runner not wired")
	}
	out, err := w.gh.RunGh(ctx, "pr", "checks", strconv.Itoa(task.PRNumber),
		"--repo", task.RepoFullName, "--json", "name,status,conclusion,detailsUrl,workflowRun")
	if err != nil {
		return nil, err
	}
	var checks []CheckRun
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		return nil, fmt.Errorf("decode checks: %w", err)
	}
	return checks, nil
}

func (w *Watcher) recordEvent(ctx context.Context, taskID string, kind domain.EventKind, payload any) {
	if w.events == nil {
		return
	}
	body, _ := json.Marshal(payload)
	_ = w.events.Create(ctx, &domain.TaskEvent{
		TaskID:      taskID,
		Kind:        kind,
		PayloadJSON: string(body),
		CreatedAt:   w.clock.Now(),
	})
}

func (w *Watcher) notify(kind string, task *domain.Task, attempts int) {
	if w.slack == nil {
		return
	}
	ctx := context.Background()
	switch kind {
	case "passed":
		_ = w.slack.NotifyCIPassed(ctx, task)
	case "stuck":
		_ = w.slack.NotifyCIStuck(ctx, task)
	case "timeout":
		_ = w.slack.NotifyCITimeout(ctx, task)
	case "exhausted":
		_ = w.slack.NotifyCIExhausted(ctx, task, attempts)
	}
}
