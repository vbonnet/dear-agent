package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOSArch_Supported(t *testing.T) {
	osArch, err := resolveOSArch()
	if err != nil {
		t.Fatalf("resolveOSArch: %v", err)
	}
	if _, ok := pinnedDigests[osArch]; !ok {
		t.Fatalf("resolveOSArch returned %q which has no pinned digest", osArch)
	}
}

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")
	want := []byte("hello bumblebee")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	h := sha256.Sum256(want)
	if got != hex.EncodeToString(h[:]) {
		t.Fatalf("sha256File = %q, want %q", got, hex.EncodeToString(h[:]))
	}
}

func TestInstallBinary_HappyPath(t *testing.T) {
	// Build a tar.gz containing a stand-in `bumblebee` binary and serve it
	// from a test HTTP server. Pin the digest matching that tarball, point
	// the installer at the test server, and assert the binary lands in the
	// prefix with the right mode.
	osArch, err := resolveOSArch()
	if err != nil {
		t.Fatalf("resolveOSArch: %v", err)
	}

	tarballBytes := buildTarball(t, "bumblebee", []byte("#!/bin/sh\necho stub\n"))
	digest := sha256.Sum256(tarballBytes)
	digestHex := hex.EncodeToString(digest[:])

	// Preserve and restore the pinned digest and release URL for this os/arch.
	prevDigest := pinnedDigests[osArch]
	pinnedDigests[osArch] = digestHex
	t.Cleanup(func() { pinnedDigests[osArch] = prevDigest })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".tar.gz") {
			http.NotFound(w, r)
			return
		}
		w.Write(tarballBytes)
	}))
	t.Cleanup(srv.Close)

	prevBase := releaseURLBase
	releaseURLBase = srv.URL
	t.Cleanup(func() { releaseURLBase = prevBase })

	prefix := t.TempDir()
	if err := installBinary(prefix, filepath.Join(prefix, "bumblebee")); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	info, err := os.Stat(filepath.Join(prefix, "bumblebee"))
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary not executable: mode=%v", info.Mode())
	}
}

func TestInstallBinary_ChecksumMismatch(t *testing.T) {
	osArch, err := resolveOSArch()
	if err != nil {
		t.Fatalf("resolveOSArch: %v", err)
	}

	tarballBytes := buildTarball(t, "bumblebee", []byte("payload"))
	// Pin a wrong digest so the verifier rejects the tarball.
	prevDigest := pinnedDigests[osArch]
	pinnedDigests[osArch] = strings.Repeat("0", 64)
	t.Cleanup(func() { pinnedDigests[osArch] = prevDigest })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarballBytes)
	}))
	t.Cleanup(srv.Close)

	prevBase := releaseURLBase
	releaseURLBase = srv.URL
	t.Cleanup(func() { releaseURLBase = prevBase })

	prefix := t.TempDir()
	err = installBinary(prefix, filepath.Join(prefix, "bumblebee"))
	if err == nil {
		t.Fatal("installBinary succeeded with wrong digest; want CHECKSUM MISMATCH")
	}
	if !strings.Contains(err.Error(), "CHECKSUM MISMATCH") {
		t.Fatalf("installBinary error = %v; want CHECKSUM MISMATCH", err)
	}
	if _, statErr := os.Stat(filepath.Join(prefix, "bumblebee")); !os.IsNotExist(statErr) {
		t.Fatalf("binary was installed despite checksum mismatch: %v", statErr)
	}
}

func TestUninstallBinary_MissingIsFine(t *testing.T) {
	dir := t.TempDir()
	if err := uninstallBinary(filepath.Join(dir, "absent")); err != nil {
		t.Fatalf("uninstallBinary on missing target: %v", err)
	}
}

func TestUninstallBinary_RemovesExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bumblebee")
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := uninstallBinary(target); err != nil {
		t.Fatalf("uninstallBinary: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists after uninstall: %v", err)
	}
}

// buildTarball writes a single-file tar.gz to memory and returns the bytes.
func buildTarball(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tarball: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file Close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tarball: %v", err)
	}
	return data
}
