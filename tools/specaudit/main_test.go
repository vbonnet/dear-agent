package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/messages/go/v21"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

const testMarketplaceMetadata = "package marketplaceparity\nconst (\n NeutralCatalogPath = \".dear-agent/marketplace.json\"\n ClaudeCatalogPath = \".claude-plugin/marketplace.json\"\n)\n"
const testHarnessAliasMetadata = "package agent\nfunc NormalizeHarnessName(name string) string { switch name { default: return name } }\n"

func writeTestAliasAndConfigMetadata(t *testing.T, repo string, active, deprecated []string) {
	t.Helper()
	writeTestFile(t, repo, harnessAliasSourcePath, testHarnessAliasMetadata)
	deprecatedSet := stringSet(deprecated)
	directories := map[string]string{
		"agy":          ".agents",
		"claude-code":  ".claude",
		"codex-cli":    ".codex",
		"gemini-cli":   ".gemini",
		"opencode-cli": ".opencode",
		"pi-cli":       ".pi",
	}
	members := append(append([]string(nil), active...), deprecated...)
	sort.Strings(members)
	var body strings.Builder
	body.WriteString("package configdirparity\nfunc SurfaceForHarness(harness string) (DirectorySurface, bool) {\n switch agent.NormalizeHarnessName(harness) {\n")
	for _, member := range members {
		directory, ok := directories[member]
		if !ok {
			t.Fatalf("test metadata has no config directory for %q", member)
		}
		fmt.Fprintf(&body, " case %q: return DirectorySurface{Harness: %q, Directory: %q, Deprecated: %t, Purpose: %q}, true\n", member, member, directory, deprecatedSet[member], member+" config")
	}
	body.WriteString(" default: return DirectorySurface{}, false\n }\n}\n")
	writeTestFile(t, repo, harnessConfigSurfaceSourcePath, body.String())
	writeTestFile(t, repo, openAIAdapterSourcePath, "package agent\ntype OpenAIAdapter struct{}\nfunc (a *OpenAIAdapter) Name() string { return \"openai\" }\n")
}

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

func TestPinnedBlobBatchRejectsHugeDeclaredSizeBeforeAllocation(t *testing.T) {
	oid := strings.Repeat("a", 40)
	declared := strconv.FormatInt(int64(^uint64(0)>>1), 10)
	output := fmt.Appendf(nil, "%s blob %s\n", oid, declared)
	_, err := parsePinnedBlobBatch(output, []pinnedBlob{{
		path: "huge/SPEC.md",
		oid:  oid,
		size: int64(^uint64(0) >> 1),
	}})
	if err == nil || !strings.Contains(err.Error(), "unexpected size") {
		t.Fatalf("parsePinnedBlobBatch() error = %v, want bounded size rejection", err)
	}
}

func TestInventoryRejectsDistinctInvalidUTF8SelectedPathsWithoutCollapsingArtifact(t *testing.T) {
	paths := [][]byte{
		{'b', 'a', 'd', '-', 0xfe, '/', 'S', 'P', 'E', 'C', '.', 'm', 'd'},
		{'b', 'a', 'd', '-', 0xff, '/', 'S', 'P', 'E', 'C', '.', 'm', 'd'},
	}
	if bytes.Equal(paths[0], paths[1]) || !selectedPinnedPathBytes(paths[0]) || !selectedPinnedPathBytes(paths[1]) {
		t.Fatalf("fixture paths are not distinct selected raw SPEC paths: %q %q", paths[0], paths[1])
	}
	var tree bytes.Buffer
	for index, path := range paths {
		oid := strings.Repeat(strconv.Itoa(index+1), 40)
		fmt.Fprintf(&tree, "100644 blob %s 20\t", oid)
		tree.Write(path)
		tree.WriteByte(0)
	}
	budget := corpusBudget{byteLimit: maxCorpusBytes, fileLimit: maxInventoryFiles}
	blobs, err := selectPinnedBlobs(tree.Bytes(), &budget)
	if err == nil || len(blobs) != 0 || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("selectPinnedBlobs() blobs=%#v error=%v, want both raw paths rejected before a replacement-character artifact", blobs, err)
	}
}

func TestInventoryRejectsInvalidUTF8SelectedBlobBodiesWithoutArtifact(t *testing.T) {
	for _, path := range []string{"one/SPEC.md", "test/bdd/invalid.feature"} {
		t.Run(path, func(t *testing.T) {
			repository := t.TempDir()
			gitTest(t, repository, "init", "-q")
			gittest.HardenRepo(t, repository)
			gitTest(t, repository, "config", "user.email", "test@example.com")
			gitTest(t, repository, "config", "user.name", "Test")
			absolute := filepath.Join(repository, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(absolute, []byte{'v', 'a', 'l', 'i', 'd', '\n', 0xff, '\n'}, 0o644); err != nil {
				t.Fatal(err)
			}
			gitTest(t, repository, "add", ".")
			gitTest(t, repository, "commit", "-qm", "invalid UTF-8 body")
			revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))

			var stdout, stderr bytes.Buffer
			status := runInventory([]string{"-repo", repository, "-repository", "owner/repo", "-revision", revision}, &stdout, &stderr)
			if status == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "textual blob") || !strings.Contains(stderr.String(), "not valid UTF-8") {
				t.Fatalf("status=%d stdout=%q stderr=%q, want invalid textual blob rejected without artifact", status, stdout.String(), stderr.String())
			}
		})
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

func TestInventoryRejectsOversizedFeatureBeforeArtifact(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "one/SPEC.md", "# One\n\n**ONE-01** When the feature is inventoried, the system shall enforce its resource budget.\n")
	body := "# SPEC: one/SPEC.md\nFeature: Oversized\n" + strings.Repeat("x", maxFeatureBytes)
	writeTestFile(t, repository, "test/bdd/oversized.feature", body)
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "oversized feature")
	revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))

	var stdout, stderr bytes.Buffer
	status := runInventory([]string{"-repo", repository, "-repository", "owner/repo", "-revision", revision}, &stdout, &stderr)
	if status == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "per-file limit") {
		t.Fatalf("status=%d stdout=%q stderr=%q, want fail-closed oversized feature with no artifact", status, stdout.String(), stderr.String())
	}
}

func TestInventoryRejectsOversizedAdapterCatalogSourceBeforeBodyRead(t *testing.T) {
	for index, path := range []string{
		centralHarnessRegistryPath,
		inPackageHarnessRegistryPath,
		harnessAliasSourcePath,
		harnessConfigSurfaceSourcePath,
		marketplaceSurfaceSourcePath,
		openAIAdapterSourcePath,
	} {
		t.Run(path, func(t *testing.T) {
			oid := strings.Repeat(strconv.Itoa((index%9)+1), 40)
			tree := fmt.Appendf(nil, "100644 blob %s %d\t%s%c", oid, maxAdapterCatalogSourceBytes+1, path, byte(0))
			budget := corpusBudget{byteLimit: maxCorpusBytes, fileLimit: maxInventoryFiles}
			blobs, err := selectPinnedBlobs(tree, &budget)
			if err == nil || len(blobs) != 0 || !strings.Contains(err.Error(), "adapter-catalog source") || !strings.Contains(err.Error(), "per-file limit") {
				t.Fatalf("selectPinnedBlobs() blobs=%#v error=%v, want pre-read adapter source ceiling", blobs, err)
			}
			if budget.usedBytes != 0 || budget.usedFiles != 0 {
				t.Fatalf("budget=%#v, oversized source must be rejected before corpus admission", budget)
			}
		})
	}
}

func TestInventoryRejectsScenarioOverflowBeforeArtifact(t *testing.T) {
	repository := t.TempDir()
	gitTest(t, repository, "init", "-q")
	gittest.HardenRepo(t, repository)
	gitTest(t, repository, "config", "user.email", "test@example.com")
	gitTest(t, repository, "config", "user.name", "Test")
	writeTestFile(t, repository, "one/SPEC.md", "# One\n\n**ONE-01** When scenarios are inventoried, the system shall enforce their structural budget.\n")
	var body strings.Builder
	body.WriteString("# SPEC: one/SPEC.md\nFeature: Too many scenarios\n")
	for index := 0; index <= maxFeatureScenarios; index++ {
		fmt.Fprintf(&body, "  Scenario: bounded-%03d\n    Then the outcome is bounded\n", index)
	}
	if body.Len() >= maxFeatureBytes {
		t.Fatalf("scenario-overflow fixture=%d bytes, must exercise structural rather than byte limit", body.Len())
	}
	writeTestFile(t, repository, "test/bdd/scenarios.feature", body.String())
	gitTest(t, repository, "add", ".")
	gitTest(t, repository, "commit", "-qm", "scenario overflow")
	revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))

	var stdout, stderr bytes.Buffer
	status := runInventory([]string{"-repo", repository, "-repository", "owner/repo", "-revision", revision}, &stdout, &stderr)
	if status == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), fmt.Sprintf("%d-scenario limit", maxFeatureScenarios)) {
		t.Fatalf("status=%d stdout=%q stderr=%q, want pre-parser scenario rejection with no artifact", status, stdout.String(), stderr.String())
	}
}

func TestInventoryRejectsRetainedGherkinSurfaceOverflowsWithoutArtifact(t *testing.T) {
	docStrings := func(count, mediaBytes int, content string) string {
		var body strings.Builder
		body.WriteString("# SPEC: one/SPEC.md\nFeature: retained surfaces\n")
		for index := range count {
			if index%128 == 0 {
				fmt.Fprintf(&body, "  Scenario: bounded-%d\n", index/128)
			}
			fmt.Fprintf(&body, "    Given payload-%d\n      \"\"\"%s\n", index, strings.Repeat("m", mediaBytes))
			body.WriteString(content)
			body.WriteString("      \"\"\"\n")
		}
		return body.String()
	}
	tests := []struct {
		name string
		body func() string
		want string
	}{
		{name: "tag items", body: func() string { return strings.Repeat("@a\n", maxFeatureTags+1) + "Feature: tags\n" }, want: "retained tag limit"},
		{name: "tag bytes", body: func() string { return strings.Repeat("@"+strings.Repeat("t", 70)+"\n", 1900) + "Feature: tags\n" }, want: "retained tag limit"},
		{name: "comment count", body: func() string { return strings.Repeat("# c\n", maxFeatureComments+1) + "Feature: comments\n" }, want: "retained comment limit"},
		{name: "comment bytes", body: func() string { return strings.Repeat("# "+strings.Repeat("c", 1000)+"\n", 200) + "Feature: comments\n" }, want: "retained comment limit"},
		{name: "description lines", body: func() string {
			return "Feature: descriptions\n" + strings.Repeat("  d\n", maxFeatureDescriptionLines+1)
		}, want: "retained description limit"},
		{name: "description bytes", body: func() string {
			return "Feature: descriptions\n" + strings.Repeat("  "+strings.Repeat("d", 1000)+"\n", 200)
		}, want: "retained description limit"},
		{name: "DocString separators", body: func() string { return docStrings(maxFeatureDocStrings+1, 0, "") }, want: "retained DocString delimiter limit"},
		{name: "DocString separator bytes", body: func() string { return docStrings(250, 800, "") }, want: "retained DocString delimiter limit"},
		{name: "DocString content lines", body: func() string { return docStrings(1, 0, strings.Repeat("        x\n", maxFeatureDocStringLines+1)) }, want: "retained DocString content limit"},
		{name: "DocString content bytes", body: func() string { return docStrings(1, 0, strings.Repeat("        "+strings.Repeat("x", 1000)+"\n", 200)) }, want: "retained DocString content limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := test.body()
			if len(body) >= maxFeatureBytes {
				t.Fatalf("fixture is %d bytes and would hit the file limit before the retained-surface limit", len(body))
			}
			repository := t.TempDir()
			gitTest(t, repository, "init", "-q")
			gittest.HardenRepo(t, repository)
			gitTest(t, repository, "config", "user.email", "test@example.com")
			gitTest(t, repository, "config", "user.name", "Test")
			writeTestFile(t, repository, "one/SPEC.md", "# One\n\n**ONE-01** When Gherkin is inventoried, the system shall bound retained parser surfaces.\n")
			writeTestFile(t, repository, "test/bdd/retained.feature", body)
			gitTest(t, repository, "add", ".")
			gitTest(t, repository, "commit", "-qm", "retained Gherkin overflow")
			revision := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))

			var stdout, stderr bytes.Buffer
			status := runInventory([]string{"-repo", repository, "-repository", "owner/repo", "-revision", revision}, &stdout, &stderr)
			if status == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("status=%d stdout=%q stderr=%q, want %q rejection with no artifact", status, stdout.String(), stderr.String(), test.want)
			}
		})
	}
}

func TestPostASTGherkinRetainedSurfaceBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*messages.GherkinDocument)
		want   string
	}{
		{
			name: "tags",
			mutate: func(document *messages.GherkinDocument) {
				document.Feature.Tags = make([]*messages.Tag, maxFeatureTags+1)
				for index := range document.Feature.Tags {
					document.Feature.Tags[index] = &messages.Tag{Name: "@a"}
				}
			},
			want: "parsed AST exceeds retained tag limit",
		},
		{
			name: "comments",
			mutate: func(document *messages.GherkinDocument) {
				document.Comments = make([]*messages.Comment, maxFeatureComments+1)
				for index := range document.Comments {
					document.Comments[index] = &messages.Comment{Text: "# c"}
				}
			},
			want: "parsed AST exceeds retained comment limit",
		},
		{
			name: "description",
			mutate: func(document *messages.GherkinDocument) {
				document.Feature.Description = strings.Repeat("d\n", maxFeatureDescriptionLines+1)
			},
			want: "parsed AST exceeds retained description limit",
		},
		{
			name: "DocString",
			mutate: func(document *messages.GherkinDocument) {
				document.Feature.Children = []*messages.FeatureChild{{Scenario: &messages.Scenario{Steps: []*messages.Step{{DocString: &messages.DocString{Delimiter: "\"\"\"", Content: strings.Repeat("x\n", maxFeatureDocStringLines+1)}}}}}}
			},
			want: "parsed AST exceeds retained DocString limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := &messages.GherkinDocument{Feature: &messages.Feature{}}
			test.mutate(document)
			if err := validateGherkinDocumentBounds("test/bdd/ast.feature", document); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateGherkinDocumentBounds() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestGherkinPreflightRejectsStepExamplesAndTableOverflows(t *testing.T) {
	tests := []struct {
		name string
		body func() string
		want string
	}{
		{
			name: "steps",
			body: func() string {
				return "Feature: bounded\nScenario: steps\n" + strings.Repeat("Given one step\n", maxScenarioSteps+1)
			},
			want: "step per-scenario limit",
		},
		{
			name: "examples",
			body: func() string {
				return "Feature: bounded\nScenario Outline: examples\nThen <value>\n" + strings.Repeat("Examples:\n| value |\n| one |\n", maxScenarioExamples+1)
			},
			want: "Examples per-scenario limit",
		},
		{
			name: "table rows",
			body: func() string {
				return "Feature: bounded\nScenario: table\nGiven rows\n" + strings.Repeat("| one |\n", maxScenarioTableRows+1)
			},
			want: "table-row per-scenario limit",
		},
		{
			name: "table cells",
			body: func() string {
				return "Feature: bounded\nScenario: table\nGiven cells\n" + strings.Repeat("| value ", maxTableCellsPerRow+1) + "|\n"
			},
			want: "table row exceeding",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := preflightGherkinSource(context.Background(), "test/bdd/bounded.feature", test.body())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("preflight error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestGherkinTokenBoundsIgnoreTableLikeDocStringLines(t *testing.T) {
	var body strings.Builder
	body.WriteString("Feature: bounded DocString\n  Scenario: table-like content remains content\n    Given the payload is\n      \"\"\"\n")
	for range maxScenarioTableRows + 1 {
		body.WriteString("      | this is DocString content, not a table row\n")
	}
	body.WriteString("      \"\"\"\n    Then the payload remains valid\n")
	if body.Len() >= maxFeatureBytes {
		t.Fatalf("DocString fixture=%d bytes, must remain below per-file bound", body.Len())
	}
	if err := preflightGherkinSource(context.Background(), "test/bdd/docstring.feature", body.String()); err != nil {
		t.Fatalf("table-like DocString content was misclassified: %v", err)
	}
}

func TestGherkinTokenBoundsRecognizeLocalizedScenarioKeywords(t *testing.T) {
	var body strings.Builder
	body.WriteString("# language: fr\nFonctionnalité: limites localisées\n")
	for index := 0; index <= maxFeatureScenarios; index++ {
		fmt.Fprintf(&body, "  Scénario: borné-%03d\n    Alors le résultat reste borné\n", index)
	}
	if body.Len() >= maxFeatureBytes {
		t.Fatalf("localized scenario fixture=%d bytes, must remain below per-file bound", body.Len())
	}
	err := preflightGherkinSource(context.Background(), "test/bdd/fr.feature", body.String())
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%d-scenario limit", maxFeatureScenarios)) {
		t.Fatalf("localized preflight error=%v, want dialect-aware scenario rejection", err)
	}
}

func TestGherkinTokenBoundsEnforceContextAndLineLimits(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := preflightGherkinSource(ctx, "test/bdd/canceled.feature", "Feature: canceled\n")
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "Gherkin preflight canceled") {
			t.Fatalf("preflight error=%v, want context cancellation", err)
		}
	})

	t.Run("line bytes", func(t *testing.T) {
		body := "Feature: bounded\n" + strings.Repeat("x", maxFeatureLineBytes+1) + "\n"
		err := preflightGherkinSource(context.Background(), "test/bdd/line.feature", body)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%d-byte line limit", maxFeatureLineBytes)) {
			t.Fatalf("preflight error=%v, want byte-line rejection", err)
		}
	})
}

func TestGherkinBuilderErrorStopsWithoutBecomingResourceOverflow(t *testing.T) {
	body := strings.Join([]string{
		"Feature: malformed table",
		"  Scenario: ordinary parser error",
		"    Given values",
		"      | first | second |",
		"      | only-one |",
	}, "\n")
	err := preflightGherkinSource(context.Background(), "test/bdd/malformed-table.feature", body)
	if err == nil {
		t.Fatal("preflight accepted inconsistent table cells")
	}
	var boundError *gherkinBoundError
	if errors.As(err, &boundError) {
		t.Fatalf("ordinary AST-builder error was misclassified as a fatal resource overflow: %v", err)
	}
	parsed, _, fatalErr := parseFeature(context.Background(), "test/bdd/malformed-table.feature", body)
	if fatalErr != nil || !slices.ContainsFunc(parsed.Diagnostics, func(item diagnostic) bool { return item.Kind == "malformed-gherkin-structure" }) {
		t.Fatalf("parseFeature() fatal=%v diagnostics=%#v, want one ordinary malformed-structure diagnostic", fatalErr, parsed.Diagnostics)
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
	report.Snapshot.Repository = strings.Repeat("\x00", maxJSONStringBytes)
	if _, err := marshalReportWithLimit(report, 1024); err == nil || !strings.Contains(err.Error(), "1024-byte artifact output limit") {
		t.Fatalf("marshalReportWithLimit() error = %v, want bounded escaped JSON rejection", err)
	}
}

func TestInvalidUTF8NeverReachesJSONNormalization(t *testing.T) {
	invalid := string([]byte{'b', 'a', 'd', 0xff})
	out := &boundedJSONBuffer{limit: 1024}
	if err := writeBoundedJSONString(out, invalid); err == nil || !strings.Contains(err.Error(), "invalid UTF-8") || len(out.Bytes()) != 0 {
		t.Fatalf("writeBoundedJSONString() error=%v bytes=%q, want rejection before any replacement or partial string", err, out.Bytes())
	}
	report := validReport()
	report.Limitations = []string{invalid}
	if err := validateReport(report); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("validateReport() error=%v, want invalid generated text rejection", err)
	}
	path := filepath.Join(realTempDir(t), "invalid-utf8.json")
	if err := os.WriteFile(path, append([]byte(`{"schema_version":"`), append([]byte{0xff}, []byte(`"}`)...)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReport(path); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("readReport() error=%v, want invalid report bytes rejected before encoding/json", err)
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

func TestBoundedReportJSONRejectsTokenElementAndStringBudgetsBeforeDecode(t *testing.T) {
	data, err := json.Marshal(validReport())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		limits jsonResourceLimits
		want   string
	}{
		{name: "tokens", limits: jsonResourceLimits{depth: maxJSONDepth, tokens: 2, elements: maxJSONElements, aggregateStringBytes: maxJSONAggregateStringBytes, stringBytes: maxJSONStringBytes}, want: "token count"},
		{name: "elements", limits: jsonResourceLimits{depth: maxJSONDepth, tokens: maxJSONTokens, elements: 1, aggregateStringBytes: maxJSONAggregateStringBytes, stringBytes: maxJSONStringBytes}, want: "aggregate element count"},
		{name: "aggregate strings", limits: jsonResourceLimits{depth: maxJSONDepth, tokens: maxJSONTokens, elements: maxJSONElements, aggregateStringBytes: 32, stringBytes: maxJSONStringBytes}, want: "aggregate decoded string content"},
		{name: "single string", limits: jsonResourceLimits{depth: maxJSONDepth, tokens: maxJSONTokens, elements: maxJSONElements, aggregateStringBytes: maxJSONAggregateStringBytes, stringBytes: 4}, want: "JSON string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateBoundedReportJSON(data, reflect.TypeFor[report](), test.limits); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateBoundedReportJSON() error=%v, want %q", err, test.want)
			}
		})
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	tokenCount := 0
	for {
		if _, err := decoder.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		tokenCount++
	}
	exactLimits := defaultReportJSONLimits()
	exactLimits.tokens = tokenCount
	if _, err := validateBoundedReportJSON(data, reflect.TypeFor[report](), exactLimits); err != nil {
		t.Fatalf("exact token ceiling rejected the final EOF probe: %v", err)
	}
}

func TestBoundedReportJSONAppliesExplicitLimitationsBounds(t *testing.T) {
	items := strings.TrimSuffix(strings.Repeat(`"bounded",`, maxReportLimitations+1), ",")
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "report limitations count", body: `{"limitations":[` + items + `]}`, want: fmt.Sprintf("exceeds %d elements", maxReportLimitations)},
		{name: "finding limitations count", body: `{"candidates":[{"limitations":[` + items + `]}]}`, want: fmt.Sprintf("exceeds %d elements", maxReportLimitations)},
		{name: "report limitation bytes", body: `{"limitations":["` + strings.Repeat("x", maxReportLimitationBytes+1) + `"]}`, want: fmt.Sprintf("exceeds %d bytes", maxReportLimitationBytes)},
		{name: "finding limitation bytes", body: `{"candidates":[{"limitations":["` + strings.Repeat("x", maxReportLimitationBytes+1) + `"]}]}`, want: fmt.Sprintf("exceeds %d bytes", maxReportLimitationBytes)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateBoundedReportJSON([]byte(test.body), reflect.TypeFor[report](), defaultReportJSONLimits()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateBoundedReportJSON() error=%v, want %q", err, test.want)
			}
		})
	}

	generated := validReport()
	generated.Limitations = make([]string, maxReportLimitations+1)
	if err := validateReport(generated); err == nil || !strings.Contains(err.Error(), "bounded JSON resource contract") {
		t.Fatalf("validateReport() generated-resource error=%v, want reader/writer parity rejection", err)
	}
	generated = validReport()
	generated.Candidates = []finding{{Limitations: []string{strings.Repeat("x", maxReportLimitationBytes+1)}}}
	if err := validateReport(generated); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("exceeds %d bytes", maxReportLimitationBytes)) {
		t.Fatalf("validateReport() finding limitation error=%v, want explicit string cap", err)
	}
}

func TestBoundedReportJSONRejectsNearMaximumInputShapeUnderSmallCeiling(t *testing.T) {
	prefix := `{"limitations":[`
	suffix := `"end"]}`
	remaining := int(maxReportInputBytes) - len(prefix) - len(suffix) - 1024
	shape := prefix + strings.Repeat(`"x",`, remaining/4) + suffix
	if len(shape) < int(maxReportInputBytes)-(2*1024) || len(shape) >= int(maxReportInputBytes) {
		t.Fatalf("adversarial JSON shape=%d bytes, want just below %d", len(shape), maxReportInputBytes)
	}
	limits := defaultReportJSONLimits()
	limits.tokens = 32
	if _, err := validateBoundedReportJSON([]byte(shape), reflect.TypeFor[report](), limits); err == nil || !strings.Contains(err.Error(), "token count") {
		t.Fatalf("validateBoundedReportJSON() error=%v, want early small-token-ceiling rejection", err)
	}
}

func TestJSONDepthAndTrailingSemanticsRemainFailClosed(t *testing.T) {
	nested := strings.Repeat("[", maxJSONDepth+2) + "0" + strings.Repeat("]", maxJSONDepth+2)
	if err := validateUniqueJSONDocument([]byte(nested), maxJSONDepth); err == nil || !strings.Contains(err.Error(), "JSON nesting exceeds") {
		t.Fatalf("validateUniqueJSONDocument() error=%v, want depth rejection", err)
	}
	if err := validateUniqueJSONDocument([]byte(`{} {}`), maxJSONDepth); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("validateUniqueJSONDocument() error=%v, want trailing-value rejection", err)
	}
}

func TestReportJSONSchemaDeclaresEveryCollectionCeiling(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var visit func(reflect.Type)
	visit = func(valueType reflect.Type) {
		for valueType.Kind() == reflect.Pointer {
			valueType = valueType.Elem()
		}
		if valueType.Kind() != reflect.Struct || seen[valueType] {
			return
		}
		seen[valueType] = true
		for field := range valueType.Fields() {
			if !field.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" {
				name = field.Name
			}
			base := field.Type
			for base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			switch base.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map:
				if limit, ok := reportJSONCollectionLimit(valueType, name); !ok || limit <= 0 {
					t.Errorf("report collection %s.%s has no explicit positive ceiling", valueType, name)
				}
				visit(base.Elem())
			case reflect.Struct:
				visit(base)
			}
		}
	}
	visit(reflect.TypeFor[report]())
}

func TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gittest.HardenRepo(t, repo)
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\nvar deprecatedHarnesses = []string{\"claude-code\"}\n")
	writeTestAliasAndConfigMetadata(t, repo, []string{"codex-cli", "pi-cli"}, []string{"claude-code"})
	writeTestFile(t, repo, marketplaceSurfaceSourcePath, testMarketplaceMetadata)
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

