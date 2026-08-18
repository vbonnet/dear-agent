// Command changed-paths owns the repo's CI path taxonomy.
//
// It answers "what kind of change is this PR?" once, in one place, and the
// workflows consume the answer as job-level `if:` conditions. Two modes:
//
//	changed-paths            classify the PR diff and write $GITHUB_OUTPUT
//	changed-paths -audit     CI Gateway: verify no relevant job was skipped
//
// Callers must NOT express this selection as workflow-level `on.<event>.paths`
// for anything that produces a required status check: a workflow dropped by a
// path filter never creates a check run, so the required context sits at
// "Expected — Waiting for status to be reported" forever. A job skipped by an
// `if:` condition does report, with conclusion `skipped`, which GitHub treats
// as satisfying a required check. That asymmetry is what makes this design
// possible and is also its sharp edge — see the gateway in audit.go.
//
// FAIL-SAFE CONTRACT: every output defaults to "true". Anything that makes the
// change set unknowable (no base ref, a git failure, a non-pull_request event)
// forces every consumer to run. Under-running is a silent hole in the gate;
// over-running only costs runner minutes.
//
// See ADR-038 and .github/workflows/README.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var (
		repo  = flag.String("repo", ".", "path to the repository checkout")
		audit = flag.Bool("audit", false, "run the CI Gateway skip audit instead of change detection")
	)
	flag.Parse()

	if *audit {
		if err := runAudit(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runDetect(os.Stdout, *repo); err != nil {
		// Detection never fails the job on a classification problem — it
		// degrades to "everything is relevant". Reaching here means the
		// output file itself is unwritable, which no consumer can compensate
		// for, so fail loudly and let the gateway see a failed detector.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDetect(out *os.File, repo string) error {
	sel := detect(out, repo)
	if sel.Reason != "" {
		fmt.Fprintf(out, "::notice ::changed-paths: forcing all consumers ON — %s\n", sel.Reason)
	}
	fmt.Fprintf(out, "Selection:\n%s", sel.Render())
	return writeOutputs(sel)
}

func detect(out *os.File, repo string) Selection {
	event := os.Getenv("EVENT_NAME")
	// Only pull_request events have a meaningful base to diff against. push,
	// schedule and workflow_dispatch always run everything: they are the
	// post-merge safety valve that catches whatever PR-time selection missed,
	// so narrowing them would defeat the entire design.
	if event != "pull_request" {
		return AllTrue(fmt.Sprintf("event %q is not a pull_request", event))
	}
	base, head := os.Getenv("BASE_SHA"), os.Getenv("HEAD_SHA")
	if base == "" || head == "" {
		return AllTrue("missing base or head SHA on the event payload")
	}

	files, err := ChangedFiles(repo, base, head)
	if err != nil {
		return AllTrue("git diff failed: " + err.Error())
	}
	fmt.Fprintf(out, "Changed files (%d):\n", len(files))
	for _, f := range files {
		fmt.Fprintf(out, "  %s\n", f)
	}

	roots, err := DiscoverEmbedRoots(repo)
	if err != nil {
		fmt.Fprintf(out, "::warning ::changed-paths: partial //go:embed scan: %v\n", err)
	}
	fmt.Fprintf(out, "Discovered %d //go:embed root(s).\n", len(roots))

	return (&Classifier{EmbedRoots: roots}).Classify(files)
}

func writeOutputs(sel Selection) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	var b strings.Builder
	for _, k := range Keys {
		fmt.Fprintf(&b, "%s=%t\n", k, sel.Values[k])
	}
	// $GITHUB_OUTPUT is set by the Actions runner to a file inside its own
	// temp directory; requiring an absolute, already-clean path keeps a
	// mis-set value from writing somewhere relative to the checkout.
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("GITHUB_OUTPUT is not an absolute clean path: %q", path)
	}
	//nolint:gosec // G703: path is runner-provided and validated just above.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open GITHUB_OUTPUT: %w", err)
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return fmt.Errorf("write GITHUB_OUTPUT: %w", err)
	}
	return f.Close()
}

func runAudit(out *os.File) error {
	needs, err := ParseNeeds(os.Getenv("NEEDS_JSON"))
	if err != nil {
		return err
	}
	rep := Audit(AuditInput{Needs: needs, EventName: os.Getenv("EVENT_NAME")})
	for _, line := range rep.Log {
		fmt.Fprintln(out, line)
	}
	if len(rep.Failures) > 0 {
		fmt.Fprintln(out, "::error ::CI Gateway: upstream job(s) did not pass:")
		for _, f := range rep.Failures {
			fmt.Fprintf(out, "  %s\n", f)
		}
	}
	if len(rep.Violations) > 0 {
		fmt.Fprintln(out, "::error ::CI Gateway: a relevant job was skipped — path scoping is wrong, not the PR.")
		for _, v := range rep.Violations {
			fmt.Fprintf(out, "  - %s\n", v)
		}
		fmt.Fprintln(out, "Fix the taxonomy in cmd/changed-paths or the job `if:` conditions in .github/workflows/ci.yml.")
	}
	if rep.Failed() {
		return fmt.Errorf("CI Gateway failed")
	}
	fmt.Fprintln(out, "All jobs either ran or were legitimately out of scope for this change set.")
	return nil
}
