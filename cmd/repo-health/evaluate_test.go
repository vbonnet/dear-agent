package main

import "testing"

func TestStatusExitCode(t *testing.T) {
	cases := map[Status]int{
		StatusHealthy:  0,
		StatusDegraded: 1,
		StatusCritical: 2,
		Status("weird"): 0,
	}
	for s, want := range cases {
		if got := s.ExitCode(); got != want {
			t.Errorf("%s.ExitCode() = %d, want %d", s, got, want)
		}
	}
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		name   string
		issues []Issue
		want   Status
	}{
		{"none", nil, StatusHealthy},
		{"info only", []Issue{{Severity: SeverityInfo}}, StatusHealthy},
		{"warn", []Issue{{Severity: SeverityInfo}, {Severity: SeverityWarn}}, StatusDegraded},
		{"critical wins", []Issue{{Severity: SeverityWarn}, {Severity: SeverityCritical}}, StatusCritical},
	}
	for _, c := range cases {
		if got := verdict(c.issues); got != c.want {
			t.Errorf("%s: verdict = %s, want %s", c.name, got, c.want)
		}
	}
}

// healthyReport is a Report whose metrics are all available and well within
// thresholds — the baseline the threshold tests perturb.
func healthyReport() Report {
	return Report{
		CodeQuality: CodeQuality{
			Lint:            Metric{Available: true},
			LintFindings:    0,
			SourceFiles:     100,
			TestFiles:       50,
			TestSourceRatio: 0.5,
			AvgComplexity:   4.0,
			FunctionsAnalyzed: 200,
		},
		Architecture: Architecture{
			DepGraph:    Metric{Available: true},
			MaxDepDepth: 5,
		},
		AgentHealth: AgentHealth{
			Worktrees:     Metric{Available: true},
			WorktreeCount: 2,
			StaleBranches: Metric{Available: true},
			BDD:           Metric{Available: true},
			EARS:          Metric{Available: true},
		},
		Drift: Drift{
			Chezmoi:    Metric{Available: true},
			Hooks:      Metric{Available: true},
			DocPairing: Metric{Available: true},
		},
	}
}

// hasSeverity reports whether any issue for check has the given severity.
func hasSeverity(issues []Issue, check string, sev Severity) bool {
	for _, is := range issues {
		if is.Check == check && is.Severity == sev {
			return true
		}
	}
	return false
}

func TestEvaluateHealthy(t *testing.T) {
	r := healthyReport()
	issues := evaluate(&r, defaultOptions("/x", "m"))
	if got := verdict(issues); got != StatusHealthy {
		t.Fatalf("healthy report evaluated to %s; issues=%+v", got, issues)
	}
}

func TestEvaluateLintThresholds(t *testing.T) {
	opts := defaultOptions("/x", "m")

	r := healthyReport()
	r.CodeQuality.LintFindings = 5
	if iss := evaluate(&r, opts); !hasSeverity(iss, "lint", SeverityWarn) {
		t.Errorf("5 findings should warn; got %+v", iss)
	}

	r.CodeQuality.LintFindings = lintCritical
	if iss := evaluate(&r, opts); !hasSeverity(iss, "lint", SeverityCritical) {
		t.Errorf("%d findings should be critical", lintCritical)
	}
}

func TestEvaluateWorktreeCritical(t *testing.T) {
	r := healthyReport()
	r.AgentHealth.WorktreeCount = worktreeCritical + 1
	iss := evaluate(&r, defaultOptions("/x", "m"))
	if !hasSeverity(iss, "worktrees", SeverityCritical) {
		t.Errorf("worktree count over %d should be critical; got %+v", worktreeCritical, iss)
	}
}

func TestEvaluateCircularDepsCritical(t *testing.T) {
	r := healthyReport()
	r.Architecture.CircularDeps = [][]string{{"a", "b", "a"}}
	iss := evaluate(&r, defaultOptions("/x", "m"))
	if !hasSeverity(iss, "circular-deps", SeverityCritical) {
		t.Errorf("import cycle should be critical; got %+v", iss)
	}
}

func TestEvaluateUnavailableIsInfoNotFailure(t *testing.T) {
	r := healthyReport()
	r.CodeQuality.Lint = Metric{Available: false, Note: "golangci-lint missing"}
	iss := evaluate(&r, defaultOptions("/x", "m"))
	if verdict(iss) != StatusHealthy {
		t.Errorf("unavailable lint must not degrade the verdict; got %+v", iss)
	}
	if !hasSeverity(iss, "lint", SeverityInfo) {
		t.Errorf("unavailable lint should surface an info note; got %+v", iss)
	}
}

func TestEvaluateComplexityCritical(t *testing.T) {
	r := healthyReport()
	r.CodeQuality.AvgComplexity = complexityCritical + 1
	iss := evaluate(&r, defaultOptions("/x", "m"))
	if !hasSeverity(iss, "complexity", SeverityCritical) {
		t.Errorf("avg complexity over %.0f should be critical", complexityCritical)
	}
}
