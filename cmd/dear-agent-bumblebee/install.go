package main

// install.go — fetch a pinned, checksum-verified Bumblebee release.
//
// Why pin + verify: per ADR-027, Bumblebee is the developer-endpoint scanner.
// The whole tool's value is "we'll tell you when something installed on your
// machine is bad," so the scanner itself must not be a supply-chain hole.
// Pinning to a specific tag with embedded SHA-256s captured at audit time
// means a re-uploaded release (or a MITM on the download) is detected, not
// laundered through.
//
// The digests below were captured 2026-05-26 from two independent sources
// that matched:
//  1. github.com/perplexityai/bumblebee API release-asset digests
//  2. Upstream checksums.txt (sha256 27356f23…3394c5d2) attached to v0.1.1

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BumblebeeVersion is the pinned upstream tag. Bumping it requires recapturing
// the SHA-256s in pinnedDigests from two independent sources.
const BumblebeeVersion = "v0.1.1"

// pinnedDigests maps "<os>_<arch>" → SHA-256 of the release tarball.
var pinnedDigests = map[string]string{
	"darwin_amd64": "dd3b2573a974a2786f58215483420fa11cf62b39ff4032693f1440575940dc25",
	"darwin_arm64": "dc0a620e54e85f998c2280b0323763c342973a25eda475d8036d16b01820a2bf",
	"linux_amd64":  "0ef1c56c85a67c10f7211883c0eb5fb902de705cc30bbca0bc6f4d60941547da",
	"linux_arm64":  "41aad0296bb6c88e746b237ed32eaa3b9b93c48770a51cd66f736f8a4d07a7d1",
}

// releaseURLBase is overridable in tests; production points at GitHub Releases.
var releaseURLBase = "https://github.com/perplexityai/bumblebee/releases/download"

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	prefix := fs.String("prefix", defaultPrefix(), "install dir (overrides $BUMBLEBEE_PREFIX, default ~/.local/bin)")
	uninstall := fs.Bool("uninstall", false, "remove the installed binary instead")
	if err := fs.Parse(args); err != nil {
		return err
	}

	target := filepath.Join(*prefix, "bumblebee")

	if *uninstall {
		return uninstallBinary(target)
	}
	return installBinary(*prefix, target)
}

func defaultPrefix() string {
	if p := os.Getenv("BUMBLEBEE_PREFIX"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func uninstallBinary(target string) error {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		fmt.Printf("Not installed at %s (nothing to do)\n", target)
		return nil
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("remove %s: %w", target, err)
	}
	fmt.Printf("Removed %s\n", target)
	return nil
}

func installBinary(prefix, target string) error {
	osArch, err := resolveOSArch()
	if err != nil {
		return err
	}
	expectedSHA, ok := pinnedDigests[osArch]
	if !ok {
		return fmt.Errorf("no pinned checksum for %s", osArch)
	}

	asset := fmt.Sprintf("bumblebee_%s_%s.tar.gz", strings.TrimPrefix(BumblebeeVersion, "v"), osArch)
	url := fmt.Sprintf("%s/%s/%s", releaseURLBase, BumblebeeVersion, asset)

	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", prefix, err)
	}

	tmpdir, err := os.MkdirTemp("", "bumblebee-install-")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpdir) }()

	fmt.Printf("→ Downloading %s (%s)...\n", asset, BumblebeeVersion)
	tarPath := filepath.Join(tmpdir, asset)
	if err := download(url, tarPath); err != nil {
		return err
	}

	fmt.Println("→ Verifying SHA-256...")
	actualSHA, err := sha256File(tarPath)
	if err != nil {
		return err
	}
	if actualSHA != expectedSHA {
		return fmt.Errorf("CHECKSUM MISMATCH for %s\n  expected: %s\n  actual:   %s\nRefusing to install. Investigate before retrying",
			asset, expectedSHA, actualSHA)
	}
	fmt.Printf("  ok (%s)\n", actualSHA)

	fmt.Println("→ Extracting...")
	if err := extractTarball(tarPath, tmpdir); err != nil {
		return err
	}

	src := filepath.Join(tmpdir, "bumblebee")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("expected ./bumblebee in %s, not found: %w", asset, err)
	}
	if err := installFile(src, target, 0o755); err != nil {
		return err
	}

	fmt.Println("→ Installed:")
	printBinaryVersion(target)
	fmt.Printf("Bumblebee %s → %s\n\n", BumblebeeVersion, target)
	fmt.Printf("Next: add %s to PATH if it isn't already, then run `make bumblebee-scan`.\n", prefix)
	return nil
}

func resolveOSArch() (string, error) {
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	switch runtime.GOOS {
	case "darwin", "linux":
		return runtime.GOOS + "_" + arch, nil
	default:
		return "", fmt.Errorf("unsupported OS: %s (Bumblebee supports macOS and Linux only)", runtime.GOOS)
	}
}

func download(url, dst string) error {
	resp, err := http.Get(url) //nolint:gosec,noctx // pinned upstream release, no user-supplied URL
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractTarball shells out to tar(1) — tar handles the gzip + sparse-file
// edge cases that the stdlib leaves to the caller, and is universally present
// on macOS and Linux (the only supported OSes per resolveOSArch).
func extractTarball(tarPath, destDir string) error {
	cmd := exec.Command("tar", "-xzf", tarPath, "-C", destDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar -xzf %s: %w", tarPath, err)
	}
	return nil
}

// installFile copies src→dst with the given mode, replacing dst atomically.
func installFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", filepath.Dir(dst), err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("copy to %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		cleanup()
		return fmt.Errorf("rename %s → %s: %w", tmpName, dst, err)
	}
	return nil
}

// printBinaryVersion tries `--version` then `version`, falling back to a path
// note. Matches the bash precursor's tolerance for either flag style.
func printBinaryVersion(bin string) {
	for _, args := range [][]string{{"--version"}, {"version"}} {
		cmd := exec.Command(bin, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return
		}
	}
	fmt.Printf("  (binary at %s)\n", bin)
}
