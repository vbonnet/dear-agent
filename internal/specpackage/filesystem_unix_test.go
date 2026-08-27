//go:build darwin || linux

package specpackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

//nolint:dupl // The payload and artifact tables assert different Stage/Validate boundaries.
func TestValidateRejectsSpecialPayloadsBeforeOpeningFiles(t *testing.T) {
	tests := []struct {
		name      string
		replace   func(*testing.T, *testFixture, string)
		wantError string
	}{
		{
			name: "symbolic link",
			replace: func(t *testing.T, fixture *testFixture, target string) {
				external := filepath.Join(fixture.base, "symlink-target")
				mustWriteFile(t, external, testArtifactBytes(), 0o555)
				if err := os.Symlink(external, target); err != nil {
					t.Fatalf("create payload symlink: %v", err)
				}
			},
			wantError: "symbolic link",
		},
		{
			name: "hard link",
			replace: func(t *testing.T, fixture *testFixture, target string) {
				external := filepath.Join(fixture.base, "hardlink-target")
				mustWriteFile(t, external, testArtifactBytes(), 0o555)
				if err := os.Link(external, target); err != nil {
					t.Fatalf("create payload hard link: %v", err)
				}
			},
			wantError: "hard links",
		},
		{
			name: "FIFO",
			replace: func(t *testing.T, _ *testFixture, target string) {
				if err := unix.Mkfifo(target, 0o555); err != nil {
					t.Fatalf("create payload FIFO: %v", err)
				}
			},
			wantError: "FIFO",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStagedFixture(t)
			target := filepath.Join(fixture.staged.Root, "bin", "specaudit")
			mustRemove(t, target)
			test.replace(t, fixture, target)

			opened := false
			oldOpenHook := afterRegularFileOpen
			afterRegularFileOpen = func(string) { opened = true }
			defer func() { afterRegularFileOpen = oldOpenHook }()

			_, err := Validate(context.Background(), fixture.staged.Root)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate error = %v, want error containing %q", err, test.wantError)
			}
			if opened {
				t.Error("Validate attempted to open a payload after the tree contained a special file")
			}
		})
	}
}

func TestValidateRejectsSpecialFileSwapBetweenInspectionAndOpen(t *testing.T) {
	tests := []struct {
		name    string
		replace func(*testFixture, string) error
	}{
		{
			name: "symbolic link",
			replace: func(fixture *testFixture, target string) error {
				external := filepath.Join(fixture.base, "race-symlink-target")
				if err := os.WriteFile(external, testArtifactBytes(), 0o555); err != nil {
					return err
				}
				if err := os.Chmod(external, 0o555); err != nil {
					return err
				}
				if err := os.Remove(target); err != nil {
					return err
				}
				return os.Symlink(external, target)
			},
		},
		{
			name: "hard link",
			replace: func(fixture *testFixture, target string) error {
				external := filepath.Join(fixture.base, "race-hardlink-target")
				if err := os.WriteFile(external, testArtifactBytes(), 0o555); err != nil {
					return err
				}
				if err := os.Chmod(external, 0o555); err != nil {
					return err
				}
				if err := os.Remove(target); err != nil {
					return err
				}
				return os.Link(external, target)
			},
		},
		{
			name: "FIFO",
			replace: func(_ *testFixture, target string) error {
				if err := os.Remove(target); err != nil {
					return err
				}
				return unix.Mkfifo(target, 0o555)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStagedFixture(t)
			target := filepath.Join(fixture.staged.Root, "bin", "specaudit")
			preinspectionRan := false
			opened := false
			var mutationError error
			oldPreinspectionHook := afterRegularFilePreInspection
			oldOpenHook := afterRegularFileOpen
			afterRegularFilePreInspection = func(relative string) {
				if relative != "bin/specaudit" || preinspectionRan {
					return
				}
				preinspectionRan = true
				mutationError = test.replace(fixture, target)
			}
			afterRegularFileOpen = func(relative string) {
				if relative == "bin/specaudit" {
					opened = true
				}
			}
			defer func() {
				afterRegularFilePreInspection = oldPreinspectionHook
				afterRegularFileOpen = oldOpenHook
			}()

			_, err := Validate(context.Background(), fixture.staged.Root)
			if mutationError != nil {
				t.Fatalf("replace inspected payload: %v", mutationError)
			}
			if !preinspectionRan {
				t.Fatal("preinspection hook did not run")
			}
			if err == nil {
				t.Fatal("Validate accepted a payload replaced with a special file before open")
			}
			if opened {
				t.Error("Validate reached the post-open hook for a replacement special file")
			}
		})
	}
}

