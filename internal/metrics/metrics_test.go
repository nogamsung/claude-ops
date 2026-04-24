package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/scheduler"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time { return f.t }

type fakeBudget struct {
	snap usecase.BudgetSnapshot
	err  error
}

func (f *fakeBudget) Snapshot(_ context.Context, _ time.Time) (usecase.BudgetSnapshot, error) {
	return f.snap, f.err
}

type fakeWindow struct{ open bool }

func (f *fakeWindow) IsOpen(_ time.Time, _ bool) bool { return f.open }

type fakeFullMode struct{ on bool }

func (f *fakeFullMode) IsFullMode(_ context.Context) bool { return f.on }

func TestRecordTaskFinished_UpdatesCounterAndHistogram(t *testing.T) {
	m := New(Options{})
	start := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)

	m.RecordTaskFinished("owner/repo", "feature", "done", start, end)
	m.RecordTaskFinished("owner/repo", "feature", "done", start, end)
	m.RecordTaskFinished("owner/repo", "security", "failed", start, end)

	body := scrape(t, m)
	if !strings.Contains(body, `claude_ops_tasks_total{status="done"} 2`) {
		t.Errorf("done counter=2 missing in:\n%s", body)
	}
	if !strings.Contains(body, `claude_ops_tasks_total{status="failed"} 1`) {
		t.Errorf("failed counter=1 missing in:\n%s", body)
	}
	if !strings.Contains(body, `claude_ops_task_duration_seconds_count{repo="owner/repo",status="done",task_type="feature"} 2`) {
		t.Errorf("histogram count missing in:\n%s", body)
	}
}

func TestRecordTaskFinished_IgnoresInvalidDuration(t *testing.T) {
	m := New(Options{})
	end := time.Now()
	// startedAt zero → duration=0, still records
	m.RecordTaskFinished("r", "t", "failed", time.Time{}, end)
	// startedAt after finishedAt → duration=0
	m.RecordTaskFinished("r", "t", "failed", end.Add(time.Hour), end)

	body := scrape(t, m)
	if !strings.Contains(body, `claude_ops_tasks_total{status="failed"} 2`) {
		t.Errorf("expected 2 failed observations, got:\n%s", body)
	}
}

func TestRecordBudgetBlock_AllowedIsNoop(t *testing.T) {
	m := New(Options{})
	m.RecordBudgetBlock(scheduler.BudgetReasonAllowed) // noop
	m.RecordBudgetBlock(scheduler.BudgetReasonDailyCap)
	m.RecordBudgetBlock(scheduler.BudgetReasonWeeklyCap)
	m.RecordBudgetBlock(scheduler.BudgetReasonRateLimited)
	m.RecordWindowClose()

	body := scrape(t, m)
	for _, expect := range []string{
		`claude_ops_budget_gate_blocks_total{reason="daily_cap_reached"} 1`,
		`claude_ops_budget_gate_blocks_total{reason="weekly_cap_reached"} 1`,
		`claude_ops_budget_gate_blocks_total{reason="rate_limited"} 1`,
		`claude_ops_budget_gate_blocks_total{reason="window_closed"} 1`,
	} {
		if !strings.Contains(body, expect) {
			t.Errorf("missing %q in:\n%s", expect, body)
		}
	}
	if strings.Contains(body, `reason=""`) {
		t.Error("empty-reason series should not be recorded")
	}
}

