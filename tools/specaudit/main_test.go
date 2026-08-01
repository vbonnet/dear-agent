package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestGitOutputIsBounded(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "large.txt", strings.Repeat("x", 256))
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "large output")

	if _, err := gitWithOutputLimit(repository, 64, "show", "HEAD:large.txt"); err == nil || !strings.Contains(err.Error(), "output exceeds 64 bytes") {
		t.Fatalf("gitWithOutputLimit() error = %v, want bounded-output rejection", err)
	}
}

func TestGitWallTimeIsBounded(t *testing.T) {
	requireLinuxCallerSelectedGit(t)
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := resolveTestExecutable(t, fakeGit)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := gitBytesWithContext(ctx, executable, t.TempDir(), 64, nil, "version")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("gitBytesWithContext() error = %v, want deterministic wall-time rejection", err)
	}
}

func TestPinnedBlobBodiesUseSingleBatchProcess(t *testing.T) {
	requireLinuxCallerSelectedGit(t)
	marker := filepath.Join(t.TempDir(), "invocations")
	fakeGit := filepath.Join(t.TempDir(), "git")
	firstOID := strings.Repeat("a", 40)
	secondOID := strings.Repeat("b", 40)
	script := fmt.Sprintf(`#!/bin/sh
printf x >> %q
while IFS= read -r oid; do
  case "$oid" in
    %s) printf '%s blob 3\none\n' ;;
    %s) printf '%s blob 3\ntwo\n' ;;
    *) exit 99 ;;
  esac
done
`, marker, firstOID, firstOID, secondOID, secondOID)
	if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := resolveTestExecutable(t, fakeGit)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bodies, err := readPinnedBlobBodies(ctx, executable, t.TempDir(), []pinnedBlob{
		{path: "one/SPEC.md", oid: firstOID, size: 3},
		{path: "two/SPEC.md", oid: secondOID, size: 3},
	}, 6, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(bodies[firstOID]) != "one" || string(bodies[secondOID]) != "two" {
		t.Fatalf("batch bodies = %#v", bodies)
	}
	invocations, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(invocations) != "x" {
		t.Fatalf("batch Git invocations = %q, want exactly one", invocations)
	}
}

func resolveTestExecutable(t *testing.T, path string) gitExecutable {
	t.Helper()
	executable, err := resolveAbsoluteGitExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	return executable
}

func requireLinuxCallerSelectedGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("caller-selected executable tests require Linux sealed-descriptor execution")
	}
}

func realTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestInventoryRejectsAggregateCorpusAboveLimit(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "one/SPEC.md", "# One\n\n**ONE-01** When one runs, the system shall "+strings.Repeat("x", 96)+".\n")
	writeTestFile(t, repository, "two/SPEC.md", "# Two\n\n**TWO-01** When two runs, the system shall "+strings.Repeat("y", 96)+".\n")
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "large corpus")
	revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))

	_, err := inventoryWithCorpusLimit(repository, "owner/repository", revision, 100)
	if err == nil || !strings.Contains(err.Error(), "SPEC audit corpus exceeds 100 bytes") {
		t.Fatalf("inventoryWithCorpusLimit() error = %v, want aggregate corpus rejection", err)
	}
}

func TestInventoryRejectsAggregateCorpusFileCountAboveLimit(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, repository, "one/SPEC.md", "# One\n\n**ONE-01** When one runs, the system shall remain bounded.\n")
	writeTestFile(t, repository, "two/SPEC.md", "# Two\n\n**TWO-01** When two runs, the system shall remain bounded.\n")
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "file-count corpus")
	revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))

	_, err := inventoryWithLimits(repository, "owner/repository", revision, inventoryLimits{
		corpusBytes: maxCorpusBytes,
		corpusFiles: 2,
		wallTime:    maxInventoryDuration,
	})
	if err == nil || !strings.Contains(err.Error(), "corpus exceeds 2 files") {
		t.Fatalf("inventoryWithLimits() error = %v, want aggregate file-count rejection", err)
	}
}

func TestInventoryRejectsExpiredGlobalDeadline(t *testing.T) {
	_, err := inventoryWithLimits(t.TempDir(), "owner/repository", strings.Repeat("a", 40), inventoryLimits{
		corpusBytes: maxCorpusBytes,
		corpusFiles: maxInventoryFiles,
		wallTime:    time.Nanosecond,
	})
	if err == nil || !strings.Contains(err.Error(), "global wall-time limit") {
		t.Fatalf("inventoryWithLimits() error = %v, want global deadline rejection", err)
	}
}

