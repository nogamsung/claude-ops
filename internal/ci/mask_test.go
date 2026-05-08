package ci_test

import (
	"strings"
	"testing"

	"github.com/gs97ahn/claude-ops/internal/ci"
)

func TestMaskSecrets_Patterns(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // substring that must appear in output
		not  string // substring that must NOT appear
	}{
		{"github_secret_expr", "Run with ${{ secrets.MY_TOKEN }} please", "${{ secrets.*** }}", "MY_TOKEN"},
		{"key_value_token", "TOKEN=abcdef1234567890", "TOKEN=***", "abcdef"},
		{"key_value_api_key", "api_key: hunter2longvalue", "api_key=***", "hunter2"},
		{"bearer_header", "Authorization: Bearer ey.J0eXAi.xxxxxxxx", "Bearer ***", "ey.J0eXAi"},
		{"gh_pat_classic", "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "***", "ghp_AAAA"},
		{"gh_pat_oauth", "gho_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", "***", "gho_BBBB"},
		{"short_value_kept", "color=red", "color=red", ""}, // < 4 char value, intentionally kept
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ci.MaskSecrets(tc.in)
			if !strings.Contains(got, tc.want) {
				t.Errorf("expected %q in output, got %q", tc.want, got)
			}
			if tc.not != "" && strings.Contains(got, tc.not) {
				t.Errorf("expected %q to be masked, but it leaked: %q", tc.not, got)
			}
		})
	}
}

func TestMaskSecrets_Idempotent(t *testing.T) {
	in := "TOKEN=abcdef1234567890\n${{ secrets.X }}"
	once := ci.MaskSecrets(in)
	twice := ci.MaskSecrets(once)
	if once != twice {
		t.Errorf("not idempotent:\nonce=%q\ntwice=%q", once, twice)
	}
}

func TestTruncateLogTail(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	got := ci.TruncateLogTail(strings.Join(lines, "\n"), 3)
	if !strings.HasPrefix(got, "[truncated 2 earlier lines]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
	if !strings.HasSuffix(got, "c\nd\ne") {
		t.Errorf("expected last 3 lines preserved, got %q", got)
	}
}

func TestTruncateLogTail_NoTrunc(t *testing.T) {
	in := "x\ny\nz"
	if got := ci.TruncateLogTail(in, 5); got != in {
		t.Errorf("expected passthrough, got %q", got)
	}
}

func TestAggregateConclusion(t *testing.T) {
	cases := []struct {
		name        string
		checks      []ci.CheckRun
		wantStatus  string
		wantFailLen int
	}{
		{"all_success", []ci.CheckRun{
			{Status: "completed", Conclusion: ci.ConclusionSuccess},
			{Status: "completed", Conclusion: ci.ConclusionSkipped},
		}, "passed", 0},
		{"one_failure", []ci.CheckRun{
			{Status: "completed", Conclusion: ci.ConclusionSuccess},
			{Status: "completed", Conclusion: ci.ConclusionFailure, Name: "lint"},
		}, "failed", 1},
		{"in_progress", []ci.CheckRun{
			{Status: "completed", Conclusion: ci.ConclusionSuccess},
			{Status: "in_progress"},
		}, "", 0},
		{"action_required", []ci.CheckRun{
			{Status: "completed", Conclusion: ci.ConclusionSuccess},
			{Status: "completed", Conclusion: ci.ConclusionActionRequired},
		}, "stuck", 0},
		{"empty", nil, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, failed := ci.AggregateConclusion(tc.checks)
			if got != tc.wantStatus {
				t.Errorf("status: want %q got %q", tc.wantStatus, got)
			}
			if len(failed) != tc.wantFailLen {
				t.Errorf("failedRuns: want %d got %d", tc.wantFailLen, len(failed))
			}
		})
	}
}
