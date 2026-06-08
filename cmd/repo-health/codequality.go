package main

import (
	"encoding/json"
	"go/ast"
	"regexp"
	"strconv"
	"strings"
)

// collectCodeQuality fills the code-quality section: lint count, coverage,
// test/source ratio and average cyclomatic complexity.
func collectCodeQuality(sc *scanCtx) CodeQuality {
	cq := CodeQuality{}

	cq.Lint, cq.LintFindings = lintFindings(sc)
	if sc.opts.coverage {
		cq.Coverage, cq.CoveragePct, cq.PackageCoverage = coverage(sc)
	} else {
		cq.Coverage = Metric{Available: false, Note: "skipped (pass --coverage to run the test suite)"}
	}

	// Test/source ratio and complexity are pure AST work over the already
	// parsed sources — always available.
	var totalComplexity, fnCount int
	for _, s := range sc.sources {
		if s.isTest {
			cq.TestFiles++
			continue
		}
		cq.SourceFiles++
		for _, decl := range s.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			totalComplexity += cyclomatic(fn)
			fnCount++
		}
	}
	if cq.SourceFiles > 0 {
		cq.TestSourceRatio = round2(float64(cq.TestFiles) / float64(cq.SourceFiles))
	}
	cq.FunctionsAnalyzed = fnCount
	if fnCount > 0 {
		cq.AvgComplexity = round2(float64(totalComplexity) / float64(fnCount))
	}
	return cq
}

// golangciIssues is the slice we care about in golangci-lint JSON output.
// Both the v1 and v2 JSON schemas expose a top-level "Issues" array.
type golangciIssues struct {
	Issues []struct {
		FromLinter string `json:"FromLinter"`
	} `json:"Issues"`
}

// lintFindings returns the golangci-lint finding count. It tries the v2 and
// legacy JSON flag forms in turn so it works regardless of the installed
// major version, and never treats "findings exist" (non-zero exit) as a
// tool failure via --issues-exit-code=0.
func lintFindings(sc *scanCtx) (Metric, int) {
	if !haveBinary("golangci-lint") {
		return Metric{Available: false, Note: "golangci-lint not on PATH; install it to measure lint findings"}, 0
	}
	attempts := [][]string{
		{"run", "--issues-exit-code=0", "--output.json.path=stdout", "./..."},
		{"run", "--issues-exit-code=0", "--out-format=json", "./..."},
	}
	for _, args := range attempts {
		res := run(sc.root, sc.opts.lintTimeout, "golangci-lint", args...)
		// On the JSON path golangci-lint prints the report to stdout; a flag
		// it does not understand goes to stderr and leaves stdout non-JSON.
		jsonStr := extractJSON(res.stdout)
		if jsonStr == "" {
			continue
		}
		var parsed golangciIssues
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			continue
		}
		return Metric{Available: true}, len(parsed.Issues)
	}
	return Metric{Available: false, Note: "golangci-lint produced no parseable JSON (version flag mismatch?)"}, 0
}

// extractJSON returns the substring from the first '{' to the last '}' so
// banner/warning lines printed before the JSON object don't break parsing.
func extractJSON(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return ""
	}
	return s[i : j+1]
}

var coverageLine = regexp.MustCompile(`^(ok|FAIL)\s+(\S+).*coverage:\s+([0-9.]+)% of statements`)

// coverage runs the test suite with -cover and parses per-package statement
// coverage. It deliberately omits -race (slower, and the repo has known
// race-detector flakes) and tolerates failing packages: a package whose
// tests fail simply contributes no coverage rather than aborting the audit.
// The reported overall percentage is the unweighted mean across packages
// that emitted a coverage figure.
func coverage(sc *scanCtx) (Metric, float64, []PkgCoverage) {
	res := run(sc.root, sc.opts.coverageTimeout, "go", "test", "-cover", "./...")
	// We parse stdout regardless of exit code — a single failing package
	// makes `go test` exit non-zero but the passing packages still report.
	var pkgs []PkgCoverage
	var sum float64
	for _, line := range strings.Split(res.stdout, "\n") {
		m := coverageLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		pct, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		pkgs = append(pkgs, PkgCoverage{Package: m[2], Percent: pct})
		sum += pct
	}
	if len(pkgs) == 0 {
		note := "no package reported coverage"
		if res.stderr != "" {
			note += " (go test failed to build)"
		}
		return Metric{Available: false, Note: note}, 0, nil
	}
	return Metric{Available: true}, round2(sum / float64(len(pkgs))), pkgs
}

// round2 rounds to two decimal places without importing math for one call.
func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
