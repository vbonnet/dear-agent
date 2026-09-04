// Package deploy is the write-side counterpart to internal/drift: it deploys
// dear-agent's host artifacts (launchd plists, Claude Code hooks) from their
// source of truth in the repo to their installed location on the machine.
//
// Where internal/drift only *detects* the gap between source and host, this
// package *closes* it — and does so under the atomic-wrapper discipline of
// AGENTS.md principle 9: every artifact is written through a deterministic
// stage → verify → activate sequence that can never leave a half-written file
// in place. A failed deploy leaves the previously-installed artifact untouched.
//
// The manifest schema mirrors drift.Target (name / source / deployed / tokens /
// optional) so the two tools describe the same artifacts the same way; it adds
// only `mode` (the installed file's permission bits — 0755 for an executable
// hook, 0644 for a plist).
package deploy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind classifies how an artifact's deployed copy is compared to its source.
type Kind string

const (
	// KindFile is the default: the deployed copy is compared byte-for-byte (by
	// SHA-256) against the rendered source file. Used for launchd plists and
	// compiled Claude Code hooks — anything dear-deploy installs by copying bytes.
	KindFile Kind = "file"
	// KindBinary marks a Go binary whose source of truth is the repo's current
	// commit, not a file in the tree. Its deployed copy is compared by the
	// vcs.revision embedded at build time (what `go version -m` prints) against
	// the repo HEAD — drift means the installed binary is older than the source
	// it was built from. Binaries are status-only: dear-deploy never copies a
	// binary into place (they are built and installed out of band via their
	// Remediation command, e.g. `make install-mergeloop`).
	KindBinary Kind = "binary"
)

// Artifact is one deployable item: a source of truth in the repo and the place
// it is installed to on the host.
type Artifact struct {
	// Name is a short, stable label used to select the artifact on the command
	// line and in output. It must be unique within a manifest.
	Name string `yaml:"name"`
	// Kind selects how this artifact's deployed copy is compared to its source:
	// "file" (default — SHA-256 byte compare) or "binary" (embedded vcs.revision
	// vs repo HEAD). Empty means "file".
	Kind Kind `yaml:"kind,omitempty"`
	// Source is the path of the source of truth, relative to the repo root. For
	// a compiled hook this is the built binary under bin/; for a templated plist
	// it is the template file. For a binary artifact it is the package path
	// (e.g. cmd/mergeloop) — informational, used in output and not read.
	Source string `yaml:"source"`
	// Deployed is the installed path on the host. A leading ~ and any $ENV
	// references are expanded before use.
	Deployed string `yaml:"deployed"`
	// Mode is the octal permission string of the installed file (e.g. "0755").
	// Empty defaults to 0644. Hooks that are executed directly need "0755".
	Mode string `yaml:"mode,omitempty"`
	// Tokens maps placeholder strings in the source to their deployed values
	// (e.g. "__HOME__" -> "${HOME}"). They are substituted into the source
	// content before it is written, rendering a template into the form the host
	// actually runs. Values are $ENV-expanded. This matches drift.Target.Tokens
	// so the deployed file hashes equal to what drift-check expects.
	Tokens map[string]string `yaml:"tokens,omitempty"`
	// Optional marks an artifact whose source may legitimately be absent (e.g. a
	// binary that has not been built on this machine). A missing source for an
	// optional artifact is skipped rather than failing the run.
	Optional bool `yaml:"optional,omitempty"`
	// AbsentOnly marks a config artifact that should be seeded on first install
	// but never overwritten once the operator has customised it. Sync treats an
	// already-deployed copy as already-current (ActionUnchanged) regardless of
	// whether its content matches the source, so operator edits are preserved.
	// Status still reports drift when the file is absent. Use for files like
	// pulse registries where the repo holds the default and the host holds the
	// live operator config.
	AbsentOnly bool `yaml:"absent-only,omitempty"`
	// Remediation is the command that produces or redeploys this artifact. It is
	// surfaced when the source is missing so the operator knows the prerequisite
	// (e.g. "make build-write-guards").
	Remediation string `yaml:"remediation,omitempty"`
}

// Manifest is the set of artifacts dear-deploy knows how to deploy.
type Manifest struct {
	Artifacts []Artifact `yaml:"artifacts"`
}

