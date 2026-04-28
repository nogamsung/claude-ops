package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gs97ahn/claude-ops/internal/api"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// fakeUsageQuerier is a test double for api.UsageQuerier.
type fakeUsageQuerier struct {
	aggregateResult usecase.UsageAggregateResult
	aggregateErr    error
	byModelResult   []domain.UsageModelRow
	byModelErr      error
	limitsResult    usecase.UsageLimitsSnapshot
	limitsErr       error
}

func (f *fakeUsageQuerier) Aggregate(_ context.Context, from, to time.Time, bucket domain.BucketKind) (usecase.UsageAggregateResult, error) {
	if f.aggregateErr != nil {
		return usecase.UsageAggregateResult{}, f.aggregateErr
	}
	return f.aggregateResult, nil
}

func (f *fakeUsageQuerier) ByModel(_ context.Context, from, to time.Time) ([]domain.UsageModelRow, error) {
	return f.byModelResult, f.byModelErr
}

func (f *fakeUsageQuerier) Limits(_ context.Context, now time.Time) (usecase.UsageLimitsSnapshot, error) {
	return f.limitsResult, f.limitsErr
}

func setupUsageRouter(q api.UsageQuerier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := api.NewUsageHandler(q)
	r.GET("/usage", h.GetUsage)
	r.GET("/usage/by-model", h.GetUsageByModel)
	r.GET("/usage/limits", h.GetUsageLimits)
	return r
}

func TestGetUsage_DefaultsReturn200(t *testing.T) {
	q := &fakeUsageQuerier{
		aggregateResult: usecase.UsageAggregateResult{
			From:    "2026-03-28",
			To:      "2026-04-27",
			GroupBy: domain.BucketDay,
			Buckets: []domain.UsageBucketRow{{Bucket: "2026-04-27", TaskCount: 1, CostUSD: 0.5}},
			Totals:  domain.UsageBucketRow{TaskCount: 1, CostUSD: 0.5},
		},
	}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp api.UsageResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Buckets, 1)
	assert.InDelta(t, 0.5, resp.Totals.CostUSD, 0.001)
}

func TestGetUsage_InvalidFrom_Returns400(t *testing.T) {
	q := &fakeUsageQuerier{}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage?from=invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetUsage_InvalidGroupBy_Returns400(t *testing.T) {
	q := &fakeUsageQuerier{
		aggregateErr: domain.ErrInvalidBucket,
	}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage?group_by=year", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp api.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error, "group_by")
}

func TestGetUsage_FromAfterTo_Returns400(t *testing.T) {
	q := &fakeUsageQuerier{
		aggregateErr: domain.ErrInvalidRange,
	}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage?from=2026-04-27&to=2026-04-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetUsageByModel_Returns200_SortedByCost(t *testing.T) {
	q := &fakeUsageQuerier{
		byModelResult: []domain.UsageModelRow{
			{ModelID: "claude-opus-4-5", CostUSD: 10.0, TaskCount: 5},
			{ModelID: "claude-sonnet-4-5", CostUSD: 3.0, TaskCount: 20},
		},
	}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage/by-model", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp api.UsageByModelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Models, 2)
	assert.Equal(t, "claude-opus-4-5", resp.Models[0].ModelID)
}

func TestGetUsageLimits_Returns200(t *testing.T) {
	pct := 85.0
	q := &fakeUsageQuerier{
		limitsResult: usecase.UsageLimitsSnapshot{
			DailyCountUSD:  0.85,
			DailyMaxUSD:    1.00,
			DailyPercent:   &pct,
			DailyDate:      "2026-04-27",
			WeeklyCountUSD: 4.20,
			WeeklyMaxUSD:   5.00,
			WeeklyPercent:  &pct,
			WeeklyWeek:     "2026-W18",
		},
	}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage/limits", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp api.UsageLimitsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.InDelta(t, 0.85, resp.Daily.CountUSD, 0.001)
	assert.Equal(t, "2026-04-27", resp.Daily.Date)
	require.NotNil(t, resp.Daily.Percent)
	assert.InDelta(t, 85.0, *resp.Daily.Percent, 0.1)
}

func TestGetUsageLimits_MaxZero_NilPercent(t *testing.T) {
	q := &fakeUsageQuerier{
		limitsResult: usecase.UsageLimitsSnapshot{
			DailyCountUSD:  0.0,
			DailyMaxUSD:    0.0,
			DailyPercent:   nil, // max=0 → nil
			DailyDate:      "2026-04-27",
			WeeklyCountUSD: 0.0,
			WeeklyMaxUSD:   0.0,
			WeeklyPercent:  nil,
			WeeklyWeek:     "2026-W18",
		},
	}
	r := setupUsageRouter(q)

	req := httptest.NewRequest(http.MethodGet, "/usage/limits", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Verify percent is JSON null when nil pointer
	body := w.Body.String()
	assert.Contains(t, body, `"percent":null`, "percent should serialize as null when max=0")
}