func TestStageRejectsSpecialCanonicalSourceBeforeOpeningFiles(t *testing.T) {
	tests := []struct {
		name      string
		replace   func(*testing.T, *testFixture, string)
		wantError string
	}{
		{
			name: "symbolic link",
			replace: func(t *testing.T, fixture *testFixture, target string) {
				external := filepath.Join(fixture.base, "source-symlink-target")
				mustWriteFile(t, external, []byte("# External\n"), 0o600)
				if err := os.Symlink(external, target); err != nil {
					t.Fatalf("create source symlink: %v", err)
				}
			},
			wantError: "symbolic link",
		},
		{
			name: "hard link",
			replace: func(t *testing.T, fixture *testFixture, target string) {
				external := filepath.Join(fixture.base, "source-hardlink-target")
				mustWriteFile(t, external, []byte("# External\n"), 0o600)
				if err := os.Link(external, target); err != nil {
					t.Fatalf("create source hard link: %v", err)
				}
			},
			wantError: "hard links",
		},
		{
			name: "FIFO",
			replace: func(t *testing.T, _ *testFixture, target string) {
				if err := unix.Mkfifo(target, 0o600); err != nil {
					t.Fatalf("create source FIFO: %v", err)
				}
			},
			wantError: "FIFO",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			target := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
			mustRemove(t, target)
			test.replace(t, fixture, target)

			opened := false
			oldOpenHook := afterRegularFileOpen
			afterRegularFileOpen = func(string) { opened = true }
			defer func() { afterRegularFileOpen = oldOpenHook }()

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Stage = %#v, error = %v; want error containing %q", staged, err, test.wantError)
			}
			if opened {
				t.Error("Stage opened a file after the canonical source tree contained a special file")
			}
			assertDirectoryEmpty(t, fixture.stagingParent)
		})
	}
}

//nolint:dupl // The artifact table intentionally mirrors payload special-file coverage.
func TestStageRejectsSpecialArtifactBeforeOpeningFile(t *testing.T) {
	tests := []struct {
		name      string
		replace   func(*testing.T, *testFixture, string)
		wantError string
	}{
		{
			name: "symbolic link",
			replace: func(t *testing.T, fixture *testFixture, target string) {
				external := filepath.Join(fixture.base, "artifact-symlink-target")
				mustWriteFile(t, external, testArtifactBytes(), 0o755)
				if err := os.Symlink(external, target); err != nil {
					t.Fatalf("create artifact symlink: %v", err)
				}
			},
			wantError: "not a regular file",
		},
		{
			name: "hard link",
			replace: func(t *testing.T, fixture *testFixture, target string) {
				external := filepath.Join(fixture.base, "artifact-hardlink-target")
				mustWriteFile(t, external, testArtifactBytes(), 0o755)
				if err := os.Link(external, target); err != nil {
					t.Fatalf("create artifact hard link: %v", err)
				}
			},
			wantError: "hard links",
		},
		{
			name: "FIFO",
			replace: func(t *testing.T, _ *testFixture, target string) {
				if err := unix.Mkfifo(target, 0o755); err != nil {
					t.Fatalf("create artifact FIFO: %v", err)
				}
			},
			wantError: "not a regular file",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			mustRemove(t, fixture.artifactPath)
			test.replace(t, fixture, fixture.artifactPath)

			opened := false
			oldOpenHook := afterRegularFileOpen
			afterRegularFileOpen = func(string) { opened = true }
			defer func() { afterRegularFileOpen = oldOpenHook }()

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Stage = %#v, error = %v; want error containing %q", staged, err, test.wantError)
			}
			if opened {
				t.Error("Stage reached the post-open hook for a special artifact")
			}
			assertDirectoryEmpty(t, fixture.stagingParent)
		})
	}
}

