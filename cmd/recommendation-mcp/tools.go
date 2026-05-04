// tools.go implements the three MCP tools that ADR-016 ships:
// get_signals, get_recommendations, get_signal_trends.
//
// Every tool is read-only. The server never writes; collection is the
// aggregator's job.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// Per-tool caps prevent a misconfigured client from over-fetching. See
// ADR-016 §D2 / §D3 / §D4 for the rationale (one JSON-RPC envelope per
// response, stdio buffer + client render budgets).
const (
	defaultSignalsLimit = 100
	maxSignalsLimit     = 1000

	defaultTopN = 10
	maxTopN     = 50

	defaultRecommendationWindow = 7 * 24 * time.Hour  // 7 days
	defaultTrendWindow          = 30 * 24 * time.Hour // 30 days
	defaultTrendBucket          = 24 * time.Hour
	minTrendBucket              = time.Hour

	maxTrendBuckets = 1000
)

// toolDescriptors returns the tools/list payload. Schema fields are
// kept conservative so the same tool shape works across MCP versions.
func toolDescriptors() []map[string]any {
	return []map[string]any{
		{
			"name": "get_signals",
			"description": "Query collected signals. Filter by kind, subject (substring), and " +
				"time window. Returns up to 1000 rows.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type":        "string",
						"description": "signal kind (git_activity, lint_trend, test_coverage, dep_freshness, security_alerts); omit to query all kinds",
					},
					"subject": map[string]any{
						"type":        "string",
						"description": "substring match against signal Subject (file path, package, dep, vuln id)",
					},
					"since": map[string]any{
						"type":        "string",
						"description": "RFC3339 lower bound on collected_at",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "max rows (default 100, max 1000)",
					},
				},
			},
		},
		{
			"name": "get_recommendations",
			"description": "Ranked priority list across all signal kinds within a window. " +
				"Reduces to most-recent per (kind, subject) and applies the weighted scorer.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"top_n": map[string]any{
						"type":        "integer",
						"description": "max recommendations (default 10, max 50)",
					},
					"window": map[string]any{
						"type":        "string",
						"description": "Go duration string for the lookback window (default 168h = 7 days)",
					},
					"weights": map[string]any{
						"type":                 "object",
						"description":          "optional override of per-kind weights; missing keys fall back to DefaultWeights",
						"additionalProperties": map[string]any{"type": "number"},
					},
				},
			},
		},
		{
			"name":        "get_signal_trends",
			"description": "Time-bucketed aggregation for one signal kind. Empty buckets are emitted.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type":        "string",
						"description": "signal kind (required)",
					},
					"subject": map[string]any{
						"type":        "string",
						"description": "optional substring filter on Subject",
					},
					"window": map[string]any{
						"type":        "string",
						"description": "Go duration string for the lookback window (default 720h = 30 days)",
					},
					"bucket": map[string]any{
						"type":        "string",
						"description": "Go duration string for the bucket size (default 24h, min 1h)",
					},
				},
				"required": []string{"kind"},
			},
		},
	}
}

// signalToWire flattens an aggregator.Signal to the JSON-RPC result
// shape. RFC3339Nano keeps microsecond precision; clients that don't
// care about sub-second deltas just truncate.
func signalToWire(s aggregator.Signal) map[string]any {
	md := s.Metadata
	if md == "" {
		md = "{}"
	}
	return map[string]any{
		"id":          s.ID,
		"kind":        string(s.Kind),
		"subject":     s.Subject,
		"value":       s.Value,
		"metadata":    md,
		"collectedAt": s.CollectedAt.UTC().Format(time.RFC3339Nano),
	}
}

// parseDuration is the shared "optional Go-duration field" parser. It
// applies the default when raw is empty, rejects unparseable strings
// with -32602, and rejects non-positive durations the same way.
func parseDuration(id any, raw, field string, def time.Duration) (time.Duration, *rpcResponse) {
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		r := errResponse(id, -32602, "invalid '"+field+"' duration", err.Error())
		return 0, &r
	}
	if d <= 0 {
		r := errResponse(id, -32602, field+" must be positive", raw)
		return 0, &r
	}
	return d, nil
}

// validateKindStr maps a string to aggregator.Kind, returning -32602
// for unknown values. Empty input returns the zero Kind and nil — the
// caller decides whether empty is allowed.
func validateKindStr(id any, raw string) (aggregator.Kind, *rpcResponse) {
	if raw == "" {
		return "", nil
	}
	k := aggregator.Kind(raw)
	if err := k.Validate(); err != nil {
		r := errResponse(id, -32602, "unknown kind", err.Error())
		return "", &r
	}
	return k, nil
}

// ----- get_signals -----

type getSignalsArgs struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Since   string `json:"since"`
	Limit   int    `json:"limit"`
}

