// Package resourcemon collects process and memory metrics, detects orphaned
// gopls / llama-server processes, and optionally kills them.
package resourcemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// safeKillTargets are process names we consider safe to kill when orphaned.
var safeKillTargets = map[string]bool{
	"gopls":        true,
	"llama-server": true,
}

// watchedNames are the process name substrings we report on.
var watchedNames = []string{"claude", "gopls", "llama-server", "agm-mcp-server"}

// ProcessInfo is a single process row from ps.
type ProcessInfo struct {
	PID   int
	PPID  int
	RSSMB int64
	State string
	Name  string
}

// SystemMemory holds host memory/swap figures.
type SystemMemory struct {
	TotalMB     int64
	AvailMB     int64 // free + inactive + speculative (macOS Activity Monitor definition)
	SwapUsedMB  int64
	SwapTotalMB int64
}

// UsedPct returns the percentage of RAM in use (0–100).
// Uses the macOS Activity Monitor definition of available (free+inactive+speculative).
func (m SystemMemory) UsedPct() float64 {
	if m.TotalMB == 0 {
		return 0
	}
	return float64(m.TotalMB-m.AvailMB) / float64(m.TotalMB) * 100
}

// OrphanInfo describes a process whose parent is dead or launchd (PID 1).
type OrphanInfo struct {
	PID    int    `json:"pid"`
	Name   string `json:"name"`
	RSSMB  int64  `json:"rss_mb"`
	Reason string `json:"reason"`
}

// ProcGroup aggregates all watched processes by name.
type ProcGroup struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	RSSMB int64  `json:"rss_mb"`
}

// Report is the full snapshot returned by Collect.
type Report struct {
	Timestamp   time.Time    `json:"ts"`
	MemTotalMB  int64        `json:"mem_total_mb"`
	MemAvailMB  int64        `json:"mem_avail_mb"`
	MemUsedPct  float64      `json:"mem_used_pct"`
	SwapUsedMB  int64        `json:"swap_used_mb"`
	SwapTotalMB int64        `json:"swap_total_mb"`
	Groups      []ProcGroup  `json:"processes"`
	Orphans     []OrphanInfo `json:"orphans"`
	Zombies     []OrphanInfo `json:"zombies"`
	Alerts      []string     `json:"alerts"`
	Killed      []OrphanInfo `json:"killed,omitempty"`
}

// Collect gathers a full resource snapshot. If kill is true, it kills detected
// orphans that belong to safeKillTargets.
func Collect(kill bool) (*Report, error) {
	procs, err := listProcesses()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	mem, err := systemMemory()
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	r := &Report{
		Timestamp:   time.Now().UTC(),
		MemTotalMB:  mem.TotalMB,
		MemAvailMB:  mem.AvailMB,
		MemUsedPct:  mem.UsedPct(),
		SwapUsedMB:  mem.SwapUsedMB,
		SwapTotalMB: mem.SwapTotalMB,
		Orphans:     []OrphanInfo{},
		Zombies:     []OrphanInfo{},
		Alerts:      []string{},
	}

	// PID set for parent-liveness checks.
	pidSet := map[int]bool{}
	for _, p := range procs {
		pidSet[p.PID] = true
	}

	groups := map[string]*ProcGroup{}
	for _, p := range procs {
		matched := matchedName(p.Name)
		if matched == "" {
			continue
		}
		g := groups[matched]
		if g == nil {
			g = &ProcGroup{Name: matched}
			groups[matched] = g
		}
		g.Count++
		g.RSSMB += p.RSSMB

		if p.State == "Z" {
			r.Zombies = append(r.Zombies, OrphanInfo{
				PID: p.PID, Name: p.Name, RSSMB: p.RSSMB, Reason: "zombie",
			})
			continue
		}

		// Orphan detection: only care about processes we're willing to kill
		// (gopls, llama-server). Root-level app processes legitimately have PPID=1.
		if safeKillTargets[matched] && (p.PPID == 1 || !pidSet[p.PPID]) {
			reason := "ppid=1 (reparented to launchd)"
			if !pidSet[p.PPID] {
				reason = fmt.Sprintf("ppid=%d dead", p.PPID)
			}
			oi := OrphanInfo{PID: p.PID, Name: p.Name, RSSMB: p.RSSMB, Reason: reason}
			r.Orphans = append(r.Orphans, oi)
			if kill {
				if err2 := killProc(p.PID); err2 == nil {
					r.Killed = append(r.Killed, oi)
				}
			}
		}
	}

	for _, g := range groups {
		r.Groups = append(r.Groups, *g)
	}

	// Alerts.
	if r.MemUsedPct > 80 {
		r.Alerts = append(r.Alerts, fmt.Sprintf("memory %.0f%% used (%d/%d MB)",
			r.MemUsedPct, r.MemTotalMB-r.MemAvailMB, r.MemTotalMB))
	}
	if mem.SwapUsedMB > 1024 {
		r.Alerts = append(r.Alerts, fmt.Sprintf("swap high: %d/%d MB", mem.SwapUsedMB, mem.SwapTotalMB))
	}
	if len(r.Orphans) > 0 {
		r.Alerts = append(r.Alerts, fmt.Sprintf("%d orphaned process(es) detected", len(r.Orphans)))
	}
	if len(r.Zombies) > 0 {
		r.Alerts = append(r.Alerts, fmt.Sprintf("%d zombie process(es) detected", len(r.Zombies)))
	}

	return r, nil
}

