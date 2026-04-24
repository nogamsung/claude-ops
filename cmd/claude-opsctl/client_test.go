package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
	}
}

func TestClient_Get_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{Status: "ok", FullMode: false}) //nolint:errcheck // test helper
	})

	c := testClient(t, mux)
	var resp healthResponse
	err := c.get(context.Background(), "/healthz", &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.False(t, resp.FullMode)
}

func TestClient_Get_ErrorResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks/notfound", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"}) //nolint:errcheck // test helper
	})

	c := testClient(t, mux)
	var resp taskDetailResponse
	err := c.get(context.Background(), "/tasks/notfound", &resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "not found")
}

func TestClient_Post_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(taskResponse{ID: "abc-123", Status: "queued"}) //nolint:errcheck // test helper
	})

	c := testClient(t, mux)
	var resp taskResponse
	err := c.post(context.Background(), "/tasks", enqueueRequest{RepoFullName: "owner/repo", IssueNumber: 1}, &resp)
	require.NoError(t, err)
	assert.Equal(t, "abc-123", resp.ID)
	assert.Equal(t, "queued", resp.Status)
}

func TestClient_Patch_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/limits", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(limitsResponse{ //nolint:errcheck // test helper
			Daily:  limitsDaily{Count: 1, Max: 10, Date: "2026-04-24"},
			Weekly: limitsWeekly{Count: 5, Max: 50, Week: "2026-W17"},
		})
	})

	c := testClient(t, mux)
	daily := 10
	var resp limitsResponse
	err := c.patch(context.Background(), "/modes/limits", limitsPatchRequest{DailyMax: &daily}, &resp)
	require.NoError(t, err)
	assert.Equal(t, 10, resp.Daily.Max)
}
