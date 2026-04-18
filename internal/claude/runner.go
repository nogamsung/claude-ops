package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/claude/stream"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// WindowGate is a minimal interface so the runner can re-check the active window.
type WindowGate interface {
	AllowNow(now time.Time, fullMode bool) bool
}

// RunInput holds all parameters for a single claude CLI invocation.
type RunInput struct {
	Prompt     string
	Worktree   string
	SessionID  string // optional: for resuming a session
	FullMode   bool
	WindowGate WindowGate
}

// RunResult summarises the outcome of a claude CLI invocation.
type RunResult struct {
	TextOutput            string
	InputTokens           int
	OutputTokens          int
	CacheReadInputTokens  int
	TotalCostUSD          float64
	DurationMS            int64
	ExitCode              int
	StderrTail            string
}

// ExecCommandFunc is a hook for injecting a fake exec.Cmd in tests.
type ExecCommandFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

// Clock abstracts time.Now() to allow fake time injection in tests.
type Clock interface { // ADDED
	Now() time.Time
}

// realClock implements Clock using the real system time.
type realClock struct{} // ADDED

func (realClock) Now() time.Time { return time.Now() } // ADDED

// Runner executes the Claude CLI and streams its output.
type Runner struct {
	execCommand ExecCommandFunc
	clock       Clock // ADDED
}

// NewRunner creates a Runner using the real exec.CommandContext.
func NewRunner() *Runner {
	return &Runner{execCommand: exec.CommandContext, clock: realClock{}} // MODIFIED
}

// NewRunnerWithExec creates a Runner with a custom exec function (for testing).
func NewRunnerWithExec(fn ExecCommandFunc) *Runner {
	return &Runner{execCommand: fn, clock: realClock{}} // MODIFIED
}

// NewRunnerWithExecAndClock creates a Runner with a custom exec function and clock (for testing).
func NewRunnerWithExecAndClock(fn ExecCommandFunc, clk Clock) *Runner { // ADDED
	return &Runner{execCommand: fn, clock: clk} // ADDED
} // ADDED

// Run executes the Claude CLI for the given input and returns the result.
// It re-checks the window gate immediately before spawning (double-gate).
func (r *Runner) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	// Double-gate: verify window before spawning.
	if input.WindowGate != nil {
		if !input.WindowGate.AllowNow(r.clock.Now(), input.FullMode) { // MODIFIED: was time.Now()
			return nil, domain.ErrOutsideActiveWindow
		}
	}

	args := buildArgs(input.Prompt, input.SessionID)
	cmd := r.execCommand(ctx, "claude", args...)
	cmd.Dir = input.Worktree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Capture stdout for stream parsing.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Capture stderr for out-of-band error signals.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		pgid = cmd.Process.Pid // fallback
	}
	slog.Info("claude runner: process started", "pid", cmd.Process.Pid, "pgid", pgid)

	// Drain stderr asynchronously.
	stderrCh := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(stderr)
		stderrCh <- tail(string(b), 150)
	}()

	// Parse stdout stream.
	signalCh := make(chan stream.Signal, 64)
	parseErrCh := make(chan error, 1)
	go func() {
		parseErrCh <- stream.ParseStream(ctx, stdout, signalCh)
	}()

	// Collect text and usage from signals.
	var textParts []string
	var usage *stream.ResultUsage
	var result *stream.ResultEvent

	collectDone := make(chan struct{})
	go func() {
		defer close(collectDone)
		for sig := range signalCh {
			switch sig.Kind {
			case stream.SignalText:
				textParts = append(textParts, sig.TextChunk)
			case stream.SignalResult:
				usage = sig.Usage
				result = sig.Result
			}
		}
	}()

	parseErr := <-parseErrCh
	close(signalCh)
	<-collectDone

	stderrText := <-stderrCh

	// Check for session-missing indicators in stderr.
	if parseErr == nil && (strings.Contains(stderrText, "please login") ||
		strings.Contains(stderrText, "not logged in")) {
		parseErr = domain.ErrSessionMissing
	}

	waitErr := cmd.Wait()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	if parseErr != nil {
		return &RunResult{
			ExitCode:   exitCode,
			StderrTail: stderrText,
		}, parseErr
	}

	res := &RunResult{
		TextOutput: strings.Join(textParts, ""),
		ExitCode:   exitCode,
		StderrTail: stderrText,
	}
	if usage != nil {
		res.InputTokens = usage.InputTokens
		res.OutputTokens = usage.OutputTokens
		res.CacheReadInputTokens = usage.CacheReadInputTokens
	}
	if result != nil {
		res.TotalCostUSD = result.TotalCostUSD
		res.DurationMS = result.DurationMS
	}

	if exitCode != 0 && waitErr != nil {
		return res, fmt.Errorf("claude exited with code %d: %w", exitCode, waitErr)
	}

	return res, nil
}

func buildArgs(prompt, sessionID string) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "acceptEdits",
		"--allowedTools", "Bash,Edit,Read,Write,Glob,Grep",
		"--disallowedTools", "WebFetch,WebSearch",
	}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	return args
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
