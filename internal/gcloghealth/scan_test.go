package gcloghealth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testNow() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }

func testRecord(at time.Time, operation, extra string) string {
	return fmt.Sprintf(`{"timestamp":%q,"operation":%q%s}`,
		at.Format(time.RFC3339Nano), operation, extra)
}

func testLog(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gc.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func testOptions(now time.Time) Options {
	return Options{Now: now, MaxAge: 6 * time.Hour, IgnoredSources: []string{WatchdogSource}}
}

func TestScanExcludesWatchdogRemediationAndUnrelatedErrors(t *testing.T) {
	now := testNow()
	old := now.Add(-8 * time.Hour)
	p := testLog(t,
		testRecord(old, CompletedOperation, ""),
		testRecord(now.Add(-3*time.Minute), CompletedOperation, `,"source":"disk-watchdog"`),
		testRecord(now.Add(-2*time.Minute), "sandbox_gc_reap", `,"source":"disk-watchdog"`),
		testRecord(now.Add(-time.Minute), "gc_archive_error", `,"error":"session failure"`),
		testRecord(now.Add(-30*time.Second), "sandbox_gc_error", `,"error":"mount table unreadable"`),
	)
	s, err := Scan(p, testOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback := s.Proof()
	if !proof.Equal(old) || fallback || !s.LastReap.IsZero() {
		t.Fatalf("proof=%v fallback=%v lastReap=%v, want only stale scheduled proof", proof, fallback, s.LastReap)
	}
	if s.LastError != "mount table unreadable" {
		t.Fatalf("LastError = %q, want sandbox error only", s.LastError)
	}
}

func TestScanRejectsFailureAndFutureRecords(t *testing.T) {
	now := testNow()
	future := now.Add(15 * time.Minute)
	for _, tc := range []struct {
		name, extra, want string
	}{
		{"dry run", `,"dry_run":true`, "dry run reclaimed nothing"},
		{"deletion errors", `,"errors":2`, "2 deletion error(s)"},
		{"probe failures", `,"probe_failures":3`, "3 safety-probe failure(s)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testLog(t,
				testRecord(now.Add(-time.Minute), CompletedOperation, tc.extra),
				testRecord(future, CompletedOperation, ""),
			)
			s, err := Scan(p, testOptions(now))
			if err != nil {
				t.Fatal(err)
			}
			proof, _ := s.Proof()
			if !proof.IsZero() || !s.HasCompletion || !s.LastFutureCompletionAt.Equal(future) {
				t.Fatalf("proof=%v completion=%v future=%v", proof, s.HasCompletion, s.LastFutureCompletionAt)
			}
			if !strings.Contains(s.LastError, tc.want) {
				t.Fatalf("LastError = %q, want %q", s.LastError, tc.want)
			}
		})
	}
}

func TestScanWidensToFindRecentProof(t *testing.T) {
	now := testNow()
	recent := now.Add(-5 * time.Minute)
	lines := []string{testRecord(recent, CompletedOperation, "")}
	for range 12 {
		lines = append(lines, testRecord(now.Add(-time.Minute), "gc_archive", `,"reason":"chatter"`))
	}
	p := testLog(t, lines...)
	opts := testOptions(now)
	opts.InitialBytes, opts.MaxBytes = 128, 4096
	s, err := Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := s.Proof()
	if !proof.Equal(recent) {
		t.Fatalf("proof = %v, want buried recent heartbeat %v", proof, recent)
	}
}

func TestScanMarksCappedRecentHistoryIndeterminate(t *testing.T) {
	now := testNow()
	var lines []string
	for range 12 {
		lines = append(lines, testRecord(now.Add(-time.Minute), "gc_archive", `,"reason":"chatter"`))
	}
	p := testLog(t, lines...)
	opts := testOptions(now)
	opts.InitialBytes, opts.MaxBytes = 128, 256
	s, err := Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := s.Proof()
	if !s.Truncated || !s.Indeterminate || !proof.IsZero() {
		t.Fatalf("summary = %+v, proof = %v, want indeterminate without proof", s, proof)
	}
}

func TestScanBackdatedTailCannotHideRecentCompletion(t *testing.T) {
	now := testNow()
	recent := now.Add(-5 * time.Minute)
	lines := []string{testRecord(recent, CompletedOperation, "")}
	for range 12 {
		lines = append(lines, testRecord(now.Add(-time.Minute), "gc_archive", `,"reason":"chatter"`))
	}
	lines = append(lines, testRecord(now.Add(-48*time.Hour), "gc_archive", `,"reason":"backdated"`))
	p := testLog(t, lines...)
	opts := testOptions(now)
	opts.InitialBytes, opts.MaxBytes = 128, 4096
	s, err := Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := s.Proof()
	if !proof.Equal(recent) || s.Indeterminate {
		t.Fatalf("summary = %+v, proof = %v, want hidden recent completion", s, proof)
	}

	opts.MaxBytes = 256
	s, err = Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, _ = s.Proof()
	if !s.Truncated || !s.Indeterminate || !proof.IsZero() {
		t.Fatalf("summary = %+v, proof = %v, want bounded uncertainty", s, proof)
	}
}

