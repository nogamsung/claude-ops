package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"

	"github.com/google/uuid"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
	"github.com/gs97ahn/scheduled-dev-agent/internal/repository"
)

func setupDB(t *testing.T) (*repository.SQLiteTaskRepository, *repository.SQLiteTaskEventRepository, *repository.SQLiteAppStateRepository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := repository.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close() })

	// Run migrations.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	driver, err := migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{})
	if err != nil {
		t.Fatalf("migrate driver: %v", err)
	}

	// Find migrations directory.
	migrationsDir := findMigrationsDir(t)
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "sqlite3", driver)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err = m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	return repository.NewSQLiteTaskRepository(db),
		repository.NewSQLiteTaskEventRepository(db),
		repository.NewSQLiteAppStateRepository(db)
}

func findMigrationsDir(t *testing.T) string {
	t.Helper()
	// Walk up from current dir to find migrations/
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find migrations directory")
		}
		dir = parent
	}
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
	_, err := taskRepo.GetByID(context.Background(), "nonexistent")
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
	if got.ValueJSON != `{"enabled":true}` {
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
	if got.ValueJSON != `{"enabled":true}` {
		t.Errorf("expected upserted value, got %s", got.ValueJSON)
	}
}
