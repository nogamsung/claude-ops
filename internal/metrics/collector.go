package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// dynamicCollector exposes gauges whose value changes independently of task
// events — tasks_remaining is driven by the daily/weekly bucket rollover,
// rate_limit_block_seconds_remaining counts down in real time, and
// active_window_open depends on the wall clock. Implementing prometheus.Collector
// lets us compute them exactly once per scrape without a background goroutine.
type dynamicCollector struct {
	m *Metrics

	tasksRemaining                 *prometheus.Desc
	rateLimitBlockSecondsRemaining *prometheus.Desc
	activeWindowOpen               *prometheus.Desc
}

func newDynamicCollector(m *Metrics) *dynamicCollector {
	return &dynamicCollector{
		m: m,
		tasksRemaining: prometheus.NewDesc(
			"claude_ops_tasks_remaining",
			"Remaining task slots before the daily/weekly cap is hit.",
			[]string{"scope"}, nil,
		),
		rateLimitBlockSecondsRemaining: prometheus.NewDesc(
			"claude_ops_rate_limit_block_seconds_remaining",
			"Seconds until the observed CLI rate-limit block expires (0 if no active block).",
			nil, nil,
		),
		activeWindowOpen: prometheus.NewDesc(
			"claude_ops_active_window_open",
			"1 if the scheduler would dispatch a task at scrape time (in-window or full-mode), else 0.",
			nil, nil,
		),
	}
}

// Describe emits the static metric descriptions.
func (c *dynamicCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.tasksRemaining
	ch <- c.rateLimitBlockSecondsRemaining
	ch <- c.activeWindowOpen
}

// Collect reads live state from the injected sources. Failures log and skip
// the affected gauge — the other gauges still emit so partial outages don't
// black out the dashboard.
func (c *dynamicCollector) Collect(ch chan<- prometheus.Metric) {
	now := c.m.clock.Now()

	if c.m.budget != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		snap, err := c.m.budget.Snapshot(ctx, now)
		cancel()
		if err != nil {
			slog.Warn("metrics: budget snapshot", "err", err)
		} else {
			ch <- prometheus.MustNewConstMetric(c.tasksRemaining, prometheus.GaugeValue,
				remaining(snap.Limits.DailyMax, snap.Counters.DailyCount), "daily")
			ch <- prometheus.MustNewConstMetric(c.tasksRemaining, prometheus.GaugeValue,
				remaining(snap.Limits.WeeklyMax, snap.Counters.WeeklyCount), "weekly")

			blockSeconds := 0.0
			if snap.Block.Active(now) {
				blockSeconds = snap.Block.BlockedUntil.Sub(now).Seconds()
			}
			ch <- prometheus.MustNewConstMetric(c.rateLimitBlockSecondsRemaining,
				prometheus.GaugeValue, blockSeconds)
		}
	}

	if c.m.window != nil {
		fullMode := false
		if c.m.fullMode != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			fullMode = c.m.fullMode.IsFullMode(ctx)
			cancel()
		}
		open := 0.0
		if c.m.window.IsOpen(now, fullMode) {
			open = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.activeWindowOpen, prometheus.GaugeValue, open)
	}
}

// remaining clamps the remaining-slot gauge to zero and treats max<=0 as
// "no limit configured" — reported as math.Inf? No: Prometheus gauges of
// +Inf render poorly in Grafana. Report -1 so dashboards can special-case it.
func remaining(max, used int) float64 {
	if max <= 0 {
		return -1
	}
	diff := max - used
	if diff < 0 {
		return 0
	}
	return float64(diff)
}
