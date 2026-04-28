package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

const (
	appStateKeyTaskCounters   = "task_counters"
	appStateKeyRateLimitBlock = "rate_limit_block"
	appStateKeyLimitsOverride = "limits_override"
	appStateKeyCostWarnState  = "cost_warn_state"
)

// taskCountersJSON is the persisted shape of BudgetCounters.
type taskCountersJSON struct {
	DailyCount  int    `json:"daily_count"`
	DailyKey    string `json:"daily_key"`
	WeeklyCount int    `json:"weekly_count"`
	WeeklyKey   string `json:"weekly_key"`
}

// rateLimitBlockJSON is the persisted shape of RateLimitBlock.
type rateLimitBlockJSON struct {
	BlockedUntilUnix int64  `json:"blocked_until_unix"`
	RateLimitType    string `json:"rate_limit_type"`
	ObservedAtUnix   int64  `json:"observed_at_unix"`
}

// limitsOverrideJSON persists runtime adjustments to the configured caps.
// A zero value falls back to the config-derived value.
type limitsOverrideJSON struct {
	DailyMax  int `json:"daily_max"`
	WeeklyMax int `json:"weekly_max"`
}

// costWarnStateJSON is the persisted shape of the cost warning flags.
type costWarnStateJSON struct {
	DailyKey          string `json:"daily_key"`
	DailyWarned80     bool   `json:"daily_warned_80"`
	DailyWarned100    bool   `json:"daily_warned_100"`
	WeeklyKey         string `json:"weekly_key"`
	WeeklyWarned80    bool   `json:"weekly_warned_80"`
	WeeklyWarned100   bool   `json:"weekly_warned_100"`
}

// CostWarnNotifier sends Slack messages for cost threshold crossings.
type CostWarnNotifier interface {
	NotifyCostWarning(ctx context.Context, scope string, percent float64, current, max float64) error
}

// BudgetSnapshot is the consolidated budget state returned to API/scheduler callers.
type BudgetSnapshot struct {
	Counters scheduler.BudgetCounters
	Limits   scheduler.BudgetLimits
	Block    scheduler.RateLimitBlock
	Reason   scheduler.BudgetReason // result of EvaluateBudget at the snapshot time
}

// BudgetUseCase persists task counters and rate-limit blocks.
//
// All reads/writes go through a single mutex so increment-after-rollover stays
// race-free at the in-process level. SQLite is the single writer too, so there
// is no second writer to coordinate with.
type BudgetUseCase struct {
	appStateRepo     domain.AppStateRepository
	configLimits     scheduler.BudgetLimits
	usageRepo        domain.UsageRepository // optional; nil disables cost warn
	costWarnNotifier CostWarnNotifier       // optional; nil disables cost warn
	dailyMaxCostUSD  float64
	weeklyMaxCostUSD float64
	mu               sync.Mutex
}

// NewBudgetUseCase creates a BudgetUseCase using configLimits as the baseline,
// optionally overridden at runtime via SetLimits.
func NewBudgetUseCase(appStateRepo domain.AppStateRepository, configLimits scheduler.BudgetLimits) *BudgetUseCase {
	return &BudgetUseCase{
		appStateRepo: appStateRepo,
		configLimits: configLimits,
	}
}

// WithCostWarn configures optional cost threshold warning support.
// usageRepo queries cumulative cost; notifier sends Slack messages.
// dailyMaxUSD and weeklyMaxUSD of 0 disables warnings for that scope.
func (uc *BudgetUseCase) WithCostWarn(usageRepo domain.UsageRepository, notifier CostWarnNotifier, dailyMaxUSD, weeklyMaxUSD float64) {
	uc.usageRepo = usageRepo
	uc.costWarnNotifier = notifier
	uc.dailyMaxCostUSD = dailyMaxUSD
	uc.weeklyMaxCostUSD = weeklyMaxUSD
}

// Snapshot returns the rolled-over counters, effective limits, current block
// and the gate decision at now.
func (uc *BudgetUseCase) Snapshot(ctx context.Context, now time.Time) (BudgetSnapshot, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	return uc.snapshotLocked(ctx, now)
}

// SnapshotReason satisfies scheduler.BudgetGate — returns just the gate decision.
func (uc *BudgetUseCase) SnapshotReason(ctx context.Context, now time.Time) (scheduler.BudgetReason, error) {
	snap, err := uc.Snapshot(ctx, now)
	if err != nil {
		return "", err
	}
	return snap.Reason, nil
}

