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
			switch {
			case cd.IsFalsePositive:
				pa.FalsePositives++
				if len(pa.ExampleFPs) < 5 {
					pa.ExampleFPs = append(pa.ExampleFPs, cd)
				}
			case cd.Outcome != OutcomeUnknown && cd.Outcome != OutcomeTranscriptMissing:
				pa.TruePositives++
				if len(pa.ExampleTPs) < 3 {
					pa.ExampleTPs = append(pa.ExampleTPs, cd)
				}
			default:
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
		proposePatternFix(&analyses[i], byPattern)
	}

	return analyses
}

// proposePatternFix evaluates a single pattern: when its FP rate is high
// enough, attempt a stricter regex and attach pa.ProposedFix if the new regex
// drops at least one FP without losing any TPs.
func proposePatternFix(pa *PatternAnalysis, byPattern map[string][]ClassifiedDenial) {
	if pa.FalsePositiveRate <= 0.2 || pa.TotalDenials <= 5 {
		return
	}
	denials := byPattern[pa.PatternName]
	if len(denials) == 0 {
		return
	}
	fpCmds, tpCmds := splitFPsAndTPs(denials)
	if len(fpCmds) == 0 {
		return
	}
	proposedRegex := tryWordBoundaryPrefix(pa.PatternRegex)
	if proposedRegex == pa.PatternRegex {
		return
	}
	proposed, err := regexp.Compile(proposedRegex)
	if err != nil {
		return
	}
	original, err := regexp.Compile(pa.PatternRegex)
	if err != nil {
		return
	}
	fpsFixed, exampleFixed := countFPsFixed(fpCmds, original, proposed)
	tpsPreserved, tpsLost, exampleStillCaught := countTPsKept(tpCmds, proposed)
	if tpsLost > 0 || fpsFixed == 0 {
		return
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

// splitFPsAndTPs separates denials by whether they were judged false positives
// or true positives (excluding unknown/missing-transcript outcomes).
func splitFPsAndTPs(denials []ClassifiedDenial) ([]string, []string) {
	var fpCmds, tpCmds []string
	for _, cd := range denials {
		if cd.IsFalsePositive {
			fpCmds = append(fpCmds, cd.Denial.Command)
		} else if cd.Outcome != OutcomeUnknown && cd.Outcome != OutcomeTranscriptMissing {
			tpCmds = append(tpCmds, cd.Denial.Command)
		}
	}
	return fpCmds, tpCmds
}

// countFPsFixed returns how many FP commands the proposed regex no longer
// matches (vs. the original) plus up to 3 example fixed commands.
func countFPsFixed(fpCmds []string, original, proposed *regexp.Regexp) (int, []string) {
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
	return fpsFixed, exampleFixed
}

// countTPsKept returns how many TP commands the proposed regex still matches
// vs. has lost, plus up to 3 example still-caught commands.
func countTPsKept(tpCmds []string, proposed *regexp.Regexp) (int, int, []string) {
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
	return tpsPreserved, tpsLost, exampleStillCaught
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
