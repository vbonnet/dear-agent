// Package orphanpr detects open pull requests whose tracking bead is already
// closed.
//
// Motivation (DEAR retro 2026-06-21): PRs #579/#581/#582 sat open in a
// DIRTY/conflict state for 20+ hours while their tracking beads were CLOSED.
// Supervisors never noticed because the only signal we tracked was the forward
// direction ("PR merged ⇒ work done"); the inverse — a bead closing without
// its PR merging — left the PR orphaned and invisible. This package supplies
// the missing detection: extract the bead references from each open PR and
// flag any PR that references a CLOSED bead.
//
// Detection only. Closing an orphaned PR requires human authorization and is
// deliberately out of scope here.
package orphanpr

import (
	"regexp"
	"sort"
)

// BeadStatus is the lifecycle state of a tracking bead, as resolved from the
// bead database. Unknown means the status could not be positively established
// (bead missing, bd unavailable, parse error). Callers MUST NOT treat Unknown
// as Closed — this mirrors the conservative "do not act on an ambiguous
// signal" stance in internal/git/pr.go, so a transient bd hiccup can never
// make a live PR look orphaned.
type BeadStatus string

const (
	// BeadOpen is any non-closed lifecycle state (open, in_progress, blocked).
	BeadOpen BeadStatus = "open"
	// BeadClosed is the closed lifecycle state — the orphan trigger.
	BeadClosed BeadStatus = "closed"
	// BeadUnknown means the status could not be positively established.
	BeadUnknown BeadStatus = "unknown"
)

// PR is the minimal pull-request shape the scan needs.
type PR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// BeadRef pairs an extracted bead identifier with its resolved status.
type BeadRef struct {
	ID     string     `json:"id"`
	Status BeadStatus `json:"status"`
}

// OrphanedPR is an open PR that references at least one CLOSED bead.
type OrphanedPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	// ClosedBeads lists the referenced beads that are closed (sorted).
	ClosedBeads []string `json:"closed_beads"`
	// BeadRefs records every referenced bead with its resolved status, so a
	// reviewer can see why the PR was flagged and which refs are still open.
	BeadRefs []BeadRef `json:"bead_refs"`
}

// beadRefRe matches bead identifiers like ce-vkvk or ce-s2tn.6. The optional
// trailing ".N" captures sub-task references (e.g. ce-s2tn.6).
var beadRefRe = regexp.MustCompile(`\bce-[a-z0-9]+(?:\.[0-9]+)?\b`)

// ExtractBeadRefs returns the unique bead identifiers referenced in text,
// preserving first-seen order so output is stable and human-readable.
func ExtractBeadRefs(text string) []string {
	matches := beadRefRe.FindAllString(text, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

// StatusFunc resolves the lifecycle status of a single bead. It is injected so
// the scan logic is testable without the bd CLI.
type StatusFunc func(beadID string) BeadStatus

// Scan flags every PR that references at least one CLOSED bead. A PR with no
// bead references, or whose referenced beads are all open/unknown, is not
// flagged. statusOf is called at most once per unique bead id per PR.
//
// Bead references are taken from both the title and the body, since PRs in
// this repo reference beads in either place ("(ce-vkvk)" in the title,
// "Closes ce-vkvk" in the body).
func Scan(prs []PR, statusOf StatusFunc) []OrphanedPR {
	var orphans []OrphanedPR
	for _, pr := range prs {
		ids := ExtractBeadRefs(pr.Title + "\n" + pr.Body)
		if len(ids) == 0 {
			continue
		}
		refs := make([]BeadRef, 0, len(ids))
		var closed []string
		for _, id := range ids {
			st := statusOf(id)
			refs = append(refs, BeadRef{ID: id, Status: st})
			if st == BeadClosed {
				closed = append(closed, id)
			}
		}
		if len(closed) == 0 {
			continue
		}
		sort.Strings(closed)
		orphans = append(orphans, OrphanedPR{
			Number:      pr.Number,
			Title:       pr.Title,
			ClosedBeads: closed,
			BeadRefs:    refs,
		})
	}
	return orphans
}