// ParseManifest decodes a YAML manifest. It is strict about unknown fields so a
// typo fails loudly rather than silently dropping an artifact from the deploy
// set, and it validates that every artifact is fully specified and uniquely
// named — a duplicate name would make command-line selection ambiguous.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("parsing deploy manifest: %w", err)
	}
	if len(m.Artifacts) == 0 {
		return Manifest{}, fmt.Errorf("deploy manifest has no artifacts")
	}
	seen := make(map[string]bool, len(m.Artifacts))
	for i, a := range m.Artifacts {
		if a.Name == "" {
			return Manifest{}, fmt.Errorf("artifact %d has no name", i)
		}
		if seen[a.Name] {
			return Manifest{}, fmt.Errorf("duplicate artifact name %q", a.Name)
		}
		seen[a.Name] = true
		if a.Source == "" {
			return Manifest{}, fmt.Errorf("artifact %q has no source path", a.Name)
		}
		if a.Deployed == "" {
			return Manifest{}, fmt.Errorf("artifact %q has no deployed path", a.Name)
		}
		switch a.Kind {
		case "", KindFile, KindBinary:
			// valid
		default:
			return Manifest{}, fmt.Errorf("artifact %q has unknown kind %q (want \"file\" or \"binary\")", a.Name, a.Kind)
		}
		if _, err := a.FileMode(); err != nil {
			return Manifest{}, fmt.Errorf("artifact %q: %w", a.Name, err)
		}
	}
	return m, nil
}

// IsBinary reports whether this artifact is compared by embedded version stamp
// (a Go binary) rather than by byte content.
func (a Artifact) IsBinary() bool { return a.Kind == KindBinary }

// FileMode parses Mode into an os.FileMode, defaulting to 0644 when empty.
func (a Artifact) FileMode() (os.FileMode, error) {
	if a.Mode == "" {
		return 0o644, nil
	}
	v, err := strconv.ParseUint(a.Mode, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q (want octal, e.g. \"0755\"): %w", a.Mode, err)
	}
	return os.FileMode(v), nil
}

// Select returns the named artifacts in manifest order. With no names it
// returns every artifact. An unknown name is an error so a typo on the command
// line never silently deploys nothing.
func (m Manifest) Select(names []string) ([]Artifact, error) {
	if len(names) == 0 {
		return m.Artifacts, nil
	}
	byName := make(map[string]Artifact, len(m.Artifacts))
	for _, a := range m.Artifacts {
		byName[a.Name] = a
	}
	out := make([]Artifact, 0, len(names))
	for _, n := range names {
		a, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown artifact %q (run `dear-deploy list` to see names)", n)
		}
		out = append(out, a)
	}
	return out, nil
}

// DeployedPath returns the artifact's installed path with a leading ~ and any
// $ENV references expanded against home.
func (a Artifact) DeployedPath(home string) string {
	return expandPath(a.Deployed, home)
}

// Render reads the source of truth from repoRoot and applies the artifact's
// token substitutions, returning the exact bytes to write to the host. A
// missing source is reported with os.IsNotExist-detectable error so callers can
// distinguish "not built yet" from a real read failure.
func (a Artifact) Render(repoRoot, home string) ([]byte, error) {
	src := filepath.Join(repoRoot, a.Source)
	content, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	return applyTokens(content, a.Tokens, home), nil
}

// homeMapper resolves $HOME/${HOME} to the resolved home directory rather than
// the process environment, keeping token and path expansion consistent with the
// ~ handling and with an overridden home. It mirrors internal/drift's mapper so
// a rendered artifact hashes equal to what drift-check computes.
func homeMapper(home string) func(string) string {
	return func(k string) string {
		if k == "HOME" && home != "" {
			return home
		}
		return os.Getenv(k)
	}
}

// applyTokens substitutes each placeholder with its $ENV-expanded value.
func applyTokens(content []byte, tokens map[string]string, home string) []byte {
	if len(tokens) == 0 {
		return content
	}
	mapper := homeMapper(home)
	s := string(content)
	for placeholder, value := range tokens {
		s = strings.ReplaceAll(s, placeholder, os.Expand(value, mapper))
	}
	return []byte(s)
}

// expandPath turns a configured deployed path into an absolute filesystem path:
// a leading ~ becomes home, then $ENV references are expanded ($HOME resolving
// to the resolved home, not the process environment).
func expandPath(p, home string) string {
	if p == "~" {
		p = home
	} else if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	return os.Expand(p, homeMapper(home))
}
