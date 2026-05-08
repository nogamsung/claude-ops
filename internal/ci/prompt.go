package ci

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// FilePromptRenderer loads `<promptsDir>/ci-fix.tmpl` once at construction
// and renders it per task. Implements ci.PromptRenderer.
type FilePromptRenderer struct {
	tmpl *template.Template
}

// NewFilePromptRenderer reads the template from disk. Returns an error
// (rather than logging) so cmd/main can fail fast at startup if the
// template is missing — better than silently rendering empty prompts.
func NewFilePromptRenderer(promptsDir string) (*FilePromptRenderer, error) {
	path := filepath.Join(promptsDir, "ci-fix.tmpl")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ci-fix prompt %q: %w", path, err)
	}
	tmpl, err := template.New("ci-fix").Parse(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse ci-fix prompt: %w", err)
	}
	return &FilePromptRenderer{tmpl: tmpl}, nil
}

// RenderCIFix substitutes PromptData into the template.
func (r *FilePromptRenderer) RenderCIFix(data PromptData) (string, error) {
	if r == nil || r.tmpl == nil {
		return "", nil
	}
	var buf bytes.Buffer
	if err := r.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render ci-fix: %w", err)
	}
	return buf.String(), nil
}
