package analyzer

import (
	"regexp"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/validator"
)

// AnalyzePatterns groups classified denials by pattern and computes per-pattern statistics.
func AnalyzePatterns(classified []ClassifiedDenial) []PatternAnalysis {
	// Group by PatternName.
	groups := make(map[string][]ClassifiedDenial)
	for _, cd := range classified {
		groups[cd.Denial.PatternName] = append(groups[cd.Denial.PatternName], cd)
	}

	// Build PatternAnalysis for each group.
	var results []PatternAnalysis
	for name, denials := range groups {
		pa := PatternAnalysis{
			PatternName:  name,
			TotalDenials: len(denials),
		}

		// Look up index and regex from validator.
		for i := 0; i < validator.PatternCount(); i++ {
			if validator.PatternName(i) == name {
				pa.PatternIndex = i
				pa.PatternRegex = validator.PatternRegex(i)
				break
			}
		}

		for _, cd := range denials {
			if cd.IsFalsePositive {
				pa.FalsePositives++
				if len(pa.ExampleFPs) < 5 {
					pa.ExampleFPs = append(pa.ExampleFPs, cd)
				}
			} else if cd.Outcome != OutcomeUnknown && cd.Outcome != OutcomeTranscriptMissing {
				pa.TruePositives++
				if len(pa.ExampleTPs) < 3 {
					pa.ExampleTPs = append(pa.ExampleTPs, cd)
				}
			} else {
				pa.Uncertain++
			}
		}

		if pa.TotalDenials > 0 {
			pa.FalsePositiveRate = float64(pa.FalsePositives) / float64(pa.TotalDenials)
		}

		results = append(results, pa)
	}

	// Sort by FalsePositiveRate descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].FalsePositiveRate > results[j].FalsePositiveRate
	})

	return results
}

// ProposePatternFixes attempts to generate tighter regexes for high-FP patterns.
// For patterns with FP rate > 0.2 and > 5 denials, it analyzes whether FPs tend to
// match the pattern word inside a larger token and proposes adding word-boundary prefixes.
func ProposePatternFixes(analyses []PatternAnalysis, allDenials []ClassifiedDenial) []PatternAnalysis {
	// Index denials by pattern name for quick lookup.
	byPattern := make(map[string][]ClassifiedDenial)
	for _, cd := range allDenials {
		byPattern[cd.Denial.PatternName] = append(byPattern[cd.Denial.PatternName], cd)
	}

	for i := range analyses {
		pa := &analyses[i]
		if pa.FalsePositiveRate <= 0.2 || pa.TotalDenials <= 5 {
			continue
		}

		denials := byPattern[pa.PatternName]
		if len(denials) == 0 {
			continue
		}

		// Collect FP and TP commands.
		var fpCmds, tpCmds []string
		for _, cd := range denials {
			if cd.IsFalsePositive {
				fpCmds = append(fpCmds, cd.Denial.Command)
			} else if cd.Outcome != OutcomeUnknown && cd.Outcome != OutcomeTranscriptMissing {
				tpCmds = append(tpCmds, cd.Denial.Command)
			}
		}

		if len(fpCmds) == 0 {
			continue
		}

		// Simple heuristic: try adding (^|\s) prefix to the pattern.
		// This helps when the pattern word appears inside a path/token in FPs.
		proposedRegex := tryWordBoundaryPrefix(pa.PatternRegex)
		if proposedRegex == pa.PatternRegex {
			continue
		}

		proposed, err := regexp.Compile(proposedRegex)
		if err != nil {
			continue
		}

		original, err := regexp.Compile(pa.PatternRegex)
		if err != nil {
			continue
		}

		// Count how many FPs the proposed regex fixes.
		var fpsFixed int
		var exampleFixed []string
		for _, cmd := range fpCmds {
			if original.MatchString(cmd) && !proposed.MatchString(cmd) {
				fpsFixed++
				if len(exampleFixed) < 3 {
					exampleFixed = append(exampleFixed, cmd)
				}
			}
		}

		// Count how many TPs the proposed regex preserves vs loses.
		var tpsPreserved, tpsLost int
		var exampleStillCaught []string
		for _, cmd := range tpCmds {
			if proposed.MatchString(cmd) {
				tpsPreserved++
				if len(exampleStillCaught) < 3 {
					exampleStillCaught = append(exampleStillCaught, cmd)
				}
			} else {
				tpsLost++
			}
		}

		// Only propose if no TPs are lost and at least one FP is fixed.
		if tpsLost > 0 || fpsFixed == 0 {
			continue
		}

		pa.ProposedFix = &PatternFix{
			OriginalRegex:      pa.PatternRegex,
			ProposedRegex:      proposedRegex,
			FPsFixed:           fpsFixed,
			TPsPreserved:       tpsPreserved,
			TPsLost:            tpsLost,
			ExampleFixed:       exampleFixed,
			ExampleStillCaught: exampleStillCaught,
		}
	}

	return analyses
}

// tryWordBoundaryPrefix attempts to add a (^|\s) prefix to a regex pattern
// if it starts with a \b word boundary, converting it to a stricter match.
func tryWordBoundaryPrefix(regex string) string {
	// If the regex starts with \b, replace with (^|\s) for stricter matching.
	if strings.HasPrefix(regex, `\b`) {
		return `(^|\s)` + regex[2:]
	}
	return regex
}
