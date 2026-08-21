// Package craplens scores the functions a pull request actually changed by
// joining cyclomatic complexity with test coverage, and reports the ones whose
// combination makes them expensive to change safely.
//
// # The gap this closes
//
// Three deterministic gates already run on every diff, and none of them can
// see this:
//
//   - golangci-lint runs gocyclo at min-complexity 15 with
//     new-from-merge-base, so raw complexity on changed code is already
//     gated. It knows nothing about whether that complexity is tested.
//   - golangci-lint runs errcheck with check-blank, so a newly discarded
//     error return is already a hard failure. This package deliberately does
//     not re-implement that check: one rule, one home.
//   - cmd/structural-health has a zero-test scan, but it looks for packages
//     that ship no _test.go files at all. agm/cmd/agm-bus passed that scan
//     while sitting at 0.0% coverage, because its tests built the binary and
//     exec'd it, so every statement they exercised ran in a subprocess and
//     counted for nothing.
//
// What nothing owned is the join: a function can be under the complexity
// ceiling, free of discarded errors, and in a package with test files, and
// still be the riskiest thing in the diff because none of its branches are
// exercised. That is what the CRAP score measures.
//
// # The score
//
//	CRAP(f) = complexity(f)^2 * (1 - coverage(f))^3 + complexity(f)
//
// Alberto Savoia's Crap4j formula, 2007. With full coverage it collapses to
// the complexity itself; with no coverage it is complexity squared plus
// complexity. The cube on the uncovered fraction is what makes partial
// coverage of a branchy function score so much worse than full coverage of a
// branchier one.
//
// # Advisory by construction
//
// Analyze never fails a diff. Coverage that cannot be collected for a package
// is reported as unknown and excluded from scoring rather than treated as
// zero, because several packages in this module need a live tmux socket, a
// Dolt server, or a container runtime, and scoring those as untested would
// train readers to ignore the signal.
package craplens

import (
	"context"
	"fmt"
	"sort"
)

// DefaultThreshold is the CRAP score above which a changed function is
// reported individually. 30 is Crap4j's own default and is what keeps this
// signal short enough to read.
//
// It is not the target. See AgentTarget.
const DefaultThreshold = 30.0

// AgentTarget is the CRAP score Uncle Bob proposes for agent-written code (4
// for human-written, raised to 6 for agents because they hold more branching
// context reliably). Reaching it means a function is either simple or fully
// covered: at 100% coverage CRAP equals complexity, so a score of 6 is six
// fully-tested paths.
//
// It is reported as a proportion rather than enforced per function, because
// listing every changed function above 6 would bury the handful above
// DefaultThreshold that actually need attention.
const AgentTarget = 6.0

// CoverageUnknown marks a function whose package coverage could not be
// collected. Such functions are counted and named, never scored.
const CoverageUnknown = -1.0

// Function is one changed function in the head tree.
type Function struct {
	// Package is the import path the function belongs to.
	Package string
	// File is the repository-relative path of the file holding it.
	File string
	// Name is the function name, receiver included for a method.
	Name string
	// Line is the line the declaration starts on.
	Line int
	// EndLine is the line the declaration ends on. Coverage is derived by
	// intersecting profile blocks with the span from Line to EndLine, so an
	// unset EndLine would let a neighbouring function's blocks leak in.
	EndLine int
	// Complexity is the function's cyclomatic complexity.
	Complexity int
	// Coverage is the fraction of the function's statements exercised by the
	// package's tests, in the range 0 to 1, or CoverageUnknown.
	Coverage float64
}

// CRAP returns the function's change risk anti-pattern score, or
// CoverageUnknown when its coverage could not be collected.
func (f Function) CRAP() float64 {
	if f.Coverage == CoverageUnknown {
		return CoverageUnknown
	}
	c := float64(f.Complexity)
	uncovered := 1 - f.Coverage
	return c*c*uncovered*uncovered*uncovered + c
}

// Package is a package the diff touched.
type Package struct {
	// ImportPath identifies the package.
	ImportPath string
	// Coverage is the package's statement coverage from 0 to 1, or
	// CoverageUnknown.
	Coverage float64
	// New reports whether every changed Go file in the package is newly added,
	// which is the case the audit cared about most: a package arriving with no
	// tests at all.
	New bool
}

// Report is the outcome of scoring one diff.
type Report struct {
	// Threshold is the score above which functions were reported.
	Threshold float64
	// Scored is the count of changed functions whose coverage was known.
	Scored int
	// Unmeasured is the count of changed functions whose coverage could not be
	// determined even though their package total was known, which happens for
	// build-tagged files excluded from the profile on this platform.
	Unmeasured int
	// WithinAgentTarget is how many Scored functions are at or under
	// AgentTarget.
	WithinAgentTarget int
	// Unknown names the packages whose coverage could not be collected.
	Unknown []string
	// Over holds the changed functions above Threshold, worst first.
	Over []Function
	// Untested holds touched packages measured at zero coverage.
	Untested []Package
	// Changed is the count of changed functions found, scored or not.
	Changed int
	// CheckoutMismatch reports that the working tree was not at the head
	// revision, so no coverage could be measured and nothing was scored.
	CheckoutMismatch bool
}

