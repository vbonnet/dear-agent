package codexhooks

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestDefaultStoreBaseIgnoresCallerControlledHome(t *testing.T) {
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	callerHome := t.TempDir()
	t.Setenv("HOME", callerHome)

	got, err := DefaultStoreBase()
	if err != nil {
		t.Fatalf("DefaultStoreBase() error: %v", err)
	}
	want := filepath.Join(account.HomeDir, ".local", "share", "dear-agent", "trusted-codex-hooks")
	if got != want {
		t.Fatalf("DefaultStoreBase() = %q, want OS-account path %q", got, want)
	}
	if pathWithin(got, callerHome) {
		t.Fatalf("trusted hook store %q follows caller-controlled HOME %q", got, callerHome)
	}
}

func TestAttestAndVerifyUseCommittedHookObjects(t *testing.T) {
	source, sandbox := hookFixture(t)
	attestation, err := attestForTest(t, source, sandbox, []string{sandbox})
	if err != nil {
		t.Fatalf("Attest() error: %v", err)
	}
	if len(attestation.SourceCommit) != 40 || len(attestation.Digest) != 64 {
		t.Fatalf("Attest() = %#v", attestation)
	}
	identity, err := InspectSource(context.Background(), source)
	if err != nil {
		t.Fatalf("InspectSource() error: %v", err)
	}
	if identity.SourceRepo != attestation.SourceRepo ||
		identity.SourceCommit != attestation.SourceCommit ||
		identity.Digest != attestation.Digest {
		t.Fatalf("InspectSource() = %#v, want attested source identity %#v", identity, attestation)
	}

	// A mutable source working-tree edit cannot redefine what was reviewed:
	// verification reads the pinned Git object instead.
	writeFile(t, filepath.Join(source, ".codex", "hooks", "guard"), "#!/bin/sh\nexit 99\n", 0o755)
	if err := Verify(context.Background(), attestation, sandbox); err != nil {
		t.Fatalf("Verify() after source working-tree edit: %v", err)
	}
	identityAfterEdit, err := InspectSource(context.Background(), source)
	if err != nil {
		t.Fatalf("InspectSource() after working-tree edit: %v", err)
	}
	if identityAfterEdit != identity {
		t.Fatalf("mutable working-tree edit changed reviewed identity: before=%#v after=%#v", identity, identityAfterEdit)
	}
}

func TestGitAttestationIgnoresCallerProcessSelection(t *testing.T) {
	source := gittest.NewRepo(t)
	other := gittest.NewRepo(t)
	shimDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "invoked")
	writeFile(t, filepath.Join(shimDir, "git"), "#!/bin/sh\nprintf invoked >"+marker+"\n", 0o755)

	t.Setenv("PATH", shimDir)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(other, ".git", "objects"))

	root, err := gitRoot(context.Background(), source)
	if err != nil {
		t.Fatalf("gitRoot() error: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	if root != wantRoot {
		t.Fatalf("gitRoot() = %q, want %q", root, wantRoot)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("caller-controlled Git shim ran: %v", err)
	}
}

func TestVerifyRejectsMutatedImmutableMaterialization(t *testing.T) {
	source, sandbox := hookFixture(t)
	attestation, err := attestForTest(t, source, sandbox, []string{sandbox})
	if err != nil {
		t.Fatalf("Attest() error: %v", err)
	}
	hooksDir := filepath.Join(attestation.HookRoot, ".codex", "hooks")
	if err := os.Chmod(attestation.HookRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(attestation.HookRoot, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hooksDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "unexpected"), []byte("#!/bin/sh\n"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := lockMaterializedDirectories(attestation.HookRoot); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), attestation, sandbox); err == nil ||
		!strings.Contains(err.Error(), "unexpected asset") {
		t.Fatalf("Verify() error = %v, want unexpected-asset rejection", err)
	}
}

func TestAttestRejectsAgentWritableSourceOrStore(t *testing.T) {
	source, sandbox := hookFixture(t)
	if _, err := attestForTest(t, source, sandbox, []string{source, sandbox}); err == nil ||
		!strings.Contains(err.Error(), "hook source repository") {
		t.Fatalf("Attest() source-overlap error = %v", err)
	}

	store := filepath.Join(sandbox, "agent-writable-store")
	if _, err := Attest(context.Background(), source, sandbox, store, []string{sandbox}); err == nil ||
		!strings.Contains(err.Error(), "trusted hook root") {
		t.Fatalf("Attest() store-overlap error = %v", err)
	}
}

