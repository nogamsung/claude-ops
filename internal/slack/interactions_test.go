package slack_test

import (
	"context"
	"testing"

	"github.com/gs97ahn/scheduled-dev-agent/internal/slack"
)

type fakeCanceller struct {
	cancelled []string
}

func (c *fakeCanceller) CancelTask(_ context.Context, id string) error {
	c.cancelled = append(c.cancelled, id)
	return nil
}

func TestHandleInteraction_StopTask(t *testing.T) {
	payload := `{"type":"block_actions","actions":[{"action_id":"stop_task","value":"task:my-task-123"}]}`
	canceller := &fakeCanceller{}

	if err := slack.HandleInteraction(context.Background(), payload, canceller); err != nil {
		t.Fatalf("HandleInteraction error: %v", err)
	}
	if len(canceller.cancelled) != 1 || canceller.cancelled[0] != "my-task-123" {
		t.Errorf("expected my-task-123 to be cancelled, got %v", canceller.cancelled)
	}
}

func TestHandleInteraction_UnknownAction(t *testing.T) {
	payload := `{"type":"block_actions","actions":[{"action_id":"unknown_action","value":"something"}]}`
	canceller := &fakeCanceller{}

	if err := slack.HandleInteraction(context.Background(), payload, canceller); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(canceller.cancelled) != 0 {
		t.Error("expected no cancellations for unknown action")
	}
}

func TestHandleInteraction_InvalidJSON(t *testing.T) {
	if err := slack.HandleInteraction(context.Background(), "not-json", &fakeCanceller{}); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestHandleInteraction_EmptyTaskID(t *testing.T) {
	payload := `{"type":"block_actions","actions":[{"action_id":"stop_task","value":"task:"}]}`
	err := slack.HandleInteraction(context.Background(), payload, &fakeCanceller{})
	if err == nil {
		t.Error("expected error for empty task ID")
	}
}
