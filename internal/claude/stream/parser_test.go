package stream_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gs97ahn/scheduled-dev-agent/internal/claude/stream"
)

func parseFixture(t *testing.T, name string) ([]stream.Signal, error) {
	t.Helper()
	path := filepath.Join("testdata", "streams", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer f.Close()

	ch := make(chan stream.Signal, 32)
	ctx := context.Background()

	var parseErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		parseErr = stream.ParseStream(ctx, f, ch)
	}()
	<-done
	close(ch)

	var signals []stream.Signal
	for s := range ch {
		signals = append(signals, s)
	}
	return signals, parseErr
}

func TestParseStream_Success(t *testing.T) {
	signals, err := parseFixture(t, "success.ndjson")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 text signals + 1 result signal
	var textCount, resultCount int
	for _, s := range signals {
		switch s.Kind {
		case stream.SignalText:
			textCount++
		case stream.SignalResult:
			resultCount++
			if s.Result == nil {
				t.Error("result signal has nil Result")
			}
			if s.Result.IsError {
				t.Error("expected success, got is_error=true")
			}
		}
	}
	if textCount < 1 {
		t.Errorf("expected at least 1 text signal, got %d", textCount)
	}
	if resultCount != 1 {
		t.Errorf("expected 1 result signal, got %d", resultCount)
	}
}

func TestParseStream_RateLimit(t *testing.T) {
	_, err := parseFixture(t, "rate_limit.ndjson")
	if err == nil {
		t.Fatal("expected ErrRateLimited, got nil")
	}
	var rl *stream.ErrRateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected *ErrRateLimited, got %T: %v", err, err)
	}
	if rl.ResetsAt == 0 {
		t.Error("expected non-zero ResetsAt")
	}
	if rl.RateLimitType != "five_hour" {
		t.Errorf("expected five_hour, got %s", rl.RateLimitType)
	}
}

func TestParseStream_SessionMissing(t *testing.T) {
	_, err := parseFixture(t, "session_missing.ndjson")
	if !errors.Is(err, stream.ErrSessionMissing) {
		t.Fatalf("expected ErrSessionMissing, got %v", err)
	}
}

func TestParseStream_Cancelled(t *testing.T) {
	// Cancelled fixture has no result event — should return nil (EOF is ok)
	signals, err := parseFixture(t, "cancelled.ndjson")
	if err != nil {
		t.Fatalf("cancelled fixture: unexpected error: %v", err)
	}
	// Should have received at least 1 text signal
	var textCount int
	for _, s := range signals {
		if s.Kind == stream.SignalText {
			textCount++
		}
	}
	if textCount < 1 {
		t.Errorf("expected at least 1 text signal, got %d", textCount)
	}
}

func TestParseStream_UnknownEventType(t *testing.T) {
	ndjson := `{"type":"unknown_future_event","session_id":"s","uuid":"u","data":"something"}
{"type":"result","session_id":"s","uuid":"u","subtype":"success","is_error":false,"result":"ok","usage":{}}
`
	ch := make(chan stream.Signal, 32)
	err := stream.ParseStream(context.Background(), strings.NewReader(ndjson), ch)
	if err != nil {
		t.Fatalf("unknown event should be skipped, got error: %v", err)
	}
	close(ch)
}

func TestParseStream_ErrorResult(t *testing.T) {
	ndjson := `{"type":"system.init","session_id":"s","uuid":"u","apiKeySource":"none"}
{"type":"result","session_id":"s","uuid":"u","subtype":"error","is_error":true,"result":"execution failed","usage":{}}
`
	ch := make(chan stream.Signal, 32)
	err := stream.ParseStream(context.Background(), strings.NewReader(ndjson), ch)
	var re *stream.ErrResultError
	if !errors.As(err, &re) {
		t.Fatalf("expected *ErrResultError, got %T: %v", err, err)
	}
	if re.Subtype != "error" {
		t.Errorf("expected subtype 'error', got %q", re.Subtype)
	}
}

func TestParseStream_RateLimitInResultText(t *testing.T) {
	ndjson := `{"type":"system.init","session_id":"s","uuid":"u","apiKeySource":"none"}
{"type":"result","session_id":"s","uuid":"u","subtype":"error","is_error":true,"result":"You have exceeded the rate limit. Please try again.","usage":{}}
`
	ch := make(chan stream.Signal, 32)
	err := stream.ParseStream(context.Background(), strings.NewReader(ndjson), ch)
	var rl *stream.ErrRateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected *ErrRateLimited from result text, got %T: %v", err, err)
	}
}

func TestParseStream_LoginRequiredInResultText(t *testing.T) {
	ndjson := `{"type":"system.init","session_id":"s","uuid":"u","apiKeySource":"none"}
{"type":"result","session_id":"s","uuid":"u","subtype":"error","is_error":true,"result":"Please login to continue using Claude.","usage":{}}
`
	ch := make(chan stream.Signal, 32)
	err := stream.ParseStream(context.Background(), strings.NewReader(ndjson), ch)
	if !errors.Is(err, stream.ErrSessionMissing) {
		t.Fatalf("expected ErrSessionMissing from result text, got %T: %v", err, err)
	}
}

func TestParseStream_EmptyInput(t *testing.T) {
	ch := make(chan stream.Signal, 1)
	err := stream.ParseStream(context.Background(), strings.NewReader(""), ch)
	if err != nil {
		t.Fatalf("empty input should succeed, got: %v", err)
	}
}

func TestParseStream_ContextCancellation(t *testing.T) {
	// Long stream that will be cancelled
	ndjson := `{"type":"system.init","session_id":"s","uuid":"u","apiKeySource":"none"}
`
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ch := make(chan stream.Signal, 1)
	_ = stream.ParseStream(ctx, strings.NewReader(ndjson), ch)
	// No assertion needed — just verify it doesn't block
}

func TestParseStream_MultipleTextChunks(t *testing.T) {
	ndjson := `{"type":"system.init","apiKeySource":"none"}
{"type":"assistant","message":{"content":[{"type":"text","text":"chunk 1"},{"type":"text","text":"chunk 2"}],"usage":{}}}
{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{}}
`
	ch := make(chan stream.Signal, 32)
	err := stream.ParseStream(context.Background(), strings.NewReader(ndjson), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	var textCount int
	for s := range ch {
		if s.Kind == stream.SignalText {
			textCount++
		}
	}
	if textCount < 2 {
		t.Errorf("expected 2+ text chunks, got %d", textCount)
	}
}
