// Command merge-audit is the detection tier of safe-merge (P6).
//
// # Why this exists
//
// safe-merge gates every merge at the point of merge, but gates can be bypassed
// (admin merge with enforce_admins:false, a break-glass override, a direct push
// to a default branch, or a quietly drifted ruleset). Prevention is never
// perfect, so the detection tier scans history after the fact and makes every
// bypass *visible*: it files a P1 bead per violation and appends an audit-history
// record. Detect + alert only — it never reverts or rewrites history.
//
// It is the cron/CLI half of P6. The post-merge half lives in
// .github/workflows/merge-audit.yml, which validates a single merge in-repo on
// public repositories. This command sweeps *all* tracked repos weekly.
//
// # What it checks (per PR merged in the lookback window, per repo)
//
//  1. unresolved-threads  — review threads still open at merge time.
//  2. checks-incomplete   — required checks red or pending at mergedAt.
//  3. direct-push         — commits on the default branch with no associated PR.
//  4. break-glass         — override-audit records (gate bypasses) in the window.
//  5. ruleset-drift       — live branch ruleset differs from the checked-in
//     .github/rulesets/main.json source of truth.
//
// # Usage
//
//	merge-audit [flags]
//
//	  --repos owner/repo,owner/repo   explicit repo list (skips discovery)
//	  --src   DIR                     discover repos under DIR (default ~/src)
//	  --days  N                       lookback window in days (default 7)
//	  --ruleset PATH                  local ruleset source (default <repo>/.github/rulesets/main.json)
//	  --beads-db PATH                 beads db for filing violations (default ~/beads/context-engine/.beads)
//	  --json                          emit machine-readable JSON instead of a table
//	  --dry-run                       do not file beads or append history; report only
//
// All GitHub access goes through an authenticated gh CLI; git history is read via
// gh api, so no clone/checkout is required.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// defaultLookbackDays is the audit window when --days is not given. Matches the
// weekly cadence and the bypassed-merge-audit lookback.
const (
	defaultLookbackDays              = 7
	dearAgentRulesetID         int64 = 18061003
	githubActionsIntegrationID int64 = 15368
	legacyRulesetName                = "branch-protection"
)

// Violation is one detected safe-merge bypass. It is the unit of both the
// human/JSON report and the per-violation bead.
type Violation struct {
	Repo       string `json:"repo"`        // owner/repo
	Type       string `json:"type"`        // unresolved-threads | checks-incomplete | direct-push | break-glass | ruleset-drift
	Ref        string `json:"ref"`         // PR ref ("#123"), short SHA, or "" for repo-level
	Detail     string `json:"detail"`      // human-readable explanation
	DetectedAt string `json:"detected_at"` // RFC3339 timestamp, stamped at report time
}

