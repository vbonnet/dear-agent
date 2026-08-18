// Command docref-lint fails when a living document names a repository file
// that does not exist.
//
// # Why this exists
//
// Mining every automated-review comment on this repository's pull requests
// surfaced one recurring, mechanical mistake: a document asserts an artifact
// that is not in the tree. The reviewer raised it 32 times across 12 pull
// requests -- "Add the claimed PR-size workflow", "Add the referenced ADR or
// remove its claims", "Replace the stale activation command in the README".
// Every one of those rounds cost a review cycle to discover something a
// filesystem lookup answers for free.
//
// AGENTS.md states the rule ("a wrong living-document claim is a defect") and
// the reviewer keeps enforcing it after the fact. This turns it into a check
// that runs before the agent finishes, per harness-hygiene: turn hard
// requirements into checks.
//
// # Scope
//
// Diff-scoped by default -- only Markdown changed against the merge base is
// inspected, mirroring golangci-lint's new-from-merge-base policy. That blocks
// new breakage without demanding the pre-existing backlog be cleared first.
// Use -all for the whole-tree audit of that backlog.
//
// A reference is any backticked path carrying a known repository prefix and a
// source-file extension. It resolves against the repository root first, then
// against each ancestor directory of the citing document, because subtree docs
// such as agm/SPEC.md legitimately cite paths relative to their own subtree.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
)

// refPattern matches a backticked path with a source-file extension. Anchoring
// on the backticks keeps prose mentions of a concept from being read as paths.
var refPattern = regexp.MustCompile("`([A-Za-z0-9_./-]+\\.(?:go|yml|yaml|sh|md|json|tf|plist))`")

// knownPrefixes are the repository's real top-level source directories. A
// backticked path outside them is treated as illustrative rather than a claim
// about this tree, which keeps the check quiet on external examples.
var knownPrefixes = []string{
	".github/", ".claude/", ".codex/", "docs/", "cmd/", "scripts/", "tools/",
	"internal/", "pkg/", "agm/", "engram/", "wayfinder/", "deploy/", "infra/",
}

type finding struct {
	doc  string
	line int
	ref  string
}

func main() {
	all := flag.Bool("all", false, "audit every tracked Markdown file instead of only changed ones")
	base := flag.String("base", "origin/main", "base ref for the changed-file diff")
	flag.Parse()

	tracked, err := gitLines("ls-files")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docref-lint: list tracked files: %v\n", err)
		os.Exit(2)
	}
	trackedSet := make(map[string]bool, len(tracked))
	for _, f := range tracked {
		trackedSet[f] = true
	}

	docs := selectDocs(*all, *base, tracked)

	var findings []finding
	for _, doc := range docs {
		findings = append(findings, scanDoc(doc, trackedSet)...)
	}
	if len(findings) == 0 {
		fmt.Printf("docref-lint: %d document(s) checked, every referenced path exists\n", len(docs))
		return
	}
	report(findings)
	os.Exit(1)
}

// selectDocs returns the Markdown files to inspect. Without -all it returns
// only those changed against the merge base.
//
// A git failure here degrades to auditing the whole tree and says so on
// stderr. It deliberately does not return an error: there is no caller
// decision to make, and the one thing this must never do is quietly inspect an
// empty set, which would turn a red check green without anyone noticing.
func selectDocs(all bool, base string, tracked []string) []string {
	markdown := func(paths []string) []string {
		out := make([]string, 0, len(paths))
		for _, p := range paths {
			if strings.HasSuffix(p, ".md") && !strings.Contains(p, "/testdata/") {
				out = append(out, p)
			}
		}
		return out
	}
	if all {
		return markdown(tracked)
	}
	mergeBase, err := gitLines("merge-base", base, "HEAD")
	if err != nil || len(mergeBase) == 0 {
		fmt.Fprintf(os.Stderr, "docref-lint: no merge base against %s (%v); auditing whole tree\n", base, err)
		return markdown(tracked)
	}
	changed, err := gitLines("diff", "--name-only", "--diff-filter=ACMR", mergeBase[0], "HEAD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docref-lint: diff against %s failed (%v); auditing whole tree\n", mergeBase[0], err)
		return markdown(tracked)
	}
	trackedSet := make(map[string]bool, len(tracked))
	for _, f := range tracked {
		trackedSet[f] = true
	}
	present := make([]string, 0, len(changed))
	for _, c := range changed {
		if trackedSet[c] {
			present = append(present, c)
		}
	}
	return markdown(present)
}

// scanDoc reports every referenced path in doc that resolves nowhere.
func scanDoc(doc string, tracked map[string]bool) []finding {
	f, err := os.Open(doc)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []finding
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for n := 1; scanner.Scan(); n++ {
		for _, m := range refPattern.FindAllStringSubmatch(scanner.Text(), -1) {
			ref := m[1]
			if !hasKnownPrefix(ref) || resolves(doc, ref, tracked) {
				continue
			}
			out = append(out, finding{doc: doc, line: n, ref: ref})
		}
	}
	return out
}

func hasKnownPrefix(ref string) bool {
	for _, p := range knownPrefixes {
		if strings.HasPrefix(ref, p) {
			return true
		}
	}
	return false
}

// resolves reports whether ref names a tracked file, trying the repository root
// first and then each ancestor directory of the citing document.
func resolves(doc, ref string, tracked map[string]bool) bool {
	if tracked[ref] {
		return true
	}
	for dir := path.Dir(doc); ; dir = path.Dir(dir) {
		if dir == "." || dir == "/" {
			return false
		}
		if tracked[path.Join(dir, ref)] {
			return true
		}
	}
}

// report prints findings grouped by document, with remediation phrased the way
// a reviewer would say it: the claim is the defect, so either land the artifact
// or stop claiming it.
func report(findings []finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].doc != findings[j].doc {
			return findings[i].doc < findings[j].doc
		}
		return findings[i].line < findings[j].line
	})
	fmt.Fprintf(os.Stderr, "\ndocref-lint: %d living-document reference(s) name a file that does not exist:\n\n", len(findings))
	current := ""
	for _, f := range findings {
		if f.doc != current {
			current = f.doc
			fmt.Fprintf(os.Stderr, "  %s\n", current)
		}
		fmt.Fprintf(os.Stderr, "    line %d: %s\n", f.line, f.ref)
	}
	fmt.Fprint(os.Stderr, `
Each of these is a living-document defect (AGENTS.md: "a wrong living-document
claim is a defect"). Fix it one of two ways:

  * land the artifact the document promises, in this same change; or
  * remove or correct the claim so the document describes the tree as it is.

If the path is deliberately illustrative rather than a claim about this
repository, take it out of backticks so it does not read as one.
`)
}

func gitLines(args ...string) ([]string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, err
	}
	var lines []string
	for l := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}
