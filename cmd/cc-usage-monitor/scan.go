package main

import (
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

// SessionType classifies a Claude Code session by the role of the agent that
// produced its transcript: a "worker" executing a task vs a "supervisor"
// orchestrating other sessions. Unrecognised sessions are "unknown" rather
// than being silently folded into either bucket.
type SessionType string

const (
	SessionWorker     SessionType = "worker"
	SessionSupervisor SessionType = "supervisor"
	SessionUnknown    SessionType = "unknown"
)

// supervisorMarkers are substrings that identify an orchestration session. They
// take precedence over workerMarkers because a supervisor may run from a path
// that also contains "worker" (e.g. "vroom-orchestrator-worker-1").
var supervisorMarkers = []string{"orchestrator", "overseer", "supervisor", "meta-o"}

// workerMarkers identify a task-executing session. "sandbox" is included
// because agm spawns workers inside ~/.agm/sandboxes/<id>.
var workerMarkers = []string{"worker", "sandbox"}

// ClassifySessionType maps a free-form session identifier (an agm session name,
// a transcript cwd, or a project-directory name) to a SessionType. Supervisor
// markers win over worker markers; an empty or unrecognised name is
// SessionUnknown.
func ClassifySessionType(name string) SessionType {
	n := strings.ToLower(name)
	for _, m := range supervisorMarkers {
		if strings.Contains(n, m) {
			return SessionSupervisor
		}
	}
	for _, m := range workerMarkers {
		if strings.Contains(n, m) {
			return SessionWorker
		}
	}
	return SessionUnknown
}

// classifyBest classifies from cwd first, falling back to the project-dir hint
// when cwd is unrecognised. This keeps the strongest available signal.
func classifyBest(cwd, dirHint string) SessionType {
	if t := ClassifySessionType(cwd); t != SessionUnknown {
		return t
	}
	return ClassifySessionType(dirHint)
}

// rawEntry is the subset of a Claude Code transcript line we care about. The
// transcript is JSONL: one self-contained JSON object per line. Token usage and
// the model live under message; costUSD (when present at all — newer CC builds
// omit it) is top-level.
type rawEntry struct {
	Type      string   `json:"type"`
	Timestamp string   `json:"timestamp"`
	Cwd       string   `json:"cwd"`
	SessionID string   `json:"sessionId"`
	CostUSD   *float64 `json:"costUSD"`
	Message   struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Entry is one normalised, costed usage record from a transcript line.
type Entry struct {
	Model      string
	Type       SessionType
	Timestamp  time.Time
	Input      int64
	Output     int64
	CacheWrite int64
	CacheRead  int64
	CostUSD    float64
}

// computeCost estimates USD cost from token counts using the shared costtrack
// pricing table. Unknown models price at zero (costtrack default) rather than
// guessing. cache_creation maps to CacheWrite, cache_read to CacheRead.
func computeCost(model string, in, out, cacheWrite, cacheRead int64) float64 {
	p := costtrack.GetPricingOrDefault(model) // already per-token
	return float64(in)*p.Input +
		float64(out)*p.Output +
		float64(cacheWrite)*p.CacheWrite +
		float64(cacheRead)*p.CacheRead
}

// parseLine turns one JSONL line into a costed Entry. It returns ok=false for
// blank lines, malformed JSON, and lines without token usage (user turns,
// summaries, tool results). cwd/dirHint drive session-type classification, and
// costUSD is honoured when the line carries it, else cost is computed from
// tokens.
func parseLine(line []byte, dirHint string) (Entry, bool) {
	line = trimSpace(line)
	if len(line) == 0 {
		return Entry{}, false
	}
	var raw rawEntry
	if err := json.Unmarshal(line, &raw); err != nil {
		return Entry{}, false
	}
	if raw.Message.Usage == nil {
		return Entry{}, false
	}
	u := raw.Message.Usage

	e := Entry{
		Model:      raw.Message.Model,
		Type:       classifyBest(raw.Cwd, dirHint),
		Input:      u.InputTokens,
		Output:     u.OutputTokens,
		CacheWrite: u.CacheCreationInputTokens,
		CacheRead:  u.CacheReadInputTokens,
	}
	if raw.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
			e.Timestamp = ts.UTC()
		}
	}
	if raw.CostUSD != nil {
		e.CostUSD = *raw.CostUSD
	} else {
		e.CostUSD = computeCost(e.Model, e.Input, e.Output, e.CacheWrite, e.CacheRead)
	}
	return e, true
}

