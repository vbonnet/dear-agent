package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestContinuationReceiptKeyIsPrivateStableAndReused(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatalf("create explicit XDG state root: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateRoot)

	first, err := loadOrCreateContinuationReceiptKey()
	if err != nil {
		t.Fatalf("create continuation key: %v", err)
	}
	keyPath, err := continuationReceiptKeyPath()
	if err != nil {
		t.Fatalf("resolve continuation key path: %v", err)
	}
	firstInfo, err := os.Lstat(keyPath)
	if err != nil {
		t.Fatalf("inspect first continuation key: %v", err)
	}
	second, err := loadOrCreateContinuationReceiptKey()
	if err != nil {
		t.Fatalf("reload continuation key: %v", err)
	}
	secondInfo, err := os.Lstat(keyPath)
	if err != nil {
		t.Fatalf("inspect reloaded continuation key: %v", err)
	}
	if len(first) != continuationReceiptKeyBytes || !bytes.Equal(first, second) {
		t.Fatal("continuation key was not reused byte-for-byte")
	}
	if !os.SameFile(firstInfo, secondInfo) {
		t.Fatal("continuation key leaf was replaced during reload")
	}
	if runtime.GOOS != "windows" && firstInfo.Mode().Perm() != 0o600 {
		t.Fatalf("continuation key mode = %04o, want 0600", firstInfo.Mode().Perm())
	}
}

func TestContinuationReceiptKeyAcceptsSharedStateNamespace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions are required")
	}
	tests := []struct {
		name     string
		explicit bool
	}{
		{name: "home fallback"},
		{name: "explicit XDG root", explicit: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			var stateRoot string
			if test.explicit {
				stateRoot = filepath.Join(base, "state")
				if err := os.Mkdir(stateRoot, 0o700); err != nil {
					t.Fatalf("create explicit XDG state root: %v", err)
				}
				t.Setenv("XDG_STATE_HOME", stateRoot)
			} else {
				t.Setenv("HOME", base)
				t.Setenv("XDG_STATE_HOME", "")
				stateRoot = filepath.Join(base, ".local", "state")
			}
			sharedStateDirectory := filepath.Join(stateRoot, "dear-agent")
			if err := os.MkdirAll(sharedStateDirectory, 0o755); err != nil {
				t.Fatalf("create shared state namespace: %v", err)
			}
			if err := os.Chmod(sharedStateDirectory, 0o755); err != nil {
				t.Fatalf("set shared state namespace mode: %v", err)
			}
			before, err := os.Lstat(sharedStateDirectory)
			if err != nil {
				t.Fatalf("inspect shared state namespace before key creation: %v", err)
			}

			first, err := loadOrCreateContinuationReceiptKey()
			if err != nil {
				t.Fatalf("create continuation key below shared state namespace: %v", err)
			}
			second, err := loadOrCreateContinuationReceiptKey()
			if err != nil {
				t.Fatalf("reload continuation key below shared state namespace: %v", err)
			}
			loaded, err := loadContinuationReceiptKey()
			if err != nil {
				t.Fatalf("load continuation key below shared state namespace: %v", err)
			}
			after, err := os.Lstat(sharedStateDirectory)
			if err != nil {
				t.Fatalf("inspect shared state namespace after key creation: %v", err)
			}
			if !os.SameFile(before, after) {
				t.Fatal("shared state namespace was replaced")
			}
			if got := after.Mode().Perm(); got != 0o755 {
				t.Fatalf("shared state namespace mode = %04o, want unchanged 0755", got)
			}
			if !bytes.Equal(first, second) || !bytes.Equal(first, loaded) {
				t.Fatal("continuation key below shared state namespace was not stable")
			}
			privateDirectory := filepath.Join(sharedStateDirectory, "resolve-review-threads")
			privateInfo, err := os.Lstat(privateDirectory)
			if err != nil {
				t.Fatalf("inspect private continuation directory: %v", err)
			}
			if got := privateInfo.Mode().Perm(); got != 0o700 {
				t.Fatalf("private continuation directory mode = %04o, want 0700", got)
			}
			keyInfo, err := os.Lstat(filepath.Join(privateDirectory, continuationReceiptKeyFile))
			if err != nil {
				t.Fatalf("inspect continuation key: %v", err)
			}
			if got := keyInfo.Mode().Perm(); got != 0o600 {
				t.Fatalf("continuation key mode = %04o, want 0600", got)
			}
		})
	}
}

func TestContinuationReceiptKeyRejectsUnsafeSharedStateNamespace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions are required")
	}
	tests := []struct {
		name string
		mode os.FileMode
		file bool
	}{
		{name: "group writable", mode: 0o775},
		{name: "world writable", mode: 0o777},
		{name: "not a directory", file: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(stateRoot, 0o700); err != nil {
				t.Fatalf("create explicit XDG state root: %v", err)
			}
			t.Setenv("XDG_STATE_HOME", stateRoot)
			sharedStateDirectory := filepath.Join(stateRoot, "dear-agent")
			if test.file {
				if err := os.WriteFile(sharedStateDirectory, []byte("not a directory"), 0o600); err != nil {
					t.Fatalf("create non-directory shared state fixture: %v", err)
				}
			} else {
				if err := os.Mkdir(sharedStateDirectory, 0o700); err != nil {
					t.Fatalf("create shared state fixture: %v", err)
				}
				if err := os.Chmod(sharedStateDirectory, test.mode); err != nil {
					t.Fatalf("set unsafe shared state mode: %v", err)
				}
			}

			if _, err := loadOrCreateContinuationReceiptKey(); err == nil {
				t.Fatal("unsafe shared state namespace was accepted")
			}
			privateDirectory := filepath.Join(sharedStateDirectory, "resolve-review-threads")
			if _, err := os.Lstat(privateDirectory); err == nil {
				t.Fatal("unsafe shared state created private continuation state")
			}
		})
	}
}