// CheckAndIncrementReason satisfies scheduler.BudgetEnforcer — returns just
// the gate decision after the atomic check+increment.
func (uc *BudgetUseCase) CheckAndIncrementReason(ctx context.Context, now time.Time) (scheduler.BudgetReason, error) {
	reason, _, err := uc.CheckAndIncrement(ctx, now)
	return reason, err
}

// CheckAndIncrement atomically evaluates the budget gate and, if allowed,
// increments the counters in storage. It returns the reason (empty string for
// allowed) along with the post-increment snapshot.
func (uc *BudgetUseCase) CheckAndIncrement(ctx context.Context, now time.Time) (scheduler.BudgetReason, BudgetSnapshot, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	snap, err := uc.snapshotLocked(ctx, now)
	if err != nil {
		return "", snap, err
	}
	if snap.Reason != scheduler.BudgetReasonAllowed {
		return snap.Reason, snap, nil
	}

	snap.Counters.DailyCount++
	snap.Counters.WeeklyCount++
	if err := uc.persistCountersLocked(ctx, snap.Counters); err != nil {
		return "", snap, err
	}
	return scheduler.BudgetReasonAllowed, snap, nil
}

// RecordRateLimitBlock persists a CLI-observed rate-limit signal.
// resetsAtUnix is the unix-seconds value reported in rate_limit_event.resetsAt.
func (uc *BudgetUseCase) RecordRateLimitBlock(ctx context.Context, resetsAtUnix int64, rateLimitType string, observedAt time.Time) error {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	payload := rateLimitBlockJSON{
		BlockedUntilUnix: resetsAtUnix,
		RateLimitType:    rateLimitType,
		ObservedAtUnix:   observedAt.Unix(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal rate-limit block: %w", err)
	}
	return uc.appStateRepo.Set(ctx, &domain.AppState{
		Key:       appStateKeyRateLimitBlock,
		ValueJSON: string(b),
		UpdatedAt: observedAt,
	})
}

// SetLimits persists a runtime override of the configured caps. A value of 0
// for either field means "fall back to the config-derived value".
// Returns the resulting effective limits.
func (uc *BudgetUseCase) SetLimits(ctx context.Context, dailyMax, weeklyMax int) (scheduler.BudgetLimits, error) {
	if dailyMax < 0 || weeklyMax < 0 {
		return scheduler.BudgetLimits{}, fmt.Errorf("limits must be >= 0")
	}
	if dailyMax > 0 && weeklyMax > 0 && dailyMax > weeklyMax {
		return scheduler.BudgetLimits{}, fmt.Errorf("daily_max (%d) must be <= weekly_max (%d)", dailyMax, weeklyMax)
	}

	uc.mu.Lock()
	defer uc.mu.Unlock()

	payload := limitsOverrideJSON{DailyMax: dailyMax, WeeklyMax: weeklyMax}
	b, err := json.Marshal(payload)
	if err != nil {
		return scheduler.BudgetLimits{}, fmt.Errorf("marshal limits override: %w", err)
	}
	if err := uc.appStateRepo.Set(ctx, &domain.AppState{
		Key:       appStateKeyLimitsOverride,
		ValueJSON: string(b),
		UpdatedAt: time.Now(),
	}); err != nil {
		return scheduler.BudgetLimits{}, err
	}

	return uc.effectiveLimitsLocked(ctx)
}

// snapshotLocked must be called with uc.mu held.
func (uc *BudgetUseCase) snapshotLocked(ctx context.Context, now time.Time) (BudgetSnapshot, error) {
	limits, err := uc.effectiveLimitsLocked(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	counters, err := uc.loadCountersLocked(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	block, err := uc.loadBlockLocked(ctx)
	if err != nil {
		return BudgetSnapshot{}, err
	}

	rolled := scheduler.RolloverCounters(now, counters, limits)
	if rolled != counters {
		if err := uc.persistCountersLocked(ctx, rolled); err != nil {
			return BudgetSnapshot{}, err
		}
		counters = rolled
	}

	reason := scheduler.EvaluateBudget(now, counters, limits, block)
	return BudgetSnapshot{
		Counters: counters,
		Limits:   limits,
		Block:    block,
		Reason:   reason,
	}, nil
}

func (uc *BudgetUseCase) effectiveLimitsLocked(ctx context.Context) (scheduler.BudgetLimits, error) {
	limits := uc.configLimits

	state, err := uc.appStateRepo.Get(ctx, appStateKeyLimitsOverride)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return limits, nil
		}
		return limits, err
	}
	var ov limitsOverrideJSON
	if err := json.Unmarshal([]byte(state.ValueJSON), &ov); err != nil {
		// Corrupt override — log? We just ignore it and use config.
		return limits, nil
	}
	if ov.DailyMax > 0 {
		limits.DailyMax = ov.DailyMax
	}
	if ov.WeeklyMax > 0 {
		limits.WeeklyMax = ov.WeeklyMax
	}
	return limits, nil
}

func (uc *BudgetUseCase) loadCountersLocked(ctx context.Context) (scheduler.BudgetCounters, error) {
	state, err := uc.appStateRepo.Get(ctx, appStateKeyTaskCounters)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return scheduler.BudgetCounters{}, nil
		}
		return scheduler.BudgetCounters{}, err
	}
	var c taskCountersJSON
	if err := json.Unmarshal([]byte(state.ValueJSON), &c); err != nil {
		return scheduler.BudgetCounters{}, nil
	}
	return scheduler.BudgetCounters{
		DailyCount:  c.DailyCount,
		DailyKey:    c.DailyKey,
		WeeklyCount: c.WeeklyCount,
		WeeklyKey:   c.WeeklyKey,
	}, nil
}

