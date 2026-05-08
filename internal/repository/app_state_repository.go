package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// gormAppState is the GORM model for the app_states table.
type gormAppState struct {
	Key       string    `gorm:"column:key;primaryKey"`
	ValueJSON string    `gorm:"column:value_json"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (gormAppState) TableName() string { return "app_states" }

// GormAppStateRepository implements domain.AppStateRepository.
type GormAppStateRepository struct {
	db *gorm.DB
}

// NewGormAppStateRepository creates a new GormAppStateRepository.
func NewGormAppStateRepository(db *gorm.DB) *GormAppStateRepository {
	return &GormAppStateRepository{db: db}
}

// Get retrieves a state entry by key. The column name `key` is a MySQL
// reserved word, so we escape it with backticks in the raw WHERE clause.
func (r *GormAppStateRepository) Get(ctx context.Context, key string) (*domain.AppState, error) {
	var g gormAppState
	if err := r.db.WithContext(ctx).First(&g, "`key` = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get app state %q: %w", key, err)
	}
	return &domain.AppState{Key: g.Key, ValueJSON: g.ValueJSON, UpdatedAt: g.UpdatedAt}, nil
}

// Set upserts a state entry.
func (r *GormAppStateRepository) Set(ctx context.Context, state *domain.AppState) error {
	g := &gormAppState{
		Key:       state.Key,
		ValueJSON: state.ValueJSON,
		UpdatedAt: time.Now(),
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
	}).Create(g).Error; err != nil {
		return fmt.Errorf("set app state %q: %w", state.Key, err)
	}
	return nil
}
