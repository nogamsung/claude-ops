package qualitygate

import (
	"context"
	"time"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

// Adapter bridges a set of RepoConfig entries to the scheduler.QualityGate
// interface. Repos without Checks.Commands are silently skipped (no gating).
type Adapter struct {
	gate  *Gate
	repos map[string]config.RepoConfig
}

// NewAdapter constructs an Adapter over repos using a fresh ShellRunner.
// Exposed separately so tests can inject a scripted runner via NewAdapterWithGate.
func NewAdapter(repos []config.RepoConfig) *Adapter {
	return NewAdapterWithGate(repos, NewGate(&ShellRunner{}, 50))
}

// NewAdapterWithGate is the testable constructor — pass a Gate wired with a
// scripted CommandRunner.
func NewAdapterWithGate(repos []config.RepoConfig, gate *Gate) *Adapter {
	m := make(map[string]config.RepoConfig, len(repos))
	for _, r := range repos {
		m[r.Name] = r
	}
	return &Adapter{gate: gate, repos: m}
}

// Lookup returns the configured commands + timeout for repoFullName. ok is
// false when the repo is unknown or has no commands, so the Worker can skip
// the gate stage without a branch on both fields.
func (a *Adapter) Lookup(repoFullName string) ([]string, time.Duration, bool) {
	r, ok := a.repos[repoFullName]
	if !ok || len(r.Checks.Commands) == 0 {
		return nil, 0, false
	}
	timeout := r.Checks.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return r.Checks.Commands, timeout, true
}

// Run executes the configured commands and maps the result into the
// scheduler-side DTO (no package coupling).
func (a *Adapter) Run(ctx context.Context, worktreeDir string, commands []string, timeout time.Duration) scheduler.QualityGateResult {
	res := a.gate.Run(ctx, worktreeDir, commands, timeout)
	return scheduler.QualityGateResult{
		Passed:        res.Passed,
		FailedCommand: res.FailedCommand,
		ExitCode:      res.ExitCode,
		OutputTail:    res.OutputTail,
	}
}