func TestReportInputsAndArtifactsAreBounded(t *testing.T) {
	input := filepath.Join(realTempDir(t), "report.json")
	if err := os.WriteFile(input, []byte(strings.Repeat("x", 128)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readReportWithLimit(input, 64); err == nil || !strings.Contains(err.Error(), "exceeds 64 bytes") {
		t.Fatalf("readReportWithLimit() error = %v, want bounded-input rejection", err)
	}

	report := validReport()
	report.Limitations = []string{strings.Repeat("z", 256)}
	if _, err := marshalReportWithLimit(report, 64); err == nil || !strings.Contains(err.Error(), "64-byte artifact output limit") {
		t.Fatalf("marshalReportWithLimit() error = %v, want bounded JSON rejection", err)
	}
	if _, err := renderHTMLWithLimit(report, nil, 64); err == nil || !strings.Contains(err.Error(), "64-byte artifact output limit") {
		t.Fatalf("renderHTMLWithLimit() error = %v, want bounded HTML rejection", err)
	}
}

func TestBoundedJSONEncodingStopsBeforeEscapedExpansion(t *testing.T) {
	report := validReport()
	report.Limitations = []string{strings.Repeat("\x00", 8<<20)}
	if _, err := marshalReportWithLimit(report, 1024); err == nil || !strings.Contains(err.Error(), "1024-byte artifact output limit") {
		t.Fatalf("marshalReportWithLimit() error = %v, want bounded escaped JSON rejection", err)
	}
}

func TestBoundedJSONEncodingMatchesOmitEmptyAndRoundTrips(t *testing.T) {
	type optionalFields struct {
		Enabled bool `json:"enabled,omitempty"`
		Count   int  `json:"count,omitempty"`
	}
	output := &boundedJSONBuffer{limit: 64}
	if err := encodeBoundedJSON(output, reflect.ValueOf(optionalFields{}), "", "  "); err != nil {
		t.Fatal(err)
	}
	if got := output.Bytes(); string(got) != "{}" {
		t.Fatalf("omitempty encoding=%q, want {}", got)
	}

	semantic := validReport()
	semantic.Candidates = []finding{{ID: "SPEC-CLUSTER-001", Rank: 0}}
	data, err := marshalReportWithLimit(semantic, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"rank"`)) {
		t.Fatalf("zero finding rank was not omitted: %s", data)
	}
	var decoded report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, semantic) {
		t.Fatalf("bounded JSON round trip changed report:\n got=%#v\nwant=%#v", decoded, semantic)
	}
}

func TestAlternateObjectRoutingCapturesNestedRoutesAndRejectsCyclesOrDuplicates(t *testing.T) {
	root := realTempDir(t)
	objects := filepath.Join(root, "objects")
	first := filepath.Join(root, "alternate-one")
	second := filepath.Join(root, "alternate-two")
	for _, directory := range []string{objects, first, second} {
		if err := os.MkdirAll(filepath.Join(directory, "info"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(objects, "info", "alternates"), []byte("../alternate-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "info", "alternates"), []byte("../alternate-two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := alternateObjectDirIdentities(objects)
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity, err := directoryPathIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity, err := directoryPathIdentity(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(routes, []string{firstIdentity, secondIdentity}) {
		t.Fatalf("nested alternate routes=%#v, want %#v", routes, []string{firstIdentity, secondIdentity})
	}

	if err := os.WriteFile(filepath.Join(second, "info", "alternates"), []byte("../objects\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := alternateObjectDirIdentities(objects); err == nil || !strings.Contains(err.Error(), "duplicate or cycle") {
		t.Fatalf("cycle error=%v, want fail-closed duplicate or cycle", err)
	}
	if err := os.WriteFile(filepath.Join(second, "info", "alternates"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(objects, "info", "alternates"), []byte("../alternate-one\n../alternate-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := alternateObjectDirIdentities(objects); err == nil || !strings.Contains(err.Error(), "duplicate or cycle") {
		t.Fatalf("duplicate error=%v, want fail-closed duplicate or cycle", err)
	}
}

func TestEscapedHTMLStopsAtArtifactLimit(t *testing.T) {
	out := newBoundedHTMLBuilder(8)
	fmt.Fprintf(out, "%s", esc(strings.Repeat("&", 1<<20)))
	if !errors.Is(out.Err(), errHTMLArtifactLimit) {
		t.Fatalf("bounded renderer error = %v, want artifact-limit rejection", out.Err())
	}
	if got := len(out.String()); got > 8 {
		t.Fatalf("bounded renderer retained %d bytes, want at most 8", got)
	}
}

func TestStableReportReadRejectsInPlaceMutation(t *testing.T) {
	path := filepath.Join(realTempDir(t), "report.json")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("mutated!"), 0o644); err != nil {
		t.Fatal(err)
	}
	changedTime := before.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	after, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	pathAfter, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stableReportRead(before, after, pathAfter, len("mutated!")) {
		t.Fatal("stableReportRead accepted an in-place mutation on the same inode")
	}
}

func TestReportInputDoubleReadRejectsContentMutationWithRestoredMetadata(t *testing.T) {
	path := filepath.Join(realTempDir(t), "report.json")
	original := []byte("original")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = readStableBoundedFileWithHook(path, 1024, func() error {
		if err := os.WriteFile(path, []byte("mutated!"), 0o600); err != nil {
			return err
		}
		return os.Chtimes(path, info.ModTime(), info.ModTime())
	})
	if err == nil || !strings.Contains(err.Error(), "failed double-read content authentication") {
		t.Fatalf("readStableBoundedFileWithHook() error = %v, want content-authentication rejection", err)
	}
}

func TestReportInputRejectsSymlinkAndHardlinkAliases(t *testing.T) {
	directory := realTempDir(t)
	target := filepath.Join(directory, "target.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		make func(string) error
		want string
	}{
		{name: "symlink", make: func(path string) error { return os.Symlink(target, path) }, want: "non-symlink"},
		{name: "hardlink", make: func(path string) error { return os.Link(target, path) }, want: "exactly one filesystem link"},
	} {
		t.Run(test.name, func(t *testing.T) {
			alias := filepath.Join(directory, test.name+".json")
			if err := test.make(alias); err != nil {
				t.Fatal(err)
			}
			_, err := readStableBoundedFile(alias, 1024)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readStableBoundedFile() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReportInputRejectsOriginalSymlinkAncestor(t *testing.T) {
	realDirectory := realTempDir(t)
	if err := os.WriteFile(filepath.Join(realDirectory, "report.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasParent := filepath.Join(realTempDir(t), "linked-parent")
	if err := os.Symlink(realDirectory, aliasParent); err != nil {
		t.Fatal(err)
	}
	_, err := readStableBoundedFile(filepath.Join(aliasParent, "report.json"), 1024)
	if err == nil || !strings.Contains(err.Error(), "not a stable directory") {
		t.Fatalf("readStableBoundedFile() error = %v, want original-ancestor symlink rejection", err)
	}
}

func TestReportInputRejectsDuplicateJSONKeysAndMultipleValues(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "top-level duplicate", body: `{"schema_version":"spec-audit/v1","schema_version":"spec-audit/v1"}`, want: `duplicate JSON object key "schema_version"`},
		{name: "nested duplicate", body: `{"schema_version":"spec-audit/v1","snapshot":{"repository":"one","repository":"two"}}`, want: `duplicate JSON object key "repository"`},
		{name: "multiple values", body: `{}` + "\n" + `{}`, want: "multiple JSON values"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(realTempDir(t), "report.json")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readReportWithLimit(path, 4096)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readReportWithLimit() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When a codex-cli request runs, the system shall persist identity.\n\n## BDD Traceability\n\n- Feature: `test/bdd/shared.feature`\n")
	writeTestFile(t, repo, "two/SPEC.md", "# Two\n\n**ONE-01** When a codex-cli request runs, the system shall persist identity.\n\n## BDD Traceability\n\n- Feature: `test/bdd/shared.feature`\n")
	writeTestFile(t, repo, "three/SPEC.md", "# One\n\n**ONE-01** When a codex-cli request runs, the system shall persist identity.\n\n## BDD Traceability\n\n- Feature: `test/bdd/shared.feature`\n")
	writeTestFile(t, repo, "test/bdd/shared.feature", "# SPEC: one/SPEC.md\n# RELATED-SPEC: two/SPEC.md\n# RELATED-SPEC: three/SPEC.md\nFeature: Shared\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "initial")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeTestFile(t, repo, "one/SPEC.md", "not tracked at this revision\n")

	var first, second, stderr bytes.Buffer
	args := []string{"inventory", "-repo", repo, "-repository", "owner/repo", "-revision", revision}
	if code := run(args, &first, &stderr); code != 0 {
		t.Fatalf("first inventory=%d: %s", code, stderr.String())
	}
	if code := run(args, &second, &stderr); code != 0 {
		t.Fatalf("second inventory=%d: %s", code, stderr.String())
	}
	if first.String() != second.String() {
		t.Fatal("inventory JSON is not deterministic")
	}
	var got report
	if err := json.Unmarshal(first.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Snapshot.Repository != "owner/repo" || got.Snapshot.Revision != revision || got.Summary.SpecFiles != 3 || got.Summary.Requirements != 3 {
		t.Fatalf("unexpected inventory summary: %#v", got.Summary)
	}
	if len(got.Seeds) != 5 {
		t.Fatalf("seeds=%d, want exact-body, duplicate-id, shared-bdd, identical-file, harness-terminology", len(got.Seeds))
	}
	wantKinds := []string{"duplicate-id", "exact-body", "harness-terminology", "identical-file", "shared-bdd"}
	for index, want := range wantKinds {
		if got.Seeds[index].Kind != want {
			t.Fatalf("seed kinds=%#v, want deterministic order %#v", got.Seeds, wantKinds)
		}
	}
	if len(got.Candidates) != 0 || len(got.NonCandidates) != 0 {
		t.Fatalf("lexical seeds became semantic verdicts: candidates=%#v non_candidates=%#v", got.Candidates, got.NonCandidates)
	}
	if got.Inventory[0].Requirements[0].Excerpt == "not tracked at this revision" {
		t.Fatal("inventory read working tree instead of pinned revision")
	}
}

func TestHarnessTerminologySeedsRequireLexicalBoundariesAndDistinctPaths(t *testing.T) {
	files := []specFile{
		{Path: "false-one/SPEC.md", SHA256: strings.Repeat("a", 64), Requirements: []requirement{{ID: "FALSE-01", Line: 3, Body: "false one", Excerpt: "**FALSE-01** When strategy changes, the system shall ignore embedded substrings."}}},
		{Path: "false-two/SPEC.md", SHA256: strings.Repeat("b", 64), Requirements: []requirement{{ID: "FALSE-02", Line: 4, Body: "false two", Excerpt: "**FALSE-02** When codex-cli-tools run, the system shall ignore extended identifiers."}}},
		{Path: "true-one/SPEC.md", SHA256: strings.Repeat("c", 64), Requirements: []requirement{{ID: "TRUE-01", Line: 5, Body: "true one", Excerpt: "**TRUE-01** When AGY runs, the system shall preserve the active harness outcome."}}},
		{Path: "true-two/SPEC.md", SHA256: strings.Repeat("d", 64), Requirements: []requirement{{ID: "TRUE-02", Line: 6, Body: "true two", Excerpt: "**TRUE-02** When agy runs, the system shall preserve the active harness boundary."}}},
	}
	seeds := collectSeeds(files, []string{"agy", "codex-cli"})
	if len(seeds) != 1 || seeds[0].Kind != "harness-terminology" || seeds[0].Key != "agy" || distinctPaths(seeds[0].Evidence) != 2 {
		t.Fatalf("harness terminology seeds=%#v, want one boundary-aware agy seed across two paths", seeds)
	}
}

func TestInventoryReadsCanonicalAndLegacyHarnessRegistryPaths(t *testing.T) {
	tests := []struct {
		name           string
		canonical      string
		legacy         string
		want           string
		wantLimitation bool
	}{
		{
			name:           "canonical registry takes precedence",
			canonical:      "package harnessregistry\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\n",
			legacy:         "package agent\nvar activeHarnesses = []string{\"claude-code\"}\n",
			want:           "codex-cli,pi-cli",
			wantLimitation: true,
		},
		{
			name:           "legacy registry remains auditable",
			legacy:         "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n",
			want:           "codex-cli",
			wantLimitation: true,
		},
		{
			name:           "malformed canonical registry never falls back to legacy",
			canonical:      "package harnessregistry\nfunc ActiveHarnesses() []string { return nil }\n",
			legacy:         "package agent\nvar activeHarnesses = []string{\"claude-code\"}\n",
			wantLimitation: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			gitTest(t, repo, "init", "-q")
			gittest.HardenRepo(t, repo)
			if test.canonical != "" {
				writeTestFile(t, repo, canonicalActiveHarnessRegistryPath, test.canonical)
			}
			if test.legacy != "" {
				writeTestFile(t, repo, legacyActiveHarnessRegistryPath, test.legacy)
			}
			writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When a harness registry moves, the system shall preserve pinned audit compatibility.\n")
			gitTest(t, repo, "add", ".")
			gitTest(t, repo, "commit", "-qm", "harness registry fixture")
			revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

			got, err := inventory(repo, "owner/repo", revision)
			if err != nil {
				t.Fatal(err)
			}
			if active := strings.Join(got.Scope.ActiveMembers, ","); active != test.want {
				t.Fatalf("active harnesses = %q, want %q; limitations=%q", active, test.want, got.Limitations)
			}
			if hasLimitation := len(got.Limitations) != 0; hasLimitation != test.wantLimitation {
				t.Fatalf("limitations = %q, want limitation=%v", got.Limitations, test.wantLimitation)
			}
		})
	}
}

func TestLinkedWorktreeWithConfiguredAlternatesDisclosesGitTrustBoundary(t *testing.T) {
	source := realTempDir(t)
	gitTest(t, source, "init", "-q")
	gittest.HardenRepo(t, source)
	writeTestFile(t, source, canonicalActiveHarnessRegistryPath, "package harnessregistry\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, source, "one/SPEC.md", "# One\n\n**ONE-01** When codex-cli runs, the system shall preserve one pinned outcome.\n")
	writeTestFile(t, source, "two/SPEC.md", "# Two\n\n**TWO-01** When codex-cli runs, the system shall preserve another pinned outcome.\n")
	gitTest(t, source, "add", ".")
	gitTest(t, source, "commit", "-qm", "alternate object fixture")

	parent := realTempDir(t)
	shared := filepath.Join(parent, "shared")
	linked := filepath.Join(parent, "linked")
	gitTest(t, parent, "clone", "-q", "--shared", source, shared)
	gittest.HardenRepo(t, shared)
	if data, err := os.ReadFile(filepath.Join(shared, ".git", "objects", "info", "alternates")); err != nil || strings.TrimSpace(string(data)) == "" {
		t.Fatalf("shared-clone fixture lacks a configured object alternate: data=%q err=%v", data, err)
	}
	gitTest(t, shared, "worktree", "add", "-q", "-b", "audit-linked", linked)
	if info, err := os.Stat(filepath.Join(linked, ".git")); err != nil || info.Mode().IsRegular() == false {
		t.Fatalf("linked-worktree fixture lacks a Git metadata file: info=%v err=%v", info, err)
	}
	revision := strings.TrimSpace(gitTest(t, linked, "rev-parse", "HEAD"))

	got, err := inventory(linked, "owner/repo", revision)
	if err != nil {
		t.Fatal(err)
	}
	if got.Methodology.GitEvidenceTrust != gitEvidenceTrustDisclosure {
		t.Fatalf("Git evidence trust disclosure=%q, want fixed boundary", got.Methodology.GitEvidenceTrust)
	}
	trust := got.Methodology.GitTrustInputs
	if !validGitTrustInputs(trust) || len(trust.AlternateObjectDirs) != 1 {
		t.Fatalf("linked-worktree trust inputs=%#v, want concrete executable, Git, common, object, and alternate identities", trust)
	}
	for _, identity := range append([]string{trust.Executable, trust.WorkTreeRoot, trust.GitDir, trust.CommonDir, trust.ObjectDir}, trust.AlternateObjectDirs...) {
		if strings.Contains(identity, source) || strings.Contains(identity, shared) || strings.Contains(identity, linked) {
			t.Fatalf("trust identity leaked local path %q", identity)
		}
	}
	rendered := renderHTML(got, &got)
	for _, phrase := range []string{"PATH-selected Git executable", "common object store", "configured object alternates", "does not independently authenticate source provenance"} {
		if !strings.Contains(rendered, phrase) {
			t.Fatalf("rendered linked-worktree inventory omitted trust phrase %q", phrase)
		}
	}
	for _, identity := range append([]string{trust.Executable, trust.WorkTreeRoot, trust.GitDir, trust.CommonDir, trust.ObjectDir}, trust.AlternateObjectDirs...) {
		if !strings.Contains(rendered, identity) {
			t.Fatalf("rendered linked-worktree inventory omitted trust identity %q", identity)
		}
	}
}

func TestMethodologyRejectsMissingOrModifiedGitTrustDisclosure(t *testing.T) {
	for _, disclosure := range []string{"", "Git output is authenticated."} {
		report := validReport()
		report.Methodology.GitEvidenceTrust = disclosure
		if err := validateReport(report); err == nil || !strings.Contains(err.Error(), "git_evidence_trust") {
			t.Fatalf("validateReport() error=%v, want fixed Git trust disclosure rejection", err)
		}
	}
}

func TestInventoryIgnoresGitReplacementObjectsAndInheritedRepositoryOverrides(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# Original\n\n**ORIGINAL-01** When an audit reads a pinned revision, the system shall use the original object.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "original")
	original := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, "one/SPEC.md", "# Forged\n\n**FORGED-01** When an audit reads a pinned revision, the system shall accept substituted content.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "replacement")
	replacement := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "replace", original, replacement)
	if substituted := gitTest(t, repo, "show", original+":one/SPEC.md"); !strings.Contains(substituted, "FORGED-01") {
		t.Fatalf("test replacement was not active: %q", substituted)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "attacker.git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())

	got, err := inventory(repo, "owner/repo", original)
	if err != nil {
		t.Fatal(err)
	}
	requirement := inventoryRequirement(t, got, "one/SPEC.md")
	if requirement.ID != "ORIGINAL-01" || strings.Contains(requirement.Excerpt, "FORGED-01") {
		t.Fatalf("replacement object changed pinned evidence: %#v", requirement)
	}
}

func TestInventoryMissingPromisorObjectDoesNotStartLazyFetch(t *testing.T) {
	repo := realTempDir(t)
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When an audit reads a pinned revision, the system shall fail closed if an object is absent.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "promisor fixture")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	missingBlob := strings.TrimSpace(gitTest(t, repo, "rev-parse", revision+":one/SPEC.md"))

	helper := filepath.Join(realTempDir(t), "local-promisor-transport")
	marker := helper + ".started"
	if err := os.WriteFile(helper, []byte("#!/bin/sh\n: > \"$0.started\"\nexit 86\n"), 0o755); err != nil {
		t.Fatalf("write local promisor transport: %v", err)
	}
	gitTest(t, repo, "config", "core.repositoryformatversion", "1")
	gitTest(t, repo, "config", "extensions.partialClone", "origin")
	gitTest(t, repo, "config", "remote.origin.promisor", "true")
	gitTest(t, repo, "config", "remote.origin.partialclonefilter", "blob:none")
	gitTest(t, repo, "config", "remote.origin.url", "ext::"+helper)
	gitTest(t, repo, "config", "protocol.ext.allow", "always")

	objectPath := filepath.Join(repo, ".git", "objects", missingBlob[:2], missingBlob[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatalf("remove local promisor object fixture %s: %v", missingBlob, err)
	}
	executable, err := trustedGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	controlEnv := make([]string, 0, len(cleanGitEnvironment(executable.Path())))
	for _, entry := range cleanGitEnvironment(executable.Path()) {
		if !strings.HasPrefix(entry, "GIT_NO_LAZY_FETCH=") {
			controlEnv = append(controlEnv, entry)
		}
	}
	controlCtx, controlCancel := context.WithTimeout(context.Background(), 2*time.Second)
	control := exec.CommandContext(controlCtx, executable.Path(), "--no-replace-objects", "-C", repo, "cat-file", "blob", missingBlob)
	control.Dir = repo
	control.Env = controlEnv
	control.Stdout = io.Discard
	control.Stderr = io.Discard
	control.WaitDelay = maxGitWaitDelay
	_ = control.Run()
	controlCancel()
	if _, err := os.Stat(marker); err != nil {
		t.Skipf("installed Git did not exercise the promisor transport control; deterministic command-policy coverage remains active: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatalf("reset local promisor transport marker: %v", err)
	}

	started := time.Now()
	_, err = inventoryWithLimits(repo, "owner/repo", revision, inventoryLimits{
		corpusBytes: maxCorpusBytes,
		corpusFiles: maxInventoryFiles,
		wallTime:    2 * time.Second,
	})
	if err == nil {
		t.Fatal("inventory accepted a pinned revision with an absent promisor object")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("missing-object failure exceeded bounded wall time: %s: %v", elapsed, err)
	}
	if !strings.Contains(err.Error(), missingBlob) || !strings.Contains(err.Error(), "missing object") {
		t.Fatalf("inventory error = %v, want bounded missing-object failure for %s", err, missingBlob)
	}
	if _, markerErr := os.Stat(marker); !os.IsNotExist(markerErr) {
		t.Fatalf("absent object started repository-configured promisor transport: %v", markerErr)
	}
}

func TestGitCommandPolicyDisablesLazyFetchAndAmbientRouting(t *testing.T) {
	helper := filepath.Join(realTempDir(t), "record-git-policy")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\"",
		"printf 'GIT_NO_LAZY_FETCH=%s\\n' \"${GIT_NO_LAZY_FETCH-}\"",
		"printf 'GIT_NO_REPLACE_OBJECTS=%s\\n' \"${GIT_NO_REPLACE_OBJECTS-}\"",
		"printf 'GIT_DIR=%s\\n' \"${GIT_DIR-}\"",
		"printf 'GIT_WORK_TREE=%s\\n' \"${GIT_WORK_TREE-}\"",
	}, "\n") + "\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "attacker.git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := gitBytesWithContext(ctx, gitExecutable{path: helper}, t.TempDir(), 4096, nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	policy := string(output)
	for _, want := range []string{
		"--no-replace-objects\n--no-lazy-fetch\n-C\n",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_DIR=\n",
		"GIT_WORK_TREE=\n",
	} {
		if !strings.Contains(policy, want) {
			t.Fatalf("Git command policy output %q omitted %q", policy, want)
		}
	}
}

func TestInventoryIgnoresAmbientGitRepositoryContext(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# Expected\n\n**EXPECTED-01** When an audit pins a repository, the system shall ignore ambient Git repository context.\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "expected repository")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	other := t.TempDir()
	gitTest(t, other, "init", "-q")
	gittest.HardenRepo(t, other)
	gitTest(t, other, "config", "user.email", "test@example.com")
	gitTest(t, other, "config", "user.name", "Test")
	writeTestFile(t, other, "other/SPEC.md", "# Wrong\n\n**WRONG-01** When ambient Git variables are inherited, the system shall read the wrong repository.\n")
	gitTest(t, other, "add", ".")
	gitTest(t, other, "commit", "-qm", "ambient repository")

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	got, err := inventory(repo, "owner/expected", revision)
	if err != nil {
		t.Fatal(err)
	}
	requirement := inventoryRequirement(t, got, "one/SPEC.md")
	if requirement.ID != "EXPECTED-01" || got.Snapshot.Repository != "owner/expected" {
		t.Fatalf("ambient Git context changed pinned evidence: snapshot=%#v requirement=%#v", got.Snapshot, requirement)
	}
}

func TestParseSpecCountsOnlyCanonicalRequirementIDs(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"**Scope:** This is metadata, not a requirement.",
		"**lower-01** When a request runs, the system shall ignore a non-canonical identifier.",
		"**NOHYPHEN** When a request runs, the system shall ignore an identifier without a separator.",
		"**GOOD-01** When a request runs, the system shall count the canonical requirement.",
	}, "\n"))

	if len(parsed.Requirements) != 1 {
		t.Fatalf("requirements=%d, want 1: %#v", len(parsed.Requirements), parsed.Requirements)
	}
	if parsed.Requirements[0].ID != "GOOD-01" {
		t.Fatalf("requirement ID=%q, want GOOD-01", parsed.Requirements[0].ID)
	}
}

func TestParseSpecInventoriesCanonicalMarkdownRequirementForms(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"**REQ-01** When a request runs, the system shall preserve the canonical ID.",
		"- **REQ-02** When a bullet runs, the system shall preserve the canonical ID.",
		"* __REQ-03__ When emphasis changes, the system shall preserve the canonical ID.",
		"+ `REQ-04` When inline code is used, the system shall preserve the canonical ID.",
		"1. REQ-05 When a numbered item runs, the system shall preserve the canonical ID.",
		"2) **REQ-06** When a parenthesized item runs, the system shall preserve the canonical ID.",
		"### **REQ-07** When a heading is used, the system shall preserve the canonical ID.",
	}, "\n"))

	wantIDs := []string{"REQ-01", "REQ-02", "REQ-03", "REQ-04", "REQ-05", "REQ-06", "REQ-07"}
	if len(parsed.Requirements) != len(wantIDs) {
		t.Fatalf("requirements=%d, want %d: %#v", len(parsed.Requirements), len(wantIDs), parsed.Requirements)
	}
	for index, wantID := range wantIDs {
		if got := parsed.Requirements[index]; got.ID != wantID || strings.HasPrefix(got.Body, wantID+" ") {
			t.Fatalf("requirement[%d]=%#v, want stable ID %q and body without ID", index, got, wantID)
		}
	}
	if len(parsed.Diagnostics) != 0 {
		t.Fatalf("canonical Markdown forms produced diagnostics: %#v", parsed.Diagnostics)
	}
}

func TestInventoryReportsFeatureFirstDiagnosticsFromPinnedObjects(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\n")
	writeTestFile(t, repo, "valid/SPEC.md", "# Valid\n\n**VALID-01** When traceability is valid, the system shall retain both links.\n\n## BDD Traceability\n\n- Feature: `test/bdd/valid.feature`\n")
	writeTestFile(t, repo, "one-sided/SPEC.md", "# One sided\n\n**ONE-SIDED-01** When traceability is one sided, the system shall diagnose it.\n")
	writeTestFile(t, repo, "first/SPEC.md", "# First\n\n**FIRST-01** When primary ownership is ambiguous, the system shall diagnose it.\n\n## BDD Traceability\n\n- Feature: `test/bdd/ambiguous.feature`\n")
	writeTestFile(t, repo, "second/SPEC.md", "# Second\n\n**SECOND-01** When primary ownership is ambiguous, the system shall diagnose it.\n\n## BDD Traceability\n\n- Feature: `test/bdd/ambiguous.feature`\n")
	writeTestFile(t, repo, "test/bdd/valid.feature", "# SPEC: valid/SPEC.md\nFeature: Valid\n")
	writeTestFile(t, repo, "test/bdd/zero.feature", "Feature: Missing link\n")
	writeTestFile(t, repo, "test/bdd/missing-spec.feature", "# SPEC: absent/SPEC.md\nFeature: Missing SPEC\n")
	writeTestFile(t, repo, "test/bdd/one-sided.feature", "# SPEC: one-sided/SPEC.md\nFeature: One sided\n")
	writeTestFile(t, repo, "test/bdd/malformed.feature", "# SPEC malformed/SPEC.md\nFeature: Malformed\n")
	writeTestFile(t, repo, "test/bdd/ambiguous.feature", "# SPEC: first/SPEC.md\n# SPEC: second/SPEC.md\nFeature: Ambiguous\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "feature traceability fixture")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, "one-sided/SPEC.md", "# Working tree only\n\n## BDD Traceability\n\n- Feature: `test/bdd/one-sided.feature`\n")
	writeTestFile(t, repo, "absent/SPEC.md", "# Working tree only\n\n## BDD Traceability\n\n- Feature: `test/bdd/missing-spec.feature`\n")

	got, err := inventory(repo, "owner/repo", revision)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"test/bdd/zero.feature":         "missing-feature-spec-reference",
		"test/bdd/missing-spec.feature": "missing-feature-spec",
		"test/bdd/one-sided.feature":    "nonreciprocal-feature-spec",
		"test/bdd/malformed.feature":    "malformed-feature-spec-reference",
		"test/bdd/ambiguous.feature":    "ambiguous-feature-spec-reference",
	}
	for path, wantKind := range want {
		feature := inventoryFeature(t, got, path)
		if len(feature.Diagnostics) != 1 || feature.Diagnostics[0].Kind != wantKind {
			t.Fatalf("%s diagnostics=%#v, want one %s", path, feature.Diagnostics, wantKind)
		}
	}
	validFeature := inventoryFeature(t, got, "test/bdd/valid.feature")
	if len(validFeature.Diagnostics) != 0 {
		t.Fatalf("valid reciprocal feature diagnostics=%#v, want none", validFeature.Diagnostics)
	}
	validJSON, err := json.Marshal(validFeature)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(validJSON, []byte(`"diagnostics"`)) {
		t.Fatalf("valid feature changed the v1 serialized shape: %s", validJSON)
	}
	if got.Summary.Diagnostics != len(want) {
		t.Fatalf("summary diagnostics=%d, want %d", got.Summary.Diagnostics, len(want))
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded report
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := validateReport(decoded); err != nil {
		t.Fatalf("feature diagnostics did not round-trip through v1 validation: %v", err)
	}
}

func TestParseSpecSkipsFencesAndRecordsUnidentifiedRequirements(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"```markdown",
		"**FENCED-01** When a sample runs, the system shall not count it.",
		"- Feature: `agm/test/bdd/features/fenced.feature`",
		"```",
		"",
		"When a requirement has no ID, the system shall record a diagnostic.",
		"**REAL-01** When the source is parsed, the system shall count real requirements.",
		"",
		"## BDD Traceability",
		"",
		"- Feature: `agm/test/bdd/features/real.feature`",
	}, "\n"))

	if len(parsed.Requirements) != 1 || parsed.Requirements[0].ID != "REAL-01" {
		t.Fatalf("requirements=%#v, want REAL-01 only", parsed.Requirements)
	}
	if len(parsed.Diagnostics) != 1 || parsed.Diagnostics[0].Kind != "anonymous-requirement" {
		t.Fatalf("diagnostics=%#v, want one anonymous requirement", parsed.Diagnostics)
	}
	if len(parsed.BDDFeatures) != 1 || parsed.BDDFeatures[0].Path != "agm/test/bdd/features/real.feature" {
		t.Fatalf("BDD features=%#v, want only the traceability-section feature", parsed.BDDFeatures)
	}
}

