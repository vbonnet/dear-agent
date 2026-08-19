package specguard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestEvaluateRejectsMalformedRequests(t *testing.T) {
	t.Parallel()
	tests := []Request{
		{},
		{Repository: ".", Mode: "other"},
		{Repository: ".", Mode: ModeStaged, Base: "main"},
		{Repository: ".", Mode: ModeCommitted},
		{Repository: ".", Mode: ModeCommitted, Base: "--upload-pack=bad"},
		{Repository: ".", Mode: ModeCommitted, Base: "main..other"},
	}
	for _, request := range tests {
		t.Run(fmt.Sprintf("%s-%s", request.Mode, request.Base), func(t *testing.T) {
			result := Evaluate(context.Background(), request)
			assertDecisionAndCode(t, result, DecisionBlock, "invalid-input")
		})
	}
}

func TestNoGovernedChangesAreAllowed(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("README.md", "ordinary documentation change\n")
	fixture.git("add", "--", "README.md")

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if result.Decision != DecisionAllow || len(result.Changed) != 0 || len(result.Findings) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestChangedSpecRequiresStrictEARSNeutralOwnerAndReciprocalBDD(t *testing.T) {
	t.Parallel()
	t.Run("harness registration owners", func(t *testing.T) {
		for _, specPath := range []string{
			".codex/SPEC.md",
			"claude-code/SPEC.md",
			"codex-cli/SPEC.md",
			"opencode-cli/SPEC.md",
			"pi-cli/SPEC.md",
			"agy-cli/SPEC.md",
			"antigravity/SPEC.md",
			"wayfinder/.claude-plugin/new/SPEC.md",
			"product/.gemini/SPEC.md",
			"agm/harnesses/pi/SPEC.md",
			"agm/harness/future-harness/SPEC.md",
			"agm/harnesses/future-cli/SPEC.md",
			"agm/internal/.codex/SPEC.md",
			"agm/cmd/.claude/SPEC.md",
			"agm/internal/harnesses/future-cli/SPEC.md",
			"agm/internal/plugins/example/SPEC.md",
			"agm/cmd/future-plugin/SPEC.md",
			"agm/agm-plugin/commands/SPEC.md",
		} {
			t.Run(strings.ReplaceAll(specPath, "/", "_"), func(t *testing.T) {
				fixture := newGuardRepository(t)
				fixture.write(specPath, validSpec("agm/test/bdd/features/harness.feature"))
				fixture.write("agm/test/bdd/features/harness.feature", validFeature(specPath))
				fixture.git("add", "--", specPath, "agm/test/bdd/features/harness.feature")
				result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
				assertDecisionAndCode(t, result, DecisionBlock, "non-neutral-spec-owner")
			})
		}
	})

	t.Run("logical internal owners", func(t *testing.T) {
		for _, specPath := range []string{
			".dear-agent/SPEC.md",
			"agm/internal/agent/SPEC.md",
			"internal/codexarchive/SPEC.md",
		} {
			t.Run(strings.ReplaceAll(specPath, "/", "_"), func(t *testing.T) {
				fixture := newGuardRepository(t)
				fixture.write(specPath, validSpec("agm/test/bdd/features/logical.feature"))
				fixture.write("agm/test/bdd/features/logical.feature", validFeature(specPath))
				fixture.git("add", "--", specPath, "agm/test/bdd/features/logical.feature")
				result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
				if result.Decision != DecisionReminder || len(result.Findings) != 0 {
					t.Fatalf("result = %#v, want reminder without deterministic findings", result)
				}
			})
		}
	})

	t.Run("missing reciprocal feature link", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/other/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "nonreciprocal-bdd-link")
	})

	t.Run("two primary owners", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("pkg/other/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\n# SPEC: pkg/other/SPEC.md\nFeature: Example\n  Scenario: Works\n    Given an observable condition\n")
		fixture.git("add", "--", "pkg/example/SPEC.md", "pkg/other/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "bdd-primary-owner-count")
	})

	// A file that merely ends in ".feature" outside the runner's tree is
	// referenced by no SPEC and cannot be executed, so governing it would emit
	// findings nobody can act on. The runner boundary, not the suffix, decides.
	t.Run("nonexecutable standalone feature is ungoverned", func(t *testing.T) {
		for _, ungoverned := range []string{
			"docs/example.feature",
			"agm/test/bdd/features/nested/example.feature",
			"internal/example/testdata/example.feature",
		} {
			t.Run(ungoverned, func(t *testing.T) {
				fixture := newGuardRepository(t)
				fixture.write(ungoverned, "Feature: Example\n  Scenario: Works\n    Given a condition\n")
				fixture.git("add", "--", ungoverned)
				result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
				if hasCode(result, "bdd-primary-owner-count") {
					t.Fatalf("nonexecutable feature %q was governed: %+v", ungoverned, result.Findings)
				}
			})
		}
	})

	// The boundary must not become an escape hatch: an executable feature with
	// no primary owner is still governed.
	t.Run("executable standalone feature stays governed", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("agm/test/bdd/features/example.feature", "Feature: Example\n  Scenario: Works\n    Given an observable condition\n    Then the outcome is preserved\n")
		fixture.git("add", "--", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if !hasCode(result, "bdd-primary-owner-count") {
			t.Fatalf("executable feature escaped governance: %+v", result.Findings)
		}
	})

	t.Run("Given and When only are not runnable BDD", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: No assertion\n    Given an observable condition\n    When the contract is evaluated\n")
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "scenario-without-assertion")
	})

	t.Run("Scenario Outline requires nonempty Examples", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Missing examples\n    Given an observable <condition>\n    Then the outcome is preserved\n")
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "outline-without-examples")
	})
}

