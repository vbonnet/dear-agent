package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A completion that is not a healthy heartbeat proves the reaper ran and did
// not do its job. The producer records that as structured counts with no
// `error` field, so before this the alarm reported only "no proof of life"
// once the last healthy heartbeat went stale — a responder could not tell a
// dead schedule from one that runs and fails every time (DW-19).
func TestScanGCLog_RejectedCompletionSurfacesItsFailureCounts(t *testing.T) {
	now := time.Now()
	stamp := now.Add(-1 * time.Minute).Format(time.RFC3339Nano)

	tests := []struct {
		name   string
		record string
		want   string
	}{
		{
			name:   "deletion errors",
			record: `{"timestamp":"` + stamp + `","operation":"sandbox_gc_completed","errors":3}`,
			want:   "3 deletion error(s)",
		},
		{
			name:   "safety probe failures",
			record: `{"timestamp":"` + stamp + `","operation":"sandbox_gc_completed","probe_failures":2}`,
			want:   "2 safety-probe failure(s)",
		},
		{
			name:   "dry run",
			record: `{"timestamp":"` + stamp + `","operation":"sandbox_gc_completed","dry_run":true}`,
			want:   "dry run reclaimed nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanGCLog(writeGCLog(t, tt.record), now, defaultGCMaxAge)
			if err != nil {
				t.Fatalf("scanGCLog: %v", err)
			}
			if !got.HasCompletion {
				t.Error("a rejected completion is still a completion record")
			}
			if !got.LastSuccess.IsZero() {
				t.Errorf("a rejected completion must not count as a heartbeat, got %v", got.LastSuccess)
			}
			if !strings.Contains(got.LastError, tt.want) {
				t.Errorf("LastError = %q, want it to mention %q", got.LastError, tt.want)
			}
			if got.LastErrorAt.IsZero() {
				t.Error("LastErrorAt must be set alongside LastError")
			}
		})
	}
}

func TestScanGCLog_HealthyCompletionRecordsNoFailure(t *testing.T) {
	now := time.Now()
	got, err := scanGCLog(writeGCLog(t, gcCompletedAt(now.Add(-1*time.Minute))), now, defaultGCMaxAge)
	if err != nil {
		t.Fatalf("scanGCLog: %v", err)
	}
	if got.LastError != "" {
		t.Errorf("a healthy heartbeat must record no failure, got %q", got.LastError)
	}
	if got.LastSuccess.IsZero() {
		t.Error("a healthy heartbeat must set LastSuccess")
	}
}

// An explicit `error` record must still win over a synthesized rejection
// reason when it is newer, so the more specific message is what responders see.
func TestScanGCLog_NewerExplicitErrorWinsOverRejectedCompletion(t *testing.T) {
	now := time.Now()
	older := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	newer := now.Add(-1 * time.Minute).Format(time.RFC3339Nano)
	got, err := scanGCLog(writeGCLog(t,
		`{"timestamp":"`+older+`","operation":"sandbox_gc_completed","errors":1}`,
		`{"timestamp":"`+newer+`","operation":"sandbox_gc_error","error":"mount table unreadable"}`,
	), now, defaultGCMaxAge)
	if err != nil {
		t.Fatalf("scanGCLog: %v", err)
	}
	if !strings.Contains(got.LastError, "mount table unreadable") {
		t.Errorf("LastError = %q, want the newer explicit error", got.LastError)
	}
}

// gc.jsonl is append-only and shared with the session GC. Reading from byte
// zero every tick makes the work grow with total history, and launchd does not
// overlap instances of the same job, so a slow enough scan starves every later
// disk sample.
func TestScanGCLog_ReadsOnlyABoundedTail(t *testing.T) {
	now := time.Now()
	p := filepath.Join(t.TempDir(), "gc.jsonl")

	var b strings.Builder
	// A stale heartbeat far outside the tail window must not be observed.
	b.WriteString(`{"timestamp":"` + now.Add(-48*time.Hour).Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_completed","reason":"ancient"}` + "\n")
	filler := `{"operation":"gc_archive","reason":"` + strings.Repeat("y", 4096) + `"}` + "\n"
	for written := int64(0); written < maxGCLogScanBytes+(2*1024*1024); written += int64(len(filler)) {
		b.WriteString(filler)
	}
	b.WriteString(gcCompletedAt(now.Add(-2*time.Minute)) + "\n")

	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat gc log: %v", err)
	}
	if info.Size() <= maxGCLogScanBytes {
		t.Fatalf("fixture must exceed the scan bound, got %d bytes", info.Size())
	}

	got, err := scanGCLog(p, now, defaultGCMaxAge)
	if err != nil {
		t.Fatalf("scanGCLog: %v", err)
	}
	if got.LastSuccess.IsZero() {
		t.Fatal("the heartbeat at the end of the log must be observed")
	}
	if got.LastSuccess.Before(now.Add(-1 * time.Hour)) {
		t.Errorf("observed heartbeat %v predates the tail window — the whole file was read", got.LastSuccess)
	}
}

// A file at or below the bound is read in full, so bounding never loses the
// only heartbeat on an ordinary host.
func TestScanGCLog_SmallLogIsReadInFull(t *testing.T) {
	now := time.Now()
	got, err := scanGCLog(writeGCLog(t, gcCompletedAt(now.Add(-3*time.Minute))), now, defaultGCMaxAge)
	if err != nil {
		t.Fatalf("scanGCLog: %v", err)
	}
	if got.LastSuccess.IsZero() {
		t.Error("a small log must still yield its heartbeat")
	}
}

// The record straddling the cut is skipped rather than parsed, so a truncated
// line is never mistaken for a malformed one.
func TestScanGCLog_SkipsRecordStraddlingTheCut(t *testing.T) {
	now := time.Now()
	p := filepath.Join(t.TempDir(), "gc.jsonl")

	var b strings.Builder
	filler := `{"operation":"gc_archive","reason":"` + strings.Repeat("z", 8192) + `"}` + "\n"
	for written := int64(0); written < maxGCLogScanBytes+4096; written += int64(len(filler)) {
		b.WriteString(filler)
	}
	for i := range 3 {
		b.WriteString(fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","reason":"tail-%d"}`,
			now.Add(-time.Duration(3-i)*time.Minute).Format(time.RFC3339Nano), i) + "\n")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}

	got, err := scanGCLog(p, now, defaultGCMaxAge)
	if err != nil {
		t.Fatalf("scanGCLog on a straddled cut: %v", err)
	}
	if got.LastSuccess.IsZero() {
		t.Error("records after the cut must still be parsed")
	}
}
