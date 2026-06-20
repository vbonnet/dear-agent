package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		version, goos, goarch, want string
	}{
		{"v2.19.0", "darwin", "arm64", "jaeger-2.19.0-darwin-arm64.tar.gz"},
		{"2.19.0", "linux", "amd64", "jaeger-2.19.0-linux-amd64.tar.gz"},
	}
	for _, c := range cases {
		if got := assetName(c.version, c.goos, c.goarch); got != c.want {
			t.Errorf("assetName(%q,%q,%q) = %q, want %q", c.version, c.goos, c.goarch, got, c.want)
		}
	}
}

func TestParseChecksum(t *testing.T) {
	contents := "deadbeef  jaeger-2.19.0-darwin-arm64.tar.gz\ncafef00d  other-file.tar.gz\n"
	got, err := parseChecksum(contents, "jaeger-2.19.0-darwin-arm64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "deadbeef" {
		t.Errorf("got %q, want deadbeef", got)
	}

	if _, err := parseChecksum(contents, "missing.tar.gz"); err == nil {
		t.Error("expected error for missing asset, got nil")
	}
}

func TestParseChecksumPathQualified(t *testing.T) {
	// Some checksum files list a path; we match on the basename.
	contents := "abc123  ./dist/jaeger-2.19.0-linux-amd64.tar.gz\n"
	got, err := parseChecksum(contents, "jaeger-2.19.0-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc123" {
		t.Errorf("got %q, want abc123", got)
	}
}

func TestExtractBinary(t *testing.T) {
	// Build a gzip-compressed tar with a nested `jaeger` binary and a decoy.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	want := []byte("#!/bin/sh\necho jaeger\n")
	files := []struct {
		name string
		body []byte
	}{
		{"jaeger-2.19.0-darwin-arm64/README.md", []byte("docs")},
		{"jaeger-2.19.0-darwin-arm64/jaeger", want},
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "jaeger")
	if err := extractBinary(buf.Bytes(), "jaeger", dst); err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted body = %q, want %q", got, want)
	}
}

func TestExtractBinaryNotFound(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "only-docs.txt", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()

	dst := filepath.Join(t.TempDir(), "jaeger")
	if err := extractBinary(buf.Bytes(), "jaeger", dst); err == nil {
		t.Error("expected error when binary missing, got nil")
	}
}

// TestChecksumRoundTrip documents the verify path: a real digest computed over
// bytes matches what parseChecksum pulls from an sha256sum-format file.
func TestChecksumRoundTrip(t *testing.T) {
	payload := []byte("pretend tarball")
	sum := sha256.Sum256(payload)
	hexsum := hex.EncodeToString(sum[:])
	file := hexsum + "  jaeger-2.19.0-darwin-arm64.tar.gz\n"

	got, err := parseChecksum(file, "jaeger-2.19.0-darwin-arm64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != hexsum {
		t.Errorf("got %q, want %q", got, hexsum)
	}
}
