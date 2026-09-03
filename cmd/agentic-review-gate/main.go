// Command agentic-review-gate decides whether a pull request's agentic review
// lifecycle permits a merge, reading only labels.
//
// It is the merge-blocking half of the per-family review gate. Each reviewer
// family publishes its own lifecycle as labels — agentic-review:<family>:
// started before its model is invoked, then posted, then a verdict — and this
// command collapses those labels into one decision using the quorum in
// .github/agentic-review.yml.
//
// Nothing here calls a model. That is the point: the reviewers are the
// expensive, quota-limited, occasionally-down part, so a merge must never wait
// on one of them to answer a second time. The gate reads three cheap GitHub
// endpoints and a policy file, which means a quota incident degrades the review
// itself while leaving the ability to decide whether a review happened intact.
//
// Usage:
//
//	agentic-review-gate --repo owner/name --pr 123     # evaluate a live PR
//	agentic-review-gate --input-file fixture.json      # evaluate recorded state
//
// Exit 0 = the gate permits a merge, 1 = the reviewers decided against it,
// 3 = the lifecycle has not resolved yet, 2 = usage or internal error. A usage
// error is a block: an unreadable policy never becomes a pass.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
)

// Exit codes. Anything other than exitPass leaves the required check red.
//
// Pending is distinct from blocked so a caller can tell "no verdict yet" from
// "the reviewers decided against this". The gate job waits on the first and
// stops immediately on the second; collapsing them would mean either waiting
// out a deadline on a decided rejection or giving up on a review still in
// flight.
const (
	exitPass    = 0
	exitBlocked = 1
	exitUsage   = 2
	exitPending = 3
)

// ghRunner runs a gh invocation and returns its stdout. It is injected so the
// fetch path is testable without a network or a token.
type ghRunner func(args []string) (string, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agentic-review-gate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		configPath = fs.String("config", agenticreview.DefaultConfigPath, "path to the agentic review policy file")
		repo       = fs.String("repo", "", "owner/name of the repository holding the pull request")
		prNumber   = fs.Int("pr", 0, "pull request number to evaluate")
		inputFile  = fs.String("input-file", "", "evaluate recorded label state from a JSON file instead of GitHub")
		quorum     = fs.String("quorum", "", "override the configured approval quorum")
		asJSON     = fs.Bool("json", false, "emit the verdict as JSON instead of a text summary")
	)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	if err := checkSources(*inputFile, *repo, *prNumber); err != nil {
		fmt.Fprintln(stderr, "agentic-review-gate:", err)
		return exitUsage
	}
	cfg, err := resolveConfig(*configPath, *quorum)
	if err != nil {
		fmt.Fprintln(stderr, "agentic-review-gate:", err)
		return exitUsage
	}

	in, err := loadInput(*inputFile, *repo, *prNumber)
	if err != nil {
		fmt.Fprintln(stderr, "agentic-review-gate:", err)
		return exitUsage
	}

	verdict, err := cfg.Evaluate(in)
	if err != nil {
		fmt.Fprintln(stderr, "agentic-review-gate:", err)
		return exitUsage
	}

	if err := emit(stdout, verdict, *asJSON); err != nil {
		fmt.Fprintln(stderr, "agentic-review-gate:", err)
		return exitUsage
	}

	switch verdict.Decision {
	case agenticreview.DecisionPass:
		return exitPass
	case agenticreview.DecisionPending:
		return exitPending
	case agenticreview.DecisionBlock:
		return exitBlocked
	default:
		// An unrecognized decision is not a pass. The gate is the last thing
		// between an unreviewed head and main.
		fmt.Fprintln(stderr, "agentic-review-gate: unrecognized decision", verdict.Decision)
		return exitBlocked
	}
}

// resolveConfig loads the policy and applies any command-line quorum override.
func resolveConfig(configPath, quorum string) (agenticreview.Config, error) {
	cfg, err := agenticreview.LoadConfig(configPath)
	if err != nil {
		return agenticreview.Config{}, err
	}
	if quorum == "" {
		return cfg, nil
	}
	if cfg.Quorum, err = parseQuorum(quorum); err != nil {
		return agenticreview.Config{}, err
	}
	return cfg, nil
}

