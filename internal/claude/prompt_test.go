package claude_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gs97ahn/scheduled-dev-agent/internal/claude"
)

func TestRenderPrompt_Feature(t *testing.T) {
	dir := t.TempDir()
	tmpl := `Hello {{.Repo}} issue #{{.Issue.Number}} branch {{.Branch}}`
	if err := os.WriteFile(filepath.Join(dir, "feature.tmpl"), []byte(tmpl), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := claude.RenderPrompt(dir, "feature", claude.PromptData{
		Repo:   "owner/repo",
		Branch: "claude/issue-42",
		Issue:  claude.IssueCtx{Number: 42},
	})
	if err != nil {
		t.Fatalf("RenderPrompt error: %v", err)
	}
	if !strings.Contains(result, "owner/repo") {
		t.Error("expected repo in rendered prompt")
	}
	if !strings.Contains(result, "42") {
		t.Error("expected issue number in rendered prompt")
	}
}

func TestRenderPrompt_MissingTemplate(t *testing.T) {
	_, err := claude.RenderPrompt("/nonexistent", "feature", claude.PromptData{})
	if err == nil {
		t.Error("expected error for missing template file")
	}
}

func TestRenderPrompt_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "feature.tmpl"), []byte("{{.BadSyntax"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := claude.RenderPrompt(dir, "feature", claude.PromptData{})
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

// TestRenderPrompt_NoHTMLEscape verifies C3 fix: text/template is used instead of
// html/template, so <, >, & in the issue body must not be escaped.
func TestRenderPrompt_NoHTMLEscape(t *testing.T) { // ADDED
	dir := t.TempDir()
	// Template that references Issue.Body directly (mirrors real templates).
	tmpl := `body:
<user_issue_body>
{{.Issue.Body}}
</user_issue_body>`
	if err := os.WriteFile(filepath.Join(dir, "feature.tmpl"), []byte(tmpl), 0o600); err != nil {
		t.Fatal(err)
	}

	specialBody := `<script>alert("xss")</script> & "quotes" -- comment`
	result, err := claude.RenderPrompt(dir, "feature", claude.PromptData{
		Issue: claude.IssueCtx{Body: specialBody},
	})
	if err != nil {
		t.Fatalf("RenderPrompt error: %v", err)
	}

	// Must contain the raw special characters, not HTML-escaped versions.
	if !strings.Contains(result, "<script>") {
		t.Errorf("C3: expected raw <script> in output, got escaped or missing: %q", result)
	}
	if !strings.Contains(result, "&") && strings.Contains(specialBody, "&") {
		t.Errorf("C3: expected raw & in output, got escaped: %q", result)
	}
	if strings.Contains(result, "&lt;") || strings.Contains(result, "&gt;") || strings.Contains(result, "&amp;") {
		t.Errorf("C3: html/template escape sequences found in output — text/template not used: %q", result)
	}

	// Must contain boundary markers.
	if !strings.Contains(result, "<user_issue_body>") {
		t.Errorf("C3: expected <user_issue_body> boundary marker in output")
	}
	if !strings.Contains(result, "</user_issue_body>") {
		t.Errorf("C3: expected </user_issue_body> boundary marker in output")
	}
}

// TestRenderPrompt_BoundaryMarkerInRealTemplate verifies that the real feature.tmpl
// file (loaded from ../../prompts/) contains the user_issue_body boundary.
func TestRenderPrompt_BoundaryMarkerInRealTemplate(t *testing.T) { // ADDED
	// Resolve path relative to this test file's package.
	promptsDir := "../../prompts"
	result, err := claude.RenderPrompt(promptsDir, "feature", claude.PromptData{
		Repo:   "owner/repo",
		Branch: "claude/issue-1",
		Issue:  claude.IssueCtx{Number: 1, Title: "test", Body: "hello <world> & more"},
	})
	if err != nil {
		t.Fatalf("RenderPrompt with real template error: %v", err)
	}
	if !strings.Contains(result, "<user_issue_body>") {
		t.Errorf("C3: real feature.tmpl missing <user_issue_body> boundary marker")
	}
	if strings.Contains(result, "&lt;") {
		t.Errorf("C3: real feature.tmpl HTML-escaping issue body — text/template not used")
	}
}
