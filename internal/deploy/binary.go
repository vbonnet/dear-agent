package deploy

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// gitTimeout bounds every git invocation so a wedged git can never hang a
// status run (which the Overseer calls on every tick).
const gitTimeout = 2 * time.Second

// shortRevLen is how many hex characters of a commit sha to show in status
// output — long enough to be unambiguous, short enough to align in a table.
const shortRevLen = 12

// readBinaryVersion is the indirection through which binaryStatus reads a
// deployed binary's embedded revision. It is a package var so tests can supply
// synthetic revisions without building real binaries (whose VCS stamping is
// environment-dependent); production code always uses binaryVersion.
var readBinaryVersion = binaryVersion

// binaryVersion reads the embedded vcs.revision (and dirty flag) from a Go
// binary at path — the same build metadata `go version -m` prints. A binary
// built with -buildvcs=false (or otherwise without a stamp) returns an empty
// revision and no error, which the caller treats as unverifiable provenance.
func binaryVersion(path string) (revision string, modified bool, err error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, modified, nil
}

// binaryStatus compares a deployed Go binary's embedded vcs.revision against the
// repo HEAD it should have been built from. It never reads the source path as a
// file: a binary's source of truth is the current commit, not bytes in the tree.
//
// States:
//
//	StateOK      deployed commit == repo HEAD (current)
//	StateDrift   deployed commit is an ancestor of HEAD (stale, Detail "stale"),
//	             or off-trunk / not in the repo (Detail "divergent")
//	StateMissing nothing installed at the deployed path (NOT DEPLOYED)
//	StateError   the binary carries no vcs stamp, or the source revision /
//	             build info could not be read
func binaryStatus(a Artifact, opts Options, home string) StatusResult {
	res := StatusResult{
		Name:         a.Name,
		Kind:         KindBinary,
		Optional:     a.Optional,
		Remediation:  a.Remediation,
		DeployedPath: a.DeployedPath(home),
	}

	// Source of truth: the commit the binary should have been built from.
	head, err := gitRev(opts.RepoRoot, "HEAD")
	if err != nil {
		res.State = StateError
		res.Error = fmt.Sprintf("cannot resolve source revision in %s: %v", opts.RepoRoot, err)
		return res
	}
	res.SourceVersion = shortRev(head)

	// Deployed copy: the vcs.revision baked into the installed binary.
	rev, modified, err := readBinaryVersion(res.DeployedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			res.State = StateMissing
			return res
		}
		res.State = StateError
		res.Error = fmt.Sprintf("reading build info from %s: %v", res.DeployedPath, err)
		return res
	}
	if rev == "" {
		res.DeployedVersion = "unstamped"
		res.State = StateError
		res.Error = "deployed binary has no embedded vcs.revision — rebuild with VCS stamping (go build -buildvcs=true)"
		return res
	}
	res.DeployedVersion = shortRev(rev)
	if modified {
		res.DeployedVersion += "-dirty"
	}

	switch {
	case sameRev(rev, head):
		res.State = StateOK
	case gitIsAncestor(opts.RepoRoot, rev, head):
		// The deployed commit is in the source history but behind HEAD: a fix
		// landed in the tree that the installed binary predates.
		res.State = StateDrift
		res.Detail = "stale (deployed older than source)"
	default:
		// Off-trunk, rolled back, or built from a commit not in this checkout.
		res.State = StateDrift
		res.Detail = "divergent (deployed commit not an ancestor of source HEAD)"
	}
	return res
}

// sameRev reports whether two commit shas refer to the same commit, tolerating
// one being an abbreviation of the other (HEAD is full-length; an embedded
// stamp is usually full but may be abbreviated).
func sameRev(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// shortRev truncates a commit sha to shortRevLen characters for display.
func shortRev(sha string) string {
	if len(sha) > shortRevLen {
		return sha[:shortRevLen]
	}
	return sha
}

// gitRev resolves a ref to its full commit sha in repoPath.
func gitRev(repoPath, ref string) (string, error) {
	out, err := runGit(repoPath, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// gitIsAncestor reports whether commit is an ancestor of ref in repoPath. A
// missing commit object or any git error answers false (the binary cannot be
// proven current, so it is treated as drift, not silently passed).
func gitIsAncestor(repoPath, commit, ref string) bool {
	_, err := runGit(repoPath, "merge-base", "--is-ancestor", commit, ref)
	return err == nil
}

// runGit runs a time-bounded git command in repoPath and returns its stdout.
func runGit(repoPath string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	full := append([]string{"-C", repoPath}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	return string(out), err
}
