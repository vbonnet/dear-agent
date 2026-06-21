package logrotate

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedNow returns a now func pinned to t, for deterministic timestamps.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// writeFile writes n bytes to path, creating parent dirs as needed.
func writeFile(t *testing.T, path string, n int, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), n), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRotate_SizeThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trail.jsonl")
	writeFile(t, path, 100, 0o644)

	r := New(Config{MaxSizeBytes: 1000})
	if err := r.Rotate(path); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// Under threshold: file untouched, no rotated siblings.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 100 {
		t.Errorf("size = %d, want 100 (unchanged)", info.Size())
	}
	if got := countRotated(t, dir); got != 0 {
		t.Errorf("rotated siblings = %d, want 0", got)
	}
}

func TestRotate_MissingFile(t *testing.T) {
	dir := t.TempDir()
	r := New(Config{MaxSizeBytes: 10})
	if err := r.Rotate(filepath.Join(dir, "absent.log")); err != nil {
		t.Errorf("Rotate on missing file = %v, want nil", err)
	}
}

func TestRotate_OverThreshold(t *testing.T) {
	cases := []struct {
		name     string
		compress bool
		wantExt  string
	}{
		{"uncompressed", false, ""},
		{"compressed", true, ".gz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "trail.jsonl")
			writeFile(t, path, 2000, 0o640)

			now := time.Date(2026, 6, 20, 13, 14, 15, 0, time.Local)
			r := New(Config{MaxSizeBytes: 1000, Compress: tc.compress})
			r.cfg.now = fixedNow(now)

			if err := r.Rotate(path); err != nil {
				t.Fatalf("Rotate: %v", err)
			}

			// Live file exists again and is empty.
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("live file gone: %v", err)
			}
			if info.Size() != 0 {
				t.Errorf("live file size = %d, want 0", info.Size())
			}
			if perm := info.Mode().Perm(); perm != 0o640 {
				t.Errorf("live file mode = %o, want 640", perm)
			}

			// Exactly one rotated sibling with the expected name/extension.
			want := fmt.Sprintf("trail.jsonl.%s%s", now.Format(timestampLayout), tc.wantExt)
			rotated := filepath.Join(dir, want)
			ri, err := os.Stat(rotated)
			if err != nil {
				t.Fatalf("rotated file %s missing: %v", want, err)
			}
			if got := countRotated(t, dir); got != 1 {
				t.Errorf("rotated siblings = %d, want 1", got)
			}

			// Content preserved (decompress if gz).
			data := readMaybeGz(t, rotated, tc.compress)
			if len(data) != 2000 {
				t.Errorf("rotated content = %d bytes, want 2000", len(data))
			}
			if tc.compress && ri.Size() >= 2000 {
				t.Errorf("gz size = %d, expected smaller than original 2000", ri.Size())
			}
		})
	}
}

func TestPrune_MaxAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local)

	// Two old (10 days) and one fresh (1 hour) rotated siblings.
	old1 := now.Add(-10 * 24 * time.Hour)
	old2 := now.Add(-9 * 24 * time.Hour)
	fresh := now.Add(-1 * time.Hour)
	for _, ts := range []time.Time{old1, old2, fresh} {
		name := fmt.Sprintf("trail.jsonl.%s", ts.Format(timestampLayout))
		writeFile(t, filepath.Join(dir, name), 10, 0o644)
	}

	r := New(Config{MaxAge: 7 * 24 * time.Hour, MaxFiles: 100})
	r.cfg.now = fixedNow(now)
	if err := r.Prune(dir); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if got := countRotated(t, dir); got != 1 {
		t.Errorf("rotated remaining = %d, want 1 (only fresh)", got)
	}
	freshName := fmt.Sprintf("trail.jsonl.%s", fresh.Format(timestampLayout))
	if _, err := os.Stat(filepath.Join(dir, freshName)); err != nil {
		t.Errorf("fresh file should survive: %v", err)
	}
}

func TestPrune_MaxFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local)

	// Five recent rotated siblings, distinct timestamps, all within MaxAge.
	var stamps []time.Time
	for i := range 5 {
		ts := now.Add(-time.Duration(i+1) * time.Hour)
		stamps = append(stamps, ts)
		name := fmt.Sprintf("trail.jsonl.%s", ts.Format(timestampLayout))
		writeFile(t, filepath.Join(dir, name), 10, 0o644)
	}

	r := New(Config{MaxAge: 365 * 24 * time.Hour, MaxFiles: 2})
	r.cfg.now = fixedNow(now)
	if err := r.Prune(dir); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if got := countRotated(t, dir); got != 2 {
		t.Errorf("rotated remaining = %d, want 2", got)
	}
	// The two newest (smallest offsets) must survive.
	for _, ts := range stamps[:2] {
		name := fmt.Sprintf("trail.jsonl.%s", ts.Format(timestampLayout))
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("newest file %s should survive: %v", name, err)
		}
	}
}