func TestParseSpecBDDTraceabilityRequiresExactBoundedEntries(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"- Feature: `outside.feature`",
		"",
		"## BDD Traceability",
		"- Feature: `test/bdd/valid.feature`",
		"- `test/bdd/described.feature` keeps the .feature suffix stable.",
		"- Related feature: `test/bdd/related.feature`",
		"- BDD: `test/bdd/bdd.feature`",
		"- BDD feature: `test/bdd/bdd-feature.feature`",
		"- BDD Tests: `test/bdd/bdd-tests.feature`",
		"- Command BDD: `test/bdd/command.feature`",
		"- Cross-surface BDD: `test/bdd/cross-bdd.feature`",
		"- Cross-surface contracts: `test/bdd/cross-contracts.feature`",
		"- Status BDD: `test/bdd/status.feature`",
		"- Strict-spec linkage: `test/bdd/strict-link.feature`",
		"- Strictness BDD: `test/bdd/strictness.feature`",
		"This prose mentions test/bdd/not-a-claim.feature without declaring it.",
		"- Feature: test/bdd/malformed.feature",
		"- Arbitrary prose: `test/bdd/not-supported.feature`",
		"- Feature: `test/bdd/valid.feature`",
		"### Traceability details",
		"- Feature: `test/bdd/nested-detail.feature`",
		"## Follow-on section",
		"- Feature: `test/bdd/outside-boundary.feature`",
		"",
		"## BDD Traceability",
		"- Feature: `test/bdd/second.feature`",
	}, "\n"))
	wantPaths := []string{
		"test/bdd/valid.feature",
		"test/bdd/described.feature",
		"test/bdd/related.feature",
		"test/bdd/bdd.feature",
		"test/bdd/bdd-feature.feature",
		"test/bdd/bdd-tests.feature",
		"test/bdd/command.feature",
		"test/bdd/cross-bdd.feature",
		"test/bdd/cross-contracts.feature",
		"test/bdd/status.feature",
		"test/bdd/strict-link.feature",
		"test/bdd/strictness.feature",
		"test/bdd/nested-detail.feature",
		"test/bdd/second.feature",
	}
	if len(parsed.BDDFeatures) != len(wantPaths) {
		t.Fatalf("BDD features=%#v, want %v", parsed.BDDFeatures, wantPaths)
	}
	for index, want := range wantPaths {
		if parsed.BDDFeatures[index].Path != want {
			t.Fatalf("BDD features=%#v, want %v", parsed.BDDFeatures, wantPaths)
		}
	}
	wantKinds := []string{"malformed-bdd-feature-reference", "malformed-bdd-feature-reference", "duplicate-bdd-feature-reference", "ambiguous-bdd-traceability-section"}
	if len(parsed.Diagnostics) != len(wantKinds) {
		t.Fatalf("BDD diagnostics=%#v, want %v", parsed.Diagnostics, wantKinds)
	}
	for index, want := range wantKinds {
		if parsed.Diagnostics[index].Kind != want {
			t.Fatalf("BDD diagnostics=%#v, want %v", parsed.Diagnostics, wantKinds)
		}
	}
}

