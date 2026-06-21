package main

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionType partitions usage into the two cohorts the supervisor mesh cares
// about. The whole point of ce-bkxa is to make supervisor burn visible BEFORE
// an Opus upgrade lifts it, so the worker/supervisor split is first-class.
const (
	SessionWorker     = "worker"
	SessionSupervisor = "supervisor"
)

// supervisorMarkers are substrings that, when present in a session name, mark
// the session as a supervisor rather than a worker. Matched case-insensitively.
// These mirror the supervisor.Role vocabulary ("meta-orchestrator",
// "orchestrator", "overseer") plus the umbrella word "supervisor".
var supervisorMarkers = []string{
	"supervisor",
	"overseer",
	"orchestrator",
	"meta-orch",
}

// classifySessionType maps a session name to SessionWorker or SessionSupervisor.
// Unknown / empty names default to worker: a misclassified worker is harmless,
// whereas silently dropping a session would hide burn.
func classifySessionType(name string) string {
	n := strings.ToLower(name)
	for _, m := range supervisorMarkers {
		if strings.Contains(n, m) {
			return SessionSupervisor
		}
	}
	return SessionWorker
}

// Pricing is the per-million-token cost used to estimate spend for records that
// carry no explicit costUSD field (the newer Claude Code transcript format
// records token counts but not cost). Defaults approximate Claude Opus list
// pricing; override via flags for other model tiers.
type Pricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// DefaultPricing approximates Claude Opus list pricing (USD per 1M tokens).
var DefaultPricing = Pricing{InputPerMTok: 15.0, OutputPerMTok: 75.0}

// estimateCost derives a USD cost from token counts using p.
func (p Pricing) estimateCost(inputTokens, outputTokens int64) float64 {
	return float64(inputTokens)/1e6*p.InputPerMTok +
		float64(outputTokens)/1e6*p.OutputPerMTok
}

// rawRecord is the subset of a Claude Code transcript line we consume. The
// transcript has shifted formats over time: older lines carried a top-level
// costUSD plus a top-level usage object; newer lines nest usage under message
// and omit cost. We tolerate both.
type rawRecord struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Slug      string      `json:"slug"`
	SessionID string      `json:"sessionId"`
	CostUSD   *float64    `json:"costUSD"`
	Usage     *rawUsage   `json:"usage"`
	Message   *rawMessage `json:"message"`
}

type rawMessage struct {
	Usage *rawUsage `json:"usage"`
}

type rawUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// usage returns the effective token usage for the record, preferring the nested
// message.usage (current format) and falling back to a top-level usage (legacy).
func (r rawRecord) usage() (input, output int64, ok bool) {
	if r.Message != nil && r.Message.Usage != nil {
		u := r.Message.Usage
		if u.InputTokens != 0 || u.OutputTokens != 0 {
			return u.InputTokens, u.OutputTokens, true
		}
	}
	if r.Usage != nil && (r.Usage.InputTokens != 0 || r.Usage.OutputTokens != 0) {
		return r.Usage.InputTokens, r.Usage.OutputTokens, true
	}
	return 0, 0, false
}

// Usage accumulates token and cost totals for one cohort.
type Usage struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

func (u *Usage) add(input, output int64, cost float64) {
	u.InputTokens += input
	u.OutputTokens += output
	u.CostUSD += cost
}

// Result is the aggregated view emitted as metrics and printed as a summary.
type Result struct {
	ByType map[string]*Usage `json:"by_type"`
	Total  Usage             `json:"total"`

	// WindowCostUSD is the cost incurred within BurnWindow ending at the
	// evaluation time — the basis for the burn-rate projection.
	WindowCostUSD float64       `json:"window_cost_usd"`
	BurnWindow    time.Duration `json:"burn_window"`

	// Projected24hUSD extrapolates WindowCostUSD to a full 24h.
	Projected24hUSD float64 `json:"projected_24h_usd"`

	FilesScanned   int `json:"files_scanned"`
	RecordsCounted int `json:"records_counted"`
}