func TestHarnessRegistrationOwnerBoundary(t *testing.T) {
	t.Parallel()
	for _, specPath := range []string{
		".agents/SPEC.md",
		".claude/SPEC.md",
		".codex/SPEC.md",
		".gemini/SPEC.md",
		".opencode/SPEC.md",
		".pi/SPEC.md",
		"claude-code/SPEC.md",
		"claudecode/SPEC.md",
		"codex-cli/SPEC.md",
		"codex_cli/SPEC.md",
		"opencode-cli/SPEC.md",
		"opencodecli/SPEC.md",
		"pi-cli/SPEC.md",
		"picli/SPEC.md",
		"agy-cli/SPEC.md",
		"antigravity/SPEC.md",
		"product/.claude-code/SPEC.md",
		"wayfinder/.claude-plugin/new/SPEC.md",
		"product/.codex-plugin/SPEC.md",
		"Internal/.codex/SPEC.md",
		"agm/internal/.codex/SPEC.md",
		"agm/cmd/.claude/SPEC.md",
		"agm/internal/plugins/example/SPEC.md",
		"agm/cmd/future-plugin/SPEC.md",
		"agm/harness/codex/SPEC.md",
		"agm/harness/codex-cli/SPEC.md",
		"agm/harness/future-harness/SPEC.md",
		"agm/harnesses/pi/SPEC.md",
		"agm/harnesses/future-cli/SPEC.md",
		"agm/internal/harnesses/future-cli/SPEC.md",
		"agm/youtube-plugin/SPEC.md",
	} {
		if !isHarnessRegistrationOwner(specPath) {
			t.Errorf("registration owner %q was accepted", specPath)
		}
	}
	for _, specPath := range []string{
		".dear-agent/SPEC.md",
		"agm/internal/agent/SPEC.md",
		"internal/codexarchive/SPEC.md",
		"agm/internal/codex-cli/SPEC.md",
		"agm/cmd/claude/SPEC.md",
		"agm/cmd/opencode-cli/SPEC.md",
		"future-cli/SPEC.md",
		"pkg/plugin/SPEC.md",
	} {
		if isHarnessRegistrationOwner(specPath) {
			t.Errorf("logical owner %q was rejected", specPath)
		}
	}
}

func TestBareTopLevelHarnessAuthorityIsPinnedClosed(t *testing.T) {
	t.Parallel()
	if !isHarnessRegistrationOwner("codex-cli/SPEC.md") {
		t.Fatal("pinned canonical harness owner was accepted")
	}
	if isHarnessRegistrationOwner("future-cli/SPEC.md") {
		t.Fatal("unknown bare top-level owner was classified without a pinned-authority update")
	}
	if !isHarnessRegistrationOwner("harnesses/future-cli/SPEC.md") {
		t.Fatal("structural harness collection incorrectly depended on the pinned authority")
	}
}

