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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// defaultLookbackDays is the audit window when --days is not given. Matches the
// weekly cadence and the bypassed-merge-audit lookback.
const defaultLookbackDays = 7

// rulesetName is the name of the zero-bypass ruleset tracked in
// .github/rulesets/main.json; live rulesets are matched against it by name.
const rulesetName = "main-zero-bypass"

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
	fs := flag.NewFlagSet("merge-audit", flag.ContinueOnError)
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
		return 2
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
		return fail("--days must be a positive integer, got %d", opts.days)
	}

	ctx := context.Background()
	repos := opts.repos
	if len(repos) == 0 {
		discovered, err := discoverRepos(ctx, opts.src)
		if err != nil {
			return fail("discover repos under %s: %v", opts.src, err)
		}
		repos = discovered
	}
	if len(repos) == 0 {
		return fail("no repos to audit (use --repos or --src)")
	}

	since := opts.now.AddDate(0, 0, -opts.days)
	var violations []Violation
	for _, repo := range repos {
		vs, err := auditRepo(ctx, repo, since, opts.rulesetPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "merge-audit: %s: %v\n", repo, err)
			continue
		}
		violations = append(violations, vs...)
	}

	// Check 4: break-glass overrides are recorded locally (not per-repo), so the
	// override-audit log is scanned once across the whole window.
	if data, err := os.ReadFile(overrideLogPath()); err == nil {
		violations = append(violations, breakGlassViolations(data, since)...)
	}

	stamp := opts.now.Format(time.RFC3339)
	for i := range violations {
		violations[i].DetectedAt = stamp
	}

	if err := report(violations, opts); err != nil {
		return fail("%v", err)
	}
	return 0
}

// auditRepo runs all five checks against one repo and returns its violations.
func auditRepo(ctx context.Context, repo string, since time.Time, rulesetPath string) ([]Violation, error) {
	branch, err := defaultBranch(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("default branch: %w", err)
	}

	var out []Violation
	prVs, err := auditMergedPRs(ctx, repo, branch, since)
	if err != nil {
		return nil, err
	}
	out = append(out, prVs...)

	pushVs, err := auditDirectPushes(ctx, repo, branch, since)
	if err != nil {
		return nil, err
	}
	out = append(out, pushVs...)

	out = append(out, auditRulesetDrift(ctx, repo, rulesetPath)...)
	return out, nil
}

