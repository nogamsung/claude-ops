package slack_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gs97ahn/claude-ops/internal/slack"
)

func TestBuildStarted_ContainsStopButton(t *testing.T) {
	msg := slack.BuildStarted("task-123", "owner/repo", 42, "https://github.com/owner/repo/issues/42")
	b, _ := json.Marshal(msg)
	raw := string(b)

	if !strings.Contains(raw, "stop_task") {
		t.Error("started message should contain stop_task action_id")
	}
	if !strings.Contains(raw, "task-123") {
		t.Error("started message should contain task ID")
	}
	if !strings.Contains(raw, "owner/repo") {
		t.Error("started message should contain repo name")
	}
}

func TestBuildDone_ContainsPRLink(t *testing.T) {
	msg := slack.BuildDone("owner/repo", 42, "https://github.com/owner/repo/pull/10", 10)
	b, _ := json.Marshal(msg)
	raw := string(b)

	if !strings.Contains(raw, "PR #10") {
		t.Error("done message should mention PR number")
	}
	if !strings.Contains(raw, "https://github.com/owner/repo/pull/10") {
		t.Error("done message should contain PR URL")
	}
}

func TestBuildFailed_TruncatesLongMessage(t *testing.T) {
	longMsg := strings.Repeat("x", 1000)
	msg := slack.BuildFailed("owner/repo", 1, longMsg)
	b, _ := json.Marshal(msg)

	if len(b) > 3000 {
		t.Errorf("failed message should truncate long error messages, got %d bytes", len(b))
	}
}

func TestBuildCancelled_ContainsNoEntry(t *testing.T) {
	msg := slack.BuildCancelled("owner/repo", 5)
	b, _ := json.Marshal(msg)

	if !strings.Contains(string(b), ":no_entry:") {
		t.Error("cancelled message should contain :no_entry: emoji")
	}
}

func TestBuildModeChange_Enabled(t *testing.T) {
	msg := slack.BuildModeChange(true)
	b, _ := json.Marshal(msg)

	if !strings.Contains(string(b), "enabled") {
		t.Error("mode change message should say enabled")
	}
}

func TestBuildModeChange_Disabled(t *testing.T) {
	msg := slack.BuildModeChange(false)
	b, _ := json.Marshal(msg)

	if !strings.Contains(string(b), "disabled") {
		t.Error("mode change message should say disabled")
	}
}
