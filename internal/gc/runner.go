// Package gc garbage-collects stale worktrees and reconciles orphan tasks
// that the service restarted mid-run. Failures are logged and do not abort —
// the next tick will re-attempt.
package gc

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// Clock lets tests inject deterministic time.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// SlackNotifier is the subset of the Slack client used by orphan recovery. A
// nil notifier silently skips human-facing messages (still updates DB).
type SlackNotifier interface {
	NotifyOrphaned(ctx context.Context, task *domain.Task) error
}

// GitRunner abstracts `git` invocations so tests can verify calls without
// spawning real processes.
type GitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

// ExecGitRunner is the default real-shell runner.
type ExecGitRunner struct{}

// Run invokes git with args and returns the combined output.
func (ExecGitRunner) Run(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	return string(out), err
}

// Config holds GC runtime knobs.
type Config struct {
	RetentionDays int           // older than this → eligible for worktree removal (<=0 disables)
	Interval      time.Duration // how often to scan; 24h default
}

// Runner does two jobs on a schedule:
//  1. RecoverOrphans — on boot, reconcile tasks stuck in running
//  2. SweepWorktrees — on boot + daily, remove worktrees for
//     terminal-status tasks older than RetentionDays
type Runner struct {
	cfg   Config
	tasks domain.TaskRepository
	git   GitRunner
	slack SlackNotifier
	clock Clock
}

// NewRunner wires dependencies. A nil git/slack/clock is fine — defaults are
// ExecGitRunner, no-op Slack, realClock.
func NewRunner(cfg Config, tasks domain.TaskRepository, git GitRunner, slack SlackNotifier, clock Clock) *Runner {
	if git == nil {
		git = ExecGitRunner{}
	}
	if clock == nil {
		clock = realClock{}
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	return &Runner{cfg: cfg, tasks: tasks, git: git, slack: slack, clock: clock}
}

// RunOnBoot runs both orphan recovery and a worktree sweep immediately.
// Intended for DI wiring during main() startup before the HTTP server listens.
func (r *Runner) RunOnBoot(ctx context.Context) {
	r.RecoverOrphans(ctx)
	r.SweepWorktrees(ctx)
}

// Start blocks until ctx cancels, triggering SweepWorktrees every
// cfg.Interval. RecoverOrphans is only run on boot (by RunOnBoot).
func (r *Runner) Start(ctx context.Context) {
	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.SweepWorktrees(ctx)
		}
	}
}

// RecoverOrphans reconciles tasks that were running at shutdown. Policy (per
// issue #12):
//   - worktree exists AND dirty → status=orphaned (human judgment) + Slack
//   - worktree missing OR clean → status=failed, stderr=service_restart
func (r *Runner) RecoverOrphans(ctx context.Context) {
	running, err := r.tasks.GetRunning(ctx)
	if err != nil {
		slog.Error("gc: list running tasks", "err", err)
		return
	}
	for _, t := range running {
		r.reconcileOrphan(ctx, t)
	}
}

func (r *Runner) reconcileOrphan(ctx context.Context, task *domain.Task) {
	dirty := r.isDirty(ctx, task.WorktreePath)
	if dirty {
		task.Status = domain.TaskStatusOrphaned
		task.StderrTail = "service restarted while task was running; worktree has unsaved changes — manual review required"
		if err := r.tasks.Update(ctx, task); err != nil {
			slog.Error("gc: mark orphaned", "task_id", task.ID, "err", err)
			return
		}
		slog.Warn("gc: task marked orphaned (worktree dirty)", "task_id", task.ID, "worktree", task.WorktreePath)
		if r.slack != nil {
			if notifyErr := r.slack.NotifyOrphaned(ctx, task); notifyErr != nil {
				slog.Warn("gc: slack notify orphaned", "err", notifyErr)
			}
		}
		return
	}

	task.Status = domain.TaskStatusFailed
	task.StderrTail = "service_restart"
	if err := r.tasks.Update(ctx, task); err != nil {
		slog.Error("gc: mark failed on restart", "task_id", task.ID, "err", err)
		return
	}
	slog.Warn("gc: task auto-failed on restart (worktree clean)", "task_id", task.ID)
}

// isDirty returns true when worktree path exists and git reports uncommitted
// changes. Absent path or any git error → treated as clean, because the
// recovery default on uncertainty is "fail, don't orphan" (orphan status is
// strictly worse for the user — it requires manual action).
func (r *Runner) isDirty(ctx context.Context, worktree string) bool {
	if worktree == "" {
		return false
	}
	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		return false
	}
	out, err := r.git.Run(ctx, "-C", worktree, "status", "--porcelain")
	if err != nil {
		slog.Warn("gc: git status failed, treating as clean", "worktree", worktree, "err", err)
		return false
	}
	return len(out) > 0
}

// SweepWorktrees removes on-disk worktrees for tasks that have been terminal
// for longer than RetentionDays. Runs `git worktree remove --force` first (so
// git's metadata stays consistent) and falls back to plain os.RemoveAll.
func (r *Runner) SweepWorktrees(ctx context.Context) {
	if r.cfg.RetentionDays <= 0 {
		return
	}
	cutoff := r.clock.Now().AddDate(0, 0, -r.cfg.RetentionDays)
	terminal := []domain.TaskStatus{domain.TaskStatusFailed, domain.TaskStatusCancelled}

	for _, status := range terminal {
		filter := domain.TaskFilter{Status: &status, Limit: 500}
		tasks, err := r.tasks.List(ctx, filter)
		if err != nil {
			slog.Error("gc: list tasks for sweep", "status", status, "err", err)
			continue
		}
		for _, t := range tasks {
			if t.WorktreePath == "" || t.FinishedAt == nil || t.FinishedAt.After(cutoff) {
				continue
			}
			r.removeWorktree(ctx, t)
		}
	}
}

func (r *Runner) removeWorktree(ctx context.Context, task *domain.Task) {
	path := task.WorktreePath
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Already gone — clear the DB pointer so we don't keep scanning it.
		task.WorktreePath = ""
		if err := r.tasks.Update(ctx, task); err != nil {
			slog.Warn("gc: clear missing worktree ref", "task_id", task.ID, "err", err)
		}
		return
	}
	if _, err := r.git.Run(ctx, "worktree", "remove", "--force", path); err != nil {
		slog.Warn("gc: git worktree remove failed, falling back to rm", "path", path, "err", err)
		if rmErr := os.RemoveAll(filepath.Clean(path)); rmErr != nil {
			slog.Error("gc: rm worktree", "path", path, "err", rmErr)
			return
		}
	}
	task.WorktreePath = ""
	if err := r.tasks.Update(ctx, task); err != nil {
		slog.Warn("gc: clear worktree ref after removal", "task_id", task.ID, "err", err)
	}
	slog.Info("gc: swept worktree", "task_id", task.ID, "path", path)
}