func TestVerifyRejectsMutatedSandboxHookAssets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "manifest",
			mutate: func(t *testing.T, sandbox string) {
				writeFile(t, filepath.Join(sandbox, ".codex", "hooks.json"), `{"hooks":{}}`, 0o644)
			},
		},
		{
			name: "referenced script",
			mutate: func(t *testing.T, sandbox string) {
				writeFile(t, filepath.Join(sandbox, ".codex", "hooks", "guard"), "#!/bin/sh\nexit 99\n", 0o755)
			},
		},
		{
			name: "relative referenced script",
			mutate: func(t *testing.T, sandbox string) {
				writeFile(t, filepath.Join(sandbox, "tools", "relative-guard"), "#!/bin/sh\nexit 99\n", 0o755)
			},
		},
		{
			name: "transitively referenced script",
			mutate: func(t *testing.T, sandbox string) {
				writeFile(t, filepath.Join(sandbox, "tools", "transitive-guard"), "#!/bin/sh\nexit 99\n", 0o755)
			},
		},
		{
			name: "referenced script symlink",
			mutate: func(t *testing.T, sandbox string) {
				path := filepath.Join(sandbox, ".codex", "hooks", "guard")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/bin/true", path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, sandbox := hookFixture(t)
			attestation, err := attestForTest(t, source, sandbox, []string{sandbox})
			if err != nil {
				t.Fatalf("Attest() error: %v", err)
			}
			tt.mutate(t, sandbox)
			if err := Verify(context.Background(), attestation, sandbox); err == nil {
				t.Fatal("Verify() error = nil, want fail-closed mismatch")
			}
		})
	}
}

func TestAttestRejectsMutableTransitiveHookRuntimeDependencies(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "working directory executable", script: "#!/bin/sh\n$PWD/helper\n"},
		{name: "dot relative executable", script: "#!/bin/sh\n./helper\n"},
		{name: "nested relative executable", script: "#!/bin/sh\ntools/helper\n"},
		{name: "temporary executable", script: "#!/bin/sh\n/tmp/helper\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, sandbox := hookFixtureWithGuard(t, tt.script)
			if _, err := attestForTest(t, source, sandbox, []string{sandbox}); err == nil ||
				!strings.Contains(err.Error(), "unsupported mutable") {
				t.Fatalf("Attest() error = %v, want mutable transitive runtime-path rejection", err)
			}
		})
	}
}

func TestReferencedScriptAssetsRejectsBareInterpreterAndSourceOperands(t *testing.T) {
	for _, script := range []string{
		"#!/bin/sh\n/bin/sh helper\n",
		"#!/bin/sh\n/bin/sh ./helper\n",
		"#!/bin/sh\n/usr/bin/env bash helper\n",
		"#!/bin/sh\nexec /bin/sh helper\n",
		"#!/bin/sh\nif /bin/sh helper; then exit 0; fi\n",
		"#!/bin/bash\nsource helper\n",
		"#!/bin/sh\n. helper\n",
	} {
		if _, err := referencedScriptAssets([]byte(script)); err == nil ||
			!strings.Contains(err.Error(), "mutable interpreter or sourced-file operand") {
			t.Fatalf("referencedScriptAssets(%q) error = %v, want mutable operand rejection", script, err)
		}
	}
}

func TestReferencedScriptAssetsAllowsCommittedInterpreterOperand(t *testing.T) {
	const script = "#!/bin/sh\n/bin/sh \"${AGM_CODEX_HOOK_ROOT:-.}/tools/helper\"\n"
	references, err := referencedScriptAssets([]byte(script))
	if err != nil {
		t.Fatalf("referencedScriptAssets() error: %v", err)
	}
	if !slices.Equal(references, []string{"tools/helper"}) {
		t.Fatalf("references = %v, want committed interpreter operand", references)
	}
}

func TestRepositoryEnabledHookScriptsHaveClosedRuntimeDependencies(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	manifest, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(hooksManifestPath)))
	if err != nil {
		t.Fatalf("read repository hook manifest: %v", err)
	}
	references, err := referencedHookAssets(manifest)
	if err != nil {
		t.Fatalf("parse repository hook manifest: %v", err)
	}
	seen := make(map[string]struct{})
	for len(references) > 0 {
		reference := references[0]
		references = references[1:]
		if _, exists := seen[reference]; exists {
			continue
		}
		seen[reference] = struct{}{}
		content, readErr := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(reference)))
		if readErr != nil {
			t.Fatalf("read repository hook asset %q: %v", reference, readErr)
		}
		transitive, parseErr := referencedScriptAssets(content)
		if parseErr != nil {
			t.Fatalf("validate repository hook asset %q: %v", reference, parseErr)
		}
		references = append(references, transitive...)
	}
}