func TestParseSpecRecognizesCurrentTraceabilitySectionHeadings(t *testing.T) {
	for _, heading := range []string{"BDD Traceability", "Test Traceability", "Package Test Traceability", "Traceability"} {
		t.Run(heading, func(t *testing.T) {
			parsed := parseSpec("example/SPEC.md", strings.Join([]string{
				"# Example",
				"## " + heading,
				"- BDD: `test/bdd/current.feature`",
			}, "\n"))
			if len(parsed.BDDFeatures) != 1 || parsed.BDDFeatures[0].Path != "test/bdd/current.feature" {
				t.Fatalf("BDD features=%#v, want current traceability form", parsed.BDDFeatures)
			}
			if len(parsed.Diagnostics) != 0 {
				t.Fatalf("diagnostics=%#v, want none", parsed.Diagnostics)
			}
		})
	}
}

func TestValidateRejectsInvalidPositiveFinding(t *testing.T) {
	report := validReport()
	report.Candidates = []finding{{
		ID: "SPEC-CLUSTER-001", Rank: 1, Title: "bad", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the behavior"}}, ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "already owns the neutral behavior"}, SharedOutcome: "same", MaterialDifferences: []string{"none observed"}, Evidence: []evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}},
		ApplicabilityBasis: "active-members", ApplicabilityRationale: "supported by the active member", Applicability: []applicability{{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}}}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "merge"}, Recommendation: []string{"merge"}, Risk: "bounded", Decision: "approve", DecisionStatus: pendingMaintainerApproval,
	}}
	report.Summary.CandidateCount = 1
	report.Summary.ByVerdict = map[string]int{"merge-now": 1}
	if err := validateReport(report); err == nil || !strings.Contains(err.Error(), "product SPEC current owners") {
		t.Fatalf("validateReport error=%v, want positive evidence rejection", err)
	}
}

func TestValidateFindingOwnerStateCoherence(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	candidateFinding := semanticReport.Candidates[0]
	active := map[string]bool{"codex-cli": true, "pi-cli": true}

	candidateFinding.ProposedOwner = &proposedOwnerClaim{Path: "shared/SPEC.md", State: "new", Rationale: "A new implemented shared seam is required.", NeutralityRationale: "The shared seam is outside harness configuration and owns only the shared observable."}
	candidateFinding.OwnershipPlan.Requirements[1].TargetPath = "shared/SPEC.md"
	candidateFinding.OwnershipPlan.Requirements[1].TargetRequirementID = "TWO-01"
	candidateFinding.OwnershipPlan.Requirements[1].TargetState = "planned"
	candidateFinding.OwnershipPlan.Features[1].TargetPath = "shared/SPEC.md"
	candidateFinding.OwnershipPlan.Features[1].TargetState = "planned"
	candidateFinding.OwnershipPlan.Features[3].TargetPath = "shared/SPEC.md"
	candidateFinding.OwnershipPlan.Features[3].TargetState = "planned"
	if err := validateFinding(candidateFinding, false, active); err != nil {
		t.Fatalf("new proposed owner should be valid: %v", err)
	}
	if err := validateAgainstInventory(report{
		SchemaVersion: schemaVersion,
		DocumentKind:  ledgerDocumentKind,
		InventoryRef:  semanticReport.InventoryRef,
		Snapshot:      semanticReport.Snapshot,
		Scope:         semanticReport.Scope,
		Summary:       semanticReport.Summary,
		Methodology:   semanticReport.Methodology,
		Candidates:    []finding{candidateFinding},
		NonCandidates: []finding{},
		Limitations:   semanticReport.Limitations,
	}, inventoryReport); err != nil {
		t.Fatalf("absent new owner should validate against pinned evidence: %v", err)
	}

	candidateFinding.Verdict = "insufficient-evidence"
	candidateFinding.Strength = "moderate"
	if err := validateFinding(candidateFinding, false, active); err == nil || !strings.Contains(err.Error(), "cannot select a canonical owner") {
		t.Fatalf("non-positive proposed owner error = %v", err)
	}
}

