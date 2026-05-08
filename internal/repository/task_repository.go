package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	sqlcdb "github.com/gs97ahn/claude-ops/db/sqlc"
	"github.com/gs97ahn/claude-ops/internal/domain"
)

// gormTask is the GORM model for the tasks table.
type gormTask struct {
	ID                       string     `gorm:"column:id;primaryKey"`
	RepoFullName             string     `gorm:"column:repo_full_name"`
	IssueNumber              int        `gorm:"column:issue_number"`
	IssueTitle               string     `gorm:"column:issue_title"`
	TaskType                 string     `gorm:"column:task_type"`
	Status                   string     `gorm:"column:status"`
	Source                   string     `gorm:"column:source"`
	MaintenanceName          string     `gorm:"column:maintenance_name"`
	PromptTemplate           string     `gorm:"column:prompt_template"`
	WorktreePath             string     `gorm:"column:worktree_path"`
	PRURL                    string     `gorm:"column:pr_url"`
	PRNumber                 int        `gorm:"column:pr_number"`
	StartedAt                *time.Time `gorm:"column:started_at"`
	FinishedAt               *time.Time `gorm:"column:finished_at"`
	EstimatedInputTokens     int        `gorm:"column:estimated_input_tokens"`
	EstimatedOutputTokens    int        `gorm:"column:estimated_output_tokens"`
	ExitCode                 *int       `gorm:"column:exit_code"`
	StderrTail               string     `gorm:"column:stderr_tail"`
	CreatedAt                time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	CostUSD                  float64    `gorm:"column:cost_usd"`
	TotalInputTokens         int64      `gorm:"column:total_input_tokens"`
	TotalOutputTokens        int64      `gorm:"column:total_output_tokens"`
	CacheCreationInputTokens int64      `gorm:"column:cache_creation_input_tokens"`
	CacheReadInputTokens     int64      `gorm:"column:cache_read_input_tokens"`
	ModelUsageJSON           string     `gorm:"column:model_usage_json"`
	WorkerID                 *string    `gorm:"column:worker_id"`
	ClaimedAt                *time.Time `gorm:"column:claimed_at"`
	ParentTaskID             *string    `gorm:"column:parent_task_id"`
	FixAttemptCount          int        `gorm:"column:fix_attempt_count"`
	CIStatus                 string     `gorm:"column:ci_status"`
	HeadSHA                  string     `gorm:"column:head_sha"`
	CILastPolledAt           *time.Time `gorm:"column:ci_last_polled_at"`
}

func (gormTask) TableName() string { return "tasks" }

func toGORMTask(t *domain.Task) *gormTask {
	src := string(t.Source)
	if src == "" {
		src = string(domain.TaskSourceGitHubIssue)
	}
	modelJSON := t.ModelUsageJSON
	if modelJSON == "" {
		modelJSON = "{}"
	}
	return &gormTask{
		ID:                       t.ID,
		RepoFullName:             t.RepoFullName,
		IssueNumber:              t.IssueNumber,
		IssueTitle:               t.IssueTitle,
		TaskType:                 string(t.TaskType),
		Status:                   string(t.Status),
		Source:                   src,
		MaintenanceName:          t.MaintenanceName,
		PromptTemplate:           t.PromptTemplate,
		WorktreePath:             t.WorktreePath,
		PRURL:                    t.PRURL,
		PRNumber:                 t.PRNumber,
		StartedAt:                t.StartedAt,
		FinishedAt:               t.FinishedAt,
		EstimatedInputTokens:     t.EstimatedInputTokens,
		EstimatedOutputTokens:    t.EstimatedOutputTokens,
		ExitCode:                 t.ExitCode,
		StderrTail:               t.StderrTail,
		CreatedAt:                t.CreatedAt,
		UpdatedAt:                t.UpdatedAt,
		CostUSD:                  t.CostUSD,
		TotalInputTokens:         t.TotalInputTokens,
		TotalOutputTokens:        t.TotalOutputTokens,
		CacheCreationInputTokens: t.CacheCreationInputTokens,
		CacheReadInputTokens:     t.CacheReadInputTokens,
		ModelUsageJSON:           modelJSON,
		WorkerID:                 t.WorkerID,
		ClaimedAt:                t.ClaimedAt,
		ParentTaskID:             t.ParentTaskID,
		FixAttemptCount:          t.FixAttemptCount,
		CIStatus:                 string(t.CIStatus),
		HeadSHA:                  t.HeadSHA,
		CILastPolledAt:           t.CILastPolledAt,
	}
}

