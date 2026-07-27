package sandboxgc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testBase = "/home/u/.agm/sandboxes"

func TestValidateSandboxPath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		wantErr bool
	}{
		{"ok: directly under base", testBase, testBase + "/abc123", false},
		{"ok: uuid-style name", testBase, testBase + "/79a3f24f-1234-5678-9abc-def012345678", false},
		{"reject: base itself", testBase, testBase, true},
		{"reject: nested deeper", testBase, testBase + "/abc/merged", true},
		{"reject: sibling of base", testBase, "/home/u/.agm/other/abc", true},
		{"reject: parent escape", testBase, testBase + "/../repos", true},
		{"reject: dot-dot component", testBase, testBase + "/abc/..", true},
		{"reject: relative path", testBase, ".agm/sandboxes/abc", true},
		{"reject: root", testBase, "/", true},
		{"reject: home dir", testBase, "/home/u", true},
		{"reject: unclean path", testBase, testBase + "/abc/./", true},
		{"reject: base not ending in .agm/sandboxes", "/home/u/repos", "/home/u/repos/abc", true},
		{"reject: relative base", ".agm/sandboxes", ".agm/sandboxes/abc", true},
		{"reject: prefix-collision base", testBase + "x", testBase + "x/abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSandboxPath(tt.base, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSandboxPath(%q, %q) error = %v, wantErr %v",
					tt.base, tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestMountInside(t *testing.T) {
	dir := testBase + "/abc"
	tests := []struct {
		name   string
		mounts []string
		want   bool
	}{
		{"no mounts", nil, false},
		{"mount elsewhere", []string{"/", "/System/Volumes/Data"}, false},
		{"mount at merged", []string{"/", dir + "/merged"}, true},
		{"mount at dir itself", []string{dir}, true},
		{"mount deep inside", []string{dir + "/merged/repo/.git"}, true},
		{"prefix collision does not match", []string{testBase + "/abcdef"}, false},
		{"trailing slash mount", []string{dir + "/merged/"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := MountInside(tt.mounts, dir)
			if got != tt.want {
				t.Errorf("MountInside(%v, %q) = %v, want %v", tt.mounts, dir, got, tt.want)
			}
		})
	}
}

func TestProcessInside(t *testing.T) {
	dir := testBase + "/abc"
	tests := []struct {
		name  string
		procs []ProcPath
		want  bool
	}{
		{"none", nil, false},
		{"cwd elsewhere", []ProcPath{{1, "/tmp"}, {2, "/home/u"}}, false},
		{"cwd inside", []ProcPath{{42, dir + "/merged"}}, true},
		{"fd deep inside", []ProcPath{{42, dir + "/upper/x/y.log"}}, true},
		{"exactly dir", []ProcPath{{42, dir}}, true},
		{"prefix collision", []ProcPath{{42, testBase + "/abcdef/f"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := ProcessInside(tt.procs, dir)
			if got != tt.want {
				t.Errorf("ProcessInside(%v, %q) = %v, want %v", tt.procs, dir, got, tt.want)
			}
		})
	}
}

func TestParseMountOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{
			"macOS style",
			"/dev/disk3s1s1 on / (apfs, sealed, local, read-only, journaled)\n" +
				"bindfs@osxfuse0 on /home/u/.agm/sandboxes/abc/merged (osxfuse, nodev, nosuid, synchronous)\n",
			[]string{"/", "/home/u/.agm/sandboxes/abc/merged"},
		},
		{
			"mount point containing ' (' handled via last-paren split",
			"dev on /mnt/weird (name) dir (fuse, local)\n",
			[]string{"/mnt/weird (name) dir"},
		},
		{
			"linux style",
			"overlay on /home/u/.agm/sandboxes/abc/merged type overlay (rw,lowerdir=/repo)\n",
			[]string{"/home/u/.agm/sandboxes/abc/merged"},
		},
		{"empty", "", nil},
		{"garbage line skipped", "not a mount line\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMountOutput(tt.out)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseMountOutput() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mount[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseLsofOutput(t *testing.T) {
	out := "p123\nfcwd\nn/home/u/.agm/sandboxes/abc/merged\np456\nf5\nn/tmp/x.log\nnpipe:[123]\n"
	got := ParseLsofOutput(out)
	want := []ProcPath{
		{123, "/home/u/.agm/sandboxes/abc/merged"},
		{456, "/tmp/x.log"},
	}
	if len(got) != len(want) {
		t.Fatalf("ParseLsofOutput() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("proc[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// fakeChecker builds a Checker over fakes; mutate fields per test case.
type fakeHost struct {
	mounts     []string
	mountsErr  error
	procs      []ProcPath
	procsErr   error
	live       map[string]bool
	liveErr    error
	unmounted  []string
	unmountErr error
	removed    []string
	removeErr  error
	// unmountClears: when true, unmounting removes the path from mounts,
	// simulating a successful unmount.
	unmountClears bool
}

func (f *fakeHost) checker(base string, withLiveGate bool) *Checker {
	c := &Checker{
		Base: base,
		ListMounts: func(context.Context) ([]string, error) {
			return append([]string(nil), f.mounts...), f.mountsErr
		},
		ListProcPaths: func(context.Context) ([]ProcPath, error) { return f.procs, f.procsErr },
		Unmount: func(p string) error {
			f.unmounted = append(f.unmounted, p)
			if f.unmountClears {
				var kept []string
				for _, m := range f.mounts {
					if m != p {
						kept = append(kept, m)
					}
				}
				f.mounts = kept
			}
			return f.unmountErr
		},
		Remove: func(p string) error {
			f.removed = append(f.removed, p)
			return f.removeErr
		},
	}
	if withLiveGate {
		c.LiveSessionIDs = func() (map[string]bool, error) { return f.live, f.liveErr }
	}
	return c
}

func TestCheckReapable(t *testing.T) {
	dir := testBase + "/deadbeef"
	tests := []struct {
		name          string
		host          fakeHost
		dir           string
		liveGate      bool
		wantReason    string // "" = reapable
		wantProbeFail bool
		wantPID       int
	}{
		{
			name:     "reapable: no session, no process, no mount",
			host:     fakeHost{mounts: []string{"/", "/System/Volumes/Data"}},
			dir:      dir,
			liveGate: true,
		},
		{
			name:       "refused: path outside base",
			host:       fakeHost{},
			dir:        "/home/u/repos/x",
			liveGate:   true,
			wantReason: ReasonBadPath,
		},
		{
			name:       "refused: nested path",
			host:       fakeHost{},
			dir:        dir + "/merged",
			liveGate:   true,
			wantReason: ReasonBadPath,
		},
		{
			name:       "refused: live session references sandbox",
			host:       fakeHost{live: map[string]bool{"deadbeef": true}},
			dir:        dir,
			liveGate:   true,
			wantReason: ReasonLiveSession,
		},
		{
			name:          "refused (fail closed): session store unreachable",
			host:          fakeHost{liveErr: errors.New("dolt down")},
			dir:           dir,
			liveGate:      true,
			wantReason:    ReasonLiveSession,
			wantProbeFail: true,
		},
		{
			name:       "refused: process cwd inside",
			host:       fakeHost{procs: []ProcPath{{7, dir + "/merged"}}},
			dir:        dir,
			liveGate:   true,
			wantReason: ReasonLiveProcess,
			wantPID:    7,
		},
		{
			name:       "refused: open fd inside",
			host:       fakeHost{procs: []ProcPath{{7, dir + "/upper/log.txt"}}},
			dir:        dir,
			liveGate:   true,
			wantReason: ReasonLiveProcess,
			wantPID:    7,
		},
		{
			name:          "refused (fail closed): lsof unavailable",
			host:          fakeHost{procsErr: errors.New("lsof missing")},
			dir:           dir,
			liveGate:      true,
			wantReason:    ReasonLiveProcess,
			wantProbeFail: true,
		},
		{
			name:     "archive path: nil live gate skips session check",
			host:     fakeHost{live: map[string]bool{"deadbeef": true}},
			dir:      dir,
			liveGate: false, // gate disabled — caller archived the session itself
		},
		{
			name:       "session with dir-name prefix collision does not protect",
			host:       fakeHost{live: map[string]bool{"deadbeefcafe": true}},
			dir:        dir,
			liveGate:   true,
			wantReason: "", // deadbeef != deadbeefcafe — reapable
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.host.checker(testBase, tt.liveGate)
			err := c.CheckReapable(tt.dir)
			if tt.wantReason == "" {
				if err != nil {
					t.Fatalf("CheckReapable(%q) = %v, want reapable", tt.dir, err)
				}
				return
			}
			var ref *RefusalError
			if !errors.As(err, &ref) {
				t.Fatalf("CheckReapable(%q) = %v, want RefusalError", tt.dir, err)
			}
			if ref.Reason != tt.wantReason {
				t.Errorf("refusal reason = %q, want %q (detail: %s)", ref.Reason, tt.wantReason, ref.Detail)
			}
			if ref.ProbeFailure != tt.wantProbeFail {
				t.Errorf("ProbeFailure = %v, want %v (detail: %s)", ref.ProbeFailure, tt.wantProbeFail, ref.Detail)
			}
			if ref.ProcessID != tt.wantPID {
				t.Errorf("refusal process ID = %d, want %d (detail: %s)", ref.ProcessID, tt.wantPID, ref.Detail)
			}
		})
	}
}

func TestReap(t *testing.T) {
	dir := testBase + "/deadbeef"
	tests := []struct {
		name          string
		host          fakeHost
		wantReason    string // "" = reaped
		wantRemoved   bool
		wantProbeFail bool
	}{
		{
			name:        "happy path: unmount clears merged, then removed",
			host:        fakeHost{mounts: []string{"/", dir + "/merged"}, unmountClears: true},
			wantRemoved: true,
		},
		{
			name:        "happy path: nothing mounted at all",
			host:        fakeHost{mounts: []string{"/"}},
			wantRemoved: true,
		},
		{
			name:       "HARD GATE: mount survives unmount -> refuse, never remove",
			host:       fakeHost{mounts: []string{dir + "/merged"}, unmountClears: false},
			wantReason: ReasonMountInside,
		},
		{
			name: "HARD GATE: unmount reports success but table still shows mount",
			host: fakeHost{mounts: []string{dir + "/merged/inner"}, unmountClears: true},
			// unmountClears only clears exact-match paths; the inner mount
			// (not one we tried to unmount) survives and must block removal.
			wantReason: ReasonMountInside,
		},
		{
			name:       "live process blocks before any unmount happens",
			host:       fakeHost{procs: []ProcPath{{9, dir + "/merged/f"}}},
			wantReason: ReasonLiveProcess,
		},
		{
			name:       "live session blocks",
			host:       fakeHost{live: map[string]bool{"deadbeef": true}},
			wantReason: ReasonLiveSession,
		},
		{
			name:          "HARD GATE (fail closed): mount table unreadable -> refuse",
			host:          fakeHost{mountsErr: errors.New("mount cmd failed")},
			wantReason:    ReasonMountInside,
			wantProbeFail: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.host.checker(testBase, true)
			err := c.Reap(dir)
			if tt.wantReason == "" {
				if err != nil {
					t.Fatalf("Reap(%q) = %v, want success", dir, err)
				}
			} else {
				var ref *RefusalError
				if !errors.As(err, &ref) {
					t.Fatalf("Reap(%q) = %v, want RefusalError", dir, err)
				}
				if ref.Reason != tt.wantReason {
					t.Errorf("refusal reason = %q, want %q (detail: %s)", ref.Reason, tt.wantReason, ref.Detail)
				}
				if ref.ProbeFailure != tt.wantProbeFail {
					t.Errorf("ProbeFailure = %v, want %v (detail: %s)", ref.ProbeFailure, tt.wantProbeFail, ref.Detail)
				}
			}
			gotRemoved := len(tt.host.removed) > 0
			if gotRemoved != tt.wantRemoved {
				t.Errorf("removed = %v (%v), want removed=%v", gotRemoved, tt.host.removed, tt.wantRemoved)
			}
			// Invariant: a refused reap must NEVER call Remove.
			if tt.wantReason != "" && gotRemoved {
				t.Errorf("SAFETY VIOLATION: refused reap still removed %v", tt.host.removed)
			}
		})
	}
}

func TestReapProcessGateBlocksBeforeUnmount(t *testing.T) {
	dir := testBase + "/deadbeef"
	host := fakeHost{procs: []ProcPath{{9, dir + "/merged/f"}}, mounts: []string{dir + "/merged"}}
	c := host.checker(testBase, true)
	if err := c.Reap(dir); err == nil {
		t.Fatal("want refusal")
	}
	if len(host.unmounted) != 0 {
		t.Errorf("unmount attempted despite live process: %v", host.unmounted)
	}
}

func TestReapContextBoundsInFlightSafetyScans(t *testing.T) {
	dir := testBase + "/deadbeef"
	tests := []struct {
		name          string
		listProcPaths func(context.Context) ([]ProcPath, error)
		listMounts    func(context.Context) ([]string, error)
		wantUnmounts  int
	}{
		{
			name: "process inspection",
			listProcPaths: func(ctx context.Context) ([]ProcPath, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			listMounts: func(context.Context) ([]string, error) {
				return nil, nil
			},
		},
		{
			name: "mount inspection",
			listProcPaths: func(context.Context) ([]ProcPath, error) {
				return nil, nil
			},
			listMounts: func(ctx context.Context) ([]string, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			wantUnmounts: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var unmounts, removes int
			checker := &Checker{
				Base:          testBase,
				ListMounts:    tt.listMounts,
				ListProcPaths: tt.listProcPaths,
				Unmount: func(string) error {
					unmounts++
					return nil
				},
				Remove: func(string) error {
					removes++
					return nil
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := checker.ReapContext(ctx, dir)
			if time.Since(start) > 250*time.Millisecond {
				t.Fatalf("ReapContext exceeded bounded scan lifetime: %v", time.Since(start))
			}
			var refusal *RefusalError
			if !errors.As(err, &refusal) {
				t.Fatalf("ReapContext() error = %v, want fail-closed refusal", err)
			}
			if refusal.ProcessID != 0 {
				t.Fatalf("deadline refusal process ID = %d, want 0", refusal.ProcessID)
			}
			if unmounts != tt.wantUnmounts {
				t.Fatalf("unmount calls = %d, want %d", unmounts, tt.wantUnmounts)
			}
			if removes != 0 {
				t.Fatalf("SAFETY VIOLATION: expired scan removed sandbox %d time(s)", removes)
			}
		})
	}
}

func TestNewCheckerDefaults(t *testing.T) {
	c := NewChecker(testBase, nil)
	if c.Base != testBase {
		t.Errorf("Base = %q, want %q", c.Base, testBase)
	}
	if c.ListMounts == nil || c.ListProcPaths == nil || c.Unmount == nil || c.Remove == nil {
		t.Error("NewChecker left a dependency nil")
	}
	if c.LiveSessionIDs != nil {
		t.Error("LiveSessionIDs should stay nil when not provided")
	}
}

// TestReapRealFS exercises Reap against a real temp directory with real
// os.RemoveAll (mount/proc tables faked empty) to prove the deletion path
// works end-to-end on disk, including non-git partial content (ce-nd1z).
func TestReapRealFS(t *testing.T) {
	base := filepath.Join(t.TempDir(), ".agm", "sandboxes")
	dir := filepath.Join(base, "deadbeef")
	// Partial, non-git sandbox content: must be reapable, never an error.
	if err := os.MkdirAll(filepath.Join(dir, "upper", "partial"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "upper", "partial", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &Checker{
		Base:           base,
		ListMounts:     func(context.Context) ([]string, error) { return []string{"/"}, nil },
		ListProcPaths:  func(context.Context) ([]ProcPath, error) { return nil, nil },
		LiveSessionIDs: func() (map[string]bool, error) { return nil, nil },
		Unmount:        func(string) error { return nil },
		Remove:         os.RemoveAll,
	}
	if err := c.Reap(dir); err != nil {
		t.Fatalf("Reap() = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("sandbox dir still exists after Reap: %v", err)
	}
}

func TestRefusalErrorMessage(t *testing.T) {
	e := &RefusalError{Path: "/p", Reason: ReasonMountInside, Detail: "d"}
	want := fmt.Sprintf("refusing to reap %s: %s (%s)", "/p", ReasonMountInside, "d")
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}
