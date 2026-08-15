package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/sandboxgc"
)

// sandboxGCStorage is a minimal dolt.Storage fake that tolerates a nil filter
// (unlike mockStorage in stall_detector_test.go).
type sandboxGCStorage struct {
	mockStorage
	listErr   error
	listCalls int
}

func (s *sandboxGCStorage) ListSessions(filter *dolt.SessionFilter) ([]*manifest.Manifest, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.sessions, nil
}

// mkSandbox creates a fake sandbox dir (non-git, partial content) under base
// and backdates its mtime so the MinAge gate does not apply.
func mkSandbox(t *testing.T, base, name string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(filepath.Join(dir, "upper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "upper", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-age)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newTestChecker returns a checker over a temp base with empty mount/proc
// tables and real removal.
func newTestChecker(base string, live map[string]bool, liveErr error) *sandboxgc.Checker {
	return &sandboxgc.Checker{
		Base:           base,
		ListMounts:     func() ([]string, error) { return []string{"/"}, nil },
		ListProcPaths:  func() ([]sandboxgc.ProcPath, error) { return nil, nil },
		LiveSessionIDs: func() (map[string]bool, error) { return live, liveErr },
		Unmount:        func(string) error { return nil },
		Remove:         os.RemoveAll,
	}
}

func sandboxTestBase(t *testing.T) string {
	t.Helper()
	base := filepath.Join(t.TempDir(), ".agm", "sandboxes")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	return base
}

func TestSandboxGCDryRunByDefault(t *testing.T) {
	base := sandboxTestBase(t)
	dead := mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, map[string]bool{}, nil)

	res, err := sandboxGCWithChecker(&SandboxGCRequest{}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if !res.DryRun {
		t.Error("default run must be dry-run")
	}
	if res.Reaped != 1 || res.Scanned != 1 {
		t.Errorf("scanned=%d reaped=%d, want 1/1 (would-reap)", res.Scanned, res.Reaped)
	}
	if _, err := os.Stat(dead); err != nil {
		t.Errorf("dry-run must not delete anything: %v", err)
	}
	if res.Entries[0].Action != "would-reap" {
		t.Errorf("action = %q, want would-reap", res.Entries[0].Action)
	}
}

func TestSandboxGCReap(t *testing.T) {
	base := sandboxTestBase(t)
	dead := mkSandbox(t, base, "deadbeef", 24*time.Hour)
	liveDir := mkSandbox(t, base, "cafe0001", 24*time.Hour)
	fresh := mkSandbox(t, base, "beef0002", 10*time.Minute) // younger than MinAge
	checker := newTestChecker(base, map[string]bool{"cafe0001": true}, nil)

	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if res.Scanned != 3 || res.Reaped != 1 || res.Kept != 2 || res.Errors != 0 {
		t.Errorf("scanned=%d reaped=%d kept=%d errors=%d, want 3/1/2/0",
			res.Scanned, res.Reaped, res.Kept, res.Errors)
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Error("dead sandbox should have been reaped")
	}
	if _, err := os.Stat(liveDir); err != nil {
		t.Errorf("live-session sandbox must survive: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh sandbox must survive the age gate: %v", err)
	}
}

func TestSandboxGCRefusesWhenSessionStoreDown(t *testing.T) {
	base := sandboxTestBase(t)
	dead := mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, nil, errors.New("dolt down"))

	if _, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker); err == nil {
		t.Fatal("sweep must abort when live sessions cannot be enumerated")
	}
	if _, err := os.Stat(dead); err != nil {
		t.Errorf("nothing may be deleted on an aborted sweep: %v", err)
	}
}

func TestSandboxGCRefusesWithoutLiveSessionSource(t *testing.T) {
	base := sandboxTestBase(t)
	mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, nil, nil)
	checker.LiveSessionIDs = nil

	if _, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker); err == nil {
		t.Fatal("sweep must refuse to run without a live-session source")
	}
}

