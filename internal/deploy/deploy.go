package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Action is what a deploy did to one artifact.
type Action string

const (
	// ActionInstalled means the artifact was written where nothing existed.
	ActionInstalled Action = "installed"
	// ActionUpdated means an existing, differing deployed file was replaced.
	ActionUpdated Action = "updated"
	// ActionUnchanged means the deployed file already matched the source; sync
	// skipped it. (install always rewrites, so it never reports this.)
	ActionUnchanged Action = "unchanged"
	// ActionSkipped means an optional artifact's source was not present.
	ActionSkipped Action = "skipped"
)

// Result is the outcome of deploying one artifact.
type Result struct {
	Name         string
	DeployedPath string
	Action       Action
	// SHA256 is the hash of the bytes now installed (empty on skip).
	SHA256 string
}

// Options configures a deploy run.
type Options struct {
	// RepoRoot is the directory source paths resolve against. Required.
	RepoRoot string
	// Home overrides the home directory used to expand ~ in deployed paths.
	// Empty uses os.UserHomeDir.
	Home string
	// Force rewrites an artifact even when the deployed copy already matches the
	// source. Sync leaves Force false (idempotent reconcile); install sets it.
	Force bool
}

func resolveHome(opts Options) (string, error) {
	if opts.Home != "" {
		return opts.Home, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("deploy: cannot resolve home dir: %w", err)
	}
	return h, nil
}

// Deploy installs one artifact through the principle-9 atomic sequence:
//
//	stage    — render the source and write it to a temp file in the *same*
//	           directory as the target, so the final step is a same-filesystem
//	           rename (atomic on POSIX). The mode is set on the staged file.
//	verify   — read the staged file back and confirm its hash equals the hash
//	           of the bytes we intended to write. This catches a short write or
//	           a filesystem that silently mangled the content before we make it
//	           live.
//	activate — os.Rename the verified staged file over the target. Rename is
//	           atomic: a reader sees either the old file or the new one, never a
//	           truncated mix.
//
// On any failure before activate the staged temp file is removed and the
// previously-installed artifact is left exactly as it was. There is no bypass
// flag: the only way to deploy is through this sequence (ADR-031, principle 9).
func Deploy(a Artifact, opts Options) (Result, error) {
	if opts.RepoRoot == "" {
		return Result{}, fmt.Errorf("deploy: RepoRoot is required")
	}
	home, err := resolveHome(opts)
	if err != nil {
		return Result{}, err
	}
	deployedPath := a.DeployedPath(home)
	res := Result{Name: a.Name, DeployedPath: deployedPath}

	// Binary artifacts are status-only: dear-deploy never copies a binary into
	// place. They are built and installed out of band by their Remediation
	// command (e.g. `make install-mergeloop`), so sync/install skip them rather
	// than write the package-path "source" as if it were file content.
	if a.IsBinary() {
		res.Action = ActionSkipped
		return res, nil
	}

	mode, err := a.FileMode()
	if err != nil {
		return res, err
	}

	content, err := a.Render(opts.RepoRoot, home)
	if err != nil {
		if a.Optional && errors.Is(err, os.ErrNotExist) {
			res.Action = ActionSkipped
			return res, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			hint := ""
			if a.Remediation != "" {
				hint = fmt.Sprintf(" — produce it with: %s", a.Remediation)
			}
			return res, fmt.Errorf("source not found for %q: %s%s", a.Name, filepath.Join(opts.RepoRoot, a.Source), hint)
		}
		return res, fmt.Errorf("rendering %q: %w", a.Name, err)
	}
	wantHash := sha256hex(content)
	res.SHA256 = wantHash

	// Decide install vs update vs unchanged from the current host state.
	existing, statErr := os.ReadFile(deployedPath)
	switch {
	case statErr == nil && a.AbsentOnly && !opts.Force:
		// Absent-only: already deployed, preserve operator edits unconditionally.
		res.Action = ActionUnchanged
		return res, nil
	case statErr == nil && sha256hex(existing) == wantHash && !opts.Force:
		res.Action = ActionUnchanged
		return res, nil
	case statErr == nil:
		res.Action = ActionUpdated
	case errors.Is(statErr, os.ErrNotExist):
		res.Action = ActionInstalled
	default:
		return res, fmt.Errorf("reading deployed %q: %w", deployedPath, statErr)
	}

	if err := atomicWrite(deployedPath, content, mode, wantHash); err != nil {
		return res, fmt.Errorf("deploying %q: %w", a.Name, err)
	}
	return res, nil
}

