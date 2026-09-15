package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// securityPatchGoMod reproduces the shape of PR 1441, the x/crypto and
// fast-uri security patch that deadlocked the repository behind the
// "go.mod changed" escalation.
const securityPatchGoMod = `module github.com/vbonnet/dear-agent

go 1.26.5

toolchain go1.26.5

require (
	golang.org/x/mod v0.29.0
	golang.org/x/crypto v0.44.0 // indirect
)
`

func TestClassifyDependencyGraphDeltaAcceptsRoutineVersionUpdates(t *testing.T) {
	patched := strings.Replace(securityPatchGoMod, "golang.org/x/crypto v0.44.0", "golang.org/x/crypto v0.45.0", 1)
	baseSum := dependencySumLine("golang.org/x/crypto", "v0.44.0", "crypto-base")
	headSum := dependencySumLine("golang.org/x/crypto", "v0.45.0", "crypto-head")

	for _, test := range []struct {
		name  string
		base  map[string]string
		head  map[string]string
		paths []string
	}{
		{
			name:  "PR 1441-like security patch of an indirect module",
			base:  map[string]string{"go.mod": securityPatchGoMod, "go.sum": baseSum},
			head:  map[string]string{"go.mod": patched, "go.sum": headSum},
			paths: []string{"go.mod", "go.sum"},
		},
		{
			name: "PR 1434-like family patch bump",
			base: map[string]string{"go.mod": securityPatchGoMod, "go.sum": baseSum},
			head: map[string]string{
				"go.mod": strings.Replace(patched, "golang.org/x/mod v0.29.0", "golang.org/x/mod v0.29.1", 1),
				"go.sum": headSum,
			},
			paths: []string{"go.mod", "go.sum"},
		},
		{
			name:  "go.mod only",
			base:  map[string]string{"go.mod": securityPatchGoMod},
			head:  map[string]string{"go.mod": patched},
			paths: []string{"go.mod"},
		},
		{
			name: "bump that resolves a new indirect module",
			base: map[string]string{"go.mod": securityPatchGoMod},
			head: map[string]string{
				"go.mod": strings.Replace(patched, "\n)", "\n\tgolang.org/x/text v0.31.0 // indirect\n)", 1),
			},
			paths: []string{"go.mod"},
		},
		{
			name:  "dependency update beside unrelated source changes",
			base:  map[string]string{"go.mod": securityPatchGoMod, "internal/app/app.go": "package app\n"},
			head:  map[string]string{"go.mod": patched, "internal/app/app.go": "package app\n\nconst Name = \"app\"\n"},
			paths: []string{"go.mod"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			delta := evaluateDependencyGraphDelta(t, test.base, test.head)
			if !delta.Routine {
				t.Fatalf("classifyDependencyGraphDelta() Routine = false, causes = %v", delta.Causes)
			}
			if strings.Join(delta.Paths, ",") != strings.Join(test.paths, ",") {
				t.Fatalf("classifyDependencyGraphDelta() Paths = %v, want %v", delta.Paths, test.paths)
			}
		})
	}
}

