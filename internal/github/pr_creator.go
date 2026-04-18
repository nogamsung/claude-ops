package github

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
)

// GhRunner abstracts the gh CLI invocation for testing.
type GhRunner interface {
	RunGh(ctx context.Context, args ...string) (string, error)
}

// RealGhRunner invokes the real gh CLI binary.
type RealGhRunner struct{}

// RunGh invokes gh with the given args and returns stdout.
func (r *RealGhRunner) RunGh(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %v: %w\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// PRCreator creates pull requests using gh CLI.
type PRCreator struct {
	gh    GhRunner
	repos map[string]config.RepoConfig
}

// NewPRCreator creates a PRCreator.
func NewPRCreator(gh GhRunner, repos []config.RepoConfig) *PRCreator {
	repoMap := make(map[string]config.RepoConfig, len(repos))
	for _, r := range repos {
		repoMap[r.Name] = r
	}
	return &PRCreator{gh: gh, repos: repoMap}
}

// CreatePR commits, pushes, and creates a PR for the given task.
func (c *PRCreator) CreatePR(ctx context.Context, task *domain.Task) (string, int, error) {
	repo, ok := c.repos[task.RepoFullName]
	if !ok {
		return "", 0, fmt.Errorf("repo %q not in allowlist", task.RepoFullName)
	}

	branch := fmt.Sprintf("claude/issue-%d", task.IssueNumber)
	worktree := task.WorktreePath

	// git add + commit
	if err := runGitInDir(ctx, worktree, "add", "-A"); err != nil {
		return "", 0, fmt.Errorf("git add: %w", err)
	}

	commitMsg := fmt.Sprintf("feat: resolve issue #%d via claude\n\nCloses #%d", task.IssueNumber, task.IssueNumber)
	if err := runGitInDir(ctx, worktree, "commit", "-m", commitMsg); err != nil {
		return "", 0, fmt.Errorf("git commit: %w", err)
	}

	// git push
	pushErr := runGitInDir(ctx, worktree, "push", "origin", branch)
	if pushErr != nil {
		// Rebase once, then retry.
		slog.Warn("pr_creator: push failed, attempting rebase", "err", pushErr)
		if rebaseErr := runGitInDir(ctx, worktree, "pull", "--rebase", "origin", repo.DefaultBranch); rebaseErr != nil {
			return "", 0, fmt.Errorf("rebase: %w", rebaseErr)
		}
		if pushErr = runGitInDir(ctx, worktree, "push", "origin", branch); pushErr != nil {
			return "", 0, fmt.Errorf("push after rebase: %w", pushErr)
		}
	}

	// gh pr create
	prTitle := fmt.Sprintf("fix: issue #%d — %s", task.IssueNumber, task.IssueTitle)
	prBody := fmt.Sprintf("Closes #%d\n\nAutomatically resolved by Claude Ops.", task.IssueNumber)

	ghArgs := []string{
		"pr", "create",
		"--repo", task.RepoFullName,
		"--base", repo.DefaultBranch,
		"--head", branch,
		"--title", prTitle,
		"--body", prBody,
	}
	for _, reviewer := range repo.Reviewers {
		ghArgs = append(ghArgs, "--reviewer", reviewer)
	}

	prURL, err := c.gh.RunGh(ctx, ghArgs...)
	if err != nil {
		return "", 0, fmt.Errorf("gh pr create: %w", err)
	}

	// Extract PR number from URL (last path segment).
	prNum, _ := strconv.Atoi(prURL[strings.LastIndex(prURL, "/")+1:])

	return prURL, prNum, nil
}

func runGitInDir(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}
