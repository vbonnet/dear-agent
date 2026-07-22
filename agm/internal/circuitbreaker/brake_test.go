package circuitbreaker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/admission"
)

// findGate returns the named gate from a CheckResult, or a zero GateResult when
// the gate did not run. Shared by the gate tests in this package.
func findGate(r CheckResult, name string) GateResult {
	for _, g := range r.Gates {
		if g.Gate == name {
			return g
		}
	}
	return GateResult{}
}

// gateRan reports whether the named gate appears in the result at all.
func gateRan(r CheckResult, name string) bool {
	for _, g := range r.Gates {
		if g.Gate == name {
			return true
		}
	}
	return false
}

type stubBrake struct {
	brake *admission.Brake
	err   error
}

func (s stubBrake) Brake() (*admission.Brake, error) { return s.brake, s.err }

// checkWithBrake runs Check with permissive readers so the brake gate is the
// only thing that can refuse.
func checkWithBrake(br BrakeReader) CheckResult {
	lr, wc, st := passReaders()
	return Check(baseCfg(), lr, wc, st, nil, WithBrakeReader(br))
}

func TestBrakeGate_NotWiredMeansNoGate(t *testing.T) {
	lr, wc, st := passReaders()
	r := Check(baseCfg(), lr, wc, st, nil)

	if gateRan(r, "admission_brake") {
		t.Error("admission_brake gate ran without a reader wired")
	}
	if !r.Allowed {
		t.Errorf("spawn refused with no brake reader wired: %s", FormatDenied(r))
	}
}

func TestBrakeGate_NoBrakePasses(t *testing.T) {
	r := checkWithBrake(stubBrake{})

	if !r.Allowed {
		t.Errorf("spawn refused with no brake engaged: %s", FormatDenied(r))
	}
	if g := findGate(r, "admission_brake"); !g.Passed {
		t.Errorf("admission_brake should pass, got %q", g.Message)
	}
}

func TestBrakeGate_EngagedBrakeRefuses(t *testing.T) {
	now := time.Now().UTC()
	r := checkWithBrake(stubBrake{brake: &admission.Brake{
		Source:     "disk-watchdog",
		Reason:     "worktree-sweep remediation failed: signal: killed",
		SetAtUTC:   now.Add(-3 * time.Minute),
		ExpiresUTC: now.Add(27 * time.Minute),
	}})

	if r.Allowed {
		t.Fatal("an engaged brake must refuse the spawn")
	}
	g := findGate(r, "admission_brake")
	if g.Passed {
		t.Fatal("admission_brake gate passed with a live brake")
	}
	if !strings.Contains(g.Message, "disk-watchdog") {
		t.Errorf("message must name the brake source, got %q", g.Message)
	}
	if !strings.Contains(g.Message, "signal: killed") {
		t.Errorf("message must carry the brake reason, got %q", g.Message)
	}
}

func TestBrakeGate_UnreadableBrakeFailsClosed(t *testing.T) {
	r := checkWithBrake(stubBrake{err: errors.New("parse: unexpected EOF")})

	if r.Allowed {
		t.Fatal("an unreadable brake must refuse the spawn — it is not evidence of health")
	}
	if g := findGate(r, "admission_brake"); g.Passed {
		t.Errorf("admission_brake gate passed with an unreadable brake: %q", g.Message)
	}
}

// The unverified-probe override covers load and memory only. The brake is the
// gate that exists precisely to be unconditional (ce-93lw.18).
func TestBrakeGate_OverrideCannotPassABrake(t *testing.T) {
	t.Setenv(allowUnverifiedEnv, "1")
	now := time.Now().UTC()

	engaged := checkWithBrake(stubBrake{brake: &admission.Brake{
		Source:     "vroom-governor",
		Reason:     "load probe unreadable",
		SetAtUTC:   now,
		ExpiresUTC: now.Add(time.Hour),
	}})
	if engaged.Allowed {
		t.Errorf("%s must not pass an engaged brake", allowUnverifiedEnv)
	}

	unreadable := checkWithBrake(stubBrake{err: errors.New("boom")})
	if unreadable.Allowed {
		t.Errorf("%s must not pass an unreadable brake", allowUnverifiedEnv)
	}
}

func TestBrakeGate_MessageNamesTheFileToDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	if err := admission.Engage(path, "disk-watchdog", "sweep killed", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	r := checkWithBrake(FileBrakeReader{Path: path})
	g := findGate(r, "admission_brake")
	if g.Passed {
		t.Fatal("live brake on disk should refuse")
	}
	if !strings.Contains(g.Message, path) {
		t.Errorf("message must name %s so an operator can clear it, got %q", path, g.Message)
	}
}

func TestFileBrakeReader_ReadsWhatEngageWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	reader := FileBrakeReader{Path: path}

	got, err := reader.Brake()
	if err != nil || got != nil {
		t.Fatalf("absent brake: got=%v err=%v, want nil/nil", got, err)
	}

	if err := admission.Engage(path, "disk-watchdog", "sweep killed", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	got, err = reader.Brake()
	if err != nil {
		t.Fatalf("Brake: %v", err)
	}
	if got == nil || got.Source != "disk-watchdog" {
		t.Fatalf("Brake() = %+v, want a disk-watchdog brake", got)
	}

	if err := admission.Release(path); err != nil {
		t.Fatalf("Release: %v", err)
	}
	got, err = reader.Brake()
	if err != nil || got != nil {
		t.Fatalf("after Release: got=%v err=%v, want nil/nil", got, err)
	}
}

func TestFileBrakeReader_CorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := (FileBrakeReader{Path: path}).Brake(); err == nil {
		t.Error("a corrupt brake file must surface as an error so the gate fails closed")
	}
}

func TestFileBrakeReader_DefaultsToAdmissionPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGM_CONFIG_DIR", dir)
	if got, want := (FileBrakeReader{}).BrakePath(), admission.DefaultPath(); got != want {
		t.Errorf("BrakePath() = %q, want %q", got, want)
	}
}

// DefaultConfig's disk floor is a bead acceptance criterion, not an incidental
// constant (ce-93lw.18 (a)).
func TestDefaultConfig_DiskFloorIs15GiB(t *testing.T) {
	t.Setenv("AGM_MIN_FREE_DISK_GB", "")
	if got := DefaultConfig().MinFreeDiskGB; got != 15 {
		t.Errorf("MinFreeDiskGB = %v, want 15", got)
	}
}

func TestDefaultConfig_DiskFloorHonoursOverride(t *testing.T) {
	t.Setenv("AGM_MIN_FREE_DISK_GB", "40")
	if got := DefaultConfig().MinFreeDiskGB; got != 40 {
		t.Errorf("MinFreeDiskGB = %v, want 40", got)
	}
}
