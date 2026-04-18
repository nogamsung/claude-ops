package usecase_test

import (
	"context"
	"testing"

	"github.com/gs97ahn/claude-ops/internal/usecase"
)

func TestModeUseCase_GetFullMode_Default(t *testing.T) {
	uc := usecase.NewModeUseCase(&fakeAppStateRepo{})
	state, err := uc.GetFullMode(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Enabled {
		t.Error("expected default full mode to be false")
	}
}

func TestModeUseCase_SetAndGetFullMode(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewModeUseCase(repo)

	result, err := uc.SetFullMode(context.Background(), true)
	if err != nil {
		t.Fatalf("SetFullMode error: %v", err)
	}
	if !result.Enabled {
		t.Error("expected enabled=true")
	}
	if result.Since == nil {
		t.Error("expected Since to be set when enabling")
	}

	// Now read back
	state, err := uc.GetFullMode(context.Background())
	if err != nil {
		t.Fatalf("GetFullMode error: %v", err)
	}
	if !state.Enabled {
		t.Error("expected persisted enabled=true")
	}
}

func TestModeUseCase_DisableFullMode(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewModeUseCase(repo)

	if _, err := uc.SetFullMode(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.SetFullMode(context.Background(), false); err != nil {
		t.Fatal(err)
	}

	state, err := uc.GetFullMode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled {
		t.Error("expected disabled=false after SetFullMode(false)")
	}
	if state.Since != nil {
		t.Error("Since should be nil when disabled")
	}
}