func TestValidateFindingRejectsNonProductPositiveOwners(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	base := semanticReport.Candidates[0]
	active := map[string]bool{"codex-cli": true, "pi-cli": true}

	for _, path := range []string{
		"testdata/owners/SPEC.md",
		"vendor/example/SPEC.md",
		"generated/owners/SPEC.md",
		"fixture/owners/SPEC.md",
	} {
		t.Run(path, func(t *testing.T) {
			finding := base
			finding.Classification = "fixture"
			finding.CurrentOwners = append([]ownerClaim(nil), base.CurrentOwners...)
			finding.CurrentOwners[0].Path = path
			finding.Evidence = append([]evidence(nil), base.Evidence...)
			finding.Evidence[0].Path = path
			if err := validateFinding(finding, false, active); err == nil || !strings.Contains(err.Error(), "product SPEC current owners") {
				t.Fatalf("validateFinding() error = %v, want product-owner rejection", err)
			}
		})
	}

	for _, path := range []string{
		"testdata/new/SPEC.md",
		"vendor/new/SPEC.md",
		"generated/new/SPEC.md",
		"fixture/new/SPEC.md",
	} {
		t.Run("proposed/"+path, func(t *testing.T) {
			finding := base
			finding.Classification = "fixture"
			finding.ProposedOwner = &proposedOwnerClaim{Path: path, State: "new", Rationale: "must become the product owner"}
			if err := validateFinding(finding, false, active); err == nil || !strings.Contains(err.Error(), "product SPEC proposed owner") {
				t.Fatalf("validateFinding() error = %v, want product-owner rejection", err)
			}
		})
	}
}

func TestRenderIsOfflineAndEscapesEvidence(t *testing.T) {
	report := validReport()
	report.NonCandidates = []finding{{
		ID: "SPEC-CLUSTER-002", Title: "native <adapter>", Verdict: "keep-separate", Relationship: "same-vocabulary-only", Classification: "capability-variation", Confidence: "confirmed", Strength: "moderate",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the native behavior"}}, SharedOutcome: "separate", MaterialDifferences: []string{"native path"}, Evidence: []evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: 2, RequirementID: "ONE-01", Excerpt: "<script>alert(1)</script>"}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "applicability-specific"}, Recommendation: []string{"keep it"}, Risk: "bounded", Limitations: []string{"sentinel limitation"}, Decision: "retain", Boundary: "native behavior differs",
	}}
	report.Summary.ByVerdict = map[string]int{"keep-separate": 1}
	report.Snapshot.ComparisonRevision = strings.Repeat("b", 40)
	report.Scope.Excluded = []exclusion{{Path: "vendor", Reason: "dependency corpus"}}
	output := renderHTML(report, nil)
	if strings.Contains(output, "src=\"http") || strings.Contains(output, "href=\"http") || strings.Contains(output, "<script>alert") {
		t.Fatalf("renderer leaked external runtime or unsafe evidence: %s", output)
	}
	if !strings.Contains(output, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("renderer did not escape evidence: %s", output)
	}
	for _, sentinel := range []string{"SPEC-CLUSTER-002", "same-vocabulary-only", "capability-variation", "one/SPEC.md", "native path", "agm/test/bdd/features/example.feature", "keep it", "sentinel limitation", "dependency corpus", strings.Repeat("b", 40)} {
		if !strings.Contains(output, sentinel) {
			t.Fatalf("renderer omitted %q", sentinel)
		}
	}
}

func TestRenderRetainsEscapedApplicabilityEvidenceRecords(t *testing.T) {
	report := validReport()
	applicabilityEvidence := evidence{
		Kind:          "normative-contract",
		Path:          "one/SPEC.md",
		Line:          17,
		RequirementID: "ONE-01",
		Excerpt:       `<img src=x onerror="alert('unsafe')">`,
	}
	report.Candidates = []finding{{
		ID: "SPEC-CLUSTER-001", Rank: 1, Title: "applicability evidence", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the behavior"}, {Path: "two/SPEC.md", Rationale: "also owns the behavior"}}, ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "owns the canonical behavior"},
		SharedOutcome: "same", MaterialDifferences: []string{"none observed"}, Evidence: []evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}, {Kind: "normative-contract", Path: "two/SPEC.md", Line: 1, RequirementID: "TWO-01", Excerpt: "two"}},
		ApplicabilityBasis: "active-members", ApplicabilityRationale: "both claims use the same pinned evidence", Applicability: []applicability{{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{applicabilityEvidence, applicabilityEvidence}}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "merge"}, Recommendation: []string{"merge"}, Risk: "bounded", Decision: "approve",
	}}
	report.Summary.CandidateCount = 1
	report.Summary.ByVerdict = map[string]int{"merge-now": 1}

	output := renderHTML(report, nil)
	const escapedExcerpt = "&lt;img src=x onerror=&#34;alert(&#39;unsafe&#39;)&#34;&gt;"
	if strings.Contains(output, `<img src=x onerror="alert('unsafe')">`) {
		t.Fatalf("renderer emitted unescaped applicability excerpt: %s", output)
	}
	if got := strings.Count(output, escapedExcerpt); got != 2 {
		t.Fatalf("escaped applicability excerpts=%d, want 2 to preserve duplicate records", got)
	}
	if got := strings.Count(output, "one/SPEC.md:17"); got != 2 {
		t.Fatalf("applicability path and line occurrences=%d, want 2", got)
	}
	if got := strings.Count(output, "one/SPEC.md:17</code> <span class=\"pill\">ONE-01</span><br>"); got != 2 {
		t.Fatalf("applicability requirement ID occurrences=%d, want 2", got)
	}
}

func TestCommandsRejectFilesystemOutputFlags(t *testing.T) {
	for _, args := range [][]string{
		{"inventory", "-output", "/tmp/inventory.json"},
		{"render", "-output", "/tmp/report.html"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "flag provided but not defined: -output") {
			t.Fatalf("run(%v)=%d stderr=%q, want output-flag rejection", args, code, stderr.String())
		}
	}
}

func TestInventoryValidateRenderPreserveTargetRepositoryState(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	writeTestFile(t, repo, "one/SPEC.md", "dirty working-tree bytes must survive the audit\n")
	writeTestFile(t, repo, ".beads/issue-state.json", "{\"state\":\"unchanged\"}\n")
	writeTestFile(t, repo, "wayfinder/delivery-state.json", "{\"phase\":\"unchanged\"}\n")
	before := snapshotRepositoryState(t, repo)

	var inventoryOutput, stderr bytes.Buffer
	if code := run([]string{
		"inventory",
		"-repo", repo,
		"-repository", semanticReport.Snapshot.Repository,
		"-revision", semanticReport.Snapshot.Revision,
	}, &inventoryOutput, &stderr); code != 0 {
		t.Fatalf("inventory=%d: %s", code, stderr.String())
	}

	inputDir := realTempDir(t)
	inventoryPath := filepath.Join(inputDir, "inventory.json")
	semanticPath := filepath.Join(inputDir, "findings.json")
	if err := os.WriteFile(inventoryPath, inventoryOutput.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := decisionLedgerFromReport(semanticReport, inventoryReport)
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, semanticPath, ledger)

	stderr.Reset()
	var validateOutput bytes.Buffer
	proofArgs := []string{"-input", semanticPath, "-inventory", inventoryPath, "-repo", repo}
	if code := run(append([]string{"validate"}, proofArgs...), &validateOutput, &stderr); code != 0 {
		t.Fatalf("validate=%d: %s", code, stderr.String())
	}
	if !strings.Contains(validateOutput.String(), "valid spec-audit/v2 decision ledger") {
		t.Fatalf("unexpected validate output: %s", validateOutput.String())
	}

	stderr.Reset()
	var renderOutput bytes.Buffer
	if code := run(append([]string{"render"}, proofArgs...), &renderOutput, &stderr); code != 0 {
		t.Fatalf("render=%d: %s", code, stderr.String())
	}
	if !strings.Contains(renderOutput.String(), "<!doctype html>") {
		t.Fatalf("render did not emit a complete HTML artifact")
	}

	after := snapshotRepositoryState(t, repo)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("target repository changed across inventory, validate, and render\nbefore=%#v\nafter=%#v", before, after)
	}
}

func snapshotRepositoryState(t *testing.T, root string) map[string]string {
	t.Helper()
	state := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			state[key] = "symlink:" + target
		case info.IsDir():
			state[key] = "dir:" + info.Mode().String()
		case info.Mode().IsRegular():
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			state[key] = fmt.Sprintf("file:%s:%x", info.Mode(), sha256.Sum256(body))
		default:
			state[key] = "other:" + info.Mode().String()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestValidatePinsFindingsToGitResolvedInventory(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	inputDir := realTempDir(t)
	inventoryPath := filepath.Join(inputDir, "inventory.json")
	reportPath := filepath.Join(inputDir, "findings.json")
	writeJSON(t, inventoryPath, inventoryDocumentFromReport(inventoryReport))
	ledger, err := decisionLedgerFromReport(semanticReport, inventoryReport)
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, reportPath, ledger)

	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-input", reportPath, "-inventory", inventoryPath, "-repo", repo}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("validate=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid spec-audit/v2 decision ledger") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	rendered := renderHTML(semanticReport, &inventoryReport)
	for _, sentinel := range []string{semanticReport.Candidates[0].CurrentOwners[0].Rationale, semanticReport.Candidates[0].OwnershipCompleteness, semanticReport.Candidates[0].ProposedOwner.Rationale, "one/SPEC.md:", "does not independently authenticate source provenance"} {
		if !strings.Contains(rendered, sentinel) {
			t.Fatalf("pinned-evidence renderer omitted ownership proof %q", sentinel)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"validate", "-input", reportPath}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "-inventory and -repo are required") {
		t.Fatalf("validate without proof=%d, stderr=%q", code, stderr.String())
	}
}

func TestReadReportRejectsSemanticNullInventoryPayload(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	data, err := json.Marshal(semanticReport)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["inventory"] = json.RawMessage("null")
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realTempDir(t), "semantic-with-null-inventory.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	decoded, err := readReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAgainstInventory(decoded, inventoryReport); err == nil || !strings.Contains(err.Error(), "must omit inventory, features, and seeds") {
		t.Fatalf("embedded null inventory error=%v, want semantic payload rejection", err)
	}
}