func TestGovernedDeletionValidatesSurvivingGraphAndAllowsReviewedRetirement(t *testing.T) {
	t.Parallel()
	newFixture := func(t *testing.T) guardRepository {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		fixture.git("commit", "-m", "add contract")
		return fixture
	}

	t.Run("deleted SPEC leaves a dangling feature edge", func(t *testing.T) {
		fixture := newFixture(t)
		if err := os.Remove(filepath.Join(fixture.root, "pkg/example/SPEC.md")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", "pkg/example/SPEC.md")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-related-spec")
	})

	t.Run("deleted feature leaves a dangling SPEC edge", func(t *testing.T) {
		fixture := newFixture(t)
		if err := os.Remove(filepath.Join(fixture.root, "agm/test/bdd/features/example.feature")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-bdd-feature")
	})

	t.Run("deleted SPEC leaves a dangling owner edge", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		for _, relative := range []string{"pkg/example/SPEC.md", "agm/test/bdd/features/example.feature"} {
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(relative))); err != nil {
				t.Fatal(err)
			}
		}
		fixture.git("add", "-u", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-spec-owner-target")
	})

	t.Run("deleted owner leaves a live implementation unowned", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.Remove(filepath.Join(fixture.root, ".opencode/plugins/SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", ".opencode/plugins/SPEC.owner")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-implementation-spec-owner")
	})

	t.Run("owner retirement ignores unrelated implementation changes", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.write("internal/unrelated/existing.go", "package unrelated\n\nconst value = 1\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner", "internal/unrelated/existing.go")
		fixture.git("commit", "-m", "add implementation owner edge")
		for _, relative := range []string{".opencode/plugins/SPEC.owner", ".opencode/plugins/adapter.mjs"} {
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(relative))); err != nil {
				t.Fatal(err)
			}
		}
		fixture.write("internal/unrelated/existing.go", "package unrelated\n\nconst value = 2\n")
		fixture.write("internal/added/new.go", "package added\n")
		fixture.git("add", "-A", "--", ".opencode/plugins/SPEC.owner", ".opencode/plugins/adapter.mjs", "internal/unrelated/existing.go", "internal/added/new.go")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || strings.Join(result.Changed, "\n") != ".opencode/plugins/SPEC.owner" {
			t.Fatalf("result = %#v, want retirement reminder without unrelated ownership findings", result)
		}
	})

	t.Run("owner deletion cannot hide an object-identical implementation relocation", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.MkdirAll(filepath.Join(fixture.root, "internal", "relocated"), 0o700); err != nil {
			t.Fatal(err)
		}
		fixture.git("mv", ".opencode/plugins/adapter.mjs", "internal/relocated/adapter.mjs")
		if err := os.Remove(filepath.Join(fixture.root, ".opencode/plugins/SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", ".opencode/plugins/SPEC.owner")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-implementation-spec-owner")
	})

	t.Run("implementation relocation with its owner reaches semantic review", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.MkdirAll(filepath.Join(fixture.root, "internal", "relocated"), 0o700); err != nil {
			t.Fatal(err)
		}
		fixture.git("mv", ".opencode/plugins/adapter.mjs", "internal/relocated/adapter.mjs")
		fixture.git("mv", ".opencode/plugins/SPEC.owner", "internal/relocated/SPEC.owner")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || len(result.Changed) != 2 {
			t.Fatalf("result = %#v, want valid owned implementation relocation reminder", result)
		}
	})

	t.Run("owner is replaced by permitted local SPEC ownership", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write("internal/adapter/adapter.go", "package adapter\n")
		fixture.write("internal/adapter/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", "internal/adapter/adapter.go", "internal/adapter/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.Remove(filepath.Join(fixture.root, "internal/adapter/SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		fixture.write("internal/adapter/SPEC.md", validSpec("agm/test/bdd/features/adapter.feature"))
		fixture.write("agm/test/bdd/features/adapter.feature", validFeature("internal/adapter/SPEC.md"))
		fixture.git("add", "-A", "--", "internal/adapter/SPEC.owner", "internal/adapter/SPEC.md", "agm/test/bdd/features/adapter.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 {
			t.Fatalf("result = %#v, want valid replacement ownership reminder", result)
		}
	})

	t.Run("complete retirement reaches semantic review", func(t *testing.T) {
		fixture := newFixture(t)
		for _, relative := range []string{"pkg/example/SPEC.md", "agm/test/bdd/features/example.feature"} {
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(relative))); err != nil {
				t.Fatal(err)
			}
		}
		fixture.git("add", "-u", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || len(result.Changed) != 2 ||
			!strings.Contains(result.Reminder, "retirement or stable-ID migration") {
			t.Fatalf("result = %#v, want reviewed retirement reminder", result)
		}
	})

	t.Run("same-change relocation validates replacement graph", func(t *testing.T) {
		fixture := newFixture(t)
		if err := os.MkdirAll(filepath.Join(fixture.root, "pkg", "relocated"), 0o700); err != nil {
			t.Fatal(err)
		}
		fixture.git("mv", "pkg/example/SPEC.md", "pkg/relocated/SPEC.md")
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/relocated/SPEC.md"))
		fixture.git("add", "--", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || len(result.Changed) != 3 {
			t.Fatalf("result = %#v, want valid relocation reminder", result)
		}
	})
}