func (uc *BudgetUseCase) persistCountersLocked(ctx context.Context, counters scheduler.BudgetCounters) error {
	payload := taskCountersJSON{
		DailyCount:  counters.DailyCount,
		DailyKey:    counters.DailyKey,
		WeeklyCount: counters.WeeklyCount,
		WeeklyKey:   counters.WeeklyKey,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal counters: %w", err)
	}
	return uc.appStateRepo.Set(ctx, &domain.AppState{
		Key:       appStateKeyTaskCounters,
		ValueJSON: string(b),
		UpdatedAt: time.Now(),
	})
}

func (uc *BudgetUseCase) loadBlockLocked(ctx context.Context) (scheduler.RateLimitBlock, error) {
	state, err := uc.appStateRepo.Get(ctx, appStateKeyRateLimitBlock)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return scheduler.RateLimitBlock{}, nil
		}
		return scheduler.RateLimitBlock{}, err
	}
	var b rateLimitBlockJSON
	if err := json.Unmarshal([]byte(state.ValueJSON), &b); err != nil {
		return scheduler.RateLimitBlock{}, nil
	}
	if b.BlockedUntilUnix == 0 {
		return scheduler.RateLimitBlock{}, nil
	}
	return scheduler.RateLimitBlock{
		BlockedUntil:  time.Unix(b.BlockedUntilUnix, 0),
		RateLimitType: b.RateLimitType,
	}, nil
}

