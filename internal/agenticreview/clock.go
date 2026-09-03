package agenticreview

import "time"

// TimelineEvent is the subset of a GitHub issue timeline entry the gate reads.
// Labels carry no time of their own, so the timeline is the only place a
// started label's age can come from.
type TimelineEvent struct {
	Event     string    `json:"event"`
	CreatedAt time.Time `json:"created_at"`
	Label     struct {
		Name string `json:"name"`
	} `json:"label"`
}

// Head describes the pull request state the review clock is derived from.
type Head struct {
	// Labels currently on the pull request.
	Labels []string
	// CreatedAt is when the pull request was opened, used as the readiness
	// fallback for a pull request that was never a draft.
	CreatedAt time.Time
	// CommittedAt is the head commit's date.
	CommittedAt time.Time
	// IsDraft suppresses the readiness clock entirely: a draft has not gone
	// ready, so no reviewer is late.
	IsDraft bool
}

// Clock derives the label ages and the readiness time the gate needs, so the
// required check and the merge loop age reviewers by exactly the same rule.
//
// Two properties matter more than they look:
//
// The timeline is replayed in order, and an unlabeled event drops the
// timestamp. A label removed by a push and reapplied by the new review must
// carry the new time; keeping the old one would let a reviewer that was reset
// look as though it had been running since before the push, and age it out
// into a false "down" that the quorum then merges around.
//
// Readiness is the later of the ready event and the head commit. A push onto
// an already-ready pull request clears the review labels, so the dispatch
// window has to restart with the new head instead of arriving already spent.
func Clock(events []TimelineEvent, head Head) (map[string]time.Time, time.Time) {
	live := make(map[string]bool, len(head.Labels))
	for _, name := range head.Labels {
		live[name] = true
	}

	applied := make(map[string]time.Time)
	var readyAt time.Time
	for _, e := range events {
		switch e.Event {
		case "labeled":
			applied[e.Label.Name] = e.CreatedAt
		case "unlabeled":
			delete(applied, e.Label.Name)
		case "ready_for_review":
			readyAt = e.CreatedAt
		}
	}

	out := make(map[string]time.Time, len(applied))
	for name, at := range applied {
		if live[name] {
			out[name] = at
		}
	}

	if head.IsDraft {
		return out, time.Time{}
	}
	if readyAt.IsZero() {
		readyAt = head.CreatedAt
	}
	if head.CommittedAt.After(readyAt) {
		readyAt = head.CommittedAt
	}
	return out, readyAt
}
