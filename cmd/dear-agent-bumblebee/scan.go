package main

// scan.go — run Bumblebee with dear-agent defaults and write NDJSON to the
// per-user data dir.
//
// Output: $XDG_DATA_HOME/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson on Linux,
//
//	~/Library/Application Support/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson on macOS.
//
// Local-only, atomic write (temp + rename) — no network egress beyond what
// Bumblebee itself does (catalog fetch only, and only if one is passed).
//
// Catalog: if BUMBLEBEE_CATALOG points at a file, or if etc/bumblebee/catalog.json
// exists at the repo root, it's passed as --exposure-catalog. Otherwise the
// scan runs in inventory-only mode (still useful: diff-vs-yesterday reveals
// new MCP/IDE/browser-extension installs).

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	profile := fs.String("profile", "project", "Bumblebee scan profile")
	root := fs.String("root", "", "root directory to scan (defaults to Bumblebee's default)")
	output := fs.String("output", "", "NDJSON output path (defaults to per-user data dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	extra := fs.Args()

	bin, err := resolveBumblebeeBinary()
	if err != nil {
		return err
	}

	outPath := *output
	if outPath == "" {
		outPath, err = defaultOutputPath(time.Now().UTC())
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
	}

	catalog := discoverCatalog()

	cmdArgs := []string{"scan", "--profile", *profile}
	if *root != "" {
		cmdArgs = append(cmdArgs, "--root", *root)
	}
	if catalog != "" {
		cmdArgs = append(cmdArgs, "--exposure-catalog", catalog)
	}
	cmdArgs = append(cmdArgs, extra...)

	tmpOut, err := os.CreateTemp(filepath.Dir(outPath), filepath.Base(outPath)+".XXXXXX")
	if err != nil {
		return fmt.Errorf("mktemp adjacent to %s: %w", outPath, err)
	}
	tmpName := tmpOut.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	cmd := exec.Command(bin, cmdArgs...)
	cmd.Stdout = tmpOut
	cmd.Stderr = os.Stderr

	catalogStr := ""
	if catalog != "" {
		catalogStr = " catalog=" + catalog
	}
	fmt.Fprintf(os.Stderr, "[bumblebee-scan] profile=%s%s → %s\n", *profile, catalogStr, outPath)

	if err := cmd.Run(); err != nil {
		_ = tmpOut.Close()
		return fmt.Errorf("bumblebee scan: %w", err)
	}
	if err := tmpOut.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmpName, outPath, err)
	}

	records, _ := countLines(outPath)
	fmt.Fprintf(os.Stderr, "[bumblebee-scan] wrote %d records to %s\n", records, outPath)
	return nil
}

// resolveBumblebeeBinary picks $BUMBLEBEE_BIN → $PATH lookup → ~/.local/bin/bumblebee.
func resolveBumblebeeBinary() (string, error) {
	if env := os.Getenv("BUMBLEBEE_BIN"); env != "" {
		if info, err := os.Stat(env); err == nil && info.Mode()&0o111 != 0 {
			return env, nil
		}
	}
	if p, err := exec.LookPath("bumblebee"); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	fallback := filepath.Join(home, ".local", "bin", "bumblebee")
	if info, err := os.Stat(fallback); err == nil && info.Mode()&0o111 != 0 {
		return fallback, nil
	}
	return "", errors.New("bumblebee not found. Run `make bumblebee-install` first")
}

// defaultOutputPath returns the per-user, per-platform NDJSON path for `now`.
func defaultOutputPath(now time.Time) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve $HOME: %w", err)
	}
	return filepath.Join(outputDir(runtime.GOOS, home, os.Getenv("XDG_DATA_HOME")), now.Format("2006-01-02")+".ndjson"), nil
}

// outputDir resolves the per-platform NDJSON parent directory. Split out
// from defaultOutputPath so tests can exercise every branch (darwin /
// linux-with-XDG / linux-without-XDG / other) without depending on the
// dev machine's runtime.GOOS — the macOS dev loop never executed the
// XDG branch before this split, see the 2026-05-27 coverage audit.
func outputDir(goos, home, xdgDataHome string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "dear-agent", "bumblebee")
	case "linux":
		base := xdgDataHome
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "dear-agent", "bumblebee")
	default:
		return filepath.Join(home, ".dear-agent", "bumblebee")
	}
}

// discoverCatalog honors $BUMBLEBEE_CATALOG if it points at a real file,
// otherwise falls back to repo-root etc/bumblebee/catalog.json (best-effort —
// `git rev-parse` may fail when running under launchd outside a worktree).
func discoverCatalog() string {
	if env := os.Getenv("BUMBLEBEE_CATALOG"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(repoRoot, "etc", "bumblebee", "catalog.json")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// gitRepoRoot returns the repo root containing the current working directory,
// or an error if we aren't in a worktree.
func gitRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// countLines returns the number of newline-terminated records in path.
func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(data), "\n"), nil
}