func TestStageRejectsSourceAndArtifactLeafReplacementBeforeOpen(t *testing.T) {
	tests := []struct {
		name        string
		hookPath    func(*testFixture) string
		targetPath  func(*testFixture) string
		retainsRoot bool
	}{
		{
			name:        "canonical source leaf",
			retainsRoot: true,
			hookPath: func(*testFixture) string {
				return "spec-governance/skills/audit-specs/SKILL.md"
			},
			targetPath: func(fixture *testFixture) string {
				return filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "SKILL.md")
			},
		},
		{
			name: "artifact leaf",
			hookPath: func(fixture *testFixture) string {
				return filepath.Base(fixture.artifactPath)
			},
			targetPath: func(fixture *testFixture) string {
				return fixture.artifactPath
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			hookPath := test.hookPath(fixture)
			target := test.targetPath(fixture)
			external := filepath.Join(fixture.base, "leaf-replacement")
			mustWriteFile(t, external, []byte("replacement\n"), 0o755)
			preinspectionRan := false
			opened := false
			var mutationError error
			oldPreinspectionHook := afterRegularFilePreInspection
			oldOpenHook := afterRegularFileOpen
			afterRegularFilePreInspection = func(relative string) {
				if relative != hookPath || preinspectionRan {
					return
				}
				preinspectionRan = true
				if mutationError = os.Remove(target); mutationError == nil {
					mutationError = os.Symlink(external, target)
				}
			}
			afterRegularFileOpen = func(relative string) {
				if relative == hookPath {
					opened = true
				}
			}
			defer func() {
				afterRegularFilePreInspection = oldPreinspectionHook
				afterRegularFileOpen = oldOpenHook
			}()

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if mutationError != nil {
				t.Fatalf("replace inspected leaf: %v", mutationError)
			}
			if !preinspectionRan {
				t.Fatal("preinspection hook did not run")
			}
			if err == nil {
				t.Fatalf("Stage accepted leaf replacement: %#v", staged)
			}
			if opened {
				t.Error("Stage reached the post-open hook for a replacement symlink")
			}
			if test.retainsRoot {
				assertRetainedFailedRoot(t, fixture.stagingParent, err)
			} else {
				assertDirectoryEmpty(t, fixture.stagingParent)
			}
		})
	}
}

func TestValidateRejectsDistributionRootReplacementDuringRead(t *testing.T) {
	fixture := newStagedFixture(t)
	originalRoot := fixture.staged.Root
	movedRoot := originalRoot + "-moved"
	preinspectionRan := false
	var mutationError error
	oldHook := afterRegularFilePreInspection
	afterRegularFilePreInspection = func(relative string) {
		if relative != "bin/specaudit" || preinspectionRan {
			return
		}
		preinspectionRan = true
		if mutationError = os.Rename(originalRoot, movedRoot); mutationError == nil {
			mutationError = os.Mkdir(originalRoot, 0o700)
		}
	}
	defer func() { afterRegularFilePreInspection = oldHook }()

	receipt, err := Validate(context.Background(), originalRoot)
	if mutationError != nil {
		t.Fatalf("replace distribution root: %v", mutationError)
	}
	if !preinspectionRan {
		t.Fatal("preinspection hook did not run")
	}
	if err == nil {
		t.Fatalf("Validate returned receipt %#v after its distribution root was replaced", receipt)
	}
}