func TestCollector_TasksRemainingAndRateLimit(t *testing.T) {
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	clock := &fakeClock{t: now}
	budget := &fakeBudget{snap: usecase.BudgetSnapshot{
		Counters: scheduler.BudgetCounters{DailyCount: 3, WeeklyCount: 12},
		Limits:   scheduler.BudgetLimits{DailyMax: 10, WeeklyMax: 50, ResetTZ: time.UTC},
		Block: scheduler.RateLimitBlock{
			BlockedUntil:  now.Add(90 * time.Second),
			RateLimitType: "5hour",
		},
	}}
	win := &fakeWindow{open: true}
	fm := &fakeFullMode{on: false}
	m := New(Options{Budget: budget, Window: win, FullMode: fm, Clock: clock})

	body := scrape(t, m)
	for _, expect := range []string{
		`claude_ops_tasks_remaining{scope="daily"} 7`,
		`claude_ops_tasks_remaining{scope="weekly"} 38`,
		`claude_ops_rate_limit_block_seconds_remaining 90`,
		`claude_ops_active_window_open 1`,
	} {
		if !strings.Contains(body, expect) {
			t.Errorf("missing %q in:\n%s", expect, body)
		}
	}
}

func TestCollector_NoLimitsReportsNegativeOne(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	budget := &fakeBudget{snap: usecase.BudgetSnapshot{
		Counters: scheduler.BudgetCounters{DailyCount: 5, WeeklyCount: 20},
		Limits:   scheduler.BudgetLimits{DailyMax: 0, WeeklyMax: 0, ResetTZ: time.UTC},
	}}
	m := New(Options{Budget: budget, Clock: clock})

	body := scrape(t, m)
	if !strings.Contains(body, `claude_ops_tasks_remaining{scope="daily"} -1`) {
		t.Errorf("daily unlimited should emit -1, got:\n%s", body)
	}
}

func TestCollector_WindowClosedReportsZero(t *testing.T) {
	m := New(Options{
		Budget: &fakeBudget{snap: usecase.BudgetSnapshot{Limits: scheduler.BudgetLimits{ResetTZ: time.UTC}}},
		Window: &fakeWindow{open: false},
		Clock:  &fakeClock{t: time.Now()},
	})
	body := scrape(t, m)
	if !strings.Contains(body, `claude_ops_active_window_open 0`) {
		t.Errorf("closed window should emit 0, got:\n%s", body)
	}
}

func TestForecast_ReturnsETAForBothBuckets(t *testing.T) {
	// 10:00 UTC, daily_count=5 out of 20 over 10 hours → ~0.5/hr → ETA in ~30h
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	snap := usecase.BudgetSnapshot{
		Counters: scheduler.BudgetCounters{
			DailyCount: 5, DailyKey: "2026-04-24",
			WeeklyCount: 10, WeeklyKey: "2026-W17",
		},
		Limits: scheduler.BudgetLimits{
			DailyMax: 20, WeeklyMax: 50,
			WeekStartsOn: time.Monday, ResetTZ: time.UTC,
		},
	}
	h := NewHandler(New(Options{Budget: &fakeBudget{snap: snap}, Clock: &fakeClock{t: now}}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics/forecast", http.NoBody)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"daily_used":5`) || !strings.Contains(body, `"weekly_used":10`) {
		t.Errorf("forecast body missing counters: %s", body)
	}
	if !strings.Contains(body, `"daily_eta"`) {
		t.Errorf("expected daily_eta field, got: %s", body)
	}
}

func TestForecast_SkipsETAWhenUnused(t *testing.T) {
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	snap := usecase.BudgetSnapshot{
		Counters: scheduler.BudgetCounters{DailyCount: 0, WeeklyCount: 0},
		Limits:   scheduler.BudgetLimits{DailyMax: 20, WeeklyMax: 50, ResetTZ: time.UTC},
	}
	h := NewHandler(New(Options{Budget: &fakeBudget{snap: snap}, Clock: &fakeClock{t: now}}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics/forecast", http.NoBody)
	r.ServeHTTP(w, req)

	if strings.Contains(w.Body.String(), `"daily_eta"`) {
		t.Errorf("daily_eta should be omitted when used=0, got: %s", w.Body.String())
	}
}

// scrape serves /metrics and returns the response body. Helper to keep tests terse.
func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(m).Register(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/metrics returned %d", w.Code)
	}
	return w.Body.String()
}