func TestInventoryReadsCentralAndInPackageHarnessRegistryPaths(t *testing.T) {
	tests := []struct {
		name           string
		central        string
		inPackage      string
		want           string
		wantLimitation bool
	}{
		{
			name:      "central registry takes precedence when present",
			central:   "package harnessregistry\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\nvar deprecatedHarnesses = []string{\"claude-code\"}\n",
			inPackage: "package agent\nvar activeHarnesses = []string{\"claude-code\"}\n",
			want:      "codex-cli,pi-cli",
		},
		{
			name:      "in-package registry remains auditable",
			inPackage: "package agent\nvar activeHarnesses = []string{\"codex-cli\"}\nvar deprecatedHarnesses = []string{\"claude-code\"}\n",
			want:      "codex-cli",
		},
		{
			name:           "malformed central registry never falls back to in-package metadata",
			central:        "package harnessregistry\nfunc ActiveHarnesses() []string { return nil }\n",
			inPackage:      "package agent\nvar activeHarnesses = []string{\"claude-code\"}\n",
			wantLimitation: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			gitTest(t, repo, "init", "-q")
			gittest.HardenRepo(t, repo)
			if test.central != "" {
				writeTestFile(t, repo, centralHarnessRegistryPath, test.central)
			}
			if test.inPackage != "" {
				writeTestFile(t, repo, inPackageHarnessRegistryPath, test.inPackage)
			}
			switch test.name {
			case "central registry takes precedence when present":
				writeTestAliasAndConfigMetadata(t, repo, []string{"codex-cli", "pi-cli"}, []string{"claude-code"})
			case "in-package registry remains auditable":
				writeTestAliasAndConfigMetadata(t, repo, []string{"codex-cli"}, []string{"claude-code"})
			default:
				writeTestAliasAndConfigMetadata(t, repo, []string{"claude-code"}, nil)
			}
			writeTestFile(t, repo, marketplaceSurfaceSourcePath, testMarketplaceMetadata)
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

func TestAdapterScopeCatalogUsesPinnedGoMetadata(t *testing.T) {
	registry := "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\", \"agy\", \"opencode-cli\", \"pi-cli\"}\nvar deprecatedHarnesses = []string{\"gemini-cli\"}\n"
	aliases := "package agent\nfunc NormalizeHarnessName(name string) string {\n switch name {\n case \"agy-cli\", \"antigravity\": return \"agy\"\n case \"pi\": return \"pi-cli\"\n default: return name\n }\n}\n"
	configs := `package configdirparity
func SurfaceForHarness(harness string) (DirectorySurface, bool) {
 switch agent.NormalizeHarnessName(harness) {
 case "claude-code": return DirectorySurface{Harness: "claude-code", Directory: ".claude", Purpose: "Claude config"}, true
 case "codex-cli": return DirectorySurface{Harness: "codex-cli", Directory: ".codex", Purpose: "Codex config"}, true
 case "agy": return DirectorySurface{Harness: "agy", Directory: ".agents", Purpose: "AGY config"}, true
 case "opencode-cli": return DirectorySurface{Harness: "opencode-cli", Directory: ".opencode", Purpose: "OpenCode config"}, true
 case "pi-cli": return DirectorySurface{Harness: "pi-cli", Directory: ".pi", Purpose: "Pi config"}, true
 case "gemini-cli": return DirectorySurface{Harness: "gemini-cli", Directory: ".gemini", Deprecated: true, Purpose: "Gemini compatibility"}, true
 default: return DirectorySurface{}, false
 }
}
`
	marketplace := "package marketplaceparity\nconst (\n NeutralCatalogPath = \".dear-agent/marketplace.json\"\n ClaudeCatalogPath = \".claude-plugin/marketplace.json\"\n)\n"
	openAI := "package agent\ntype OpenAIAdapter struct{}\nfunc (a *OpenAIAdapter) Name() string { return \"openai\" }\n"
	blobs := []pinnedBlob{
		{path: inPackageHarnessRegistryPath, oid: "registry"},
		{path: harnessAliasSourcePath, oid: "aliases"},
		{path: harnessConfigSurfaceSourcePath, oid: "configs"},
		{path: marketplaceSurfaceSourcePath, oid: "marketplace"},
		{path: openAIAdapterSourcePath, oid: "openai"},
	}
	bodies := map[string][]byte{
		"registry":    []byte(registry),
		"aliases":     []byte(aliases),
		"configs":     []byte(configs),
		"marketplace": []byte(marketplace),
		"openai":      []byte(openAI),
	}
	scopes, active, limitations := adapterScopesFromPinnedBodies(blobs, bodies)
	wantActive := []string{"agy", "claude-code", "codex-cli", "opencode-cli", "pi-cli"}
	if !reflect.DeepEqual(active, wantActive) || len(limitations) != 0 {
		t.Fatalf("active=%q limitations=%q, want full pinned active catalog", active, limitations)
	}
	byID := map[string]adapterScope{}
	for _, scope := range scopes {
		byID[scope.ID] = scope
	}
	if !containsString(byID["agy"].Names, "antigravity") || !containsString(byID["agy"].Names, ".agents") || containsString(byID["agy"].Names, ".gemini") || !containsString(byID["claude-code"].Names, ".claude-plugin") || !containsString(byID["codex-cli"].Names, ".codex") || !containsString(byID["gemini-cli"].Names, ".gemini") || byID["gemini-cli"].Lifecycle != "deprecated" || byID["openai"].Kind != "compatibility-adapter" {
		t.Fatalf("adapter scopes=%#v, want aliases, config roots, deprecated lifecycle, and OpenAI compatibility", scopes)
	}
	for _, scope := range scopes {
		if containsString(scope.Names, ".dear-agent") {
			t.Fatalf("neutral marketplace root leaked into adapter scope %#v", scope)
		}
	}
	if !slices.ContainsFunc(byID["claude-code"].Evidence, func(item scopeEvidence) bool {
		return item.Path == marketplaceSurfaceSourcePath && strings.Contains(item.Excerpt, `ClaudeCatalogPath = ".claude-plugin/marketplace.json"`)
	}) {
		t.Fatalf("Claude scope lacks pinned native marketplace evidence: %#v", byID["claude-code"].Evidence)
	}
	if !slices.ContainsFunc(byID["agy"].Evidence, func(item scopeEvidence) bool {
		return item.Path == harnessConfigSurfaceSourcePath && strings.Contains(item.Excerpt, `Directory: ".agents"`)
	}) {
		t.Fatalf("AGY scope lacks pinned .agents mapping evidence: %#v", byID["agy"].Evidence)
	}
	for harness, directory := range map[string]string{"claude-code": ".claude", "codex-cli": ".codex", "agy": ".agents", "opencode-cli": ".opencode", "pi-cli": ".pi"} {
		if !containsString(byID[harness].Names, directory) {
			t.Fatalf("active harness %q lacks authoritative config directory %q: %#v", harness, directory, byID[harness])
		}
	}
	for _, path := range []string{"agm/internal/codexsession/SPEC.md", "agm/internal/agent/openai/SPEC.md", ".codex/SPEC.md"} {
		if !isHarnessSurfacePath(path, scopes) {
			t.Fatalf("catalog failed to classify %q", path)
		}
	}
	for _, path := range []string{"internal/domain/SPEC.md", "pkg/contracts/SPEC.md", "pkg/dearagent/SPEC.md", "agm/internal/agent/SPEC.md"} {
		if isHarnessSurfacePath(path, scopes) {
			t.Fatalf("bounded catalog over-classified %q", path)
		}
	}
}

func TestAdapterScopeCatalogRequiresPinnedMarketplaceMetadata(t *testing.T) {
	tests := []struct {
		name        string
		marketplace string
		present     bool
	}{
		{name: "absent"},
		{name: "malformed", marketplace: "package marketplaceparity\nconst ClaudeCatalogPath = filepath.Join(\".claude-plugin\", \"marketplace.json\")\n", present: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			gitTest(t, repo, "init", "-q")
			gittest.HardenRepo(t, repo)
			writeTestFile(t, repo, inPackageHarnessRegistryPath, "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n")
			writeTestAliasAndConfigMetadata(t, repo, []string{"claude-code", "codex-cli"}, nil)
			if test.present {
				writeTestFile(t, repo, marketplaceSurfaceSourcePath, test.marketplace)
			}
			writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When catalog metadata is incomplete, the audit shall record the exact limitation.\n")
			gitTest(t, repo, "add", ".")
			gitTest(t, repo, "commit", "-qm", "marketplace metadata fixture")
			revision := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

			got, err := inventory(repo, "owner/repo", revision)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.Limitations, []string{adapterCatalogIncompleteLimitation}) {
				t.Fatalf("limitations=%q, want exact incomplete-catalog limitation", got.Limitations)
			}
			if len(got.Scope.AdapterScopes) != 0 || len(got.Scope.ActiveMembers) != 0 {
				t.Fatalf("scope=%#v, want fail-closed empty adapter catalog", got.Scope)
			}
		})
	}
}

func TestPositiveFindingsRejectEveryIncompleteAdapterCatalogLimitation(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	for _, limitation := range []string{
		activeHarnessUnavailableLimitation,
		activeHarnessUnparseableLimitation,
		deprecatedHarnessUnparseableLimitation,
		adapterCatalogIncompleteLimitation,
	} {
		t.Run(limitation, func(t *testing.T) {
			report := cloneReport(t, semanticReport)
			report.Limitations = []string{limitation}
			err := validateReport(report)
			if err == nil || !strings.Contains(err.Error(), "pinned adapter-scope catalog is incomplete") {
				t.Fatalf("validateReport() error=%v, want positive-finding catalog limitation rejection", err)
			}
		})
	}
}

func TestAdapterScopeCatalogRejectsUnsupportedPinnedMetadataShapes(t *testing.T) {
	const registry = "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n"
	const validAliases = "package agent\nfunc NormalizeHarnessName(name string) string {\n switch name {\n case \"codex\": return \"codex-cli\"\n default: return name\n }\n}\n"
	const validConfig = `package configdirparity
func SurfaceForHarness(harness string) (DirectorySurface, bool) {
 switch agent.NormalizeHarnessName(harness) {
 case "claude-code": return DirectorySurface{Harness: "claude-code", Directory: ".claude", Purpose: "Claude config"}, true
 case "codex-cli": return DirectorySurface{Harness: "codex-cli", Directory: ".codex", Purpose: "Codex config"}, true
 default: return DirectorySurface{}, false
 }
}
`
	const validMarketplace = "package marketplaceparity\nconst (\n NeutralCatalogPath = \".dear-agent/marketplace.json\"\n ClaudeCatalogPath = \".claude-plugin/marketplace.json\"\n)\n"
	const validOpenAI = "package agent\ntype OpenAIAdapter struct{}\nfunc (a *OpenAIAdapter) Name() string { return \"openai\" }\n"
	tests := []struct {
		name        string
		aliases     string
		config      string
		marketplace string
		openAI      string
	}{
		{name: "computed alias case", aliases: "package agent\nfunc NormalizeHarnessName(name string) string { switch name { case aliasName: return \"codex-cli\"; default: return name } }\n"},
		{name: "computed alias return", aliases: "package agent\nfunc NormalizeHarnessName(name string) string { switch name { case \"codex\": return strings.TrimSpace(name); default: return name } }\n"},
		{name: "unknown alias canonical", aliases: "package agent\nfunc NormalizeHarnessName(name string) string { switch name { case \"codex\": return \"unknown\"; default: return name } }\n"},
		{name: "multi-harness config case", config: "package configdirparity\nfunc SurfaceForHarness(harness string) (DirectorySurface, bool) { switch agent.NormalizeHarnessName(harness) { case \"codex-cli\", \"agy\": return DirectorySurface{Harness: \"codex-cli\", Directory: \".codex\", Purpose: \"config\"}, true; default: return DirectorySurface{}, false } }\n"},
		{name: "computed config return", config: "package configdirparity\nfunc SurfaceForHarness(harness string) (DirectorySurface, bool) { switch agent.NormalizeHarnessName(harness) { case \"codex-cli\": return makeSurface(), true; default: return DirectorySurface{}, false } }\n"},
		{name: "active config marked deprecated", config: "package configdirparity\nfunc SurfaceForHarness(harness string) (DirectorySurface, bool) { switch agent.NormalizeHarnessName(harness) { case \"codex-cli\": return DirectorySurface{Harness: \"codex-cli\", Directory: \".codex\", Deprecated: true, Purpose: \"config\"}, true; default: return DirectorySurface{}, false } }\n"},
		{name: "computed marketplace path", marketplace: "package marketplaceparity\nconst NeutralCatalogPath = \".dear-agent/marketplace.json\"\nconst ClaudeCatalogPath = filepath.Join(\".claude-plugin\", \"marketplace.json\")\n"},
		{name: "marketplace roots are not distinct", marketplace: "package marketplaceparity\nconst NeutralCatalogPath = \".dear-agent/marketplace.json\"\nconst ClaudeCatalogPath = \".dear-agent/marketplace.json\"\n"},
		{name: "computed adapter name", openAI: "package agent\ntype OpenAIAdapter struct{}\nfunc (a *OpenAIAdapter) Name() string { return strings.ToLower(\"openai\") }\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aliases := validAliases
			if test.aliases != "" {
				aliases = test.aliases
			}
			config := validConfig
			if test.config != "" {
				config = test.config
			}
			marketplace := validMarketplace
			if test.marketplace != "" {
				marketplace = test.marketplace
			}
			openAI := validOpenAI
			if test.openAI != "" {
				openAI = test.openAI
			}
			blobs := []pinnedBlob{
				{path: inPackageHarnessRegistryPath, oid: "registry"},
				{path: harnessAliasSourcePath, oid: "aliases"},
				{path: harnessConfigSurfaceSourcePath, oid: "config"},
				{path: marketplaceSurfaceSourcePath, oid: "marketplace"},
				{path: openAIAdapterSourcePath, oid: "openai"},
			}
			scopes, active, limitations := adapterScopesFromPinnedBodies(blobs, map[string][]byte{
				"registry":    []byte(registry),
				"aliases":     []byte(aliases),
				"config":      []byte(config),
				"marketplace": []byte(marketplace),
				"openai":      []byte(openAI),
			})
			if len(scopes) != 0 || len(active) != 0 || len(limitations) != 1 || !strings.Contains(limitations[0], "bounded or unambiguous pinned metadata contract") {
				t.Fatalf("scopes=%#v active=%#v limitations=%#v, want fail-closed metadata limitation", scopes, active, limitations)
			}
		})
	}
}

func TestAdapterScopeCatalogRequiresExactlyOneSupportedRegistryTarget(t *testing.T) {
	const validRegistry = "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n"
	tests := []struct {
		name     string
		target   string
		registry string
	}{
		{
			name:     "duplicate active declarations",
			target:   "activeHarnesses",
			registry: "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar activeHarnesses = []string{\"pi-cli\"}\nvar deprecatedHarnesses = []string{}\n",
		},
		{
			name:     "duplicate deprecated declarations",
			target:   "deprecatedHarnesses",
			registry: "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\nvar deprecatedHarnesses = []string{\"gemini-cli\"}\n",
		},
		{
			name:     "malformed active before valid active",
			target:   "activeHarnesses",
			registry: "package agent\nvar activeHarnesses = discoverHarnesses()\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n",
		},
		{
			name:     "malformed deprecated before valid deprecated",
			target:   "deprecatedHarnesses",
			registry: "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = [1]string{\"gemini-cli\"}\nvar deprecatedHarnesses = []string{}\n",
		},
		{
			name:     "missing deprecated declaration",
			target:   "deprecatedHarnesses",
			registry: "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\n",
		},
		{
			name:     "local active declaration",
			target:   "activeHarnesses",
			registry: validRegistry + "func shadow() { var activeHarnesses = []string{\"pi-cli\"}; _ = activeHarnesses }\n",
		},
		{
			name:     "function parameter",
			target:   "activeHarnesses",
			registry: validRegistry + "func shadow(activeHarnesses []string) { _ = activeHarnesses }\n",
		},
		{
			name:     "closure parameter",
			target:   "activeHarnesses",
			registry: validRegistry + "var _ = func(activeHarnesses []string) { _ = activeHarnesses }\n",
		},
		{
			name:     "range binding",
			target:   "activeHarnesses",
			registry: validRegistry + "func shadow(values []string) { for activeHarnesses := range values { _ = activeHarnesses } }\n",
		},
		{
			name:     "range assignment",
			target:   "activeHarnesses",
			registry: validRegistry + "func mutate(values []string) { for activeHarnesses = range values {} }\n",
		},
		{
			name:     "recursive index assignment",
			target:   "activeHarnesses",
			registry: validRegistry + "func mutate() { activeHarnesses[0] = \"pi-cli\" }\n",
		},
		{
			name:     "recursive index increment",
			target:   "activeHarnesses",
			registry: validRegistry + "func mutate() { activeHarnesses[0]++ }\n",
		},
	}

	if values, ok := goStringSliceValues(inPackageHarnessRegistryPath, validRegistry, "activeHarnesses"); !ok || len(values) != 2 {
		t.Fatalf("supported package declaration values=%#v ok=%v, want exact active registry", values, ok)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if values, ok := goStringSliceValues(inPackageHarnessRegistryPath, test.registry, test.target); ok {
				t.Fatalf("goStringSliceValues() values=%#v ok=true, want fail-closed %s rejection", values, test.target)
			}
			blobs, bodies := completeAdapterCatalogTestInputs(test.registry)
			scopes, active, limitations := adapterScopesFromPinnedBodies(blobs, bodies)
			if len(scopes) != 0 || len(active) != 0 || !slices.Equal(limitations, []string{adapterCatalogIncompleteLimitation}) {
				t.Fatalf("scopes=%#v active=%#v limitations=%#v, want exact fail-closed catalog limitation", scopes, active, limitations)
			}
		})
	}
}

