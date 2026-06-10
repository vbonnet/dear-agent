package main

import (
	"time"
)

// Options configures a scan: where to look, what to run, and the structural
// thresholds. Timeouts bound every external command so a hung tool can never
// wedge the auditor.
type Options struct {
	root     string
	module   string
	coverage bool

	maxFileLines int // files longer than this are flagged (default 500)
	maxFuncLines int // functions longer than this are flagged (default 50)
	staleDays    int // branches older than this are "stale" (default 30)

	lintTimeout     time.Duration
	coverageTimeout time.Duration
	goListTimeout   time.Duration
	gitTimeout      time.Duration
}

// defaultOptions returns Options with the documented defaults applied.
func defaultOptions(root, module string) Options {
	return Options{
		root:            root,
		module:          module,
		maxFileLines:    500,
		maxFuncLines:    50,
		staleDays:       30,
		lintTimeout:     5 * time.Minute,
		coverageTimeout: 10 * time.Minute,
		goListTimeout:   2 * time.Minute,
		gitTimeout:      30 * time.Second,
	}
}

// scanCtx carries everything the collectors need. Parsing the Go tree once
// up front (sources) keeps every AST-based metric cheap.
type scanCtx struct {
	root    string
	module  string
	now     time.Time
	opts    Options
	sources []goSource
	skipped []string
}

// scan runs every collector and returns a fully populated, evaluated Report.
// now is injected (not time.Now) so tests are deterministic.
func scan(opts Options, now time.Time) Report {
	sources, skipped := parseGoFiles(opts.root)
	sc := &scanCtx{
		root:    opts.root,
		module:  opts.module,
		now:     now,
		opts:    opts,
		sources: sources,
		skipped: skipped,
	}

	rep := Report{
		GeneratedAt:  now,
		Repo:         opts.root,
		Commit:       headCommit(sc),
		CodeQuality:  collectCodeQuality(sc),
		Architecture: collectArchitecture(sc),
		AgentHealth:  collectAgentHealth(sc),
		Drift:        collectDrift(sc),
	}
	rep.Issues = evaluate(&rep, opts)
	rep.Status = verdict(rep.Issues)
	return rep
}

// headCommit returns the short HEAD SHA, or "unknown" off a git checkout.
func headCommit(sc *scanCtx) string {
	res := run(sc.root, sc.opts.gitTimeout, "git", "rev-parse", "--short", "HEAD")
	if res.ok() && res.stdout != "" {
		return res.stdout
	}
	return "unknown"
}
