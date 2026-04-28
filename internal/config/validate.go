package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// Validate validates the Config and returns an error describing any issues.
func (c *Config) Validate() error {
	if err := c.validateWindows(); err != nil {
		return err
	}
	if err := c.validateRepos(); err != nil {
		return err
	}
	if c.Runtime.TickInterval <= 0 {
		return fmt.Errorf("runtime.tick_interval must be positive")
	}
	if c.GitHub.PollInterval <= 0 {
		return fmt.Errorf("github.poll_interval must be positive")
	}
	if err := c.validateLimits(); err != nil {
		return err
	}
	if err := c.validateMaintenanceTasks(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateMaintenanceTasks() error {
	// Build repo allowlist for O(1) lookup.
	repoSet := make(map[string]struct{}, len(c.GitHub.Repos))
	for _, r := range c.GitHub.Repos {
		repoSet[r.Name] = struct{}{}
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	seen := make(map[string]struct{})

	for i, mt := range c.Scheduler.MaintenanceTasks {
		name := strings.TrimSpace(mt.Name)
		if name == "" {
			return fmt.Errorf("scheduler.maintenance_tasks[%d]: name is required", i)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("scheduler.maintenance_tasks[%d]: duplicate name %q", i, name)
		}
		seen[name] = struct{}{}

		if mt.Cron == "" {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: cron is required", i, name)
		}
		if _, err := parser.Parse(mt.Cron); err != nil {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: invalid cron spec %q: %w", i, name, mt.Cron, err)
		}

		repo := strings.TrimSpace(mt.Repo)
		if repo == "" {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: repo is required", i, name)
		}
		if _, ok := repoSet[repo]; !ok {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: repo %q not in github.repos allowlist", i, name, repo)
		}

		if mt.PromptTemplate == "" {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: prompt_template is required", i, name)
		}

		if mt.BudgetSubCap.Daily < 0 {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: budget_sub_cap.daily must be >= 0", i, name)
		}
		if mt.BudgetSubCap.Weekly < 0 {
			return fmt.Errorf("scheduler.maintenance_tasks[%d] %q: budget_sub_cap.weekly must be >= 0", i, name)
		}
	}
	return nil
}

func (c *Config) validateLimits() error {
	if c.Limits.DailyMaxTasks < 0 {
		return fmt.Errorf("limits.daily_max_tasks must be >= 0")
	}
	if c.Limits.WeeklyMaxTasks < 0 {
		return fmt.Errorf("limits.weekly_max_tasks must be >= 0")
	}
	if c.Limits.WeekStartsOn != "" {
		if _, ok := weekStartMap[strings.ToLower(c.Limits.WeekStartsOn)]; !ok {
			return fmt.Errorf("limits.week_starts_on %q must be mon|tue|wed|thu|fri|sat|sun", c.Limits.WeekStartsOn)
		}
	}
	if c.Limits.ResetTZ != "" {
		if _, err := time.LoadLocation(c.Limits.ResetTZ); err != nil {
			return fmt.Errorf("limits.reset_tz %q: %w", c.Limits.ResetTZ, err)
		}
	}
	if c.Limits.DailyMaxTasks > 0 && c.Limits.WeeklyMaxTasks > 0 &&
		c.Limits.DailyMaxTasks > c.Limits.WeeklyMaxTasks {
		return fmt.Errorf("limits.daily_max_tasks (%d) must be <= weekly_max_tasks (%d)",
			c.Limits.DailyMaxTasks, c.Limits.WeeklyMaxTasks)
	}
	if c.Limits.DailyMaxCostUSD < 0 {
		return fmt.Errorf("limits.daily_max_cost_usd must be >= 0")
	}
	if c.Limits.WeeklyMaxCostUSD < 0 {
		return fmt.Errorf("limits.weekly_max_cost_usd must be >= 0")
	}
	return nil
}

// weekStartMap maps lowercase day abbreviation to time.Weekday for limits config.
var weekStartMap = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

// ResolvedLimits returns concrete daily/weekly caps with derived defaults applied:
//   - both zero → unlimited (both stay 0)
//   - only daily set → weekly = daily * 7
//   - only weekly set → daily = ceil(weekly / 7)
func (c *LimitsConfig) ResolvedLimits() (dailyMax, weeklyMax int, weekStart time.Weekday, tz *time.Location, err error) {
	dailyMax = c.DailyMaxTasks
	weeklyMax = c.WeeklyMaxTasks
	switch {
	case dailyMax == 0 && weeklyMax > 0:
		dailyMax = (weeklyMax + 6) / 7
	case weeklyMax == 0 && dailyMax > 0:
		weeklyMax = dailyMax * 7
	}

	startName := strings.ToLower(c.WeekStartsOn)
	if startName == "" {
		startName = "mon"
	}
	wd, ok := weekStartMap[startName]
	if !ok {
		return 0, 0, 0, nil, fmt.Errorf("invalid week_starts_on %q", c.WeekStartsOn)
	}
	weekStart = wd

	tzName := c.ResetTZ
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("load tz %q: %w", tzName, err)
	}
	tz = loc
	return dailyMax, weeklyMax, weekStart, tz, nil
}

func (c *Config) validateWindows() error {
	for i, wc := range c.Scheduler.ActiveWindows {
		w := &domain.ActiveWindow{
			Days:  wc.Days,
			Start: wc.Start,
			End:   wc.End,
			TZ:    wc.TZ,
		}
		if err := w.Validate(); err != nil {
			return fmt.Errorf("scheduler.active_windows[%d]: %w", i, err)
		}
	}
	return nil
}

func (c *Config) validateRepos() error {
	seen := make(map[string]struct{})
	for i, r := range c.GitHub.Repos {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			return fmt.Errorf("github.repos[%d]: name is required", i)
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("github.repos[%d]: name %q must be in owner/repo format", i, name)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("github.repos[%d]: duplicate repo %q", i, name)
		}
		seen[name] = struct{}{}
		if r.DefaultBranch == "" {
			c.GitHub.Repos[i].DefaultBranch = "main"
		}
		if len(r.Checks.Commands) > 0 {
			for j, cmd := range r.Checks.Commands {
				if strings.TrimSpace(cmd) == "" {
					return fmt.Errorf("github.repos[%d].checks.commands[%d]: command cannot be empty", i, j)
				}
			}
			if r.Checks.Timeout <= 0 {
				c.GitHub.Repos[i].Checks.Timeout = 5 * time.Minute
			}
		}
	}
	return nil
}

// ValidateEnv validates the Env struct for required fields.
func (e *Env) ValidateEnv() error {
	if e.GitHubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	if e.SlackBotToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN environment variable is required")
	}
	if e.SlackSigningSecret == "" {
		return fmt.Errorf("SLACK_SIGNING_SECRET environment variable is required")
	}
	if e.GitHubWebhookSecret == "" {
		slog.Warn("GITHUB_WEBHOOK_SECRET unset — /github/webhook endpoint will not be registered") // MODIFIED
	}
	return nil
}

// ActiveWindows converts the config window slice to domain.ActiveWindow slice.
func (c *Config) ActiveWindows() ([]*domain.ActiveWindow, error) {
	out := make([]*domain.ActiveWindow, 0, len(c.Scheduler.ActiveWindows))
	for i, wc := range c.Scheduler.ActiveWindows {
		w := &domain.ActiveWindow{
			Days:  wc.Days,
			Start: wc.Start,
			End:   wc.End,
			TZ:    wc.TZ,
		}
		if err := w.Validate(); err != nil {
			return nil, fmt.Errorf("active_windows[%d]: %w", i, err)
		}
		out = append(out, w)
	}
	return out, nil
}

// ParseDuration parses a duration string into a time.Duration.
func ParseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}
