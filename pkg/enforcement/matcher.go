package enforcement

import (
	"regexp"
)

// MatchResult contains information about a regex match.
type MatchResult struct {
	Matched    bool
	MatchText  string
	Groups     []string
	StartIndex int
	EndIndex   int
}

// Matcher provides regex matching utilities for pattern detection.
type Matcher struct {
	regex *regexp.Regexp
}

// NewMatcher creates a new matcher from a regex pattern string.
func NewMatcher(pattern string) (*Matcher, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &Matcher{regex: regex}, nil
}

// Match checks if the pattern matches the input string.
func (m *Matcher) Match(input string) bool {
	return m.regex.MatchString(input)
}

// FindMatch returns the first match in the input string.
func (m *Matcher) FindMatch(input string) *MatchResult {
	loc := m.regex.FindStringIndex(input)
	if loc == nil {
		return &MatchResult{Matched: false}
	}

	matchText := input[loc[0]:loc[1]]
	groups := m.regex.FindStringSubmatch(input)

	return &MatchResult{
		Matched:    true,
		MatchText:  matchText,
		Groups:     groups,
		StartIndex: loc[0],
		EndIndex:   loc[1],
	}
}

// FindAllMatches returns all matches in the input string.
func (m *Matcher) FindAllMatches(input string) []*MatchResult {
	locs := m.regex.FindAllStringIndex(input, -1)
	if locs == nil {
		return nil
	}

	matches := make([]*MatchResult, 0, len(locs))
	allGroups := m.regex.FindAllStringSubmatch(input, -1)

	for i, loc := range locs {
		matchText := input[loc[0]:loc[1]]
		var groups []string
		if i < len(allGroups) {
			groups = allGroups[i]
		}

		matches = append(matches, &MatchResult{
			Matched:    true,
			MatchText:  matchText,
			Groups:     groups,
			StartIndex: loc[0],
			EndIndex:   loc[1],
		})
	}

	return matches
}

// ReplaceAll replaces all matches with the replacement string.
func (m *Matcher) ReplaceAll(input, replacement string) string {
	return m.regex.ReplaceAllString(input, replacement)
}

// Split splits the input string by the regex pattern.
func (m *Matcher) Split(input string, n int) []string {
	return m.regex.Split(input, n)
}

// QuickMatch checks if a pattern matches without creating a Matcher.
func QuickMatch(pattern, input string) (bool, error) {
	return regexp.MatchString(pattern, input)
}

// ExtractGroups extracts named groups from a match.
func (m *Matcher) ExtractGroups(input string) map[string]string {
	groups := make(map[string]string)
	match := m.regex.FindStringSubmatch(input)
	if match == nil {
		return groups
	}

	names := m.regex.SubexpNames()
	for i, name := range names {
		if i > 0 && i < len(match) && name != "" {
			groups[name] = match[i]
		}
	}

	return groups
}