func TestImplementationRelocationsKeepRepeatedObjectAssociationsCompact(t *testing.T) {
	const directoryCount = 2048
	const sharedOID = "0123456789abcdef0123456789abcdef01234567"

	baseEntries := make([]treeEntry, 0, directoryCount)
	targetEntries := make([]treeEntry, 0, directoryCount)
	changes := make([]change, 0, directoryCount*2)
	for index := range directoryCount {
		source := fmt.Sprintf("internal/source-%04d/adapter.go", index)
		target := fmt.Sprintf("internal/target-%04d/adapter.go", index)
		baseEntries = append(baseEntries, treeEntry{path: source, oid: sharedOID, mode: "100644", objectType: "blob"})
		targetEntries = append(targetEntries, treeEntry{path: target, oid: sharedOID, mode: "100644", objectType: "blob"})
		changes = append(changes, change{path: source, status: "D"}, change{path: target, status: "A"})
	}

	moves := implementationRelocations(baseEntries, targetEntries, changes)
	if len(moves.deletedOIDsByDir) != directoryCount {
		t.Fatalf("deleted source directories = %d, want %d", len(moves.deletedOIDsByDir), directoryCount)
	}
	if got := len(moves.addedDirsByOID[sharedOID]); got != directoryCount {
		t.Fatalf("added target directories = %d, want %d", got, directoryCount)
	}
	for sourceDir, objectIDs := range moves.deletedOIDsByDir {
		if len(objectIDs) != 1 {
			t.Fatalf("deleted object identities for %s = %d, want 1", sourceDir, len(objectIDs))
		}
	}

	deletedOwnerDirs := make(map[string]bool, directoryCount)
	for index := range directoryCount {
		deletedOwnerDirs[fmt.Sprintf("internal/source-%04d", index)] = true
	}
	visited := make(map[string]bool)
	moves.visitTargets(deletedOwnerDirs, func(targetDir string) {
		visited[targetDir] = true
	})
	if len(visited) != directoryCount {
		t.Fatalf("visited relocation targets = %d, want %d", len(visited), directoryCount)
	}
}

func TestParseSpecOwnerAcceptsNeutralCanonicalTargets(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"SPEC.md", ".dear-agent/SPEC.md", "internal/hookparity/SPEC.md"} {
		got, err := ParseSpecOwner([]byte(target+"\n"), ".opencode/plugins/SPEC.md")
		if err != nil || got != target {
			t.Errorf("ParseSpecOwner(%q) = (%q, %v)", target, got, err)
		}
	}
}

func TestParseSpecOwnerRejectsMalformedAndHarnessOwnedTargets(t *testing.T) {
	t.Parallel()
	invalid := [][]byte{
		nil,
		[]byte("../internal/shared/SPEC.md\n"),
		[]byte("internal/../shared/SPEC.md\n"),
		[]byte("/internal/shared/SPEC.md\n"),
		[]byte("internal\\shared\\SPEC.md\n"),
		[]byte("https://example.com/SPEC.md\n"),
		[]byte("internal/shared/SPEC.md#fragment\n"),
		[]byte("internal/*/SPEC.md\n"),
		[]byte("internal/shared/SPEC.md\r\n"),
		[]byte("internal/shared/SPEC.md\ninternal/other/SPEC.md\n"),
		[]byte("codex-cli/SPEC.md\n"),
		[]byte(".opencode/SPEC.md\n"),
		[]byte("internal/plugins/SPEC.md\n"),
		[]byte(".opencode/plugins/SPEC.md\n"),
		{0xff, '\n'},
		append([]byte("internal/shared/"), append([]byte{0x01}, []byte("SPEC.md\n")...)...),
		[]byte(strings.Repeat("a", MaxSpecOwnerBytes+1)),
	}
	for index, owner := range invalid {
		if target, err := ParseSpecOwner(owner, ".opencode/plugins/SPEC.md"); err == nil {
			t.Errorf("invalid owner %d resolved to %q", index, target)
		}
	}
}

