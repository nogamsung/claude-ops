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

func TestHealthCmd_Table(t *testing.T) {
	tick := time.Date(2026, 4, 24, 9, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{Status: "ok", FullMode: true, TickAt: &tick}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newHealthCmd(c)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestHealthCmd_JSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{Status: "ok", FullMode: false}) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newHealthCmd(c)
	cmd.SetArgs([]string{"--output", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestHealthCmd_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`)) //nolint:errcheck // test helper
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	cmd := newHealthCmd(c)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
}
