package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ActionIDStopTask is the action_id emitted by the Slack "Stop" button.
const ActionIDStopTask = "stop_task"

// TaskCanceller can cancel a task by ID.
type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

// InteractionPayload is the parsed Slack interaction payload.
type InteractionPayload struct {
	Type    string `json:"type"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
}

// HandleInteraction processes a URL-decoded Slack interaction payload.
func HandleInteraction(ctx context.Context, rawPayload string, canceller TaskCanceller) error {
	var payload InteractionPayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		return fmt.Errorf("parse interaction payload: %w", err)
	}

	for _, action := range payload.Actions {
		if action.ActionID != ActionIDStopTask {
			continue
		}
		taskID := strings.TrimPrefix(action.Value, "task:")
		if taskID == "" {
			return fmt.Errorf("empty task ID in stop action")
		}
		if err := canceller.CancelTask(ctx, taskID); err != nil {
			return fmt.Errorf("cancel task %q: %w", taskID, err)
		}
	}
	return nil
}