func TestStageRejectsSourceRootReplacementDuringCopy(t *testing.T) {
	fixture := newFixtureInputs(t)
	originalRoot := fixture.sourceRoot
	movedRoot := originalRoot + "-moved"
	replacementRan := false
	var mutationError error
	oldHook := afterRegularFileOpen
	afterRegularFileOpen = func(relative string) {
		if relative != filepath.Base(fixture.artifactPath) || replacementRan {
			return
		}
		replacementRan = true
		if mutationError = os.Rename(originalRoot, movedRoot); mutationError == nil {
			mutationError = os.Mkdir(originalRoot, 0o700)
		}
	}
	defer func() { afterRegularFileOpen = oldHook }()

	staged, err := Stage(context.Background(), originalRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("replace source root: %v", mutationError)
	}
	if !replacementRan {
		t.Fatal("artifact-open hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v after its source root was replaced", staged)
	}
	assertDirectoryEmpty(t, fixture.stagingParent)
}

func TestStageRejectsArtifactRootReplacementDuringRead(t *testing.T) {
	fixture := newFixtureInputs(t)
	artifactRoot := filepath.Join(fixture.base, "artifact-root")
	if err := os.Mkdir(artifactRoot, 0o700); err != nil {
		t.Fatalf("create artifact root: %v", err)
	}
	artifactPath := filepath.Join(artifactRoot, "specaudit")
	if err := os.Rename(fixture.artifactPath, artifactPath); err != nil {
		t.Fatalf("move fixture artifact: %v", err)
	}
	fixture.artifactPath = artifactPath
	movedRoot := artifactRoot + "-moved"
	replacementRan := false
	var mutationError error
	oldHook := afterRegularFileOpen
	afterRegularFileOpen = func(relative string) {
		if relative != filepath.Base(fixture.artifactPath) || replacementRan {
			return
		}
		replacementRan = true
		if mutationError = os.Rename(artifactRoot, movedRoot); mutationError == nil {
			mutationError = os.Mkdir(artifactRoot, 0o700)
		}
	}
	defer func() { afterRegularFileOpen = oldHook }()

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("replace artifact root: %v", mutationError)
	}
	if !replacementRan {
		t.Fatal("artifact-open hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v after its artifact root was replaced", staged)
	}
	assertDirectoryEmpty(t, fixture.stagingParent)
}

func TestStageRejectsAStagingParentInsideTheSourceBeforeAllocation(t *testing.T) {
	fixture := newFixtureInputs(t)
	nested := filepath.Join(fixture.sourceRoot, "nested-staging-parent")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatalf("create nested staging parent: %v", err)
	}
	alias := filepath.Join(fixture.base, "source-alias")
	if err := os.Symlink(fixture.sourceRoot, alias); err != nil {
		t.Fatalf("create source-root alias: %v", err)
	}

	for _, test := range []struct {
		name          string
		stagingParent string
	}{
		{name: "source root", stagingParent: fixture.sourceRoot},
		{name: "source descendant", stagingParent: nested},
		{name: "source descendant through intermediate symlink", stagingParent: filepath.Join(alias, filepath.Base(nested))},
	} {
		t.Run(test.name, func(t *testing.T) {
			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, test.stagingParent)
			if err == nil || !strings.Contains(err.Error(), "staging parent must not be the source root or a source descendant") {
				t.Fatalf("Stage = %#v, error = %v; want source-overlap rejection", staged, err)
			}
			if staged.Root != "" || staged.Receipt.SchemaVersion != "" ||
				staged.Receipt.ManifestSHA256 != "" || len(staged.Receipt.Files) != 0 {
				t.Fatalf("Stage returned partial package %#v after source-overlap rejection", staged)
			}
			entries, readErr := os.ReadDir(test.stagingParent)
			if readErr != nil {
				t.Fatalf("read rejected staging parent: %v", readErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".spec-governance-stage-") {
					t.Errorf("Stage allocated %q inside the source tree", filepath.Join(test.stagingParent, entry.Name()))
				}
			}
		})
	}
}

