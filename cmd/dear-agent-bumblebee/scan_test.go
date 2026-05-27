package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDefaultOutputPath_DatedFile(t *testing.T) {
	now := time.Date(2026, 5, 26, 4, 0, 0, 0, time.UTC)
	got, err := defaultOutputPath(now)
	if err != nil {
		t.Fatalf("defaultOutputPath: %v", err)
	}
	if !strings.HasSuffix(got, "2026-05-26.ndjson") {
		t.Fatalf("defaultOutputPath = %q, want suffix 2026-05-26.ndjson", got)
	}
	// Per-platform parent dir layout.
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(got, "Library/Application Support/dear-agent/bumblebee") {
			t.Fatalf("darwin path %q missing Application Support segment", got)
		}
	case "linux":
		if !strings.Contains(got, "dear-agent/bumblebee") {
			t.Fatalf("linux path %q missing dear-agent/bumblebee segment", got)
		}
	}
}

func TestDefaultOutputPath_LinuxRespectsXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_DATA_HOME only honored on linux branch")
	}
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	got, err := defaultOutputPath(time.Now())
	if err != nil {
		t.Fatalf("defaultOutputPath: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("defaultOutputPath = %q, want prefix %q", got, dir)
	}
}

func TestResolveBumblebeeBinary_PrefersEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bumblebee")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed bin: %v", err)
	}
	t.Setenv("BUMBLEBEE_BIN", bin)
	t.Setenv("PATH", "")

	got, err := resolveBumblebeeBinary()
	if err != nil {
		t.Fatalf("resolveBumblebeeBinary: %v", err)
	}
	if got != bin {
		t.Fatalf("resolveBumblebeeBinary = %q, want %q", got, bin)
	}
}

func TestResolveBumblebeeBinary_NotFound(t *testing.T) {
	t.Setenv("BUMBLEBEE_BIN", "")
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	_, err := resolveBumblebeeBinary()
	if err == nil {
		t.Fatal("resolveBumblebeeBinary succeeded with no binary present; want error")
	}
	if !strings.Contains(err.Error(), "make bumblebee-install") {
		t.Fatalf("error %q should point at the install path", err)
	}
}

func TestDiscoverCatalog_EnvWins(t *testing.T) {
	dir := t.TempDir()
	cat := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(cat, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("BUMBLEBEE_CATALOG", cat)
	if got := discoverCatalog(); got != cat {
		t.Fatalf("discoverCatalog = %q, want %q", got, cat)
	}
}

func TestDiscoverCatalog_MissingEnvIgnored(t *testing.T) {
	t.Setenv("BUMBLEBEE_CATALOG", "/nonexistent/catalog.json")
	// Whatever discoverCatalog returns must not be the bogus env path.
	if got := discoverCatalog(); got == "/nonexistent/catalog.json" {
		t.Fatalf("discoverCatalog returned a non-existent env path: %q", got)
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.ndjson")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	n, err := countLines(path)
	if err != nil {
		t.Fatalf("countLines: %v", err)
	}
	if n != 3 {
		t.Fatalf("countLines = %d, want 3", n)
	}
}