func TestLoadContinuationReceiptKeyRejectsUnsafeSharedStateNamespace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions and symlinks are required")
	}
	tests := []struct {
		name   string
		mutate func(t *testing.T, stateRoot, sharedStateDirectory string)
	}{
		{
			name: "group writable",
			mutate: func(t *testing.T, _, sharedStateDirectory string) {
				t.Helper()
				if err := os.Chmod(sharedStateDirectory, 0o775); err != nil {
					t.Fatalf("make shared state namespace group writable: %v", err)
				}
			},
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, stateRoot, sharedStateDirectory string) {
				t.Helper()
				redirect := filepath.Join(stateRoot, "redirected-dear-agent")
				if err := os.Rename(sharedStateDirectory, redirect); err != nil {
					t.Fatalf("move shared state namespace for symlink fixture: %v", err)
				}
				if err := os.Symlink(redirect, sharedStateDirectory); err != nil {
					t.Fatalf("replace shared state namespace with symlink: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(stateRoot, 0o700); err != nil {
				t.Fatalf("create explicit XDG state root: %v", err)
			}
			t.Setenv("XDG_STATE_HOME", stateRoot)
			if _, err := loadOrCreateContinuationReceiptKey(); err != nil {
				t.Fatalf("create safe continuation key fixture: %v", err)
			}
			sharedStateDirectory := filepath.Join(stateRoot, "dear-agent")
			test.mutate(t, stateRoot, sharedStateDirectory)
			if _, err := loadContinuationReceiptKey(); err == nil {
				t.Fatal("continuation key loaded through unsafe shared state namespace")
			}
		})
	}
}

func TestConcurrentContinuationReceiptKeyCreationConverges(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatalf("create explicit XDG state root: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", stateRoot)

	const workers = 12
	keys := make([][]byte, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range workers {
		group.Go(func() {
			<-start
			keys[index], errs[index] = loadOrCreateContinuationReceiptKey()
		})
	}
	close(start)
	group.Wait()

	for index := range workers {
		if errs[index] != nil {
			t.Fatalf("worker %d key creation: %v", index, errs[index])
		}
		if !bytes.Equal(keys[0], keys[index]) {
			t.Fatalf("worker %d observed a different continuation key", index)
		}
	}
}

func TestExplicitXDGStateRootBelowExecuteOnlyAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execute-only ancestor semantics are required")
	}
	base := t.TempDir()
	externalAncestor := filepath.Join(base, "execute-only")
	if err := os.Mkdir(externalAncestor, 0o700); err != nil {
		t.Fatalf("create external ancestor: %v", err)
	}
	stateRoot := filepath.Join(externalAncestor, "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatalf("create explicit XDG state root: %v", err)
	}
	if err := os.Chmod(externalAncestor, 0o100); err != nil {
		t.Fatalf("make external ancestor execute-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(externalAncestor, 0o700) })
	t.Setenv("XDG_STATE_HOME", stateRoot)

	if _, err := loadOrCreateContinuationReceiptKey(); err != nil {
		t.Fatalf("create continuation key below execute-only external ancestor: %v", err)
	}
}

func TestReadContinuationReceiptKeyRejectsUnsafeLeaf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX key modes and symlink semantics are required")
	}
	tests := []struct {
		name    string
		content []byte
		mode    os.FileMode
	}{
		{name: "short", content: bytes.Repeat([]byte{'s'}, continuationReceiptKeyBytes-1), mode: 0o600},
		{name: "long", content: bytes.Repeat([]byte{'l'}, continuationReceiptKeyBytes+1), mode: 0o600},
		{name: "over permissive", content: bytes.Repeat([]byte{'p'}, continuationReceiptKeyBytes), mode: 0o644},
		{name: "wrong private mode", content: bytes.Repeat([]byte{'r'}, continuationReceiptKeyBytes), mode: 0o400},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatalf("protect key directory: %v", err)
			}
			path := filepath.Join(dir, continuationReceiptKeyFile)
			if err := os.WriteFile(path, test.content, test.mode); err != nil {
				t.Fatalf("write unsafe key fixture: %v", err)
			}
			if err := os.Chmod(path, test.mode); err != nil {
				t.Fatalf("set unsafe key mode: %v", err)
			}
			if _, err := readContinuationReceiptKey(path); err == nil {
				t.Fatal("unsafe continuation key leaf was accepted")
			}
		})
	}
}

func TestReadContinuationReceiptKeyRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("protect key directory: %v", err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, bytes.Repeat([]byte{'k'}, continuationReceiptKeyBytes), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	link := filepath.Join(dir, continuationReceiptKeyFile)
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("create Windows key symlink without required host privilege: %v", err)
		}
		t.Fatalf("create key symlink: %v", err)
	}
	if _, err := readContinuationReceiptKey(link); err == nil {
		t.Fatal("continuation key symlink was accepted")
	}
}

func TestEnsurePrivateContinuationDirectoryRejectsManagedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	stateRoot := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateRoot)
	redirect := filepath.Join(t.TempDir(), "redirect")
	if err := os.Mkdir(redirect, 0o700); err != nil {
		t.Fatalf("create redirect directory: %v", err)
	}
	if err := os.Symlink(redirect, filepath.Join(stateRoot, "dear-agent")); err != nil {
		t.Fatalf("create managed-directory symlink: %v", err)
	}
	keyPath, err := continuationReceiptKeyPath()
	if err != nil {
		t.Fatalf("resolve continuation key path: %v", err)
	}
	if err := ensurePrivateContinuationDirectory(filepath.Dir(keyPath)); err == nil {
		t.Fatal("managed continuation directory symlink was accepted")
	}
}