func toDomainTask(g *gormTask) *domain.Task {
	src := domain.TaskSource(g.Source)
	if src == "" {
		src = domain.TaskSourceGitHubIssue
	}
	modelJSON := g.ModelUsageJSON
	if modelJSON == "" {
		modelJSON = "{}"
	}
	return &domain.Task{
		ID:                       g.ID,
		RepoFullName:             g.RepoFullName,
		IssueNumber:              g.IssueNumber,
		IssueTitle:               g.IssueTitle,
		TaskType:                 domain.TaskType(g.TaskType),
		Status:                   domain.TaskStatus(g.Status),
		Source:                   src,
		MaintenanceName:          g.MaintenanceName,
		PromptTemplate:           g.PromptTemplate,
		WorktreePath:             g.WorktreePath,
		PRURL:                    g.PRURL,
		PRNumber:                 g.PRNumber,
		StartedAt:                g.StartedAt,
		FinishedAt:               g.FinishedAt,
		EstimatedInputTokens:     g.EstimatedInputTokens,
		EstimatedOutputTokens:    g.EstimatedOutputTokens,
		ExitCode:                 g.ExitCode,
		StderrTail:               g.StderrTail,
		CreatedAt:                g.CreatedAt,
		UpdatedAt:                g.UpdatedAt,
		CostUSD:                  g.CostUSD,
		TotalInputTokens:         g.TotalInputTokens,
		TotalOutputTokens:        g.TotalOutputTokens,
		CacheCreationInputTokens: g.CacheCreationInputTokens,
		CacheReadInputTokens:     g.CacheReadInputTokens,
		ModelUsageJSON:           modelJSON,
		WorkerID:                 g.WorkerID,
		ClaimedAt:                g.ClaimedAt,
		ParentTaskID:             g.ParentTaskID,
		FixAttemptCount:          g.FixAttemptCount,
		CIStatus:                 domain.CIStatus(g.CIStatus),
		HeadSHA:                  g.HeadSHA,
		CILastPolledAt:           g.CILastPolledAt,
	}
}

// GormTaskRepository implements domain.TaskRepository using GORM on MySQL.
// The sqlc Queries handle is used only for the concurrency-sensitive
// ClaimNext / ReclaimStale paths that need MySQL-specific FOR UPDATE
// SKIP LOCKED + NOT EXISTS semantics; everything else stays on GORM.
type GormTaskRepository struct {
	db      *gorm.DB
	queries *sqlcdb.Queries
}

// NewGormTaskRepository creates a new GormTaskRepository. queries may be nil
// for callers that never invoke ClaimNext/ReclaimStale (e.g. legacy tests),
// in which case those methods return an error.
func NewGormTaskRepository(db *gorm.DB, queries *sqlcdb.Queries) *GormTaskRepository {
	return &GormTaskRepository{db: db, queries: queries}
}

// Create inserts a new task.
func (r *GormTaskRepository) Create(ctx context.Context, task *domain.Task) error {
	result := r.db.WithContext(ctx).Create(toGORMTask(task))
	if result.Error != nil {
		return fmt.Errorf("create task: %w", result.Error)
	}
	return nil
}

// GetByID fetches a task by its ID.
func (r *GormTaskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	var g gormTask
	result := r.db.WithContext(ctx).First(&g, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get task by id: %w", result.Error)
	}
	return toDomainTask(&g), nil
}

// Update saves all fields of a task.
func (r *GormTaskRepository) Update(ctx context.Context, task *domain.Task) error {
	g := toGORMTask(task)
	result := r.db.WithContext(ctx).Save(g)
	if result.Error != nil {
		return fmt.Errorf("update task: %w", result.Error)
	}
	return nil
}

// List returns tasks filtered by the given criteria.
// Complex filtering uses raw SQL via GORM to stay consistent without sqlc for this simple case.
func (r *GormTaskRepository) List(ctx context.Context, filter domain.TaskFilter) ([]*domain.Task, error) {
	query := r.db.WithContext(ctx).Model(&gormTask{})

	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.Source != nil {
		query = query.Where("source = ?", string(*filter.Source))
	}
	if filter.CIStatus != nil {
		query = query.Where("ci_status = ?", string(*filter.CIStatus))
	}
	if filter.Cursor != "" {
		query = query.Where("id < ?", filter.Cursor)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query = query.Order("created_at DESC").Limit(limit)

	var rows []gormTask
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	tasks := make([]*domain.Task, len(rows))
	for i, row := range rows {
		row := row
		tasks[i] = toDomainTask(&row)
	}
	return tasks, nil
}

// GetRunning returns all tasks with status=running.
func (r *GormTaskRepository) GetRunning(ctx context.Context) ([]*domain.Task, error) {
	var rows []gormTask
	if err := r.db.WithContext(ctx).Where("status = ?", string(domain.TaskStatusRunning)).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("get running tasks: %w", err)
	}

	tasks := make([]*domain.Task, len(rows))
	for i, row := range rows {
		row := row
		tasks[i] = toDomainTask(&row)
	}
	return tasks, nil
}

