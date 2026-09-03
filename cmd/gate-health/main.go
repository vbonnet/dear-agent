// Command gate-health reports whether one required check is failing across a
// large share of the open pull-request queue, which means the merge pipeline is
// blocked by a single repo-wide cause rather than ordinary per-PR churn.
//
// It is a sibling of cmd/jaeger-health and cmd/merge-health and copies their
// contract on purpose: exit 0 healthy / 1 degraded / 2 down / 3 usage, --json,
// no side effects. That shared shape is what lets cmd/absence-alarm register it
// as a command pulse, schedule it, and escalate a non-zero exit to the desktop
// without any gate-specific logic.
//
// Why this probe exists alongside merge-health. On 2026-09-03 two x/crypto
// advisories failed the required govulncheck gate on every branch cut from
// main. Merges stopped for nine hours and nobody was told. Main itself stayed
// green the whole time, so the scheduled CI Health Monitor, which audits the
// last five CI runs on main, correctly reported success three times across the
// blackout. merge-health would have caught the silence, but only as "nothing
// merged" - a symptom shared by a holiday, a quiet night, and a dead fetch
// loop. This probe names the cause instead: which check, how much of the queue
// it holds, and the likely single fix. That is the difference between knowing
// something is wrong and knowing what to do, and it is where most of the nine
// hours went.
//
// The probe is read-only by design. It never reruns a check, rebases a branch,
// or opens a remediation PR. Detection and remediation are split so a false
// positive costs a notification rather than a fleet of unwanted pull requests;
// the report carries a remediation_kind so a separate driver can decide what is
// safe to act on.
//
// Usage:
//
//	gate-health [--repo owner/name] [--min-fraction 0.30] [--min-prs 5] [--limit 100] [--json]
//
// Drafts are counted by default. See gatehealth.Config.ExcludeDrafts for why:
// excluding them read healthy straight through the outage that motivated this.
//
// Exit codes:
//
//	0  healthy  — no single check dominates the queue's failures
//	1  degraded — a systemic gate failure is present
//	2  down     — the queue could not be read, or nothing was evaluable
//	3  usage    — bad flags
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/gatehealth"
)

// Exit codes, shared with the other absence probes (PULSE-01..PULSE-04).
const (
	exitHealthy  = 0
	exitDegraded = 1
	exitDown     = 2
	exitUsage    = 3
)

// Report is the machine-readable output emitted with --json. It embeds the
// domain verdict so the JSON shape and the detection rule cannot drift apart.
type Report struct {
	CheckedAt string `json:"checked_at"`
	Repo      string `json:"repo"`
	gatehealth.Report
	MinFraction float64 `json:"min_fraction"`
	MinPRs      int     `json:"min_prs"`
	Error       string  `json:"error,omitempty"`
}

// queuePR is one open pull request as read from the forge.
type queuePR struct {
	Number        int
	FailingChecks []string
	Draft         bool
	ChecksUnknown bool
}

// queryFunc collects the open pull-request queue. Injected so tests exercise
// the exit contract without a network.
type queryFunc func(ctx context.Context, repo string, limit int) ([]queuePR, error)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, ghQuery)) }

