package repository_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	sqlcdb "github.com/gs97ahn/claude-ops/db/sqlc"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/repository"
	"github.com/gs97ahn/claude-ops/testutil"
)

func setupDB(t *testing.T) (*repository.GormTaskRepository, *repository.GormTaskEventRepository, *repository.GormAppStateRepository) {
	t.Helper()
	db := testutil.NewTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	queries := sqlcdb.New(sqlDB)
	return repository.NewGormTaskRepository(db, queries),
		repository.NewGormTaskEventRepository(db),
		repository.NewGormAppStateRepository(db)
}

func TestTaskRepository_CreateAndGetByID(t *testing.T) {
	taskRepo, _, _ := setupDB(t)

	task := &domain.Task{
		ID:           uuid.New().String(),
		RepoFullName: "owner/repo",
		IssueNumber:  42,
		IssueTitle:   "Test issue",
		TaskType:     domain.TaskTypeFeature,
		Status:       domain.TaskStatusQueued,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := taskRepo.GetByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.IssueNumber != 42 {
		t.Errorf("expected issue 42, got %d", got.IssueNumber)
	}
	if got.Status != domain.TaskStatusQueued {
		t.Errorf("expected queued, got %s", got.Status)
	}
}

func TestTaskRepository_GetByID_NotFound(t *testing.T) {
	taskRepo, _, _ := setupDB(t)
	_, err := taskRepo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTaskRepository_Update(t *testing.T) {
	taskRepo, _, _ := setupDB(t)

	task := &domain.Task{
		ID:        uuid.New().String(),
		TaskType:  domain.TaskTypeFeature,
		Status:    domain.TaskStatusQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	task.Status = domain.TaskStatusRunning
	if err := taskRepo.Update(context.Background(), task); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := taskRepo.GetByID(context.Background(), task.ID)
	if got.Status != domain.TaskStatusRunning {
		t.Errorf("expected running, got %s", got.Status)
	}
}

func TestTaskRepository_List_FilterByStatus(t *testing.T) {
	taskRepo, _, _ := setupDB(t)

	for _, status := range []domain.TaskStatus{domain.TaskStatusQueued, domain.TaskStatusDone, domain.TaskStatusQueued} {
		if err := taskRepo.Create(context.Background(), &domain.Task{
			ID:        uuid.New().String(),
			TaskType:  domain.TaskTypeFeature,
			Status:    status,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	queued := domain.TaskStatusQueued
	tasks, err := taskRepo.List(context.Background(), domain.TaskFilter{Status: &queued})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 queued tasks, got %d", len(tasks))
	}
}

func TestTaskRepository_ExistsByRepoAndIssue(t *testing.T) {
	taskRepo, _, _ := setupDB(t)

	if err := taskRepo.Create(context.Background(), &domain.Task{
		ID:           uuid.New().String(),
		RepoFullName: "owner/repo",
		IssueNumber:  7,
		TaskType:     domain.TaskTypeFeature,
		Status:       domain.TaskStatusQueued,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	exists, err := taskRepo.ExistsByRepoAndIssue(context.Background(), "owner/repo", 7)
	if err != nil {
		t.Fatalf("ExistsByRepoAndIssue: %v", err)
	}
	if !exists {
		t.Error("expected task to exist")
	}

	notExists, _ := taskRepo.ExistsByRepoAndIssue(context.Background(), "owner/repo", 999)
	if notExists {
		t.Error("expected non-existent issue to return false")
	}
}

func TestTaskEventRepository_CreateAndList(t *testing.T) {
	taskRepo, eventRepo, _ := setupDB(t)

	task := &domain.Task{
		ID:        uuid.New().String(),
		TaskType:  domain.TaskTypeFeature,
		Status:    domain.TaskStatusQueued,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	event := &domain.TaskEvent{
		ID:          uuid.New().String(),
		TaskID:      task.ID,
		Kind:        domain.EventKindStarted,
		PayloadJSON: `{"test":true}`,
		CreatedAt:   time.Now(),
	}
	if err := eventRepo.Create(context.Background(), event); err != nil {
		t.Fatalf("Create event: %v", err)
	}

	events, err := eventRepo.ListByTaskID(context.Background(), task.ID, 10)
	if err != nil {
		t.Fatalf("ListByTaskID: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Kind != domain.EventKindStarted {
		t.Errorf("expected started kind, got %s", events[0].Kind)
	}
}

func TestAppStateRepository_SetAndGet(t *testing.T) {
	_, _, appStateRepo := setupDB(t)

	state := &domain.AppState{
		Key:       "full_mode",
		ValueJSON: `{"enabled":true}`,
		UpdatedAt: time.Now(),
	}
	if err := appStateRepo.Set(context.Background(), state); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := appStateRepo.Get(context.Background(), "full_mode")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// MySQL JSON columns canonicalize whitespace on read, so compare
	// semantically rather than byte-for-byte.
	if !jsonEqual(t, got.ValueJSON, `{"enabled":true}`) {
		t.Errorf("unexpected value: %s", got.ValueJSON)
	}
}

func TestAppStateRepository_Get_NotFound(t *testing.T) {
	_, _, appStateRepo := setupDB(t)
	_, err := appStateRepo.Get(context.Background(), "nonexistent_key")
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAppStateRepository_Upsert(t *testing.T) {
	_, _, appStateRepo := setupDB(t)

	if err := appStateRepo.Set(context.Background(), &domain.AppState{
		Key: "full_mode", ValueJSON: `{"enabled":false}`, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := appStateRepo.Set(context.Background(), &domain.AppState{
		Key: "full_mode", ValueJSON: `{"enabled":true}`, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, _ := appStateRepo.Get(context.Background(), "full_mode")
	if !jsonEqual(t, got.ValueJSON, `{"enabled":true}`) {
		t.Errorf("expected upserted value, got %s", got.ValueJSON)
	}
}

// jsonEqual reports whether two JSON strings are semantically equal
// (whitespace-insensitive). Needed because MySQL JSON columns reformat
// the stored bytes on read.
func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		t.Fatalf("jsonEqual: bad lhs %q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		t.Fatalf("jsonEqual: bad rhs %q: %v", b, err)
	}
	return reflect.DeepEqual(av, bv)
}