// EvaluateCostWarn checks accumulated daily/weekly cost against configured limits
// and fires Slack warnings when the 80% or 100% thresholds are crossed for the first time
// in the current bucket. Idempotent — repeated calls in the same bucket never re-send.
// Satisfies scheduler.CostWarnEvaluator.
func (uc *BudgetUseCase) EvaluateCostWarn(ctx context.Context, now time.Time) error {
	if uc.usageRepo == nil || uc.costWarnNotifier == nil {
		return nil
	}

	uc.mu.Lock()
	defer uc.mu.Unlock()

	limits, err := uc.effectiveLimitsLocked(ctx)
	if err != nil {
		return fmt.Errorf("evaluate cost warn: load limits: %w", err)
	}

	warnState, err := uc.loadCostWarnStateLocked(ctx)
	if err != nil {
		return fmt.Errorf("evaluate cost warn: load state: %w", err)
	}

	dailyKey := scheduler.DateKey(now, limits.ResetTZ)
	weeklyKey := scheduler.WeekKey(now, limits.ResetTZ, limits.WeekStartsOn)

	// Reset flags when the bucket rolls over.
	if warnState.DailyKey != dailyKey {
		warnState.DailyKey = dailyKey
		warnState.DailyWarned80 = false
		warnState.DailyWarned100 = false
	}
	if warnState.WeeklyKey != weeklyKey {
		warnState.WeeklyKey = weeklyKey
		warnState.WeeklyWarned80 = false
		warnState.WeeklyWarned100 = false
	}

	// Evaluate daily.
	if uc.dailyMaxCostUSD > 0 {
		dailyCost, err := uc.usageRepo.SumDailyCost(ctx, dailyKey)
		if err != nil {
			slog.Warn("evaluate cost warn: sum daily cost", "err", err)
		} else {
			pct := dailyCost / uc.dailyMaxCostUSD * 100
			pct = math.Round(pct*10) / 10

			if pct >= 100 && !warnState.DailyWarned100 {
				warnState.DailyWarned100 = true
				// Always set flag before sending — prevents retry storm on failure (PRD R3).
				if persistErr := uc.persistCostWarnStateLocked(ctx, warnState, now); persistErr != nil { // MODIFIED
					slog.Warn("cost warn: persist state failed", "err", persistErr)
				}
				if notifyErr := uc.costWarnNotifier.NotifyCostWarning(ctx, "daily", pct, dailyCost, uc.dailyMaxCostUSD); notifyErr != nil {
					slog.Warn("cost warn: notify daily 100%", "err", notifyErr)
				}
			} else if pct >= 80 && !warnState.DailyWarned80 {
				warnState.DailyWarned80 = true
				if persistErr := uc.persistCostWarnStateLocked(ctx, warnState, now); persistErr != nil { // MODIFIED
					slog.Warn("cost warn: persist state failed", "err", persistErr)
				}
				if notifyErr := uc.costWarnNotifier.NotifyCostWarning(ctx, "daily", pct, dailyCost, uc.dailyMaxCostUSD); notifyErr != nil {
					slog.Warn("cost warn: notify daily 80%", "err", notifyErr)
				}
			}
		}
	}

	// Evaluate weekly.
	if uc.weeklyMaxCostUSD > 0 {
		weeklyCost, err := uc.usageRepo.SumWeeklyCost(ctx, weeklyKey)
		if err != nil {
			slog.Warn("evaluate cost warn: sum weekly cost", "err", err)
		} else {
			pct := weeklyCost / uc.weeklyMaxCostUSD * 100
			pct = math.Round(pct*10) / 10

			if pct >= 100 && !warnState.WeeklyWarned100 {
				warnState.WeeklyWarned100 = true
				if persistErr := uc.persistCostWarnStateLocked(ctx, warnState, now); persistErr != nil { // MODIFIED
					slog.Warn("cost warn: persist state failed", "err", persistErr)
				}
				if notifyErr := uc.costWarnNotifier.NotifyCostWarning(ctx, "weekly", pct, weeklyCost, uc.weeklyMaxCostUSD); notifyErr != nil {
					slog.Warn("cost warn: notify weekly 100%", "err", notifyErr)
				}
			} else if pct >= 80 && !warnState.WeeklyWarned80 {
				warnState.WeeklyWarned80 = true
				if persistErr := uc.persistCostWarnStateLocked(ctx, warnState, now); persistErr != nil { // MODIFIED
					slog.Warn("cost warn: persist state failed", "err", persistErr)
				}
				if notifyErr := uc.costWarnNotifier.NotifyCostWarning(ctx, "weekly", pct, weeklyCost, uc.weeklyMaxCostUSD); notifyErr != nil {
					slog.Warn("cost warn: notify weekly 80%", "err", notifyErr)
				}
			}
		}
	}

	return uc.persistCostWarnStateLocked(ctx, warnState, now)
}

func (uc *BudgetUseCase) loadCostWarnStateLocked(ctx context.Context) (costWarnStateJSON, error) {
	state, err := uc.appStateRepo.Get(ctx, appStateKeyCostWarnState)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return costWarnStateJSON{}, nil
		}
		return costWarnStateJSON{}, err
	}
	var s costWarnStateJSON
	if err := json.Unmarshal([]byte(state.ValueJSON), &s); err != nil {
		return costWarnStateJSON{}, nil
	}
	return s, nil
}

func (uc *BudgetUseCase) persistCostWarnStateLocked(ctx context.Context, s costWarnStateJSON, now time.Time) error {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal cost warn state: %w", err)
	}
	return uc.appStateRepo.Set(ctx, &domain.AppState{
		Key:       appStateKeyCostWarnState,
		ValueJSON: string(b),
		UpdatedAt: now,
	})
}
