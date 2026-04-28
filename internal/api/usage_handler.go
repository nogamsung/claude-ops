package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// UsageQuerier is the interface the handler depends on.
type UsageQuerier interface {
	Aggregate(ctx context.Context, from, to time.Time, bucket domain.BucketKind) (usecase.UsageAggregateResult, error)
	ByModel(ctx context.Context, from, to time.Time) ([]domain.UsageModelRow, error)
	Limits(ctx context.Context, now time.Time) (usecase.UsageLimitsSnapshot, error)
}

// UsageHandler handles the /usage, /usage/by-model and /usage/limits endpoints.
type UsageHandler struct {
	uc UsageQuerier
}

// NewUsageHandler creates a new UsageHandler.
func NewUsageHandler(uc UsageQuerier) *UsageHandler {
	return &UsageHandler{uc: uc}
}

// parseFromTo parses the from/to query params; both default to today / today-30d.
func parseFromTo(c *gin.Context) (from, to time.Time, ok bool) {
	now := time.Now().UTC()
	toStr := c.Query("to")
	fromStr := c.Query("from")

	if toStr == "" {
		to = now.Truncate(24 * time.Hour)
	} else {
		var err error
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid to: must be YYYY-MM-DD"})
			return time.Time{}, time.Time{}, false
		}
	}

	if fromStr == "" {
		from = to.AddDate(0, 0, -30)
	} else {
		var err error
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid from: must be YYYY-MM-DD"})
			return time.Time{}, time.Time{}, false
		}
	}

	return from, to, true
}

func mapUsageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidRange):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid range: from must not be after to"})
	case errors.Is(err, domain.ErrRangeTooLarge):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "range too large: must not exceed 365 days"})
	case errors.Is(err, domain.ErrInvalidBucket):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid group_by: must be one of day, week, month"})
	default:
		slog.Error("usage handler error", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
	}
}

// GetUsage godoc
//
//	@Summary	Aggregated usage by time bucket
//	@Tags		usage
//	@Produce	json
//	@Param		from		query		string	false	"Start date YYYY-MM-DD (default: to-30d)"
//	@Param		to			query		string	false	"End date YYYY-MM-DD (default: today)"
//	@Param		group_by	query		string	false	"Bucket granularity: day|week|month (default: day)" Enums(day, week, month)
//	@Success	200			{object}	api.UsageResponse
//	@Failure	400			{object}	api.ErrorResponse
//	@Failure	500			{object}	api.ErrorResponse
//	@Router		/usage [get]
func (h *UsageHandler) GetUsage(c *gin.Context) {
	from, to, ok := parseFromTo(c)
	if !ok {
		return
	}

	groupBy := c.DefaultQuery("group_by", "day")
	bucket := domain.BucketKind(groupBy)

	result, err := h.uc.Aggregate(c.Request.Context(), from, to, bucket)
	if err != nil {
		mapUsageError(c, err)
		return
	}

	buckets := make([]UsageBucketResponse, len(result.Buckets))
	for i, b := range result.Buckets {
		buckets[i] = UsageBucketResponse{
			Bucket:              b.Bucket,
			TaskCount:           b.TaskCount,
			CostUSD:             b.CostUSD,
			InputTokens:         b.InputTokens,
			OutputTokens:        b.OutputTokens,
			CacheReadTokens:     b.CacheReadTokens,
			CacheCreationTokens: b.CacheCreationTokens,
			FailedCostUSD:       b.FailedCostUSD,
		}
	}

	c.JSON(http.StatusOK, UsageResponse{
		From:    result.From,
		To:      result.To,
		GroupBy: string(result.GroupBy),
		Buckets: buckets,
		Totals: UsageTotalsResponse{
			TaskCount:           result.Totals.TaskCount,
			CostUSD:             result.Totals.CostUSD,
			InputTokens:         result.Totals.InputTokens,
			OutputTokens:        result.Totals.OutputTokens,
			CacheReadTokens:     result.Totals.CacheReadTokens,
			CacheCreationTokens: result.Totals.CacheCreationTokens,
			FailedCostUSD:       result.Totals.FailedCostUSD,
		},
	})
}

// GetUsageByModel godoc
//
//	@Summary	Per-model usage aggregation
//	@Tags		usage
//	@Produce	json
//	@Param		from	query		string	false	"Start date YYYY-MM-DD (default: today-30d)"
//	@Param		to		query		string	false	"End date YYYY-MM-DD (default: today)"
//	@Success	200		{object}	api.UsageByModelResponse
//	@Failure	400		{object}	api.ErrorResponse
//	@Failure	500		{object}	api.ErrorResponse
//	@Router		/usage/by-model [get]
func (h *UsageHandler) GetUsageByModel(c *gin.Context) {
	from, to, ok := parseFromTo(c)
	if !ok {
		return
	}

	rows, err := h.uc.ByModel(c.Request.Context(), from, to)
	if err != nil {
		mapUsageError(c, err)
		return
	}

	models := make([]UsageModelItemResponse, len(rows))
	for i, r := range rows {
		models[i] = UsageModelItemResponse{
			ModelID:             r.ModelID,
			TaskCount:           r.TaskCount,
			CostUSD:             r.CostUSD,
			InputTokens:         r.InputTokens,
			OutputTokens:        r.OutputTokens,
			CacheReadTokens:     r.CacheReadTokens,
			CacheCreationTokens: r.CacheCreationTokens,
		}
	}

	c.JSON(http.StatusOK, UsageByModelResponse{
		From:   from.Format("2006-01-02"),
		To:     to.Format("2006-01-02"),
		Models: models,
	})
}

// GetUsageLimits godoc
//
//	@Summary	Current cost usage vs configured limits
//	@Tags		usage
//	@Produce	json
//	@Success	200	{object}	api.UsageLimitsResponse
//	@Failure	500	{object}	api.ErrorResponse
//	@Router		/usage/limits [get]
func (h *UsageHandler) GetUsageLimits(c *gin.Context) {
	snap, err := h.uc.Limits(c.Request.Context(), time.Now().UTC())
	if err != nil {
		slog.Error("usage limits handler error", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	c.JSON(http.StatusOK, UsageLimitsResponse{
		Daily: UsageLimitsDailyResponse{
			UsageLimitsPeriod: UsageLimitsPeriod{
				CountUSD: snap.DailyCountUSD,
				MaxUSD:   snap.DailyMaxUSD,
				Percent:  snap.DailyPercent,
			},
			Date: snap.DailyDate,
		},
		Weekly: UsageLimitsWeeklyResponse{
			UsageLimitsPeriod: UsageLimitsPeriod{
				CountUSD: snap.WeeklyCountUSD,
				MaxUSD:   snap.WeeklyMaxUSD,
				Percent:  snap.WeeklyPercent,
			},
			Week: snap.WeeklyWeek,
		},
	})
}
