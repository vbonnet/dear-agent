package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

const dependabotCandidateBaseGoMod = `module example.com/project

go 1.26.5

toolchain go1.26.5

require (
	example.com/direct v1.2.3
	example.com/indirect v0.4.5 // indirect
)
`

func TestDependabotModuleOnlyCandidateAcceptsDependencyVersionLedUpdates(t *testing.T) {
	versionBump := strings.ReplaceAll(
		strings.ReplaceAll(dependabotCandidateBaseGoMod, "v1.2.3", "v1.2.4"),
		"v0.4.5", "v0.5.0",
	)
	baseSum := dependencySumLine("example.com/direct", "v1.2.3", "direct-base") +
		dependencySumLine("example.com/indirect", "v0.4.5", "indirect-base")
	headSum := dependencySumLine("example.com/direct", "v1.2.4", "direct-head") +
		dependencySumLine("example.com/indirect", "v0.5.0", "indirect-head")
	graphExpansion := strings.Replace(
		versionBump,
		"\n)",
		"\n\texample.com/new-indirect-one v0.1.0 // indirect\n\texample.com/new-indirect-two v0.2.0 // indirect\n)",
		1,
	)

	for _, test := range []struct {
		name string
		base map[string]string
		head map[string]string
	}{
		{
			name: "go.mod only",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod},
			head: map[string]string{"go.mod": versionBump},
		},
		{
			name: "PR 1195-like go.mod and go.sum",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod, "go.sum": baseSum},
			head: map[string]string{"go.mod": versionBump, "go.sum": headSum},
		},
		{
			name: "PR 1192-like version bump and indirect graph expansion",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod, "go.sum": baseSum},
			head: map[string]string{"go.mod": graphExpansion, "go.sum": headSum},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluateDependabotModuleCandidate(t, test.base, test.head)
			if err != nil {
				t.Fatalf("dependabotModuleOnlyCandidate() error = %v", err)
			}
			if !got {
				t.Fatal("dependabotModuleOnlyCandidate() = false, want true")
			}
		})
	}
}

func TestDependabotModuleOnlyCandidateRejectsPathAndStatusChanges(t *testing.T) {
	versionBump := strings.Replace(dependabotCandidateBaseGoMod, "v1.2.3", "v1.2.4", 1)
	baseSum := dependencySumLine("example.com/direct", "v1.2.3", "direct-base")
	headSum := dependencySumLine("example.com/direct", "v1.2.4", "direct-head")

	for _, test := range []struct {
		name string
		base map[string]string
		head map[string]string
	}{
		{
			name: "extra path",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod, "go.sum": baseSum, "README.md": "base\n"},
			head: map[string]string{"go.mod": versionBump, "go.sum": headSum, "README.md": "changed\n"},
		},
		{
			name: "go.sum only",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod, "go.sum": baseSum},
			head: map[string]string{"go.mod": dependabotCandidateBaseGoMod, "go.sum": headSum},
		},
		{
			name: "go.mod added",
			base: map[string]string{},
			head: map[string]string{"go.mod": versionBump},
		},
		{
			name: "go.mod removed",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod},
			head: map[string]string{},
		},
		{
			name: "go.sum added",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod},
			head: map[string]string{"go.mod": versionBump, "go.sum": headSum},
		},
		{
			name: "go.mod renamed",
			base: map[string]string{"go.mod": dependabotCandidateBaseGoMod},
			head: map[string]string{"dependencies.mod": versionBump},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluateDependabotModuleCandidate(t, test.base, test.head)
			if err != nil {
				t.Fatalf("dependabotModuleOnlyCandidate() error = %v", err)
			}
			if got {
				t.Fatal("dependabotModuleOnlyCandidate() = true, want false")
			}
		})
	}
}

