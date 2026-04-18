package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gs97ahn/scheduled-dev-agent/internal/claude"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// SlackNotifier sends task lifecycle notifications to Slack.
type SlackNotifier interface {
	NotifyStarted(ctx context.Context, task *domain.Task) error
	NotifyDone(ctx context.Context, task *domain.Task) error
	NotifyFailed(ctx context.Context, task *domain.Task, errMsg string) error
	NotifyCancelled(ctx context.Context, task *domain.Task) error
}

// PRCreator creates pull requests for completed tasks.
type PRCreator interface {
	CreatePR(ctx context.Context, task *domain.Task) (string, int, error)
}

// WorkerConfig holds dependencies for the Worker.
type WorkerConfig struct {
	TaskRepo     domain.TaskRepository
	EventRepo    domain.TaskEventRepository
	AppStateRepo domain.AppStateRepository
	Runner       *claude.Runner
	Canceller    claude.Canceller
	Slack        SlackNotifier
	PRCreator    PRCreator
	Clock        Clock
	Windows      []*domain.ActiveWindow
	WorktreeRoot string
	PromptsDir   string
	LogDir       string
}

// Worker executes a single task end-to-end.
type Worker struct {
	cfg WorkerConfig
}

// NewWorker creates a new Worker.
func NewWorker(cfg WorkerConfig) *Worker {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	return &Worker{cfg: cfg}
}

// RunTask is the main task execution pipeline.
func (w *Worker) RunTask(ctx context.Context, task *domain.Task) error {
	// Mark running.
	now := w.cfg.Clock.Now()
	task.Status = domain.TaskStatusRunning
	task.StartedAt = &now
	if err := w.cfg.TaskRepo.Update(ctx, task); err != nil {
		return fmt.Errorf("mark running: %w", err)
	}
	w.recordEvent(ctx, task.ID, domain.EventKindStarted, map[string]interface{}{
		"repo": task.RepoFullName, "issue": task.IssueNumber,
	})

	// Notify Slack.
	if w.cfg.Slack != nil {
		if err := w.cfg.Slack.NotifyStarted(ctx, task); err != nil {
			slog.Warn("worker: slack notify started failed", "err", err, "task_id", task.ID)
		}
		w.recordEvent(ctx, task.ID, domain.EventKindSlackSent, map[string]interface{}{"kind": "started"})
	}

	// Create worktree.
	worktreePath := filepath.Join(w.cfg.WorktreeRoot, "task-"+task.ID)
	branch := fmt.Sprintf("claude/issue-%d", task.IssueNumber)
	if err := w.createWorktree(ctx, task, worktreePath, branch); err != nil {
		return w.fail(ctx, task, fmt.Sprintf("create worktree: %v", err))
	}
	task.WorktreePath = worktreePath
	if err := w.cfg.TaskRepo.Update(ctx, task); err != nil {
		slog.Warn("worker: update worktree path", "err", err)
	}

	// Check if context was cancelled.
	select {
	case <-ctx.Done():
		return w.cancel(ctx, task)
	default:
	}

	// Render prompt.
	prompt, err := claude.RenderPrompt(w.cfg.PromptsDir, string(task.TaskType), claude.PromptData{
		Repo:       task.RepoFullName,
		Branch:     branch,
		BaseBranch: "main",
		Worktree:   worktreePath,
		TaskType:   string(task.TaskType),
		Issue: claude.IssueCtx{
			Number: task.IssueNumber,
			Title:  task.IssueTitle,
		},
	})
	if err != nil {
		return w.fail(ctx, task, fmt.Sprintf("render prompt: %v", err))
	}

	// Re-check window gate (double gate).
	fullMode := w.isFullMode(ctx)
	if !AllowNow(w.cfg.Clock.Now(), fullMode, w.cfg.Windows) {
		_ = w.removeWorktree(task.WorktreePath)
		return w.cancel(ctx, task)
	}

	// Run Claude.
	gate := &funcWindowGate{fn: func(t time.Time, fm bool) bool {
		return AllowNow(t, fm, w.cfg.Windows)
	}}

	result, runErr := w.cfg.Runner.Run(ctx, claude.RunInput{
		Prompt:     prompt,
		Worktree:   worktreePath,
		FullMode:   fullMode,
		WindowGate: gate,
	})

	// Log stdout to file.
	if result != nil && result.TextOutput != "" {
		w.writeLog(task.ID, result.TextOutput)
		w.recordEvent(ctx, task.ID, domain.EventKindClaudeStdoutChunk, map[string]interface{}{
			"len": len(result.TextOutput),
		})
	}

	// Handle Claude errors.
	if runErr != nil {
		slog.Error("worker: claude run failed", "err", runErr, "task_id", task.ID)
		stderrTail := ""
		if result != nil {
			stderrTail = result.StderrTail
		}
		// Check if cancelled.
		select {
		case <-ctx.Done():
			return w.cancel(ctx, task)
		default:
		}
		return w.fail(ctx, task, fmt.Sprintf("claude: %v\n%s", runErr, stderrTail))
	}

	// Update token usage.
	if result != nil {
		task.EstimatedInputTokens = result.InputTokens
		task.EstimatedOutputTokens = result.OutputTokens
	}

	// Create PR.
	if w.cfg.PRCreator != nil {
		prURL, prNum, prErr := w.cfg.PRCreator.CreatePR(ctx, task)
		if prErr != nil {
			slog.Error("worker: create PR failed", "err", prErr, "task_id", task.ID)
			_ = w.removeWorktree(task.WorktreePath)
			return w.fail(ctx, task, fmt.Sprintf("create PR: %v", prErr))
		}
		task.PRURL = prURL
		task.PRNumber = prNum
		w.recordEvent(ctx, task.ID, domain.EventKindPRCreated, map[string]interface{}{
			"pr_url": prURL, "pr_number": prNum,
		})
	}

	// Mark done.
	finishedAt := w.cfg.Clock.Now()
	task.Status = domain.TaskStatusDone
	task.FinishedAt = &finishedAt
	if err = w.cfg.TaskRepo.Update(ctx, task); err != nil {
		slog.Error("worker: mark done", "err", err, "task_id", task.ID)
	}

	// Notify Slack done.
	if w.cfg.Slack != nil {
		if err = w.cfg.Slack.NotifyDone(ctx, task); err != nil {
			slog.Warn("worker: slack notify done", "err", err)
		}
	}

	// Clean up worktree.
	_ = w.removeWorktree(task.WorktreePath)
	return nil
}