func checkSources(inputFile, repo string, prNumber int) error {
	live := repo != "" || prNumber != 0
	switch {
	case inputFile == "" && !live:
		return errors.New("need either --input-file or both --repo and --pr")
	case inputFile != "" && live:
		return errors.New("--input-file and --repo/--pr are alternatives; pass one")
	case live && (repo == "" || prNumber <= 0):
		return errors.New("--repo and --pr must be given together")
	}
	return nil
}

func parseQuorum(raw string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		return 0, fmt.Errorf("--quorum %q is not a number", raw)
	}
	return n, nil
}

func loadInput(inputFile, repo string, prNumber int) (agenticreview.Input, error) {
	if inputFile != "" {
		raw, err := os.ReadFile(inputFile)
		if err != nil {
			return agenticreview.Input{}, fmt.Errorf("read input file: %w", err)
		}
		var in agenticreview.Input
		if err := json.Unmarshal(raw, &in); err != nil {
			return agenticreview.Input{}, fmt.Errorf("parse %s: %w", inputFile, err)
		}
		if in.Now.IsZero() {
			in.Now = time.Now().UTC()
		}
		return in, nil
	}
	return fetchInput(execGH, repo, prNumber)
}

// prView is the subset of gh pull request JSON the gate reads.
type prView struct {
	Number     int       `json:"number"`
	HeadRefOID string    `json:"headRefOid"`
	CreatedAt  time.Time `json:"createdAt"`
	IsDraft    bool      `json:"isDraft"`
	Labels     []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// fetchInput assembles the gate's input from three cheap GitHub reads: the
// pull request's current labels, the timeline that says when each was applied,
// and the head commit's date.
func fetchInput(gh ghRunner, repo string, number int) (agenticreview.Input, error) {
	var pr prView
	if err := ghJSON(gh, &pr, "pr", "view", fmt.Sprint(number), "--repo", repo,
		"--json", "number,headRefOid,createdAt,isDraft,labels"); err != nil {
		return agenticreview.Input{}, fmt.Errorf("read pull request: %w", err)
	}

	var events []agenticreview.TimelineEvent
	if err := ghJSON(gh, &events, "api", "--paginate",
		fmt.Sprintf("repos/%s/issues/%d/timeline", repo, number)); err != nil {
		return agenticreview.Input{}, fmt.Errorf("read pull request timeline: %w", err)
	}

	var head struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := ghJSON(gh, &head, "api", fmt.Sprintf("repos/%s/commits/%s", repo, pr.HeadRefOID)); err != nil {
		return agenticreview.Input{}, fmt.Errorf("read head commit: %w", err)
	}

	live := make([]string, 0, len(pr.Labels))
	for _, l := range pr.Labels {
		live = append(live, l.Name)
	}
	appliedAt, readyAt := agenticreview.Clock(events, agenticreview.Head{
		Labels:      live,
		CreatedAt:   pr.CreatedAt,
		CommittedAt: head.Commit.Committer.Date,
		IsDraft:     pr.IsDraft,
	})
	return agenticreview.Input{
		Labels:    live,
		AppliedAt: appliedAt,
		ReadyAt:   readyAt,
		Now:       time.Now().UTC(),
	}, nil
}

func ghJSON(gh ghRunner, into any, args ...string) error {
	out, err := gh(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("gh %s returned no output", strings.Join(args, " "))
	}
	return json.Unmarshal([]byte(out), into)
}

func execGH(args []string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func emit(w io.Writer, v agenticreview.Verdict, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "agentic review gate: %s\n%s\n\n", v.Decision, v.Reason)
	for _, fv := range v.Families {
		fmt.Fprintf(&b, "  %-8s %-9s %s\n", fv.Family, fv.State, fv.Reason)
	}
	fmt.Fprintf(&b, "\n%d approved, %d down, quorum %d\n", v.Approved, v.Down, v.Quorum)
	for _, f := range v.Unconfigured {
		fmt.Fprintf(&b, "warning: %q published review labels but is not a configured family\n", f)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
