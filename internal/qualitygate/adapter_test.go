package qualitygate

import (
	"context"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/config"
)

func TestAdapter_Lookup_ReturnsConfiguredCommands(t *testing.T) {
	a := NewAdapter([]config.RepoConfig{
		{Name: "owner/repo", Checks: config.ChecksConfig{
			Commands: []string{"go test", "golangci-lint run"},
			Timeout:  3 * time.Minute,
		}},
	})

	cmds, timeout, ok := a.Lookup("owner/repo")
	if !ok {
		t.Fatal("expected ok=true for configured repo")
	}
	if len(cmds) != 2 {
		t.Errorf("expected 2 commands, got %d", len(cmds))
	}
	if timeout != 3*time.Minute {
		t.Errorf("expected 3m timeout, got %v", timeout)
	}
}

func TestAdapter_Lookup_UnknownRepoReturnsFalse(t *testing.T) {
	a := NewAdapter([]config.RepoConfig{
		{Name: "owner/known", Checks: config.ChecksConfig{Commands: []string{"x"}}},
	})
	_, _, ok := a.Lookup("owner/unknown")
	if ok {
		t.Error("expected ok=false for unknown repo")
	}
}

func TestAdapter_Lookup_EmptyCommandsReturnsFalse(t *testing.T) {
	a := NewAdapter([]config.RepoConfig{
		{Name: "owner/repo", Checks: config.ChecksConfig{}}, // no commands
	})
	_, _, ok := a.Lookup("owner/repo")
	if ok {
		t.Error("expected ok=false when no commands configured")
	}
}

func TestAdapter_Lookup_ZeroTimeoutFallsBackToDefault(t *testing.T) {
	a := NewAdapter([]config.RepoConfig{
		{Name: "owner/repo", Checks: config.ChecksConfig{
			Commands: []string{"go test"},
			Timeout:  0,
		}},
	})
	_, timeout, _ := a.Lookup("owner/repo")
	if timeout != 5*time.Minute {
		t.Errorf("expected 5m default, got %v", timeout)
	}
}

func TestAdapter_Run_MapsResult(t *testing.T) {
	gate := NewGate(&scriptedRunner{steps: []stepResult{
		{ExitCode: 1, Output: "failure message"},
	}}, 50)
	a := NewAdapterWithGate([]config.RepoConfig{
		{Name: "owner/repo"},
	}, gate)

	res := a.Run(context.Background(), "/tmp", []string{"go test"}, time.Minute)

	if res.Passed {
		t.Fatal("expected failure mapping")
	}
	if res.FailedCommand != "go test" {
		t.Errorf("want failed 'go test', got %q", res.FailedCommand)
	}
	if res.ExitCode != 1 {
		t.Errorf("want exit 1, got %d", res.ExitCode)
	}
}
