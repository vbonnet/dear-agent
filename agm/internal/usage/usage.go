// Package usage implements path-2 Claude Code usage tracking: it parses the
// JSONL conversation transcripts that Claude Code writes under
// ~/.claude/projects/**/*.jsonl and aggregates per-message token counts into
// USD cost, burn rate, and threshold alerts.
//
// This is the "after-the-fact" path — it works immediately with no shell/env
// changes (unlike the OTel path, which requires CLAUDE_CODE_ENABLE_TELEMETRY=1).
package usage

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

// Pricing is the per-million-token USD price for a model tier.
type Pricing struct {
	InputPerM      float64
	OutputPerM     float64
	CacheReadPerM  float64
	CacheWritePerM float64
}

// Approximate Anthropic public pricing (USD per million tokens). Cache-read is
// ~0.1x input and 5-minute cache-write is ~1.25x input, following Anthropic's
// published multipliers. These are deliberately approximate — the JSONL records
// no price, so cost here is an estimate for quota visibility, not billing.
var (
	opusPricing   = Pricing{InputPerM: 15.00, OutputPerM: 75.00, CacheReadPerM: 1.50, CacheWritePerM: 18.75}
	opus5Pricing  = pricingFromCosttrack("claude-opus-5")
	sonnetPricing = Pricing{InputPerM: 3.00, OutputPerM: 15.00, CacheReadPerM: 0.30, CacheWritePerM: 3.75}
	haikuPricing  = Pricing{InputPerM: 1.00, OutputPerM: 5.00, CacheReadPerM: 0.10, CacheWritePerM: 1.25}
	// Fable pricing is not public; treat it at the Opus frontier tier as a
	// conservative upper-bound estimate.
	fablePricing = Pricing{InputPerM: 15.00, OutputPerM: 75.00, CacheReadPerM: 1.50, CacheWritePerM: 18.75}
)

// pricingFromCosttrack adapts the canonical per-million-token pricing to the
// transcript estimator's local representation.
func pricingFromCosttrack(model string) Pricing {
	p := costtrack.PricingTable[model]
	return Pricing{
		InputPerM:      p.Input,
		OutputPerM:     p.Output,
		CacheReadPerM:  p.CacheRead,
		CacheWritePerM: p.CacheWrite,
	}
}

// PriceFor returns the pricing tier for a model id by substring match. Unknown
// models (including "<synthetic>") price at zero so they never inflate cost.
func PriceFor(model string) Pricing {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus-5"):
		return opus5Pricing
	case strings.Contains(m, "opus"):
		return opusPricing
	case strings.Contains(m, "sonnet"):
		return sonnetPricing
	case strings.Contains(m, "haiku"):
		return haikuPricing
	case strings.Contains(m, "fable"):
		return fablePricing
	default:
		return Pricing{}
	}
}

// Tokens holds a token count broken out by category.
type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

// Total returns the sum across all token categories.
func (t Tokens) Total() int64 { return t.Input + t.Output + t.CacheRead + t.CacheWrite }

// Add accumulates another token count into the receiver.
func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
}

// Cost computes the USD cost of a token count at the given pricing.
func (t Tokens) Cost(p Pricing) float64 {
	const perM = 1_000_000.0
	return float64(t.Input)/perM*p.InputPerM +
		float64(t.Output)/perM*p.OutputPerM +
		float64(t.CacheRead)/perM*p.CacheReadPerM +
		float64(t.CacheWrite)/perM*p.CacheWritePerM
}

// Entry is one assistant message's usage, parsed from a JSONL line.
type Entry struct {
	Timestamp   time.Time
	Model       string
	SessionID   string
	Cwd         string
	RequestID   string
	MessageID   string
	IsSidechain bool
	Tokens      Tokens
}

// Cost returns the estimated USD cost of this entry at its model's pricing.
func (e Entry) Cost() float64 { return e.Tokens.Cost(PriceFor(e.Model)) }

// SessionType classifies an entry as "worker" or "supervisor". AGM workers run
// inside sandboxes (cwd under ~/.agm/sandboxes/); everything else (interactive
// supervision in ~/src or worktrees) is treated as supervisor. This is a
// heuristic — the JSONL carries no AGM session role.
func (e Entry) SessionType() string {
	if strings.Contains(e.Cwd, "/.agm/sandboxes/") {
		return "worker"
	}
	return "supervisor"
}

// rawLine is the subset of a JSONL record needed for usage accounting.
type rawLine struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd"`
	RequestID   string `json:"requestId"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input       int64 `json:"input_tokens"`
			Output      int64 `json:"output_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// DefaultRoot returns the default Claude Code projects directory.
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/projects"
	}
	return filepath.Join(home, ".claude", "projects")
}

// maxLineBytes bounds the scanner buffer. JSONL lines embed thinking signatures
// and tool results and can be large, so allow up to 32 MiB per line.
const maxLineBytes = 32 * 1024 * 1024

