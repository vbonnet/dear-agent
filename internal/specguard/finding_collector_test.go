package specguard

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

// TestFindingCollectorTruncationIsDeterministic pins the property the Stop
// retry loop depends on. Findings arrive from map iterations whose order Go
// leaves unspecified, so truncating during collection retained whichever
// findings happened to arrive first: two evaluations of the same immutable
// snapshot could emit different subsets, produce different feedback digests,
// and read to Claude and Codex as a fresh attempt every time rather than a
// repeat of one they already saw.
func TestFindingCollectorTruncationIsDeterministic(t *testing.T) {
	const limit = 8
	all := make([]Finding, 0, limit*4)
	for i := range cap(all) {
		all = append(all, Finding{
			Code:    fmt.Sprintf("code-%02d", i%5),
			Path:    fmt.Sprintf("dir%02d/SPEC.md", i),
			Line:    i,
			Message: fmt.Sprintf("finding %02d", i),
		})
	}

	collect := func(order []Finding) []Finding {
		collector := findingCollector{limit: limit}
		for _, finding := range order {
			collector.add(finding)
		}
		return collector.sorted()
	}

	shuffled := slices.Clone(all)
	want := collect(shuffled)
	if len(want) != limit {
		t.Fatalf("truncated findings = %d, want %d", len(want), limit)
	}
	if want[limit-1].Code != "finding-limit" {
		t.Fatalf("last finding = %q, want the overflow marker", want[limit-1].Code)
	}

	// Any arrival order of the same finding set must produce the same report.
	generator := rand.New(rand.NewPCG(1, 2))
	for attempt := range 64 {
		generator.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := collect(shuffled)
		if !slices.Equal(got, want) {
			t.Fatalf("arrival order %d changed the truncated report:\n got %+v\nwant %+v", attempt, got, want)
		}
	}
}

// TestFindingCollectorCollapsesPastTheCeiling proves the one case where no
// deterministic subset exists still yields the same single result every time.
func TestFindingCollectorCollapsesPastTheCeiling(t *testing.T) {
	collector := findingCollector{limit: 4}
	for i := range findingCollectionCeiling + 16 {
		collector.add(Finding{Code: "code", Path: fmt.Sprintf("dir%06d/SPEC.md", i)})
	}
	got := collector.sorted()
	if len(got) != 1 || got[0].Code != "finding-limit" {
		t.Fatalf("past-ceiling report = %+v, want one finding-limit entry", got)
	}
}

// TestFindingCollectorKeepsEveryFindingBelowTheLimit guards the ordinary path:
// no overflow marker and no truncation when the set fits.
func TestFindingCollectorKeepsEveryFindingBelowTheLimit(t *testing.T) {
	collector := findingCollector{limit: 4}
	collector.add(Finding{Code: "b", Path: "z/SPEC.md"})
	collector.add(Finding{Code: "a", Path: "a/SPEC.md"})
	got := collector.sorted()
	if len(got) != 2 || got[0].Path != "a/SPEC.md" || got[1].Path != "z/SPEC.md" {
		t.Fatalf("report = %+v, want both findings sorted by path", got)
	}
}
