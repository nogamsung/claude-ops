package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedTime() time.Time {
	return time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
}

func sampleTask() taskResponse {
	t := fixedTime()
	return taskResponse{
		ID:           "task-001",
		RepoFullName: "owner/repo",
		IssueNumber:  42,
		IssueTitle:   "Fix bug",
		TaskType:     "feature",
		Status:       "queued",
		CreatedAt:    t,
		UpdatedAt:    t,
	}
}

func TestTasksLsCmd_Table(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(taskListResponse{Items: []taskResponse{sampleTask()}}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"ls"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestTasksLsCmd_WithStatus(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(taskListResponse{}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"ls", "--status", "running"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "status=running", gotPath)
}

func TestTasksLsCmd_JSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(taskListResponse{Items: []taskResponse{sampleTask()}}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"ls", "--output", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestTasksShowCmd_Table(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks/task-001", func(w http.ResponseWriter, _ *http.Request) {
		task := sampleTask()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(taskDetailResponse{ //nolint:errcheck // test helper
			taskResponse: task,
			Events: []taskEventResponse{
				{ID: "evt-1", Kind: "started", CreatedAt: fixedTime()},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"show", "task-001"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestTasksShowCmd_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks/bad-id", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"show", "bad-id"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestTasksRunCmd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(sampleTask()) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"run", "owner/repo", "--issue", "42"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestTasksRunCmd_BadRepo(t *testing.T) {
	c := &Client{baseURL: "http://localhost", httpClient: http.DefaultClient}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"run", "badrepo", "--issue", "1"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "owner/repo format")
}

func TestTasksRunCmd_MissingIssue(t *testing.T) {
	c := &Client{baseURL: "http://localhost", httpClient: http.DefaultClient}
	cmd := newTasksCmd(c)
	cmd.SetArgs([]string{"run", "owner/repo"})
	err := cmd.Execute()
	assert.Error(t, err)
}
