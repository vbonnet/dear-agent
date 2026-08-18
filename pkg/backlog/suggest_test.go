package backlog

import (
	"context"
	"errors"
	"testing"
)

type errSource struct{}

func (errSource) Name() string                          { return "err" }
func (errSource) Items(context.Context) ([]Item, error) { return nil, errors.New("boom") }

func ids(s []Suggestion) []string {
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = x.Item.ID
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newFixtureSuggester(t *testing.T) *Suggester {
	t.Helper()
	return NewSuggester(NewMarkdownSource("testdata/sample.md"))
}

func TestSuggestDefault(t *testing.T) {
	res, err := newFixtureSuggester(t).Suggest(context.Background(), Context{Phase: -1})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if res.Total != 12 {
		t.Errorf("Total = %d, want 12", res.Total)
	}
	if !eq(ids(res.Suggested), []string{"1.1", "1.4"}) {
		t.Errorf("Suggested = %v, want [1.1 1.4]", ids(res.Suggested))
	}
	if !eq(ids(res.Blocked), []string{"1.2", "1.5"}) {
		t.Errorf("Blocked = %v, want [1.2 1.5]", ids(res.Blocked))
	}
}

func TestSuggestPhaseFilter(t *testing.T) {
	res, err := newFixtureSuggester(t).Suggest(context.Background(), Context{Phase: 1})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if res.Total != 5 {
		t.Errorf("Total = %d, want 5 (phase 1 only)", res.Total)
	}
	if !eq(ids(res.Suggested), []string{"1.1", "1.4"}) {
		t.Errorf("Suggested = %v, want [1.1 1.4]", ids(res.Suggested))
	}
}

func TestSuggestCapacity(t *testing.T) {
	res, err := newFixtureSuggester(t).Suggest(context.Background(), Context{Phase: -1, Capacity: 1})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if !eq(ids(res.Suggested), []string{"1.1"}) {
		t.Errorf("Suggested = %v, want [1.1] under capacity 1", ids(res.Suggested))
	}
}

func TestSuggestMaxEffortCap(t *testing.T) {
	res, err := newFixtureSuggester(t).Suggest(context.Background(),
		Context{Phase: -1, MaxEffort: EffortSmall})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	// 1.1 is M and exceeds the S cap; 1.4 is S and stays.
	if !eq(ids(res.Suggested), []string{"1.4"}) {
		t.Errorf("Suggested = %v, want [1.4] under MaxEffort=S", ids(res.Suggested))
	}
}

func TestSuggestSourceError(t *testing.T) {
	_, err := (&Suggester{Source: errSource{}}).Suggest(context.Background(), Context{})
	if err == nil {
		t.Fatal("expected error from failing source")
	}
}