// Violation type constants — also the bead-title prefixes.
const (
	typeUnresolvedThreads = "unresolved-threads"
	typeChecksIncomplete  = "checks-incomplete"
	typeDirectPush        = "direct-push"
	typeBreakGlass        = "break-glass"
	typeRulesetDrift      = "ruleset-drift"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

type runDependencies struct {
	auditRepo         func(context.Context, string, time.Time, string) ([]Violation, error)
	readFile          func(string) ([]byte, error)
	stdout            io.Writer
	stderr            io.Writer
	persistViolations func([]Violation, options, io.Writer)
}

type options struct {
	repos       []string
	src         string
	days        int
	rulesetPath string
	beadsDB     string
	jsonOut     bool
	dryRun      bool
	now         time.Time
}

func run(args []string) int {
	return runWithDependencies(args, runDependencies{
		auditRepo:         auditRepo,
		readFile:          os.ReadFile,
		stdout:            os.Stdout,
		stderr:            os.Stderr,
		persistViolations: persistViolations,
	})
}

// auditTimeout bounds a whole audit run. It is generous because the run scales
// with the managed fleet and each repository costs several GitHub queries; it
// exists to convert a hang into a reported incomplete audit, not to police
// normal duration.
const auditTimeout = 30 * time.Minute

func runWithDependencies(args []string, deps runDependencies) int {
	opts, exitCode := parseOptions(args, deps.stderr)
	if exitCode != 0 {
		return exitCode
	}

	// Every GitHub query below shells out to `gh`. An unbounded run in the
	// scheduled workflow would hold a runner until the job timeout kills it,
	// and a killed job reports no findings at all — the audit's worst
	// outcome, since MAC-06 exists precisely so an unanswerable audit never
	// reads as clean. A deadline lets the run end as an explicit incomplete
	// audit instead.
	ctx, cancel := context.WithTimeout(context.Background(), auditTimeout)
	defer cancel()

	repos, err := resolveRepositories(ctx, opts)
	if err != nil {
		fmt.Fprintf(deps.stderr, "merge-audit: %v\n", err)
		return 1
	}

	violations, auditErrors := auditRepositories(ctx, repos, opts, deps.auditRepo)
	if data, err := deps.readFile(overrideLogPath()); err == nil {
		since := opts.now.AddDate(0, 0, -opts.days)
		violations = append(violations, breakGlassViolations(data, since)...)
	}
	stampViolations(violations, opts.now)
	return finishAudit(violations, auditErrors, opts, deps)
}

func parseOptions(args []string, stderr io.Writer) (options, int) {
	fs := flag.NewFlagSet("merge-audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		reposCSV = fs.String("repos", "", "comma-separated owner/repo list (skips discovery)")
		src      = fs.String("src", defaultSrcDir(), "discover repos under this dir")
		days     = fs.Int("days", defaultLookbackDays, "lookback window in days")
		ruleset  = fs.String("ruleset", "", "local ruleset source path (default <repo>/.github/rulesets/main.json)")
		beadsDB  = fs.String("beads-db", defaultBeadsDB(), "beads db path for filing violations")
		jsonOut  = fs.Bool("json", false, "emit JSON instead of a table")
		dryRun   = fs.Bool("dry-run", false, "report only; do not file beads or append history")
	)
	if err := fs.Parse(args); err != nil {
		return options{}, 2
	}

	opts := options{
		src:         *src,
		days:        *days,
		rulesetPath: *ruleset,
		beadsDB:     *beadsDB,
		jsonOut:     *jsonOut,
		dryRun:      *dryRun,
		now:         time.Now().UTC(),
	}
	if *reposCSV != "" {
		opts.repos = splitCSV(*reposCSV)
	}
	if opts.days <= 0 {
		fmt.Fprintf(stderr, "merge-audit: --days must be a positive integer, got %d\n", opts.days)
		return options{}, 1
	}
	return opts, 0
}

func resolveRepositories(ctx context.Context, opts options) ([]string, error) {
	repos := opts.repos
	if len(repos) == 0 {
		discovered, err := discoverRepos(ctx, opts.src)
		if err != nil {
			return nil, fmt.Errorf("discover repos under %s: %w", opts.src, err)
		}
		repos = discovered
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repos to audit (use --repos or --src)")
	}
	return repos, nil
}

func auditRepositories(ctx context.Context, repos []string, opts options, auditRepoFn func(context.Context, string, time.Time, string) ([]Violation, error)) ([]Violation, []string) {
	since := opts.now.AddDate(0, 0, -opts.days)
	var violations []Violation
	var auditErrors []string
	for _, repo := range repos {
		vs, err := auditRepoFn(ctx, repo, since, opts.rulesetPath)
		// A repository audit may discover durable findings before a later,
		// independent evidence source fails. Retain those findings while still
		// marking the overall report incomplete.
		violations = append(violations, vs...)
		if err != nil {
			auditErrors = append(auditErrors, fmt.Sprintf("%s: %v", repo, err))
			continue
		}
	}
	return violations, auditErrors
}

func stampViolations(violations []Violation, now time.Time) {
	stamp := now.Format(time.RFC3339)
	for i := range violations {
		violations[i].DetectedAt = stamp
	}
}

func finishAudit(violations []Violation, auditErrors []string, opts options, deps runDependencies) int {
	// A partial audit must not masquerade as a valid JSON report. In particular,
	// never emit [] when every selected repository failed before comparison.
	if len(auditErrors) > 0 {
		for _, auditErr := range auditErrors {
			fmt.Fprintf(deps.stderr, "merge-audit: %s\n", auditErr)
		}
		if len(violations) > 0 {
			fmt.Fprintln(deps.stderr, "merge-audit: partial findings retained from checks completed before the audit error:")
			if err := writeReport(deps.stderr, violations, false); err != nil {
				fmt.Fprintf(deps.stderr, "merge-audit: report partial findings: %v\n", err)
			}
			if !opts.dryRun {
				deps.persistViolations(violations, opts, deps.stderr)
			}
		}
		fmt.Fprintln(deps.stderr, "merge-audit: audit incomplete; refusing to emit a partial report on stdout")
		return 1
	}
	if err := report(violations, opts, deps.stdout, deps.stderr, deps.persistViolations); err != nil {
		fmt.Fprintf(deps.stderr, "merge-audit: %v\n", err)
		return 1
	}
	return 0
}

type repoAuditDependencies struct {
	defaultBranch     func(context.Context, string) (string, error)
	auditMergedPRs    func(context.Context, string, string, time.Time) ([]Violation, error)
	auditDirectPushes func(context.Context, string, string, time.Time) ([]Violation, error)
	auditRulesetDrift func(context.Context, string, string) ([]Violation, error)
}

// auditRepo runs all five checks against one repo and returns every finding
// discovered before an evidence error. The caller treats a non-nil error as an
// incomplete audit, even when the returned slice is non-empty.
func auditRepo(ctx context.Context, repo string, since time.Time, rulesetPath string) ([]Violation, error) {
	return auditRepoWithDependencies(ctx, repo, since, rulesetPath, repoAuditDependencies{
		defaultBranch:     defaultBranch,
		auditMergedPRs:    auditMergedPRs,
		auditDirectPushes: auditDirectPushes,
		auditRulesetDrift: auditRulesetDrift,
	})
}

// auditRepoWithDependencies runs the three per-repo evidence sources
// (merged-PR review/checks, direct pushes, ruleset drift) independently.
// Per MAC-09, a failure in one evidence source must not suppress findings
// from another: every source still runs and every finding it returns is
// retained, even when an earlier source in this same function already
// failed. The combined error (nil if every source succeeded) marks the
// overall audit incomplete without discarding what was gathered.
func auditRepoWithDependencies(ctx context.Context, repo string, since time.Time, rulesetPath string, deps repoAuditDependencies) ([]Violation, error) {
	var out []Violation
	var errs []error

	branch, err := deps.defaultBranch(ctx, repo)
	if err != nil {
		// Ruleset drift does not depend on the default branch name, so it
		// still runs even when this resolution fails.
		errs = append(errs, fmt.Errorf("default branch: %w", err))
	} else {
		prVs, err := deps.auditMergedPRs(ctx, repo, branch, since)
		out = append(out, prVs...)
		if err != nil {
			errs = append(errs, err)
		}

		pushVs, err := deps.auditDirectPushes(ctx, repo, branch, since)
		out = append(out, pushVs...)
		if err != nil {
			errs = append(errs, err)
		}
	}

	rulesetVs, err := deps.auditRulesetDrift(ctx, repo, rulesetPath)
	out = append(out, rulesetVs...)
	if err != nil {
		errs = append(errs, err)
	}

	return out, errors.Join(errs...)
}

// auditMergedPRs runs checks 1 (unresolved threads) and 2 (incomplete checks)
// for every PR merged into branch in the window.
func auditMergedPRs(ctx context.Context, repo, branch string, since time.Time) ([]Violation, error) {
	return auditMergedPRsWithDependencies(ctx, repo, branch, since, mergedPRAuditDependencies{
		mergedPRs:             mergedPRs,
		unresolvedThreadCount: unresolvedThreadCount,
		checkRuns:             checkRuns,
	})
}

type mergedPRAuditDependencies struct {
	mergedPRs             func(context.Context, string, string, time.Time) ([]mergedPR, error)
	unresolvedThreadCount func(context.Context, string, int) (int, error)
	checkRuns             func(context.Context, string, string) ([]checkRun, error)
}

func auditMergedPRsWithDependencies(ctx context.Context, repo, branch string, since time.Time, deps mergedPRAuditDependencies) ([]Violation, error) {
	prs, err := deps.mergedPRs(ctx, repo, branch, since)
	if err != nil {
		return nil, fmt.Errorf("merged PRs: %w", err)
	}
	var out []Violation
	for _, pr := range prs {
		threads, err := deps.unresolvedThreadCount(ctx, repo, pr.Number)
		if err != nil {
			return out, fmt.Errorf("PR #%d threads: %w", pr.Number, err)
		}
		if threads > 0 {
			out = append(out, Violation{
				Repo:   repo,
				Type:   typeUnresolvedThreads,
				Ref:    fmt.Sprintf("#%d", pr.Number),
				Detail: fmt.Sprintf("PR #%d (%s) merged with %d unresolved review thread(s)", pr.Number, pr.Title, threads),
			})
		}

		runs, err := deps.checkRuns(ctx, repo, pr.HeadSHA)
		if err != nil {
			return out, fmt.Errorf("PR #%d checks: %w", pr.Number, err)
		}
		if red, pending := redOrPendingChecks(runs, pr.MergedAt); len(red) > 0 || len(pending) > 0 {
			out = append(out, Violation{
				Repo:   repo,
				Type:   typeChecksIncomplete,
				Ref:    fmt.Sprintf("#%d", pr.Number),
				Detail: checksDetail(pr.Number, pr.Title, red, pending),
			})
		}
	}
	return out, nil
}

// auditDirectPushes runs check 3: commits on the default branch with no PR.
func auditDirectPushes(ctx context.Context, repo, branch string, since time.Time) ([]Violation, error) {
	commits, err := branchCommits(ctx, repo, branch, since)
	if err != nil {
		return nil, fmt.Errorf("commits: %w", err)
	}
	var out []Violation
	for _, c := range commits {
		if isDirectPush(c.Message, c.Parents) {
			out = append(out, Violation{
				Repo:   repo,
				Type:   typeDirectPush,
				Ref:    shortSHA(c.SHA),
				Detail: fmt.Sprintf("commit %s on %s has no associated PR: %q", shortSHA(c.SHA), branch, firstLine(c.Message)),
			})
		}
	}
	return out, nil
}

// canonicalRulesetSources declares the repositories whose checked-in policy is
// authoritative. Other fleet repositories are intentionally inventory-owned,
// so their lack of this file is not a failed comparison.
var canonicalRulesetSources = map[string]string{
	"vbonnet/dear-agent": ".github/rulesets/main.json",
}

// auditRulesetDrift runs check 5: live branch ruleset vs the checked-in source.
// An explicit --ruleset declares the source expected for every selected repo.
// Missing, unreadable, malformed, or absent declared policy fails closed;
// undeclared fleet repositories are skipped rather than made unauditable.
func auditRulesetDrift(ctx context.Context, repo, rulesetPath string) ([]Violation, error) {
	localPath, declared := canonicalRulesetPath(repo, rulesetPath)
	if !declared {
		return nil, nil
	}
	local, err := os.ReadFile(localPath) // #nosec G304 — operator-supplied audit input path
	if err != nil {
		return nil, fmt.Errorf("read canonical ruleset %s: %w", localPath, err)
	}
	expected, err := parseRuleset(local)
	if err != nil {
		return nil, fmt.Errorf("parse canonical ruleset %s: %w", localPath, err)
	}
	if err := validateCanonicalRuleset(expected, repo); err != nil {
		return nil, fmt.Errorf("validate canonical ruleset %s: %w", localPath, err)
	}
	live, err := liveRuleset(ctx, repo, expected.Name)
	if err != nil {
		return nil, fmt.Errorf("load expected ruleset %q: %w", expected.Name, err)
	}
	var out []Violation
	for _, d := range rulesetDrift(local, live) {
		out = append(out, Violation{Repo: repo, Type: typeRulesetDrift, Detail: d})
	}
	return out, nil
}

func canonicalRulesetPath(repo, override string) (string, bool) {
	if override != "" {
		return override, true
	}
	canonicalRepo := strings.ToLower(strings.TrimSpace(repo))
	relative, ok := canonicalRulesetSources[canonicalRepo]
	if !ok {
		return "", false
	}
	return filepath.Join(homeDir(), "src", repoName(canonicalRepo), filepath.FromSlash(relative)), true
}

// ---- pure classification (unit-tested) ----------------------------------

// checkRun is the subset of a GitHub check-run we reason about.
type checkRun struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

// redOrPendingChecks splits check-runs into those that failed before the merge
// and those still in-flight at merge time. It mirrors the bypassed-merge-audit
// jq logic so the two detectors agree. mergedAt is an RFC3339 timestamp.
func redOrPendingChecks(runs []checkRun, mergedAt string) (red, pending []string) {
	for _, r := range runs {
		completedBeforeMerge := r.CompletedAt != "" && r.CompletedAt <= mergedAt
		startedBeforeMerge := r.StartedAt != "" && r.StartedAt <= mergedAt
		if r.Status == "completed" && completedBeforeMerge {
			switch r.Conclusion {
			case "success", "skipped", "neutral", "":
				// not a failure
			default:
				red = append(red, fmt.Sprintf("%s [%s]", r.Name, r.Conclusion))
			}
			continue
		}
		// Started before the merge but not completed by merge time → pending.
		if startedBeforeMerge && (r.Status != "completed" || r.CompletedAt > mergedAt) {
			pending = append(pending, r.Name)
		}
	}
	return red, pending
}

// checksDetail formats a checks-incomplete violation detail.
func checksDetail(pr int, title string, red, pending []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PR #%d (%s) merged on red:", pr, title)
	if len(red) > 0 {
		fmt.Fprintf(&b, " failed: %s;", strings.Join(red, ", "))
	}
	if len(pending) > 0 {
		fmt.Fprintf(&b, " in-progress at merge: %s;", strings.Join(pending, ", "))
	}
	return strings.TrimRight(b.String(), ";")
}

// isDirectPush reports whether a default-branch commit looks like a direct push
// rather than a merged PR. A squash merge has one parent and a "(#NNN)" suffix;
// a true merge commit has two parents. A direct push has one parent and no PR
// reference and is not itself a "Merge …" commit.
func isDirectPush(message string, parents int) bool {
	if parents != 1 {
		return false // merge commits (2 parents) and roots (0) are not direct pushes
	}
	first := firstLine(message)
	if strings.HasPrefix(first, "Merge ") {
		return false
	}
	return !containsPRRef(message)
}

// containsPRRef reports whether the message carries a "(#123)" PR reference,
// the marker GitHub appends to squash-merge commit subjects.
func containsPRRef(message string) bool {
	for {
		i := strings.IndexByte(message, '#')
		if i < 0 || i == 0 {
			return digitsParenAfter(message, i)
		}
		if message[i-1] == '(' && digitsParenAfter(message, i) {
			return true
		}
		message = message[i+1:]
	}
}

// digitsParenAfter checks that message[i+1:] is one-or-more digits then ')'.
func digitsParenAfter(message string, i int) bool {
	j := i + 1
	n := 0
	for j < len(message) && message[j] >= '0' && message[j] <= '9' {
		j++
		n++
	}
	return n > 0 && j < len(message) && message[j] == ')'
}

// ---- break-glass (override-audit) ---------------------------------------

// overrideRecord is the subset of an override-audit.jsonl entry we surface. An
// override is the repo's break-glass mechanism: a gate was deliberately bypassed.
type overrideRecord struct {
	Timestamp string `json:"timestamp"`
	Tool      string `json:"tool"`
	Gate      string `json:"gate"`
	Reason    string `json:"reason"`
	Allowed   bool   `json:"allowed"`
}

// breakGlassViolations parses an override-audit log and returns one violation per
// granted override (Allowed=true) within the window. Malformed lines are skipped.
func breakGlassViolations(data []byte, since time.Time) []Violation {
	cutoff := since.Format(time.RFC3339)
	var out []Violation
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var r overrideRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		if !r.Allowed || r.Timestamp < cutoff {
			continue
		}
		out = append(out, Violation{
			Repo:   "local",
			Type:   typeBreakGlass,
			Ref:    r.Timestamp,
			Detail: fmt.Sprintf("break-glass override granted for %s gate %q at %s: %s", r.Tool, r.Gate, r.Timestamp, r.Reason),
		})
	}
	return out
}

