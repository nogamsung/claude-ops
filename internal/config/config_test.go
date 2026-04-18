package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gs97ahn/claude-ops/internal/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	yaml := `
runtime:
  http_bind_addr: "127.0.0.1:8787"
  db_path: "data/agent.db"
  log_level: "info"
  tick_interval: "30s"
  worktree_root: ".worktrees"
  prompts_dir: "prompts"
scheduler:
  active_windows:
    - days: ["mon","tue","wed","thu","fri"]
      start: "09:00"
      end: "18:00"
      tz: "Asia/Seoul"
github:
  poll_interval: "60s"
  repos:
    - name: "gs97ahn/example"
      default_branch: "main"
      labels: ["claude-ops"]
      reviewers: ["gs97ahn"]
      checks:
        security: true
        perf: false
slack:
  channel_id: "C0XXXXXXX"
  mention_user_id: "U0XXXXXXX"
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Runtime.HTTPBindAddr != "127.0.0.1:8787" {
		t.Errorf("unexpected bind addr: %s", cfg.Runtime.HTTPBindAddr)
	}
	if len(cfg.GitHub.Repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(cfg.GitHub.Repos))
	}
	if cfg.GitHub.Repos[0].Name != "gs97ahn/example" {
		t.Errorf("unexpected repo name: %s", cfg.GitHub.Repos[0].Name)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestValidate_InvalidTZ(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "Invalid/Zone"
github:
  poll_interval: "60s"
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() should not fail on parse: %v", err)
	}
	if err = cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid timezone")
	}
}

func TestValidate_DuplicateRepo(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
github:
  poll_interval: "60s"
  repos:
    - name: "owner/repo"
      default_branch: "main"
    - name: "owner/repo"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err == nil {
		t.Error("expected duplicate repo validation error")
	}
}

func TestValidate_InvalidRepoFormat(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
github:
  poll_interval: "60s"
  repos:
    - name: "invalid-no-slash"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err == nil {
		t.Error("expected invalid repo name format error")
	}
}

func TestValidateEnv_MissingToken(t *testing.T) {
	env := &config.Env{}
	if err := env.ValidateEnv(); err == nil {
		t.Error("expected error for missing GITHUB_TOKEN")
	}
}

func TestValidateEnv_Valid(t *testing.T) {
	env := &config.Env{
		GitHubToken:        "ghp_test",
		SlackBotToken:      "xoxb_test",
		SlackSigningSecret: "secret",
	}
	if err := env.ValidateEnv(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
