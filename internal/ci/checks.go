// Package ci implements the CI watcher loop that wraps the post-PR
// `gh pr checks` polling, log extraction, and secret masking.
//
// The watcher itself (the goroutine that ticks against the database)
// lives in internal/ci/watcher.go in the next commit. This file owns
// the pure data structures and the small parsing helpers that the
// watcher orchestrates.
package ci

// CheckConclusion is the post-completion outcome of a single check run.
// Values mirror the GitHub Actions API; "" means the check has not yet
// completed.
type CheckConclusion string

// CheckConclusion enum.
const (
	ConclusionEmpty          CheckConclusion = ""
	ConclusionSuccess        CheckConclusion = "success"
	ConclusionFailure        CheckConclusion = "failure"
	ConclusionTimedOut       CheckConclusion = "timed_out"
	ConclusionCancelled      CheckConclusion = "cancelled"
	ConclusionActionRequired CheckConclusion = "action_required"
	ConclusionSkipped        CheckConclusion = "skipped"
	ConclusionNeutral        CheckConclusion = "neutral"
	ConclusionStale          CheckConclusion = "stale"
)

// CheckRun is the projection of a `gh pr checks --json` row that the
// watcher cares about.
type CheckRun struct {
	Name        string          `json:"name"`
	Status      string          `json:"status"` // queued|in_progress|completed
	Conclusion  CheckConclusion `json:"conclusion"`
	DetailsURL  string          `json:"detailsUrl,omitempty"`
	WorkflowRun int64           `json:"workflowRun,omitempty"` // run_id when extractable
}

// AggregateConclusion classifies a slice of CheckRun results into the
// CIStatus the watcher persists. Returns CIStatusEmpty when at least one
// check is still in_progress / queued — caller should keep polling.
//
// Decision tree (matches PRD §6.1 step 5):
//   - any non-completed → still watching
//   - all success / skipped / neutral → passed
//   - any failure / timed_out / cancelled → failed
//   - else (only action_required / stale, no failures) → stuck
func AggregateConclusion(checks []CheckRun) (status string, failedRuns []CheckRun) {
	if len(checks) == 0 {
		return "", nil
	}
	allTerminal := true
	hasFailure := false
	hasActionRequired := false
	for _, c := range checks {
		if c.Status != "completed" {
			allTerminal = false
		}
		switch c.Conclusion {
		case ConclusionFailure, ConclusionTimedOut, ConclusionCancelled:
			hasFailure = true
			failedRuns = append(failedRuns, c)
		case ConclusionActionRequired:
			hasActionRequired = true
		}
	}
	if !allTerminal {
		return "", nil
	}
	if hasFailure {
		return "failed", failedRuns
	}
	if hasActionRequired {
		return "stuck", nil
	}
	return "passed", nil
}