func TestSpecOwnerChangesGovernCanonicalSharedContract(t *testing.T) {
	fixture := newGuardRepository(t)
	const (
		specPath    = "internal/hookparity/SPEC.md"
		featurePath = "agm/test/bdd/features/hook_parity.feature"
		ownerPath   = ".opencode/plugins/SPEC.owner"
	)
	fixture.write(specPath, validSpec(featurePath))
	fixture.write(featurePath, validFeature(specPath))
	fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
	fixture.git("add", "--", specPath, featurePath, ".opencode/plugins/adapter.mjs")
	fixture.git("commit", "-m", "add shared contract")
	base := fixture.git("rev-parse", "HEAD")

	fixture.write(ownerPath, specPath+"\n")
	fixture.git("add", "--", ownerPath)
	staged := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if staged.Decision != DecisionReminder || strings.Join(staged.Changed, "\n") != ownerPath {
		t.Fatalf("staged owner result = %#v", staged)
	}

	fixture.git("commit", "-m", "link adapter to shared contract")
	committed := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeCommitted, Base: base})
	if committed.Decision != DecisionReminder || strings.Join(committed.Changed, "\n") != ownerPath {
		t.Fatalf("committed owner result = %#v", committed)
	}
}

func TestAddingLocalSpecBesideExistingOwnerFailsClosed(t *testing.T) {
	fixture := newGuardRepository(t)
	const (
		sharedSpec    = "internal/hookparity/SPEC.md"
		sharedFeature = "agm/test/bdd/features/hook_parity.feature"
		ownerPath     = ".opencode/plugins/SPEC.owner"
		localSpec     = ".opencode/plugins/SPEC.md"
		localFeature  = "agm/test/bdd/features/local.feature"
	)
	fixture.write(sharedSpec, validSpec(sharedFeature))
	fixture.write(sharedFeature, validFeature(sharedSpec))
	fixture.write(ownerPath, sharedSpec+"\n")
	fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
	fixture.git("add", "--", sharedSpec, sharedFeature, ownerPath, ".opencode/plugins/adapter.mjs")
	fixture.git("commit", "-m", "add shared ownership edge")

	fixture.write(localSpec, validSpec(localFeature))
	fixture.write(localFeature, validFeature(localSpec))
	fixture.git("add", "--", localSpec, localFeature)
	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	assertDecisionAndCode(t, result, DecisionBlock, "ambiguous-spec-owner")
}

func TestSourceLessOrNonregularSpecOwnerFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, guardRepository, string)
	}{
		{name: "no source"},
		{
			name: "symlink source",
			setup: func(t *testing.T, fixture guardRepository, _ string) {
				fixture.write("adapter-target", "export default {};\n")
				if err := os.MkdirAll(filepath.Join(fixture.root, ".opencode", "plugins"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join("..", "..", "adapter-target"), filepath.Join(fixture.root, ".opencode", "plugins", "adapter.mjs")); err != nil {
					t.Fatal(err)
				}
				fixture.git("add", "--", "adapter-target", ".opencode/plugins/adapter.mjs")
			},
		},
		{
			name: "gitlink source",
			setup: func(_ *testing.T, fixture guardRepository, head string) {
				fixture.git("update-index", "--add", "--cacheinfo", "160000,"+head+",.opencode/plugins/adapter.go")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGuardRepository(t)
			const (
				specPath    = "internal/hookparity/SPEC.md"
				featurePath = "agm/test/bdd/features/hook_parity.feature"
				ownerPath   = ".opencode/plugins/SPEC.owner"
			)
			fixture.write(specPath, validSpec(featurePath))
			fixture.write(featurePath, validFeature(specPath))
			fixture.git("add", "--", specPath, featurePath)
			fixture.git("commit", "-m", "add shared contract")
			head := fixture.git("rev-parse", "HEAD")
			if test.setup != nil {
				test.setup(t, fixture, head)
			}
			fixture.write(ownerPath, specPath+"\n")
			fixture.git("add", "--", ownerPath)
			result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
			assertDecisionAndCode(t, result, DecisionBlock, "orphan-spec-owner")
		})
	}
}