// ---- small pure helpers --------------------------------------------------

func firstLine(s string) string {
	before, _, _ := strings.Cut(s, "\n")
	return before
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// repoName returns the repo half of an "owner/repo" slug.
func repoName(slug string) string {
	if i := strings.LastIndexByte(slug, '/'); i >= 0 {
		return slug[i+1:]
	}
	return slug
}

func splitCSV(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// beadTitle is the deterministic bead title for a violation. Deterministic so
// re-runs do not file duplicate beads for the same violation.
func beadTitle(v Violation) string {
	if v.Ref != "" {
		return fmt.Sprintf("merge-audit: %s on %s %s", v.Type, v.Repo, v.Ref)
	}
	return fmt.Sprintf("merge-audit: %s on %s", v.Type, v.Repo)
}

// ---- reporting + side effects -------------------------------------------

func report(violations []Violation, opts options, stdout, stderr io.Writer, persist func([]Violation, options, io.Writer)) error {
	if err := writeReport(stdout, violations, opts.jsonOut); err != nil {
		return err
	}

	if len(violations) > 0 && !opts.dryRun {
		persist(violations, opts, stderr)
	}
	return nil
}

func writeReport(w io.Writer, violations []Violation, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(violations); err != nil {
			return err
		}
	} else {
		printTable(w, violations)
	}
	return nil
}

func persistViolations(violations []Violation, opts options, stderr io.Writer) {
	if err := appendHistory(violations); err != nil {
		fmt.Fprintf(stderr, "merge-audit: append history: %v\n", err)
	}
	for _, v := range violations {
		title := beadTitle(v)
		// Idempotency: weekly runs overlap their lookback windows, so skip
		// filing when an open bead already exists for this exact violation.
		if beadExists(opts.beadsDB, title) {
			continue
		}
		if err := fileBead(opts.beadsDB, v); err != nil {
			fmt.Fprintf(stderr, "merge-audit: file bead for %s: %v\n", title, err)
		}
	}
}

// beadExists reports whether an open/in-progress bead with exactly this title
// already exists. On any error it returns false so a transient bd failure does
// not silently suppress a violation bead.
func beadExists(db, title string) bool {
	// #nosec G204 — fixed "bd" binary; args passed as argv (no shell).
	cmd := exec.Command("bd", "--db", db, "list",
		"--title", title, "--status", "open,in_progress", "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	var beads []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out.Bytes(), &beads); err != nil {
		return false
	}
	for _, b := range beads {
		if b.Title == title {
			return true
		}
	}
	return false
}

func printTable(w io.Writer, violations []Violation) {
	if len(violations) == 0 {
		fmt.Fprintln(w, "merge-audit: no violations in the audit window ✓")
		return
	}
	fmt.Fprintf(w, "merge-audit: %d violation(s) detected\n\n", len(violations))
	fmt.Fprintf(w, "%-22s  %-24s  %-14s  %s\n", "TYPE", "REPO", "REF", "DETAIL")
	for _, v := range violations {
		fmt.Fprintf(w, "%-22s  %-24s  %-14s  %s\n", v.Type, v.Repo, v.Ref, v.Detail)
	}
}

// appendHistory appends one JSON line per violation to the audit-history log.
func appendHistory(violations []Violation) error {
	path := historyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 — fixed config path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, v := range violations {
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	return nil
}

// fileBead files a single P1 bug bead for a violation via the bd CLI.
func fileBead(db string, v Violation) error {
	// #nosec G204 — fixed "bd" binary; all args passed as argv (no shell).
	cmd := exec.Command("bd", "--db", db, "create",
		"--title", beadTitle(v),
		"--priority", "P1",
		"--type", "bug")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// ---- gh / git I/O --------------------------------------------------------

type mergedPR struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	MergedAt string `json:"mergedAt"`
	HeadSHA  string `json:"headRefOid"`
}

// mergedPRs lists PRs merged into branch on or after since.
func mergedPRs(ctx context.Context, repo, branch string, since time.Time) ([]mergedPR, error) {
	raw, err := ghJSON(ctx, "pr", "list",
		"--repo", repo, "--state", "merged", "--base", branch,
		"--limit", "100",
		"--json", "number,title,mergedAt,headRefOid")
	if err != nil {
		return nil, err
	}
	var all []mergedPR
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	cutoff := since.Format(time.RFC3339)
	var out []mergedPR
	for _, pr := range all {
		if pr.MergedAt >= cutoff {
			out = append(out, pr)
		}
	}
	return out, nil
}

// unresolvedThreadCount returns how many review threads were unresolved on a PR.
func unresolvedThreadCount(ctx context.Context, repo string, pr int) (int, error) {
	owner, name, ok := splitRepo(repo)
	if !ok {
		return 0, fmt.Errorf("repo %q is not owner/repo", repo)
	}
	const q = `query($owner:String!,$repo:String!,$pr:Int!){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$pr){
      reviewThreads(first:100){ nodes { isResolved } }
    }
  }
}`
	raw, err := ghJSON(ctx, "api", "graphql",
		"-f", "owner="+owner, "-f", "repo="+name,
		"-F", fmt.Sprintf("pr=%d", pr), "-f", "query="+q)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							IsResolved bool `json:"isResolved"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, fmt.Errorf("parse reviewThreads: %w", err)
	}
	n := 0
	for _, t := range resp.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if !t.IsResolved {
			n++
		}
	}
	return n, nil
}

// checkRuns fetches the check-runs for a commit SHA.
func checkRuns(ctx context.Context, repo, sha string) ([]checkRun, error) {
	raw, err := ghJSON(ctx, "api", fmt.Sprintf("repos/%s/commits/%s/check-runs", repo, sha),
		"--jq", ".check_runs // []")
	if err != nil {
		return nil, err
	}
	var runs []checkRun
	if err := json.Unmarshal(raw, &runs); err != nil {
		return nil, fmt.Errorf("parse check-runs: %w", err)
	}
	return runs, nil
}

type commit struct {
	SHA     string
	Message string
	Parents int
}

// branchCommits fetches commits on branch since the cutoff.
func branchCommits(ctx context.Context, repo, branch string, since time.Time) ([]commit, error) {
	raw, err := ghJSON(ctx, "api", "--paginate",
		fmt.Sprintf("repos/%s/commits?sha=%s&since=%s", repo, branch, since.Format(time.RFC3339)))
	if err != nil {
		return nil, err
	}
	var rawCommits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	if err := json.Unmarshal(raw, &rawCommits); err != nil {
		return nil, fmt.Errorf("parse commits: %w", err)
	}
	out := make([]commit, 0, len(rawCommits))
	for _, c := range rawCommits {
		out = append(out, commit{SHA: c.SHA, Message: c.Commit.Message, Parents: len(c.Parents)})
	}
	return out, nil
}

type rulesetSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// liveRuleset returns the single live ruleset matching the declared identity.
// Listing is exhaustive, and ambiguity is an audit failure. During dear-agent's
// one-time rename, the legacy and current names both resolve only when the
// selected object retains canonical ID 18061003.
func liveRuleset(ctx context.Context, repo, expectedName string) ([]byte, error) {
	pages, err := ghJSON(ctx, "api", "--paginate", "--slurp", fmt.Sprintf("repos/%s/rulesets?per_page=100", repo))
	if err != nil {
		return nil, err
	}
	rulesets, err := parseRulesetSummaryPages(pages)
	if err != nil {
		return nil, err
	}
	selected, err := selectExpectedRuleset(repo, expectedName, rulesets)
	if err != nil {
		return nil, err
	}
	return ghJSON(ctx, "api", fmt.Sprintf("repos/%s/rulesets/%d", repo, selected.ID))
}

func parseRulesetSummaryPages(raw []byte) ([]rulesetSummary, error) {
	var pages [][]rulesetSummary
	if err := json.Unmarshal(raw, &pages); err != nil {
		return nil, fmt.Errorf("parse paginated rulesets list: %w", err)
	}
	var rulesets []rulesetSummary
	for _, page := range pages {
		rulesets = append(rulesets, page...)
	}
	return rulesets, nil
}

// isDearAgentRepo reports whether repo is dear-agent itself, the only
// repository whose canonical ruleset declaration is checked in and therefore
// held to dear-agent-specific invariants (fixed ruleset ID, mandatory
// GitHub Actions integration ID on required checks). Other managed fleet
// repositories are inventory-owned and legitimately use context-only checks
// (see infra/variables.tf required_checks vs required_check_identities).
func isDearAgentRepo(repo string) bool {
	return strings.EqualFold(strings.TrimSpace(repo), "vbonnet/dear-agent")
}

func selectExpectedRuleset(repo, expectedName string, rulesets []rulesetSummary) (rulesetSummary, error) {
	isDearAgent := isDearAgentRepo(repo)
	var matches []rulesetSummary
	for _, ruleset := range rulesets {
		if isDearAgent {
			if ruleset.ID == dearAgentRulesetID || ruleset.Name == expectedName || ruleset.Name == legacyRulesetName {
				matches = append(matches, ruleset)
			}
			continue
		}
		if ruleset.Name == expectedName {
			matches = append(matches, ruleset)
		}
	}
	if len(matches) != 1 {
		return rulesetSummary{}, fmt.Errorf("expected exactly one ruleset matching %q on %s, found %d", expectedName, repo, len(matches))
	}
	selected := matches[0]
	if selected.ID <= 0 || strings.TrimSpace(selected.Name) == "" {
		return rulesetSummary{}, fmt.Errorf("matching ruleset on %s has incomplete identity", repo)
	}
	if isDearAgent {
		if selected.ID != dearAgentRulesetID {
			return rulesetSummary{}, fmt.Errorf("dear-agent ruleset %q has ID %d, expected %d", selected.Name, selected.ID, dearAgentRulesetID)
		}
		if selected.Name != expectedName && selected.Name != legacyRulesetName {
			return rulesetSummary{}, fmt.Errorf("dear-agent ruleset ID %d has unexpected name %q", selected.ID, selected.Name)
		}
	}
	return selected, nil
}

// defaultBranch returns the repo's default branch name.
func defaultBranch(ctx context.Context, repo string) (string, error) {
	raw, err := ghJSON(ctx, "repo", "view", repo, "--json", "defaultBranchRef")
	if err != nil {
		return "", err
	}
	var resp struct {
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse defaultBranchRef: %w", err)
	}
	if resp.DefaultBranchRef.Name == "" {
		return "main", nil
	}
	return resp.DefaultBranchRef.Name, nil
}

// discoverRepos finds owner/repo slugs for every git repo directly under dir.
func discoverRepos(ctx context.Context, dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var repos []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		slug, err := originSlug(ctx, path)
		if err != nil || slug == "" {
			continue
		}
		repos = append(repos, slug)
	}
	sort.Strings(repos)
	return repos, nil
}

// originSlug returns the owner/repo slug from a repo's origin remote, or "".
func originSlug(ctx context.Context, path string) (string, error) {
	// #nosec G204 — fixed "git" binary; path is a local directory argument.
	cmd := exec.CommandContext(ctx, "git", "-C", path, "remote", "get-url", "origin")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return parseRemoteSlug(strings.TrimSpace(out.String())), nil
}

// parseRemoteSlug extracts owner/repo from an https or ssh GitHub remote URL.
func parseRemoteSlug(url string) string {
	url = strings.TrimSuffix(url, ".git")
	if _, rest, found := strings.Cut(url, "github.com"); found {
		rest = strings.TrimLeft(rest, ":/")
		if strings.Count(rest, "/") == 1 {
			return rest
		}
	}
	return ""
}

func splitRepo(repo string) (owner, name string, ok bool) {
	i := strings.IndexByte(repo, '/')
	if i <= 0 || i == len(repo)-1 {
		return "", "", false
	}
	return repo[:i], repo[i+1:], true
}

// ghJSON runs `gh <args...>` and returns stdout, folding stderr into errors.
func ghJSON(ctx context.Context, args ...string) ([]byte, error) {
	// #nosec G204 — fixed "gh" binary; args passed as argv (no shell), so
	// repo/PR values cannot inject commands.
	cmd := exec.CommandContext(ctx, "gh", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh %s: %w: %s", args[0], err, msg)
		}
		return nil, fmt.Errorf("gh %s: %w", args[0], err)
	}
	return out.Bytes(), nil
}

// ---- path helpers --------------------------------------------------------

func defaultSrcDir() string  { return filepath.Join(homeDir(), "src") }
func defaultBeadsDB() string { return filepath.Join(homeDir(), "beads", "context-engine", ".beads") }
func historyPath() string {
	return filepath.Join(homeDir(), ".config", "dear-agent", "merge-audit-history.jsonl")
}
func overrideLogPath() string {
	return filepath.Join(homeDir(), ".local", "state", "dear-agent", "override-audit.jsonl")
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
