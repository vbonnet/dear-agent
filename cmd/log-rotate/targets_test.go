package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The agm log directory was the single largest unbounded log writer on the
// host: ~/.agm/logs held 197 MB across 36 files with no rotation at all
// (cleanup.jsonl 71 MB, procwatch-metrics.jsonl 51 MB, procwatch.log 22 MB,
// audit.jsonl 18 MB, procwatch-alerts.jsonl 14 MB, gc.jsonl 13 MB). None of
// its appenders call into internal/logrotate, and defaultTargets did not name
// the directory, so log-rotate could never have touched it even when run.
func TestDefaultTargetsCoverAgmLogs(t *testing.T) {
	got := defaultTargets()
	joined := strings.Join(got, "\n")

	for _, want := range []string{
		filepath.Join(".agm", "logs", "*.jsonl"),
		filepath.Join(".agm", "logs", "*.log"),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("defaultTargets missing %q\ngot:\n%s", want, joined)
		}
	}
}

// gc.jsonl is what the reclaim health check reads to decide whether the
// collector reclaimed anything. Rotating it must not be the reason that
// history disappears, so it has to be covered by the same glob rather than
// left as a special case someone forgets.
func TestDefaultTargetsCoverGCLog(t *testing.T) {
	joined := strings.Join(defaultTargets(), "\n")
	if !strings.Contains(joined, filepath.Join(".agm", "logs", "*.jsonl")) {
		t.Error("gc.jsonl is not covered by any default target glob")
	}
}

func TestDefaultTargetsKeepExistingTrail(t *testing.T) {
	joined := strings.Join(defaultTargets(), "\n")
	if !strings.Contains(joined, filepath.Join(".agm", "vroom", "trail.jsonl")) {
		t.Error("regression: the vroom decision trail is no longer a default target")
	}
}
