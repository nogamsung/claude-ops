package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleLimits() limitsResponse {
	return limitsResponse{
		Daily:  limitsDaily{Count: 3, Max: 5, Date: "2026-04-24"},
		Weekly: limitsWeekly{Count: 12, Max: 35, Week: "2026-W17"},
	}
}

func TestLimitsShowCmd_Table(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/limits", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleLimits()) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newLimitsCmd(c)
	cmd.SetArgs([]string{"show"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestLimitsShowCmd_JSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/limits", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleLimits()) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newLimitsCmd(c)
	cmd.SetArgs([]string{"show", "--output", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestLimitsSetCmd_Daily(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/limits", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		var req limitsPatchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		require.NotNil(t, req.DailyMax)
		assert.Equal(t, 10, *req.DailyMax)
		assert.Nil(t, req.WeeklyMax)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleLimits()) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newLimitsCmd(c)
	cmd.SetArgs([]string{"set", "--daily", "10"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestLimitsSetCmd_Weekly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/limits", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		var req limitsPatchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.Nil(t, req.DailyMax)
		require.NotNil(t, req.WeeklyMax)
		assert.Equal(t, 50, *req.WeeklyMax)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleLimits()) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newLimitsCmd(c)
	cmd.SetArgs([]string{"set", "--weekly", "50"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestLimitsSetCmd_NoFlags_Error(t *testing.T) {
	c := &Client{baseURL: "http://localhost", httpClient: http.DefaultClient}
	cmd := newLimitsCmd(c)
	cmd.SetArgs([]string{"set"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestLimitsSetCmd_BothFlags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/limits", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		var req limitsPatchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		require.NotNil(t, req.DailyMax)
		require.NotNil(t, req.WeeklyMax)
		assert.Equal(t, 8, *req.DailyMax)
		assert.Equal(t, 40, *req.WeeklyMax)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleLimits()) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newLimitsCmd(c)
	cmd.SetArgs([]string{"set", "--daily", "8", "--weekly", "40"})
	err := cmd.Execute()
	require.NoError(t, err)
}