func TestStageDoesNotWriteThroughAStagingRootReplacement(t *testing.T) {
	fixture := newFixtureInputs(t)
	replacement := t.TempDir()
	marker := filepath.Join(replacement, "preserve")
	if err := os.WriteFile(marker, []byte("preserve\n"), 0o600); err != nil {
		t.Fatalf("write replacement marker: %v", err)
	}
	replacementRan := false
	var mutationError error
	oldHook := afterPrivateStagingRootCreation
	afterPrivateStagingRootCreation = func(root string) {
		if replacementRan {
			return
		}
		replacementRan = true
		if mutationError = os.Rename(root, root+"-moved"); mutationError == nil {
			mutationError = os.Symlink(replacement, root)
		}
	}
	defer func() { afterPrivateStagingRootCreation = oldHook }()

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("replace private staging root: %v", mutationError)
	}
	if !replacementRan {
		t.Fatal("private-root creation hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v after its private root was replaced", staged)
	}
	var retained *RetainedStagingRootError
	if !errors.As(err, &retained) || retained.IdentityVerified {
		t.Fatalf("Stage error = %v, want unverified retained-path diagnostic", err)
	}
	if content, readErr := os.ReadFile(marker); readErr != nil || string(content) != "preserve\n" {
		t.Fatalf("replacement marker changed: content = %q, error = %v", content, readErr)
	}
	for _, name := range []string{"bin", "skills", manifestPath} {
		if _, statErr := os.Lstat(filepath.Join(replacement, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("Stage wrote replacement child %q: %v", name, statErr)
		}
	}
}

func TestAllocatorFailureLeavesAnUnverifiedReplacementUntouched(t *testing.T) {
	fixture := newFixtureInputs(t)
	replacementRan := false
	var mutationError error
	var allocatedPath string
	oldHook := afterPrivateStagingRootMkdir
	afterPrivateStagingRootMkdir = func(root string) {
		if replacementRan {
			return
		}
		replacementRan = true
		allocatedPath = root
		if mutationError = os.Rename(root, root+"-created"); mutationError == nil {
			mutationError = os.Mkdir(root, 0)
		}
	}
	defer func() { afterPrivateStagingRootMkdir = oldHook }()
	defer func() {
		if allocatedPath != "" {
			_ = os.Chmod(allocatedPath, 0o700)
		}
	}()

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("replace newly allocated private root: %v", mutationError)
	}
	if !replacementRan {
		t.Fatal("post-mkdir allocator hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v after allocator root replacement", staged)
	}
	var retained *RetainedStagingRootError
	if !errors.As(err, &retained) || retained.IdentityVerified || retained.Root != allocatedPath {
		t.Fatalf("Stage error = %v, want exact unverified allocated-path diagnostic", err)
	}
	if info, statErr := os.Lstat(allocatedPath); statErr != nil || !info.IsDir() || info.Mode().Perm() != 0 {
		t.Fatalf("allocator replacement changed or disappeared: info = %#v, error = %v", info, statErr)
	}
	if info, statErr := os.Lstat(allocatedPath + "-created"); statErr != nil || !info.IsDir() {
		t.Fatalf("original allocated root changed or disappeared: info = %#v, error = %v", info, statErr)
	}
}

func TestStageRetainsFailedPrivateRootWithoutUnlinking(t *testing.T) {
	fixture := newFixtureInputs(t)
	skillPath := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "SKILL.md")
	skill := strings.Replace(string(mustReadFile(t, skillPath)), "name: audit-specs", "name: wrong-name", 1)
	mustWriteFile(t, skillPath, []byte(skill), 0o600)

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if err == nil {
		t.Fatalf("Stage returned package %#v despite invalid skill", staged)
	}
	var retained *RetainedStagingRootError
	if !errors.As(err, &retained) {
		t.Fatalf("Stage error = %v, want retained-root receipt", err)
	}
	if filepath.Dir(retained.Root) != fixture.stagingParent {
		t.Fatalf("retained root = %q, want parent %q", retained.Root, fixture.stagingParent)
	}
	if info, statErr := os.Lstat(retained.Root); statErr != nil || !info.IsDir() {
		t.Fatalf("retained failed root missing: info = %#v, error = %v", info, statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(retained.Root, manifestPath)); statErr != nil {
		t.Fatalf("failed root payload was unlinked: %v", statErr)
	}
}