// trimSpace trims leading/trailing ASCII whitespace without allocating.
func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 {
		c := b[len(b)-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}

// Aggregate accumulates token counts and cost for a group of entries.
type Aggregate struct {
	Input      int64   `json:"input_tokens"`
	Output     int64   `json:"output_tokens"`
	CacheWrite int64   `json:"cache_creation_tokens"`
	CacheRead  int64   `json:"cache_read_tokens"`
	CostUSD    float64 `json:"cost_usd"`
	Messages   int64   `json:"messages"`
}

func (a *Aggregate) add(e Entry) {
	a.Input += e.Input
	a.Output += e.Output
	a.CacheWrite += e.CacheWrite
	a.CacheRead += e.CacheRead
	a.CostUSD += e.CostUSD
	a.Messages++
}

// Report is the aggregated view over all scanned transcripts.
type Report struct {
	ByType   map[SessionType]*Aggregate `json:"by_type"`
	ByModel  map[string]*Aggregate      `json:"by_model"`
	Total    Aggregate                  `json:"total"`
	Earliest time.Time                  `json:"earliest,omitzero"`
	Latest   time.Time                  `json:"latest,omitzero"`
	Files    int                        `json:"files"`
}

func newReport() *Report {
	return &Report{
		ByType:  map[SessionType]*Aggregate{},
		ByModel: map[string]*Aggregate{},
	}
}

func (r *Report) add(e Entry) {
	bt := r.ByType[e.Type]
	if bt == nil {
		bt = &Aggregate{}
		r.ByType[e.Type] = bt
	}
	bt.add(e)

	model := e.Model
	if model == "" {
		model = "unknown"
	}
	bm := r.ByModel[model]
	if bm == nil {
		bm = &Aggregate{}
		r.ByModel[model] = bm
	}
	bm.add(e)

	r.Total.add(e)

	if !e.Timestamp.IsZero() {
		if r.Earliest.IsZero() || e.Timestamp.Before(r.Earliest) {
			r.Earliest = e.Timestamp
		}
		if e.Timestamp.After(r.Latest) {
			r.Latest = e.Timestamp
		}
	}
}

// ScanReader parses every line of r, adding usage entries to report and
// appending them to entries (returned for burn-rate analysis). dirHint feeds
// session classification when a line lacks a recognisable cwd.
func ScanReader(r io.Reader, dirHint string, report *Report, entries *[]Entry) error {
	sc := bufio.NewScanner(r)
	// Transcript lines can be large (full message content); raise the cap well
	// above the 64 KiB default to avoid bufio.ErrTooLong on big turns.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if e, ok := parseLine(sc.Bytes(), dirHint); ok {
			report.add(e)
			if entries != nil {
				*entries = append(*entries, e)
			}
		}
	}
	return sc.Err()
}

// ScanDir walks root for *.jsonl transcripts and returns the aggregated report
// plus the flat entry list. The immediate parent directory name of each file is
// used as the classification hint (Claude Code names project dirs after the
// session's working directory). Per-file read errors are skipped so one
// unreadable transcript never aborts the whole scan.
func ScanDir(root string) (*Report, []Entry, error) {
	report := newReport()
	var entries []Entry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable subtrees
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path) //nolint:gosec // G122: transcript root is a trusted, local, user-owned directory
		if err != nil {
			return nil
		}
		defer f.Close()
		dirHint := filepath.Base(filepath.Dir(path))
		if scanErr := ScanReader(f, dirHint, report, &entries); scanErr == nil {
			report.Files++
		}
		return nil
	})
	return report, entries, err
}

// ProjectedDailyCost extrapolates spend within the trailing window to a 24-hour
// burn rate: (cost in [now-window, now]) / window-hours * 24. Entries outside
// the window are ignored, so a long-idle history does not depress the estimate.
// A non-positive window yields 0.
func ProjectedDailyCost(entries []Entry, now time.Time, window time.Duration) float64 {
	if window <= 0 {
		return 0
	}
	cutoff := now.Add(-window)
	var cost float64
	for _, e := range entries {
		if e.Timestamp.IsZero() {
			continue
		}
		if e.Timestamp.After(cutoff) && !e.Timestamp.After(now) {
			cost += e.CostUSD
		}
	}
	hours := window.Hours()
	if hours <= 0 {
		return 0
	}
	return cost / hours * 24.0
}
