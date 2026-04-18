package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
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
	return nil
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