func TestStageRejectsAReparentedDirectoryBeforeWritingToEitherSibling(t *testing.T) {
	fixture := newFixtureInputs(t)
	victim := filepath.Join(fixture.stagingParent, "sibling-stage")
	for _, directory := range []string{
		"audit-specs/references",
		"write-spec/references",
	} {
		if err := os.MkdirAll(filepath.Join(victim, directory), 0o700); err != nil {
			t.Fatalf("create prepared sibling tree: %v", err)
		}
	}
	var stagedRoot string
	var capturedSibling string
	replacementRan := false
	var mutationError error
	oldRootHook := afterPrivateStagingRootCreation
	afterPrivateStagingRootCreation = func(root string) { stagedRoot = root }
	defer func() { afterPrivateStagingRootCreation = oldRootHook }()
	oldWriteHook := beforeStagedFileWrite
	beforeStagedFileWrite = func(relative string) {
		if replacementRan || relative != "skills/audit-specs/SKILL.md" {
			return
		}
		replacementRan = true
		original := filepath.Join(stagedRoot, "skills")
		capturedSibling = filepath.Join(fixture.stagingParent, "captured-skills-sibling")
		if mutationError = os.Rename(original, capturedSibling); mutationError == nil {
			mutationError = os.Rename(victim, original)
		}
	}
	defer func() { beforeStagedFileWrite = oldWriteHook }()

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("replace retained staged directory before write: %v", mutationError)
	}
	if !replacementRan {
		t.Fatal("staged write hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v despite a staged directory replacement", staged)
	}
	if !strings.Contains(err.Error(), `staged entry "skills" changed identity`) {
		t.Fatalf("Stage error = %v, want visible directory replacement rejection", err)
	}
	if _, statErr := os.Lstat(filepath.Join(stagedRoot, "skills", "audit-specs", "SKILL.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Stage modified replacement sibling tree: %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(capturedSibling, "audit-specs", "SKILL.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Stage modified the reparented captured sibling: %v", statErr)
	}
}

func TestStageRejectsAnUnexpectedEntryAfterSharedValidation(t *testing.T) {
	fixture := newFixtureInputs(t)
	hookRan := false
	var mutationError error
	oldHook := beforeFinalStagedVerification
	beforeFinalStagedVerification = func(root string) {
		if hookRan {
			return
		}
		hookRan = true
		mutationError = os.WriteFile(filepath.Join(root, "late-unexpected"), []byte("late\n"), 0o444)
	}
	defer func() { beforeFinalStagedVerification = oldHook }()

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("add late unexpected staged entry: %v", mutationError)
	}
	if !hookRan {
		t.Fatal("final staged verification hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v with a late unexpected entry", staged)
	}
	if !strings.Contains(err.Error(), `late-unexpected`) {
		t.Fatalf("Stage error = %v, want final exact-tree rejection", err)
	}
	assertRetainedFailedRoot(t, fixture.stagingParent, err)
}

func TestStageRejectsAStagingRootModeChangeAfterSharedValidation(t *testing.T) {
	fixture := newFixtureInputs(t)
	hookRan := false
	var mutationError error
	oldHook := beforeFinalStagedVerification
	beforeFinalStagedVerification = func(root string) {
		if hookRan {
			return
		}
		hookRan = true
		mutationError = os.Chmod(root, 0o755)
	}
	defer func() { beforeFinalStagedVerification = oldHook }()

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if mutationError != nil {
		t.Fatalf("change staged-root mode: %v", mutationError)
	}
	if !hookRan {
		t.Fatal("final staged verification hook did not run")
	}
	if err == nil {
		t.Fatalf("Stage returned package %#v after a staged-root mode change", staged)
	}
	if !strings.Contains(err.Error(), "staged root changed mode") {
		t.Fatalf("Stage error = %v, want final staged-root mode rejection", err)
	}
	assertRetainedFailedRootWithMode(t, fixture.stagingParent, err, 0o755)
}