func TestScanCappedStaleCompletionIsNotDefinitiveLatest(t *testing.T) {
	now := testNow()
	stale := now.Add(-48 * time.Hour)
	lines := []string{testRecord(now.Add(-5*time.Minute), CompletedOperation, "")}
	for range 12 {
		lines = append(lines, testRecord(now.Add(-time.Minute), "gc_archive", `,"reason":"chatter"`))
	}
	lines = append(lines, testRecord(stale, CompletedOperation, ""))
	p := testLog(t, lines...)
	opts := testOptions(now)
	opts.InitialBytes, opts.MaxBytes = 128, 256
	s, err := Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := s.Proof()
	if !s.Truncated || !s.Indeterminate || !s.LastSuccess.Equal(stale) || !proof.IsZero() {
		t.Fatalf("summary = %+v, proof = %v, want stale observation but uncertain latest", s, proof)
	}
}

func TestScanLegacyReapRequiresWholeLog(t *testing.T) {
	now := testNow()
	reap := testRecord(now.Add(-time.Minute), "sandbox_gc_reap", "")
	var lines []string
	lines = append(lines, testRecord(now.Add(-8*time.Hour), CompletedOperation, `,"errors":1`))
	for range 12 {
		lines = append(lines, testRecord(now.Add(-time.Minute), "gc_archive", `,"reason":"chatter"`))
	}
	lines = append(lines, reap)
	p := testLog(t, lines...)
	opts := testOptions(now)
	opts.InitialBytes, opts.MaxBytes = 128, 4096
	s, err := Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback := s.Proof()
	if !proof.IsZero() || fallback || !s.HasCompletion {
		t.Fatalf("summary = %+v, proof = %v fallback=%v, want modern failed completion to disqualify reap", s, proof, fallback)
	}

	opts.MaxBytes = 256
	s, err = Scan(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback = s.Proof()
	if !s.Indeterminate || !proof.IsZero() || fallback {
		t.Fatalf("summary = %+v, proof = %v fallback=%v, want indeterminate at cap", s, proof, fallback)
	}

	legacy := testLog(t, reap)
	s, err = Scan(legacy, testOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback = s.Proof()
	if proof.IsZero() || !fallback {
		t.Fatalf("proof=%v fallback=%v, want complete legacy log accepted", proof, fallback)
	}
}

func TestScanHistoricalUntaggedWatchdogReapCannotBeLegacyProof(t *testing.T) {
	now := testNow()
	p := testLog(t,
		testRecord(now.Add(-2*time.Minute), "sandbox_gc_reap", ""),
		testRecord(now.Add(-time.Minute), CompletedOperation, `,"source":"disk-watchdog"`),
	)
	s, err := Scan(p, testOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback := s.Proof()
	if !proof.IsZero() || fallback || !s.HasCompletion {
		t.Fatalf("summary = %+v, proof = %v fallback=%v, want no scheduled proof", s, proof, fallback)
	}
}

func TestScanTaggedModernReapWithoutCompletionIsNotLegacyProof(t *testing.T) {
	now := testNow()
	p := testLog(t, testRecord(now.Add(-time.Minute), "sandbox_gc_reap", `,"source":"launchd"`))
	s, err := Scan(p, testOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback := s.Proof()
	if !proof.IsZero() || fallback || !s.LastReap.IsZero() {
		t.Fatalf("summary = %+v, proof = %v fallback=%v, want no whole-sweep proof", s, proof, fallback)
	}
}

func TestScanSkipsMalformedAndOversizedRecord(t *testing.T) {
	now := testNow()
	recent := now.Add(-time.Minute)
	p := testLog(t,
		"{not json}",
		`{"operation":"gc_archive","reason":"`+strings.Repeat("x", maxRecordBytes)+`"}`,
		testRecord(recent, CompletedOperation, ""),
	)
	s, err := Scan(p, testOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := s.Proof()
	if !proof.Equal(recent) {
		t.Fatalf("proof = %v, want post-oversize heartbeat %v", proof, recent)
	}
}

func TestScanProducerWireFixture(t *testing.T) {
	now := testNow()
	s, err := Scan("testdata/wire.jsonl", testOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	proof, fallback := s.Proof()
	if !proof.Equal(now.Add(-30*time.Minute)) || fallback || s.LastError != "mount table unreadable" {
		t.Fatalf("summary = %+v, proof = %v fallback=%v", s, proof, fallback)
	}
}