// parsedGetSignalsArgs is the validated form: every field is ready to
// hand to the store without further checks.
type parsedGetSignalsArgs struct {
	Kinds   []aggregator.Kind // empty when caller asked for all
	Subject string
	Since   time.Time // zero ⇒ no lower bound
	Limit   int
}

func (s *Server) parseGetSignalsArgs(ctx context.Context, id any, args json.RawMessage) (*parsedGetSignalsArgs, *rpcResponse) {
	var a getSignalsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		r := errResponse(id, -32602, "invalid arguments", err.Error())
		return nil, &r
	}
	if a.Limit <= 0 {
		a.Limit = defaultSignalsLimit
	}
	if a.Limit > maxSignalsLimit {
		r := errResponse(id, -32602, "limit exceeds max", maxSignalsLimit)
		return nil, &r
	}

	var since time.Time
	if a.Since != "" {
		t, err := time.Parse(time.RFC3339, a.Since)
		if err != nil {
			r := errResponse(id, -32602, "invalid 'since' timestamp", err.Error())
			return nil, &r
		}
		since = t
	}

	out := &parsedGetSignalsArgs{Subject: a.Subject, Since: since, Limit: a.Limit}
	if a.Kind != "" {
		k, errResp := validateKindStr(id, a.Kind)
		if errResp != nil {
			return nil, errResp
		}
		out.Kinds = []aggregator.Kind{k}
		return out, nil
	}
	ks, err := s.Store.Kinds(ctx)
	if err != nil {
		r := errResponse(id, -32000, "kinds", err.Error())
		return nil, &r
	}
	out.Kinds = ks
	return out, nil
}

func (s *Server) toolGetSignals(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	parsed, errResp := s.parseGetSignalsArgs(ctx, id, args)
	if errResp != nil {
		return *errResp
	}

	out := make([]map[string]any, 0, parsed.Limit)
	for _, k := range parsed.Kinds {
		rows, err := s.queryByKind(ctx, k, parsed.Since, parsed.Limit)
		if err != nil {
			return errResponse(id, -32000, "query", err.Error())
		}
		for _, r := range rows {
			if parsed.Subject != "" && !strings.Contains(r.Subject, parsed.Subject) {
				continue
			}
			out = append(out, signalToWire(r))
			if len(out) >= parsed.Limit {
				break
			}
		}
		if len(out) >= parsed.Limit {
			break
		}
	}

	// Echo the requested kind so a client driving multiple parallel
	// requests can correlate responses. Empty when fanning out across kinds.
	var kindEcho string
	if len(parsed.Kinds) == 1 {
		kindEcho = string(parsed.Kinds[0])
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"kind":    kindEcho,
		"signals": out,
	}}
}

// queryByKind picks Recent (no lower bound) vs Range (with lower
// bound). Branching here keeps toolGetSignals' loop body readable.
func (s *Server) queryByKind(ctx context.Context, k aggregator.Kind, since time.Time, limit int) ([]aggregator.Signal, error) {
	if since.IsZero() {
		return s.Store.Recent(ctx, k, limit)
	}
	return s.Store.Range(ctx, k, since)
}

// ----- get_recommendations -----

type getRecommendationsArgs struct {
	TopN    int                `json:"top_n"`
	Window  string             `json:"window"`
	Weights map[string]float64 `json:"weights"`
}

type parsedGetRecommendationsArgs struct {
	TopN    int
	Window  time.Duration
	Weights map[aggregator.Kind]float64
}

func parseGetRecommendationsArgs(id any, args json.RawMessage) (*parsedGetRecommendationsArgs, *rpcResponse) {
	var a getRecommendationsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		r := errResponse(id, -32602, "invalid arguments", err.Error())
		return nil, &r
	}
	if a.TopN <= 0 {
		a.TopN = defaultTopN
	}
	if a.TopN > maxTopN {
		r := errResponse(id, -32602, "top_n exceeds max", maxTopN)
		return nil, &r
	}
	window, errResp := parseDuration(id, a.Window, "window", defaultRecommendationWindow)
	if errResp != nil {
		return nil, errResp
	}
	weights, err := convertWeights(a.Weights)
	if err != nil {
		r := errResponse(id, -32602, "invalid weights", err.Error())
		return nil, &r
	}
	return &parsedGetRecommendationsArgs{TopN: a.TopN, Window: window, Weights: weights}, nil
}

func (s *Server) toolGetRecommendations(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	parsed, errResp := parseGetRecommendationsArgs(id, args)
	if errResp != nil {
		return *errResp
	}

	since := time.Now().Add(-parsed.Window)
	reduced, errResp := s.collectLatestPerSubject(ctx, id, since)
	if errResp != nil {
		return *errResp
	}

	scorer := aggregator.Scorer{Weights: parsed.Weights}
	scores := scorer.Score(reduced)
	if len(scores) > parsed.TopN {
		scores = scores[:parsed.TopN]
	}

	recs := make([]map[string]any, 0, len(scores))
	for _, sc := range scores {
		recs = append(recs, map[string]any{
			"kind":     string(sc.Kind),
			"subject":  sc.Subject,
			"raw":      sc.Raw,
			"norm":     sc.Norm,
			"weight":   sc.Weight,
			"weighted": sc.Weighted,
			"summary":  fmt.Sprintf("%s on %s (raw=%g)", sc.Kind, sc.Subject, sc.Raw),
		})
	}

	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"generated_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"window":          parsed.Window.String(),
		"total_score":     scorer.Total(scores),
		"recommendations": recs,
	}}
}