func TestValidateRejectsMutationsAfterInitialTreeEnumeration(t *testing.T) {
	tests := []struct {
		name    string
		trigger string
		mutate  func(*testing.T, *testFixture) error
	}{
		{
			name:    "unexpected entry",
			trigger: "bin/specaudit",
			mutate: func(_ *testing.T, fixture *testFixture) error {
				return os.WriteFile(filepath.Join(fixture.staged.Root, "unexpected"), []byte("unexpected\n"), 0o444)
			},
		},
		{
			name:    "prior leaf replacement",
			trigger: "skills/audit-specs/SKILL.md",
			mutate: func(t *testing.T, fixture *testFixture) error {
				target := filepath.Join(fixture.staged.Root, "bin", "specaudit")
				content := mustReadFile(t, target)
				if err := os.Remove(target); err != nil {
					return err
				}
				return os.WriteFile(target, content, 0o555)
			},
		},
		{
			name:    "mode change during read",
			trigger: "bin/specaudit",
			mutate: func(_ *testing.T, fixture *testFixture) error {
				return os.Chmod(filepath.Join(fixture.staged.Root, "bin", "specaudit"), 0o755)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStagedFixture(t)
			mutationRan := false
			var mutationError error
			oldHook := afterRegularFileOpen
			afterRegularFileOpen = func(relative string) {
				if relative != test.trigger || mutationRan {
					return
				}
				mutationRan = true
				mutationError = test.mutate(t, fixture)
			}
			defer func() { afterRegularFileOpen = oldHook }()

			receipt, err := Validate(context.Background(), fixture.staged.Root)
			if mutationError != nil {
				t.Fatalf("mutate package tree: %v", mutationError)
			}
			if !mutationRan {
				t.Fatal("post-open mutation hook did not run")
			}
			if err == nil {
				t.Fatalf("Validate returned receipt %#v after %s", receipt, test.name)
			}
		})
	}
}

func TestRegularLeafReplacementAfterOpenIsRejected(t *testing.T) {
	t.Run("distribution payload", func(t *testing.T) {
		fixture := newStagedFixture(t)
		target := filepath.Join(fixture.staged.Root, "bin", "specaudit")
		replacementRan := false
		var mutationError error
		oldHook := afterRegularFileOpen
		afterRegularFileOpen = func(relative string) {
			if relative != "bin/specaudit" || replacementRan {
				return
			}
			replacementRan = true
			if mutationError = os.Remove(target); mutationError == nil {
				mutationError = os.WriteFile(target, testArtifactBytes(), 0o555)
			}
		}
		defer func() { afterRegularFileOpen = oldHook }()

		_, err := Validate(context.Background(), fixture.staged.Root)
		if mutationError != nil {
			t.Fatalf("replace opened payload: %v", mutationError)
		}
		if !replacementRan {
			t.Fatal("post-open hook did not run")
		}
		if err == nil {
			t.Fatalf("Validate error = %v, want post-open replacement rejection", err)
		}
	})

	t.Run("artifact", func(t *testing.T) {
		fixture := newFixtureInputs(t)
		replacementRan := false
		var mutationError error
		oldHook := afterRegularFileOpen
		afterRegularFileOpen = func(relative string) {
			if relative != filepath.Base(fixture.artifactPath) || replacementRan {
				return
			}
			replacementRan = true
			if mutationError = os.Remove(fixture.artifactPath); mutationError == nil {
				mutationError = os.WriteFile(fixture.artifactPath, testArtifactBytes(), 0o755)
			}
		}
		defer func() { afterRegularFileOpen = oldHook }()

		staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
		if mutationError != nil {
			t.Fatalf("replace opened artifact: %v", mutationError)
		}
		if !replacementRan {
			t.Fatal("post-open hook did not run")
		}
		if err == nil {
			t.Fatalf("Stage = %#v, error = %v; want post-open replacement rejection", staged, err)
		}
		assertDirectoryEmpty(t, fixture.stagingParent)
	})
}