// ExistsByRepoAndIssue reports whether a non-terminal task exists for the given repo+issue.
func (r *GormTaskRepository) ExistsByRepoAndIssue(ctx context.Context, repoFullName string, issueNumber int) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&gormTask{}).
		Where("repo_full_name = ? AND issue_number = ? AND status IN ('queued','running')", repoFullName, issueNumber).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("exists by repo and issue: %w", err)
	}
	return count > 0, nil
}

// ClaimNext atomically picks the oldest queued task whose repo has no
// running task, flips it to running with the given workerID, and returns the
// loaded row. Uses MySQL FOR UPDATE SKIP LOCKED so concurrent workers do not
// step on each other. Returns (nil, nil) when no eligible row exists.
func (r *GormTaskRepository) ClaimNext(ctx context.Context, workerID string) (*domain.Task, error) {
	if r.queries == nil {
		return nil, fmt.Errorf("ClaimNext: sqlc queries not wired")
	}
	wid := sql.NullString{String: workerID, Valid: workerID != ""}
	rows, err := r.queries.ClaimNextTask(ctx, wid)
	if err != nil {
		return nil, fmt.Errorf("claim next task: %w", err)
	}
	if rows == 0 {
		return nil, nil
	}
	taskID, err := r.queries.GetClaimedTaskID(ctx, wid)
	if err != nil {
		return nil, fmt.Errorf("load claimed task id: %w", err)
	}
	return r.GetByID(ctx, taskID)
}

// ReclaimStale marks running rows whose claim is older than cutoff as
// orphaned and clears their worker_id, so a periodic safety net can recover
// after a crashed worker.
func (r *GormTaskRepository) ReclaimStale(ctx context.Context, cutoff time.Time) (int64, error) {
	if r.queries == nil {
		return 0, fmt.Errorf("ReclaimStale: sqlc queries not wired")
	}
	rows, err := r.queries.ReclaimStaleTasks(ctx, sql.NullTime{Time: cutoff, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("reclaim stale tasks: %w", err)
	}
	return rows, nil
}

// ListWatchingIDs returns IDs of tasks the CI watcher should poll, oldest
// poll first (NULLs ahead of any timestamp).
func (r *GormTaskRepository) ListWatchingIDs(ctx context.Context) ([]string, error) {
	if r.queries == nil {
		return nil, fmt.Errorf("ListWatchingIDs: sqlc queries not wired")
	}
	ids, err := r.queries.ListWatchingTaskIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list watching task ids: %w", err)
	}
	return ids, nil
}

// FindFixChild returns the existing ci-fix child task for a (parent,
// head_sha) pair, or (nil, nil) when no duplicate exists.
func (r *GormTaskRepository) FindFixChild(ctx context.Context, parentTaskID, headSHA string) (*domain.Task, error) {
	if r.queries == nil {
		return nil, fmt.Errorf("FindFixChild: sqlc queries not wired")
	}
	id, err := r.queries.FindFixChildTaskID(ctx, sqlcdb.FindFixChildTaskIDParams{
		ParentTaskID: sql.NullString{String: parentTaskID, Valid: parentTaskID != ""},
		HeadSha:      headSHA,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find fix child: %w", err)
	}
	return r.GetByID(ctx, id)
}

// UpdateCIStatus advances the watcher fields without rewriting the rest of
// the row. status="" is allowed (legacy clear).
func (r *GormTaskRepository) UpdateCIStatus(ctx context.Context, id string, status domain.CIStatus, headSHA string, polledAt time.Time) error {
	if r.queries == nil {
		return fmt.Errorf("UpdateCIStatus: sqlc queries not wired")
	}
	if err := r.queries.UpdateTaskCIStatus(ctx, sqlcdb.UpdateTaskCIStatusParams{
		CiStatus:       string(status),
		HeadSha:        headSHA,
		CiLastPolledAt: sql.NullTime{Time: polledAt, Valid: !polledAt.IsZero()},
		ID:             id,
	}); err != nil {
		return fmt.Errorf("update ci status: %w", err)
	}
	return nil
}
