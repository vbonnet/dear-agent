package fuzzy

import (
	"testing"
	"testing/quick"
)

// TestPropertyFindSimilarSelfIsMaxScore verifies that searching for a string
// among candidates that include itself always returns it with similarity 1.0.
func TestPropertyFindSimilarSelfIsMaxScore(t *testing.T) {
	f := func(s string) bool {
		if len(s) == 0 {
			return true // skip empty strings (max=0 causes continue)
		}
		candidates := []string{s}
		matches := FindSimilar(s, candidates, 0.0)
		if len(matches) == 0 {
			return false
		}
		return matches[0].Similarity == 1.0 && matches[0].Distance == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarDeterministic verifies FindSimilar returns the same
// results for the same input.
func TestPropertyFindSimilarDeterministic(t *testing.T) {
	f := func(input string) bool {
		candidates := []string{"alpha", "bravo", "charlie", "delta", "echo"}
		matches1 := FindSimilar(input, candidates, 0.3)
		matches2 := FindSimilar(input, candidates, 0.3)
		if len(matches1) != len(matches2) {
			return false
		}
		for i := range matches1 {
			if matches1[i].Name != matches2[i].Name {
				return false
			}
			if matches1[i].Similarity != matches2[i].Similarity {
				return false
			}
			if matches1[i].Distance != matches2[i].Distance {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarEmptyCandidates verifies FindSimilar returns empty
// for empty candidates.
func TestPropertyFindSimilarEmptyCandidates(t *testing.T) {
	f := func(input string) bool {
		matches := FindSimilar(input, []string{}, 0.0)
		return len(matches) == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarMaxFiveResults verifies FindSimilar never returns
// more than 5 results.
func TestPropertyFindSimilarMaxFiveResults(t *testing.T) {
	f := func(input string) bool {
		candidates := []string{
			"aaa", "bbb", "ccc", "ddd", "eee",
			"fff", "ggg", "hhh", "iii", "jjj",
		}
		matches := FindSimilar(input, candidates, 0.0)
		return len(matches) <= 5
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarSortedDescending verifies results are sorted by
// similarity in descending order.
func TestPropertyFindSimilarSortedDescending(t *testing.T) {
	f := func(input string) bool {
		candidates := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}
		matches := FindSimilar(input, candidates, 0.0)
		for i := 1; i < len(matches); i++ {
			if matches[i].Similarity > matches[i-1].Similarity {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarSimilarityBounded verifies similarity is in [0, 1].
func TestPropertyFindSimilarSimilarityBounded(t *testing.T) {
	f := func(input string) bool {
		candidates := []string{"alpha", "bravo", "charlie"}
		matches := FindSimilar(input, candidates, 0.0)
		for _, m := range matches {
			if m.Similarity < 0.0 || m.Similarity > 1.0 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarDistanceNonNegative verifies Distance is always >= 0.
func TestPropertyFindSimilarDistanceNonNegative(t *testing.T) {
	f := func(input string) bool {
		candidates := []string{"alpha", "bravo", "charlie"}
		matches := FindSimilar(input, candidates, 0.0)
		for _, m := range matches {
			if m.Distance < 0 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarThresholdFilter verifies all returned matches meet
// the threshold requirement.
func TestPropertyFindSimilarThresholdFilter(t *testing.T) {
	f := func(input string) bool {
		threshold := 0.5
		candidates := []string{"alpha", "bravo", "charlie", "delta"}
		matches := FindSimilar(input, candidates, threshold)
		for _, m := range matches {
			if m.Similarity < threshold {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSimilarHigherThresholdFewerResults verifies that a higher
// threshold produces equal or fewer results than a lower one.
func TestPropertyFindSimilarHigherThresholdFewerResults(t *testing.T) {
	f := func(input string) bool {
		candidates := []string{"alpha", "bravo", "charlie", "delta", "echo"}
		matchesLow := FindSimilar(input, candidates, 0.3)
		matchesHigh := FindSimilar(input, candidates, 0.7)
		return len(matchesHigh) <= len(matchesLow)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