func TestSandboxGCKeepsMountedSandbox(t *testing.T) {
	base := sandboxTestBase(t)
	mounted := mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, map[string]bool{}, nil)
	// Simulate an overlay mount that survives the unmount attempt.
	checker.ListMounts = func() ([]string, error) {
		return []string{"/", filepath.Join(mounted, "merged")}, nil
	}

	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if res.Reaped != 0 || res.Kept != 1 {
		t.Errorf("reaped=%d kept=%d, want 0/1", res.Reaped, res.Kept)
	}
	if res.ProbeFailures != 0 {
		t.Errorf("ProbeFailures = %d, want 0 — the mount table was read fine and genuinely found a mount", res.ProbeFailures)
	}
	if _, err := os.Stat(mounted); err != nil {
		t.Errorf("mounted sandbox must never be removed: %v", err)
	}
	if res.Entries[0].Reason == "" {
		t.Error("kept entry should carry a refusal reason")
	}
}

func TestSandboxGCKeepsSandboxWithLiveProcess(t *testing.T) {
	base := sandboxTestBase(t)
	busy := mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, map[string]bool{}, nil)
	checker.ListProcPaths = func() ([]sandboxgc.ProcPath, error) {
		return []sandboxgc.ProcPath{{PID: 42, Path: filepath.Join(busy, "merged")}}, nil
	}

	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if res.Reaped != 0 || res.Kept != 1 {
		t.Errorf("reaped=%d kept=%d, want 0/1", res.Reaped, res.Kept)
	}
	if res.ProbeFailures != 0 {
		t.Errorf("ProbeFailures = %d, want 0 — lsof ran fine and genuinely found a live process", res.ProbeFailures)
	}
	if _, err := os.Stat(busy); err != nil {
		t.Errorf("in-use sandbox must never be removed: %v", err)
	}
}

// TestSandboxGCCountsProbeFailuresSeparately guards the review gap where a
// systemic safety-probe failure (lsof missing, mount table unreadable) was
// indistinguishable from a genuine live-process/mount finding: both landed in
// Kept with Errors left at 0, so a sweep that could not evaluate ANY sandbox
// still looked like a healthy idle sweep to the disk-watchdog heartbeat.
func TestSandboxGCCountsProbeFailuresSeparately(t *testing.T) {
	base := sandboxTestBase(t)
	mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, map[string]bool{}, nil)
	checker.ListProcPaths = func() ([]sandboxgc.ProcPath, error) {
		return nil, errors.New("lsof: command not found")
	}

	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if res.Reaped != 0 || res.Kept != 1 {
		t.Errorf("reaped=%d kept=%d, want 0/1", res.Reaped, res.Kept)
	}
	if res.Errors != 0 {
		t.Errorf("Errors = %d, want 0 — a probe failure is a refusal, not a removal failure", res.Errors)
	}
	if res.ProbeFailures != 1 {
		t.Errorf("ProbeFailures = %d, want 1 — the sweep could not evaluate this sandbox at all", res.ProbeFailures)
	}
}

// TestSandboxGCDryRunCountsProbeFailures covers the dry-run classification
// path (CheckReapable, not Reap), which computes ProbeFailures separately.
func TestSandboxGCDryRunCountsProbeFailures(t *testing.T) {
	base := sandboxTestBase(t)
	mkSandbox(t, base, "deadbeef", 24*time.Hour)
	checker := newTestChecker(base, map[string]bool{}, nil)
	checker.ListProcPaths = func() ([]sandboxgc.ProcPath, error) {
		return nil, errors.New("lsof: command not found")
	}

	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: false}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if res.Kept != 1 || res.ProbeFailures != 1 {
		t.Errorf("kept=%d probeFailures=%d, want 1/1", res.Kept, res.ProbeFailures)
	}
}

