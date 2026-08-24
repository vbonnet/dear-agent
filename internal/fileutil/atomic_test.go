package fileutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicWrite_CreatesFileWithContentAndPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	want := []byte("hello: world\n")
	if err := AtomicWrite(path, want, 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("content = %q, want %q", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestAtomicWrite_OverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	if err := AtomicWrite(path, []byte("v1"), 0o600); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := AtomicWrite(path, []byte("version-two-longer"), 0o640); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "version-two-longer" {
		t.Errorf("content = %q, want overwrite", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Errorf("perm = %o, want 640", info.Mode().Perm())
	}
}

func TestAtomicWrite_CreatesMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "state.yaml")

	if err := AtomicWrite(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

// TestAtomicWrite_NoTempLeftBehind verifies the temp file is renamed away (not
// left as clutter) on the success path.
func TestAtomicWrite_NoTempLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	if err := AtomicWrite(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.yaml" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir contents = %v, want exactly [state.yaml]", names)
	}
}

func TestAtomicWrite_DurabilityOperationOrder(t *testing.T) {
	testRoot := t.TempDir()
	logicalDir := filepath.Join(testRoot, "logical", "alias")
	physicalDir := filepath.Join(testRoot, "physical", "parent")
	path := filepath.Join(logicalDir, "state.yaml")
	tmpPath := filepath.Join(physicalDir, ".tmp-state.yaml-fixed")

	var got []string
	record := func(step string) { got = append(got, step) }
	tmp := &recordingAtomicFile{name: tmpPath, record: record}
	parent := &recordingAtomicDir{record: record}
	ops := atomicWriteOps{
		mkdirAll: func(path string, perm os.FileMode) error {
			record(fmt.Sprintf("mkdir:%s:%o", path, perm))
			return nil
		},
		evalSymlinks: func(path string) (string, error) {
			record("resolve:" + path)
			return physicalDir, nil
		},
		createTemp: func(dir, pattern string) (atomicWriteFile, error) {
			record("create:" + dir + ":" + pattern)
			return tmp, nil
		},
		rename: func(oldPath, newPath string) error {
			record("rename:" + oldPath + ":" + newPath)
			return nil
		},
		openDir: func(path string) (atomicWriteDir, error) {
			record("open-dir:" + path)
			return parent, nil
		},
		remove: func(path string) error {
			record("remove:" + path)
			return nil
		},
	}

	if err := atomicWrite(path, []byte("payload"), 0o640, ops); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}

	want := []string{
		fmt.Sprintf("mkdir:%s:700", logicalDir),
		"resolve:" + logicalDir,
		"create:" + physicalDir + ":.tmp-state.yaml-*",
		"write",
		"chmod:640",
		"file-sync",
		"file-close",
		"rename:" + tmpPath + ":" + filepath.Join(physicalDir, "state.yaml"),
		"open-dir:" + physicalDir,
		"dir-sync",
		"dir-close",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("operation order:\n got %v\nwant %v", got, want)
	}
}

func TestAtomicWrite_PrePublishFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile original: %v", err)
	}

	syncErr := errors.New("injected temp sync failure")
	ops := osAtomicWriteOps
	ops.createTemp = func(dir, pattern string) (atomicWriteFile, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &syncFailingAtomicFile{File: file, syncErr: syncErr}, nil
	}

	err := atomicWrite(path, []byte("replacement"), 0o640, ops)
	if !errors.Is(err, syncErr) {
		t.Fatalf("error = %v, want sync failure", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile destination: %v", readErr)
	}
	if string(got) != "original" {
		t.Fatalf("destination = %q, want original content", got)
	}
	assertOnlyDestination(t, dir, "state.yaml")
}

func TestAtomicWrite_PostPublishErrorsPreserveDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile original: %v", err)
	}

	dirSyncErr := errors.New("injected directory sync failure")
	dirCloseErr := errors.New("injected directory close failure")
	ops := osAtomicWriteOps
	ops.openDir = func(string) (atomicWriteDir, error) {
		return &recordingAtomicDir{syncErr: dirSyncErr, closeErr: dirCloseErr}, nil
	}

	err := atomicWrite(path, []byte("published"), 0o640, ops)
	if !errors.Is(err, dirSyncErr) {
		t.Errorf("error = %v, want directory sync failure", err)
	}
	if !errors.Is(err, dirCloseErr) {
		t.Errorf("error = %v, want directory close failure", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile destination: %v", readErr)
	}
	if string(got) != "published" {
		t.Fatalf("destination = %q, want published content", got)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("Stat destination: %v", statErr)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Errorf("perm = %o, want 640", info.Mode().Perm())
	}
	assertOnlyDestination(t, dir, "state.yaml")
}

func TestAtomicWrite_JoinsPrePublishCleanupErrors(t *testing.T) {
	writeErr := errors.New("injected write failure")
	closeErr := errors.New("injected cleanup close failure")
	removeErr := errors.New("injected cleanup remove failure")
	testRoot := t.TempDir()
	physicalDir := filepath.Join(testRoot, "physical")
	logicalPath := filepath.Join(testRoot, "logical", "state.yaml")
	tmp := &recordingAtomicFile{
		name:     filepath.Join(physicalDir, ".tmp-state.yaml-fixed"),
		writeErr: writeErr,
		closeErr: closeErr,
	}
	ops := atomicWriteOps{
		mkdirAll: func(string, os.FileMode) error { return nil },
		evalSymlinks: func(string) (string, error) {
			return physicalDir, nil
		},
		createTemp: func(string, string) (atomicWriteFile, error) { return tmp, nil },
		rename:     func(string, string) error { return nil },
		openDir:    func(string) (atomicWriteDir, error) { return &recordingAtomicDir{}, nil },
		remove:     func(string) error { return removeErr },
	}

	err := atomicWrite(logicalPath, []byte("payload"), 0o600, ops)
	for _, want := range []error{writeErr, closeErr, removeErr} {
		if !errors.Is(err, want) {
			t.Errorf("error = %v, want errors.Is(_, %q)", err, want)
		}
	}
}

func assertOnlyDestination(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("dir contents = %v, want exactly [%s]", names, name)
	}
}

type recordingAtomicFile struct {
	name     string
	record   func(string)
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
}

func (f *recordingAtomicFile) Write(p []byte) (int, error) {
	f.recordStep("write")
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *recordingAtomicFile) Chmod(perm os.FileMode) error {
	f.recordStep(fmt.Sprintf("chmod:%o", perm))
	return f.chmodErr
}

func (f *recordingAtomicFile) Sync() error {
	f.recordStep("file-sync")
	return f.syncErr
}

func (f *recordingAtomicFile) Close() error {
	f.recordStep("file-close")
	return f.closeErr
}

func (f *recordingAtomicFile) Name() string { return f.name }

func (f *recordingAtomicFile) recordStep(step string) {
	if f.record != nil {
		f.record(step)
	}
}

type syncFailingAtomicFile struct {
	*os.File
	syncErr error
}

func (f *syncFailingAtomicFile) Sync() error { return f.syncErr }

type recordingAtomicDir struct {
	record   func(string)
	syncErr  error
	closeErr error
}

func (d *recordingAtomicDir) Sync() error {
	if d.record != nil {
		d.record("dir-sync")
	}
	return d.syncErr
}

func (d *recordingAtomicDir) Close() error {
	if d.record != nil {
		d.record("dir-close")
	}
	return d.closeErr
}