// collectLatestPerSubject implements the "most recent per (kind, subject)
// within the window" reduction. Store.Range returns rows ordered
// most-recent first, so the first time we see a (kind, subject) is
// the latest.
func (s *Server) collectLatestPerSubject(ctx context.Context, id any, since time.Time) ([]aggregator.Signal, *rpcResponse) {
	kinds, err := s.Store.Kinds(ctx)
	if err != nil {
		r := errResponse(id, -32000, "kinds", err.Error())
		return nil, &r
	}
	type key struct {
		k aggregator.Kind
		s string
	}
	latest := make(map[key]aggregator.Signal)
	for _, k := range kinds {
		rows, err := s.Store.Range(ctx, k, since)
		if err != nil {
			r := errResponse(id, -32000, "range", err.Error())
			return nil, &r
		}
		for _, r := range rows {
			kk := key{k: r.Kind, s: r.Subject}
			if _, seen := latest[kk]; !seen {
				latest[kk] = r
			}
		}
	}
	out := make([]aggregator.Signal, 0, len(latest))
	for _, sig := range latest {
		out = append(out, sig)
	}
	return out, nil
}

// convertWeights validates the input map and reshapes it to the typed
// form the scorer wants. An unknown kind is a -32602 — the client
// should fix its call rather than have weights silently ignored.
func convertWeights(in map[string]float64) (map[aggregator.Kind]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[aggregator.Kind]float64, len(in))
	for k, v := range in {
		kk := aggregator.Kind(k)
		if err := kk.Validate(); err != nil {
			return nil, err
		}
		out[kk] = v
	}
	return out, nil
}

// ----- get_signal_trends -----

type getSignalTrendsArgs struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Window  string `json:"window"`
	Bucket  string `json:"bucket"`
}

type parsedGetSignalTrendsArgs struct {
	Kind    aggregator.Kind
	Subject string
	Window  time.Duration
	Bucket  time.Duration
}

func parseGetSignalTrendsArgs(id any, args json.RawMessage) (*parsedGetSignalTrendsArgs, *rpcResponse) {
	var a getSignalTrendsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		r := errResponse(id, -32602, "invalid arguments", err.Error())
		return nil, &r
	}
	if a.Kind == "" {
		r := errResponse(id, -32602, "kind is required", nil)
		return nil, &r
	}
	k, errResp := validateKindStr(id, a.Kind)
	if errResp != nil {
		return nil, errResp
	}
	window, errResp := parseDuration(id, a.Window, "window", defaultTrendWindow)
	if errResp != nil {
		return nil, errResp
	}
	bucket, errResp := parseDuration(id, a.Bucket, "bucket", defaultTrendBucket)
	if errResp != nil {
		return nil, errResp
	}
	if bucket < minTrendBucket {
		r := errResponse(id, -32602, "bucket below minimum (1h)", bucket.String())
		return nil, &r
	}
	if int(window/bucket) > maxTrendBuckets {
		r := errResponse(id, -32602, "window/bucket exceeds 1000 buckets", map[string]any{
			"window": window.String(), "bucket": bucket.String(),
		})
		return nil, &r
	}
	return &parsedGetSignalTrendsArgs{
		Kind: k, Subject: a.Subject, Window: window, Bucket: bucket,
	}, nil
}

func (s *Server) toolGetSignalTrends(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	parsed, errResp := parseGetSignalTrendsArgs(id, args)
	if errResp != nil {
		return *errResp
	}

	since := time.Now().Add(-parsed.Window)
	rows, err := s.Store.Range(ctx, parsed.Kind, since)
	if err != nil {
		return errResponse(id, -32000, "range", err.Error())
	}
	if parsed.Subject != "" {
		filtered := rows[:0]
		for _, r := range rows {
			if strings.Contains(r.Subject, parsed.Subject) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	buckets := bucketize(since, parsed.Window, parsed.Bucket, rows)
	wireBuckets := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		wireBuckets = append(wireBuckets, map[string]any{
			"start": b.Start.UTC().Format(time.RFC3339),
			"end":   b.End.UTC().Format(time.RFC3339),
			"count": b.Count,
			"mean":  b.Mean,
			"min":   b.Min,
			"max":   b.Max,
		})
	}

	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"kind":    string(parsed.Kind),
		"window":  parsed.Window.String(),
		"bucket":  parsed.Bucket.String(),
		"buckets": wireBuckets,
	}}
}

