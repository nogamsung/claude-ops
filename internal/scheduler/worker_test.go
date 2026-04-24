package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

// fakeSlack implements scheduler.SlackNotifier for tests.
type fakeSlack struct {
	startedCalls   int
	doneCalls      int
	failedCalls    int
	cancelledCalls int
}

func (s *fakeSlack) NotifyStarted(_ context.Context, _ *domain.Task) error {
	s.startedCalls++
	return nil
}
func (s *fakeSlack) NotifyDone(_ context.Context, _ *domain.Task) error {
	s.doneCalls++
	return nil
}
func (s *fakeSlack) NotifyFailed(_ context.Context, _ *domain.Task, _ string) error {
	s.failedCalls++
	return nil
}
func (s *fakeSlack) NotifyCancelled(_ context.Context, _ *domain.Task) error {
	s.cancelledCalls++
	return nil
}

// fakePRCreator implements scheduler.PRCreator.
type fakePRCreator struct {
	err   error
	calls int
}

func (p *fakePRCreator) CreatePR(_ context.Context, _ *domain.Task) (string, int, error) {
	p.calls++
	if p.err != nil {
		return "", 0, p.err
	}
	return "https://github.com/owner/repo/pull/1", 1, nil
}

func TestWorker_TaskFails_IfPromptDirMissing(t *testing.T) {
	taskRepo := &fakeTaskRepo{}
	task := &domain.Task{
		ID:           "w-test-1",
		Status:       domain.TaskStatusQueued,
		RepoFullName: "owner/repo",
		IssueNumber:  1,
		TaskType:     domain.TaskTypeFeature,
	}
	taskRepo.tasks = append(taskRepo.tasks, task)

	slackNotifier := &fakeSlack{}
	worker := scheduler.NewWorker(scheduler.WorkerConfig{
		TaskRepo:  taskRepo,
		EventRepo: &fakeEventRepo{},
		// Runner is nil, PromptsDir is empty — prompt render will fail.
		Slack:        slackNotifier,
		PRCreator:    &fakePRCreator{},
		Clock:        &scheduler.FakeClock{T: time.Now()},
		WorktreeRoot: t.TempDir(),
		PromptsDir:   "/nonexistent_prompts_dir",
	})

	// The worker will fail at worktree creation (git not available in test env).
	err := worker.RunTask(context.Background(), task)
	// We just verify it returns an error and doesn't panic.
	if err == nil {
		t.Error("expected error when running worker with missing prompts dir")
	}
	// Slack failure notification should have been sent.
	if slackNotifier.failedCalls < 1 {
		t.Error("expected slack failure notification")
	}
}

type fakeEventRepo struct{}

func (r *fakeEventRepo) Create(_ context.Context, _ *domain.TaskEvent) error { return nil }
func (r *fakeEventRepo) ListByTaskID(_ context.Context, _ string, _ int) ([]*domain.TaskEvent, error) {
	return nil, nil
}

// fakeMetrics captures MetricsRecorder calls for assertion.
type fakeMetrics struct {
	finished []finishedCall
	blocks   []scheduler.BudgetReason
	closes   int
}

type finishedCall struct {
	Repo, Type, Status string
	Start, End         time.Time
}

func (m *fakeMetrics) RecordTaskFinished(repo, tt, status string, start, end time.Time) {
	m.finished = append(m.finished, finishedCall{repo, tt, status, start, end})
}
func (m *fakeMetrics) RecordBudgetBlock(reason scheduler.BudgetReason) {
	m.blocks = append(m.blocks, reason)
}
func (m *fakeMetrics) RecordWindowClose() { m.closes++ }

// TestWorker_MetricsRecorded_OnFailure exercises the fail() path and asserts
// the MetricsRecorder sees the terminal-status call.
func TestWorker_MetricsRecorded_OnFailure(t *testing.T) {
	taskRepo := &fakeTaskRepo{}
	task := &domain.Task{
		ID:           "metrics-fail",
		Status:       domain.TaskStatusQueued,
		RepoFullName: "owner/repo",
		IssueNumber:  7,
		TaskType:     domain.TaskTypeFeature,
	}
	taskRepo.tasks = append(taskRepo.tasks, task)

	m := &fakeMetrics{}
	worker := scheduler.NewWorker(scheduler.WorkerConfig{
		TaskRepo:     taskRepo,
		EventRepo:    &fakeEventRepo{},
		Slack:        &fakeSlack{},
		PRCreator:    &fakePRCreator{},
		Metrics:      m,
		Clock:        &scheduler.FakeClock{T: time.Now()},
		WorktreeRoot: t.TempDir(),
		PromptsDir:   "/nonexistent_prompts_dir",
	})

	_ = worker.RunTask(context.Background(), task)

	if len(m.finished) != 1 {
		t.Fatalf("expected 1 RecordTaskFinished call, got %d", len(m.finished))
	}
	got := m.finished[0]
	if got.Repo != "owner/repo" || got.Type != "feature" || got.Status != "failed" {
		t.Errorf("unexpected finish call: %+v", got)
	}
}
