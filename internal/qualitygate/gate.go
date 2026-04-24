// Package qualitygate runs per-repo verification commands (lint/test/build)
// against a task's worktree before a PR is opened. Any failure aborts PR
// creation so broken code never reaches review.
package qualitygate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Result summarises a quality-gate run.
//
// FailedCommand is empty on success. OutputTail holds the last N lines of
// combined stdout+stderr from the failing command (for Slack notification
// bodies and task.StderrTail). ExitCode is -1 for non-exit errors (timeout,
// spawn failure).
type Result struct {
	Passed        bool
	FailedCommand string
	ExitCode      int
	OutputTail    string
	Duration      time.Duration
}

// CommandRunner abstracts exec for testing. Implementations must honor ctx
// cancellation and return a Process handle so Gate can escalate signals.
type CommandRunner interface {
	Run(ctx context.Context, dir, cmdline string, output io.Writer) (exitCode int, err error)
}

// ShellRunner runs each command through `sh -c <cmdline>`. This matches how
// config.yaml expresses commands ("go test ./...") and keeps the Gate free of
// token-parsing edge cases.
type ShellRunner struct {
	// KillGrace is how long to wait between SIGTERM and SIGKILL on timeout.
	// Default 5s (per issue #11 spec).
	KillGrace time.Duration
}

// Run executes cmdline in dir, streaming combined stdout+stderr to output.
// On ctx timeout it sends SIGTERM then SIGKILL after KillGrace.
func (r *ShellRunner) Run(ctx context.Context, dir, cmdline string, output io.Writer) (int, error) {
	grace := r.KillGrace
	if grace <= 0 {
		grace = 5 * time.Second
	}

	cmd := exec.Command("sh", "-c", cmdline)
	cmd.Dir = dir
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = procAttrNewSession()

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("spawn: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			return 0, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	case <-ctx.Done():
		// SIGTERM → wait grace → SIGKILL.
		_ = signalProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(grace):
			_ = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		return -1, fmt.Errorf("timeout: %w", ctx.Err())
	}
}

// Gate runs a sequence of shell commands, aborting at the first failure.
type Gate struct {
	runner   CommandRunner
	tailSize int
}

// NewGate constructs a Gate that runs commands through runner. tailSize caps
// how many trailing lines of output are kept for the failure report.
func NewGate(runner CommandRunner, tailSize int) *Gate {
	if tailSize <= 0 {
		tailSize = 50
	}
	return &Gate{runner: runner, tailSize: tailSize}
}

// Run executes each command in order. timeout applies per-command — not
// total — because each command already has its own expected duration and a
// shared deadline would make early commands starve the later ones.
// Returns on first failure; remaining commands are skipped.
func (g *Gate) Run(ctx context.Context, dir string, commands []string, timeout time.Duration) Result {
	start := time.Now()
	if len(commands) == 0 {
		return Result{Passed: true, Duration: time.Since(start)}
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	for _, cmdline := range commands {
		cmdline = strings.TrimSpace(cmdline)
		if cmdline == "" {
			continue
		}
		var buf bytes.Buffer
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		exitCode, err := g.runner.Run(cmdCtx, dir, cmdline, &buf)
		cancel()
		if err != nil || exitCode != 0 {
			return Result{
				Passed:        false,
				FailedCommand: cmdline,
				ExitCode:      exitCode,
				OutputTail:    tailLines(buf.String(), g.tailSize),
				Duration:      time.Since(start),
			}
		}
	}
	return Result{Passed: true, Duration: time.Since(start)}
}

// tailLines returns the last n lines of s (best-effort, no regex). Preserves
// the terminator so Slack messages render each line on its own row.
func tailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
