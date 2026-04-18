// Package claude provides the Claude CLI runner and related utilities.
package claude

import (
	"fmt"
	"log/slog"
	"syscall"
	"time"
)

// Canceller sends signals to a process group to stop a running Claude task.
type Canceller interface {
	Cancel(pgid int) error
}

// ProcessCanceller implements Canceller via SIGTERM + SIGKILL.
type ProcessCanceller struct {
	GraceTimeout time.Duration
}

// NewProcessCanceller creates a ProcessCanceller with a 5-second grace period.
func NewProcessCanceller() *ProcessCanceller {
	return &ProcessCanceller{GraceTimeout: 5 * time.Second}
}

// Cancel sends SIGTERM to the process group, waits GraceTimeout, then SIGKILL.
func (c *ProcessCanceller) Cancel(pgid int) error {
	if pgid <= 0 {
		return fmt.Errorf("invalid pgid: %d", pgid)
	}

	slog.Info("canceller: sending SIGTERM", "pgid", pgid)
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		if !isNoSuchProcess(err) {
			return fmt.Errorf("SIGTERM pgid %d: %w", pgid, err)
		}
		return nil // process already gone
	}

	// Wait for the process to exit gracefully.
	deadline := time.Now().Add(c.GraceTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if !processGroupExists(pgid) {
			slog.Info("canceller: process group exited after SIGTERM", "pgid", pgid)
			return nil
		}
	}

	slog.Warn("canceller: grace period expired, sending SIGKILL", "pgid", pgid)
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		if !isNoSuchProcess(err) {
			return fmt.Errorf("SIGKILL pgid %d: %w", pgid, err)
		}
	}
	return nil
}

func processGroupExists(pgid int) bool {
	// Use signal 0 to probe the process group.
	err := syscall.Kill(-pgid, 0)
	return err == nil
}

func isNoSuchProcess(err error) bool {
	return err == syscall.ESRCH
}
