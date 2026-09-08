//go:build darwin || linux

package steps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestScanBuildAuthorityWaitOwnershipRejectsSourcePathRaces(t *testing.T) {
	t.Run("initial symlink", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "payload.txt")
		if err := os.WriteFile(target, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatalf("write symlink payload: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(root, "source.go")); err != nil {
			t.Fatalf("create source symlink: %v", err)
		}

		_, err := scanBuildAuthorityWaitOwnership(
			context.Background(), root, defaultBuildAuthorityWaitOwnershipLimits(),
		)
		if err == nil || !strings.Contains(err.Error(), "is a symbolic link") {
			t.Fatalf("initial-symlink error = %v", err)
		}
	})

	t.Run("consumed file changes before final snapshot", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		original, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat original source: %v", err)
		}
		_, err = scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{beforeSnapshotValidation: func() error {
				time.Sleep(2 * time.Millisecond)
				if err := os.WriteFile(path, []byte("package foreign\n"), 0o600); err != nil {
					return err
				}
				return os.Chtimes(path, original.ModTime(), original.ModTime())
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "final production-source snapshot") {
			t.Fatalf("final file-snapshot error = %v", err)
		}
	})

	t.Run("completed nested directory becomes symlink before final snapshot", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "package", "nested")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "package/nested/source.go", "package fixture\n")
		foreign := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, foreign, "source.go", `package foreign
import "syscall"
func reap() { _, _ = syscall.Wait4(-1, nil, 0, nil) }
`)
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{beforeSnapshotValidation: func() error {
				if err := os.Rename(nested, nested+"-retained"); err != nil {
					return err
				}
				return os.Symlink(foreign, nested)
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "final production-source snapshot") {
			t.Fatalf("final directory-snapshot error = %v", err)
		}
	})

	t.Run("regular file replacement before open", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{beforeSourceOpen: func(_, _ string) error {
				if err := os.Rename(path, filepath.Join(root, "old.txt")); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("package replacement\n"), 0o600)
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "changed identity before opening") {
			t.Fatalf("pre-open replacement error = %v", err)
		}
	})

	t.Run("symlink replacement before open", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		target := filepath.Join(root, "payload.txt")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		if err := os.WriteFile(target, []byte("package payload\n"), 0o600); err != nil {
			t.Fatalf("write replacement payload: %v", err)
		}
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{beforeSourceOpen: func(_, _ string) error {
				if err := os.Remove(path); err != nil {
					return err
				}
				return os.Symlink(target, path)
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "changed to a symbolic link") {
			t.Fatalf("pre-open symlink replacement error = %v", err)
		}
	})

	t.Run("path replacement after open", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{afterSourceOpen: func(_, _ string) error {
				if err := os.Rename(path, filepath.Join(root, "old.txt")); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("package replacement\n"), 0o600)
			}},
		)
		if err == nil || (!strings.Contains(err.Error(), "changed after descriptor open") &&
			!strings.Contains(err.Error(), "source identity changed")) {
			t.Fatalf("post-open replacement error = %v", err)
		}
	})

	t.Run("in-place mutation after open", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{afterSourceOpen: func(_, _ string) error {
				return os.WriteFile(path, []byte("package fixture\n// changed after descriptor open\n"), 0o600)
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "changed after descriptor open") {
			t.Fatalf("in-place mutation error = %v", err)
		}
	})

	t.Run("same-size mutation with restored mtime", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		original, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat original source: %v", err)
		}
		_, err = scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{afterSourceOpen: func(_, _ string) error {
				time.Sleep(2 * time.Millisecond)
				if err := os.WriteFile(path, []byte("package foreign\n"), 0o600); err != nil {
					return err
				}
				return os.Chtimes(path, original.ModTime(), original.ModTime())
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "changed after descriptor open") {
			t.Fatalf("same-size mutation error = %v", err)
		}
	})

	t.Run("transient directory mutation with restored mtime", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		original, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat original root: %v", err)
		}
		_, err = scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{afterSourceOpen: func(_, _ string) error {
				time.Sleep(2 * time.Millisecond)
				transient := filepath.Join(root, "transient.go")
				if err := os.WriteFile(transient, []byte("package transient\n"), 0o600); err != nil {
					return err
				}
				if err := os.Remove(transient); err != nil {
					return err
				}
				return os.Chtimes(root, original.ModTime(), original.ModTime())
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "changed while scanning") {
			t.Fatalf("transient-directory mutation error = %v", err)
		}
	})

	t.Run("parent directory symlink substitution", func(t *testing.T) {
		root := t.TempDir()
		packagePath := filepath.Join(root, "package")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "package/source.go", "package fixture\n")
		foreign := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, foreign, "source.go", `package foreign
import "syscall"
func reap() { _, _ = syscall.Wait4(-1, nil, 0, nil) }
`)
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			context.Background(),
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{beforeSourceOpen: func(_, _ string) error {
				if err := os.Rename(packagePath, packagePath+"-retained"); err != nil {
					return err
				}
				return os.Symlink(foreign, packagePath)
			}},
		)
		if err == nil || !strings.Contains(err.Error(), "changed") {
			t.Fatalf("parent-directory substitution error = %v", err)
		}
	})

	t.Run("FIFO replacement cannot block", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "source.go")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "source.go", "package fixture\n")
		result := make(chan error, 1)
		go func() {
			_, err := scanBuildAuthorityWaitOwnershipWithHooks(
				context.Background(),
				root,
				defaultBuildAuthorityWaitOwnershipLimits(),
				buildAuthorityWaitOwnershipHooks{beforeSourceOpen: func(_, _ string) error {
					if err := os.Remove(path); err != nil {
						return err
					}
					return unix.Mkfifo(path, 0o600)
				}},
			)
			result <- err
		}()
		select {
		case err := <-result:
			if err == nil {
				t.Fatal("FIFO replacement unexpectedly passed")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("FIFO replacement blocked the bounded scanner")
		}
	})
}

func TestScanBuildAuthorityWaitOwnershipReadsDirectoriesInBoundedBatchesAndClosesThem(t *testing.T) {
	root := t.TempDir()
	for index := range buildAuthorityDirectoryReadBatch + 1 {
		name := filepath.Join(root, fmt.Sprintf("entry-%03d.txt", index))
		if err := os.WriteFile(name, []byte("fixture\n"), 0o600); err != nil {
			t.Fatalf("write batching fixture: %v", err)
		}
	}
	var requests []int
	var directories []*os.File
	_, err := scanBuildAuthorityWaitOwnershipWithHooks(
		context.Background(),
		root,
		defaultBuildAuthorityWaitOwnershipLimits(),
		buildAuthorityWaitOwnershipHooks{readDirectory: func(
			directory *os.File,
			count int,
		) ([]os.DirEntry, error) {
			requests = append(requests, count)
			directories = append(directories, directory)
			return directory.ReadDir(count)
		}},
	)
	if err != nil {
		t.Fatalf("scan batching fixture: %v", err)
	}
	if len(requests) < 2 {
		t.Fatalf("ReadDir requests = %v, want multiple bounded batches", requests)
	}
	for _, count := range requests {
		if count != buildAuthorityDirectoryReadBatch {
			t.Fatalf("ReadDir count = %d, want %d", count, buildAuthorityDirectoryReadBatch)
		}
	}
	for _, directory := range directories {
		if _, err := directory.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("retained directory descriptor remained open: %v", err)
		}
	}
}
