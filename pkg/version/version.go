// Package version provides build-time version stamping for CLI binaries.
// Variables are injected via -ldflags at build time:
//
//	-X github.com/vbonnet/dear-agent/pkg/version.Version=...
//	-X github.com/vbonnet/dear-agent/pkg/version.GitCommit=...
//	-X github.com/vbonnet/dear-agent/pkg/version.BuildDate=...
//	-X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=...
//
// When ldflags are absent (plain `go install`), PopulateFromBuildInfo fills
// GitCommit and BuildDate from the VCS metadata Go embeds automatically.
package version

import (
	"runtime/debug"
	"strings"
	"sync"
)

var (
	// Version is the semantic version string (e.g. "2.0.0-dev").
	Version = "dev"
	// GitCommit is the short git commit hash the binary was built from.
	GitCommit = "unknown"
	// BuildDate is the UTC timestamp the binary was built.
	BuildDate = "unknown"
	// BuiltBy identifies who/what produced the binary ("makefile", "go install", etc.).
	BuiltBy = "unknown"

	mu sync.Mutex
)

// String returns a compact one-line version summary.
func String() string {
	return Version + " (" + GitCommit + ") built " + BuildDate
}

// PopulateFromBuildInfo fills GitCommit, BuildDate, and BuiltBy from the VCS
// metadata that `go build` embeds automatically when the module is inside a
// git repository. It is a no-op when the vars have already been set by ldflags.
func PopulateFromBuildInfo() {
	mu.Lock()
	defer mu.Unlock()
	if GitCommit != "unknown" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			GitCommit = s.Value
			if len(GitCommit) > 12 {
				GitCommit = GitCommit[:12]
			}
		case "vcs.time":
			BuildDate = s.Value
		case "vcs.modified":
			if s.Value == "true" && GitCommit != "unknown" {
				GitCommit += "-dirty"
			}
		}
	}
	if GitCommit != "unknown" && BuiltBy == "unknown" {
		BuiltBy = "go install"
	}
}

// Short returns GitCommit trimmed to 12 characters (without the -dirty suffix).
func Short() string {
	return strings.TrimSuffix(GitCommit, "-dirty")
}

// RevisionIdentity returns a bounded revision suitable for exact companion
// comparisons while preserving dirty provenance. Short intentionally drops
// the marker for ancestry checks; process-boundary compatibility must not.
func RevisionIdentity(commit string) string {
	dirty := strings.HasSuffix(commit, "-dirty")
	commit = strings.TrimSuffix(commit, "-dirty")
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if dirty {
		commit += "-dirty"
	}
	return commit
}
