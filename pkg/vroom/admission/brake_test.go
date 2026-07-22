package admission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func brakePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "admission-brake.json")
}

func TestRead_AbsentBrakeIsHealthy(t *testing.T) {
	got, err := Read(brakePath(t))
	if err != nil {
		t.Fatalf("Read on absent brake: %v", err)
	}
	if got != nil {
		t.Errorf("Read on absent brake = %+v, want nil", got)
	}
}

func TestEngage_RoundTrips(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "disk-watchdog", "remediation killed", 10*time.Minute); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case got == nil:
		t.Fatal("Read returned nil for a live brake")
	case got.Source != "disk-watchdog":
		t.Errorf("Source = %q, want disk-watchdog", got.Source)
	case got.Reason != "remediation killed":
		t.Errorf("Reason = %q, want %q", got.Reason, "remediation killed")
	case !got.ExpiresUTC.Equal(got.SetAtUTC.Add(10 * time.Minute)):
		t.Errorf("ExpiresUTC = %v, want SetAtUTC+10m = %v", got.ExpiresUTC, got.SetAtUTC.Add(10*time.Minute))
	}
}

func TestEngage_ZeroTTLUsesDefault(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "vroom-governor", "probe unreadable", 0); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	got, err := Read(path)
	switch {
	case err != nil || got == nil:
		t.Fatalf("Read: got=%v err=%v", got, err)
	case !got.ExpiresUTC.Equal(got.SetAtUTC.Add(DefaultTTL)):
		t.Errorf("ExpiresUTC = %v, want SetAtUTC+DefaultTTL = %v",
			got.ExpiresUTC, got.SetAtUTC.Add(DefaultTTL))
	}
}

func TestEngage_ReplacesExistingBrake(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "disk-watchdog", "first", time.Hour); err != nil {
		t.Fatalf("first Engage: %v", err)
	}
	if err := Engage(path, "vroom-governor", "second", time.Hour); err != nil {
		t.Fatalf("second Engage: %v", err)
	}
	got, err := Read(path)
	switch {
	case err != nil || got == nil:
		t.Fatalf("Read: got=%v err=%v", got, err)
	case got.Source != "vroom-governor" || got.Reason != "second":
		t.Errorf("brake = %+v, want the second engage to win", got)
	}
}

func TestEngage_CreatesMissingDirAndLeavesNoTempFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "agm")
	path := filepath.Join(dir, "admission-brake.json")
	if err := Engage(path, "disk-watchdog", "boom", time.Minute); err != nil {
		t.Fatalf("Engage into a missing dir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "admission-brake.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir contents = %v, want only admission-brake.json (atomic write must not strand temps)", names)
	}
}

func TestEngage_WritesOwnerOnlyPermissions(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "disk-watchdog", "boom", time.Minute); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("brake perms = %v, want 0600", perm)
	}
}

func TestRead_ExpiredBrakeIsHealthy(t *testing.T) {
	path := brakePath(t)
	past := time.Now().UTC().Add(-2 * time.Hour)
	writeRawBrake(t, path, Brake{
		Source:     "disk-watchdog",
		Reason:     "stale",
		SetAtUTC:   past,
		ExpiresUTC: past.Add(time.Minute),
	})

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read on expired brake: %v", err)
	}
	if got != nil {
		t.Errorf("Read on expired brake = %+v, want nil", got)
	}
}

func TestRead_MalformedBrakeIsAnError(t *testing.T) {
	path := brakePath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("expected an error for a malformed brake — an unreadable latch is not evidence of health")
	}
}

func TestRead_MissingExpiryIsAnError(t *testing.T) {
	path := brakePath(t)
	writeRawBrake(t, path, Brake{
		Source:   "disk-watchdog",
		Reason:   "no expiry",
		SetAtUTC: time.Now().UTC(),
	})
	_, err := Read(path)
	if err == nil {
		t.Fatal("expected an error for a brake with no expiry")
	}
	if !strings.Contains(err.Error(), "expiry") {
		t.Errorf("error = %v, want it to name the missing expiry", err)
	}
}

func TestRelease_RemovesBrake(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "disk-watchdog", "boom", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	if err := Release(path); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("brake file still present after Release (stat err = %v)", err)
	}
}

func TestRelease_AbsentBrakeIsNotAnError(t *testing.T) {
	if err := Release(brakePath(t)); err != nil {
		t.Fatalf("Release on absent brake: %v", err)
	}
}

func TestDefaultPath_HonoursConfigDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	if got, want := DefaultPath(), filepath.Join(dir, "admission-brake.json"); got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPath_SitsBesideLastSpawnFile(t *testing.T) {
	t.Setenv("AGM_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory on this host: %v", err)
	}
	if got, want := DefaultPath(), filepath.Join(home, ".agm", "admission-brake.json"); got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestExpired(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name    string
		expires time.Time
		want    bool
	}{
		{"future expiry is live", now.Add(time.Minute), false},
		{"past expiry is expired", now.Add(-time.Minute), true},
		{"expiry exactly now is expired", now, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := Brake{ExpiresUTC: tc.expires}
			if got := b.Expired(now); got != tc.want {
				t.Errorf("Expired() = %v, want %v", got, tc.want)
			}
		})
	}
}

// writeRawBrake writes a Brake without going through Engage, so tests can
// construct records Engage would never produce (expired, missing expiry).
func writeRawBrake(t *testing.T, path string, b Brake) {
	t.Helper()
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal brake: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write brake: %v", err)
	}
}

func TestReleaseBySource_RemovesItsOwnBrake(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "disk-watchdog", "sweep killed", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	if err := ReleaseBySource(path, "disk-watchdog"); err != nil {
		t.Fatalf("ReleaseBySource: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("brake survived a same-source release (stat err = %v)", err)
	}
}

// Two watchdogs on different cadences must not undo each other: vroom-governor
// ticks every 30s and would otherwise clear a disk-watchdog brake within half a
// minute of it being engaged.
func TestReleaseBySource_LeavesAnotherSourcesBrake(t *testing.T) {
	path := brakePath(t)
	if err := Engage(path, "disk-watchdog", "sweep killed", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	if err := ReleaseBySource(path, "vroom-governor"); err != nil {
		t.Fatalf("ReleaseBySource: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case got == nil:
		t.Fatal("a foreign brake was cleared; one watchdog must not undo another's")
	case got.Source != "disk-watchdog":
		t.Errorf("Source = %q, want disk-watchdog", got.Source)
	}
}

func TestReleaseBySource_AbsentAndExpiredAreNoOps(t *testing.T) {
	path := brakePath(t)
	if err := ReleaseBySource(path, "disk-watchdog"); err != nil {
		t.Errorf("ReleaseBySource on an absent brake: %v", err)
	}

	past := time.Now().UTC().Add(-2 * time.Hour)
	writeRawBrake(t, path, Brake{
		Source:     "disk-watchdog",
		Reason:     "stale",
		SetAtUTC:   past,
		ExpiresUTC: past.Add(time.Minute),
	})
	if err := ReleaseBySource(path, "disk-watchdog"); err != nil {
		t.Errorf("ReleaseBySource on an expired brake: %v", err)
	}
}

// Clearing what we cannot read would silently unblock the host; an unparseable
// latch still refuses spawns, so it stays put.
func TestReleaseBySource_LeavesAnUnreadableBrake(t *testing.T) {
	path := brakePath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := ReleaseBySource(path, "disk-watchdog"); err == nil {
		t.Error("expected an error rather than a silent release of an unreadable brake")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("unreadable brake was removed: %v", err)
	}
}