// Collect walks root for *.jsonl files and returns every assistant usage Entry
// whose timestamp is at or after since. Entries are de-duplicated by
// (requestId, messageId) so retried/resumed transcripts are not double-counted.
// Files whose mtime predates since are skipped wholesale as an optimization.
func Collect(root string, since time.Time) ([]Entry, error) {
	var entries []Entry
	seen := make(map[string]struct{})

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable subtrees rather than aborting the whole walk.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if info, statErr := d.Info(); statErr == nil && info.ModTime().Before(since) {
			return nil
		}
		// collectFile tolerates unreadable/corrupt files internally, returning
		// whatever it managed to parse, so a single bad file never aborts the walk.
		entries = append(entries, collectFile(path, since, seen)...)
		return nil
	})
	if walkErr != nil {
		return entries, walkErr
	}
	return entries, nil
}

// collectFile parses one JSONL file and returns its billable entries. Errors
// (unopenable file, oversized/garbled lines) are swallowed: the function returns
// whatever it parsed before the error so one bad file never derails the walk.
func collectFile(path string, since time.Time, seen map[string]struct{}) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		e, ok := parseEntry(sc.Bytes(), since)
		if !ok {
			continue
		}
		// De-dup across files: a resumed session replays prior messages.
		if e.RequestID != "" || e.MessageID != "" {
			key := e.RequestID + "|" + e.MessageID
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, e)
	}
	return out
}

// parseEntry decodes one JSONL line into an Entry. It returns ok=false for any
// line that is not a billable assistant message at or after since (non-assistant
// records, zero-usage turns, malformed JSON, unparseable or too-old timestamps).
func parseEntry(line []byte, since time.Time) (Entry, bool) {
	if len(line) == 0 {
		return Entry{}, false
	}
	var rl rawLine
	if err := json.Unmarshal(line, &rl); err != nil {
		return Entry{}, false
	}
	if rl.Type != "assistant" {
		return Entry{}, false
	}
	u := rl.Message.Usage
	if u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheCreate == 0 {
		return Entry{}, false
	}
	ts, err := time.Parse(time.RFC3339, rl.Timestamp)
	if err != nil {
		return Entry{}, false
	}
	if ts.Before(since) {
		return Entry{}, false
	}
	return Entry{
		Timestamp:   ts,
		Model:       rl.Message.Model,
		SessionID:   rl.SessionID,
		Cwd:         rl.Cwd,
		RequestID:   rl.RequestID,
		MessageID:   rl.Message.ID,
		IsSidechain: rl.IsSidechain,
		Tokens: Tokens{
			Input:      u.Input,
			Output:     u.Output,
			CacheRead:  u.CacheRead,
			CacheWrite: u.CacheCreate,
		},
	}, true
}

// Bucket aggregates tokens, cost, and message count for one grouping key.
type Bucket struct {
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
	Messages int     `json:"messages"`
}

// Report is the aggregated usage summary produced by Build.
type Report struct {
	WindowHours       float64           `json:"window_hours"`
	TotalCostUSD      float64           `json:"total_cost_usd"`
	TotalTokens       int64             `json:"total_tokens"`
	BurnRateDailyUSD  float64           `json:"burn_rate_daily_usd"`
	TopModel          string            `json:"top_model"`
	Alert             bool              `json:"alert"`
	AlertThresholdUSD float64           `json:"alert_threshold_usd,omitempty"`
	Models            map[string]Bucket `json:"models"`
	Sessions          map[string]Bucket `json:"sessions"`
	MessageCount      int               `json:"message_count"`
}

// Build aggregates entries into a Report. windowHours drives the burn-rate
// projection (cost over the window, scaled to 24h). If alertThreshold > 0 the
// report's Alert flag is set when projected daily cost exceeds it.
func Build(entries []Entry, windowHours, alertThreshold float64) Report {
	models := make(map[string]Bucket)
	sessions := make(map[string]Bucket)
	var total Tokens
	var totalCost float64

	for _, e := range entries {
		cost := e.Cost()
		totalCost += cost
		total.Add(e.Tokens)

		model := e.Model
		if model == "" {
			model = "unknown"
		}
		mb := models[model]
		mb.Tokens += e.Tokens.Total()
		mb.CostUSD += cost
		mb.Messages++
		models[model] = mb

		st := e.SessionType()
		sb := sessions[st]
		sb.Tokens += e.Tokens.Total()
		sb.CostUSD += cost
		sb.Messages++
		sessions[st] = sb
	}

	// Round cost fields to cents to avoid float noise in output.
	for k, b := range models {
		b.CostUSD = round2(b.CostUSD)
		models[k] = b
	}
	for k, b := range sessions {
		b.CostUSD = round2(b.CostUSD)
		sessions[k] = b
	}

	var burn float64
	if windowHours > 0 {
		burn = totalCost / windowHours * 24.0
	}

	r := Report{
		WindowHours:      windowHours,
		TotalCostUSD:     round2(totalCost),
		TotalTokens:      total.Total(),
		BurnRateDailyUSD: round2(burn),
		TopModel:         topModel(models),
		Models:           models,
		Sessions:         sessions,
		MessageCount:     len(entries),
	}
	if alertThreshold > 0 {
		r.AlertThresholdUSD = alertThreshold
		r.Alert = burn > alertThreshold
	}
	return r
}

// topModel returns the model id with the highest cost.
func topModel(models map[string]Bucket) string {
	type kv struct {
		k string
		c float64
	}
	var list []kv
	for k, b := range models {
		list = append(list, kv{k, b.CostUSD})
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].c != list[j].c {
			return list[i].c > list[j].c
		}
		return list[i].k < list[j].k // stable tiebreak
	})
	return list[0].k
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
