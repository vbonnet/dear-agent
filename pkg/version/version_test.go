package version_test

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/version"
)

func TestStringFormat(t *testing.T) {
	s := version.String()
	if !strings.Contains(s, version.Version) {
		t.Errorf("String() %q does not contain Version %q", s, version.Version)
	}
	if !strings.Contains(s, version.GitCommit) {
		t.Errorf("String() %q does not contain GitCommit %q", s, version.GitCommit)
	}
	if !strings.Contains(s, version.BuildDate) {
		t.Errorf("String() %q does not contain BuildDate %q", s, version.BuildDate)
	}
}

func TestDefaultValues(t *testing.T) {
	// After a plain go test (no ldflags), defaults are the zero values.
	// We only assert they are non-empty strings.
	if version.Version == "" {
		t.Error("Version must not be empty")
	}
	if version.GitCommit == "" {
		t.Error("GitCommit must not be empty")
	}
	if version.BuildDate == "" {
		t.Error("BuildDate must not be empty")
	}
	if version.BuiltBy == "" {
		t.Error("BuiltBy must not be empty")
	}
}

func TestShortStripsTrailingDirty(t *testing.T) {
	orig := version.GitCommit
	t.Cleanup(func() { version.GitCommit = orig })

	version.GitCommit = "abc123-dirty"
	if got := version.Short(); got != "abc123" {
		t.Errorf("Short() = %q, want %q", got, "abc123")
	}

	version.GitCommit = "abc123"
	if got := version.Short(); got != "abc123" {
		t.Errorf("Short() = %q, want %q", got, "abc123")
	}
}

func TestRevisionIdentityPreservesDirtyProvenance(t *testing.T) {
	tests := []struct {
		commit string
		want   string
	}{
		{commit: "0123456789abcdef", want: "0123456789ab"},
		{commit: "0123456789abcdef-dirty", want: "0123456789ab-dirty"},
		{commit: "0123456789ab-dirty", want: "0123456789ab-dirty"},
		{commit: "unknown", want: "unknown"},
	}
	for _, tc := range tests {
		if got := version.RevisionIdentity(tc.commit); got != tc.want {
			t.Errorf("RevisionIdentity(%q) = %q, want %q", tc.commit, got, tc.want)
		}
	}
}

func TestIsStaleUnknownCommit(t *testing.T) {
	orig := version.GitCommit
	t.Cleanup(func() { version.GitCommit = orig })

	version.GitCommit = "unknown"
	stale, err := version.IsStale("/nonexistent")
	if err != nil {
		t.Fatalf("IsStale with unknown commit: unexpected error: %v", err)
	}
	if stale {
		t.Error("IsStale with unknown commit should return false (fail open)")
	}
}

func TestPopulateFromBuildInfoNoOp(t *testing.T) {
	orig := version.GitCommit
	t.Cleanup(func() { version.GitCommit = orig })

	// When GitCommit is already set, PopulateFromBuildInfo must not overwrite it.
	version.GitCommit = "preset-commit"
	version.PopulateFromBuildInfo()
	if version.GitCommit != "preset-commit" {
		t.Errorf("PopulateFromBuildInfo overwrote a pre-set GitCommit: got %q", version.GitCommit)
	}
}