// LogJSONL appends r as a JSON line to path, creating parent dirs if needed.
func LogJSONL(r *Report, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// ---- internal helpers ----

func matchedName(procName string) string {
	lower := strings.ToLower(procName)
	for _, n := range watchedNames {
		if strings.Contains(lower, n) {
			return n
		}
	}
	return ""
}

// listProcesses runs ps and returns all visible processes.
func listProcesses() ([]ProcessInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid,ppid,rss,stat,comm").Output()
	if err != nil {
		return nil, err
	}
	var procs []ProcessInfo
	sc := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			first = false
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		rss, _ := strconv.ParseInt(fields[2], 10, 64)
		state := fields[3]
		name := filepath.Base(fields[4])
		procs = append(procs, ProcessInfo{
			PID:   pid,
			PPID:  ppid,
			RSSMB: rss / 1024,
			State: state,
			Name:  name,
		})
	}
	return procs, sc.Err()
}

// systemMemory reads macOS hw.memsize, vm_stat, and vm.swapusage.
func systemMemory() (SystemMemory, error) {
	var m SystemMemory

	totalBytes, err := sysctlInt64("hw.memsize")
	if err != nil {
		return m, fmt.Errorf("hw.memsize: %w", err)
	}
	m.TotalMB = totalBytes / 1024 / 1024

	pageSize, availPages, err := vmStatAvail()
	if err != nil {
		return m, fmt.Errorf("vm_stat: %w", err)
	}
	m.AvailMB = (availPages * pageSize) / 1024 / 1024

	usedMB, totalSwapMB, err := swapUsage()
	if err == nil {
		m.SwapUsedMB = usedMB
		m.SwapTotalMB = totalSwapMB
	}
	return m, nil
}

func sysctlInt64(name string) (int64, error) {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

// vmStatAvail returns (pageSize, availablePages, error).
// Available = free + inactive + speculative — the macOS Activity Monitor definition.
func vmStatAvail() (pageSize, availPages int64, err error) {
	out, errRun := exec.Command("vm_stat").Output()
	if errRun != nil {
		return 0, 0, errRun
	}
	var freePages, inactivePages, specPages int64
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		// Header: "Mach Virtual Memory Statistics: (page size of 16384 bytes)"
		if strings.Contains(line, "page size of") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "size" && i+2 < len(parts) {
					pageSize, _ = strconv.ParseInt(parts[i+2], 10, 64)
				}
			}
			continue
		}
		val := parseVMStatLine(line)
		switch {
		case strings.HasPrefix(line, "Pages free:"):
			freePages = val
		case strings.HasPrefix(line, "Pages inactive:"):
			inactivePages = val
		case strings.HasPrefix(line, "Pages speculative:"):
			specPages = val
		}
	}
	if pageSize == 0 {
		pageSize = 4096 // Intel fallback
	}
	return pageSize, freePages + inactivePages + specPages, sc.Err()
}

// parseVMStatLine extracts the integer from a line like "Pages free:   514487."
func parseVMStatLine(line string) int64 {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return 0
	}
	raw := strings.TrimRight(strings.TrimSpace(line[colon+1:]), ".")
	v, _ := strconv.ParseInt(raw, 10, 64)
	return v
}

// swapUsage parses sysctl vm.swapusage.
// Format: "total = 10240.00M  used = 8736.81M  free = 1503.19M  (encrypted)"
func swapUsage() (usedMB, totalMB int64, err error) {
	out, err := exec.Command("sysctl", "-n", "vm.swapusage").Output()
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if i+2 >= len(fields) {
			break
		}
		switch f {
		case "total":
			totalMB = parseMB(fields[i+2])
		case "used":
			usedMB = parseMB(fields[i+2])
		}
	}
	return usedMB, totalMB, nil
}

func parseMB(s string) int64 {
	s = strings.TrimSuffix(s, "M")
	f, _ := strconv.ParseFloat(s, 64)
	return int64(f)
}

func killProc(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}
