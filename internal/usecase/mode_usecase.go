package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// FullModeState represents the persisted full-mode configuration.
type FullModeState struct {
	Enabled   bool       `json:"enabled"`
	Since     *time.Time `json:"since,omitempty"`
}

// ModeUseCase manages the full-usage-mode state.
type ModeUseCase struct {
	appStateRepo domain.AppStateRepository
}

// NewModeUseCase creates a ModeUseCase.
func NewModeUseCase(appStateRepo domain.AppStateRepository) *ModeUseCase {
	return &ModeUseCase{appStateRepo: appStateRepo}
}

// GetFullMode returns the current full-mode state.
func (uc *ModeUseCase) GetFullMode(ctx context.Context) (*FullModeState, error) {
	state, err := uc.appStateRepo.Get(ctx, "full_mode")
	if err != nil {
		if err == domain.ErrNotFound {
			return &FullModeState{Enabled: false}, nil
		}
		return nil, fmt.Errorf("get full_mode: %w", err)
	}
	var fs FullModeState
	if err = json.Unmarshal([]byte(state.ValueJSON), &fs); err != nil {
		return &FullModeState{Enabled: false}, nil
	}
	return &fs, nil
}

// SetFullMode persists the full-mode enabled state.
func (uc *ModeUseCase) SetFullMode(ctx context.Context, enabled bool) (*FullModeState, error) {
	now := time.Now()
	fs := FullModeState{Enabled: enabled}
	if enabled {
		fs.Since = &now
	}

	b, err := json.Marshal(fs)
	if err != nil {
		return nil, fmt.Errorf("marshal full_mode: %w", err)
	}

	appState := &domain.AppState{
		Key:       "full_mode",
		ValueJSON: string(b),
		UpdatedAt: now,
	}
	if err = uc.appStateRepo.Set(ctx, appState); err != nil {
		return nil, fmt.Errorf("set full_mode: %w", err)
	}
	return &fs, nil
}