func TestPinnedValidationAcceptsBDDReciprocityAcrossFeatures(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{
		"agm/test/bdd/features/one-only.feature",
		"agm/test/bdd/features/two-only.feature",
	}

	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("semantic report should be structurally valid: %v", err)
	}
	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("inventory should match the recomputed pinned repository view: %v", err)
	}
	if err := validateAgainstInventory(semanticReport, inventoryReport); err != nil {
		t.Fatalf("owners covered across reciprocal features should pass: %v", err)
	}
}

func TestPinnedValidationAcceptsNonCandidateWithNoBDDAction(t *testing.T) {
	tests := []struct {
		name     string
		features []string
	}{
		{name: "no selected feature"},
		{name: "existing reciprocal feature", features: []string{"agm/test/bdd/features/shared.feature"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, inventoryReport, semanticReport := auditFixture(t)
			nonCandidate := semanticReport.Candidates[0]
			nonCandidate.Rank = 0
			nonCandidate.Verdict = "keep-separate"
			nonCandidate.ProposedOwner = nil
			nonCandidate.OwnershipPlan = nil
			nonCandidate.BDD.Features = test.features
			nonCandidate.BDD.Consequence = "none"
			nonCandidate.Boundary = "The observables remain separately owned."
			semanticReport.Candidates = nil
			semanticReport.NonCandidates = []finding{nonCandidate}
			semanticReport.Summary.CandidateCount = 0
			semanticReport.Summary.ByVerdict = map[string]int{"keep-separate": 1}

			if err := validateReport(semanticReport); err != nil {
				t.Fatalf("no-action non-candidate should be structurally valid: %v", err)
			}
			if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
				t.Fatalf("inventory should match the recomputed pinned repository view: %v", err)
			}
			if err := validateAgainstInventory(semanticReport, inventoryReport); err != nil {
				t.Fatalf("no-action non-candidate should validate against pinned evidence: %v", err)
			}
		})
	}
}

func TestPinnedValidationRejectsCurrentOwnerWithoutSelectedFeature(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{"agm/test/bdd/features/one-only.feature"}

	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("semantic report should be structurally valid: %v", err)
	}
	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("inventory should match the recomputed pinned repository view: %v", err)
	}
	err := validateAgainstInventory(semanticReport, inventoryReport)
	if err == nil || !strings.Contains(err.Error(), "do not reciprocally name current owner \"two/SPEC.md") {
		t.Fatalf("uncovered current-owner error=%v, want pinned owner-degree rejection", err)
	}
}

func TestPinnedValidationRejectsBDDFeatureWithoutCurrentOwner(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{
		"agm/test/bdd/features/shared.feature",
		"agm/test/bdd/features/three-only.feature",
	}

	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("semantic report should be structurally valid: %v", err)
	}
	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("inventory should match the recomputed pinned repository view: %v", err)
	}
	err := validateAgainstInventory(semanticReport, inventoryReport)
	if err == nil || !strings.Contains(err.Error(), "three-only.feature\" does not reciprocally name any current owner") {
		t.Fatalf("unrelated BDD feature error=%v, want current-owner rejection", err)
	}
}

func TestPinnedValidationRejectsForgedEvidenceAndUnsafeVerdicts(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)

	tests := []struct {
		name   string
		mutate func(*report, *report)
	}{
		{
			name: "forged inventory",
			mutate: func(_ *report, inventory *report) {
				inventory.Inventory[0].SHA256 = strings.Repeat("f", 64)
			},
		},
		{
			name: "semantic embeds forged inventory",
			mutate: func(semantic *report, _ *report) {
				semantic.Inventory = []specFile{{Path: "forged/SPEC.md", SHA256: strings.Repeat("f", 64)}}
			},
		},
		{
			name: "semantic embeds forged features",
			mutate: func(semantic *report, _ *report) {
				semantic.Features = []featureFile{{Path: "forged.feature", SHA256: strings.Repeat("f", 64), RelatedSpecs: []string{}}}
			},
		},
		{
			name: "semantic embeds forged seeds",
			mutate: func(semantic *report, _ *report) {
				semantic.Seeds = []seed{}
			},
		},
		{
			name: "fake evidence line",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Evidence[0].Line++
			},
		},
		{
			name: "omitted owner evidence",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Evidence = semantic.Candidates[0].Evidence[:1]
			},
		},
		{
			name: "omitted current owner declaration",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].CurrentOwners = semantic.Candidates[0].CurrentOwners[:1]
			},
		},
		{
			name: "unrelated existing proposed owner",
			mutate: func(semantic *report, inventory *report) {
				_ = inventoryRequirement(t, *inventory, "three/SPEC.md")
				semantic.Candidates[0].ProposedOwner = &proposedOwnerClaim{Path: "three/SPEC.md", State: "existing", Rationale: "plausible but unrelated"}
			},
		},
		{
			name: "blank current owner rationale",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].CurrentOwners[0].Rationale = ""
			},
		},
		{
			name: "missing ownership completeness",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].OwnershipCompleteness = ""
			},
		},
		{
			name: "blank proposed owner rationale",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].ProposedOwner.Rationale = ""
			},
		},
		{
			name: "current owner marked new",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].ProposedOwner.State = "new"
			},
		},
		{
			name: "incomplete active matrix",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Applicability = semantic.Candidates[0].Applicability[:1]
			},
		},
		{
			name: "legacy shared disposition",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Applicability[0].Disposition = "shared"
			},
		},
		{
			name: "unknown applicability on positive finding",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Applicability[0].Disposition = "unknown"
			},
		},
		{
			name: "tentative positive",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Confidence = "tentative"
				semantic.Candidates[0].Strength = "moderate"
			},
		},
		{
			name: "missing rank",
			mutate: func(semantic *report, _ *report) {
				semantic.Candidates[0].Rank = 0
			},
		},
		{
			name: "one sided BDD",
			mutate: func(_ *report, inventory *report) {
				for index := range inventory.Features {
					if inventory.Features[index].Path == "agm/test/bdd/features/shared.feature" {
						inventory.Features[index].RelatedSpecs = []string{"one/SPEC.md"}
						return
					}
				}
				t.Fatal("shared feature missing from pinned fixture")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			semantic := cloneReport(t, semanticReport)
			inventory := cloneReport(t, inventoryReport)
			test.mutate(&semantic, &inventory)
			if err := validateReport(semantic); err != nil {
				return
			}
			if err := validateAgainstInventory(semantic, inventory); err != nil {
				return
			}
			if err := validateInventoryAgainstRepo(inventory, repo); err != nil {
				return
			}
			t.Fatal("unsafe mutation passed pinned-evidence validation")
		})
	}
}

func TestValidateReportRejectsImplementationOnlyHarnessCatalogOwners(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	for _, ownerPath := range []string{
		"agm/.claude-plugin/SPEC.md",
		"wayfinder/.claude-plugin/SPEC.md",
		"spec-governance/.claude-plugin/SPEC.md",
	} {
		t.Run(ownerPath, func(t *testing.T) {
			report := cloneReport(t, semanticReport)
			report.Candidates[0].CurrentOwners[0].Path = ownerPath
			report.Candidates[0].Evidence[0].Path = ownerPath
			report.Candidates[0].ProposedOwner.Path = ownerPath
			report.Candidates[0].ApplicabilityBasis = "non-harness-domain"
			report.Candidates[0].ApplicabilityRationale = "claimed to exclude every active harness"
			report.Candidates[0].Applicability = nil

			err := validateReport(report)
			if err == nil || !strings.Contains(err.Error(), "includes a harness registration owner") {
				t.Fatalf("validateReport() error = %v, want harness-owner applicability rejection", err)
			}
		})
	}
	t.Run("new proposed nested harness owner", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		report.Candidates[0].ProposedOwner = &proposedOwnerClaim{
			Path:                "wayfinder/.claude-plugin/new/SPEC.md",
			State:               "new",
			Rationale:           "claimed to create a non-harness-domain owner",
			NeutralityRationale: "claimed neutral owner",
		}
		report.Candidates[0].ApplicabilityBasis = "non-harness-domain"
		report.Candidates[0].ApplicabilityRationale = "claimed to exclude every active harness"
		report.Candidates[0].Applicability = nil

		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "includes a harness registration owner") {
			t.Fatalf("validateReport() error = %v, want proposed harness-owner applicability rejection", err)
		}
	})
}

