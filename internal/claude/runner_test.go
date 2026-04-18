package claude_test

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/claude"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// fakeGate implements claude.WindowGate.
type fakeGate struct {
	allow bool
}

func (g *fakeGate) AllowNow(_ time.Time, _ bool) bool { return g.allow }

// echoRunner creates a runner that invokes a Go echo program.
func echoRunner(output string) claude.ExecCommandFunc {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Use 'echo' to emit the fixture output, then exit 0.
		script := fmt.Sprintf(`cat <<'HEREDOC'
%s
HEREDOC`, output)
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
}

func successNDJSON() string {
	return `{"type":"system.init","apiKeySource":"none"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":100,"output_tokens":10}}}
{"type":"result","subtype":"success","is_error":false,"result":"Done.","stop_reason":"end_turn","total_cost_usd":0.001,"usage":{"input_tokens":100,"output_tokens":10},"terminal_reason":"completed"}`
}

func TestRunner_Success(t *testing.T) {
	r := claude.NewRunnerWithExec(echoRunner(successNDJSON()))
	gate := &fakeGate{allow: true}
	res, err := r.Run(context.Background(), claude.RunInput{
		Prompt:     "test prompt",
		Worktree:   t.TempDir(),
		FullMode:   false,
		WindowGate: gate,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", res.InputTokens)
	}
	if !strings.Contains(res.TextOutput, "Done") {
		t.Errorf("expected text output to contain 'Done', got: %q", res.TextOutput)
	}
}

func TestRunner_OutsideWindow(t *testing.T) {
	r := claude.NewRunnerWithExec(echoRunner(""))
	gate := &fakeGate{allow: false}
	_, err := r.Run(context.Background(), claude.RunInput{
		Prompt:     "test",
		Worktree:   t.TempDir(),
		WindowGate: gate,
	})
	if err == nil {
		t.Fatal("expected ErrOutsideActiveWindow")
	}
	if err != domain.ErrOutsideActiveWindow {
		t.Fatalf("expected ErrOutsideActiveWindow, got: %v", err)
	}
}

func TestRunner_RateLimit(t *testing.T) {
	rateLimitNDJSON := `{"type":"system.init","apiKeySource":"none"}
{"type":"rate_limit_event","rate_limit_info":{"status":"blocked","resetsAt":1745000000,"rateLimitType":"five_hour"}}`

	r := claude.NewRunnerWithExec(echoRunner(rateLimitNDJSON))
	gate := &fakeGate{allow: true}
	_, err := r.Run(context.Background(), claude.RunInput{
		Prompt:     "test",
		Worktree:   t.TempDir(),
		WindowGate: gate,
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limited error, got: %v", err)
	}
}

func TestRunner_NoWindowGate(t *testing.T) {
	r := claude.NewRunnerWithExec(echoRunner(successNDJSON()))
	res, err := r.Run(context.Background(), claude.RunInput{
		Prompt:   "test",
		Worktree: t.TempDir(),
		// WindowGate is nil — should not panic
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected result, got nil")
	}
}
