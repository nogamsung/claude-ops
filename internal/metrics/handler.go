package metrics

import (
	"context"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler wraps the Metrics registry for Gin registration.
type Handler struct {
	metrics *Metrics
}

// NewHandler constructs a Handler around m.
func NewHandler(m *Metrics) *Handler {
	return &Handler{metrics: m}
}

// Register wires the /metrics and /metrics/forecast routes onto r.
func (h *Handler) Register(r gin.IRouter) {
	promHandler := promhttp.HandlerFor(h.metrics.Registry, promhttp.HandlerOpts{})
	r.GET("/metrics", gin.WrapH(promHandler))
	r.GET("/metrics/forecast", h.Forecast)
}

// ForecastResponse is the JSON body returned by /metrics/forecast.
type ForecastResponse struct {
	Now                   time.Time  `json:"now" example:"2026-04-24T10:00:00Z"`
	DailyUsed             int        `json:"daily_used" example:"3"`
	DailyMax              int        `json:"daily_max" example:"10"`
	DailyKey              string     `json:"daily_key" example:"2026-04-24"`
	DailyETA              *time.Time `json:"daily_eta,omitempty"`
	WeeklyUsed            int        `json:"weekly_used" example:"12"`
	WeeklyMax             int        `json:"weekly_max" example:"50"`
	WeeklyKey             string     `json:"weekly_key" example:"2026-W17"`
	WeeklyETA             *time.Time `json:"weekly_eta,omitempty"`
	RateLimitActive       bool       `json:"rate_limit_active" example:"false"`
	RateLimitType         string     `json:"rate_limit_type,omitempty" example:"5hour"`
	RateLimitBlockedUntil *time.Time `json:"rate_limit_blocked_until,omitempty"`
}

// Forecast godoc
//
//	@Summary  Budget soak forecast
//	@Tags     metrics
//	@Produce  json
//	@Success  200 {object} ForecastResponse
//	@Failure  503 {object} map[string]string
//	@Router   /metrics/forecast [get]
func (h *Handler) Forecast(c *gin.Context) {
	if h.metrics == nil || h.metrics.budget == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "budget source not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	now := h.metrics.clock.Now()
	snap, err := h.metrics.budget.Snapshot(ctx, now)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "budget snapshot failed"})
		return
	}

	resp := ForecastResponse{
		Now:        now,
		DailyUsed:  snap.Counters.DailyCount,
		DailyMax:   snap.Limits.DailyMax,
		DailyKey:   snap.Counters.DailyKey,
		WeeklyUsed: snap.Counters.WeeklyCount,
		WeeklyMax:  snap.Limits.WeeklyMax,
		WeeklyKey:  snap.Counters.WeeklyKey,
	}

	tz := snap.Limits.ResetTZ
	if tz == nil {
		tz = time.UTC
	}

	if eta := predictDailyETA(now, tz, snap.Counters.DailyCount, snap.Limits.DailyMax); eta != nil {
		resp.DailyETA = eta
	}
	if eta := predictWeeklyETA(now, tz, snap.Limits.WeekStartsOn, snap.Counters.WeeklyCount, snap.Limits.WeeklyMax); eta != nil {
		resp.WeeklyETA = eta
	}
	if snap.Block.Active(now) {
		resp.RateLimitActive = true
		resp.RateLimitType = snap.Block.RateLimitType
		bu := snap.Block.BlockedUntil
		resp.RateLimitBlockedUntil = &bu
	}
	c.JSON(http.StatusOK, resp)
}

// predictDailyETA linearly extrapolates the current daily consumption rate to
// estimate when the daily cap will be hit. Returns nil if max is 0/unlimited,
// already exhausted, or used=0 (no data to extrapolate from).
func predictDailyETA(now time.Time, tz *time.Location, used, max int) *time.Time {
	if max <= 0 || used >= max || used <= 0 {
		return nil
	}
	dayStart := time.Date(now.In(tz).Year(), now.In(tz).Month(), now.In(tz).Day(), 0, 0, 0, 0, tz)
	elapsed := now.Sub(dayStart).Seconds()
	if elapsed <= 0 {
		return nil
	}
	ratePerSec := float64(used) / elapsed
	if ratePerSec <= 0 || math.IsInf(ratePerSec, 0) {
		return nil
	}
	remainingSlots := float64(max - used)
	etaSec := remainingSlots / ratePerSec
	eta := now.Add(time.Duration(etaSec * float64(time.Second)))
	return &eta
}

// predictWeeklyETA mirrors predictDailyETA but anchors to the configured
// week start. Same guardrails.
func predictWeeklyETA(now time.Time, tz *time.Location, weekStartsOn time.Weekday, used, max int) *time.Time {
	if max <= 0 || used >= max || used <= 0 {
		return nil
	}
	local := now.In(tz)
	offset := (int(local.Weekday()) - int(weekStartsOn) + 7) % 7
	weekStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tz).
		AddDate(0, 0, -offset)
	elapsed := now.Sub(weekStart).Seconds()
	if elapsed <= 0 {
		return nil
	}
	ratePerSec := float64(used) / elapsed
	if ratePerSec <= 0 || math.IsInf(ratePerSec, 0) {
		return nil
	}
	remainingSlots := float64(max - used)
	etaSec := remainingSlots / ratePerSec
	eta := now.Add(time.Duration(etaSec * float64(time.Second)))
	return &eta
}