func TestSpecOwnerChangesFailClosedOnInvalidOwnershipEdges(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		owner string
		setup func(guardRepository)
		code  string
	}{
		{name: "malformed", owner: "../shared/SPEC.md\n", code: "invalid-spec-owner"},
		{name: "missing target", owner: "internal/missing/SPEC.md\n", code: "missing-spec-owner-target"},
		{
			name: "ambiguous local owner", owner: "internal/shared/SPEC.md\n", code: "ambiguous-spec-owner",
			setup: func(fixture guardRepository) {
				fixture.write(".opencode/plugins/SPEC.md", validSpec("agm/test/bdd/features/local.feature"))
				fixture.write("agm/test/bdd/features/local.feature", validFeature(".opencode/plugins/SPEC.md"))
				fixture.write("internal/shared/SPEC.md", validSpec("agm/test/bdd/features/shared.feature"))
				fixture.write("agm/test/bdd/features/shared.feature", validFeature("internal/shared/SPEC.md"))
				fixture.git("add", "--", ".opencode/plugins/SPEC.md", "agm/test/bdd/features/local.feature", "internal/shared/SPEC.md", "agm/test/bdd/features/shared.feature")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGuardRepository(t)
			fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
			fixture.git("add", "--", ".opencode/plugins/adapter.mjs")
			if test.setup != nil {
				test.setup(fixture)
			}
			fixture.write(".opencode/plugins/SPEC.owner", test.owner)
			fixture.git("add", "--", ".opencode/plugins/SPEC.owner")
			result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
			assertDecisionAndCode(t, result, DecisionBlock, test.code)
		})
	}
}

func TestLogicalCorpusCountsDuplicateObjectPaths(t *testing.T) {
	t.Parallel()
	body := []byte(strings.Repeat("x", 40))
	oid := strings.Repeat("a", 40)
	entries := []treeEntry{
		{path: "one/SPEC.md", mode: "100644", oid: oid},
		{path: "two/SPEC.md", mode: "100644", oid: oid},
	}
	if _, failure := attachBodies(entries, map[string][]byte{oid: body}, 60); failure == nil || failure.code != "corpus-size-limit" {
		t.Fatalf("logical corpus failure = %#v", failure)
	}
}

func TestMarkdownFenceRequiresMatchingRunLengthAndTermination(t *testing.T) {
	t.Parallel()
	body := []byte("**EXAMPLE-01** The example contract shall remain valid.\n````text\n```\nAfter a trigger, the hidden example shall be ignored.\n````\n## BDD Traceability\n- Feature: `agm/test/bdd/features/example.feature`\n")
	document := parseSpecDocument("pkg/example/SPEC.md", body)
	for _, issue := range document.issues {
		if issue.code == "invalid-ears" || issue.code == "unterminated-markdown-fence" {
			t.Fatalf("unexpected issue: %#v", issue)
		}
	}
	unterminated := parseSpecDocument("pkg/example/SPEC.md", []byte("**EXAMPLE-01** The example contract shall remain valid.\n```text\n"))
	found := false
	for _, issue := range unterminated.issues {
		found = found || issue.code == "unterminated-markdown-fence"
	}
	if !found {
		t.Fatalf("unterminated issues = %#v", unterminated.issues)
	}
}

func TestSpecTraceabilityRejectsNonExecutableFeaturePaths(t *testing.T) {
	t.Parallel()
	for _, featurePath := range []string{
		"docs/contracts/example.feature",
		"features/example.feature",
		"agm/test/bdd/features-archive/example.feature",
		"AGM/test/bdd/features/example.feature",
		"agm/test/bdd/features/nested/example.feature",
		"agm/test/bdd/features/example feature.feature",
	} {
		document := parseSpecDocument("pkg/example/SPEC.md", []byte(validSpec(featurePath)))
		if !hasParseIssueCode(document.issues, "non-executable-bdd-reference") || len(document.features) != 0 {
			t.Fatalf("feature %q produced features=%#v issues=%#v", featurePath, document.features, document.issues)
		}
	}
	traversal := parseSpecDocument("pkg/example/SPEC.md", []byte(validSpec("agm/test/bdd/features/../example.feature")))
	if !hasParseIssueCode(traversal.issues, "malformed-bdd-reference") || len(traversal.features) != 0 {
		t.Fatalf("traversal feature produced features=%#v issues=%#v", traversal.features, traversal.issues)
	}
	document := parseSpecDocument("pkg/example/SPEC.md", []byte(validSpec("agm/test/bdd/features/example.feature")))
	if hasParseIssueCode(document.issues, "non-executable-bdd-reference") || !slices.Equal(document.features, []string{"agm/test/bdd/features/example.feature"}) {
		t.Fatalf("executable feature produced features=%#v issues=%#v", document.features, document.issues)
	}
}