// atomicWrite performs the stage → verify → activate write of content to path.
func atomicWrite(path string, content []byte, mode os.FileMode, wantHash string) error {
	dir := filepath.Dir(path)
	// Parent dir is created up front: a first-time install of a launchd plist or
	// a hook lands in a directory that may not exist yet on a fresh machine.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating target dir %s: %w", dir, err)
	}

	// --- stage -------------------------------------------------------------
	// CreateTemp in the *target* directory guarantees the later rename stays on
	// one filesystem. The "." prefix and unique suffix keep the partial file out
	// of the way of any directory scan (e.g. launchd loading *.plist).
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".staging-*")
	if err != nil {
		return fmt.Errorf("staging: %w", err)
	}
	tmpPath := tmp.Name()
	// Clean up the staged file on any early return; after a successful rename
	// the path no longer exists, so the deferred remove is a harmless no-op.
	staged := true
	defer func() {
		if staged {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("staging write: %w", err)
	}
	// fsync before rename so the bytes are durable on disk, not just in the page
	// cache, when we make the file live.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("staging sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("staging close: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("staging chmod: %w", err)
	}

	// --- verify ------------------------------------------------------------
	back, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("verify read-back: %w", err)
	}
	if got := sha256hex(back); got != wantHash {
		return fmt.Errorf("verify failed: staged hash %s != intended %s (refusing to activate)", got, wantHash)
	}

	// --- activate ----------------------------------------------------------
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("activate (rename): %w", err)
	}
	staged = false
	return nil
}

// State is one artifact's current status relative to the manifest, for `status`.
type State string

const (
	// StateOK means the deployed file matches the rendered source.
	StateOK State = "ok"
	// StateDrift means the deployed file exists but differs from the source.
	StateDrift State = "drift"
	// StateMissing means nothing is deployed at the target path.
	StateMissing State = "missing"
	// StateSourceMissing means the source of truth is absent (e.g. unbuilt).
	StateSourceMissing State = "source-missing"
	// StateError means the artifact could not be evaluated.
	StateError State = "error"
)

// StatusResult is the outcome of inspecting one artifact without changing it.
type StatusResult struct {
	Name         string `json:"name"`
	Kind         Kind   `json:"kind,omitempty"`
	DeployedPath string `json:"deployed_path"`
	State        State  `json:"state"`
	Optional     bool   `json:"optional,omitempty"`
	Remediation  string `json:"remediation,omitempty"`
	Error        string `json:"error,omitempty"`
	// DeployedVersion and SourceVersion are the short commit revisions compared
	// for a binary artifact (empty for file artifacts): the vcs.revision stamped
	// into the installed binary vs the repo HEAD it should match.
	DeployedVersion string `json:"deployed_version,omitempty"`
	SourceVersion   string `json:"source_version,omitempty"`
	// Detail sub-classifies a binary drift ("stale ..." vs "divergent ...").
	Detail string `json:"detail,omitempty"`
}

// Status reports, without writing anything, how each artifact's deployed copy
// compares to its source — the manifest-scoped twin of drift-check. File
// artifacts are compared by content hash; binary artifacts by embedded
// vcs.revision against repo HEAD (see binaryStatus).
func Status(a Artifact, opts Options) StatusResult {
	res := StatusResult{Name: a.Name, Kind: a.Kind, Optional: a.Optional, Remediation: a.Remediation}
	home, err := resolveHome(opts)
	if err != nil {
		res.State = StateError
		res.Error = err.Error()
		return res
	}
	if a.IsBinary() {
		return binaryStatus(a, opts, home)
	}
	res.DeployedPath = a.DeployedPath(home)

	content, err := a.Render(opts.RepoRoot, home)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			res.State = StateSourceMissing
			return res
		}
		res.State = StateError
		res.Error = err.Error()
		return res
	}

	deployed, err := os.ReadFile(res.DeployedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			res.State = StateMissing
			return res
		}
		res.State = StateError
		res.Error = err.Error()
		return res
	}
	if sha256hex(deployed) == sha256hex(content) {
		res.State = StateOK
	} else {
		res.State = StateDrift
	}
	return res
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
