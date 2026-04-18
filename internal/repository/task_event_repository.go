package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// gormTaskEvent is the GORM model for the task_events table.
type gormTaskEvent struct {
	ID          string    `gorm:"column:id;primaryKey"`
	TaskID      string    `gorm:"column:task_id"`
	Kind        string    `gorm:"column:kind"`
	PayloadJSON string    `gorm:"column:payload_json"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (gormTaskEvent) TableName() string { return "task_events" }

func toGORMTaskEvent(e *domain.TaskEvent) *gormTaskEvent {
	return &gormTaskEvent{
		ID:          e.ID,
		TaskID:      e.TaskID,
		Kind:        string(e.Kind),
		PayloadJSON: e.PayloadJSON,
		CreatedAt:   e.CreatedAt,
	}
}

func toDomainTaskEvent(g *gormTaskEvent) *domain.TaskEvent {
	return &domain.TaskEvent{
		ID:          g.ID,
		TaskID:      g.TaskID,
		Kind:        domain.EventKind(g.Kind),
		PayloadJSON: g.PayloadJSON,
		CreatedAt:   g.CreatedAt,
	}
}

// SQLiteTaskEventRepository implements domain.TaskEventRepository.
type SQLiteTaskEventRepository struct {
	db *gorm.DB
}

// NewSQLiteTaskEventRepository creates a new SQLiteTaskEventRepository.
func NewSQLiteTaskEventRepository(db *gorm.DB) *SQLiteTaskEventRepository {
	return &SQLiteTaskEventRepository{db: db}
}

// Create inserts a new task event.
func (r *SQLiteTaskEventRepository) Create(ctx context.Context, event *domain.TaskEvent) error {
	if err := r.db.WithContext(ctx).Create(toGORMTaskEvent(event)).Error; err != nil {
		return fmt.Errorf("create task event: %w", err)
	}
	return nil
}

// ListByTaskID returns events for a task, most-recent first, limited to limit rows.
func (r *SQLiteTaskEventRepository) ListByTaskID(ctx context.Context, taskID string, limit int) ([]*domain.TaskEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var rows []gormTaskEvent
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	events := make([]*domain.TaskEvent, len(rows))
	for i, row := range rows {
		row := row
		events[i] = toDomainTaskEvent(&row)
	}
	return events, nil
}