func TestAdapterScopeCatalogRequiresPinnedAliasAndConfigMetadata(t *testing.T) {
	const registry = "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n"
	for _, missingPath := range []string{harnessAliasSourcePath, harnessConfigSurfaceSourcePath, openAIAdapterSourcePath} {
		t.Run(missingPath, func(t *testing.T) {
			blobs, bodies := completeAdapterCatalogTestInputs(registry)
			filtered := make([]pinnedBlob, 0, len(blobs)-1)
			for _, blob := range blobs {
				if blob.path != missingPath {
					filtered = append(filtered, blob)
				}
			}
			scopes, active, limitations := adapterScopesFromPinnedBodies(filtered, bodies)
			if len(scopes) != 0 || len(active) != 0 || !slices.Equal(limitations, []string{adapterCatalogIncompleteLimitation}) {
				t.Fatalf("scopes=%#v active=%#v limitations=%#v, want absent-source fail-closed catalog limitation", scopes, active, limitations)
			}
		})
	}
}

func TestAdapterScopeCatalogIgnoresNonbindingRegistryNames(t *testing.T) {
	const validRegistry = "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n"
	tests := []struct {
		name   string
		append string
	}{
		{
			name:   "struct field",
			append: "type metadata struct { activeHarnesses []string }\n",
		},
		{
			name:   "interface method",
			append: "type metadata interface { activeHarnesses() }\n",
		},
		{
			name:   "receiver method",
			append: "type metadata struct{}\nfunc (metadata) activeHarnesses() {}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values, ok := goStringSliceValues(inPackageHarnessRegistryPath, validRegistry+test.append, "activeHarnesses")
			if !ok || len(values) != 2 {
				t.Fatalf("goStringSliceValues() values=%#v ok=%v, want unaffected package registry", values, ok)
			}
			blobs, bodies := completeAdapterCatalogTestInputs(validRegistry + test.append)
			scopes, active, limitations := adapterScopesFromPinnedBodies(blobs, bodies)
			if len(scopes) == 0 || len(active) == 0 || len(limitations) != 0 {
				t.Fatalf("scopes=%#v active=%#v limitations=%#v, want complete catalog", scopes, active, limitations)
			}
		})
	}
}

func TestAdapterScopeCatalogBoundsEverySelectedGoMetadataBlob(t *testing.T) {
	const registry = "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n"
	blobs, validBodies := completeAdapterCatalogTestInputs(registry)
	for _, blob := range blobs {
		t.Run(blob.path, func(t *testing.T) {
			bodies := make(map[string][]byte, len(validBodies))
			for oid, body := range validBodies {
				bodies[oid] = append([]byte(nil), body...)
			}
			bodies[blob.oid] = append(bodies[blob.oid], []byte(strings.Repeat("\n", maxAdapterCatalogSourceBytes-len(bodies[blob.oid])+1))...)
			scopes, active, limitations := adapterScopesFromPinnedBodies(blobs, bodies)
			if len(scopes) != 0 || len(active) != 0 || !slices.Equal(limitations, []string{adapterCatalogIncompleteLimitation}) {
				t.Fatalf("scopes=%#v active=%#v limitations=%#v, want bounded-source fail-closed catalog limitation", scopes, active, limitations)
			}
		})
	}
}

func TestAdapterScopeCatalogHonorsCanceledInventoryContext(t *testing.T) {
	const registry = "package agent\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\"}\nvar deprecatedHarnesses = []string{}\n"
	blobs, bodies := completeAdapterCatalogTestInputs(registry)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scopes, active, limitations, err := adapterScopesFromPinnedBodiesContext(ctx, blobs, bodies)
	if !errors.Is(err, context.Canceled) || len(scopes) != 0 || len(active) != 0 || len(limitations) != 0 {
		t.Fatalf("scopes=%#v active=%#v limitations=%#v error=%v, want canceled projection with no partial catalog", scopes, active, limitations, err)
	}
}

func completeAdapterCatalogTestInputs(registry string) ([]pinnedBlob, map[string][]byte) {
	const aliases = "package agent\nfunc NormalizeHarnessName(name string) string { switch name { default: return name } }\n"
	const config = `package configdirparity
func SurfaceForHarness(harness string) (DirectorySurface, bool) {
 switch agent.NormalizeHarnessName(harness) {
 case "claude-code": return DirectorySurface{Harness: "claude-code", Directory: ".claude", Purpose: "Claude config"}, true
 case "codex-cli": return DirectorySurface{Harness: "codex-cli", Directory: ".codex", Purpose: "Codex config"}, true
 default: return DirectorySurface{}, false
 }
}
`
	const openAI = "package agent\ntype OpenAIAdapter struct{}\nfunc (a *OpenAIAdapter) Name() string { return \"openai\" }\n"
	blobs := []pinnedBlob{
		{path: inPackageHarnessRegistryPath, oid: "registry"},
		{path: harnessAliasSourcePath, oid: "aliases"},
		{path: harnessConfigSurfaceSourcePath, oid: "config"},
		{path: marketplaceSurfaceSourcePath, oid: "marketplace"},
		{path: openAIAdapterSourcePath, oid: "openai"},
	}
	return blobs, map[string][]byte{
		"registry":    []byte(registry),
		"aliases":     []byte(aliases),
		"config":      []byte(config),
		"marketplace": []byte(testMarketplaceMetadata),
		"openai":      []byte(openAI),
	}
}

func TestAdapterScopeCatalogFailsClosedAboveBound(t *testing.T) {
	values := make([]string, 0, maxAdapterScopes+1)
	for index := 0; index <= maxAdapterScopes; index++ {
		values = append(values, fmt.Sprintf("%q", fmt.Sprintf("harness-%d", index)))
	}
	registry := "package agent\nvar activeHarnesses = []string{" + strings.Join(values, ",") + "}\nvar deprecatedHarnesses = []string{}\n"
	blobs := []pinnedBlob{{path: inPackageHarnessRegistryPath, oid: "registry"}}
	scopes, active, limitations := adapterScopesFromPinnedBodies(blobs, map[string][]byte{"registry": []byte(registry)})
	if len(scopes) != 0 || len(active) != 0 || len(limitations) != 1 || !strings.Contains(limitations[0], "exceeded") {
		t.Fatalf("scopes=%#v active=%#v limitations=%#v, want fail-closed bounded catalog", scopes, active, limitations)
	}
}

func TestLinkedWorktreeWithConfiguredAlternatesDisclosesGitTrustBoundary(t *testing.T) {
	source := realTempDir(t)
	gitTest(t, source, "init", "-q")
	gittest.HardenRepo(t, source)
	writeTestFile(t, source, centralHarnessRegistryPath, "package harnessregistry\nvar activeHarnesses = []string{\"codex-cli\"}\n")
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

func TestMethodologyRequiresAndRendersUnverifiedRuntimeStatus(t *testing.T) {
	for _, status := range []string{"", "VERIFIED", "PASS"} {
		report := validReport()
		report.Methodology.RuntimeStatus = status
		if err := validateReport(report); err == nil || !strings.Contains(err.Error(), "runtime_status") {
			t.Fatalf("validateReport() error=%v, want fixed runtime-status rejection for %q", err, status)
		}
	}
	report := validReport()
	if err := validateReport(report); err != nil {
		t.Fatalf("fixed runtime status should validate: %v", err)
	}
	output := renderHTML(report, nil)
	if !strings.Contains(output, "Runtime status") || !strings.Contains(output, runtimeStatusUnverified) {
		t.Fatalf("rendered report omitted explicit runtime status: %s", output)
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
	control := exec.CommandContext(controlCtx, executable.Path(), "-c", "protocol.ext.allow=always", "--no-replace-objects", "-C", repo, "cat-file", "blob", missingBlob)
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

func TestParseSpecDoesNotCloseFourBacktickFenceWithThreeBackticks(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"````markdown",
		"**EXAMPLE-01** When a sample runs, the system shall not count it.",
		"```",
		"**LEAKED-01** When a shorter fence appears, the system shall still not count it.",
		"- Feature: `test/bdd/leaked.feature`",
		"````",
		"",
		"**REAL-01** When the source is parsed, the system shall count real requirements.",
	}, "\n"))

	if len(parsed.Requirements) != 1 || parsed.Requirements[0].ID != "REAL-01" {
		t.Fatalf("requirements=%#v, want REAL-01 only", parsed.Requirements)
	}
	if len(parsed.BDDFeatures) != 0 {
		t.Fatalf("BDD features=%#v, want no example links", parsed.BDDFeatures)
	}
}

func TestParseSpecDoesNotCloseFenceWithTrailingContent(t *testing.T) {
	parsed := parseSpec("example/SPEC.md", strings.Join([]string{
		"# Example",
		"",
		"````markdown",
		"**EXAMPLE-01** When a sample runs, the system shall not count it.",
		"````go",
		"**LEAKED-01** When a fence-like line has trailing content, the system shall still not count it.",
		"````",
		"",
		"**REAL-01** When the source is parsed, the system shall count real requirements.",
	}, "\n"))

	if len(parsed.Requirements) != 1 || parsed.Requirements[0].ID != "REAL-01" {
		t.Fatalf("requirements=%#v, want REAL-01 only", parsed.Requirements)
	}
}