func auditFixture(t *testing.T) (string, report, report) {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\n")
	writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When a request runs, the system shall preserve identity.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/shared.feature`\n- Feature: `agm/test/bdd/features/one-only.feature`\n")
	writeTestFile(t, repo, "two/SPEC.md", "# Two\n\n**TWO-01** When a request runs, the system shall preserve identity.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/shared.feature`\n- Feature: `agm/test/bdd/features/two-only.feature`\n")
	writeTestFile(t, repo, "three/SPEC.md", "# Three\n\n**THREE-01** When a separate request runs, the system shall emit an unrelated metric.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/three-only.feature`\n")
	writeTestFile(t, repo, "agm/test/bdd/features/shared.feature", "# SPEC: one/SPEC.md\n# RELATED-SPEC: two/SPEC.md\nFeature: Shared identity\n")
	writeTestFile(t, repo, "agm/test/bdd/features/one-only.feature", "# SPEC: one/SPEC.md\nFeature: One-only identity\n")
	writeTestFile(t, repo, "agm/test/bdd/features/two-only.feature", "# SPEC: two/SPEC.md\nFeature: Two-only identity\n")
	writeTestFile(t, repo, "agm/test/bdd/features/three-only.feature", "# SPEC: three/SPEC.md\nFeature: Three-only metric\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "fixture")
	revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	inventoryReport, err := inventory(repo, "owner/repo", revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventoryReport.Inventory) != 3 {
		t.Fatalf("inventory files=%d, want 3", len(inventoryReport.Inventory))
	}
	first := inventoryRequirement(t, inventoryReport, "one/SPEC.md")
	second := inventoryRequirement(t, inventoryReport, "two/SPEC.md")
	fixtureApplicability := []applicability{
		{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}}},
		{Member: "pi-cli", Disposition: "supported", Evidence: []evidence{{Kind: "normative-contract", Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}}},
	}
	fixturePlan := &ownershipPlan{
		Status:                 pendingMaintainerApproval,
		ApplicabilityBasis:     "active-members",
		ApplicabilityRationale: "The shared contract applies to both pinned active members.",
		OwnerActions: []ownerPreservation{
			{OwnerPath: "one/SPEC.md", Disposition: "retain-distinct-contract", Rationale: "The selected owner retains the shared contract during maintainer review."},
			{OwnerPath: "two/SPEC.md", Disposition: "retire-normative-ownership", Rationale: "The duplicate normative ownership remains preserved until a maintainer-approved transfer."},
		},
		Requirements: []requirementPreservation{
			{ContractEvidence: contractEvidence{Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}, Disposition: "retain-distinct", TargetPath: "one/SPEC.md", TargetRequirementID: first.ID, TargetState: "existing", Rationale: "Preserve the selected owner requirement."},
			{ContractEvidence: contractEvidence{Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}, Disposition: "transfer-to-proposed-owner", TargetPath: "one/SPEC.md", TargetRequirementID: first.ID, TargetState: "existing", Rationale: "Transfer only after maintainer approval."},
		},
		Features: []featurePreservation{
			{SourceOwner: "one/SPEC.md", Path: "agm/test/bdd/features/shared.feature", Disposition: "retain-distinct", TargetPath: "one/SPEC.md", TargetState: "existing", Rationale: "Preserve the shared reciprocal BDD feature for the selected owner."},
			{SourceOwner: "two/SPEC.md", Path: "agm/test/bdd/features/shared.feature", Disposition: "transfer-to-proposed-owner", TargetPath: "one/SPEC.md", TargetState: "existing", Rationale: "Preserve the shared reciprocal BDD feature for the retiring owner."},
			{SourceOwner: "one/SPEC.md", Path: "agm/test/bdd/features/one-only.feature", Disposition: "retain-distinct", TargetPath: "one/SPEC.md", TargetState: "existing", Rationale: "Preserve the one-owner reciprocal BDD feature."},
			{SourceOwner: "two/SPEC.md", Path: "agm/test/bdd/features/two-only.feature", Disposition: "transfer-to-proposed-owner", TargetPath: "one/SPEC.md", TargetState: "planned", Rationale: "Transfer only after maintainer approval."},
		},
		Applicability: fixtureApplicability,
	}
	semantic := report{
		SchemaVersion: schemaVersion,
		DocumentKind:  ledgerDocumentKind,
		Snapshot: snapshot{
			Repository:          inventoryReport.Snapshot.Repository,
			Revision:            inventoryReport.Snapshot.Revision,
			RevisionCommittedAt: inventoryReport.Snapshot.RevisionCommittedAt,
			GeneratedAt:         "2026-07-31T12:00:00Z",
		},
		Scope: inventoryReport.Scope,
		Summary: summary{
			SpecFiles: 3, Requirements: 3, Diagnostics: 0, CandidateCount: 1,
			ByVerdict: map[string]int{"merge-now": 1},
		},
		Methodology: methodology{Collector: "go run ./tools/specaudit inventory", SeedKinds: []string{"exact-body"}, SemanticReview: "source and BDD review", GitEvidenceTrust: gitEvidenceTrustDisclosure, GitTrustInputs: inventoryReport.Methodology.GitTrustInputs, Reproduce: []string{"go run ./tools/specaudit validate fixture"}},
		Candidates: []finding{{
			ID: "SPEC-CLUSTER-001", Rank: 1, Title: "Shared identity", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
			CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "ONE-01 normatively claims the shared request outcome."}, {Path: "two/SPEC.md", Rationale: "TWO-01 independently claims the same request outcome."}}, OwnershipCompleteness: "The exact-body seed and repository search found only these two normative paths.",
			ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "ONE-01 already states the complete shared observable.", NeutralityRationale: "The selected owner is a product-domain contract rather than a harness configuration surface."},
			SharedOutcome: "Requests preserve identity.", MaterialDifferences: []string{"Only the owner path differs."}, Evidence: []evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}, {Kind: "normative-contract", Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}},
			ApplicabilityBasis: "active-members", ApplicabilityRationale: "The shared contract applies to both pinned active members.",
			Applicability: fixtureApplicability,
			BDD:           bddImpact{Features: []string{"agm/test/bdd/features/shared.feature"}, Consequence: "merge"}, Recommendation: []string{"Keep ONE-01 as canonical."}, Risk: "Traceability could be lost.", Decision: "Approve one owner.", DecisionStatus: pendingMaintainerApproval, OwnershipPlan: fixturePlan,
		}},
		NonCandidates: []finding{}, Limitations: append([]string{}, inventoryReport.Limitations...),
	}
	ref, err := canonicalInventoryRef(inventoryReport)
	if err != nil {
		t.Fatal(err)
	}
	semantic.InventoryRef = ref
	return repo, inventoryReport, semantic
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func cloneReport(t *testing.T, source report) report {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var cloned report
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	for index := range cloned.Candidates {
		if index < len(source.Candidates) {
			cloned.Candidates[index].Evidence = append([]evidence{}, source.Candidates[index].Evidence...)
			for appIndex := range cloned.Candidates[index].Applicability {
				cloned.Candidates[index].Applicability[appIndex].Evidence = append([]evidence{}, source.Candidates[index].Applicability[appIndex].Evidence...)
			}
			cloneOwnershipPlanEvidence(&cloned.Candidates[index], source.Candidates[index])
		}
	}
	for index := range cloned.NonCandidates {
		if index < len(source.NonCandidates) {
			cloned.NonCandidates[index].Evidence = append([]evidence{}, source.NonCandidates[index].Evidence...)
			for appIndex := range cloned.NonCandidates[index].Applicability {
				cloned.NonCandidates[index].Applicability[appIndex].Evidence = append([]evidence{}, source.NonCandidates[index].Applicability[appIndex].Evidence...)
			}
			cloneOwnershipPlanEvidence(&cloned.NonCandidates[index], source.NonCandidates[index])
		}
	}
	return cloned
}

func cloneOwnershipPlanEvidence(destination *finding, source finding) {
	if destination.OwnershipPlan == nil || source.OwnershipPlan == nil {
		return
	}
	for index := range destination.OwnershipPlan.Applicability {
		destination.OwnershipPlan.Applicability[index].Evidence = append([]evidence{}, source.OwnershipPlan.Applicability[index].Evidence...)
	}
}

func inventoryRequirement(t *testing.T, source report, path string) requirement {
	t.Helper()
	for _, file := range source.Inventory {
		if file.Path == path && len(file.Requirements) > 0 {
			return file.Requirements[0]
		}
	}
	t.Fatalf("inventory has no requirement for %s", path)
	return requirement{}
}

func inventoryFeature(t *testing.T, source report, path string) featureFile {
	t.Helper()
	for _, feature := range source.Features {
		if feature.Path == path {
			return feature
		}
	}
	t.Fatalf("inventory has no feature for %s", path)
	return featureFile{}
}

func validReport() report {
	return report{
		SchemaVersion: schemaVersion,
		DocumentKind:  ledgerDocumentKind,
		InventoryRef:  "sha256:" + strings.Repeat("c", 64),
		Snapshot:      snapshot{Repository: "owner/repo", Revision: strings.Repeat("a", 40), RevisionCommittedAt: "2026-07-30T00:00:00Z", GeneratedAt: "2026-07-31T00:00:00Z"},
		Scope:         scope{Roots: []string{"."}, Excluded: []exclusion{}, ActiveMembers: []string{"codex-cli"}},
		Summary:       summary{SpecFiles: 1, Requirements: 1, CandidateCount: 0, ByVerdict: map[string]int{}},
		Methodology:   methodology{Collector: "test", SeedKinds: []string{"exact-body"}, SemanticReview: "review", GitEvidenceTrust: gitEvidenceTrustDisclosure, GitTrustInputs: testGitTrustInputs(), Reproduce: []string{"go run ./tools/specaudit inventory -repo . -revision abc"}},
		Candidates:    nil, NonCandidates: nil, Limitations: []string{},
	}
}

func TestV2InventoryAndDecisionLedger(t *testing.T) {
	_, inventoryReport, review := auditFixture(t)
	inventoryDocument := inventoryDocumentFromReport(inventoryReport)
	ledger, err := decisionLedgerFromReport(review, inventoryReport)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateInventoryDocumentV2(inventoryDocument); err != nil {
		t.Fatalf("v2 inventory document rejected: %v", err)
	}
	if err := validateDecisionLedgerV2(ledger); err != nil {
		t.Fatalf("v2 decision ledger rejected: %v", err)
	}
	forged := ledger
	forged.InventoryRef = "sha256:" + strings.Repeat("f", 64)
	if err := validateDecisionLedgerV2(forged); err != nil {
		t.Fatalf("structurally valid forged ref should require inventory comparison: %v", err)
	}
	ref, err := canonicalInventoryRefV2(inventoryDocument)
	if err != nil || forged.InventoryRef == ref {
		t.Fatalf("forged inventory_ref error=%v, want digest rejection", err)
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["inventory"] = json.RawMessage("[]")
	data, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realTempDir(t), "ledger-with-inventory.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readDecisionLedgerV2(path); err == nil || !strings.Contains(err.Error(), "non-exact or unknown") {
		t.Fatalf("ledger inventory copy error=%v, want strict separation rejection", err)
	}
	legacy := ledger
	legacy.SchemaVersion = "spec-audit/v1"
	path = filepath.Join(realTempDir(t), "legacy-v1-ledger.json")
	writeJSON(t, path, legacy)
	if _, err := readDecisionLedgerV2(path); err == nil || !strings.Contains(err.Error(), "spec-audit/v2") {
		t.Fatalf("legacy v1 error=%v, want breaking-schema rejection", err)
	}
	data, err = json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["Schema_Version"] = object["schema_version"]
	delete(object, "schema_version")
	data, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(realTempDir(t), "case-mismatched-ledger.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readDecisionLedgerV2(path); err == nil || !strings.Contains(err.Error(), "non-exact") {
		t.Fatalf("case-mismatched ledger error=%v, want exact-name rejection", err)
	}
}

func TestValidateAndRenderPlatformApplicability(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		if err := authenticatedInputPlatform(goos); err != nil {
			t.Fatalf("%s should support descriptor-authenticated input: %v", goos, err)
		}
	}
	if err := authenticatedInputPlatform("windows"); err == nil || !strings.Contains(err.Error(), "before") && !strings.Contains(err.Error(), "supported only") {
		t.Fatalf("windows platform error=%v, want explicit unsupported boundary", err)
	}
}

