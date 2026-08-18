package backlog

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureItems(t *testing.T) []Item {
	t.Helper()
	src := NewMarkdownSource(filepath.Join("testdata", "sample.md"))
	items, err := src.Items(context.Background())
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return items
}

func suggestionByID(s []Suggestion, id string) (Suggestion, bool) {
	for _, x := range s {
		if x.Item.ID == id {
			return x, true
		}
	}
	return Suggestion{}, false
}

func TestRankEligibilityAndOrder(t *testing.T) {
	ranked := Ranker{}.Rank(fixtureItems(t))

	// Eligible items must sort ahead of ineligible ones.
	var eligibleIDs []string
	for _, s := range ranked {
		if s.Eligible {
			eligibleIDs = append(eligibleIDs, s.Item.ID)
		}
	}
	// 1.1 (unblocks 2) should outrank 1.4 (quick win, unblocks none).
	if len(eligibleIDs) != 2 || eligibleIDs[0] != "1.1" || eligibleIDs[1] != "1.4" {
		t.Fatalf("eligible order = %v, want [1.1 1.4]", eligibleIDs)
	}

	if s, _ := suggestionByID(ranked, "1.1"); !s.Eligible || s.Dependents != 2 {
		t.Errorf("1.1 = {eligible %v dependents %d}, want {true 2}", s.Eligible, s.Dependents)
	}
}

func TestRankBlockers(t *testing.T) {
	ranked := Ranker{}.Rank(fixtureItems(t))

	s12, _ := suggestionByID(ranked, "1.2")
	if s12.Eligible || len(s12.Blockers) == 0 ||
		!strings.Contains(s12.Blockers[0], "1.1 (pending)") {
		t.Errorf("1.2 blockers = %v, want unmet 1.1 (pending)", s12.Blockers)
	}

	s15, _ := suggestionByID(ranked, "1.5")
	if s15.Eligible || len(s15.Blockers) == 0 ||
		!strings.Contains(s15.Blockers[0], "9.9 (unknown)") {
		t.Errorf("1.5 blockers = %v, want unmet 9.9 (unknown)", s15.Blockers)
	}
}

func TestRankWildcardDependency(t *testing.T) {
	// Satisfied wildcard: phase-0 fully done -> 1.1 eligible.
	if s, _ := suggestionByID(Ranker{}.Rank(fixtureItems(t)), "1.1"); !s.Eligible {
		t.Errorf("1.1 should be eligible (0.* satisfied), blockers=%v", s.Blockers)
	}

	// Unsatisfied wildcard: flip a phase-0 item to pending.
	items := fixtureItems(t)
	for i := range items {
		if items[i].ID == "0.2" {
			items[i].Status = StatusPending
		}
	}
	s, _ := suggestionByID(Ranker{}.Rank(items), "1.1")
	if s.Eligible {
		t.Error("1.1 should be blocked when a phase-0 dep is not done")
	}
	if len(s.Blockers) == 0 || !strings.Contains(s.Blockers[0], "0.*") {
		t.Errorf("1.1 blockers = %v, want a 0.* wildcard blocker", s.Blockers)
	}
}

func TestRankPriorityBeatsPhase(t *testing.T) {
	items := []Item{
		{ID: "1.1", Phase: 1, Status: StatusPending, Effort: EffortMedium},
		{ID: "9.1", Phase: 9, Status: StatusPending, Effort: EffortMedium, Priority: PriorityHigh},
	}
	ranked := Ranker{}.Rank(items)
	if ranked[0].Item.ID != "9.1" {
		t.Errorf("explicit HIGH on a late phase should win; order=%v",
			[]string{ranked[0].Item.ID, ranked[1].Item.ID})
	}
}

func TestRankDeterministic(t *testing.T) {
	items := fixtureItems(t)
	a := Ranker{}.Rank(items)
	b := Ranker{}.Rank(items)
	if len(a) != len(b) {
		t.Fatal("length mismatch")
	}
	for i := range a {
		if a[i].Item.ID != b[i].Item.ID {
			t.Fatalf("non-deterministic at %d: %s vs %s", i, a[i].Item.ID, b[i].Item.ID)
		}
	}
}

func TestRankWeightsOrDefault(t *testing.T) {
	if (RankWeights{}).orDefault() != DefaultRankWeights {
		t.Error("zero RankWeights should resolve to DefaultRankWeights")
	}
	custom := RankWeights{Priority: 1, Leverage: 0, Effort: 0}
	if custom.orDefault() != custom {
		t.Error("non-zero RankWeights should be preserved")
	}
}

func TestCompareIDAndPhaseLess(t *testing.T) {
	if compareID("0.2", "0.10") >= 0 {
		t.Error("0.2 should sort before 0.10 numerically")
	}
	if compareID("1.1", "1.1") != 0 {
		t.Error("equal IDs should compare 0")
	}
	if compareID("2.1", "1.9") <= 0 {
		t.Error("2.1 should sort after 1.9")
	}
	if !phaseLess(0, 1) || phaseLess(1, 0) {
		t.Error("phaseLess ascending broken")
	}
	if phaseLess(-1, 0) || !phaseLess(0, -1) {
		t.Error("cross-phase (-1) should sort last")
	}
}
