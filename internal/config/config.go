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
	Runtime     RuntimeConfig     `mapstructure:"runtime"`
	Scheduler   SchedulerConfig   `mapstructure:"scheduler"`
	Concurrency ConcurrencyConfig `mapstructure:"concurrency"`
	CIFix       CIFixConfig       `mapstructure:"ci_fix"`
	Limits      LimitsConfig      `mapstructure:"limits"`
	GitHub      GitHubConfig      `mapstructure:"github"`
	Slack       SlackConfig       `mapstructure:"slack"`
}

// CIFixConfig controls the post-PR CI watcher loop. When Enabled is false
// the watcher does nothing and worker behavior is byte-for-byte identical
// to v1 (worktree GC + markDone clean-up runs immediately).
//
// MaxAttempts caps the parent→child fix-chain length. PollInterval drives
// `gh pr checks` polling cadence; PollTimeout abandons watching when
// checks never reach a terminal state. The dedup rule (migrations/000007
// ci_fix_dedup_key UNIQUE) guarantees at most one ci-fix child per
// (parent, head_sha) regardless of poll frequency.
type CIFixConfig struct {
	Enabled             bool          `mapstructure:"enabled"`
	MaxAttempts         int           `mapstructure:"max_attempts"`
	PollInterval        time.Duration `mapstructure:"poll_interval"`
	PollTimeout         time.Duration `mapstructure:"poll_timeout"`
	CommentOnExhaustion bool          `mapstructure:"comment_on_exhaustion"`
}

// ConcurrencyConfig controls the parallel-tasks worker pool. The default
// (max_parallel_tasks=1) keeps v1 byte-for-byte behavior; raise it only
// after running the spike acceptance gates in PRD §6.5.
//
// LeaseTimeout is how long a claimed-but-unfinished task may look alive
// before the periodic reclaimer marks it orphaned (worker died). Pick a
// value comfortably larger than the longest realistic task — 60m is the
// PRD-recommended default for ~30m average tasks. ReclaimInterval controls
// how often the safety-net runs (default 10m).
type ConcurrencyConfig struct {
	MaxParallelTasks int           `mapstructure:"max_parallel_tasks"`
	LeaseTimeout     time.Duration `mapstructure:"lease_timeout"`
	ReclaimInterval  time.Duration `mapstructure:"reclaim_interval"`
}

// LimitsConfig caps how many tasks may run per day/week and how the buckets reset.
// Either DailyMaxTasks or WeeklyMaxTasks may be 0; defaults derive the missing
// one from the other (daily = ceil(weekly/7); weekly = daily*7).
// DailyMaxCostUSD and WeeklyMaxCostUSD set cost thresholds for Slack warnings.
// A value of 0 means "no limit" (warnings disabled).
type LimitsConfig struct {
	DailyMaxTasks    int     `mapstructure:"daily_max_tasks"`
	WeeklyMaxTasks   int     `mapstructure:"weekly_max_tasks"`
	WeekStartsOn     string  `mapstructure:"week_starts_on"`      // mon|sun
	ResetTZ          string  `mapstructure:"reset_tz"`            // IANA tz, e.g. "Asia/Seoul"
	DailyMaxCostUSD  float64 `mapstructure:"daily_max_cost_usd"`  // 0 = disabled
	WeeklyMaxCostUSD float64 `mapstructure:"weekly_max_cost_usd"` // 0 = disabled
}

// RuntimeConfig holds server and storage settings.
type RuntimeConfig struct {
	HTTPBindAddr          string        `mapstructure:"http_bind_addr"`
	DB                    DBSettings    `mapstructure:"db"`
	LogLevel              string        `mapstructure:"log_level"`
	TickInterval          time.Duration `mapstructure:"tick_interval"`
	WorktreeRoot          string        `mapstructure:"worktree_root"`
	PromptsDir            string        `mapstructure:"prompts_dir"`
	WorktreeRetentionDays int           `mapstructure:"worktree_retention_days"`
}

// DBSettings configures the MySQL connection pool. The DSN must include
// `parseTime=true&charset=utf8mb4` (and ideally `loc=UTC`); NewDB validates this.
// Zero pool values fall back to library defaults.
type DBSettings struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
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
	GitHubToken         string
	GitHubWebhookSecret string
	SlackBotToken       string
	SlackSigningSecret  string
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
	v.SetDefault("runtime.db.dsn", "")
	v.SetDefault("runtime.db.max_open_conns", 0)
	v.SetDefault("runtime.db.max_idle_conns", 0)
	v.SetDefault("runtime.db.conn_max_lifetime", "5m")
	v.SetDefault("runtime.db.conn_max_idle_time", "1m")
	v.SetDefault("runtime.log_level", "info")
	v.SetDefault("runtime.tick_interval", "30s")
	v.SetDefault("runtime.worktree_root", ".worktrees")
	v.SetDefault("runtime.prompts_dir", "prompts")
	v.SetDefault("runtime.worktree_retention_days", 7)
	v.SetDefault("github.poll_interval", "60s")
	v.SetDefault("limits.daily_max_tasks", 5)
	v.SetDefault("limits.weekly_max_tasks", 0) // 0 → derived = daily * 7
	v.SetDefault("limits.week_starts_on", "mon")
	v.SetDefault("limits.reset_tz", "Asia/Seoul")
	v.SetDefault("concurrency.max_parallel_tasks", 1) // v1 byte-compat default
	v.SetDefault("concurrency.lease_timeout", "60m")
	v.SetDefault("concurrency.reclaim_interval", "10m")
	v.SetDefault("ci_fix.enabled", false) // opt-in until operator validates
	v.SetDefault("ci_fix.max_attempts", 2)
	v.SetDefault("ci_fix.poll_interval", "60s")
	v.SetDefault("ci_fix.poll_timeout", "30m")
	v.SetDefault("ci_fix.comment_on_exhaustion", true)

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
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		cfg.Runtime.DB.DSN = dsn
	}

	return &cfg, nil
}

// LoadEnv loads sensitive settings from environment variables.
func LoadEnv() (*Env, error) {
	_ = godotenv.Load()

	env := &Env{
		GitHubToken:         os.Getenv("GITHUB_TOKEN"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		SlackBotToken:       os.Getenv("SLACK_BOT_TOKEN"),
		SlackSigningSecret:  os.Getenv("SLACK_SIGNING_SECRET"),
	}
	return env, nil
}
