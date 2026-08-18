package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

// --- test doubles for the disk + agent-process gates ---

type stubDisk struct {
	gb  float64
	err error
}

func (s stubDisk) FreeDiskGB() (float64, error) { return s.gb, s.err }

type stubProc struct {
	count int
	err   error
}

func (s stubProc) CountAgentProcs() (int, error) { return s.count, s.err }

// baseCfg is a permissive config so a single gate under test is the only thing
// that can fail.
func baseCfg() Config {
	return Config{
		MaxWorkers:       0,
		MaxLoad5:         1e9,
		MinFreeMemPct:    0,
		MinSpawnInterval: 0,
		MinFreeDiskGB:    15,
		MaxAgentProcs:    12,
	}
}

// passReaders are readers that let every original gate pass.
func passReaders() (LoadReader, WorkerCounter, SpawnTimer) {
	return stubLoad{load: 0}, stubWorkers{count: 0}, &stubTimer{err: errors.New("no prev spawn")}
}

// --- Gate 5: disk ---

func TestCheckDisk(t *testing.T) {
	cfg := baseCfg()
	cfg.MinFreeDiskGB = 15

	tests := []struct {
		name   string
		reader DiskReader
		wantOK bool
	}{
		{"well above floor", stubDisk{gb: 40}, true},
		{"just above floor", stubDisk{gb: 15.1}, true},
		{"at floor passes (strict <, matches mem gate)", stubDisk{gb: 15}, true},
		{"just below floor refuses", stubDisk{gb: 14.9}, false},
		{"well below floor refuses", stubDisk{gb: 13.5}, false},
		{"read error fails closed", stubDisk{err: errors.New("statfs boom")}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := checkDisk(cfg, tt.reader)
			if g.Passed != tt.wantOK {
				t.Fatalf("checkDisk passed=%v want=%v (msg: %s)", g.Passed, tt.wantOK, g.Message)
			}
			if g.Gate != "disk" {
				t.Errorf("gate name = %q want disk", g.Gate)
			}
		})
	}
}

// --- Gate 6: agent processes ---

func TestCheckAgentProcs(t *testing.T) {
	cfg := baseCfg()
	cfg.MaxAgentProcs = 4

	tests := []struct {
		name   string
		cfg    Config
		reader ProcCounter
		wantOK bool
	}{
		{"under cap", cfg, stubProc{count: 2}, true},
		{"at cap refuses", cfg, stubProc{count: 4}, false},
		{"over cap refuses", cfg, stubProc{count: 9}, false},
		{"count error fails closed", cfg, stubProc{err: errors.New("ps boom")}, false},
		{"cap disabled passes", func() Config { c := cfg; c.MaxAgentProcs = 0; return c }(), stubProc{count: 999}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := checkAgentProcs(tt.cfg, tt.reader)
			if g.Passed != tt.wantOK {
				t.Fatalf("checkAgentProcs passed=%v want=%v (msg: %s)", g.Passed, tt.wantOK, g.Message)
			}
			if g.Gate != "agent_procs" {
				t.Errorf("gate name = %q want agent_procs", g.Gate)
			}
		})
	}
}

// --- Check() wiring: options enable the new gates and they can refuse ---

func TestCheckWithNewGates(t *testing.T) {
	lr, wc, st := passReaders()
	cfg := baseCfg()

	t.Run("both gates pass -> allowed", func(t *testing.T) {
		r := Check(cfg, lr, wc, st, noMem,
			WithDiskReader(stubDisk{gb: 50}),
			WithProcCounter(stubProc{count: 1}))
		if !r.Allowed {
			t.Fatalf("expected allowed, got denied: %s", FormatDenied(r))
		}
		if !hasGate(r, "disk") || !hasGate(r, "agent_procs") {
			t.Errorf("expected disk and agent_procs gates present: %+v", r.Gates)
		}
	})

	t.Run("low disk refuses", func(t *testing.T) {
		r := Check(cfg, lr, wc, st, noMem,
			WithDiskReader(stubDisk{gb: 5}),
			WithProcCounter(stubProc{count: 1}))
		if r.Allowed {
			t.Fatal("expected denial on low disk")
		}
	})

	t.Run("proc cap refuses", func(t *testing.T) {
		c := cfg
		c.MaxAgentProcs = 2
		r := Check(c, lr, wc, st, noMem,
			WithDiskReader(stubDisk{gb: 50}),
			WithProcCounter(stubProc{count: 2}))
		if r.Allowed {
			t.Fatal("expected denial at proc cap")
		}
	})

	t.Run("no options -> new gates skipped (back-compat)", func(t *testing.T) {
		r := Check(cfg, lr, wc, st, noMem)
		if hasGate(r, "disk") || hasGate(r, "agent_procs") {
			t.Errorf("new gates should be absent without options: %+v", r.Gates)
		}
		if !r.Allowed {
			t.Errorf("expected allowed with permissive config and no new gates")
		}
	})
}

func hasGate(r CheckResult, name string) bool {
	for _, g := range r.Gates {
		if g.Gate == name {
			return true
		}
	}
	return false
}

// --- process-table parsing ---

func TestCountAgentProcs(t *testing.T) {
	// Mirrors real `ps -axo command=` output, including the tricky cases:
	// a GUI app whose path contains a space ("Codex Computer Use.app"), a
	// shell wrapper that merely mentions a ~/.claude path, and the real
	// codex/claude harness executables.
	psOutput := `
/Users/v/.codex/packages/standalone/releases/0.144.6/bin/codex app-server daemon
/Users/v/.codex/packages/standalone/releases/0.144.6/bin/codex app-server --remote-control
/Users/v/.local/bin/claude --dangerously-skip-permissions --print
/bin/zsh -c source /Users/v/.claude/shell-snapshots/snapshot-zsh-123
/Users/v/.codex/computer-use/Codex Computer Use.app/Contents/MacOS/SkyComputerUseService
/Applications/ChatGPT.app/Contents/MacOS/Codex
/Users/v/go/bin/agm-mcp-server
node /some/other/thing.js
`
	got := countAgentProcs(psOutput)
	want := 3 // two codex daemons + one claude harness
	if got != want {
		t.Fatalf("countAgentProcs = %d, want %d", got, want)
	}
}

func TestCountAgentProcsEmpty(t *testing.T) {
	if got := countAgentProcs(""); got != 0 {
		t.Fatalf("countAgentProcs(empty) = %d, want 0", got)
	}
}

// --- config env overrides ---

func TestDefaultConfigDiskAndProcOverrides(t *testing.T) {
	t.Setenv("AGM_MIN_FREE_DISK_GB", "22")
	t.Setenv("AGM_MAX_AGENT_PROCS", "3")

	cfg := DefaultConfig()
	if cfg.MinFreeDiskGB != 22 {
		t.Errorf("MinFreeDiskGB = %v, want 22", cfg.MinFreeDiskGB)
	}
	if cfg.MaxAgentProcs != 3 {
		t.Errorf("MaxAgentProcs = %v, want 3", cfg.MaxAgentProcs)
	}
}

func TestDefaultConfigDefaults(t *testing.T) {
	// Ensure the disk floor has a sane non-zero default and the proc cap is
	// enabled by default (so the gate is real out of the box).
	cfg := DefaultConfig()
	if cfg.MinFreeDiskGB <= 0 {
		t.Errorf("MinFreeDiskGB default = %v, want > 0", cfg.MinFreeDiskGB)
	}
	if cfg.MaxAgentProcs <= 0 {
		t.Errorf("MaxAgentProcs default = %v, want > 0 (gate enabled by default)", cfg.MaxAgentProcs)
	}
	_ = time.Second
}
