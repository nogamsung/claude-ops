// Package config loads and validates application configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config is the root application configuration.
type Config struct {
	Runtime   RuntimeConfig   `mapstructure:"runtime"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Limits    LimitsConfig    `mapstructure:"limits"`
	GitHub    GitHubConfig    `mapstructure:"github"`
	Slack     SlackConfig     `mapstructure:"slack"`
}

// LimitsConfig caps how many tasks may run per day/week and how the buckets reset.
// Either DailyMaxTasks or WeeklyMaxTasks may be 0; defaults derive the missing
// one from the other (daily = ceil(weekly/7); weekly = daily*7).
type LimitsConfig struct {
	DailyMaxTasks  int    `mapstructure:"daily_max_tasks"`
	WeeklyMaxTasks int    `mapstructure:"weekly_max_tasks"`
	WeekStartsOn   string `mapstructure:"week_starts_on"` // mon|sun
	ResetTZ        string `mapstructure:"reset_tz"`       // IANA tz, e.g. "Asia/Seoul"
}

// RuntimeConfig holds server and storage settings.
type RuntimeConfig struct {
	HTTPBindAddr string        `mapstructure:"http_bind_addr"`
	DBPath       string        `mapstructure:"db_path"`
	LogLevel     string        `mapstructure:"log_level"`
	TickInterval time.Duration `mapstructure:"tick_interval"`
	WorktreeRoot string        `mapstructure:"worktree_root"`
	PromptsDir   string        `mapstructure:"prompts_dir"`
}

// SchedulerConfig defines active time windows and maintenance tasks.
type SchedulerConfig struct {
	ActiveWindows    []WindowConfig          `mapstructure:"active_windows"`
	MaintenanceTasks []MaintenanceTaskConfig `mapstructure:"maintenance_tasks"`
}

// MaintenanceTaskConfig describes a cron-triggered maintenance task.
type MaintenanceTaskConfig struct {
	Name           string       `mapstructure:"name"`
	Cron           string       `mapstructure:"cron"`
	Repo           string       `mapstructure:"repo"`
	PromptTemplate string       `mapstructure:"prompt_template"`
	Labels         []string     `mapstructure:"labels"`
	BudgetSubCap   SubCapConfig `mapstructure:"budget_sub_cap"`
}

// SubCapConfig constrains how many times a maintenance task may run within a period.
type SubCapConfig struct {
	Daily  int `mapstructure:"daily"`
	Weekly int `mapstructure:"weekly"`
}

// WindowConfig is a single active window definition from YAML.
type WindowConfig struct {
	Days  []string `mapstructure:"days"`
	Start string   `mapstructure:"start"`
	End   string   `mapstructure:"end"`
	TZ    string   `mapstructure:"tz"`
}

// GitHubConfig holds GitHub integration settings.
type GitHubConfig struct {
	PollInterval time.Duration `mapstructure:"poll_interval"`
	Repos        []RepoConfig  `mapstructure:"repos"`
}

// RepoConfig is a single allowlisted repository.
type RepoConfig struct {
	Name          string       `mapstructure:"name"`
	DefaultBranch string       `mapstructure:"default_branch"`
	Labels        []string     `mapstructure:"labels"`
	Reviewers     []string     `mapstructure:"reviewers"`
	Checks        ChecksConfig `mapstructure:"checks"`
}

// ChecksConfig flags which additional analysis types are enabled per repo
// and — via Commands — defines post-task quality-gate commands that must
// succeed before a PR is opened.
type ChecksConfig struct {
	Security bool          `mapstructure:"security"`
	Perf     bool          `mapstructure:"perf"`
	Commands []string      `mapstructure:"commands"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// SlackConfig holds Slack integration settings.
type SlackConfig struct {
	ChannelID     string `mapstructure:"channel_id"`
	MentionUserID string `mapstructure:"mention_user_id"`
}

// Env holds sensitive values loaded from environment variables.
type Env struct {
	GitHubToken        string
	SlackBotToken      string
	SlackSigningSecret string
}

// Load reads the YAML config file, loads .env from the working directory (if present),
// and returns a fully populated Config.
func Load(path string) (*Config, error) {
	// Best-effort .env load; ignore error if file doesn't exist.
	_ = godotenv.Load()

	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("runtime.http_bind_addr", "127.0.0.1:8787")
	v.SetDefault("runtime.db_path", "data/agent.db")
	v.SetDefault("runtime.log_level", "info")
	v.SetDefault("runtime.tick_interval", "30s")
	v.SetDefault("runtime.worktree_root", ".worktrees")
	v.SetDefault("runtime.prompts_dir", "prompts")
	v.SetDefault("github.poll_interval", "60s")
	v.SetDefault("limits.daily_max_tasks", 5)
	v.SetDefault("limits.weekly_max_tasks", 0) // 0 → derived = daily * 7
	v.SetDefault("limits.week_starts_on", "mon")
	v.SetDefault("limits.reset_tz", "Asia/Seoul")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Override from env where applicable.
	if addr := os.Getenv("HTTP_BIND_ADDR"); addr != "" {
		cfg.Runtime.HTTPBindAddr = addr
	}
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		cfg.Runtime.DBPath = dbPath
	}

	return &cfg, nil
}

// LoadEnv loads sensitive settings from environment variables.
func LoadEnv() (*Env, error) {
	_ = godotenv.Load()

	env := &Env{
		GitHubToken:        os.Getenv("GITHUB_TOKEN"),
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
	}
	return env, nil
}
