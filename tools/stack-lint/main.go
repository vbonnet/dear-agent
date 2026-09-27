// Command stack-lint refutes a pull request that presents itself as part of a
// stack but is not wired as one.
//
// Usage:
//
//	stack-lint -pr <number> [-repo <dir>] [-slug <owner/name>] [-strict] [-json]
//
// It reads the pull request and the repository's open head branches from
// GitHub, decides ancestry with git, and applies the rules in
// internal/stackguard. A blocking finding exits 1; a usage or operational
// failure exits 2.
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
	"os/signal"
	"strings"
	"syscall"

	"github.com/vbonnet/dear-agent/internal/stackguard"
	"time"
)

const (
	exitOK        = 0
	exitViolation = 1
	exitUsage     = 2
)

func main() {
	os.Exit(mainExitCode())
}

func mainExitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// runTimeout bounds the whole lint, covering both the gh query and every git
// subprocess it drives.
const runTimeout = time.Minute

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	// Bound every gh and git call this command makes. Without it a stalled
	// network call or a git subprocess waiting on a prompt hangs the lint
	// indefinitely, and this runs inside PR tooling that has to terminate.
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	flags := flag.NewFlagSet("stack-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var number int
	var repoDir, slug string
	var strict, asJSON bool
	flags.IntVar(&number, "pr", 0, "pull request number to check")
	flags.StringVar(&repoDir, "repo", "", "repository directory (defaults to the working directory)")
	flags.StringVar(&slug, "slug", "", "owner/name of the repository (defaults to the checkout's remote)")
	flags.BoolVar(&strict, "strict", false, "promote advisory findings to blocking")
	flags.BoolVar(&asJSON, "json", false, "emit findings as JSON")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if number <= 0 {
		fmt.Fprintln(stderr, "stack-lint: -pr is required and must be positive")
		flags.Usage()
		return exitUsage
	}

	pr, trunk, err := fetchPullRequest(ctx, repoDir, slug, number)
	if err != nil {
		fmt.Fprintf(stderr, "stack-lint: %v\n", err)
		return exitUsage
	}
	heads, bases, err := fetchOpenRefs(ctx, repoDir, slug)
	if err != nil {
		fmt.Fprintf(stderr, "stack-lint: %v\n", err)
		return exitUsage
	}

	findings := stackguard.Check(pr, stackguard.Repository{
		Trunk:     trunk,
		OpenHeads: heads,
		OpenBases: bases,
		Descends:  gitDescends(ctx, repoDir),
		Strict:    strict,
	})
	return report(stdout, stderr, number, findings, asJSON)
}

// report renders the findings and returns the process exit code.
func report(stdout, stderr io.Writer, number int, findings []stackguard.Finding, asJSON bool) int {
	switch {
	case asJSON:
		payload := struct {
			PullRequest int                  `json:"pull_request"`
			Blocking    bool                 `json:"blocking"`
			Findings    []stackguard.Finding `json:"findings"`
		}{number, stackguard.Blocking(findings), findings}
		if findings == nil {
			payload.Findings = []stackguard.Finding{}
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			// The caller never received the report, so this must not read as
			// a clean pass. Fail closed with the violation code.
			fmt.Fprintf(stderr, "stack-lint: encoding report: %v\n", err)
			return exitViolation
		}
	case len(findings) == 0:
		fmt.Fprintf(stdout, "stack-lint: #%d is consistent with how it describes itself\n", number)
	default:
		for _, finding := range findings {
			severity := "advisory"
			if finding.Blocking {
				severity = "blocking"
			}
			fmt.Fprintf(stdout, "%s %s: %s\n  fix: %s\n", finding.Code, severity, finding.Detail, finding.Remedy)
		}
	}
	if stackguard.Blocking(findings) {
		return exitViolation
	}
	return exitOK
}

