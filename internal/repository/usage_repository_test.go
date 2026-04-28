package repository_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlcdb "github.com/gs97ahn/claude-ops/db/sqlc"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/repository"
)

// setupUsage creates a fresh SQLite DB with migrations applied and returns both
// the task repo and the usage repo backed by the same DB.
func setupUsage(t *testing.T) (*repository.SQLiteTaskRepository, *repository.SQLiteUsageRepository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "usage_test.db")
	db, err := repository.NewDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { s, _ := db.DB(); s.Close() })

	sqlDB, err := db.DB()
	require.NoError(t, err)

	driver, err := migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{})
	require.NoError(t, err, "migrate driver")

	migrationsDir := findMigrationsDir(t)
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "sqlite3", driver)
	require.NoError(t, err, "migrate init")
	if upErr := m.Up(); upErr != nil && upErr != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", upErr)
	}

	queries := sqlcdb.New(sqlDB)
	return repository.NewSQLiteTaskRepository(db), repository.NewSQLiteUsageRepository(queries)
}

func insertDoneTask(t *testing.T, taskRepo *repository.SQLiteTaskRepository, finishedAt time.Time, costUSD float64, modelUsage map[string]interface{}) {
	t.Helper()
	mjson := "{}"
	if len(modelUsage) > 0 {
		b, _ := json.Marshal(modelUsage)
		mjson = string(b)
	}
	task := &domain.Task{
		ID:                       uuid.New().String(),
		RepoFullName:             "owner/repo",
		IssueNumber:              1,
		Status:                   domain.TaskStatusDone,
		TaskType:                 domain.TaskTypeFeature,
		Source:                   domain.TaskSourceGitHubIssue,
		FinishedAt:               &finishedAt,
		CostUSD:                  costUSD,
		TotalInputTokens:         1000,
		TotalOutputTokens:        500,
		CacheReadInputTokens:     100,
		CacheCreationInputTokens: 50,
		ModelUsageJSON:           mjson,
		CreatedAt:                finishedAt,
		UpdatedAt:                finishedAt,
	}
	require.NoError(t, taskRepo.Create(context.Background(), task))
}

func insertFailedTask(t *testing.T, taskRepo *repository.SQLiteTaskRepository, finishedAt time.Time, costUSD float64) {
	t.Helper()
	task := &domain.Task{
		ID:             uuid.New().String(),
		RepoFullName:   "owner/repo",
		Status:         domain.TaskStatusFailed,
		TaskType:       domain.TaskTypeFeature,
		Source:         domain.TaskSourceGitHubIssue,
		FinishedAt:     &finishedAt,
		CostUSD:        costUSD,
		ModelUsageJSON: "{}",
		CreatedAt:      finishedAt,
		UpdatedAt:      finishedAt,
	}
	require.NoError(t, taskRepo.Create(context.Background(), task))
}

func TestUsageRepository_SumByBucket_Day_ThreeTasks(t *testing.T) {
	taskRepo, usageRepo := setupUsage(t)

	day := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	insertDoneTask(t, taskRepo, day, 0.30, nil)
	insertDoneTask(t, taskRepo, day, 0.20, nil)
	insertDoneTask(t, taskRepo, day, 0.10, nil)
	insertFailedTask(t, taskRepo, day, 0.05) // failed: goes to failed_cost_usd only

	from := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	rows, err := usageRepo.SumByBucket(context.Background(), from, to, domain.BucketDay)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "2026-04-27", rows[0].Bucket)
	assert.Equal(t, int64(3), rows[0].TaskCount, "only done tasks counted")
	assert.InDelta(t, 0.60, rows[0].CostUSD, 0.001, "done cost_usd sum")
	assert.InDelta(t, 0.05, rows[0].FailedCostUSD, 0.001, "failed cost separated")
}

func TestUsageRepository_SumByBucket_EmptyRange(t *testing.T) {
	_, usageRepo := setupUsage(t)

	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	rows, err := usageRepo.SumByBucket(context.Background(), from, to, domain.BucketDay)
	require.NoError(t, err)
	assert.Empty(t, rows, "empty range returns empty slice — gap-fill is usecase responsibility")
}