func TestEvidenceKindsKeepSupportingCitationsNonNormative(t *testing.T) {
	if err := validateEvidence([]evidence{{Kind: "supporting", Path: "internal/probe.go", Line: 2, Excerpt: "probe"}}); err != nil {
		t.Fatalf("supporting evidence rejected: %v", err)
	}
	if err := validateEvidence([]evidence{{Kind: "supporting", Path: "internal/probe.go", Line: 2, RequirementID: "REQ-01", Excerpt: "probe"}}); err == nil {
		t.Fatal("supporting evidence with requirement_id was accepted")
	}
	if err := validateEvidence([]evidence{{Kind: "normative-contract", Path: "one/SPEC.md", Line: 2, Excerpt: "requirement"}}); err == nil {
		t.Fatal("normative evidence without requirement_id was accepted")
	}
}

func TestPinnedInventoryRejectsSymlinkModes(t *testing.T) {
	for _, mode := range []string{"120000", "160000"} {
		if _, err := pinnedGitBlobSize([]string{mode, "blob", strings.Repeat("a", 40), "12"}, "unsafe/SPEC.md"); err == nil {
			t.Fatalf("pinnedGitBlobSize accepted non-regular mode %q", mode)
		}
	}
	if _, err := pinnedGitBlobSize([]string{"100644", "blob", strings.Repeat("a", 40), "12"}, "safe/SPEC.md"); err != nil {
		t.Fatalf("pinnedGitBlobSize rejected regular blob: %v", err)
	}
}

func TestActiveMembersRequirePackageASTDeclaration(t *testing.T) {
	active, limitations := activeMembersFromBody(`package registry
// activeHarnesses = []string{"comment-only"}
const example = "activeHarnesses = []string{\\\"literal-only\\\"}"
`, true)
	if len(active) != 0 || len(limitations) == 0 {
		t.Fatalf("comment/literal registry parsed as active=%#v limitations=%#v", active, limitations)
	}
	active, limitations = activeMembersFromBody(`package registry
var activeHarnesses = []string{"pi-cli", "codex-cli"}
`, true)
	if !reflect.DeepEqual(active, []string{"codex-cli", "pi-cli"}) || len(limitations) != 0 {
		t.Fatalf("AST registry active=%#v limitations=%#v", active, limitations)
	}
}

func TestSupportingEvidenceRecordBudgetCountsDuplicates(t *testing.T) {
	items := make([]evidence, maxSupportingEvidenceRecords+1)
	for index := range items {
		items[index] = evidence{Kind: "supporting", Path: "internal/probe.go", Line: 1, Excerpt: "probe"}
	}
	_, err := supportingEvidenceRecords(report{Candidates: []finding{{ID: "FINDING-1", Evidence: items}}})
	if err == nil || !strings.Contains(err.Error(), "record limit") {
		t.Fatalf("supporting duplicate budget error=%v, want record-limit rejection", err)
	}
}

func TestSupportingEvidenceResolvesPinnedRegularBlob(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "internal/probe.go", "first\nexact pinned line\n")
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "supporting evidence")
	revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))
	item := evidence{Kind: "supporting", Path: "internal/probe.go", Line: 2, Excerpt: "exact pinned line"}
	if err := validatePinnedSupportingEvidence(repository, revision, item); err != nil {
		t.Fatalf("validatePinnedSupportingEvidence() error = %v", err)
	}
}

func TestSupportingEvidenceBodiesHonorSharedDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := validateSupportingEvidenceBodies(ctx, []supportingEvidenceRecord{{findingID: "finding FINDING-1", item: evidence{Kind: "supporting", Path: "internal/probe.go", Line: 1, Excerpt: "line"}}}, map[string]pinnedBlob{"internal/probe.go": {path: "internal/probe.go", oid: strings.Repeat("a", 40)}}, map[string][]byte{strings.Repeat("a", 40): []byte("line\n")})
	if err == nil || !strings.Contains(err.Error(), "shared deadline") {
		t.Fatalf("supporting shared deadline error=%v, want bounded rejection", err)
	}
}

func TestSupportingMetadataBudgetAllowsLongLiteralPaths(t *testing.T) {
	paths := make([]string, maxSupportingEvidencePaths)
	for index := range paths {
		paths[index] = strings.Repeat("x", maxGitPathBytes-4) + fmt.Sprintf("%04d", index)
	}
	limit, err := supportingMetadataOutputLimit(paths)
	if err != nil || limit <= 0 {
		t.Fatalf("long literal supporting paths output limit=%d err=%v", limit, err)
	}
}

func TestPositiveFindingRejectsHarnessRegistrationRegardlessOfApplicabilityBasis(t *testing.T) {
	_, _, semantic := auditFixture(t)
	semantic.Candidates[0].CurrentOwners[0].Path = "agm/.claude-plugin/SPEC.md"
	semantic.Candidates[0].Evidence[0].Path = "agm/.claude-plugin/SPEC.md"
	semantic.Candidates[0].ApplicabilityBasis = "active-members"
	semantic.Candidates[0].ApplicabilityRationale = "all active members are explicitly listed"
	if err := validateReport(semantic); err == nil || !strings.Contains(err.Error(), "harness registration owner") {
		t.Fatalf("active-members harness owner error=%v, want unconditional rejection", err)
	}
}

func TestOwnershipPlanRequiresEveryRequirementAndPerOwnerBDDLink(t *testing.T) {
	repo, inventory, semantic := auditFixture(t)
	semantic.Candidates[0].OwnershipPlan.Requirements = semantic.Candidates[0].OwnershipPlan.Requirements[:1]
	if err := validateAgainstInventory(semantic, inventory); err == nil || !strings.Contains(err.Error(), "every current-owner requirement") {
		t.Fatalf("missing requirement plan error=%v", err)
	}
	_, inventory, semantic = auditFixture(t)
	semantic.Candidates[0].OwnershipPlan.Features = semantic.Candidates[0].OwnershipPlan.Features[:1]
	if err := validateAgainstInventory(semantic, inventory); err == nil || !strings.Contains(err.Error(), "reciprocal current-owner BDD") {
		t.Fatalf("missing BDD link plan error=%v", err)
	}
	if repo == "" {
		t.Fatal("fixture repository unexpectedly empty")
	}
}

func TestOwnershipPlanCopiesApplicabilityAndPendingDecisionStatus(t *testing.T) {
	_, inventory, semantic := auditFixture(t)
	semantic.Candidates[0].OwnershipPlan.ApplicabilityBasis = "non-harness-domain"
	if err := validateAgainstInventory(semantic, inventory); err == nil || !strings.Contains(err.Error(), "applicability basis") {
		t.Fatalf("plan applicability drift error=%v", err)
	}
	_, _, semantic = auditFixture(t)
	semantic.Candidates[0].DecisionStatus = "approved"
	if err := validateReport(semantic); err == nil || !strings.Contains(err.Error(), "decision_status") {
		t.Fatalf("closed decision status error=%v", err)
	}
}

func TestHTMLRendersPendingDecisionAndOwnershipPreservationPlan(t *testing.T) {
	_, inventory, semantic := auditFixture(t)
	html := renderHTML(semantic, &inventory)
	for _, want := range []string{"pending-maintainer-approval", "Maintainer-pending ownership preservation plan", "not deletion authority", "Requirement preservation", "BDD preservation"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered HTML omitted %q", want)
		}
	}
}

func TestReviewerExclusionsResolveAndCannotSelectProposedOwner(t *testing.T) {
	_, inventory, semantic := auditFixture(t)
	finding := &semantic.Candidates[0]
	finding.ProposedOwner = &proposedOwnerClaim{Path: "three/SPEC.md", State: "existing", Rationale: "Existing neutral owner is under review.", NeutralityRationale: "It is a product-domain owner."}
	finding.OwnershipPlan.Requirements[1].TargetPath = "three/SPEC.md"
	finding.OwnershipPlan.Requirements[1].TargetRequirementID = "THREE-01"
	finding.OwnershipPlan.Features[1].TargetPath = "three/SPEC.md"
	finding.OwnershipPlan.Features[3].TargetPath = "three/SPEC.md"
	semantic.Exclusions = []reviewExclusion{{Path: "three/SPEC.md", Classification: "fixture", Rationale: "Test reviewer exclusion remains collected.", SupportingEvidence: []supportingEvidence{{Path: "three/SPEC.md", Line: 1, Excerpt: "# Three"}}}}
	if err := validateAgainstInventory(semantic, inventory); err == nil || !strings.Contains(err.Error(), "reviewer-excluded proposed owner") {
		t.Fatalf("excluded proposed owner error=%v", err)
	}
	semantic.Exclusions[0].Path = "missing/SPEC.md"
	if err := validateAgainstInventory(semantic, inventory); err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("unresolved exclusion error=%v", err)
	}
}

func TestValidPathRejectsUnsafeGitPathspecForms(t *testing.T) {
	for _, path := range []string{"dir\\file", "dir/../file", "dir/\nfile", "dir/\x7ffile", "../file"} {
		if validPath(path) {
			t.Fatalf("validPath(%q) accepted unsafe path", path)
		}
	}
	if !validPath("safe/path.go") {
		t.Fatal("validPath rejected canonical relative path")
	}
}

func TestCollectorExecutionAvailabilityIsTruthful(t *testing.T) {
	base := collectorExecution{GoToolchain: "go1.26", GOOS: "linux", GOARCH: "amd64"}
	if !validCollectorExecution(base) {
		t.Fatal("collector execution without build info should be valid and explicit")
	}
	forgedBuild := base
	forgedBuild.BuildInfoAvailable = true
	if validCollectorExecution(forgedBuild) {
		t.Fatal("collector execution accepted available build info without module path")
	}
	forgedVCS := base
	forgedVCS.VCSMetadataAvailable = true
	if validCollectorExecution(forgedVCS) {
		t.Fatal("collector execution accepted available VCS metadata without complete values")
	}
	modified := false
	complete := base
	complete.BuildInfoAvailable = true
	complete.ModulePath = "example/module"
	complete.VCSMetadataAvailable = true
	complete.VCSRevision = strings.Repeat("a", 40)
	complete.VCSModified = &modified
	if !validCollectorExecution(complete) {
		t.Fatal("collector execution rejected complete truthful build metadata")
	}
}

func testGitTrustInputs() gitTrustInputs {
	identity := "sha256:" + strings.Repeat("a", 64)
	pathIdentity := "path-sha256:" + strings.Repeat("b", 64)
	return gitTrustInputs{Executable: identity, WorkTreeRoot: pathIdentity, GitDir: pathIdentity, CommonDir: pathIdentity, ObjectDir: pathIdentity, AlternateObjectDirs: []string{}}
}

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.Run(t, dir, args...)
}

func writeTestFile(t *testing.T, root, path, data string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
