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

func TestModeFullCmd_Show(t *testing.T) {
	since := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/full", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fullModeResponse{Enabled: true, Since: &since}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newModeCmd(c)
	cmd.SetArgs([]string{"full", "show"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestModeFullCmd_On(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/full", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		var req fullModeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.True(t, req.Enabled)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fullModeResponse{Enabled: true}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newModeCmd(c)
	cmd.SetArgs([]string{"full", "--on"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestModeFullCmd_Off(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/full", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		var req fullModeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.False(t, req.Enabled)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fullModeResponse{Enabled: false}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newModeCmd(c)
	cmd.SetArgs([]string{"full", "--off"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestModeFullCmd_OnAndOff_Error(t *testing.T) {
	c := &Client{baseURL: "http://localhost", httpClient: http.DefaultClient}
	cmd := newModeCmd(c)
	cmd.SetArgs([]string{"full", "--on", "--off"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestModeFullCmd_JSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/modes/full", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fullModeResponse{Enabled: false}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newModeCmd(c)
	cmd.SetArgs([]string{"full", "--output", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
}