func TestClassifyDependencyGraphDeltaEscalatesFoundationalChanges(t *testing.T) {
	patched := strings.Replace(securityPatchGoMod, "golang.org/x/crypto v0.44.0", "golang.org/x/crypto v0.45.0", 1)
	majorBump := strings.Replace(securityPatchGoMod, "golang.org/x/mod v0.29.0", "golang.org/x/mod v1.0.0", 1)

	for _, test := range []struct {
		name      string
		base      map[string]string
		head      map[string]string
		wantCause string
	}{
		{
			name:      "new direct dependency",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": strings.Replace(patched, "\n)", "\n\texample.com/new v1.0.0\n)", 1)},
			wantCause: "adds direct requirement example.com/new",
		},
		{
			name:      "major version bump",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": majorBump},
			wantCause: "changes the major version of requirement golang.org/x/mod",
		},
		{
			name: "replace directive",
			base: map[string]string{"go.mod": securityPatchGoMod},
			head: map[string]string{
				"go.mod": patched + "\nreplace golang.org/x/mod => ../mod\n",
			},
			wantCause: "declares a replace directive",
		},
		{
			name:      "go directive change",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": strings.Replace(patched, "go 1.26.5", "go 1.27.0", 1)},
			wantCause: "changes the go directive",
		},
		{
			name:      "toolchain change",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": strings.Replace(patched, "toolchain go1.26.5", "toolchain go1.27.0", 1)},
			wantCause: "changes the toolchain directive",
		},
		{
			name:      "direct dependency removed",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": strings.Replace(patched, "\tgolang.org/x/mod v0.29.0\n", "", 1)},
			wantCause: "removes direct requirement golang.org/x/mod",
		},
		{
			name:      "no version change at all",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": strings.Replace(securityPatchGoMod, "golang.org/x/mod v0.29.0", "golang.org/x/mod    v0.29.0", 1)},
			wantCause: "changes no existing requirement version",
		},
		{
			name:      "unparsable manifest",
			base:      map[string]string{"go.mod": securityPatchGoMod},
			head:      map[string]string{"go.mod": "module github.com/vbonnet/dear-agent\nrequire (\n"},
			wantCause: "does not parse as a Go module manifest",
		},
		{
			name:      "workspace manifest",
			base:      map[string]string{"go.mod": securityPatchGoMod, "go.work": "go 1.26.5\n\nuse .\n"},
			head:      map[string]string{"go.mod": patched, "go.work": "go 1.26.5\n\nuse (\n\t.\n\t./tools\n)\n"},
			wantCause: "is a workspace or vendored build input rather than a module manifest",
		},
		{
			name:      "vendored tree",
			base:      map[string]string{"go.mod": securityPatchGoMod, "vendor/example.com/dep/dep.go": "package dep\n"},
			head:      map[string]string{"go.mod": patched, "vendor/example.com/dep/dep.go": "package dep\n\nvar X = 1\n"},
			wantCause: "is a workspace or vendored build input rather than a module manifest",
		},
		{
			name:      "go.sum without go.mod",
			base:      map[string]string{"go.sum": dependencySumLine("golang.org/x/crypto", "v0.44.0", "crypto-base")},
			head:      map[string]string{"go.sum": dependencySumLine("golang.org/x/crypto", "v0.45.0", "crypto-head")},
			wantCause: "cannot be proven without a modified go.mod",
		},
		{
			name:      "retained checksum rewritten",
			base:      map[string]string{"go.mod": securityPatchGoMod, "go.sum": dependencySumLine("golang.org/x/mod", "v0.29.0", "mod-base")},
			head:      map[string]string{"go.mod": patched, "go.sum": dependencySumLine("golang.org/x/mod", "v0.29.0", "mod-tampered")},
			wantCause: "rewrites the checksum of a retained module version",
		},
		{
			name:      "go.mod added rather than modified",
			base:      map[string]string{},
			head:      map[string]string{"go.mod": securityPatchGoMod},
			wantCause: "is added, deleted, or retyped rather than modified",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			delta := evaluateDependencyGraphDelta(t, test.base, test.head)
			if delta.Routine {
				t.Fatalf("classifyDependencyGraphDelta() Routine = true for %s", test.name)
			}
			if !slices.Contains(delta.Causes, test.wantCause) {
				t.Fatalf("classifyDependencyGraphDelta() Causes = %v, want one containing %q", delta.Causes, test.wantCause)
			}
		})
	}
}

func TestClassifyDependencyGraphDeltaIgnoresDiffsWithoutDependencyPaths(t *testing.T) {
	delta := evaluateDependencyGraphDelta(t,
		map[string]string{"docs/notes.md": "base\n"},
		map[string]string{"docs/notes.md": "head\n"},
	)
	if delta.Routine || len(delta.Paths) != 0 {
		t.Fatalf("classifyDependencyGraphDelta() = %#v, want an empty non-routine delta", delta)
	}
}

func evaluateDependencyGraphDelta(t *testing.T, baseFiles, headFiles map[string]string) dependencyGraphDelta {
	t.Helper()
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	run := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	run("init", "-q")
	sandbox.HardenRepo(t, dir)

	baseFiles = cloneDependencyFixtureFiles(baseFiles)
	headFiles = cloneDependencyFixtureFiles(headFiles)
	baseFiles["README.md"] = "fixture\n"
	headFiles["README.md"] = "fixture\n"
	writeDependencyFixtureFiles(t, dir, baseFiles)
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	base := strings.TrimSpace(run("rev-parse", "HEAD"))

	for path := range baseFiles {
		if _, retained := headFiles[path]; retained {
			continue
		}
		run("rm", "-q", "--", path)
	}
	writeDependencyFixtureFiles(t, dir, headFiles)
	run("add", "-A")
	run("commit", "--allow-empty", "-q", "-m", "head")
	head := strings.TrimSpace(run("rev-parse", "HEAD"))

	t.Chdir(dir)
	statuses := changedStatusesForTest(t, base, head)
	delta, err := classifyDependencyGraphDelta(context.Background(), base, head, statuses)
	if err != nil {
		t.Fatalf("classifyDependencyGraphDelta() error = %v", err)
	}
	return delta
}

func changedStatusesForTest(t *testing.T, base, head string) map[string]string {
	t.Helper()
	out, err := gitOutputBounded(context.Background(), maxGitMetadataBytes,
		"diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-status", "-z", base, head)
	if err != nil {
		t.Fatalf("git diff --name-status: %v", err)
	}
	fields := bytesSplitNUL(out)
	if len(fields)%2 != 0 {
		t.Fatalf("malformed name-status evidence: %q", out)
	}
	statuses := make(map[string]string, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		statuses[string(fields[i+1])] = string(fields[i])
	}
	return statuses
}