func TestNonExecutableFeatureEvidenceFailsInStagedAndCommittedModes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		featurePath string
		code        string
		writePath   bool
	}{
		{name: "documentation example", featurePath: "docs/contracts/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "near-prefix archive", featurePath: "agm/test/bdd/features-archive/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "case variant", featurePath: "AGM/test/bdd/features/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "nested feature", featurePath: "agm/test/bdd/features/nested/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "unparseable basename", featurePath: "agm/test/bdd/features/example feature.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "traversal", featurePath: "agm/test/bdd/features/../example.feature", code: "malformed-bdd-reference"},
	}
	for _, test := range tests {
		for _, mode := range []Mode{ModeStaged, ModeCommitted} {
			t.Run(test.name+"/"+string(mode), func(t *testing.T) {
				fixture := newGuardRepository(t)
				base := fixture.git("rev-parse", "HEAD")
				fixture.write("pkg/example/SPEC.md", validSpec(test.featurePath))
				paths := []string{"pkg/example/SPEC.md"}
				if test.writePath {
					fixture.write(test.featurePath, validFeature("pkg/example/SPEC.md"))
					paths = append(paths, test.featurePath)
				}
				gitArgs := append([]string{"add", "--"}, paths...)
				fixture.git(gitArgs...)
				request := Request{Repository: fixture.root, Mode: mode}
				if mode == ModeCommitted {
					fixture.git("commit", "-m", "add non-executable contract evidence")
					request.Base = base
				}
				result := Evaluate(context.Background(), request)
				assertDecisionAndCode(t, result, DecisionBlock, test.code)
			})
		}
	}
}

func TestFeatureRunnableScenarioContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		body         string
		wantRunnable int
		wantCode     string
	}{
		{
			name:         "concrete Then with assertion continuations",
			body:         "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Runnable\n    Given a condition\n    When an action occurs\n    Then an outcome is visible\n    And another outcome is visible\n    But no false outcome is visible\n",
			wantRunnable: 1,
		},
		{
			name:     "Given and When only",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Not runnable\n    Given a condition\n    When an action occurs\n",
			wantCode: "scenario-without-assertion",
		},
		{
			name:     "conjunction without Then",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Not runnable\n    Given a condition\n    And another condition\n    But no assertion\n",
			wantCode: "scenario-without-assertion",
		},
		{
			name:     "outline without Examples",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Not runnable\n    Given <condition>\n    Then an outcome is visible\n",
			wantCode: "outline-without-examples",
		},
		{
			name:     "outline with header only",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Not runnable\n    Given <condition>\n    Then an outcome is visible\n    Examples:\n      | condition |\n",
			wantCode: "outline-without-examples",
		},
		{
			name:         "outline with data row",
			body:         "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Runnable\n    Given <condition>\n    Then an outcome is visible\n    Examples:\n      | condition |\n      | ready     |\n",
			wantRunnable: 1,
		},
		{
			name:     "Scenario Template is outside the contract",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Template: Not admitted\n    Given <condition>\n    Then an outcome is visible\n    Examples:\n      | condition |\n      | ready     |\n",
			wantCode: "unsupported-scenario-kind",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := parseFeatureDocument([]byte(test.body))
			if document.executableScenarios != test.wantRunnable {
				t.Fatalf("executable scenarios = %d, want %d; issues = %#v", document.executableScenarios, test.wantRunnable, document.issues)
			}
			if test.wantCode != "" && !hasParseIssueCode(document.issues, test.wantCode) {
				t.Fatalf("issues = %#v, want %q", document.issues, test.wantCode)
			}
		})
	}
}

func TestAmbientGitIndexAndReplacementObjectsCannotChangeEvidence(t *testing.T) {
	t.Run("ambient index", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		other := newGuardRepository(t)
		t.Setenv("GIT_INDEX_FILE", filepath.Join(other.root, ".git", "index"))
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder {
			t.Fatalf("decision = %q, findings = %#v", result.Decision, result.Findings)
		}
	})

	t.Run("replacement object", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", invalidSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		staged := strings.Fields(fixture.git("ls-files", "--stage", "--", "pkg/example/SPEC.md"))
		if len(staged) < 2 {
			t.Fatalf("unexpected ls-files output: %q", staged)
		}
		validPath := filepath.Join(fixture.root, "valid-replacement")
		if err := os.WriteFile(validPath, []byte(validSpec("agm/test/bdd/features/example.feature")), 0o600); err != nil {
			t.Fatal(err)
		}
		validOID := fixture.gitInput(validSpec("agm/test/bdd/features/example.feature"), "hash-object", "-w", "--stdin")
		fixture.git("replace", staged[1], validOID)
		t.Setenv("GIT_NO_REPLACE_OBJECTS", "0")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "invalid-ears")
	})
}