func TestVerifyRejectsNestedHookManifestThatCanShadowRoot(t *testing.T) {
	source, sandbox := hookFixture(t)
	nestedWorkDir := filepath.Join(sandbox, "nested", "project")
	if err := os.MkdirAll(filepath.Join(nestedWorkDir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedWorkDir, ".codex", "hooks.json"), []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	attestation, err := attestForTest(t, source, sandbox, []string{sandbox})
	if err != nil {
		t.Fatalf("Attest() error: %v", err)
	}
	if err := Verify(context.Background(), attestation, nestedWorkDir); err == nil ||
		!strings.Contains(err.Error(), "unattested nested Codex hook manifest") {
		t.Fatalf("Verify() error = %v, want nested-manifest rejection", err)
	}
}

func TestAttestRejectsUncommittedOrUnsupportedProjectReferences(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "uncommitted referenced script",
			command: "${AGM_CODEX_HOOK_ROOT:-${CLAUDE_PROJECT_DIR:-.}}/.codex/hooks/missing",
			want:    "not a committed blob",
		},
		{
			name:    "mutable project directory",
			command: "${CLAUDE_PROJECT_DIR}/.codex/hooks/guard",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "unsupported project variable syntax",
			command: "${CLAUDE_PROJECT_DIR:-.}/.codex/hooks/guard",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "dot relative executable",
			command: "./helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "parent relative executable",
			command: "../helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "PWD executable",
			command: "$PWD/helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "braced PWD executable",
			command: "${PWD}/helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "nested relative executable",
			command: "tools/helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "home relative executable",
			command: "~/helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "temporary absolute executable",
			command: "/tmp/helper",
			want:    "unsupported mutable runtime path",
		},
		{
			name:    "home variable executable",
			command: "${HOME}/helper",
			want:    "unsupported mutable runtime path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, sandbox := hookFixture(t)
			manifest := `{"hooks":{"PreToolUse":[{"hooks":[{"command":"` + tt.command + `"}]}]}}`
			writeFile(t, filepath.Join(source, ".codex", "hooks.json"), manifest, 0o644)
			gittest.Run(t, source, "add", ".codex/hooks.json")
			gittest.Run(t, source, "commit", "-m", "change hook command")
			writeFile(t, filepath.Join(sandbox, ".codex", "hooks.json"), manifest, 0o644)
			if _, err := attestForTest(t, source, sandbox, []string{sandbox}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Attest() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAttestRejectsPathResolvedHookInterpreter(t *testing.T) {
	source, sandbox := hookFixture(t)
	const script = "#!/usr/bin/env bash\nexit 0\n"
	for _, root := range []string{source, sandbox} {
		writeFile(t, filepath.Join(root, ".codex", "hooks", "guard"), script, 0o755)
	}
	gittest.Run(t, source, "add", ".codex/hooks/guard")
	gittest.Run(t, source, "commit", "-m", "use path-resolved interpreter")

	_, err := attestForTest(t, source, sandbox, []string{sandbox})
	if err == nil || !strings.Contains(err.Error(), "trusted absolute interpreter") {
		t.Fatalf("Attest() error = %v, want trusted-interpreter rejection", err)
	}
}

func attestForTest(t *testing.T, source, sandbox string, writableRoots []string) (Attestation, error) {
	t.Helper()
	attestation, err := Attest(
		context.Background(), source, sandbox, filepath.Join(t.TempDir(), "store"), writableRoots,
	)
	if err == nil {
		t.Cleanup(func() {
			_ = filepath.WalkDir(attestation.HookRoot, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					return os.Chmod(path, 0o700)
				}
				return nil
			})
		})
	}
	return attestation, err
}

func hookFixture(t *testing.T) (string, string) {
	t.Helper()
	return hookFixtureWithGuard(t,
		"#!/bin/sh\n${AGM_CODEX_HOOK_ROOT:-.}/tools/transitive-guard\n",
	)
}

func hookFixtureWithGuard(t *testing.T, guard string) (string, string) {
	t.Helper()
	source := gittest.NewRepo(t)
	writeFile(t, filepath.Join(source, ".codex", "hooks.json"),
		`{"hooks":{"PreToolUse":[{"hooks":[{"command":"${AGM_CODEX_HOOK_ROOT:-${CLAUDE_PROJECT_DIR:-.}}/.codex/hooks/guard"},{"command":"${AGM_CODEX_HOOK_ROOT:-${CLAUDE_PROJECT_DIR:-.}}/tools/relative-guard"}]}]}}`,
		0o644,
	)
	writeFile(t, filepath.Join(source, ".codex", "hooks", "guard"), guard, 0o755)
	writeFile(t, filepath.Join(source, "tools", "relative-guard"), "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, filepath.Join(source, "tools", "transitive-guard"), "#!/bin/sh\nexit 0\n", 0o755)
	gittest.Run(t, source, "add", ".codex", "tools/relative-guard", "tools/transitive-guard")
	gittest.Run(t, source, "commit", "-m", "add reviewed hooks")

	sandbox := filepath.Join(t.TempDir(), "sandbox")
	gittest.Run(t, filepath.Dir(sandbox), "clone", "--no-hardlinks", source, sandbox)
	gittest.HardenRepo(t, sandbox)
	return source, sandbox
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
