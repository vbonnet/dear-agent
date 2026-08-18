// report.go holds the four-bucket report shape shared by the JSON and
// human renderings. Its JSON keys are a contract with
// .github/workflows/stale-branch-audit.yml.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Report is the classification of every non-protected remote branch, plus
// the lookup_failed operational-failure bucket. Field names map to the JSON
// keys stale-branch-audit.yml jq-parses.
type Report struct {
	SafeDelete                 []string `json:"safe_delete"`
	ReviewNoPR                 []string `json:"review_no_pr"`
	ReviewClosedUnmerged       []string `json:"review_closed_unmerged"`
	ReviewNewCommitsAfterMerge []string `json:"review_new_commits_after_merge"`
	// LookupFailed lists branches whose PR-history lookup itself failed
	// (auth, rate limit, transient API error, or truncation past the
	// per-branch fetch limit) -- never classified, never deleted, distinct
	// from review_no_pr.
	LookupFailed []string `json:"lookup_failed"`
	// Deleted and DeleteFailed split safe_delete by what --execute actually
	// achieved. Without --execute both are empty: safe_delete alone means
	// "would delete", never "did delete".
	Deleted      []string `json:"deleted"`
	DeleteFailed []string `json:"delete_failed"`
}

func reviewTotal(r Report) int {
	return len(r.ReviewNoPR) + len(r.ReviewClosedUnmerged) + len(r.ReviewNewCommitsAfterMerge)
}

func printJSON(w io.Writer, r Report) {
	if r.LookupFailed == nil {
		r.LookupFailed = []string{}
	}
	if r.Deleted == nil {
		r.Deleted = []string{}
	}
	if r.DeleteFailed == nil {
		r.DeleteFailed = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(r)
}

func printHuman(w io.Writer, r Report, now time.Time) {
	fmt.Fprintf(w, "## Branch reaper — %s\n\n", now.Format("2006-01-02"))
	fmt.Fprintf(w, "Safe to delete (merged, tip still the merged head): %d\n", len(r.SafeDelete))
	for _, b := range r.SafeDelete {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Review: no PR ever opened: %d\n", len(r.ReviewNoPR))
	for _, b := range r.ReviewNoPR {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Review: PR closed without merging: %d\n", len(r.ReviewClosedUnmerged))
	for _, b := range r.ReviewClosedUnmerged {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Review: merged, but tip moved off the merged head: %d\n", len(r.ReviewNewCommitsAfterMerge))
	for _, b := range r.ReviewNewCommitsAfterMerge {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Lookup failed (gh error, not classified): %d\n", len(r.LookupFailed))
	for _, b := range r.LookupFailed {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	if len(r.Deleted) == 0 && len(r.DeleteFailed) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Deleted this run: %d\n", len(r.Deleted))
	for _, b := range r.Deleted {
		fmt.Fprintf(w, "  - %s\n", b)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Deletion FAILED (still on the remote): %d\n", len(r.DeleteFailed))
	for _, b := range r.DeleteFailed {
		fmt.Fprintf(w, "  - %s\n", b)
	}
}