const pullRequestQuery = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){
    defaultBranchRef{name}
    pullRequest(number:$number){
      number title body baseRefName headRefName
      stackEntry{position}
      stack{number size entries(first:50){nodes{position pullRequest{number state}}}}
    }
  }
}`

// fetchPullRequest reads the pull request and the repository's default branch.
// The stack fields are a preview surface, so a query that fails only on those
// is retried without them and the pull request is marked registration-unread
// rather than reported as unregistered.
func fetchPullRequest(ctx context.Context, repoDir, slug string, number int) (stackguard.PullRequest, string, error) {
	owner, name, err := resolveSlug(ctx, repoDir, slug)
	if err != nil {
		return stackguard.PullRequest{}, "", err
	}
	raw, err := gh(ctx, repoDir, "api", "graphql",
		"-f", "query="+pullRequestQuery,
		"-F", "owner="+owner, "-F", "name="+name, "-F", fmt.Sprintf("number=%d", number))
	unread := false
	if err != nil {
		fallback := strings.NewReplacer(
			"stackEntry{position}", "",
			"stack{number size entries(first:50){nodes{position pullRequest{number state}}}}", "",
		).Replace(pullRequestQuery)
		raw, err = gh(ctx, repoDir, "api", "graphql",
			"-f", "query="+fallback,
			"-F", "owner="+owner, "-F", "name="+name, "-F", fmt.Sprintf("number=%d", number))
		if err != nil {
			return stackguard.PullRequest{}, "", fmt.Errorf("reading pull request #%d: %w", number, err)
		}
		unread = true
	}

	var payload struct {
		// A failed query returns errors with an empty data block. Without
		// reading them, node.Number stays 0 and the caller reports "not
		// found" for what is really a permission or query problem.
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Repository struct {
				DefaultBranchRef struct{ Name string } `json:"defaultBranchRef"`
				PullRequest      struct {
					Number      int
					Title       string
					Body        string
					BaseRefName string
					HeadRefName string
					StackEntry  *struct{ Position int } `json:"stackEntry"`
					Stack       *struct {
						Number, Size int
						Entries      struct {
							Nodes []struct {
								Position    int
								PullRequest struct {
									Number int
									State  string
								} `json:"pullRequest"`
							}
						} `json:"entries"`
					}
				} `json:"pullRequest"`
			}
		}
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return stackguard.PullRequest{}, "", fmt.Errorf("decoding pull request #%d: %w", number, err)
	}
	if err := graphQLError(payload.Errors, number); err != nil {
		return stackguard.PullRequest{}, "", err
	}
	node := payload.Data.Repository.PullRequest
	if node.Number == 0 {
		return stackguard.PullRequest{}, "", fmt.Errorf("pull request #%d was not found in %s/%s", number, owner, name)
	}
	pr := stackguard.PullRequest{
		Number:             node.Number,
		Title:              node.Title,
		Body:               node.Body,
		BaseRef:            node.BaseRefName,
		HeadRef:            node.HeadRefName,
		RegistrationUnread: unread,
	}
	if node.Stack != nil {
		pr.StackNumber, pr.StackSize = node.Stack.Number, node.Stack.Size
	}
	if node.StackEntry != nil {
		pr.StackPosition = node.StackEntry.Position
	}
	if node.Stack != nil && pr.StackPosition > 0 {
		for _, entry := range node.Stack.Entries.Nodes {
			if entry.Position < pr.StackPosition && entry.PullRequest.State == "OPEN" {
				pr.LowerEntriesOpen = true
				break
			}
		}
	}
	trunk := payload.Data.Repository.DefaultBranchRef.Name
	if trunk == "" {
		trunk = "main"
	}
	return pr, trunk, nil
}

// fetchOpenRefs returns every open pull request's head branch and, separately,
// every branch that has an open pull request targeting it. The second map is
// what distinguishes a genuine stack bottom from a lone claim on the trunk.
func fetchOpenRefs(ctx context.Context, repoDir, slug string) (heads, bases map[string]int, err error) {
	args := []string{"pr", "list", "--state", "open", "--limit", "200", "--json", "number,headRefName,baseRefName"}
	if slug != "" {
		args = append(args, "--repo", slug)
	}
	raw, err := gh(ctx, repoDir, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("listing open pull requests: %w", err)
	}
	var list []struct {
		Number      int
		HeadRefName string
		BaseRefName string
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, nil, fmt.Errorf("decoding open pull requests: %w", err)
	}
	heads = make(map[string]int, len(list))
	bases = make(map[string]int, len(list))
	for _, item := range list {
		heads[item.HeadRefName] = item.Number
		bases[item.BaseRefName] = item.Number
	}
	return heads, bases, nil
}

// gitDescends answers ancestry from the checkout's remote-tracking refs, which
// are what the pull request's branches actually resolve to.
func gitDescends(ctx context.Context, repoDir string) func(string, string) (bool, error) {
	return func(base, head string) (bool, error) {
		for _, ref := range []string{base, head} {
			if err := runGit(ctx, repoDir, "rev-parse", "--verify", "--quiet", "origin/"+ref+"^{commit}"); err != nil {
				return false, fmt.Errorf("origin/%s is not present in this checkout (fetch it first): %w", ref, err)
			}
		}
		err := runGit(ctx, repoDir, "merge-base", "--is-ancestor", "origin/"+base, "origin/"+head)
		if err == nil {
			return true, nil
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
}

func resolveSlug(ctx context.Context, repoDir, slug string) (string, string, error) {
	if slug == "" {
		raw, err := gh(ctx, repoDir, "repo", "view", "--json", "owner,name")
		if err != nil {
			return "", "", fmt.Errorf("resolving the repository: %w", err)
		}
		var view struct {
			Owner struct{ Login string }
			Name  string
		}
		if err := json.Unmarshal(raw, &view); err != nil {
			return "", "", fmt.Errorf("decoding the repository: %w", err)
		}
		return view.Owner.Login, view.Name, nil
	}
	owner, name, found := strings.Cut(slug, "/")
	if !found || owner == "" || name == "" {
		return "", "", fmt.Errorf("-slug %q is not in owner/name form", slug)
	}
	return owner, name, nil
}

func gh(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Dir = repoDir
	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// graphQLError turns a non-empty GraphQL errors array into one error. A failed
// query returns errors alongside an empty data block, so without this the
// caller reads number 0 and reports "not found" for what is really a
// permission or query problem.
func graphQLError(errs []struct {
	Message string `json:"message"`
}, number int) error {
	if len(errs) == 0 {
		return nil
	}
	messages := make([]string, 0, len(errs))
	for _, e := range errs {
		messages = append(messages, e.Message)
	}
	return fmt.Errorf("querying pull request #%d: %s", number, strings.Join(messages, "; "))
}

func runGit(ctx context.Context, repoDir string, args ...string) error {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = repoDir
	// Never block on a credential prompt: with no TTY the helper can wait
	// forever, which is the hang the context timeout would then have to
	// absorb rather than prevent.
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}
