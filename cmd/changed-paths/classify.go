package main

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Keys is the ordered set of outputs this tool emits. Callers consume them as
// job-level `if:` conditions, so the set is part of the workflow contract.
var Keys = []string{"go", "agm", "engram", "deps", "docs", "adr", "global"}

// Selection is the classification of one change set.
type Selection struct {
	Values map[string]bool
	// Reason is non-empty when the classifier bailed out to the fail-safe
	// "everything is relevant" answer. It is surfaced as a GitHub notice.
	Reason string
}

// AllTrue is the fail-safe answer: anything that makes the change set
// unknowable forces every consumer to run. Under-running is a silent hole in
// the gate; over-running only costs runner minutes.
func AllTrue(reason string) Selection {
	v := make(map[string]bool, len(Keys))
	for _, k := range Keys {
		v[k] = true
	}
	return Selection{Values: v, Reason: reason}
}

// globalRe matches inputs that can invalidate the meaning of the selection
// itself: build metadata, the CI definition (including this classifier's own
// workflow), and lint/tool configuration that changes what a check means.
var globalRe = regexp.MustCompile(
	`(^|/)(go\.mod|go\.sum|go\.work|go\.work\.sum|Makefile)$` +
		`|^vendor/` +
		`|^\.github/` +
		`|^\.golangci\.ya?ml$` +
		`|^\.dear-agent\.ya?ml$`)

var depsRe = regexp.MustCompile(
	`(^|/)(go\.mod|go\.sum|package\.json|package-lock\.json|pnpm-lock\.yaml)$|\.lock$`)

// documentExts are the extensions of files that are *only* ever read by
// humans. Everything else is treated as a build input.
//
// The polarity here is deliberate and is the opposite of the obvious design.
// An allowlist of "Go-relevant" extensions (`.go`, `.tmpl`, …) silently
// under-selects the moment someone adds a new kind of embedded asset: the
// compiled program changes, the classifier says `go=false`, and Build & Test
// skips. A denylist of "provably documentation" over-selects instead, which
// costs runner minutes. See ADR-038.
var documentExts = map[string]bool{
	".md": true, ".mdx": true, ".markdown": true,
	".txt": true, ".rst": true, ".adoc": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".webp": true, ".ico": true, ".pdf": true,
}

// verifiedRoots hold non-Go assets that a step inside the Build & Test job
// checks. They are product, not documentation, even when they are Markdown:
//
//   - agm/agm-plugin/commands — content-hashed by `make plugin-verify-hashes`
//   - skills                  — validated by `go run ./tools/skill-lint`
//
// //go:embed ownership is discovered from the tree (see embed.go) rather than
// listed here, so a new embedded asset cannot drift out of the taxonomy.
var verifiedRoots = []string{
	"agm/agm-plugin/commands",
	"skills",
}

// Classifier turns a change set into a Selection.
type Classifier struct {
	// EmbedRoots are repo-relative files and directories reachable from a
	// //go:embed directive. A change to any of them changes the compiled
	// program even though no .go file moved.
	EmbedRoots []string
}

// Classify applies the path taxonomy to a change set. files must already
// include both sides of every rename (see diff.go).
func (c *Classifier) Classify(files []string) Selection {
	if len(files) == 0 {
		return AllTrue("empty diff (nothing to select on)")
	}
	for _, f := range files {
		if globalRe.MatchString(f) {
			return AllTrue("a global input changed (build metadata, .github/**, or lint config): " + f)
		}
	}

	sel := Selection{Values: map[string]bool{}}
	for _, k := range Keys {
		sel.Values[k] = false
	}
	for _, f := range files {
		if c.isBuildInput(f) {
			sel.Values["go"] = true
		}
		if hasDirPrefix(f, "agm") {
			sel.Values["agm"] = true
		}
		if hasDirPrefix(f, "engram") {
			sel.Values["engram"] = true
		}
		if depsRe.MatchString(f) {
			sel.Values["deps"] = true
		}
		if strings.EqualFold(path.Ext(f), ".md") {
			sel.Values["docs"] = true
		}
		if hasDirPrefix(f, "docs/adr") {
			sel.Values["adr"] = true
		}
	}
	return sel
}

// isBuildInput reports whether a path can change what the Go toolchain
// produces or what a Build & Test step verifies.
func (c *Classifier) isBuildInput(f string) bool {
	if !documentExts[strings.ToLower(path.Ext(f))] {
		return true
	}
	// Markdown and other document formats are still build inputs when they are
	// compiled in via //go:embed or hash-verified inside Build & Test.
	for _, root := range verifiedRoots {
		if hasDirPrefix(f, root) {
			return true
		}
	}
	return c.isEmbedded(f)
}

func (c *Classifier) isEmbedded(f string) bool {
	for _, root := range c.EmbedRoots {
		if f == root || hasDirPrefix(f, root) {
			return true
		}
	}
	return false
}

// hasDirPrefix reports whether f sits at or under the directory dir, without
// the `agm/x` vs `agmx` false positive a plain strings.HasPrefix would give.
func hasDirPrefix(f, dir string) bool {
	return f == dir || strings.HasPrefix(f, dir+"/")
}

// Render formats the selection for the job log.
func (s Selection) Render() string {
	keys := append([]string(nil), Keys...)
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString("  ")
		b.WriteString(k)
		b.WriteString("=")
		if s.Values[k] {
			b.WriteString("true\n")
		} else {
			b.WriteString("false\n")
		}
	}
	return b.String()
}