// auditMergedPRs runs checks 1 (unresolved threads) and 2 (incomplete checks)
// for every PR merged into branch in the window.
func auditMergedPRs(ctx context.Context, repo, branch string, since time.Time) ([]Violation, error) {
	prs, err := mergedPRs(ctx, repo, branch, since)
	if err != nil {
		return nil, fmt.Errorf("merged PRs: %w", err)
	}
	var out []Violation
	for _, pr := range prs {
		threads, err := unresolvedThreadCount(ctx, repo, pr.Number)
		if err != nil {
			return nil, fmt.Errorf("PR #%d threads: %w", pr.Number, err)
		}
		if threads > 0 {
			out = append(out, Violation{
				Repo:   repo,
				Type:   typeUnresolvedThreads,
				Ref:    fmt.Sprintf("#%d", pr.Number),
				Detail: fmt.Sprintf("PR #%d (%s) merged with %d unresolved review thread(s)", pr.Number, pr.Title, threads),
			})
		}

		runs, err := checkRuns(ctx, repo, pr.HeadSHA)
		if err != nil {
			return nil, fmt.Errorf("PR #%d checks: %w", pr.Number, err)
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

// auditRulesetDrift runs check 5: live branch ruleset vs the checked-in source.
// Missing local source or an absent live ruleset is treated as "nothing to
// compare" (no violation), not an error.
func auditRulesetDrift(ctx context.Context, repo, rulesetPath string) []Violation {
	localPath := rulesetPath
	if localPath == "" {
		localPath = filepath.Join("/Users", currentUser(), "src", repoName(repo), ".github", "rulesets", "main.json")
	}
	local, err := os.ReadFile(localPath) // #nosec G304 — operator-supplied audit input path
	if err != nil {
		return nil
	}
	live, err := liveRuleset(ctx, repo)
	if err != nil || live == nil {
		return nil
	}
	var out []Violation
	for _, d := range rulesetDrift(local, live) {
		out = append(out, Violation{Repo: repo, Type: typeRulesetDrift, Detail: d})
	}
	return out
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

// rulesetView is the normalized projection of a ruleset we compare for drift.
// Comparing a projection (not the raw JSON) avoids false positives from
// server-only fields (id, created_at, _links, node_id, source).
type rulesetView struct {
	Enforcement   string
	BypassActors  int
	RuleTypes     []string
	RequiredCheck []string
}

// rulesetDrift returns a list of human-readable drift descriptions between the
// local source-of-truth ruleset and the live ruleset. Empty slice means in sync.
func rulesetDrift(localJSON, liveJSON []byte) []string {
	local, err1 := parseRuleset(localJSON)
	live, err2 := parseRuleset(liveJSON)
	if err1 != nil || err2 != nil {
		return []string{"ruleset drift: unable to parse local or live ruleset for comparison"}
	}
	var drift []string
	if local.Enforcement != live.Enforcement {
		drift = append(drift, fmt.Sprintf("ruleset enforcement drift: source=%q live=%q", local.Enforcement, live.Enforcement))
	}
	if local.BypassActors != live.BypassActors {
		drift = append(drift, fmt.Sprintf("ruleset bypass_actors drift: source=%d live=%d", local.BypassActors, live.BypassActors))
	}
	if d := diffSet("rule types", local.RuleTypes, live.RuleTypes); d != "" {
		drift = append(drift, d)
	}
	if d := diffSet("required status checks", local.RequiredCheck, live.RequiredCheck); d != "" {
		drift = append(drift, d)
	}
	return drift
}

// parseRuleset normalizes a ruleset JSON document into a rulesetView.
func parseRuleset(raw []byte) (rulesetView, error) {
	var doc struct {
		Enforcement  string            `json:"enforcement"`
		BypassActors []json.RawMessage `json:"bypass_actors"`
		Rules        []struct {
			Type       string `json:"type"`
			Parameters struct {
				RequiredStatusChecks []struct {
					Context string `json:"context"`
				} `json:"required_status_checks"`
			} `json:"parameters"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return rulesetView{}, err
	}
	v := rulesetView{Enforcement: doc.Enforcement, BypassActors: len(doc.BypassActors)}
	for _, r := range doc.Rules {
		v.RuleTypes = append(v.RuleTypes, r.Type)
		for _, c := range r.Parameters.RequiredStatusChecks {
			v.RequiredCheck = append(v.RequiredCheck, c.Context)
		}
	}
	sort.Strings(v.RuleTypes)
	sort.Strings(v.RequiredCheck)
	return v, nil
}

// diffSet returns a drift description if the two string sets differ, else "".
func diffSet(label string, a, b []string) string {
	missing := setDiff(a, b) // in source, not live
	extra := setDiff(b, a)   // in live, not source
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing from live: "+strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		parts = append(parts, "unexpected in live: "+strings.Join(extra, ", "))
	}
	return fmt.Sprintf("ruleset %s drift: %s", label, strings.Join(parts, "; "))
}

// setDiff returns elements present in a but absent from b.
func setDiff(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
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

func report(violations []Violation, opts options) error {
	if opts.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(violations); err != nil {
			return err
		}
	} else {
		printTable(violations)
	}

	if len(violations) == 0 || opts.dryRun {
		return nil
	}
	if err := appendHistory(violations); err != nil {
		fmt.Fprintf(os.Stderr, "merge-audit: append history: %v\n", err)
	}
	for _, v := range violations {
		title := beadTitle(v)
		// Idempotency: weekly runs overlap their lookback windows, so skip
		// filing when an open bead already exists for this exact violation.
		if beadExists(opts.beadsDB, title) {
			continue
		}
		if err := fileBead(opts.beadsDB, v); err != nil {
			fmt.Fprintf(os.Stderr, "merge-audit: file bead for %s: %v\n", title, err)
		}
	}
	return nil
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

func printTable(violations []Violation) {
	if len(violations) == 0 {
		fmt.Println("merge-audit: no violations in the audit window ✓")
		return
	}
	fmt.Printf("merge-audit: %d violation(s) detected\n\n", len(violations))
	fmt.Printf("%-22s  %-24s  %-14s  %s\n", "TYPE", "REPO", "REF", "DETAIL")
	for _, v := range violations {
		fmt.Printf("%-22s  %-24s  %-14s  %s\n", v.Type, v.Repo, v.Ref, v.Detail)
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

// liveRuleset returns the live JSON of the named ruleset, or nil if absent.
func liveRuleset(ctx context.Context, repo string) ([]byte, error) {
	list, err := ghJSON(ctx, "api", fmt.Sprintf("repos/%s/rulesets", repo))
	if err != nil {
		return nil, err
	}
	var rulesets []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(list, &rulesets); err != nil {
		return nil, fmt.Errorf("parse rulesets list: %w", err)
	}
	id := 0
	for _, r := range rulesets {
		if r.Name == rulesetName {
			id = r.ID
			break
		}
	}
	if id == 0 {
		return nil, nil
	}
	return ghJSON(ctx, "api", fmt.Sprintf("repos/%s/rulesets/%d", repo, id))
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

func currentUser() string {
	h := homeDir()
	return filepath.Base(h)
}

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "merge-audit: "+format+"\n", a...)
	return 1
}