func (w *Worker) fail(ctx context.Context, task *domain.Task, msg string) error {
	slog.Error("worker: task failed", "task_id", task.ID, "msg", msg)
	finishedAt := w.cfg.Clock.Now()
	task.Status = domain.TaskStatusFailed
	task.FinishedAt = &finishedAt
	task.StderrTail = msg
	if err := w.cfg.TaskRepo.Update(ctx, task); err != nil {
		slog.Error("worker: update failed task", "err", err)
	}
	w.recordEvent(ctx, task.ID, domain.EventKindFailed, map[string]interface{}{"msg": msg})
	if w.cfg.Slack != nil {
		if err := w.cfg.Slack.NotifyFailed(ctx, task, msg); err != nil {
			slog.Warn("worker: slack notify failed", "err", err)
		}
	}
	return fmt.Errorf("task %s failed: %s", task.ID, msg)
}

func (w *Worker) cancel(ctx context.Context, task *domain.Task) error {
	slog.Info("worker: task cancelled", "task_id", task.ID)
	finishedAt := w.cfg.Clock.Now()
	task.Status = domain.TaskStatusCancelled
	task.FinishedAt = &finishedAt
	if err := w.cfg.TaskRepo.Update(ctx, task); err != nil {
		slog.Error("worker: update cancelled task", "err", err)
	}
	w.recordEvent(ctx, task.ID, domain.EventKindCancelled, nil)
	_ = w.removeWorktree(task.WorktreePath)
	if w.cfg.Slack != nil {
		if err := w.cfg.Slack.NotifyCancelled(ctx, task); err != nil {
			slog.Warn("worker: slack notify cancelled", "err", err)
		}
	}
	return nil
}

func (w *Worker) recordEvent(ctx context.Context, taskID string, kind domain.EventKind, payload interface{}) {
	if w.cfg.EventRepo == nil {
		return
	}
	payloadJSON := "{}"
	if payload != nil {
		b, _ := json.Marshal(payload)
		payloadJSON = string(b)
	}
	event := &domain.TaskEvent{
		ID:          uuid.New().String(),
		TaskID:      taskID,
		Kind:        kind,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now(),
	}
	if err := w.cfg.EventRepo.Create(ctx, event); err != nil {
		slog.Warn("worker: record event", "err", err)
	}
}

func (w *Worker) createWorktree(ctx context.Context, task *domain.Task, path, branch string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("mkdir worktrees: %w", err)
	}

	// Find the repo path (expected to be a checked-out clone somewhere).
	// For simplicity, run git commands in the CWD.
	// Production usage expects a pre-cloned repo at WorktreeRoot/../<repo>.
	repoDir := filepath.Join(w.cfg.WorktreeRoot, "..", strings.ReplaceAll(task.RepoFullName, "/", "_"))
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		// Fallback: use cwd
		repoDir = "."
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, path)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return nil
}

func (w *Worker) removeWorktree(path string) error {
	if path == "" {
		return nil
	}
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("worker: git worktree remove", "path", path, "err", err, "out", string(out))
	}
	return err
}

func (w *Worker) writeLog(taskID, content string) {
	if w.cfg.LogDir == "" {
		return
	}
	if err := os.MkdirAll(w.cfg.LogDir, 0o750); err != nil {
		return
	}
	path := filepath.Join(w.cfg.LogDir, taskID+".log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		slog.Warn("worker: write task log", "err", err)
	}
}

func (w *Worker) isFullMode(ctx context.Context) bool {
	if w.cfg.AppStateRepo == nil {
		return false
	}
	state, err := w.cfg.AppStateRepo.Get(ctx, "full_mode")
	if err != nil {
		return false
	}
	return len(state.ValueJSON) > 0 && containsTrue(state.ValueJSON)
}

// funcWindowGate adapts a function to the claude.WindowGate interface.
type funcWindowGate struct {
	fn func(t time.Time, fullMode bool) bool
}

func (g *funcWindowGate) AllowNow(t time.Time, fullMode bool) bool {
	return g.fn(t, fullMode)
}
