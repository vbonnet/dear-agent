package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"10MB", 10 << 20, false},
		{"10mb", 10 << 20, false},
		{"512KB", 512 << 10, false},
		{"2G", 2 << 30, false},
		{"1048576", 1048576, false},
		{"5B", 5, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-1MB", 0, true},
	}
	for _, tc := range cases {
		got, err := parseSize(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseSize(%q) = %d, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSize(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"48h", 48 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"0.5d", 12 * time.Hour, false},
		{"bad", 0, true},
		{"xd", 0, true},
	}
	for _, tc := range cases {
		got, err := parseDuration(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseDuration(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRun_RotatesPositionalPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trail.jsonl")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2000), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code, err := run([]string{"--max-size", "1KB", path}, &out, &errOut)
	if err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, errOut.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%s)", code, errOut.String())
	}

	// Live file recreated empty; one rotated .gz sibling present.
	if info, _ := os.Stat(path); info == nil || info.Size() != 0 {
		t.Errorf("live file not reset to empty")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "trail.jsonl.*.gz"))
	if len(matches) != 1 {
		t.Errorf("rotated .gz files = %d, want 1", len(matches))
	}
}

func TestRun_NoMatches(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	code, err := run([]string{filepath.Join(dir, "nope.log")}, &out, &errOut)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRun_DryRunNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2000), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code, err := run([]string{"--max-size", "1KB", "--dry-run", path}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("run: code=%d err=%v", code, err)
	}
	if info, _ := os.Stat(path); info == nil || info.Size() != 2000 {
		t.Errorf("dry-run modified the live file")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "a.log.*"))
	if len(matches) != 0 {
		t.Errorf("dry-run created %d rotated files, want 0", len(matches))
	}
}

func TestRun_BadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := run([]string{"--max-size", "notasize"}, &out, &errOut)
	if err == nil {
		t.Errorf("expected error for bad --max-size")
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRun_TargetsGlob(t *testing.T) {
	dir := t.TempDir()
	for i := range 2 {
		p := filepath.Join(dir, fmt.Sprintf("s%d.log", i))
		if err := os.WriteFile(p, bytes.Repeat([]byte("x"), 2000), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	pattern := filepath.Join(dir, "*.log")
	code, err := run([]string{"--max-size", "1KB", "--no-compress", "--targets", pattern}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("run: code=%d err=%v stderr=%s", code, err, errOut.String())
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "s*.log.*"))
	if len(matches) != 2 {
		t.Errorf("rotated files = %d, want 2", len(matches))
	}
}
