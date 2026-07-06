package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// buildTimeout bounds the `go build` invoked by AtomicInstall so a wedged build
// can never hang a deploy (the post-merge hook runs it synchronously).
const buildTimeout = 5 * time.Minute

// buildBinary compiles the Go package pkg (relative to repoRoot) to outPath with
// VCS stamping required, so AtomicInstall's ancestry gate can read the built
// revision. It is a package var so tests substitute a stub without a toolchain.
var buildBinary = goBuild

func goBuild(repoRoot, pkg, outPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	// -buildvcs=true forces the stamp: a checkout that cannot be VCS-stamped
	// fails the build here rather than producing an unprovable binary.
	cmd := exec.CommandContext(ctx, "go", "build", "-buildvcs=true", "-o", outPath, pkg)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("go build %s timed out after %s: %w", pkg, buildTimeout, ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("go build %s: %w\n%s", pkg, err, out)
	}
	return nil
}

// InstallResult reports a successful atomic install.
type InstallResult struct {
	Name      string `json:"name"`
	Package   string `json:"package"`
	Target    string `json:"target"`
	Revision  string `json:"revision"`   // vcs.revision baked into the new binary
	SourceRef string `json:"source_ref"` // the ref gated against + its short sha
}

// AtomicInstall is the crash-safe, stale-proof replacement for `go install`.
//
// The post-merge deploy hook previously ran `GOBIN=~/go/bin go install ./pkg`,
// which truncates and rewrites the target binary IN PLACE. On macOS, rewriting a
// running Mach-O's backing file invalidates the code pages mapped by every live
// instance, and the kernel SIGKILLs them with "Code Signature Invalid" (the
// 2026-07-06 crash loop). That path also silently reinstalled a ~3-week-old
// binary whenever the build env was wrong, because a bad build was never gated.
//
// AtomicInstall fixes both failure modes:
//
//  1. Resolve the intended source revision (sourceRef, e.g. "origin/main"). A
//     ref that will not resolve is a broken deploy env — fail loud, install
//     nothing.
//  2. Build to a temp file in the target's own directory (same filesystem, so
//     the final rename is atomic). A build failure fails loud and leaves the
//     live binary untouched — never half-written.
//  3. Ancestry gate (ce-mb9g): read the vcs.revision stamped into the FRESH
//     binary and require it to be sourceRef or an ancestor of it. An unstamped
//     or off-history build (the "reinstalled June-14 code" failure) is REFUSED.
//  4. os.Rename(temp, target): atomic replace. A running instance keeps its
//     original inode and mapping until it exits, so it is never codesign-killed.
func AtomicInstall(pkg, target, sourceRef string, opts Options) (InstallResult, error) {
	res := InstallResult{Name: filepath.Base(target), Package: pkg, Target: target}
	if opts.RepoRoot == "" {
		return res, fmt.Errorf("AtomicInstall: RepoRoot is required")
	}
	if pkg == "" || target == "" {
		return res, fmt.Errorf("AtomicInstall: pkg and target are required")
	}
	if sourceRef == "" {
		sourceRef = "origin/main"
	}

	// 1. Intended source revision — a deploy env that cannot resolve it is
	//    broken; fail loud rather than guess.
	srcSha, err := gitRev(opts.RepoRoot, sourceRef)
	if err != nil {
		return res, fmt.Errorf("deploy env broken: cannot resolve %s in %s: %w", sourceRef, opts.RepoRoot, err)
	}
	res.SourceRef = fmt.Sprintf("%s@%s", sourceRef, shortRev(srcSha))

	// 2. Build to a temp file beside the target (same fs => atomic rename).
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, fmt.Errorf("ensure target dir %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(target)+".install-*")
	if err != nil {
		return res, fmt.Errorf("create temp binary in %s: %w", dir, err)
	}
	tmpPath := f.Name()
	f.Close()
	installed := false
	defer func() {
		if !installed {
			os.Remove(tmpPath) // never leave a temp binary behind on failure
		}
	}()

	// 3. Build to temp and gate the freshly built binary's revision.
	revLabel, err := buildAndVerify(pkg, tmpPath, srcSha, res.SourceRef, opts)
	if err != nil {
		return res, err
	}
	res.Revision = revLabel

	// 4. Atomic replace: rename over the target on the same filesystem. The old
	//    binary keeps its inode/mapping for any running instance until it exits.
	//nolint:gosec // an installed executable must be 0755; this is a binary, not data.
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return res, fmt.Errorf("chmod temp binary %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return res, fmt.Errorf("atomic rename %s -> %s: %w", tmpPath, target, err)
	}
	installed = true
	return res, nil
}

// buildAndVerify builds pkg to tmpPath, then enforces the ancestry gate: the
// freshly built binary must carry a vcs.revision that is srcSha or an ancestor
// of it. It returns the display label for the built revision, or an error that
// callers surface loudly (a bad build never reaches the atomic rename).
func buildAndVerify(pkg, tmpPath, srcSha, sourceRefLabel string, opts Options) (string, error) {
	if err := buildBinary(opts.RepoRoot, pkg, tmpPath); err != nil {
		return "", fmt.Errorf("refusing to install (build failed, live binary untouched): %w", err)
	}
	rev, modified, err := readBinaryVersion(tmpPath)
	if err != nil {
		return "", fmt.Errorf("read build info from freshly built %s: %w", pkg, err)
	}
	if rev == "" {
		return "", fmt.Errorf("freshly built %s carries no vcs.revision — build env broken (buildvcs off / not a git checkout); refusing to install stale", pkg)
	}
	if !sameRev(rev, srcSha) && !gitIsAncestor(opts.RepoRoot, rev, srcSha) {
		return "", fmt.Errorf("refusing to install: built revision %s is neither %s nor an ancestor of it — stale or off-history build (would ship the wrong code)", shortRev(rev), sourceRefLabel)
	}
	label := shortRev(rev)
	if modified {
		label += "-dirty"
	}
	return label, nil
}