// burnProjection extrapolates a window cost to 24h. A zero or negative window
// yields zero so a misconfigured flag can never fabricate an alert.
func burnProjection(windowCost float64, window time.Duration) float64 {
	if window <= 0 {
		return 0
	}
	return windowCost * (24 * float64(time.Hour) / float64(window))
}

// aggregate walks records, partitions them by session type, and computes the
// burn-rate projection from records whose timestamp falls within [now-window, now].
//
// Records are already priced at parse time (explicit costUSD when present, else
// an estimate); aggregate only sums and partitions them.
func aggregate(records []parsedRecord, now time.Time, window time.Duration) Result {
	res := Result{
		ByType:     map[string]*Usage{},
		BurnWindow: window,
	}
	windowStart := now.Add(-window)
	for _, rec := range records {
		bucket := res.ByType[rec.SessionType]
		if bucket == nil {
			bucket = &Usage{}
			res.ByType[rec.SessionType] = bucket
		}
		bucket.add(rec.InputTokens, rec.OutputTokens, rec.CostUSD)
		res.Total.add(rec.InputTokens, rec.OutputTokens, rec.CostUSD)
		res.RecordsCounted++

		if !rec.Timestamp.IsZero() && !rec.Timestamp.Before(windowStart) && !rec.Timestamp.After(now) {
			res.WindowCostUSD += rec.CostUSD
		}
	}
	res.Projected24hUSD = burnProjection(res.WindowCostUSD, window)
	return res
}

// parsedRecord is one normalized, priced usage line.
type parsedRecord struct {
	SessionType  string
	Timestamp    time.Time
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// parseLine decodes one JSONL line into a parsedRecord. ok is false for lines
// that carry no usage (user turns, summaries, tool results), which are skipped.
func parseLine(line []byte, fallbackName string, pricing Pricing) (parsedRecord, bool) {
	var raw rawRecord
	if err := json.Unmarshal(line, &raw); err != nil {
		return parsedRecord{}, false
	}
	input, output, ok := raw.usage()
	if !ok {
		return parsedRecord{}, false
	}

	name := raw.Slug
	if name == "" {
		name = fallbackName
	}

	cost := pricing.estimateCost(input, output)
	if raw.CostUSD != nil {
		cost = *raw.CostUSD
	}

	return parsedRecord{
		SessionType:  classifySessionType(name),
		Timestamp:    raw.Timestamp,
		InputTokens:  input,
		OutputTokens: output,
		CostUSD:      cost,
	}, true
}

// scanFile parses every usage-bearing line in one transcript file.
func scanFile(path string, pricing Pricing) ([]parsedRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Transcript lines can be large (full message bodies); grow the scanner
	// buffer well past bufio's 64KiB default so long lines aren't dropped.
	fallbackName := sessionNameFromPath(path)
	var out []parsedRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if rec, ok := parseLine(sc.Bytes(), fallbackName, pricing); ok {
			out = append(out, rec)
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// sessionNameFromPath derives a fallback session name from a transcript path
// when the line itself has no slug. The Claude Code layout is
// ~/.claude/projects/<project-dir>/<session-uuid>.jsonl; the project dir
// encodes the working directory and is the best name signal available.
func sessionNameFromPath(path string) string {
	return filepath.Base(filepath.Dir(path))
}

// ScanProjects walks projectsDir for *.jsonl transcripts and returns every
// priced usage record plus the count of files scanned. A read error on one
// file is skipped rather than aborting the whole scan: a monitor must keep
// reporting the sessions it can see.
func ScanProjects(projectsDir string, pricing Pricing) ([]parsedRecord, int, error) {
	// A missing root is a real misconfiguration worth surfacing; WalkDir would
	// otherwise swallow it because the callback skips per-entry errors.
	if _, err := os.Stat(projectsDir); err != nil {
		return nil, 0, err
	}

	var files []string
	err := filepath.WalkDir(projectsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries; a monitor reports what it can see
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)

	var all []parsedRecord
	for _, f := range files {
		recs, _ := scanFile(f, pricing) //nolint:errcheck // best-effort: skip a file we can't read mid-scan
		all = append(all, recs...)
	}
	return all, len(files), nil
}
