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

// TestChecksumAssetName pins the published checksum asset name. Jaeger names it
// after the platform tuple WITHOUT the archive extension
// (jaeger-2.19.0-darwin-arm64.sha256sum.txt), not as "<tarball>.sha256sum.txt".
// Requesting the latter returns 404 and breaks `otel-local up --fetch`.
func TestChecksumAssetName(t *testing.T) {
	got := checksumAssetName("v2.19.0", "darwin", "arm64")
	want := "jaeger-2.19.0-darwin-arm64.sha256sum.txt"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got == assetName("v2.19.0", "darwin", "arm64")+".sha256sum.txt" {
		t.Error("checksum asset must not be the tarball name with a suffix appended")
	}
}

// TestParseChecksumRealJaegerFormat uses the verbatim body of the published
// jaeger-2.19.0-darwin-arm64.sha256sum.txt. The file digests the files INSIDE
// the archive, so there is no entry for the tarball itself; verification must
// target the extracted "jaeger" binary.
func TestParseChecksumRealJaegerFormat(t *testing.T) {
	contents := "06e03f4aeec5fc0fbee9632383550ef520aec51274e5893eaae61cfd48422dc6 *jaeger-2.19.0-darwin-arm64/example-hotrod\n" +
		"c8888ba69550490b69c0a545ea9b3e07a349fd99feba226eaba764e62ebcbebf *jaeger-2.19.0-darwin-arm64/jaeger\n"

	got, err := parseChecksum(contents, "jaeger")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "c8888ba69550490b69c0a545ea9b3e07a349fd99feba226eaba764e62ebcbebf"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// The archive itself is deliberately absent from the file.
	if _, err := parseChecksum(contents, "jaeger-2.19.0-darwin-arm64.tar.gz"); err == nil {
		t.Error("expected no checksum entry for the tarball, got one")
	}
}

// TestParseChecksumBinaryModeMarker covers a flat entry carrying sha256sum's
// binary-mode "*" marker, where the marker is not absorbed by a path component.
func TestParseChecksumBinaryModeMarker(t *testing.T) {
	got, err := parseChecksum("abc123 *jaeger\n", "jaeger")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc123" {
		t.Errorf("got %q, want abc123", got)
	}
}

// tarWithJaeger builds a gzip-compressed tar carrying body as the nested
// `jaeger` binary, mirroring the layout of a real Jaeger release archive.
func tarWithJaeger(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name: "jaeger-2.19.0-darwin-arm64/jaeger",
		Mode: 0o644,
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestInstallVerifiedRejectsBadChecksum covers OTEL-LOCAL-12: a mismatch must
// leave nothing behind at the launch path, and no stray .incoming file.
func TestInstallVerifiedRejectsBadChecksum(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "cache", "jaeger")
	tarball := tarWithJaeger(t, []byte("malicious payload"))

	err := installVerified(tarball, "00000000000000000000000000000000000000000000000000000000deadbeef", dst)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("unverified binary was left at the launch path %s", dst)
	}
	if _, statErr := os.Stat(dst + ".incoming"); !os.IsNotExist(statErr) {
		t.Error("temp .incoming file was left behind")
	}
}

// TestInstallVerifiedInstallsOnMatch is the happy path: correct digest lands an
// executable at dst.
func TestInstallVerifiedInstallsOnMatch(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "cache", "jaeger")
	body := []byte("#!/bin/sh\necho jaeger\n")
	sum := sha256.Sum256(body)

	if err := installVerified(tarWithJaeger(t, body), hex.EncodeToString(sum[:]), dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("content mismatch: got %q", got)
	}
	if !isExecutable(dst) {
		t.Error("installed binary is not executable")
	}
}
