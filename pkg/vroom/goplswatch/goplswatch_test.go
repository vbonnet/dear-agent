package goplswatch

import "testing"

func TestSampleAggregates(t *testing.T) {
	s := Sample{
		Procs: []Proc{
			{PID: 10, PPID: 1, RSSKiB: 1024 * 1024, Cmd: "gopls"},  // orphan, 1024 MB
			{PID: 11, PPID: 500, RSSKiB: 512 * 1024, Cmd: "gopls"}, // live, 512 MB
			{PID: 12, PPID: 1, RSSKiB: 256 * 1024, Cmd: "gopls"},   // orphan, 256 MB
		},
		FDFraction: 0.42,
	}
	if got := s.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
	if got := s.OrphanCount(); got != 2 {
		t.Errorf("OrphanCount() = %d, want 2", got)
	}
	if got := s.RSSMB(); got != 1792 {
		t.Errorf("RSSMB() = %v, want 1792", got)
	}
}

func TestEvaluate(t *testing.T) {
	gopls := func(n int, ppid int, rssMB int64) []Proc {
		var ps []Proc
		for i := range n {
			ps = append(ps, Proc{PID: 100 + i, PPID: ppid, RSSKiB: rssMB * 1024, Cmd: "gopls"})
		}
		return ps
	}

	tests := []struct {
		name     string
		sample   Sample
		thresh   Thresholds
		breached []string // expected metrics, in order
	}{
		{
			name:   "all within limits",
			sample: Sample{Procs: gopls(2, 500, 100), FDFraction: 0.5},
		},
		{
			name:   "many LIVE gopls do not alarm (ce-u7v9 phantom guard)",
			sample: Sample{Procs: gopls(40, 500, 100), FDFraction: 0.5}, // 40 live, 0 orphan
		},
		{
			name:     "count breach keys on orphans",
			sample:   Sample{Procs: gopls(5, 1, 10), FDFraction: 0.1}, // 5 orphans
			breached: []string{"count"},
		},
		{
			name:     "rss breach keys on orphans",
			sample:   Sample{Procs: gopls(2, 1, 1500), FDFraction: 0.1}, // 2 orphans (<=3), 3000 MB
			breached: []string{"rss"},
		},
		{
			name:     "fd breach only (no orphans to reap)",
			sample:   Sample{Procs: gopls(1, 500, 10), FDFraction: 0.95},
			breached: []string{"fd"},
		},
		{
			name:     "all three breach",
			sample:   Sample{Procs: gopls(6, 1, 500), FDFraction: 0.9}, // 6 orphans, 3000 MB
			breached: []string{"count", "rss", "fd"},
		},
		{
			name:   "at threshold is not a breach (strict >)",
			sample: Sample{Procs: gopls(3, 1, 10), FDFraction: 0.80},
			thresh: Thresholds{MaxCount: 3, MaxRSSMB: 2000, MaxFDFraction: 0.80},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alarms := Evaluate(tt.sample, tt.thresh)
			if len(alarms) != len(tt.breached) {
				t.Fatalf("got %d alarms %+v, want %d (%v)", len(alarms), alarms, len(tt.breached), tt.breached)
			}
			for i, want := range tt.breached {
				if alarms[i].Metric != want {
					t.Errorf("alarm[%d].Metric = %q, want %q", i, alarms[i].Metric, want)
				}
			}
		})
	}
}

func TestEvaluateUsesDefaultsForZeroFields(t *testing.T) {
	// A zero Thresholds{} must fall back to DefaultThresholds, not disable all
	// checks (which would make the watchdog silently inert).
	s := Sample{Procs: make([]Proc, 10), FDFraction: 0.99}
	for i := range s.Procs {
		s.Procs[i] = Proc{PID: i, PPID: 1, RSSKiB: 1024 * 1024, Cmd: "gopls"}
	}
	alarms := Evaluate(s, Thresholds{})
	if len(alarms) != 3 {
		t.Fatalf("got %d alarms, want 3 (count+rss+fd via defaults): %+v", len(alarms), alarms)
	}
}

func TestParsePS(t *testing.T) {
	// pid ppid rss comm args — gopls lines plus noise that must be ignored.
	out := `
  501     1  1048576 gopls /opt/homebrew/bin/gopls -mode=stdio
  502   400   524288 gopls /Users/x/go/bin/gopls serve
  600   400    20480 go    go build ./...
  601     1    10240 agm-mcp-server /Users/x/go/bin/agm-mcp-server
garbage line
`
	procs := parsePS(out)
	if len(procs) != 2 {
		t.Fatalf("parsePS returned %d procs, want 2: %+v", len(procs), procs)
	}
	if procs[0].PID != 501 || procs[0].PPID != 1 || procs[0].RSSKiB != 1048576 {
		t.Errorf("procs[0] = %+v, want pid=501 ppid=1 rss=1048576", procs[0])
	}
	if !procs[0].IsOrphan() {
		t.Errorf("procs[0] should be an orphan (PPID==1)")
	}
	if procs[1].PID != 502 || procs[1].IsOrphan() {
		t.Errorf("procs[1] = %+v, want pid=502 non-orphan", procs[1])
	}
}
