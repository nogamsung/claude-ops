package qualitygate

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type scriptedRunner struct {
	steps []stepResult
	seen  []string
}

type stepResult struct {
	Output   string
	ExitCode int
	Err      error
	Delay    time.Duration
}

func (r *scriptedRunner) Run(ctx context.Context, _ string, cmdline string, out io.Writer) (int, error) {
	r.seen = append(r.seen, cmdline)
	if len(r.seen) > len(r.steps) {
		return 0, nil
	}
	step := r.steps[len(r.seen)-1]
	if step.Delay > 0 {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(step.Delay):
		}
	}
	if step.Output != "" {
		_, _ = io.WriteString(out, step.Output)
	}
	return step.ExitCode, step.Err
}

func TestGate_AllPass_ReturnsPassed(t *testing.T) {
	runner := &scriptedRunner{steps: []stepResult{
		{ExitCode: 0}, {ExitCode: 0}, {ExitCode: 0},
	}}
	gate := NewGate(runner, 50)

	res := gate.Run(context.Background(), "/tmp", []string{"a", "b", "c"}, time.Minute)

	if !res.Passed {
		t.Fatalf("expected passed, got failed: %+v", res)
	}
	if len(runner.seen) != 3 {
		t.Errorf("expected 3 commands run, got %d", len(runner.seen))
	}
}

func TestGate_FirstFailureHaltsChain(t *testing.T) {
	runner := &scriptedRunner{steps: []stepResult{
		{ExitCode: 0},
		{ExitCode: 1, Output: "assertion failed\ntest report\n"},
		{ExitCode: 0}, // never reached
	}}
	gate := NewGate(runner, 50)

	res := gate.Run(context.Background(), "/tmp", []string{"go vet", "go test", "golangci-lint run"}, time.Minute)

	if res.Passed {
		t.Fatalf("expected failed, got passed")
	}
	if res.FailedCommand != "go test" {
		t.Errorf("expected failed command 'go test', got %q", res.FailedCommand)
	}
	if res.ExitCode != 1 {
		t.Errorf("expected exit 1, got %d", res.ExitCode)
	}
	if !strings.Contains(res.OutputTail, "assertion failed") {
		t.Errorf("expected output tail, got %q", res.OutputTail)
	}
	if len(runner.seen) != 2 {
		t.Errorf("expected 2 commands executed, got %d (chain should abort)", len(runner.seen))
	}
}

func TestGate_EmptyCommandsPasses(t *testing.T) {
	gate := NewGate(&scriptedRunner{}, 50)
	res := gate.Run(context.Background(), "/tmp", nil, time.Minute)
	if !res.Passed {
		t.Error("empty command list should pass")
	}
}

func TestGate_PerCommandTimeout(t *testing.T) {
	runner := &scriptedRunner{steps: []stepResult{
		{ExitCode: 0, Delay: 50 * time.Millisecond},
		{ExitCode: 0, Delay: 5 * time.Second}, // will time out
	}}
	gate := NewGate(runner, 50)

	res := gate.Run(context.Background(), "/tmp", []string{"fast", "slow"}, 100*time.Millisecond)

	if res.Passed {
		t.Fatal("expected timeout failure")
	}
	if res.FailedCommand != "slow" {
		t.Errorf("expected 'slow' failed, got %q", res.FailedCommand)
	}
}

func TestTailLines(t *testing.T) {
	in := "a\nb\nc\nd\ne\n"
	if got := tailLines(in, 3); got != "c\nd\ne" {
		t.Errorf("tail=3 want 'c\\nd\\ne', got %q", got)
	}
	if got := tailLines(in, 10); got != "a\nb\nc\nd\ne" {
		t.Errorf("tail>=len want full, got %q", got)
	}
	if got := tailLines("", 3); got != "" {
		t.Errorf("empty want empty, got %q", got)
	}
}

// TestShellRunner_RealExec exercises the real sh path with a no-op command
// to guard against regressions in the shell wiring (Setpgid, Stdout pipe).
func TestShellRunner_RealExec(t *testing.T) {
	r := &ShellRunner{}
	var buf strings.Builder
	exit, err := r.Run(context.Background(), t.TempDir(), "echo hello", &buf)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d", exit)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected 'hello' in output, got %q", buf.String())
	}
}

func TestShellRunner_NonZeroExit(t *testing.T) {
	r := &ShellRunner{}
	var buf strings.Builder
	exit, err := r.Run(context.Background(), t.TempDir(), "exit 3", &buf)
	if err != nil {
		t.Fatalf("unexpected err for exit code: %v", err)
	}
	if exit != 3 {
		t.Errorf("expected exit 3, got %d", exit)
	}
}

func TestShellRunner_TimeoutKills(t *testing.T) {
	r := &ShellRunner{KillGrace: 200 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var buf strings.Builder
	_, err := r.Run(ctx, t.TempDir(), "sleep 5", &buf)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected 'timeout' in error, got %v", err)
	}
}
