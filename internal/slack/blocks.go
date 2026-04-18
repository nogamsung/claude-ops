package slack

import "fmt"

// Block Kit structure types.

// Message is a top-level Slack message with blocks.
type Message struct {
	Blocks []Block `json:"blocks"`
}

// Block represents a Slack Block Kit block.
type Block struct {
	Type     string    `json:"type"`
	Text     *TextObj  `json:"text,omitempty"`
	Elements []Element `json:"elements,omitempty"`
}

// TextObj is a rich text object.
type TextObj struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Element is a Slack Block Kit interactive element.
type Element struct {
	Type     string   `json:"type"`
	Text     *TextObj `json:"text,omitempty"`
	Style    string   `json:"style,omitempty"`
	Value    string   `json:"value,omitempty"`
	URL      string   `json:"url,omitempty"`
	ActionID string   `json:"action_id,omitempty"`
}

// BuildStarted builds a "Task started" Block Kit message.
func BuildStarted(taskID, repoFullName string, issueNumber int, issueURL string) Message {
	return Message{
		Blocks: []Block{
			{
				Type: "section",
				Text: &TextObj{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":rocket: *Task started* — `%s#%d`", repoFullName, issueNumber),
				},
			},
			{
				Type: "actions",
				Elements: []Element{
					{
						Type:     "button",
						Text:     &TextObj{Type: "plain_text", Text: "Stop"},
						Style:    "danger",
						Value:    "task:" + taskID,
						ActionID: "stop_task",
					},
					{
						Type: "button",
						Text: &TextObj{Type: "plain_text", Text: "View Issue"},
						URL:  issueURL,
					},
				},
			},
		},
	}
}

// BuildDone builds a "Task done" Block Kit message.
func BuildDone(repoFullName string, issueNumber int, prURL string, prNumber int) Message {
	prText := fmt.Sprintf("View PR #%d", prNumber)
	return Message{
		Blocks: []Block{
			{
				Type: "section",
				Text: &TextObj{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":white_check_mark: *Done* — `%s#%d` → PR #%d", repoFullName, issueNumber, prNumber),
				},
			},
			{
				Type: "actions",
				Elements: []Element{
					{
						Type:  "button",
						Text:  &TextObj{Type: "plain_text", Text: prText},
						URL:   prURL,
						Style: "primary",
					},
				},
			},
		},
	}
}

// BuildFailed builds a "Task failed" Block Kit message.
func BuildFailed(repoFullName string, issueNumber int, errMsg string) Message {
	if len(errMsg) > 500 {
		errMsg = errMsg[:500] + "…"
	}
	return Message{
		Blocks: []Block{
			{
				Type: "section",
				Text: &TextObj{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":x: *Task failed* — `%s#%d`\n```%s```", repoFullName, issueNumber, errMsg),
				},
			},
		},
	}
}

// BuildCancelled builds a "Task cancelled" Block Kit message.
func BuildCancelled(repoFullName string, issueNumber int) Message {
	return Message{
		Blocks: []Block{
			{
				Type: "section",
				Text: &TextObj{
					Type: "mrkdwn",
					Text: fmt.Sprintf(":no_entry: *Cancelled* — `%s#%d`", repoFullName, issueNumber),
				},
			},
		},
	}
}

// BuildModeChange builds a mode change notification Block Kit message.
func BuildModeChange(enabled bool) Message {
	status := "disabled"
	emoji := ":zzz:"
	if enabled {
		status = "enabled"
		emoji = ":zap:"
	}
	return Message{
		Blocks: []Block{
			{
				Type: "section",
				Text: &TextObj{
					Type: "mrkdwn",
					Text: fmt.Sprintf("%s *Full usage mode %s*", emoji, status),
				},
			},
		},
	}
}
