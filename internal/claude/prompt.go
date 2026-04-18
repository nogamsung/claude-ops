package claude

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template" // MODIFIED: was html/template — html/template escapes <, >, & which corrupts Claude prompts
)

// IssueCtx holds issue metadata passed to prompt templates.
type IssueCtx struct {
	Number int
	Title  string
	Body   string
	Labels []string
	Author string
	URL    string
}

// PromptData is the full context available inside all prompt templates.
type PromptData struct {
	Repo       string // "owner/name"
	Issue      IssueCtx
	Branch     string // e.g. "claude/issue-123"
	BaseBranch string // e.g. "main"
	Worktree   string // absolute path
	TaskType   string // "feature" | "security" | "performance"
}

// RenderPrompt loads and renders the template for the given task type.
// promptsDir is the directory containing {feature,security,performance}.tmpl files.
func RenderPrompt(promptsDir, taskType string, data PromptData) (string, error) {
	tmplName := taskType + ".tmpl"
	tmplPath := filepath.Join(promptsDir, tmplName)

	content, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("read prompt template %q: %w", tmplPath, err)
	}

	tmpl, err := template.New(tmplName).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse prompt template %q: %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute prompt template: %w", err)
	}
	return buf.String(), nil
}