// Flagged reports whether the diff produced anything worth commenting on.
// A report with nothing flagged is the normal outcome and must stay silent.
//
// A checkout mismatch is never flagged: nothing was measured, so there is
// nothing to report and a comment would be noise.
func (r Report) Flagged() bool {
	if r.CheckoutMismatch {
		return false
	}
	return len(r.Over) > 0 || len(r.Untested) > 0
}

// Analyze scores the functions changed between base and head.
//
// It never returns an error for an unhealthy diff, only for an unusable one:
// missing revisions, or a diff that cannot be read at all.
func Analyze(ctx context.Context, repoDir, base, head string, threshold float64) (Report, error) {
	if base == "" || head == "" {
		return Report{}, fmt.Errorf("both a base and a head revision are required")
	}
	if threshold <= 0 {
		threshold = DefaultThreshold
	}

	touched, err := changedGoFiles(ctx, repoDir, base, head)
	if err != nil {
		return Report{}, err
	}
	if len(touched) == 0 {
		return Report{Threshold: threshold}, nil
	}

	funcs := changedFunctions(ctx, repoDir, head, touched)
	pkgs := packagesOf(touched)

	// Coverage is measured against the checkout while spans and complexity
	// come from the committed head tree, so both must describe the same code.
	// A different revision, or the same revision with uncommitted or
	// untracked changes, makes the two disagree: an untracked test can make an
	// uncovered function look covered.
	if !headIsCheckedOut(ctx, repoDir, head) || !workingTreeIsClean(ctx, repoDir) {
		return Report{
			Threshold:        threshold,
			Changed:          len(funcs),
			Unknown:          sortedSlice(pkgs),
			CheckoutMismatch: true,
		}, nil
	}

	cov := collectCoverage(ctx, repoDir, pkgs)
	applyCoverage(funcs, cov)

	report := Report{Threshold: threshold, Changed: len(funcs)}
	scoreFunctions(&report, funcs, threshold)
	classifyPackages(ctx, &report, cov, touched, repoDir, base)
	return report, nil
}

// applyCoverage attaches each function's measured coverage, leaving it unknown
// when its package could not be measured.
func applyCoverage(funcs []Function, cov coverageData) {
	for i := range funcs {
		pkgCov, ok := cov.packages[funcs[i].Package]
		if !ok || pkgCov.coverage == CoverageUnknown {
			funcs[i].Coverage = CoverageUnknown
			continue
		}
		funcs[i].Coverage = cov.functionCoverage(funcs[i])
	}
}

// scoreFunctions fills in the scored counts and the over-threshold list,
// ordered worst first.
func scoreFunctions(report *Report, funcs []Function, threshold float64) {
	for _, f := range funcs {
		if f.Coverage == CoverageUnknown {
			// A file excluded from the profile (a build-tagged file on the
			// wrong runner, for example) leaves its functions unmeasured even
			// when the package total is known. Counting them keeps that
			// visible instead of silently narrowing what was scored.
			report.Unmeasured++
			continue
		}
		report.Scored++
		score := f.CRAP()
		if score <= AgentTarget {
			report.WithinAgentTarget++
		}
		if score > threshold {
			report.Over = append(report.Over, f)
		}
	}
	sort.Slice(report.Over, func(i, j int) bool {
		a, b := report.Over[i], report.Over[j]
		if a.CRAP() != b.CRAP() {
			return a.CRAP() > b.CRAP()
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}

// classifyPackages separates the touched packages that could not be measured
// from those measured at zero coverage.
func classifyPackages(ctx context.Context, report *Report, cov coverageData, touched touchedSet, repoDir, base string) {
	for _, p := range sortedKeys(cov.packages) {
		switch cov.packages[p].coverage {
		case CoverageUnknown:
			report.Unknown = append(report.Unknown, p)
		case 0:
			// New only when the diff adds every changed file in the package
			// AND the directory held no Go source at the base. Without the
			// second check, adding one file to an existing package would
			// report the whole package as new.
			isNew := touched.allFilesAdded(p) && !treeHasGoFiles(ctx, repoDir, base, p)
			report.Untested = append(report.Untested, Package{
				ImportPath: p,
				Coverage:   0,
				New:        isNew,
			})
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sortedSlice returns a sorted copy, so report fields have a stable order
// regardless of map iteration.
func sortedSlice(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