func run(args []string, stdout, stderr io.Writer, query queryFunc) int {
	fs := flag.NewFlagSet("gate-health", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "vbonnet/dear-agent", "repository in owner/name form")
	minFraction := fs.Float64("min-fraction", gatehealth.DefaultConfig().MinFraction,
		"share of evaluated open PRs a check must fail on to count as systemic")
	minPRs := fs.Int("min-prs", gatehealth.DefaultConfig().MinPRs,
		"absolute number of open PRs a check must fail on to count as systemic")
	limit := fs.Int("limit", 100, "maximum open pull requests to inspect")
	excludeDrafts := fs.Bool("exclude-drafts", false,
		"drop draft pull requests from the sample (drafts are counted by default: draft status describes review intent, not gate health)")
	timeout := fs.Duration("timeout", 120*time.Second, "overall timeout for reading the queue")
	asJSON := fs.Bool("json", false, "emit a JSON report to stdout instead of a human summary")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg := gatehealth.Config{MinFraction: *minFraction, MinPRs: *minPRs, ExcludeDrafts: *excludeDrafts}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if *limit < 1 {
		fmt.Fprintf(stderr, "gate-health: --limit must be at least 1, got %d\n", *limit)
		return exitUsage
	}

	report := Report{
		CheckedAt:   time.Now().UTC().Format(time.RFC3339),
		Repo:        *repo,
		MinFraction: cfg.MinFraction,
		MinPRs:      cfg.MinPRs,
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	prs, err := query(ctx, *repo, *limit)
	if err != nil {
		report.Status = "down"
		report.Error = err.Error()
		return emit(report, *asJSON, stdout,
			fmt.Sprintf("gate-health: DOWN: cannot read the open PR queue for %s: %v", *repo, err),
			exitDown)
	}

	domain := make([]gatehealth.PullRequest, 0, len(prs))
	for _, p := range prs {
		domain = append(domain, gatehealth.PullRequest{
			Number:        p.Number,
			FailingChecks: p.FailingChecks,
			Draft:         p.Draft,
			ChecksUnknown: p.ChecksUnknown,
		})
	}

	report.Report = gatehealth.Detect(domain, cfg)

	switch report.Status {
	case gatehealth.StatusNoQueue:
		// Deliberately not exit 0. An empty queue is the absence of evidence,
		// and a probe that reports health when it saw nothing is the silent
		// monitor this tool replaces.
		report.Error = "no evaluable open pull requests"
		return emit(report, *asJSON, stdout,
			fmt.Sprintf("gate-health: DOWN: %s has no evaluable open pull requests (%d skipped as drafts or unreported)",
				*repo, report.SkippedPRs),
			exitDown)
	case gatehealth.StatusSystemic:
		return emit(report, *asJSON, stdout, systemicSummary(report), exitDegraded)
	case gatehealth.StatusHealthy:
		return emit(report, *asJSON, stdout,
			fmt.Sprintf("gate-health: HEALTHY: no check fails on more than %.0f%% of %d evaluated open PRs (%d have some failure)",
				cfg.MinFraction*100, report.EvaluatedPRs, report.PRsWithFailures),
			exitHealthy)
	default:
		// An unrecognised status is a bug in the domain, not health. Report it
		// as down so a new status can never silently read as passing.
		report.Error = fmt.Sprintf("unrecognised status %q", report.Status)
		return emit(report, *asJSON, stdout,
			fmt.Sprintf("gate-health: DOWN: unrecognised status %q", report.Status), exitDown)
	}
}

// systemicSummary is the human body, and also what a desktop notification
// shows. It leads with the verdict and the fix because that is what a
// responder acts on; the supporting numbers follow.
func systemicSummary(r Report) string {
	var b strings.Builder
	d := r.Dominant
	fmt.Fprintf(&b, "gate-health: SYSTEMIC GATE FAILURE in %s\n", r.Repo)
	fmt.Fprintf(&b, "  check:  %s\n", d.Check)
	fmt.Fprintf(&b, "  scope:  %d of %d evaluated open PRs (%.1f%%)\n", d.PRCount, r.EvaluatedPRs, d.Fraction*100)
	fmt.Fprintf(&b, "  likely fix (%s): %s\n", r.RemediationKind, r.Remediation)
	fmt.Fprintf(&b, "  example PRs: %s\n", formatPRs(d.PRs))
	if len(r.Systemic) > 1 {
		b.WriteString("  also over threshold:\n")
		for _, c := range r.Systemic[1:] {
			fmt.Fprintf(&b, "    %s (%d PRs, %.1f%%)\n", c.Check, c.PRCount, c.Fraction*100)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatPRs shows at most five numbers; a responder needs one reproducer, not
// the whole list, and the full set is in the JSON report.
func formatPRs(nums []int) string {
	const show = 5
	parts := make([]string, 0, show)
	for i, n := range nums {
		if i == show {
			parts = append(parts, fmt.Sprintf("and %d more", len(nums)-show))
			break
		}
		parts = append(parts, fmt.Sprintf("#%d", n))
	}
	return strings.Join(parts, " ")
}

func emit(r Report, asJSON bool, stdout io.Writer, summary string, code int) int {
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return exitDown
		}
		return code
	}
	fmt.Fprintln(stdout, summary)
	return code
}

// rollupQuery asks for each open PR's head-commit check rollup in one page.
// Only failing contexts matter, but GitHub has no server-side filter for
// conclusion, so the filtering happens here.
const rollupQuery = `query($owner:String!, $name:String!, $limit:Int!) {
  repository(owner:$owner, name:$name) {
    pullRequests(states:OPEN, first:$limit, orderBy:{field:UPDATED_AT, direction:DESC}) {
      nodes {
        number
        isDraft
        commits(last:1) { nodes { commit { statusCheckRollup { contexts(last:100) { nodes {
          __typename
          ... on CheckRun { name conclusion }
          ... on StatusContext { context state }
        } } } } } }
      }
    }
  }
}`

// ghQuery reads the queue through the gh CLI, which already holds the host's
// GitHub credentials. Shelling out rather than importing an API client keeps
// this probe free of an auth story of its own.
func ghQuery(ctx context.Context, repo string, limit int) ([]queuePR, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return nil, fmt.Errorf("repo must be owner/name, got %q", repo)
	}

	cmd := exec.CommandContext(ctx, "gh", "api", "graphql",
		"-f", "query="+rollupQuery,
		"-F", "owner="+owner,
		"-F", "name="+name,
		"-F", fmt.Sprintf("limit=%d", limit),
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("gh api graphql: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	return parseRollup(out)
}

// rollupResponse mirrors only the fields this probe reads.
type rollupResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number  int  `json:"number"`
					IsDraft bool `json:"isDraft"`
					Commits struct {
						Nodes []struct {
							Commit struct {
								StatusCheckRollup *struct {
									Contexts struct {
										Nodes []struct {
											Name       string `json:"name"`
											Conclusion string `json:"conclusion"`
											Context    string `json:"context"`
											State      string `json:"state"`
										} `json:"nodes"`
									} `json:"contexts"`
								} `json:"statusCheckRollup"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

func parseRollup(raw []byte) ([]queuePR, error) {
	var resp rollupResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode rollup: %w", err)
	}

	nodes := resp.Data.Repository.PullRequests.Nodes
	prs := make([]queuePR, 0, len(nodes))
	for _, n := range nodes {
		pr := queuePR{Number: n.Number, Draft: n.IsDraft}
		if len(n.Commits.Nodes) == 0 || n.Commits.Nodes[0].Commit.StatusCheckRollup == nil {
			// No rollup means the checks have not reported. Excluded from the
			// denominator rather than counted as passing.
			pr.ChecksUnknown = true
			prs = append(prs, pr)
			continue
		}
		for _, c := range n.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
			name, state := c.Name, c.Conclusion
			if name == "" {
				name, state = c.Context, c.State
			}
			if state == "FAILURE" || state == "ERROR" {
				pr.FailingChecks = append(pr.FailingChecks, name)
			}
		}
		prs = append(prs, pr)
	}
	return prs, nil
}
