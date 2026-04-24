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

func TestValidate_MaintenanceTasks_Valid(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
  maintenance_tasks:
    - name: "daily-dep-update"
      cron: "0 2 * * *"
      repo: "owner/repo"
      prompt_template: "maintenance/dep-update.tmpl"
      labels: ["chore"]
      budget_sub_cap:
        daily: 1
        weekly: 3
github:
  poll_interval: "60s"
  repos:
    - name: "owner/repo"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err = cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if len(cfg.Scheduler.MaintenanceTasks) != 1 {
		t.Fatalf("expected 1 maintenance task, got %d", len(cfg.Scheduler.MaintenanceTasks))
	}
	mt := cfg.Scheduler.MaintenanceTasks[0]
	if mt.Name != "daily-dep-update" {
		t.Errorf("expected name %q, got %q", "daily-dep-update", mt.Name)
	}
	if mt.BudgetSubCap.Daily != 1 {
		t.Errorf("expected daily sub-cap 1, got %d", mt.BudgetSubCap.Daily)
	}
	if mt.BudgetSubCap.Weekly != 3 {
		t.Errorf("expected weekly sub-cap 3, got %d", mt.BudgetSubCap.Weekly)
	}
}

func TestValidate_MaintenanceTasks_InvalidCron(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
  maintenance_tasks:
    - name: "bad-cron"
      cron: "not-a-cron"
      repo: "owner/repo"
      prompt_template: "maintenance/dep-update.tmpl"
github:
  poll_interval: "60s"
  repos:
    - name: "owner/repo"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid cron spec")
	}
}

func TestValidate_MaintenanceTasks_RepoNotInAllowlist(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
  maintenance_tasks:
    - name: "orphan-task"
      cron: "0 2 * * *"
      repo: "unknown/repo"
      prompt_template: "maintenance/dep-update.tmpl"
github:
  poll_interval: "60s"
  repos:
    - name: "owner/repo"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for repo not in allowlist")
	}
}

func TestValidate_MaintenanceTasks_DuplicateName(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
  maintenance_tasks:
    - name: "same-name"
      cron: "0 2 * * *"
      repo: "owner/repo"
      prompt_template: "maintenance/dep-update.tmpl"
    - name: "same-name"
      cron: "0 3 * * *"
      repo: "owner/repo"
      prompt_template: "maintenance/dep-update.tmpl"
github:
  poll_interval: "60s"
  repos:
    - name: "owner/repo"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for duplicate maintenance task name")
	}
}

func TestValidate_MaintenanceTasks_MissingPromptTemplate(t *testing.T) {
	yaml := `
runtime:
  tick_interval: "30s"
scheduler:
  active_windows:
    - days: ["mon"]
      start: "09:00"
      end: "18:00"
      tz: "UTC"
  maintenance_tasks:
    - name: "no-template"
      cron: "0 2 * * *"
      repo: "owner/repo"
      prompt_template: ""
github:
  poll_interval: "60s"
  repos:
    - name: "owner/repo"
      default_branch: "main"
`
	path := writeTemp(t, yaml)
	cfg, _ := config.Load(path)
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing prompt_template")
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
