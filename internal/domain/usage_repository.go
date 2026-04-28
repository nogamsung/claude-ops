package domain

import (
	"context"
	"time"
)

// BucketKind defines the granularity for time-series aggregation.
type BucketKind string

// BucketDay, BucketWeek, and BucketMonth enumerate the supported aggregation granularities.
const (
	BucketDay   BucketKind = "day"
	BucketWeek  BucketKind = "week"
	BucketMonth BucketKind = "month"
)

// UsageBucketRow holds aggregated usage data for a single time bucket.
type UsageBucketRow struct {
	Bucket              string
	TaskCount           int64
	CostUSD             float64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	FailedCostUSD       float64
}

// UsageModelRow holds aggregated usage data for a single model.
type UsageModelRow struct {
	ModelID             string
	TaskCount           int64
	CostUSD             float64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

// UsageRepository defines storage operations for usage aggregation queries.
type UsageRepository interface {
	SumByBucket(ctx context.Context, from, to time.Time, bucket BucketKind) ([]UsageBucketRow, error)
	SumByModel(ctx context.Context, from, to time.Time) ([]UsageModelRow, error)
	SumDailyCost(ctx context.Context, dayKey string) (float64, error)
	SumWeeklyCost(ctx context.Context, weekKey string) (float64, error)
}