func TestPrune_GroupsPerBase(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local)

	// Three rotated for "a.log" and three for "b.log", all recent.
	for i := range 3 {
		ts := now.Add(-time.Duration(i+1) * time.Hour).Format(timestampLayout)
		writeFile(t, filepath.Join(dir, "a.log."+ts), 10, 0o644)
		writeFile(t, filepath.Join(dir, "b.log."+ts), 10, 0o644)
	}

	r := New(Config{MaxAge: 365 * 24 * time.Hour, MaxFiles: 1})
	r.cfg.now = fixedNow(now)
	if err := r.Prune(dir); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// MaxFiles=1 applies per base: one "a" and one "b" survive.
	if got := countRotated(t, dir); got != 2 {
		t.Errorf("rotated remaining = %d, want 2 (one per base)", got)
	}
}

func TestPrune_IgnoresNonRotated(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local)

	// Live log and an unrelated file should never be pruned.
	writeFile(t, filepath.Join(dir, "trail.jsonl"), 10, 0o644)
	writeFile(t, filepath.Join(dir, "notes.txt"), 10, 0o644)
	// One genuinely old rotated sibling.
	oldName := "trail.jsonl." + now.Add(-30*24*time.Hour).Format(timestampLayout)
	writeFile(t, filepath.Join(dir, oldName), 10, 0o644)

	r := New(Config{MaxAge: 7 * 24 * time.Hour, MaxFiles: 10})
	r.cfg.now = fixedNow(now)
	if err := r.Prune(dir); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	for _, keep := range []string{"trail.jsonl", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s should not be pruned: %v", keep, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, oldName)); !os.IsNotExist(err) {
		t.Errorf("old rotated file should be pruned, stat err = %v", err)
	}
}

func TestDryRun(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.Local)

	// A live log over threshold and an old rotated sibling eligible for prune.
	path := filepath.Join(dir, "trail.jsonl")
	writeFile(t, path, 5000, 0o644)
	oldName := "trail.jsonl." + now.Add(-30*24*time.Hour).Format(timestampLayout)
	writeFile(t, filepath.Join(dir, oldName), 10, 0o644)

	var buf bytes.Buffer
	r := New(Config{MaxSizeBytes: 1000, MaxAge: 7 * 24 * time.Hour, MaxFiles: 1, Compress: true, DryRun: true})
	r.cfg.now = fixedNow(now)
	r.cfg.stderr = &buf

	if err := r.Rotate(path); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if err := r.Prune(dir); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Nothing changed: live file still 5000 bytes, old sibling still present,
	// no new rotated file created.
	if info, _ := os.Stat(path); info == nil || info.Size() != 5000 {
		t.Errorf("live file modified in dry-run")
	}
	if _, err := os.Stat(filepath.Join(dir, oldName)); err != nil {
		t.Errorf("old sibling removed in dry-run: %v", err)
	}
	if got := countRotated(t, dir); got != 1 {
		t.Errorf("rotated siblings = %d, want 1 (no new file)", got)
	}

	out := buf.String()
	if !strings.Contains(out, "would rotate") {
		t.Errorf("dry-run output missing rotate notice: %q", out)
	}
	if !strings.Contains(out, "would prune") {
		t.Errorf("dry-run output missing prune notice: %q", out)
	}
}

func TestNew_Defaults(t *testing.T) {
	r := New(Config{})
	if r.cfg.MaxSizeBytes != DefaultMaxSizeBytes {
		t.Errorf("MaxSizeBytes = %d, want %d", r.cfg.MaxSizeBytes, DefaultMaxSizeBytes)
	}
	if r.cfg.MaxAge != DefaultMaxAge {
		t.Errorf("MaxAge = %v, want %v", r.cfg.MaxAge, DefaultMaxAge)
	}
	if r.cfg.MaxFiles != DefaultMaxFiles {
		t.Errorf("MaxFiles = %d, want %d", r.cfg.MaxFiles, DefaultMaxFiles)
	}
}

// countRotated counts rotated siblings (parseable timestamp names) in dir.
func countRotated(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if _, _, ok := parseRotatedName(e.Name()); ok {
			n++
		}
	}
	return n
}

// readMaybeGz reads path, transparently decompressing when gz is true.
func readMaybeGz(t *testing.T, path string, gz bool) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var rd io.Reader = f
	if gz {
		gr, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer gr.Close()
		rd = gr
	}
	data, err := io.ReadAll(rd)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
