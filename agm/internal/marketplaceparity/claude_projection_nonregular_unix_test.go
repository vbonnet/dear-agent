//go:build darwin || linux

package marketplaceparity

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestProjectionRejectsFIFOAndSocketPromptly(t *testing.T) {
	for _, target := range projectionTestAuthorityTargets() {
		t.Run(target.name+"/fifo", func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			path := target.path(fixture)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := unix.Mkfifo(path, 0o600); err != nil {
				t.Skipf("FIFOs unavailable: %v", err)
			}
			projectionTestRequirePromptError(t, target.name+" FIFO", func() error {
				return target.validate(fixture.root)
			}, func() {
				fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0)
				if err == nil {
					_ = unix.Close(fd)
				}
			})
		})

		t.Run(target.name+"/socket", func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			path := target.path(fixture)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Skipf("Unix sockets unavailable: %v", err)
			}
			defer listener.Close()
			projectionTestRequirePromptError(t, target.name+" socket", func() error {
				return target.validate(fixture.root)
			}, nil)
		})
	}
}

func projectionTestRequirePromptError(t *testing.T, operation string, validate func() error, unblock func()) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		result <- validate()
	}()

	timer := time.NewTimer(1500 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-result:
		if err == nil {
			t.Fatalf("%s accepted a nonregular path", operation)
		}
	case <-timer.C:
		if unblock != nil {
			unblock()
		}
		select {
		case <-result:
		case <-time.After(time.Second):
		}
		t.Fatalf("%s did not reject within 1.5s", operation)
	}
}

func TestProjectionRejectsSocketNestedBelowSource(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	path := filepath.Join(fixture.root, "spec-governance", "skills", "audit-specs", "references", "socket.md")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	defer listener.Close()
	projectionTestRequirePromptError(t, "nested exported socket", func() error {
		return ValidateClaudeMarketplaceMirror(fixture.root)
	}, nil)
}

func TestProjectionRejectsCaseAliasedProviderComponentFiles(t *testing.T) {
	for _, component := range []string{
		".LSP.json",
		".MCP.json",
		"Bun.lock",
		"BUN.lockb",
		"Npm-Shrinkwrap.json",
		"Package-Lock.json",
		"Package.json",
		"Pnpm-Lock.yaml",
		"Settings.json",
		"Workflows",
		"Yarn.lock",
	} {
		t.Run(component, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			componentPath := filepath.Join(fixture.root, "spec-governance", component)
			if strings.EqualFold(component, "Workflows") {
				if err := os.Mkdir(componentPath, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				projectionTestWriteFile(t, componentPath, "{}\n")
			}
			err := ValidateClaudeMarketplaceMirror(fixture.root)
			if err == nil {
				t.Fatalf("ValidateClaudeMarketplaceMirror() accepted provider component case alias %q", component)
			}
			if !strings.Contains(err.Error(), "provider-default component surface") {
				t.Fatalf("ValidateClaudeMarketplaceMirror() error = %q, want provider-default component rejection", err)
			}
		})
	}
}

func TestProjectionRejectsCaseAliasedAuthorityPathsOnCaseInsensitiveVolume(t *testing.T) {
	tests := []struct {
		name     string
		original func(projectionTestFixture) string
		alias    func(projectionTestFixture) string
		validate func(string) error
		want     string
	}{
		{
			name:     "catalog directory",
			original: func(f projectionTestFixture) string { return filepath.Join(f.root, ".dear-agent") },
			alias:    func(f projectionTestFixture) string { return filepath.Join(f.root, ".DEAR-AGENT") },
			validate: ValidateCatalog,
			want:     "case alias",
		},
		{
			name:     "catalog file",
			original: func(f projectionTestFixture) string { return f.neutralPath },
			alias: func(f projectionTestFixture) string {
				return filepath.Join(filepath.Dir(f.neutralPath), "Marketplace.json")
			},
			validate: ValidateCatalog,
			want:     "case alias",
		},
		{
			name:     "plugin source directory",
			original: func(f projectionTestFixture) string { return filepath.Join(f.root, "spec-governance") },
			alias:    func(f projectionTestFixture) string { return filepath.Join(f.root, "SPEC-GOVERNANCE") },
			validate: ValidateClaudeMarketplaceMirror,
			want:     "case alias",
		},
		{
			name:     "repository license",
			original: func(f projectionTestFixture) string { return filepath.Join(f.root, canonicalRepositoryLicense) },
			alias:    func(f projectionTestFixture) string { return filepath.Join(f.root, "License") },
			validate: ValidateClaudeMarketplaceMirror,
			want:     "case alias",
		},
		{
			name: "packaged license",
			original: func(f projectionTestFixture) string {
				return filepath.Join(f.root, "spec-governance", canonicalPackagedLicense)
			},
			alias: func(f projectionTestFixture) string {
				return filepath.Join(f.root, "spec-governance", "License")
			},
			validate: ValidateClaudeMarketplaceMirror,
			want:     "plugin LICENSE must be a regular file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			original := test.original(fixture)
			alias := test.alias(fixture)
			projectionTestRenameCaseAlias(t, original, alias)
			err := test.validate(fixture.root)
			if err == nil {
				t.Fatalf("validation accepted on-disk case alias %q for %q", alias, original)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %q, want rejection containing %q", err, test.want)
			}
		})
	}
}

func projectionTestRenameCaseAlias(t *testing.T, original, alias string) {
	t.Helper()
	if err := os.Rename(original, alias); err != nil {
		t.Skipf("case-only rename unavailable: %v", err)
	}
	if _, err := os.Lstat(original); err != nil {
		if os.IsNotExist(err) {
			t.Skip("test volume uses case-sensitive lookup")
		}
		t.Fatalf("inspect case-aliased fixture path: %v", err)
	}
}

func TestAnchoredReadRejectsSameSizeRewriteWithRestoredMtime(t *testing.T) {
	root := t.TempDir()
	const relative = "authority.json"
	path := filepath.Join(root, relative)
	original := []byte("trusted\n")
	replacement := []byte("altered\n")
	if len(original) != len(replacement) {
		t.Fatal("fixture replacement must preserve size")
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var before unix.Stat_t
	if err := unix.Lstat(path, &before); err != nil {
		t.Fatal(err)
	}
	openedChangeTime := anchoredChangeTime(&before)
	mutationCompleted := false

	_, err = readAnchoredRegularAtCheckpoint(root, relative, int64(len(original)), func() error {
		deadline := time.Now().Add(2 * time.Second)
		for {
			if err := os.WriteFile(path, replacement, info.Mode().Perm()); err != nil {
				return err
			}
			if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
				return err
			}
			var after unix.Stat_t
			if err := unix.Lstat(path, &after); err != nil {
				return err
			}
			if anchoredChangeTime(&after) != openedChangeTime {
				mutationCompleted = true
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("filesystem change time did not advance")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	if !mutationCompleted {
		t.Fatalf("same-size fixture mutation failed: %v", err)
	}
	if err == nil {
		t.Fatal("anchored read accepted a same-size rewrite with restored mtime")
	}
	if !strings.Contains(err.Error(), "changed while it was read") {
		t.Fatalf("anchored read error = %q, want changed-while-read rejection", err)
	}
}
