package codexhooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestAttestAndVerifyUseCommittedHookObjects(t *testing.T) {
	source, sandbox := hookFixture(t)
	attestation, err := Attest(context.Background(), source, sandbox)
	if err != nil {
		t.Fatalf("Attest() error: %v", err)
	}
	if len(attestation.SourceCommit) != 40 || len(attestation.Digest) != 64 {
		t.Fatalf("Attest() = %#v", attestation)
	}

	// A mutable source working-tree edit cannot redefine what was reviewed:
	// verification reads the pinned Git object instead.
	writeFile(t, filepath.Join(source, ".codex", "hooks", "guard"), "#!/bin/sh\nexit 99\n", 0o755)
	if err := Verify(context.Background(), attestation, sandbox); err != nil {
		t.Fatalf("Verify() after source working-tree edit: %v", err)
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
			attestation, err := Attest(context.Background(), source, sandbox)
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

func TestVerifyRejectsNestedHookManifestThatCanShadowRoot(t *testing.T) {
	source, sandbox := hookFixture(t)
	nestedWorkDir := filepath.Join(sandbox, "nested", "project")
	if err := os.MkdirAll(filepath.Join(nestedWorkDir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedWorkDir, ".codex", "hooks.json"), []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	attestation, err := Attest(context.Background(), source, sandbox)
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
			command: "${CLAUDE_PROJECT_DIR}/.codex/hooks/missing",
			want:    "not a committed blob",
		},
		{
			name:    "unsupported project variable syntax",
			command: "${CLAUDE_PROJECT_DIR:-.}/.codex/hooks/guard",
			want:    "unsupported project-directory reference",
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
			if _, err := Attest(context.Background(), source, sandbox); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Attest() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func hookFixture(t *testing.T) (string, string) {
	t.Helper()
	source := gittest.NewRepo(t)
	writeFile(t, filepath.Join(source, ".codex", "hooks.json"),
		`{"hooks":{"PreToolUse":[{"hooks":[{"command":"${CLAUDE_PROJECT_DIR}/.codex/hooks/guard"},{"command":"tools/relative-guard"}]}]}}`,
		0o644,
	)
	writeFile(t, filepath.Join(source, ".codex", "hooks", "guard"), "#!/bin/sh\nexit 0\n", 0o755)
	writeFile(t, filepath.Join(source, "tools", "relative-guard"), "#!/bin/sh\nexit 0\n", 0o755)
	gittest.Run(t, source, "add", ".codex", "tools/relative-guard")
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
