package github

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gh "github.com/google/go-github/v60/github"
	"github.com/google/uuid"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
)

// IssuesService abstracts the GitHub issues API for testing.
type IssuesService interface {
	ListByRepo(ctx context.Context, owner, repo string, opts *gh.IssueListByRepoOptions) ([]*gh.Issue, *gh.Response, error)
}

// Poller fetches open issues matching the allowlist and enqueues them.
type Poller struct {
	issues   IssuesService
	taskRepo domain.TaskRepository
	repos    []config.RepoConfig
}

// NewPoller creates a Poller.
func NewPoller(issues IssuesService, taskRepo domain.TaskRepository, repos []config.RepoConfig) *Poller {
	return &Poller{issues: issues, taskRepo: taskRepo, repos: repos}
}

// Poll fetches new issues for all configured repos and enqueues tasks.
func (p *Poller) Poll(ctx context.Context) error {
	for _, repo := range p.repos {
		if err := p.pollRepo(ctx, repo); err != nil {
			slog.Error("poller: repo error", "repo", repo.Name, "err", err)
			// Continue with other repos.
		}
	}
	return nil
}

func (p *Poller) pollRepo(ctx context.Context, repo config.RepoConfig) error {
	parts := strings.SplitN(repo.Name, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo name %q", repo.Name)
	}
	owner, name := parts[0], parts[1]

	opts := &gh.IssueListByRepoOptions{
		State:  "open",
		Labels: repo.Labels,
		ListOptions: gh.ListOptions{
			PerPage: 50,
		},
	}

	issues, _, err := p.issues.ListByRepo(ctx, owner, name, opts)
	if err != nil {
		return fmt.Errorf("list issues %s: %w", repo.Name, err)
	}

	for _, issue := range issues {
		if issue.IsPullRequest() {
			continue // Skip pull requests.
		}
		if !hasAllLabels(issue, repo.Labels) {
			continue
		}

		issueNum := issue.GetNumber()
		exists, err := p.taskRepo.ExistsByRepoAndIssue(ctx, repo.Name, issueNum)
		if err != nil {
			slog.Error("poller: check exists", "repo", repo.Name, "issue", issueNum, "err", err)
			continue
		}
		if exists {
			continue // Already queued or running.
		}

		taskType := detectTaskType(issue)
		task := &domain.Task{
			ID:           uuid.New().String(),
			RepoFullName: repo.Name,
			IssueNumber:  issueNum,
			IssueTitle:   issue.GetTitle(),
			TaskType:     taskType,
			Status:       domain.TaskStatusQueued,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err = p.taskRepo.Create(ctx, task); err != nil {
			slog.Error("poller: create task", "repo", repo.Name, "issue", issueNum, "err", err)
			continue
		}
		slog.Info("poller: enqueued task", "task_id", task.ID, "repo", repo.Name, "issue", issueNum)
	}

	return nil
}

// hasAllLabels reports whether the issue has all of the required labels.
func hasAllLabels(issue *gh.Issue, required []string) bool {
	if len(required) == 0 {
		return true
	}
	issueLabels := make(map[string]struct{}, len(issue.Labels))
	for _, l := range issue.Labels {
		issueLabels[l.GetName()] = struct{}{}
	}
	for _, r := range required {
		if _, ok := issueLabels[r]; !ok {
			return false
		}
	}
	return true
}

// detectTaskType returns the task type based on issue labels.
func detectTaskType(issue *gh.Issue) domain.TaskType {
	for _, l := range issue.Labels {
		switch l.GetName() {
		case "security":
			return domain.TaskTypeSecurity
		case "perf", "performance":
			return domain.TaskTypePerf
		}
	}
	return domain.TaskTypeFeature
}
