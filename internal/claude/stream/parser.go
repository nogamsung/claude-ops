package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// ErrRateLimited is returned when the Claude CLI signals a rate limit.
type ErrRateLimited struct {
	ResetsAt      int64
	RateLimitType string
}

func (e *ErrRateLimited) Error() string {
	return fmt.Sprintf("rate limited (type=%s, resetsAt=%d)", e.RateLimitType, e.ResetsAt)
}

// ErrSessionMissing is returned when the Claude session is not logged in.
var ErrSessionMissing = errors.New("claude session missing: apiKeySource is not 'none'")

// ErrResultError is returned when the result event indicates an error.
type ErrResultError struct {
	Subtype string
	Message string
}

func (e *ErrResultError) Error() string {
	return fmt.Sprintf("claude result error (subtype=%s): %s", e.Subtype, e.Message)
}

// Signal is a parsed event emitted by the Parser.
type Signal struct {
	Kind      SignalKind
	TextChunk string       // set when Kind == SignalText
	Usage     *ResultUsage // set when Kind == SignalResult
	Result    *ResultEvent // set when Kind == SignalResult
}

// SignalKind distinguishes between signal types.
type SignalKind int

// SignalText, SignalUsageWarning, SignalResult, and SignalRateLimit classify
// NDJSON events emitted by the Claude Code CLI stream.
const (
	SignalText SignalKind = iota
	SignalUsageWarning
	SignalResult
	SignalRateLimit
)

// ParseStream reads NDJSON lines from r, emits signals to ch, and returns when:
//   - a "result" event is received (EOF of the session),
//   - r returns io.EOF, or
//   - an error / context cancellation occurs.
//
// Any unrecognised event type is logged at Debug level and skipped.
func ParseStream(ctx context.Context, r io.Reader, ch chan<- Signal) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var env struct {
			Type      string          `json:"type"`
			SessionID string          `json:"session_id"`
			UUID      string          `json:"uuid"`
			Raw       json.RawMessage // capture full line
		}
		raw := json.RawMessage(line)
		if err := json.Unmarshal(raw, &env); err != nil {
			slog.Debug("stream: failed to parse envelope", "err", err, "line", line)
			continue
		}
		env.Raw = raw

		switch env.Type {
		case "system.init":
			var init SystemInit
			if err := json.Unmarshal(raw, &struct {
				Data *SystemInit `json:"data"`
			}{Data: &init}); err == nil && init.APIKeySource == "" {
				// Try direct field (v2.1.113 format)
				_ = json.Unmarshal(raw, &init)
			} else {
				_ = json.Unmarshal(raw, &init)
			}
			if init.APIKeySource != "" && init.APIKeySource != "none" {
				return ErrSessionMissing
			}

		case "assistant":
			var ae AssistantEvent
			if err := json.Unmarshal(raw, &ae); err != nil {
				slog.Debug("stream: failed to parse assistant event", "err", err)
				continue
			}
			for _, c := range ae.Message.Content {
				if c.Type == "text" && c.Text != "" {
					select {
					case ch <- Signal{Kind: SignalText, TextChunk: c.Text}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}

		case "rate_limit_event":
			var rle RateLimitEvent
			if err := json.Unmarshal(raw, &rle); err != nil {
				slog.Debug("stream: failed to parse rate_limit_event", "err", err)
				continue
			}
			if rle.RateLimitInfo.Status != "allowed" {
				return &ErrRateLimited{
					ResetsAt:      rle.RateLimitInfo.ResetsAt,
					RateLimitType: rle.RateLimitInfo.RateLimitType,
				}
			}
			// Allowed — emit usage warning for monitoring
			select {
			case ch <- Signal{Kind: SignalUsageWarning}:
			case <-ctx.Done():
				return ctx.Err()
			}

		case "result":
			var re ResultEvent
			if err := json.Unmarshal(raw, &re); err != nil {
				slog.Debug("stream: failed to parse result event", "err", err)
				continue
			}

			select {
			case ch <- Signal{Kind: SignalResult, Usage: &re.Usage, Result: &re}:
			case <-ctx.Done():
				return ctx.Err()
			}

			if re.IsError {
				msg := re.Result
				if strings.Contains(strings.ToLower(msg), "rate limit") {
					return &ErrRateLimited{RateLimitType: "unknown"}
				}
				if strings.Contains(strings.ToLower(msg), "please login") ||
					strings.Contains(strings.ToLower(msg), "not logged in") {
					return ErrSessionMissing
				}
				return &ErrResultError{Subtype: re.Subtype, Message: msg}
			}
			// Successful result — we're done.
			return nil

		case "system.hook_started", "hook_response", "system.meta":
			// Informational lifecycle events — ignore.

		default:
			slog.Debug("stream: unknown event type", "type", env.Type, "raw", string(raw))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream scanner: %w", err)
	}
	return nil
}
