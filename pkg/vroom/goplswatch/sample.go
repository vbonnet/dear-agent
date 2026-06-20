package goplswatch

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

// SampleNow gathers a live gopls pressure sample: it enumerates gopls processes
// (with RSS) via ps and reads system-wide FD-table usage via the platform
// resource probe. It is best-effort — an unavailable FD probe yields
// FDFraction 0 (treated as "unknown", never an alarm) rather than an error.
func SampleNow(ctx context.Context) (Sample, error) {
	procs, err := listGopls(ctx)
	if err != nil {
		return Sample{}, err
	}
	sample := Sample{Procs: procs}
	// FD usage is one of three independent signals; if the probe fails we still
	// report count/RSS rather than failing the whole pass.
	if snap, perr := supervisor.NewSysResourceProbe().Snapshot(ctx); perr == nil {
		sample.FDFraction = snap.OpenFDFraction
	}
	return sample, nil
}

// listGopls runs ps and returns the gopls processes it finds. The RSS column is
// reported by ps in kibibytes on both macOS and Linux.
func listGopls(ctx context.Context) ([]Proc, error) {
	// args= is the untruncated full argv; comm= is truncated to 16 chars on
	// macOS when not the last column, so we match on the args basename and keep
	// comm only as a fallback (mirrors agm/internal/orphan parsing).
	out, err := exec.CommandContext(ctx, "ps", "-ax", "-o", "pid=,ppid=,rss=,comm=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps failed: %w", err)
	}
	return parsePS(string(out)), nil
}

// parsePS parses `ps -ax -o pid=,ppid=,rss=,comm=,args=` output and returns
// only the gopls processes. It is pure so it can be unit-tested without ps.
func parsePS(out string) []Proc {
	var procs []Proc
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidTok, rest := nextField(line)
		pid, err := strconv.Atoi(pidTok)
		if err != nil {
			continue
		}
		ppidTok, rest := nextField(rest)
		ppid, err := strconv.Atoi(ppidTok)
		if err != nil {
			continue
		}
		rssTok, rest := nextField(rest)
		rss, err := strconv.ParseInt(rssTok, 10, 64)
		if err != nil {
			continue
		}
		commTok, args := nextField(rest)
		if commTok == "" {
			continue
		}
		// Prefer the untruncated argv[0] basename; fall back to comm.
		argv0, _ := nextField(args)
		cmd := basename(argv0)
		if cmd == "" {
			cmd = basename(commTok)
		}
		if cmd != "gopls" {
			continue
		}
		procs = append(procs, Proc{PID: pid, PPID: ppid, RSSKiB: rss, Cmd: cmd})
	}
	return procs
}

// nextField splits off the first whitespace-delimited token, returning it and
// the remainder with leading whitespace trimmed.
func nextField(s string) (token, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

func basename(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}