func TestUsageRepository_SumByModel_ModelBreakdown(t *testing.T) {
	taskRepo, usageRepo := setupUsage(t)

	day := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)

	// Task with two models in JSON breakdown
	modelAB := map[string]interface{}{
		"modelA": map[string]interface{}{"costUSD": 0.10, "inputTokens": 100, "outputTokens": 50, "cacheReadInputTokens": 0, "cacheCreationInputTokens": 0},
		"modelB": map[string]interface{}{"costUSD": 0.20, "inputTokens": 200, "outputTokens": 100, "cacheReadInputTokens": 0, "cacheCreationInputTokens": 0},
	}
	insertDoneTask(t, taskRepo, day, 0.30, modelAB)

	// Task without model breakdown — cost attributed to "unknown"
	insertDoneTask(t, taskRepo, day, 0.05, nil)

	from := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)
	rows, err := usageRepo.SumByModel(context.Background(), from, to)
	require.NoError(t, err)

	modelMap := make(map[string]domain.UsageModelRow)
	for _, r := range rows {
		modelMap[r.ModelID] = r
	}
	assert.Contains(t, modelMap, "modelA")
	assert.Contains(t, modelMap, "modelB")
	assert.Contains(t, modelMap, "unknown", "empty model_usage_json row goes to 'unknown'")

	// Verify sorted by cost desc
	for i := 1; i < len(rows); i++ {
		assert.GreaterOrEqual(t, rows[i-1].CostUSD, rows[i].CostUSD, "should be sorted descending by cost")
	}
}

func TestUsageRepository_SumDailyCost(t *testing.T) {
	taskRepo, usageRepo := setupUsage(t)

	day := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	insertDoneTask(t, taskRepo, day, 0.40, nil)
	insertDoneTask(t, taskRepo, day, 0.35, nil)
	insertFailedTask(t, taskRepo, day, 0.10) // should not be counted

	total, err := usageRepo.SumDailyCost(context.Background(), "2026-04-27")
	require.NoError(t, err)
	assert.InDelta(t, 0.75, total, 0.001, "only done tasks counted in daily cost")
}

func TestUsageRepository_SumWeeklyCost(t *testing.T) {
	taskRepo, usageRepo := setupUsage(t)

	// 2026-04-27 is a Monday (week 17 when counting from Mon)
	day1 := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	insertDoneTask(t, taskRepo, day1, 0.50, nil)
	insertDoneTask(t, taskRepo, day2, 0.25, nil)

	// SQLite strftime('%Y-W%W', date) uses Sunday-based week numbers (0-based)
	// Week 17 in that system for 2026-04-27 Monday.
	total, err := usageRepo.SumWeeklyCost(context.Background(), "2026-W18")
	require.NoError(t, err)
	assert.InDelta(t, 0.75, total, 0.001, "both tasks in same week counted")
}

func TestUsageRepository_SumByBucket_Week(t *testing.T) {
	taskRepo, usageRepo := setupUsage(t)

	day := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	insertDoneTask(t, taskRepo, day, 0.30, nil)

	from := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	rows, err := usageRepo.SumByBucket(context.Background(), from, to, domain.BucketWeek)
	require.NoError(t, err)
	assert.NotEmpty(t, rows, "should have at least one week bucket")
}

func TestUsageRepository_SumByBucket_Month(t *testing.T) {
	taskRepo, usageRepo := setupUsage(t)

	day := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	insertDoneTask(t, taskRepo, day, 0.30, nil)

	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows, err := usageRepo.SumByBucket(context.Background(), from, to, domain.BucketMonth)
	require.NoError(t, err)
	assert.NotEmpty(t, rows, "should have at least one month bucket")
}

func TestUsageRepository_SumByBucket_InvalidBucket(t *testing.T) {
	_, usageRepo := setupUsage(t)
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	_, err := usageRepo.SumByBucket(context.Background(), from, to, domain.BucketKind("year"))
	assert.ErrorIs(t, err, domain.ErrInvalidBucket)
}