func TestMissingPromisorBlobDoesNotLazyFetch(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
	fields := strings.Fields(fixture.git("ls-files", "--stage", "--", "pkg/example/SPEC.md"))
	if len(fields) < 2 {
		t.Fatalf("unexpected ls-files output: %q", fields)
	}
	oid := fields[1]
	fixture.git("config", "extensions.partialClone", "origin")
	fixture.git("config", "remote.origin.promisor", "true")
	fixture.git("config", "remote.origin.url", "file:///definitely-not-a-repository")
	objectPath := filepath.Join(fixture.root, ".git", "objects", oid[:2], oid[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatalf("remove test-owned loose object: %v", err)
	}

	started := time.Now()
	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if result.Decision != DecisionBlock || (!hasCode(result, "git-object-read") && !hasCode(result, "missing-git-object")) {
		t.Fatalf("result = %#v", result)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("missing promisor object took %s; lazy fetch may have been attempted", elapsed)
	}
}

func TestCleanGitEnvironmentDropsAmbientControlVariables(t *testing.T) {
	t.Setenv("GIT_INDEX_FILE", "/tmp/hostile-index")
	t.Setenv("GIT_OBJECT_DIRECTORY", "/tmp/hostile-objects")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.fsmonitor'='hostile'")
	t.Setenv("GIT_NO_LAZY_FETCH", "0")
	environment := cleanGitEnvironment("/usr/bin/git")
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	for _, forbidden := range []string{"GIT_INDEX_FILE=", "GIT_OBJECT_DIRECTORY=", "GIT_CONFIG_PARAMETERS="} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Fatalf("ambient %s leaked into %#v", forbidden, environment)
		}
	}
	for _, required := range []string{"GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_OPTIONAL_LOCKS=0"} {
		if !strings.Contains(joined, "\n"+required+"\n") {
			t.Fatalf("required %s missing from %#v", required, environment)
		}
	}
}

type guardRepository struct {
	t       *testing.T
	root    string
	sandbox *gittest.Sandbox
}

func newGuardRepository(t *testing.T) guardRepository {
	t.Helper()
	return newGuardRepositoryAt(t, t.TempDir())
}

func newGuardRepositoryAt(t *testing.T, root string) guardRepository {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	sandbox := gittest.New(t)
	sandbox.Run(t, root, "init")
	fixture := guardRepository{t: t, root: root, sandbox: sandbox}
	fixture.write("README.md", "base\n")
	fixture.git("add", "--", "README.md")
	fixture.git("commit", "-m", "base")
	return fixture
}

func (fixture guardRepository) write(relative, body string) {
	fixture.t.Helper()
	filePath := filepath.Join(fixture.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(body), 0o600); err != nil {
		fixture.t.Fatal(err)
	}
}

func (fixture guardRepository) git(args ...string) string {
	fixture.t.Helper()
	return strings.TrimSpace(fixture.sandbox.Run(fixture.t, fixture.root, args...))
}

func (fixture guardRepository) gitInput(input string, args ...string) string {
	fixture.t.Helper()
	command := fixture.sandbox.Command(fixture.root, args...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		fixture.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func validSpec(featurePath string) string {
	return "# Example contract\n\n## Requirements\n\n**EXAMPLE-01** The example contract shall preserve one provider-neutral outcome.\n\n## BDD Traceability\n\n- BDD: `" + featurePath + "`\n"
}

func invalidSpec(featurePath string) string {
	return "# Example contract\n\n## Requirements\n\n**EXAMPLE-01** After an event, the example contract shall preserve an ambiguous outcome.\n\n## BDD Traceability\n\n- BDD: `" + featurePath + "`\n"
}

func validFeature(specPath string) string {
	return "# SPEC: " + specPath + "\nFeature: Example contract\n  Scenario: Preserve the outcome\n    Given an observable condition\n    When the contract is evaluated\n    Then the outcome is preserved\n"
}

func hasCode(result Result, code string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func hasParseIssueCode(issues []parseIssue, code string) bool {
	for _, issue := range issues {
		if issue.code == code {
			return true
		}
	}
	return false
}

func assertDecisionAndCode(t *testing.T, result Result, decision Decision, code string) {
	t.Helper()
	if result.Decision != decision || !hasCode(result, code) {
		t.Fatalf("decision = %q, wanted %q with %q; findings = %#v", result.Decision, decision, code, result.Findings)
	}
	if result.Scope != GuardScope || !strings.Contains(result.EvidenceClaim, "no provider") {
		t.Fatalf("scope/evidence claim = %q / %q", result.Scope, result.EvidenceClaim)
	}
}

func writeFakeGit(t *testing.T, body string) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(filePath, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return filePath
}