func TestSandboxGCReapsNonGitAndPartialDirs(t *testing.T) {
	// ce-nd1z regression: non-git and partial/corrupt sandbox layouts are
	// reapable content, never errors.
	base := sandboxTestBase(t)
	// Bare empty dir (partial provisioning).
	empty := filepath.Join(base, "empty001")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	// Stray regular file directly under base.
	stray := filepath.Join(base, "stray.tmp")
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	for _, p := range []string{empty, stray} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	checker := newTestChecker(base, map[string]bool{}, nil)

	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker)
	if err != nil {
		t.Fatalf("sandboxGCWithChecker: %v", err)
	}
	if res.Errors != 0 {
		t.Errorf("non-git/partial content must not produce errors: %+v", res.Entries)
	}
	if res.Reaped != 2 {
		t.Errorf("reaped=%d, want 2 (empty dir + stray file): %+v", res.Reaped, res.Entries)
	}
	for _, p := range []string{empty, stray} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should have been reaped", p)
		}
	}
}

func TestSandboxGCMissingBaseIsNoop(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".agm", "sandboxes") // never created
	checker := newTestChecker(base, map[string]bool{}, nil)
	res, err := sandboxGCWithChecker(&SandboxGCRequest{Reap: true}, base, checker)
	if err != nil {
		t.Fatalf("missing base must be a no-op, got %v", err)
	}
	if res.Scanned != 0 {
		t.Errorf("scanned = %d, want 0", res.Scanned)
	}
}

func TestLiveSessionIDsFromStorage(t *testing.T) {
	live := &manifest.Manifest{SessionID: "live1", Lifecycle: ""}
	archived := &manifest.Manifest{SessionID: "gone1", Lifecycle: manifest.LifecycleArchived}

	tests := []struct {
		name     string
		storage  *sandboxGCStorage
		wantErr  bool
		wantLive map[string]bool
	}{
		{
			name:     "live and archived split",
			storage:  &sandboxGCStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{live, archived}}},
			wantLive: map[string]bool{"live1": true},
		},
		{
			name:    "storage error fails closed",
			storage: &sandboxGCStorage{listErr: fmt.Errorf("dolt down")},
			wantErr: true,
		},
		{
			name:    "zero sessions fails closed",
			storage: &sandboxGCStorage{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &OpContext{Storage: tt.storage}
			got, err := liveSessionIDsFromStorage(ctx)()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(got) != len(tt.wantLive) {
				t.Errorf("live IDs = %v, want %v", got, tt.wantLive)
			}
			for id := range tt.wantLive {
				if !got[id] {
					t.Errorf("missing live ID %s", id)
				}
			}
		})
	}
}

func TestLiveSessionIDsMemoized(t *testing.T) {
	storage := &sandboxGCStorage{mockStorage: mockStorage{sessions: []*manifest.Manifest{
		{SessionID: "live1"},
	}}}
	fn := liveSessionIDsFromStorage(&OpContext{Storage: storage})
	for i := range 5 {
		if _, err := fn(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if storage.listCalls != 1 {
		t.Errorf("ListSessions called %d times, want 1 (memoized)", storage.listCalls)
	}

	// Errors are memoized too — a failed store is not retried mid-sweep.
	failing := &sandboxGCStorage{listErr: fmt.Errorf("dolt down")}
	fnErr := liveSessionIDsFromStorage(&OpContext{Storage: failing})
	for i := range 3 {
		if _, err := fnErr(); err == nil {
			t.Fatalf("call %d: want error", i)
		}
	}
	if failing.listCalls != 1 {
		t.Errorf("ListSessions called %d times on error path, want 1", failing.listCalls)
	}
}

func TestLiveSessionIDsNilContextFailsClosed(t *testing.T) {
	if _, err := liveSessionIDsFromStorage(nil)(); err == nil {
		t.Error("nil OpContext must fail closed")
	}
	if _, err := liveSessionIDsFromStorage(&OpContext{})(); err == nil {
		t.Error("nil Storage must fail closed")
	}
}
