package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

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
	}
}

// SQLiteTaskRepository implements domain.TaskRepository using GORM + SQLite.
type SQLiteTaskRepository struct {
	db *gorm.DB
}

// NewSQLiteTaskRepository creates a new SQLiteTaskRepository.
func NewSQLiteTaskRepository(db *gorm.DB) *SQLiteTaskRepository {
	return &SQLiteTaskRepository{db: db}
}

// Create inserts a new task.
func (r *SQLiteTaskRepository) Create(ctx context.Context, task *domain.Task) error {
	result := r.db.WithContext(ctx).Create(toGORMTask(task))
	if result.Error != nil {
		return fmt.Errorf("create task: %w", result.Error)
	}
	return nil
}

// GetByID fetches a task by its ID.
func (r *SQLiteTaskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
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
func (r *SQLiteTaskRepository) Update(ctx context.Context, task *domain.Task) error {
	g := toGORMTask(task)
	result := r.db.WithContext(ctx).Save(g)
	if result.Error != nil {
		return fmt.Errorf("update task: %w", result.Error)
	}
	return nil
}

// List returns tasks filtered by the given criteria.
// Complex filtering uses raw SQL via GORM to stay consistent without sqlc for this simple case.
func (r *SQLiteTaskRepository) List(ctx context.Context, filter domain.TaskFilter) ([]*domain.Task, error) {
	query := r.db.WithContext(ctx).Model(&gormTask{})

	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.Source != nil {
		query = query.Where("source = ?", string(*filter.Source))
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
func (r *SQLiteTaskRepository) GetRunning(ctx context.Context) ([]*domain.Task, error) {
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
func (r *SQLiteTaskRepository) ExistsByRepoAndIssue(ctx context.Context, repoFullName string, issueNumber int) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&gormTask{}).
		Where("repo_full_name = ? AND issue_number = ? AND status IN ('queued','running')", repoFullName, issueNumber).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("exists by repo and issue: %w", err)
	}
	return count > 0, nil
}
