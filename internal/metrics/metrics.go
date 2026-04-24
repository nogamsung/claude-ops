// Package metrics exposes Prometheus metrics for task throughput, budget
// pressure, and active-window state. Counters/histograms are updated by the
// Worker at task lifecycle transitions; gauges are scraped live from
// BudgetSource and WindowSource so operators see the same value the scheduler
// would use on the next tick.
package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/gs97ahn/claude-ops/internal/scheduler"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// BudgetSource returns the live budget snapshot — used by the scrape-time
// collector. The Worker also uses BudgetUseCase, so the same source is read.
type BudgetSource interface {
	Snapshot(ctx context.Context, now time.Time) (usecase.BudgetSnapshot, error)
}

// WindowSource reports whether the scheduler would dispatch a task at t.
// fullMode=true means bypass window restriction (budget still applies).
type WindowSource interface {
	IsOpen(t time.Time, fullMode bool) bool
}

// FullModeSource reports whether the global full-mode toggle is on. Scrape
// never blocks on it — an error is treated as "off".
type FullModeSource interface {
	IsFullMode(ctx context.Context) bool
}

// Clock abstracts time.Now for test determinism.
type Clock interface {
	Now() time.Time
}

// realClock delegates to the stdlib.
type realClock struct{}

// Now returns the current wall-clock time.
func (realClock) Now() time.Time { return time.Now() }

// Metrics holds all Prometheus instruments and the registry that exposes them.
type Metrics struct {
	TaskDuration     *prometheus.HistogramVec
	TasksTotal       *prometheus.CounterVec
	BudgetGateBlocks *prometheus.CounterVec

	Registry *prometheus.Registry

	// Scrape-time sources (read-only, no writes during scrape).
	budget   BudgetSource
	window   WindowSource
	fullMode FullModeSource
	clock    Clock
}

// Options configures Metrics wiring. Any nil source results in the
// corresponding gauge being absent rather than exported as 0, so dashboards
// don't show misleading values.
type Options struct {
	Budget   BudgetSource
	Window   WindowSource
	FullMode FullModeSource
	Clock    Clock
}

// New constructs Metrics and registers every collector on a fresh Registry.
// The returned Registry is what the /metrics handler should serve from — it
// intentionally skips the process/go collectors to keep the endpoint focused
// on claude-ops semantics.
func New(opts Options) *Metrics {
	clock := opts.Clock
	if clock == nil {
		clock = realClock{}
	}
	m := &Metrics{
		TaskDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "claude_ops_task_duration_seconds",
			Help:    "End-to-end duration of a claude-ops task, from queued-to-running until terminal status.",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200, 1800, 3600},
		}, []string{"repo", "status", "task_type"}),
		TasksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "claude_ops_tasks_total",
			Help: "Count of task transitions to a terminal status.",
		}, []string{"status"}),
		BudgetGateBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "claude_ops_budget_gate_blocks_total",
			Help: "Count of budget-gate rejections, by reason.",
		}, []string{"reason"}),
		Registry: prometheus.NewRegistry(),
		budget:   opts.Budget,
		window:   opts.Window,
		fullMode: opts.FullMode,
		clock:    clock,
	}
	m.Registry.MustRegister(m.TaskDuration, m.TasksTotal, m.BudgetGateBlocks)
	m.Registry.MustRegister(newDynamicCollector(m))
	return m
}

// RecordTaskFinished updates the duration histogram and status counter for a
// terminal-status transition. Call once per task, at the lifecycle end.
// startedAt zero or finishedAt before startedAt is treated as duration=0 —
// avoids polluting the histogram with negative or nonsense values from
// orphan-marked tasks whose StartedAt we discarded.
func (m *Metrics) RecordTaskFinished(repo, taskType, status string, startedAt, finishedAt time.Time) {
	if m == nil {
		return
	}
	dur := 0.0
	if !startedAt.IsZero() && !finishedAt.Before(startedAt) {
		dur = finishedAt.Sub(startedAt).Seconds()
	}
	m.TaskDuration.WithLabelValues(repo, status, taskType).Observe(dur)
	m.TasksTotal.WithLabelValues(status).Inc()
}

// RecordBudgetBlock increments the block counter. reason comes from
// scheduler.BudgetReason — the empty "allowed" case is a no-op so callers
// can pipe every gate decision through without a branch.
func (m *Metrics) RecordBudgetBlock(reason scheduler.BudgetReason) {
	if m == nil || reason == scheduler.BudgetReasonAllowed {
		return
	}
	m.BudgetGateBlocks.WithLabelValues(string(reason)).Inc()
}

// RecordWindowClose is a distinct reason label for tasks killed when the
// active window closes mid-dispatch — this is not a budget reason, but we
// surface it through the same counter so operators have one place to look
// for "why isn't my task dispatching".
func (m *Metrics) RecordWindowClose() {
	if m == nil {
		return
	}
	m.BudgetGateBlocks.WithLabelValues("window_closed").Inc()
}