func TestParseFeatureDoesNotCloseFourTildeFenceWithThreeTildes(t *testing.T) {
	parsed, refs, err := parseFeature(context.Background(), "test/bdd/example.feature", strings.Join([]string{
		"~~~~gherkin",
		"# SPEC: example/SPEC.md",
		"~~~",
		"# SPEC: leaked/SPEC.md",
		"~~~~",
		"# SPEC: real/SPEC.md",
		"Feature: Real traceability",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(parsed.RelatedSpecs, []string{"real/SPEC.md"}) {
		t.Fatalf("related specs=%#v, want real/SPEC.md only", parsed.RelatedSpecs)
	}
	if len(refs) != 1 || refs[0].Path != "real/SPEC.md" {
		t.Fatalf("refs=%#v, want real/SPEC.md only", refs)
	}
	if len(parsed.Diagnostics) != 0 {
		t.Fatalf("diagnostics=%#v, want none", parsed.Diagnostics)
	}
}

func TestParseFeatureProjectsBoundedGherkinScenarioProof(t *testing.T) {
	parsed, _, err := parseFeature(context.Background(), "test/bdd/shared.feature", strings.Join([]string{
		"# SPEC: one/SPEC.md",
		"Feature: Shared behavior",
		"",
		"  Scenario Outline: Active members share the observable",
		"    Given the <harness> harness is active",
		"    When the behavior runs",
		"    Then the shared outcome is visible",
		"    And the outcome remains stable",
		"",
		"    Examples:",
		"      | harness   | family |",
		"      | codex-cli | openai |",
		"      | pi-cli    | pi     |",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Scenarios) != 1 {
		t.Fatalf("scenarios=%#v, want one projected outline", parsed.Scenarios)
	}
	scenario := parsed.Scenarios[0]
	if scenario.Line != 4 || scenario.Name != "Active members share the observable" || scenario.Kind != "scenario-outline" || scenario.MemberColumn != "harness" || !scenario.UsesMemberPlaceholder {
		t.Fatalf("scenario=%#v, want exact outline identity and member placeholder", scenario)
	}
	if len(scenario.Outcomes) != 2 || scenario.Outcomes[0].Line != 7 || len(scenario.MemberCases) != 2 || scenario.MemberCases[1].Member != "pi-cli" {
		t.Fatalf("scenario proof=%#v, want Then/And outcomes and exact examples cases", scenario)
	}
}

func TestCanonicalRepositoryRelativePathsRejectAliases(t *testing.T) {
	for _, path := range []string{"one/SPEC.md", "agm/test/bdd/features/shared.feature"} {
		if !validPath(path) {
			t.Fatalf("validPath(%q) = false, want canonical path accepted", path)
		}
	}
	for _, path := range []string{
		"./one/SPEC.md",
		"one/../two/SPEC.md",
		"one/./SPEC.md",
		"one//SPEC.md",
		"one/SPEC.md/",
		" one/SPEC.md",
		"one/SPEC.md ",
		`one\SPEC.md`,
	} {
		if validPath(path) {
			t.Fatalf("validPath(%q) = true, want noncanonical alias rejected", path)
		}
	}

	parsed, refs, err := parseFeature(context.Background(), "test/bdd/alias.feature", strings.Join([]string{
		"# SPEC: ./one/SPEC.md",
		"Feature: Noncanonical SPEC alias",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 || len(parsed.RelatedSpecs) != 0 || !slices.ContainsFunc(parsed.Diagnostics, func(item diagnostic) bool {
		return item.Kind == "malformed-feature-spec-reference"
	}) {
		t.Fatalf("parsed feature=%#v refs=%#v, want rejected ./ SPEC alias", parsed, refs)
	}

	_, _, semanticReport := auditFixture(t)
	finding := semanticReport.Candidates[0]
	finding.Verdict = "extract-neutral-contract"
	finding.ProposedOwner = &proposedOwnerClaim{
		Path:                "./one/SPEC.md",
		State:               "new",
		Rationale:           "This alias must not masquerade as a new owner.",
		NeutralityRationale: "This alias does not create a distinct neutral seam.",
	}
	if err := validateFinding(finding, false, stringSet(semanticReport.Scope.ActiveMembers), semanticReport.Scope.AdapterScopes); err == nil || !strings.Contains(err.Error(), "product SPEC proposed owner") {
		t.Fatalf("validateFinding() error=%v, want noncanonical proposed-owner rejection", err)
	}
}

func TestSharedScenarioRejectsAdditionalNonMemberExamplesTable(t *testing.T) {
	body := strings.Join([]string{
		"# SPEC: one/SPEC.md",
		"# RELATED-SPEC: two/SPEC.md",
		"Feature: Shared behavior",
		"",
		"  Scenario Outline: Active members preserve request identity",
		"    Given the <harness> harness is active",
		"    When the behavior runs in <region>",
		"    Then the shared outcome is visible",
		"",
		"    Examples: members",
		"      | harness   | region |",
		"      | codex-cli | west   |",
		"      | pi-cli    | east   |",
		"",
		"    Examples: regions without members",
		"      | region  |",
		"      | central |",
	}, "\n")
	parsed, _, err := parseFeature(context.Background(), "agm/test/bdd/features/shared.feature", body)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(parsed.Diagnostics, func(item diagnostic) bool {
		return item.Kind == "malformed-gherkin-member-cases" && strings.Contains(item.Excerpt, "non-member Examples")
	}) {
		t.Fatalf("diagnostics=%#v, want mixed Examples structural diagnostic", parsed.Diagnostics)
	}

	_, inventoryReport, semanticReport := auditFixture(t)
	pinnedFeature := inventoryFeaturePointer(t, &inventoryReport, "agm/test/bdd/features/shared.feature")
	inventoryReport.Summary.Diagnostics += len(parsed.Diagnostics) - len(pinnedFeature.Diagnostics)
	semanticReport.Summary.Diagnostics = inventoryReport.Summary.Diagnostics
	*pinnedFeature = parsed
	err = validateAgainstInventory(semanticReport, inventoryReport)
	if err == nil || !strings.Contains(err.Error(), "incomplete structural inventory") {
		t.Fatalf("validateAgainstInventory() error=%v, want selected shared scenario rejection", err)
	}
}

func TestSharedScenarioIgnoresUnrelatedMalformedMemberCases(t *testing.T) {
	body := strings.Join([]string{
		"# SPEC: one/SPEC.md",
		"# RELATED-SPEC: two/SPEC.md",
		"Feature: Shared behavior",
		"",
		"  Scenario Outline: Active members preserve request identity",
		"    Given the <harness> harness is active",
		"    When the behavior runs",
		"    Then the shared outcome is visible",
		"",
		"    Examples:",
		"      | harness   |",
		"      | codex-cli |",
		"      | pi-cli    |",
		"",
		"  Scenario Outline: An unrelated malformed matrix",
		"    Given the <harness> harness is active",
		"    Then the unrelated outcome is visible",
		"",
		"    Examples: members",
		"      | harness   |",
		"      | codex-cli |",
		"",
		"    Examples: non-members",
		"      | region  |",
		"      | central |",
	}, "\n")
	parsed, _, err := parseFeature(context.Background(), "agm/test/bdd/features/shared.feature", body)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(parsed.Diagnostics, func(item diagnostic) bool {
		return item.Kind == "malformed-gherkin-member-cases" && strings.Contains(item.Excerpt, "non-member Examples")
	}) {
		t.Fatalf("diagnostics=%#v, want unrelated malformed member-cases diagnostic", parsed.Diagnostics)
	}

	_, inventoryReport, semanticReport := auditFixture(t)
	pinnedFeature := inventoryFeaturePointer(t, &inventoryReport, "agm/test/bdd/features/shared.feature")
	inventoryReport.Summary.Diagnostics += len(parsed.Diagnostics) - len(pinnedFeature.Diagnostics)
	semanticReport.Summary.Diagnostics = inventoryReport.Summary.Diagnostics
	*pinnedFeature = parsed
	if err := validateAgainstInventory(semanticReport, inventoryReport); err != nil {
		t.Fatalf("validateAgainstInventory() error=%v, want unrelated outline diagnostic ignored", err)
	}
}

func TestParseFeaturePreservesGherkinBacktickDocStrings(t *testing.T) {
	parsed, _, err := parseFeature(context.Background(), "test/bdd/docstring.feature", strings.Join([]string{
		"# SPEC: one/SPEC.md",
		"Feature: DocString behavior",
		"",
		"  Scenario: A native Gherkin DocString remains parseable",
		"    Given the configuration is:",
		"      ```json",
		"      {\"enabled\": true}",
		"      ```",
		"    When the configuration is applied",
		"    Then the behavior is enabled",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Scenarios) != 1 || len(parsed.Scenarios[0].Outcomes) != 1 {
		t.Fatalf("scenarios=%#v diagnostics=%#v, want parsed Gherkin DocString scenario", parsed.Scenarios, parsed.Diagnostics)
	}
	for _, item := range parsed.Diagnostics {
		if item.Kind == "malformed-gherkin-structure" {
			t.Fatalf("native Gherkin DocString was treated as Markdown: %#v", item)
		}
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
		"## Test Traceability",
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

func TestParseSpecUsesFirstFeatureBearingTraceabilitySection(t *testing.T) {
	tests := []struct {
		name            string
		body            []string
		wantFeatures    []string
		wantDiagnostics []string
	}{
		{
			name: "BDD plus unit-only test traceability",
			body: []string{
				"# Example",
				"## BDD Traceability",
				"- Feature: `test/bdd/first.feature`",
				"## Test Traceability",
				"- Unit: `pkg/example/example_test.go`",
			},
			wantFeatures: []string{"test/bdd/first.feature"},
		},
		{
			name: "sole test traceability feature",
			body: []string{
				"# Example",
				"## Test Traceability",
				"- Feature: `test/bdd/only.feature`",
			},
			wantFeatures: []string{"test/bdd/only.feature"},
		},
		{
			name: "unit-only section before first feature-bearing section",
			body: []string{
				"# Example",
				"## Package Test Traceability",
				"- Unit: `pkg/example/example_test.go`",
				"## BDD Traceability",
				"- Feature: `test/bdd/first.feature`",
			},
			wantFeatures: []string{"test/bdd/first.feature"},
		},
		{
			name: "repeated feature-bearing heading is ambiguous",
			body: []string{
				"# Example",
				"## BDD Traceability",
				"- Feature: `test/bdd/first.feature`",
				"## BDD Traceability",
				"- Feature: `test/bdd/ignored.feature`",
			},
			wantFeatures:    []string{"test/bdd/first.feature"},
			wantDiagnostics: []string{"ambiguous-bdd-traceability-section"},
		},
		{
			name: "malformed later feature claim is ambiguous and malformed",
			body: []string{
				"# Example",
				"## BDD Traceability",
				"- Feature: `test/bdd/first.feature`",
				"## Package Test Traceability",
				"- Feature: test/bdd/malformed.feature",
			},
			wantFeatures:    []string{"test/bdd/first.feature"},
			wantDiagnostics: []string{"ambiguous-bdd-traceability-section", "malformed-bdd-feature-reference"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := parseSpec("example/SPEC.md", strings.Join(test.body, "\n"))
			features := make([]string, 0, len(parsed.BDDFeatures))
			for _, feature := range parsed.BDDFeatures {
				features = append(features, feature.Path)
			}
			if !slices.Equal(features, test.wantFeatures) {
				t.Fatalf("BDD features=%#v, want %#v", features, test.wantFeatures)
			}
			diagnostics := make([]string, 0, len(parsed.Diagnostics))
			for _, item := range parsed.Diagnostics {
				diagnostics = append(diagnostics, item.Kind)
			}
			if !slices.Equal(diagnostics, test.wantDiagnostics) {
				t.Fatalf("diagnostics=%#v, want %#v", parsed.Diagnostics, test.wantDiagnostics)
			}
		})
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
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the behavior"}}, ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "already owns the neutral behavior"}, SharedOutcome: "same", MaterialDifferences: []string{"none observed"}, Evidence: []evidence{{Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}},
		ApplicabilityBasis: "active-members", ApplicabilityRationale: "supported by the active member", Applicability: []applicability{{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}}}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "merge"}, Recommendation: []string{"merge"}, Risk: "bounded", Decision: "approve",
	}}
	report.Summary.CandidateCount = 1
	report.Summary.ByVerdict = map[string]int{"merge-now": 1}
	if err := validateReport(report); err == nil || !strings.Contains(err.Error(), "at least two pinned current owners") {
		t.Fatalf("validateReport error=%v, want merge-now owner-cardinality rejection", err)
	}
}

func TestValidateFindingOwnerStateCoherence(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	candidateFinding := semanticReport.Candidates[0]
	active := map[string]bool{"codex-cli": true, "pi-cli": true}

	candidateFinding.Verdict = "extract-neutral-contract"
	candidateFinding.ProposedOwner = &proposedOwnerClaim{Path: "shared/SPEC.md", State: "new", Rationale: "A new implemented shared seam is required.", NeutralityRationale: "The top-level shared contract is outside every harness registration surface."}
	for index := range candidateFinding.OwnershipPlan.CurrentOwners[1].Preservation.Requirements {
		candidateFinding.OwnershipPlan.CurrentOwners[1].Preservation.Requirements[index].TargetState = "planned"
	}
	for index := range candidateFinding.OwnershipPlan.CurrentOwners[1].Preservation.BDD {
		candidateFinding.OwnershipPlan.CurrentOwners[1].Preservation.BDD[index].TargetOwner = "shared/SPEC.md"
	}
	candidateFinding.OwnershipPlan.CurrentOwners[0] = ownershipPlanOwner{
		Path: "one/SPEC.md", Action: "retire-normative-ownership", Rationale: "The old owner will become a canonical reference.", Preservation: &preservationPlan{
			Requirements: []requirementPreservation{{Source: candidateFinding.Evidence[0], TargetID: candidateFinding.Evidence[0].RequirementID, TargetState: "planned", Strategy: "preserve-id"}},
			BDD: []bddPreservation{
				{Feature: candidateFinding.BDD.SharedContractFeature, SourceOwner: "one/SPEC.md", TargetOwner: "shared/SPEC.md"},
				{Feature: "agm/test/bdd/features/one-only.feature", SourceOwner: "one/SPEC.md", TargetOwner: "shared/SPEC.md"},
			},
			ApplicabilityBasis: candidateFinding.ApplicabilityBasis,
			Applicability:      candidateFinding.Applicability,
		},
	}
	if err := validateFinding(candidateFinding, false, active, inventoryReport.Scope.AdapterScopes); err != nil {
		t.Fatalf("new proposed owner should be valid: %v", err)
	}
	semanticSummary := semanticReport.Summary
	semanticSummary.ByVerdict = map[string]int{"extract-neutral-contract": 1}
	if err := validateAgainstInventory(report{
		SchemaVersion: schemaVersion,
		Snapshot:      semanticReport.Snapshot,
		Scope:         semanticReport.Scope,
		Summary:       semanticSummary,
		Methodology:   semanticReport.Methodology,
		Candidates:    []finding{candidateFinding},
		NonCandidates: []finding{},
		Limitations:   semanticReport.Limitations,
	}, inventoryReport); err != nil {
		t.Fatalf("absent new owner should validate against pinned evidence: %v", err)
	}

	candidateFinding.Verdict = "insufficient-evidence"
	candidateFinding.Strength = "moderate"
	if err := validateFinding(candidateFinding, false, active, inventoryReport.Scope.AdapterScopes); err == nil || !strings.Contains(err.Error(), "cannot carry a canonical owner") {
		t.Fatalf("non-positive proposed owner error = %v", err)
	}
}

func TestSingleOwnerExtractRequiresNewOwnerAndCompleteRetirement(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	candidate := cloneReport(t, semanticReport).Candidates[0]
	candidate.Verdict = "extract-neutral-contract"
	candidate.CurrentOwners = candidate.CurrentOwners[1:]
	candidate.Evidence = candidate.Evidence[1:]
	candidate.ProposedOwner = &proposedOwnerClaim{
		Path:                "shared/SPEC.md",
		State:               "new",
		Rationale:           "The shared domain seam does not yet have a normative SPEC.",
		NeutralityRationale: "The proposed owner is outside the pinned adapter-scope catalog.",
	}
	candidate.OwnershipPlan.CurrentOwners = candidate.OwnershipPlan.CurrentOwners[1:]
	retirement := &candidate.OwnershipPlan.CurrentOwners[0]
	for index := range retirement.Preservation.Requirements {
		retirement.Preservation.Requirements[index].TargetState = "planned"
	}
	for index := range retirement.Preservation.BDD {
		retirement.Preservation.BDD[index].TargetOwner = candidate.ProposedOwner.Path
	}

	active := stringSet(semanticReport.Scope.ActiveMembers)
	if err := validateFinding(candidate, false, active, semanticReport.Scope.AdapterScopes); err != nil {
		t.Fatalf("single-owner extract should be structurally valid: %v", err)
	}
	semanticReport.Candidates = []finding{candidate}
	semanticReport.Summary.ByVerdict = map[string]int{"extract-neutral-contract": 1}
	if err := validateAgainstInventory(semanticReport, inventoryReport); err != nil {
		t.Fatalf("single-owner extraction with full pinned retirement should validate: %v", err)
	}

	existing := candidate
	existing.ProposedOwner = &proposedOwnerClaim{Path: "two/SPEC.md", State: "existing", Rationale: "claimed existing seam", NeutralityRationale: "claimed neutral"}
	if err := validateFinding(existing, false, active, semanticReport.Scope.AdapterScopes); err == nil || !strings.Contains(err.Error(), "requires a new proposed owner") {
		t.Fatalf("single-owner existing proposal error=%v, want new-owner rejection", err)
	}

	retained := cloneReport(t, semanticReport).Candidates[0]
	retained.OwnershipPlan.CurrentOwners[0].Action = "retain"
	retained.OwnershipPlan.CurrentOwners[0].Preservation = nil
	if err := validateFinding(retained, false, active, semanticReport.Scope.AdapterScopes); err == nil || !strings.Contains(err.Error(), "must retire normative ownership") {
		t.Fatalf("single-owner retained plan error=%v, want complete-retirement rejection", err)
	}
}

func TestMergeNowRejectsNewProposedOwner(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	finding := cloneReport(t, semanticReport).Candidates[0]
	finding.ProposedOwner = &proposedOwnerClaim{Path: "shared/SPEC.md", State: "new", Rationale: "claimed new seam", NeutralityRationale: "claimed neutral"}
	err := validateFinding(finding, false, stringSet(semanticReport.Scope.ActiveMembers), semanticReport.Scope.AdapterScopes)
	if err == nil || !strings.Contains(err.Error(), "existing proposed owner") {
		t.Fatalf("merge-now new owner error=%v, want existing-owner rejection", err)
	}
}

func TestExtractNeutralContractRejectsIncompatibleRelationships(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	for _, relationship := range []string{"contradictory-observables", "same-vocabulary-only", "fixture-or-generated-copy"} {
		t.Run(relationship, func(t *testing.T) {
			candidate := cloneReport(t, semanticReport).Candidates[0]
			candidate.Verdict = "extract-neutral-contract"
			candidate.Relationship = relationship
			err := validateFinding(candidate, false, stringSet(semanticReport.Scope.ActiveMembers), semanticReport.Scope.AdapterScopes)
			if err == nil || !strings.Contains(err.Error(), "requires same or overlapping observables") {
				t.Fatalf("extract relationship %q error=%v, want compatibility rejection", relationship, err)
			}
		})
	}
}

func TestValidateFindingRejectsNewOwnerNestedBelowRootSpec(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	finding := cloneReport(t, semanticReport).Candidates[0]
	finding.Verdict = "extract-neutral-contract"
	finding.CurrentOwners[0].Path = "SPEC.md"
	finding.Evidence[0].Path = "SPEC.md"
	finding.ProposedOwner = &proposedOwnerClaim{
		Path:                "shared/SPEC.md",
		State:               "new",
		Rationale:           "A new neutral seam would own the shared behavior.",
		NeutralityRationale: "The proposed owner is outside harness configuration surfaces.",
	}

	err := validateFinding(finding, false, map[string]bool{"codex-cli": true, "pi-cli": true}, semanticReport.Scope.AdapterScopes)
	if err == nil || !strings.Contains(err.Error(), "beneath a current-owner directory") {
		t.Fatalf("validateFinding() error = %v, want root-owner nesting rejection", err)
	}
}

func TestValidateFindingRejectsCatalogAdapterOwner(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	for _, ownerPath := range []string{
		"agm/internal/codex/SPEC.md",
		"agm/internal/codexsession/SPEC.md",
	} {
		t.Run(ownerPath, func(t *testing.T) {
			finding := cloneReport(t, semanticReport).Candidates[0]
			finding.Verdict = "extract-neutral-contract"
			finding.ProposedOwner = &proposedOwnerClaim{
				Path:                ownerPath,
				State:               "new",
				Rationale:           "claimed product owner",
				NeutralityRationale: "claimed neutral",
			}

			err := validateFinding(finding, false, map[string]bool{"codex-cli": true, "pi-cli": true}, semanticReport.Scope.AdapterScopes)
			if err == nil || !strings.Contains(err.Error(), "harness-surface owner") {
				t.Fatalf("validateFinding() error = %v, want catalog-surface rejection", err)
			}
		})
	}
}

func TestCatalogClassifierDoesNotBlanketRejectRegistryParents(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	for _, ownerPath := range []string{
		"agm/internal/harnessregistry/SPEC.md",
		"agm/internal/agent/SPEC.md",
	} {
		if isHarnessSurfacePath(ownerPath, semanticReport.Scope.AdapterScopes) {
			t.Fatalf("isHarnessSurfacePath(%q) = true, want bounded catalog classification", ownerPath)
		}
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
			if err := validateFinding(finding, false, active, semanticReport.Scope.AdapterScopes); err == nil || !strings.Contains(err.Error(), "product SPEC current owners") {
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
			if err := validateFinding(finding, false, active, semanticReport.Scope.AdapterScopes); err == nil || !strings.Contains(err.Error(), "product SPEC proposed owner") {
				t.Fatalf("validateFinding() error = %v, want product-owner rejection", err)
			}
		})
	}
}

func TestRenderIsOfflineAndEscapesEvidence(t *testing.T) {
	report := validReport()
	report.NonCandidates = []finding{{
		ID: "SPEC-CLUSTER-002", Title: "native <adapter>", Verdict: "keep-separate", Relationship: "same-vocabulary-only", Classification: "native-adapter", Confidence: "confirmed", Strength: "moderate",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the native behavior"}}, SharedOutcome: "separate", MaterialDifferences: []string{"native path"}, Evidence: []evidence{{Path: "one/SPEC.md", Line: 2, RequirementID: "ONE-01", Excerpt: "<script>alert(1)</script>"}},
		BDD: bddImpact{Features: []string{"agm/test/bdd/features/example.feature"}, Consequence: "adapter-only"}, Recommendation: []string{"keep it"}, Risk: "bounded", Limitations: []string{"sentinel limitation"}, Decision: "retain", Boundary: "native behavior differs",
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
	for _, sentinel := range []string{"SPEC-CLUSTER-002", "same-vocabulary-only", "native-adapter", "one/SPEC.md", "native path", "agm/test/bdd/features/example.feature", "keep it", "sentinel limitation", "dependency corpus", strings.Repeat("b", 40)} {
		if !strings.Contains(output, sentinel) {
			t.Fatalf("renderer omitted %q", sentinel)
		}
	}
}

func TestRenderRetainsEscapedApplicabilityEvidenceRecords(t *testing.T) {
	report := validReport()
	applicabilityEvidence := evidence{
		Path:          "one/SPEC.md",
		Line:          17,
		RequirementID: "ONE-01",
		Excerpt:       `<img src=x onerror="alert('unsafe')">`,
	}
	report.Candidates = []finding{{
		ID: "SPEC-CLUSTER-001", Rank: 1, Title: "applicability evidence", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
		CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "owns the behavior"}, {Path: "two/SPEC.md", Rationale: "also owns the behavior"}}, ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "owns the canonical behavior"},
		SharedOutcome: "same", MaterialDifferences: []string{"none observed"}, Evidence: []evidence{{Path: "one/SPEC.md", Line: 1, RequirementID: "ONE-01", Excerpt: "one"}, {Path: "two/SPEC.md", Line: 1, RequirementID: "TWO-01", Excerpt: "two"}},
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

func TestRenderRetainsEveryEscapedPreservationSourceRecord(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	retirement := semanticReport.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation
	if len(retirement.Requirements) != 2 || slices.ContainsFunc(semanticReport.Candidates[0].Evidence, func(item evidence) bool {
		return item.RequirementID == "TWO-02"
	}) {
		t.Fatal("fixture must include a preservation-only TWO-02 source record")
	}
	second := retirement.Requirements[1].Source
	output := renderHTML(semanticReport, &inventoryReport)
	for _, sentinel := range []string{
		fmt.Sprintf("%s:%d", second.Path, second.Line),
		second.RequirementID,
		second.Excerpt,
	} {
		if !strings.Contains(output, sentinel) {
			t.Fatalf("renderer omitted preservation-only source field %q", sentinel)
		}
	}

	const unsafe = `<script>mapped & extra</script>`
	retirement.Requirements[1].Source.Excerpt = unsafe
	output = renderHTML(semanticReport, &inventoryReport)
	if strings.Contains(output, unsafe) || !strings.Contains(output, "&lt;script&gt;mapped &amp; extra&lt;/script&gt;") {
		t.Fatalf("renderer did not safely retain preservation source excerpt: %s", output)
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
	repo, _, semanticReport := auditFixture(t)
	gittest.HardenRepo(t, repo)
	writeTestFile(t, repo, "one/SPEC.md", "dirty working-tree bytes must survive the audit\n")
	writeTestFile(t, repo, "agm/test/bdd/features/shared.feature", "# staged feature bytes\nFeature: staged\n")
	gitTest(t, repo, "add", "agm/test/bdd/features/shared.feature")
	writeTestFile(t, repo, "agm/test/bdd/features/shared.feature", "# unstaged feature bytes\nFeature: unstaged\n")
	writeTestFile(t, repo, ".beads/issue-state.json", "{\"state\":\"unchanged\"}\n")
	writeTestFile(t, repo, "wayfinder/delivery-state.json", "{\"phase\":\"unchanged\"}\n")
	before := snapshotTargetRepository(t, repo)

	var inventoryOutput, stderr bytes.Buffer
	if code := run([]string{
		"inventory",
		"-repo", repo,
		"-repository", semanticReport.Snapshot.Repository,
		"-revision", semanticReport.Snapshot.Revision,
	}, &inventoryOutput, &stderr); code != 0 {
		t.Fatalf("inventory=%d: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("inventory stderr=%q, want empty", stderr.String())
	}
	var emittedInventory report
	if err := json.Unmarshal(inventoryOutput.Bytes(), &emittedInventory); err != nil {
		t.Fatalf("inventory stdout is not report JSON: %v", err)
	}
	if emittedInventory.Snapshot.Revision != semanticReport.Snapshot.Revision || len(emittedInventory.Inventory) != 3 {
		t.Fatalf("inventory stdout omitted pinned target content: snapshot=%#v files=%d", emittedInventory.Snapshot, len(emittedInventory.Inventory))
	}
	assertTargetRepositorySnapshot(t, "inventory", before, snapshotTargetRepository(t, repo))

	inputDir := realTempDir(t)
	inventoryPath := filepath.Join(inputDir, "inventory.json")
	semanticPath := filepath.Join(inputDir, "findings.json")
	if err := os.WriteFile(inventoryPath, inventoryOutput.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, semanticPath, semanticReport)

	stderr.Reset()
	var validateOutput bytes.Buffer
	proofArgs := []string{"-input", semanticPath, "-inventory", inventoryPath, "-repo", repo}
	if code := run(append([]string{"validate"}, proofArgs...), &validateOutput, &stderr); code != 0 {
		t.Fatalf("validate=%d: %s", code, stderr.String())
	}
	if got := validateOutput.String(); got != "specaudit: valid "+schemaVersion+" report\n" {
		t.Fatalf("validate stdout=%q, want success confirmation", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("validate stderr=%q, want empty", stderr.String())
	}
	assertTargetRepositorySnapshot(t, "validate", before, snapshotTargetRepository(t, repo))

	stderr.Reset()
	var renderOutput bytes.Buffer
	if code := run(append([]string{"render"}, proofArgs...), &renderOutput, &stderr); code != 0 {
		t.Fatalf("render=%d: %s", code, stderr.String())
	}
	if !strings.HasPrefix(renderOutput.String(), "<!doctype html>") || !strings.Contains(renderOutput.String(), semanticReport.Candidates[0].ID) {
		t.Fatalf("render stdout omitted the complete HTML audit artifact")
	}
	if stderr.Len() != 0 {
		t.Fatalf("render stderr=%q, want empty", stderr.String())
	}
	assertTargetRepositorySnapshot(t, "render", before, snapshotTargetRepository(t, repo))
}

type targetRepositorySnapshot struct {
	status                 string
	trackedWorktreeContent map[string][]byte
	indexPath              string
	indexIdentity          string
	indexContent           []byte
	head                   string
	headContent            []byte
	refs                   string
	relevantContractBytes  map[string][]byte
	externalStateBytes     map[string][]byte
}

func snapshotTargetRepository(t *testing.T, root string) targetRepositorySnapshot {
	t.Helper()
	status := gitTest(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	trackedWorktreeContent := readTargetRepositoryFiles(t, root, gitTest(t, root, "ls-files", "-z"))
	gitDir := strings.TrimSpace(gitTest(t, root, "rev-parse", "--absolute-git-dir"))
	indexPath := filepath.Join(gitDir, "index")
	indexContent, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	headContent, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	return targetRepositorySnapshot{
		status:                 status,
		trackedWorktreeContent: trackedWorktreeContent,
		indexPath:              indexPath,
		indexIdentity:          gitTest(t, root, "ls-files", "--stage", "-z"),
		indexContent:           indexContent,
		head:                   strings.TrimSpace(gitTest(t, root, "rev-parse", "HEAD")),
		headContent:            headContent,
		refs:                   gitTest(t, root, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(symref)"),
		relevantContractBytes: readTargetRepositoryFiles(t, root, strings.Join([]string{
			"one/SPEC.md",
			"two/SPEC.md",
			"three/SPEC.md",
			"agm/test/bdd/features/shared.feature",
			"agm/test/bdd/features/one-only.feature",
			"agm/test/bdd/features/two-only.feature",
			"agm/test/bdd/features/three-only.feature",
			"",
		}, "\x00")),
		externalStateBytes: readTargetRepositoryFiles(t, root, strings.Join([]string{
			".beads/issue-state.json",
			"wayfinder/delivery-state.json",
			"",
		}, "\x00")),
	}
}

func readTargetRepositoryFiles(t *testing.T, root, nulSeparatedPaths string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	for path := range strings.SplitSeq(nulSeparatedPaths, "\x00") {
		if path == "" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read target repository file %s: %v", path, err)
		}
		files[path] = content
	}
	return files
}

func assertTargetRepositorySnapshot(t *testing.T, command string, before, after targetRepositorySnapshot) {
	t.Helper()
	if before.status != after.status {
		t.Fatalf("%s changed target worktree status: before=%q after=%q", command, before.status, after.status)
	}
	if !reflect.DeepEqual(before.trackedWorktreeContent, after.trackedWorktreeContent) {
		t.Fatalf("%s changed tracked target worktree content", command)
	}
	if before.indexPath != after.indexPath || before.indexIdentity != after.indexIdentity || !bytes.Equal(before.indexContent, after.indexContent) {
		t.Fatalf("%s changed target index identity or content", command)
	}
	if before.head != after.head || !bytes.Equal(before.headContent, after.headContent) || before.refs != after.refs {
		t.Fatalf("%s changed target HEAD or refs", command)
	}
	if !reflect.DeepEqual(before.relevantContractBytes, after.relevantContractBytes) {
		t.Fatalf("%s changed relevant target SPEC or feature bytes", command)
	}
	if !reflect.DeepEqual(before.externalStateBytes, after.externalStateBytes) {
		t.Fatalf("%s changed target issue or delivery state bytes", command)
	}
}

func TestValidatePinsFindingsToGitResolvedInventory(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	inputDir := realTempDir(t)
	inventoryPath := filepath.Join(inputDir, "inventory.json")
	reportPath := filepath.Join(inputDir, "findings.json")
	writeJSON(t, inventoryPath, inventoryReport)
	writeJSON(t, reportPath, semanticReport)

	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-input", reportPath, "-inventory", inventoryPath, "-repo", repo}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("validate=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid "+schemaVersion) {
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

func TestReadReportRejectsNonExactSemanticPayloadKeys(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	data, err := json.Marshal(semanticReport)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		key  string
	}{
		{name: "uppercase inventory", key: "Inventory"},
		{name: "mixed case features", key: "fEaTuReS"},
		{name: "uppercase seeds", key: "SEEDS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]json.RawMessage
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			document[test.key] = json.RawMessage("null")
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(realTempDir(t), "semantic-with-non-exact-payload.json")
			if err := os.WriteFile(path, encoded, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err = readReport(path)
			if err == nil || !strings.Contains(err.Error(), `non-exact or unknown JSON object key "`+test.key+`"`) {
				t.Fatalf("readReport() error = %v, want exact-key rejection for %q", err, test.key)
			}
		})
	}
}

func TestReadReportRejectsNonExactNestedJSONKey(t *testing.T) {
	_, _, semanticReport := auditFixture(t)
	data, err := json.Marshal(semanticReport)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(document["snapshot"], &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot["Repository"] = snapshot["repository"]
	delete(snapshot, "repository")
	document["snapshot"], err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realTempDir(t), "semantic-with-non-exact-nested-key.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = readReport(path)
	if err == nil || !strings.Contains(err.Error(), `non-exact or unknown JSON object key "Repository"`) {
		t.Fatalf("readReport() error = %v, want generic nested exact-key rejection", err)
	}
}

func TestPinnedValidationAcceptsBDDReciprocityAcrossFeatures(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{
		"agm/test/bdd/features/shared.feature",
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
			nonCandidate.BDD.SharedContractFeature = ""
			nonCandidate.BDD.SharedContractScenario = nil
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
	_, _, semanticReport := auditFixture(t)
	semanticReport.Candidates[0].BDD.Features = []string{"agm/test/bdd/features/one-only.feature"}

	if err := validateReport(semanticReport); err == nil || !strings.Contains(err.Error(), "shared BDD feature") {
		t.Fatalf("semantic report error=%v, want shared-contract feature rejection", err)
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

func TestPositiveOwnershipPlanRejectsDuplicateRetentionAndDivergentApplicability(t *testing.T) {
	_, _, semanticReport := auditFixture(t)

	duplicateRetention := cloneReport(t, semanticReport)
	duplicateRetention.Candidates[0].OwnershipPlan.CurrentOwners[1].Action = "retain"
	duplicateRetention.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation = nil
	if err := validateReport(duplicateRetention); err == nil || !strings.Contains(err.Error(), "must retire normative ownership") {
		t.Fatalf("duplicate retention error=%v, want retirement requirement", err)
	}

	divergentMatrix := cloneReport(t, semanticReport)
	divergentMatrix.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation.Applicability[0].Disposition = "adapted"
	if err := validateReport(divergentMatrix); err == nil || !strings.Contains(err.Error(), "copy the finding applicability basis and matrix exactly") {
		t.Fatalf("divergent applicability error=%v, want exact-copy rejection", err)
	}
}

func TestPinnedValidationRejectsIncompleteSharedFeatureAndPreservationTargets(t *testing.T) {
	repo, inventoryReport, semanticReport := auditFixture(t)

	oneSided := cloneReport(t, inventoryReport)
	for index := range oneSided.Features {
		if oneSided.Features[index].Path == "agm/test/bdd/features/shared.feature" {
			oneSided.Features[index].RelatedSpecs = []string{"one/SPEC.md"}
		}
	}
	if err := validateAgainstInventory(semanticReport, oneSided); err == nil || !strings.Contains(err.Error(), "shared BDD feature") {
		t.Fatalf("one-sided shared feature error=%v, want all-owner reciprocity rejection", err)
	}

	unknownTarget := cloneReport(t, semanticReport)
	unknownTarget.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation.Requirements[0].TargetID = "MISSING-99"
	if err := validateReport(unknownTarget); err != nil {
		t.Fatalf("unknown target should require pinned validation, got structural error: %v", err)
	}
	if err := validateAgainstInventory(unknownTarget, inventoryReport); err == nil || !strings.Contains(err.Error(), "target ID") {
		t.Fatalf("unknown target error=%v, want pinned target-ID rejection", err)
	}

	if err := validateInventoryAgainstRepo(inventoryReport, repo); err != nil {
		t.Fatalf("fixture inventory no longer matches repo: %v", err)
	}
}

func TestPinnedValidationRequiresEveryPinnedReciprocalFeatureInRetirementPlan(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	retirement := semanticReport.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation
	retirement.BDD = retirement.BDD[:1]
	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("missing BDD mapping should require pinned validation, got structural error: %v", err)
	}
	if err := validateAgainstInventory(semanticReport, inventoryReport); err == nil || !strings.Contains(err.Error(), "must exactly cover all pinned reciprocal features") {
		t.Fatalf("incomplete pinned BDD mappings error=%v, want exact coverage rejection", err)
	}
}

func TestPinnedValidationRequiresEveryRequirementInRetiredSpec(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	retirement := semanticReport.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation
	retirement.Requirements = retirement.Requirements[:1]
	if err := validateReport(semanticReport); err != nil {
		t.Fatalf("partial retirement should reach pinned validation: %v", err)
	}
	err := validateAgainstInventory(semanticReport, inventoryReport)
	if err == nil || !strings.Contains(err.Error(), "exactly cover all pinned requirements") {
		t.Fatalf("partial retirement error=%v, want all-pinned-requirement rejection", err)
	}
}

func TestSharedContractScenarioProofIsExactAndCrossMember(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	tests := []struct {
		name   string
		mutate func(*featureFile, *finding)
		want   string
	}{
		{
			name: "header-only feature",
			mutate: func(feature *featureFile, _ *finding) {
				feature.Scenarios = nil
			},
			want: "is absent from the pinned feature inventory",
		},
		{
			name: "no observable Then outcome",
			mutate: func(feature *featureFile, _ *finding) {
				feature.Scenarios[0].Outcomes = nil
			},
			want: "observable outline",
		},
		{
			name: "member placeholder unused",
			mutate: func(feature *featureFile, _ *finding) {
				feature.Scenarios[0].UsesMemberPlaceholder = false
			},
			want: "observable outline",
		},
		{
			name: "incomplete cases",
			mutate: func(feature *featureFile, _ *finding) {
				feature.Scenarios[0].MemberCases = feature.Scenarios[0].MemberCases[:1]
			},
			want: "exactly cover applicable members",
		},
		{
			name: "duplicate cases",
			mutate: func(feature *featureFile, _ *finding) {
				feature.Scenarios[0].MemberCases = append(feature.Scenarios[0].MemberCases, feature.Scenarios[0].MemberCases[0])
			},
			want: "invalid or duplicate member cases",
		},
		{
			name: "forged exact reference",
			mutate: func(_ *featureFile, finding *finding) {
				finding.BDD.SharedContractScenario.Line++
			},
			want: "is absent from the pinned feature inventory",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inventory := cloneReport(t, inventoryReport)
			semantic := cloneReport(t, semanticReport)
			feature := inventoryFeaturePointer(t, &inventory, semantic.Candidates[0].BDD.SharedContractFeature)
			test.mutate(feature, &semantic.Candidates[0])
			err := validateAgainstInventory(semantic, inventory)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("shared scenario error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestPositiveFindingApplicabilityPreservesSharedProofAcrossClassifications(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)

	t.Run("implementation-only matrix uses pinned owners and shared proof", func(t *testing.T) {
		inventory := cloneReport(t, inventoryReport)
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		additionalContext := inventoryRequirement(t, inventory, "three/SPEC.md")
		finding.Classification = "implementation-detail"
		finding.ApplicabilityBasis = "implementation-only"
		finding.ApplicabilityRationale = "The two pinned packages are the complete implementation set for this observable."
		finding.Applicability = []applicability{
			{Member: "one-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[0], evidenceForRequirement("three/SPEC.md", additionalContext)}},
			{Member: "two-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[1]}},
		}
		finding.OwnershipPlan.CurrentOwners[1].Preservation.ApplicabilityBasis = finding.ApplicabilityBasis
		finding.OwnershipPlan.CurrentOwners[1].Preservation.Applicability = finding.Applicability
		feature := inventoryFeaturePointer(t, &inventory, finding.BDD.SharedContractFeature)
		feature.Scenarios[0].MemberColumn = "member"
		feature.Scenarios[0].MemberCases = []scenarioMemberCase{
			{Line: 12, Member: "one-package", Source: "examples-member"},
			{Line: 13, Member: "two-package", Source: "examples-member"},
		}

		if err := validateReport(report); err != nil {
			t.Fatalf("validateReport() implementation-only positive error=%v", err)
		}
		if err := validateAgainstInventory(report, inventory); err != nil {
			t.Fatalf("validateAgainstInventory() implementation-only positive error=%v", err)
		}
	})

	t.Run("implementation-only matrix cannot relabel active harnesses", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		finding.ApplicabilityBasis = "implementation-only"
		finding.Applicability = []applicability{
			{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{finding.Evidence[0]}},
			{Member: "pi-cli", Disposition: "supported", Evidence: []evidence{finding.Evidence[1]}},
		}
		finding.OwnershipPlan.CurrentOwners[1].Preservation.ApplicabilityBasis = finding.ApplicabilityBasis
		finding.OwnershipPlan.CurrentOwners[1].Preservation.Applicability = finding.Applicability

		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "active harness member") {
			t.Fatalf("validateReport() error=%v, want active-harness relabeling rejection", err)
		}
	})

	t.Run("implementation-only shared proof cannot use a harness column", func(t *testing.T) {
		inventory := cloneReport(t, inventoryReport)
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		finding.ApplicabilityBasis = "implementation-only"
		finding.Applicability = []applicability{
			{Member: "one-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[0]}},
			{Member: "two-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[1]}},
		}
		finding.OwnershipPlan.CurrentOwners[1].Preservation.ApplicabilityBasis = finding.ApplicabilityBasis
		finding.OwnershipPlan.CurrentOwners[1].Preservation.Applicability = finding.Applicability

		if err := validateReport(report); err != nil {
			t.Fatalf("validateReport() should reach pinned shared-proof validation: %v", err)
		}
		err := validateAgainstInventory(report, inventory)
		if err == nil || !strings.Contains(err.Error(), "member examples column") {
			t.Fatalf("validateAgainstInventory() error=%v, want harness-column rejection", err)
		}
	})

	t.Run("implementation-only matrix requires two implementations", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		finding.ApplicabilityBasis = "implementation-only"
		finding.Applicability = []applicability{{Member: "one-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[0]}}}
		finding.OwnershipPlan.CurrentOwners[1].Preservation.ApplicabilityBasis = finding.ApplicabilityBasis
		finding.OwnershipPlan.CurrentOwners[1].Preservation.Applicability = finding.Applicability

		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "at least two implementations") {
			t.Fatalf("validateReport() error=%v, want bounded implementation-matrix rejection", err)
		}
	})

	t.Run("implementation-only matrix cites every current owner", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		finding.ApplicabilityBasis = "implementation-only"
		finding.Applicability = []applicability{
			{Member: "one-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[0]}},
			{Member: "two-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[0]}},
		}
		finding.OwnershipPlan.CurrentOwners[1].Preservation.ApplicabilityBasis = finding.ApplicabilityBasis
		finding.OwnershipPlan.CurrentOwners[1].Preservation.Applicability = finding.Applicability

		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "cite every current owner") {
			t.Fatalf("validateReport() error=%v, want implementation-owner evidence rejection", err)
		}
	})

	t.Run("harness-surface owner cannot use implementation-only matrix", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		const oldOwner = "two/SPEC.md"
		const harnessOwner = ".codex/SPEC.md"
		finding.CurrentOwners[1].Path = harnessOwner
		for index := range finding.Evidence {
			if finding.Evidence[index].Path == oldOwner {
				finding.Evidence[index].Path = harnessOwner
			}
		}
		retired := &finding.OwnershipPlan.CurrentOwners[1]
		retired.Path = harnessOwner
		for index := range retired.Preservation.Requirements {
			retired.Preservation.Requirements[index].Source.Path = harnessOwner
		}
		for index := range retired.Preservation.BDD {
			retired.Preservation.BDD[index].SourceOwner = harnessOwner
		}
		finding.ApplicabilityBasis = "implementation-only"
		finding.Applicability = []applicability{
			{Member: "one-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[0]}},
			{Member: "codex-package", Disposition: "supported", Evidence: []evidence{finding.Evidence[1]}},
		}
		retired.Preservation.ApplicabilityBasis = finding.ApplicabilityBasis
		retired.Preservation.Applicability = finding.Applicability

		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "harness-surface owner") {
			t.Fatalf("validateReport() error=%v, want harness-surface applicability rejection", err)
		}
	})

	t.Run("split local features cannot impersonate shared proof", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		finding := &report.Candidates[0]
		finding.Classification = "implementation-detail"
		finding.BDD.Features = []string{"agm/test/bdd/features/one-only.feature", "agm/test/bdd/features/two-only.feature"}
		finding.BDD.SharedContractFeature = "agm/test/bdd/features/one-only.feature"
		finding.BDD.SharedContractScenario = &scenarioRef{Line: 1, Name: "Local implementation behavior"}
		retirement := finding.OwnershipPlan.CurrentOwners[1].Preservation
		retirement.BDD = append(retirement.BDD, bddPreservation{
			Feature:     finding.BDD.SharedContractFeature,
			SourceOwner: "two/SPEC.md",
			TargetOwner: "one/SPEC.md",
		})
		if err := validateReport(report); err != nil {
			t.Fatalf("split-feature bypass should reach pinned validation: %v", err)
		}
		err := validateAgainstInventory(report, inventoryReport)
		if err == nil || !strings.Contains(err.Error(), "must reciprocally link current owner") {
			t.Fatalf("validateAgainstInventory() error=%v, want all-owner shared-feature rejection", err)
		}
	})

	t.Run("classification vocabulary does not weaken valid proof", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		report.Candidates[0].Classification = "implementation-detail"
		if err := validateReport(report); err != nil {
			t.Fatalf("classification-independent positive proof should be structurally valid: %v", err)
		}
		if err := validateAgainstInventory(report, inventoryReport); err != nil {
			t.Fatalf("classification-independent positive proof should match pinned evidence: %v", err)
		}
	})

	t.Run("retirement always preserves the shared feature", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		report.Candidates[0].Classification = "implementation-detail"
		retirement := report.Candidates[0].OwnershipPlan.CurrentOwners[1].Preservation
		retirement.BDD = retirement.BDD[1:]
		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "must include bdd.shared_contract_feature") {
			t.Fatalf("validateReport() error=%v, want classification-independent shared-feature preservation", err)
		}
	})
}

func TestRenderIncludesEscapedPinnedSelectedScenarioProof(t *testing.T) {
	_, inventoryReport, semanticReport := auditFixture(t)
	feature := inventoryFeaturePointer(t, &inventoryReport, semanticReport.Candidates[0].BDD.SharedContractFeature)
	feature.Scenarios[0].Outcomes[0].Text = `identity <img src=x onerror=alert(1)> & remains pinned`
	feature.Scenarios[0].MemberCases[0].Member = `codex-cli<script>alert(2)</script>`

	rendered := renderHTML(semanticReport, &inventoryReport)
	for _, want := range []string{
		"Pinned selected-scenario proof",
		"<strong>Kind:</strong> <code>scenario-outline</code>",
		"<strong>Member column:</strong> <code>harness</code>",
		"<strong>Uses member placeholder:</strong> <code>true</code>",
		"<code>line 8</code>",
		"identity &lt;img src=x onerror=alert(1)&gt; &amp; remains pinned",
		"codex-cli&lt;script&gt;alert(2)&lt;/script&gt;",
		"<td>12</td>",
		"examples-harness",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered selected-scenario proof omitted %q", want)
		}
	}
	for _, unsafe := range []string{`<img src=x onerror=alert(1)>`, `<script>alert(2)</script>`} {
		if strings.Contains(rendered, unsafe) {
			t.Fatalf("rendered selected-scenario proof leaked unsafe sentinel %q", unsafe)
		}
	}
}

func TestHarnessSurfaceClassifierIsBoundedAndIncludesKnownAliases(t *testing.T) {
	scopes := []adapterScope{
		{ID: "claude-code", Kind: "harness", Lifecycle: "active", Names: []string{"claude-code", ".claude", ".claude-plugin"}, Evidence: []scopeEvidence{{Path: inPackageHarnessRegistryPath, Line: 1, Excerpt: "claude-code"}}},
		{ID: "codex-cli", Kind: "harness", Lifecycle: "active", Names: []string{"codex-cli", ".codex"}, Evidence: []scopeEvidence{{Path: inPackageHarnessRegistryPath, Line: 1, Excerpt: "codex-cli"}}},
		{ID: "pi-cli", Kind: "harness", Lifecycle: "active", Names: []string{"pi-cli", ".pi"}, Evidence: []scopeEvidence{{Path: inPackageHarnessRegistryPath, Line: 1, Excerpt: "pi-cli"}}},
		{ID: "openai", Kind: "compatibility-adapter", Lifecycle: "compatibility", Names: []string{"openai"}, Evidence: []scopeEvidence{{Path: openAIAdapterSourcePath, Line: 1, Excerpt: "openai"}}},
	}
	for _, path := range []string{
		"agm/internal/codexsession/SPEC.md",
		"agm/internal/codexarchive/SPEC.md",
		"agm/internal/piadapter/SPEC.md",
		"agm/internal/claudeui/SPEC.md",
		"agm/internal/codex/SPEC.md",
		"agm/internal/pi/SPEC.md",
		"agm/internal/agent/openai/SPEC.md",
	} {
		if !isHarnessSurfacePath(path, scopes) {
			t.Fatalf("known harness surface %q was accepted", path)
		}
	}
	for _, path := range []string{"internal/domain/SPEC.md", "pkg/contracts/SPEC.md", "internal/pipeline/SPEC.md"} {
		if isHarnessSurfacePath(path, scopes) {
			t.Fatalf("ordinary implementation path %q was incorrectly classified as a harness surface", path)
		}
	}
	for _, member := range []string{"codex-cli", "codex", ".codex", "pi", ".pi", "claude-hooks"} {
		if !isActiveHarnessMemberName(member, scopes) {
			t.Fatalf("active harness member or alias %q was accepted as an implementation label", member)
		}
	}
	for _, member := range []string{"one-package", "two-package", "openai"} {
		if isActiveHarnessMemberName(member, scopes) {
			t.Fatalf("ordinary implementation label %q was classified as an active harness", member)
		}
	}
	if !isStrictDescendantOfCurrentOwner("one/nested/SPEC.md", []string{"one/SPEC.md", "two/SPEC.md"}) {
		t.Fatal("new child owner should be rejected beneath current owner directory")
	}
	if isStrictDescendantOfCurrentOwner("shared/SPEC.md", []string{"one/SPEC.md", "two/SPEC.md"}) {
		t.Fatal("sibling neutral owner should not be rejected as a descendant")
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
			name: "semantic substitutes adapter catalog",
			mutate: func(semantic *report, _ *report) {
				semantic.Scope.AdapterScopes[0].Names = append(
					[]string(nil),
					semantic.Scope.AdapterScopes[0].Names[:len(semantic.Scope.AdapterScopes[0].Names)-1]...,
				)
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
			report.Candidates[0].ApplicabilityBasis = "implementation-only"
			report.Candidates[0].ApplicabilityRationale = "claimed to exclude every active harness"
			report.Candidates[0].Applicability = nil

			err := validateReport(report)
			if err == nil || !strings.Contains(err.Error(), "proposes harness-surface owner") {
				t.Fatalf("validateReport() error = %v, want harness-surface proposed-owner rejection", err)
			}
		})
	}
	t.Run("new proposed nested harness owner", func(t *testing.T) {
		report := cloneReport(t, semanticReport)
		report.Candidates[0].Verdict = "extract-neutral-contract"
		report.Summary.ByVerdict = map[string]int{"extract-neutral-contract": 1}
		report.Candidates[0].ProposedOwner = &proposedOwnerClaim{
			Path:                "wayfinder/.claude-plugin/new/SPEC.md",
			State:               "new",
			Rationale:           "claimed to create an implementation-only owner",
			NeutralityRationale: "claimed neutral",
		}
		report.Candidates[0].ApplicabilityBasis = "implementation-only"
		report.Candidates[0].ApplicabilityRationale = "claimed to exclude every active harness"
		report.Candidates[0].Applicability = nil

		err := validateReport(report)
		if err == nil || !strings.Contains(err.Error(), "proposes harness-surface owner") {
			t.Fatalf("validateReport() error = %v, want proposed harness-surface rejection", err)
		}
	})
}

func auditFixture(t *testing.T) (string, report, report) {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gitTest(t, repo, "config", "user.email", "test@example.com")
	gitTest(t, repo, "config", "user.name", "Test")
	writeTestFile(t, repo, "agm/internal/agent/harnesses.go", "package agent\nvar activeHarnesses = []string{\"codex-cli\", \"pi-cli\"}\nvar deprecatedHarnesses = []string{\"claude-code\"}\n")
	writeTestAliasAndConfigMetadata(t, repo, []string{"codex-cli", "pi-cli"}, []string{"claude-code"})
	writeTestFile(t, repo, marketplaceSurfaceSourcePath, testMarketplaceMetadata)
	writeTestFile(t, repo, "one/SPEC.md", "# One\n\n**ONE-01** When a request runs, the system shall preserve identity.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/shared.feature`\n- Feature: `agm/test/bdd/features/one-only.feature`\n")
	writeTestFile(t, repo, "two/SPEC.md", "# Two\n\n**TWO-01** When a request runs, the system shall preserve identity.\n\n**TWO-02** When identity is preserved, the system shall retain its canonical reference.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/shared.feature`\n- Feature: `agm/test/bdd/features/two-only.feature`\n")
	writeTestFile(t, repo, "three/SPEC.md", "# Three\n\n**THREE-01** When a separate request runs, the system shall emit an unrelated metric.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/three-only.feature`\n")
	writeTestFile(t, repo, "agm/test/bdd/features/shared.feature", strings.Join([]string{
		"# SPEC: one/SPEC.md",
		"# RELATED-SPEC: two/SPEC.md",
		"Feature: Shared identity",
		"",
		"  Scenario Outline: Active members preserve request identity",
		"    Given the <harness> harness is active",
		"    When a shared request runs",
		"    Then the request identity is preserved",
		"",
		"    Examples:",
		"      | harness   |",
		"      | codex-cli |",
		"      | pi-cli    |",
	}, "\n"))
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
	secondReference := inventoryRequirementByID(t, inventoryReport, "two/SPEC.md", "TWO-02")
	semantic := report{
		SchemaVersion: schemaVersion,
		Snapshot: snapshot{
			Repository:          inventoryReport.Snapshot.Repository,
			Revision:            inventoryReport.Snapshot.Revision,
			RevisionCommittedAt: inventoryReport.Snapshot.RevisionCommittedAt,
			GeneratedAt:         "2026-07-31T12:00:00Z",
		},
		Scope: inventoryReport.Scope,
		Summary: summary{
			SpecFiles: 3, Requirements: 4, Diagnostics: 0, CandidateCount: 1,
			ByVerdict: map[string]int{"merge-now": 1},
		},
		Methodology: methodology{Collector: "go run ./tools/specaudit inventory", SeedKinds: []string{"exact-body"}, SemanticReview: "source and BDD review", RuntimeStatus: runtimeStatusUnverified, GitEvidenceTrust: gitEvidenceTrustDisclosure, GitTrustInputs: inventoryReport.Methodology.GitTrustInputs, Reproduce: []string{"go run ./tools/specaudit validate fixture"}},
		Candidates: []finding{{
			ID: "SPEC-CLUSTER-001", Rank: 1, Title: "Shared identity", Verdict: "merge-now", Relationship: "same-observable", Classification: "shared-contract", Confidence: "confirmed", Strength: "strong",
			CurrentOwners: []ownerClaim{{Path: "one/SPEC.md", Rationale: "ONE-01 normatively claims the shared request outcome."}, {Path: "two/SPEC.md", Rationale: "TWO-01 independently claims the same request outcome."}}, OwnershipCompleteness: "The exact-body seed and repository search found only these two normative paths.",
			ProposedOwner: &proposedOwnerClaim{Path: "one/SPEC.md", State: "existing", Rationale: "ONE-01 already states the complete shared observable.", NeutralityRationale: "The top-level product contract is outside every harness registration surface."},
			OwnershipPlan: &ownershipPlan{Approval: "pending-maintainer-approval", CurrentOwners: []ownershipPlanOwner{
				{Path: "one/SPEC.md", Action: "retain", Rationale: "The existing neutral owner remains normative."},
				{Path: "two/SPEC.md", Action: "retire-normative-ownership", Rationale: "The duplicate promise will become a canonical reference.", Preservation: &preservationPlan{
					Requirements: []requirementPreservation{
						{Source: evidence{Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}, TargetID: first.ID, TargetState: "existing", Strategy: "canonical-reference"},
						{Source: evidence{Path: "two/SPEC.md", Line: secondReference.Line, RequirementID: secondReference.ID, Excerpt: secondReference.Excerpt}, TargetID: first.ID, TargetState: "existing", Strategy: "canonical-reference"},
					},
					BDD: []bddPreservation{
						{Feature: "agm/test/bdd/features/shared.feature", SourceOwner: "two/SPEC.md", TargetOwner: "one/SPEC.md"},
						{Feature: "agm/test/bdd/features/two-only.feature", SourceOwner: "two/SPEC.md", TargetOwner: "one/SPEC.md"},
					},
					ApplicabilityBasis: "active-members",
					Applicability: []applicability{
						{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}}},
						{Member: "pi-cli", Disposition: "supported", Evidence: []evidence{{Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}}},
					},
				}},
			}},
			SharedOutcome: "Requests preserve identity.", MaterialDifferences: []string{"Only the owner path differs."}, Evidence: []evidence{{Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}, {Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}},
			ApplicabilityBasis: "active-members", ApplicabilityRationale: "The shared contract applies to both pinned active members.",
			Applicability: []applicability{
				{Member: "codex-cli", Disposition: "supported", Evidence: []evidence{{Path: "one/SPEC.md", Line: first.Line, RequirementID: first.ID, Excerpt: first.Excerpt}}},
				{Member: "pi-cli", Disposition: "supported", Evidence: []evidence{{Path: "two/SPEC.md", Line: second.Line, RequirementID: second.ID, Excerpt: second.Excerpt}}},
			},
			BDD: bddImpact{Features: []string{"agm/test/bdd/features/shared.feature"}, SharedContractFeature: "agm/test/bdd/features/shared.feature", SharedContractScenario: &scenarioRef{Line: 5, Name: "Active members preserve request identity"}, Consequence: "merge"}, Recommendation: []string{"Keep ONE-01 as canonical."}, Risk: "Traceability could be lost.", Decision: "Approve one owner.",
		}},
		NonCandidates: []finding{}, Limitations: append([]string{}, inventoryReport.Limitations...),
	}
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
	return cloned
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

func inventoryRequirementByID(t *testing.T, source report, path, id string) requirement {
	t.Helper()
	for _, file := range source.Inventory {
		if file.Path != path {
			continue
		}
		for _, item := range file.Requirements {
			if item.ID == id {
				return item
			}
		}
	}
	t.Fatalf("inventory has no requirement %s in %s", id, path)
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

func inventoryFeaturePointer(t *testing.T, source *report, path string) *featureFile {
	t.Helper()
	for index := range source.Features {
		if source.Features[index].Path == path {
			return &source.Features[index]
		}
	}
	t.Fatalf("inventory has no feature for %s", path)
	return nil
}

func validReport() report {
	return report{
		SchemaVersion: schemaVersion,
		Snapshot:      snapshot{Repository: "owner/repo", Revision: strings.Repeat("a", 40), RevisionCommittedAt: "2026-07-30T00:00:00Z", GeneratedAt: "2026-07-31T00:00:00Z"},
		Scope: scope{Roots: []string{"."}, Excluded: []exclusion{}, ActiveMembers: []string{"codex-cli"}, AdapterScopes: []adapterScope{
			{ID: "codex-cli", Kind: "harness", Lifecycle: "active", Names: []string{"codex-cli", ".codex"}, Evidence: []scopeEvidence{{Path: inPackageHarnessRegistryPath, Line: 1, Excerpt: "codex-cli"}}},
		}},
		Summary:     summary{SpecFiles: 1, Requirements: 1, CandidateCount: 0, ByVerdict: map[string]int{}},
		Methodology: methodology{Collector: "test", SeedKinds: []string{"exact-body"}, SemanticReview: "review", RuntimeStatus: runtimeStatusUnverified, GitEvidenceTrust: gitEvidenceTrustDisclosure, GitTrustInputs: testGitTrustInputs(), Reproduce: []string{"go run ./tools/specaudit inventory -repo . -revision abc"}},
		Candidates:  []finding{}, NonCandidates: []finding{}, Limitations: []string{},
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
