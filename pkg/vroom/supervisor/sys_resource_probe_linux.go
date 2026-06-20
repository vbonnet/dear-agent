//go:build linux

package supervisor

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
)

// sysMemoryUsedFraction returns the fraction of physical RAM currently in use
// on Linux by parsing /proc/meminfo.
func sysMemoryUsedFraction() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	vals := make(map[string]uint64, 4)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// rest looks like "  1234567 kB"
		fields := strings.Fields(strings.TrimSpace(rest))
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v
		if len(vals) == 2 && vals["MemTotal"] > 0 && vals["MemAvailable"] > 0 {
			break
		}
	}

	total := vals["MemTotal"]
	avail := vals["MemAvailable"]
	if total == 0 || avail > total {
		return 0
	}
	return float64(total-avail) / float64(total)
}

// sysSwapUsedFraction returns the fraction of swap space currently in use on
// Linux by parsing SwapTotal/SwapFree from /proc/meminfo.
func sysSwapUsedFraction() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	vals := make(map[string]uint64, 2)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if key != "SwapTotal" && key != "SwapFree" {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(rest))
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v
		if len(vals) == 2 {
			break
		}
	}

	total := vals["SwapTotal"]
	free := vals["SwapFree"]
	if total == 0 {
		return 0
	}
	if free >= total {
		return 0
	}
	return float64(total-free) / float64(total)
}

// sysFDUsedFraction returns the fraction of the system-wide open-file
// descriptor limit currently in use on Linux.
//
// /proc/sys/fs/file-nr contains three whitespace-separated fields:
// allocated_fds, unused_allocated_fds (always 0 since Linux 2.6), max_fds.
func sysFDUsedFraction() float64 {
	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return 0
	}
	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) < 3 {
		return 0
	}
	alloc, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || alloc < 0 {
		return 0
	}
	maxFDs, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || maxFDs <= 0 {
		return 0
	}
	if alloc >= maxFDs {
		return 1
	}
	return float64(alloc) / float64(maxFDs)
}

// sysVnodeUsedFraction returns 0 on Linux — the vnode abstraction is Darwin-
// specific. Linux uses a unified page cache and dentry/inode caches instead.
func sysVnodeUsedFraction() float64 { return 0 }

// sysGoplsCount returns the number of *orphaned* gopls processes on Linux —
// gopls instances reparented to PID 1 because the Claude session that spawned
// them died. This is the leak signal the Overseer escalates on.
//
// It must NOT count live gopls: every healthy Claude Code session runs its own
// gopls (via the gopls-lsp plugin), so a raw process count scales with the
// number of live sessions and produces phantom leak alarms (ce-u7v9). We match
// /proc/<pid>/comm exactly against "gopls" (never a substring of a longer
// command line) AND require PPid==1 read from /proc/<pid>/status.
func sysGoplsCount(ctx context.Context) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return n
		default:
		}
		if !e.IsDir() {
			continue
		}
		if _, parseErr := strconv.ParseInt(e.Name(), 10, 64); parseErr != nil {
			continue
		}
		comm, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) != "gopls" {
			continue
		}
		if procPPID("/proc/"+e.Name()+"/status") == 1 {
			n++
		}
	}
	return n
}

// procPPID parses the PPid field from a /proc/<pid>/status file. Returns 0 if
// the file cannot be read or the field is absent (so the process is not counted
// as an orphan).
func procPPID(statusPath string) int {
	f, err := os.Open(statusPath)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		ppid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
		if err != nil {
			return 0
		}
		return ppid
	}
	return 0
}
