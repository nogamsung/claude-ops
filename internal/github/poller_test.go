package github_test

import (
	"context"
	"testing"
	"time"

	gh "github.com/google/go-github/v60/github"

	"github.com/gs97ahn/scheduled-dev-agent/internal/config"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
	igithub "github.com/gs97ahn/scheduled-dev-agent/internal/github"
)

// fakeIssuesService implements IssuesService.
type fakeIssuesService struct {
	issues []*gh.Issue
}

func (f *fakeIssuesService) ListByRepo(_ context.Context, _, _ string, _ *gh.IssueListByRepoOptions) ([]*gh.Issue, *gh.Response, error) {
	return f.issues, &gh.Response{}, nil
}

// fakeTaskRepo implements domain.TaskRepository.
type fakeTaskRepo struct {
	tasks  []*domain.Task
	exists bool
}

func (r *fakeTaskRepo) Create(_ context.Context, t *domain.Task) error {
	r.tasks = append(r.tasks, t)
	return nil
}
func (r *fakeTaskRepo) GetByID(_ context.Context, _ string) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (r *fakeTaskRepo) Update(_ context.Context, _ *domain.Task) error { return nil }
func (r *fakeTaskRepo) List(_ context.Context, _ domain.TaskFilter) ([]*domain.Task, error) {
	return r.tasks, nil
}
func (r *fakeTaskRepo) GetRunning(_ context.Context) ([]*domain.Task, error) { return nil, nil }
func (r *fakeTaskRepo) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return r.exists, nil
}

func makeIssue(number int, title string, labels []string, isPR bool) *gh.Issue {
	ghLabels := make([]*gh.Label, len(labels))
	for i, l := range labels {
		l := l
		ghLabels[i] = &gh.Label{Name: &l}
	}
	issue := &gh.Issue{
		Number: &number,
		Title:  &title,
		Labels: ghLabels,
	}
	if isPR {
		issue.PullRequestLinks = &gh.PullRequestLinks{}
	}
	return issue
}

func TestPoller_EnqueuesMatchingIssues(t *testing.T) {
	svc := &fakeIssuesService{
		issues: []*gh.Issue{
			makeIssue(42, "Fix the bug", []string{"claude-ops"}, false),
		},
	}
	repo := &fakeTaskRepo{}
	repos := []config.RepoConfig{
		{Name: "owner/repo", DefaultBranch: "main", Labels: []string{"claude-ops"}},
	}

	poller := igithub.NewPoller(svc, repo, repos)
	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(repo.tasks))
	}
	if repo.tasks[0].IssueNumber != 42 {
		t.Errorf("expected issue 42, got %d", repo.tasks[0].IssueNumber)
	}
}

func TestPoller_SkipsPullRequests(t *testing.T) {
	svc := &fakeIssuesService{
		issues: []*gh.Issue{
			makeIssue(1, "PR should be skipped", []string{"claude-ops"}, true),
		},
	}
	repo := &fakeTaskRepo{}
	repos := []config.RepoConfig{
		{Name: "owner/repo", DefaultBranch: "main", Labels: []string{"claude-ops"}},
	}

	poller := igithub.NewPoller(svc, repo, repos)
	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	if len(repo.tasks) != 0 {
		t.Errorf("expected 0 tasks (PR skipped), got %d", len(repo.tasks))
	}
}

func TestPoller_SkipsAlreadyQueued(t *testing.T) {
	svc := &fakeIssuesService{
		issues: []*gh.Issue{
			makeIssue(99, "Already queued", []string{"claude-ops"}, false),
		},
	}
	repo := &fakeTaskRepo{exists: true}
	repos := []config.RepoConfig{
		{Name: "owner/repo", DefaultBranch: "main", Labels: []string{"claude-ops"}},
	}

	poller := igithub.NewPoller(svc, repo, repos)
	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	if len(repo.tasks) != 0 {
		t.Errorf("expected 0 tasks (already exists), got %d", len(repo.tasks))
	}
}

func TestPoller_DetectsSecurityLabel(t *testing.T) {
	svc := &fakeIssuesService{
		issues: []*gh.Issue{
			makeIssue(7, "Security audit", []string{"claude-ops", "security"}, false),
		},
	}
	repo := &fakeTaskRepo{}
	repos := []config.RepoConfig{
		{Name: "owner/repo", DefaultBranch: "main", Labels: []string{"claude-ops"}},
	}

	poller := igithub.NewPoller(svc, repo, repos)
	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(repo.tasks))
	}
	if repo.tasks[0].TaskType != domain.TaskTypeSecurity {
		t.Errorf("expected security task type, got %s", repo.tasks[0].TaskType)
	}
}

func TestPoller_SkipsMissingLabel(t *testing.T) {
	svc := &fakeIssuesService{
		issues: []*gh.Issue{
			makeIssue(3, "No required label", []string{"other-label"}, false),
		},
	}
	repo := &fakeTaskRepo{}
	repos := []config.RepoConfig{
		{Name: "owner/repo", DefaultBranch: "main", Labels: []string{"claude-ops"}},
	}

	poller := igithub.NewPoller(svc, repo, repos)
	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	if len(repo.tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(repo.tasks))
	}
}

func TestPoller_SetsCorrectDefaults(t *testing.T) {
	svc := &fakeIssuesService{
		issues: []*gh.Issue{
			makeIssue(55, "Test defaults", []string{"claude-ops"}, false),
		},
	}
	repo := &fakeTaskRepo{}
	repos := []config.RepoConfig{
		{Name: "owner/repo", DefaultBranch: "main", Labels: []string{"claude-ops"}},
	}

	before := time.Now()
	poller := igithub.NewPoller(svc, repo, repos)
	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	after := time.Now()

	if len(repo.tasks) != 1 {
		t.Fatalf("expected 1 task")
	}
	task := repo.tasks[0]
	if task.Status != domain.TaskStatusQueued {
		t.Errorf("expected queued status, got %s", task.Status)
	}
	if task.CreatedAt.Before(before) || task.CreatedAt.After(after) {
		t.Errorf("unexpected created_at: %v", task.CreatedAt)
	}
}
