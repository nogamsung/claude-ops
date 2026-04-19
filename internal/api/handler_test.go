package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/api"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakes

type fakeTaskRepo struct {
	tasks  []*domain.Task
	exists bool
}

func (r *fakeTaskRepo) Create(_ context.Context, t *domain.Task) error {
	r.tasks = append(r.tasks, t)
	return nil
}
func (r *fakeTaskRepo) GetByID(_ context.Context, id string) (*domain.Task, error) {
	for _, t := range r.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeTaskRepo) Update(_ context.Context, t *domain.Task) error {
	for i, existing := range r.tasks {
		if existing.ID == t.ID {
			r.tasks[i] = t
		}
	}
	return nil
}
func (r *fakeTaskRepo) List(_ context.Context, f domain.TaskFilter) ([]*domain.Task, error) {
	var out []*domain.Task
	for _, t := range r.tasks {
		if f.Status != nil && t.Status != *f.Status {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
func (r *fakeTaskRepo) GetRunning(_ context.Context) ([]*domain.Task, error) { return nil, nil }
func (r *fakeTaskRepo) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return r.exists, nil
}

type fakeEventRepo struct{}

func (r *fakeEventRepo) Create(_ context.Context, _ *domain.TaskEvent) error { return nil }
func (r *fakeEventRepo) ListByTaskID(_ context.Context, _ string, _ int) ([]*domain.TaskEvent, error) {
	return nil, nil
}

type fakeAppStateRepo struct {
	states map[string]*domain.AppState
}

func (r *fakeAppStateRepo) Get(_ context.Context, key string) (*domain.AppState, error) {
	if r.states == nil {
		return nil, domain.ErrNotFound
	}
	s, ok := r.states[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}
func (r *fakeAppStateRepo) Set(_ context.Context, s *domain.AppState) error {
	if r.states == nil {
		r.states = make(map[string]*domain.AppState)
	}
	r.states[s.Key] = s
	return nil
}

type fakeCanceller struct{}

func (c *fakeCanceller) CancelTask(_ context.Context, _ string) error { return nil }

func setupRouter(taskRepo *fakeTaskRepo, appStateRepo *fakeAppStateRepo) *gin.Engine {
	canceller := &fakeCanceller{}
	taskUC := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, appStateRepo, canceller, nil)
	modeUC := usecase.NewModeUseCase(appStateRepo)
	budgetUC := usecase.NewBudgetUseCase(appStateRepo, scheduler.BudgetLimits{})

	healthH := api.NewHealthHandler(modeUC)
	taskH := api.NewTaskHandler(taskUC)
	modeH := api.NewModeHandler(modeUC)
	limitsH := api.NewLimitsHandler(budgetUC)
	slackH := api.NewSlackHandler("test-secret", canceller)

	return api.NewRouter(healthH, taskH, modeH, limitsH, slackH)
}

func TestHealthEndpoint(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp api.HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

func TestListTasksEndpoint_Empty(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp api.TaskListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected empty list, got %d items", len(resp.Items))
	}
}

func TestListTasksEndpoint_WithStatus(t *testing.T) {
	taskRepo := &fakeTaskRepo{
		tasks: []*domain.Task{
			{ID: "t1", Status: domain.TaskStatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: "t2", Status: domain.TaskStatusDone, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	r := setupRouter(taskRepo, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodGet, "/tasks?status=queued", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp api.TaskListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 queued task, got %d", len(resp.Items))
	}
}

func TestGetTaskEndpoint_NotFound(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodGet, "/tasks/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetTaskEndpoint_Found(t *testing.T) {
	taskRepo := &fakeTaskRepo{
		tasks: []*domain.Task{
			{ID: "task-123", Status: domain.TaskStatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	r := setupRouter(taskRepo, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodGet, "/tasks/task-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestEnqueueTaskEndpoint_OutsideWindow(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	body := `{"repo":"owner/repo","issue_number":42}`
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// No repos configured → 404 (not found in allowlist)
	if w.Code != http.StatusNotFound && w.Code != http.StatusConflict {
		t.Fatalf("expected 404 or 409, got %d", w.Code)
	}
}

func TestEnqueueTaskEndpoint_BadBody(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestStopTaskEndpoint_NotFound(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodPost, "/tasks/nonexistent/stop", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestStopTaskEndpoint_Done(t *testing.T) {
	taskRepo := &fakeTaskRepo{
		tasks: []*domain.Task{
			{ID: "done-task", Status: domain.TaskStatusDone},
		},
	}
	r := setupRouter(taskRepo, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodPost, "/tasks/done-task/stop", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 (not cancellable), got %d", w.Code)
	}
}

func TestGetFullModeEndpoint(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodGet, "/modes/full", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp api.FullModeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Enabled {
		t.Error("expected full mode to default to false")
	}
}

func TestSetFullModeEndpoint(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	body := `{"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/modes/full", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	var resp api.FullModeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestSlackInteractionsEndpoint_InvalidSignature(t *testing.T) {
	r := setupRouter(&fakeTaskRepo{}, &fakeAppStateRepo{})
	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", bytes.NewBufferString("payload={}"))
	req.Header.Set("X-Slack-Request-Timestamp", "1234567890")
	req.Header.Set("X-Slack-Signature", "v0=invalid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid signature, got %d", w.Code)
	}
}

// TestServer_Shutdown_C4 verifies C4 fix: Shutdown actually delegates to http.Server.Shutdown
// and does not return a no-op nil when the server has started.
func TestServer_Shutdown_C4(t *testing.T) { // ADDED
	// Start a real server on a random available port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // release so NewServer can bind

	srv := api.NewServer(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	listenErrCh := make(chan error, 1)
	go func() {
		listenErrCh <- srv.ListenAndServe()
	}()

	// Give the server a moment to start.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown must not return an error (server closes cleanly).
	if shutdownErr := srv.Shutdown(ctx); shutdownErr != nil {
		t.Errorf("C4: Shutdown returned unexpected error: %v", shutdownErr)
	}

	// ListenAndServe should have returned http.ErrServerClosed.
	select {
	case listenErr := <-listenErrCh:
		if listenErr != nil && listenErr != http.ErrServerClosed {
			t.Errorf("C4: expected ErrServerClosed, got: %v", listenErr)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("C4: server did not stop within timeout after Shutdown")
	}
}