func TestDependabotModuleOnlyCandidateRejectsNonVersionManifestChanges(t *testing.T) {
	versionBump := strings.Replace(dependabotCandidateBaseGoMod, "v1.2.3", "v1.2.4", 1)
	withPolicyDirectives := strings.Replace(
		dependabotCandidateBaseGoMod,
		"require (",
		"exclude example.com/legacy v1.0.0\n\nretract v0.9.0\n\nrequire (",
		1,
	)
	withUnchangedPolicyBump := strings.Replace(withPolicyDirectives, "v1.2.3", "v1.2.4", 1)
	withReplace := dependabotCandidateBaseGoMod + "\nreplace example.com/direct => ../direct\n"
	withReplaceBump := strings.Replace(withReplace, "v1.2.3", "v1.2.4", 1)
	indirectVersionBump := strings.Replace(dependabotCandidateBaseGoMod, "v0.4.5", "v0.5.0", 1)

	for _, test := range []struct {
		name    string
		baseMod string
		headMod string
		want    bool
		wantErr bool
	}{
		{
			name:    "unchanged exclude and retract",
			baseMod: withPolicyDirectives,
			headMod: withUnchangedPolicyBump,
			want:    true,
		},
		{
			name:    "direct require added",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "\n)", "\n\texample.com/added v1.0.0\n)", 1),
		},
		{
			name:    "indirect require added",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "\n)", "\n\texample.com/added v1.0.0 // indirect\n)", 1),
			want:    true,
		},
		{
			name:    "indirect require removed",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "\n\texample.com/indirect v0.4.5 // indirect", "", 1),
			want:    true,
		},
		{
			name:    "direct require removed",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(indirectVersionBump, "\texample.com/direct v1.2.3\n", "", 1),
		},
		{
			name:    "retained indirect requirement becomes direct",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, " // indirect", "", 1),
			want:    true,
		},
		{
			name:    "retained direct requirement becomes indirect",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "example.com/direct v1.2.4", "example.com/direct v1.2.4 // indirect", 1),
			want:    true,
		},
		{
			name:    "require added without existing version bump",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(dependabotCandidateBaseGoMod, "\n)", "\n\texample.com/added v1.0.0\n)", 1),
		},
		{
			name:    "require removed without existing version bump",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(dependabotCandidateBaseGoMod, "\n\texample.com/indirect v0.4.5 // indirect", "", 1),
		},
		{
			name:    "retained indirect requirement becomes direct without existing version bump",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(dependabotCandidateBaseGoMod, " // indirect", "", 1),
		},
		{
			name:    "module directive changed",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "example.com/project", "example.com/other", 1),
		},
		{
			name:    "go directive changed",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "go 1.26.5", "go 1.27.0", 1),
		},
		{
			name:    "toolchain directive changed",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "toolchain go1.26.5", "toolchain go1.27.0", 1),
		},
		{
			name:    "any replace directive is rejected",
			baseMod: withReplace,
			headMod: withReplaceBump,
		},
		{
			name:    "exclude directive changed",
			baseMod: withPolicyDirectives,
			headMod: strings.Replace(withUnchangedPolicyBump, "example.com/legacy v1.0.0", "example.com/legacy v1.1.0", 1),
		},
		{
			name:    "retract directive changed",
			baseMod: withPolicyDirectives,
			headMod: strings.Replace(withUnchangedPolicyBump, "retract v0.9.0", "retract v0.9.1", 1),
		},
		{
			name:    "require comment changed",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "example.com/direct v1.2.4", "example.com/direct v1.2.4 // reviewed", 1),
		},
		{
			name:    "non-require comment changed",
			baseMod: "// base policy\n" + dependabotCandidateBaseGoMod,
			headMod: "// changed policy\n" + versionBump,
		},
		{
			name:    "require comment preserved",
			baseMod: strings.Replace(dependabotCandidateBaseGoMod, "example.com/direct v1.2.3", "example.com/direct v1.2.3 // pinned", 1),
			headMod: strings.Replace(versionBump, "example.com/direct v1.2.4", "example.com/direct v1.2.4 // pinned", 1),
			want:    true,
		},
		{
			name:    "new require with custom annotation",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(versionBump, "\n)", "\n\texample.com/added v1.0.0 // pinned\n)", 1),
		},
		{
			name:    "formatting without a version change",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: strings.Replace(dependabotCandidateBaseGoMod, "example.com/direct v1.2.3", "example.com/direct    v1.2.3", 1),
		},
		{
			name:    "malformed base",
			baseMod: "module example.com/project\nrequire (\n",
			headMod: versionBump,
			wantErr: true,
		},
		{
			name:    "malformed head",
			baseMod: dependabotCandidateBaseGoMod,
			headMod: "module example.com/project\nrequire (\n",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluateDependabotModuleCandidate(
				t,
				map[string]string{"go.mod": test.baseMod},
				map[string]string{"go.mod": test.headMod},
			)
			if test.wantErr {
				if err == nil {
					t.Fatalf("dependabotModuleOnlyCandidate() = (%v, nil), want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("dependabotModuleOnlyCandidate() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("dependabotModuleOnlyCandidate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDependabotModuleOnlyCandidateAuthenticatesRequireBlockComments(t *testing.T) {
	versionBump := strings.Replace(dependabotCandidateBaseGoMod, "v1.2.3", "v1.2.4", 1)
	policyBlocks := `module example.com/project

go 1.26.5

toolchain go1.26.5

require ( // runtime policy
	example.com/direct v1.2.3
)

require ( // tooling policy
	example.com/indirect v0.4.5 // indirect
)
`
	policyBlockSwap := `module example.com/project

go 1.26.5

toolchain go1.26.5

require ( // runtime policy
	example.com/indirect v0.4.5 // indirect
)

require ( // tooling policy
	example.com/direct v1.2.4
)
`

	for _, test := range []struct {
		name    string
		baseMod string
		headMod string
		want    bool
	}{
		{
			name:    "block opener comment preserved",
			baseMod: strings.Replace(dependabotCandidateBaseGoMod, "require (", "require ( // dependency policy", 1),
			headMod: strings.Replace(versionBump, "require (", "require ( // dependency policy", 1),
			want:    true,
		},
		{
			name:    "block opener comment changed",
			baseMod: strings.Replace(dependabotCandidateBaseGoMod, "require (", "require ( // dependency policy", 1),
			headMod: strings.Replace(versionBump, "require (", "require ( // changed policy", 1),
		},
		{
			name:    "block opener comment removed",
			baseMod: strings.Replace(dependabotCandidateBaseGoMod, "require (", "require ( // dependency policy", 1),
			headMod: versionBump,
		},
		{
			name:    "block closing comment changed",
			baseMod: strings.Replace(dependabotCandidateBaseGoMod, "\n)", "\n\t// dependency policy\n)", 1),
			headMod: strings.Replace(versionBump, "\n)", "\n\t// changed policy\n)", 1),
		},
		{
			name:    "requirements swapped across policy-commented blocks",
			baseMod: policyBlocks,
			headMod: policyBlockSwap,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluateDependabotModuleCandidate(
				t,
				map[string]string{"go.mod": test.baseMod},
				map[string]string{"go.mod": test.headMod},
			)
			if err != nil {
				t.Fatalf("dependabotModuleOnlyCandidate() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("dependabotModuleOnlyCandidate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDependabotModuleOnlyCandidateValidatesGoSumSemantics(t *testing.T) {
	versionBump := strings.Replace(dependabotCandidateBaseGoMod, "v1.2.3", "v1.2.4", 1)
	oldVersion := dependencySumLine("example.com/direct", "v1.2.3", "old-version")
	newVersion := dependencySumLine("example.com/direct", "v1.2.4", "new-version")
	commonGoMod := dependencySumLine("example.com/stable", "v1.0.0/go.mod", "common-go-mod")

	for _, test := range []struct {
		name    string
		baseSum string
		headSum string
		want    bool
		wantErr bool
	}{
		{
			name:    "legitimate checksum removal and addition",
			baseSum: commonGoMod + oldVersion,
			headSum: commonGoMod + newVersion,
			want:    true,
		},
		{
			name:    "Windows CRLF checksum files",
			baseSum: strings.ReplaceAll(commonGoMod+oldVersion, "\n", "\r\n"),
			headSum: strings.ReplaceAll(commonGoMod+newVersion, "\n", "\r\n"),
			want:    true,
		},
		{
			name:    "common checksum mutated",
			baseSum: dependencySumLine("example.com/stable", "v1.0.0", "stable-base") + oldVersion,
			headSum: dependencySumLine("example.com/stable", "v1.0.0", "stable-head") + newVersion,
		},
		{
			name:    "duplicate checksum key",
			baseSum: oldVersion,
			headSum: newVersion + newVersion,
			wantErr: true,
		},
		{
			name:    "malformed checksum hash",
			baseSum: oldVersion,
			headSum: "example.com/direct v1.2.4 h1:not-canonical\n",
			wantErr: true,
		},
		{
			name:    "malformed checksum shape",
			baseSum: oldVersion,
			headSum: "example.com/direct  v1.2.4 h1:not-canonical\n",
			wantErr: true,
		},
		{
			name:    "invalid module version",
			baseSum: oldVersion,
			headSum: dependencySumLine("example.com/direct", "v1.2", "non-canonical-version"),
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := evaluateDependabotModuleCandidate(
				t,
				map[string]string{"go.mod": dependabotCandidateBaseGoMod, "go.sum": test.baseSum},
				map[string]string{"go.mod": versionBump, "go.sum": test.headSum},
			)
			if test.wantErr {
				if err == nil {
					t.Fatalf("dependabotModuleOnlyCandidate() = (%v, nil), want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("dependabotModuleOnlyCandidate() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("dependabotModuleOnlyCandidate() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDependabotModuleOnlyCandidateRequiresAuthenticatedMergeBase(t *testing.T) {
	got, err := dependabotModuleOnlyCandidate(context.Background(), "HEAD", "HEAD")
	if err == nil {
		t.Fatalf("dependabotModuleOnlyCandidate() = (%v, nil), want an authentication error", got)
	}
}

func evaluateDependabotModuleCandidate(t *testing.T, baseFiles, headFiles map[string]string) (bool, error) {
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
	if _, ok := baseFiles["README.md"]; !ok {
		baseFiles["README.md"] = "fixture\n"
	}
	if _, ok := headFiles["README.md"]; !ok {
		headFiles["README.md"] = "fixture\n"
	}
	writeDependencyFixtureFiles(t, dir, baseFiles)
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	base := strings.TrimSpace(run("rev-parse", "HEAD"))

	for path := range baseFiles {
		if _, retained := headFiles[path]; retained {
			continue
		}
		if err := os.Remove(filepath.Join(dir, path)); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
	writeDependencyFixtureFiles(t, dir, headFiles)
	run("add", "-A")
	run("commit", "--allow-empty", "-q", "-m", "head")
	head := strings.TrimSpace(run("rev-parse", "HEAD"))

	t.Chdir(dir)
	return dependabotModuleOnlyCandidate(context.Background(), base, head)
}

func cloneDependencyFixtureFiles(files map[string]string) map[string]string {
	cloned := maps.Clone(files)
	return cloned
}

func writeDependencyFixtureFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, contents := range files {
		absolute := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatalf("create parent for %s: %v", path, err)
		}
		if err := os.WriteFile(absolute, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func dependencySumLine(modulePath, version, seed string) string {
	digest := sha256.Sum256([]byte(seed))
	return modulePath + " " + version + " h1:" + base64.StdEncoding.EncodeToString(digest[:]) + "\n"
}
