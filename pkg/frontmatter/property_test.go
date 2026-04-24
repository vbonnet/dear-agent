package frontmatter

import (
	"testing"
	"testing/quick"
)

// TestPropertyLevenshteinNonNegative verifies edit distance is always >= 0.
func TestPropertyLevenshteinNonNegative(t *testing.T) {
	f := func(a, b string) bool {
		return levenshteinDistance(a, b) >= 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyLevenshteinSymmetric verifies distance(a,b) == distance(b,a).
func TestPropertyLevenshteinSymmetric(t *testing.T) {
	f := func(a, b string) bool {
		return levenshteinDistance(a, b) == levenshteinDistance(b, a)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyLevenshteinIdentity verifies distance(x, x) == 0.
func TestPropertyLevenshteinIdentity(t *testing.T) {
	f := func(s string) bool {
		return levenshteinDistance(s, s) == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyLevenshteinTriangleInequality verifies d(a,c) <= d(a,b) + d(b,c).
func TestPropertyLevenshteinTriangleInequality(t *testing.T) {
	f := func(a, b, c string) bool {
		dAC := levenshteinDistance(a, c)
		dAB := levenshteinDistance(a, b)
		dBC := levenshteinDistance(b, c)
		return dAC <= dAB+dBC
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyLevenshteinBounded verifies distance <= max(len(a), len(b)).
func TestPropertyLevenshteinBounded(t *testing.T) {
	f := func(a, b string) bool {
		d := levenshteinDistance(a, b)
		maxLen := maxInt(len([]rune(a)), len([]rune(b)))
		return d <= maxLen
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFuzzyMatchSelfAlwaysTrue verifies FuzzyMatch(x, x, threshold) is
// true for any reasonable threshold <= 1.0.
func TestPropertyFuzzyMatchSelfAlwaysTrue(t *testing.T) {
	p := NewParser()
	f := func(s string) bool {
		// Self-match should always succeed at threshold 1.0
		return p.FuzzyMatch(s, s, 1.0)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFuzzyMatchSymmetric verifies FuzzyMatch(a,b,t) == FuzzyMatch(b,a,t).
func TestPropertyFuzzyMatchSymmetric(t *testing.T) {
	p := NewParser()
	f := func(a, b string) bool {
		threshold := 0.75
		return p.FuzzyMatch(a, b, threshold) == p.FuzzyMatch(b, a, threshold)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyParseNeverErrors verifies Parse never returns an error for any input string.
func TestPropertyParseNeverErrors(t *testing.T) {
	p := NewParser()
	f := func(markdown string) bool {
		_, err := p.Parse(markdown)
		return err == nil
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyParseSectionLevelsValid verifies all parsed sections have level 1-6.
func TestPropertyParseSectionLevelsValid(t *testing.T) {
	p := NewParser()
	f := func(markdown string) bool {
		sections, err := p.Parse(markdown)
		if err != nil {
			return false
		}
		for _, s := range sections {
			if s.Level < 1 || s.Level > 6 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyParseSectionStartLinesPositive verifies all StartLine values are >= 1.
func TestPropertyParseSectionStartLinesPositive(t *testing.T) {
	p := NewParser()
	f := func(markdown string) bool {
		sections, err := p.Parse(markdown)
		if err != nil {
			return false
		}
		for _, s := range sections {
			if s.StartLine < 1 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFindSectionsExactSubsetOfFuzzy verifies that exact matches are
// always a subset of fuzzy matches (anything found exactly is also found fuzzily).
func TestPropertyFindSectionsExactSubsetOfFuzzy(t *testing.T) {
	p := NewParser()
	f := func(heading string) bool {
		sections := []Section{
			{Heading: heading, Level: 1, StartLine: 1},
		}
		exact := p.FindSections(sections, heading, false)
		fuzzy := p.FindSections(sections, heading, true)
		// If exact found it, fuzzy must also find it
		if len(exact) > 0 && len(fuzzy) == 0 {
			return false
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
