package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os/exec"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MergeVelocityData holds a point-in-time snapshot of merge-pipeline throughput
// for one repository. The rolling/snapshot distinction matters: merges_per_day and
// created_per_day are 24-hour rates, open_pr_count is a live snapshot, and
// median_time_to_merge is derived from the last 20 merged PRs.
type MergeVelocityData struct {
	MergesPerDay           float64 // PRs merged in the last 24 h
	OpenPRCount            int64   // live open PR count
	MedianTimeToMergeHours float64 // median time-to-merge over last 20 PRs
	CreatedVsMergedDelta   float64 // (created/day) - (merged/day): positive → backlog growing
}

// MergeVelocityMetrics holds OTel instruments for merge-velocity signals.
type MergeVelocityMetrics struct {
	mergesPerDay         metric.Float64ObservableGauge
	openPRCount          metric.Int64ObservableGauge
	medianTimeToMerge    metric.Float64Histogram
	createdVsMergedDelta metric.Float64ObservableGauge

	mu   sync.Mutex
	last map[string]MergeVelocityData // keyed by repo
}

var (
	mergeVelOnce    sync.Once
	mergeVelMetrics *MergeVelocityMetrics
)

// MergeVelocity returns the process-wide MergeVelocityMetrics, creating
// instruments on first call. Call after InitMeter so instruments bind to the
// real provider.
func MergeVelocity() *MergeVelocityMetrics {
	mergeVelOnce.Do(func() {
		mergeVelMetrics = newMergeVelocityMetrics(otel.GetMeterProvider())
	})
	return mergeVelMetrics
}

func newMergeVelocityMetrics(mp metric.MeterProvider) *MergeVelocityMetrics {
	m := mp.Meter(instrumentationScope)
	mv := &MergeVelocityMetrics{
		last: make(map[string]MergeVelocityData),
	}

	// Gauges are observable: they pull the last recorded value at collection time.
	mv.mergesPerDay, _ = m.Float64ObservableGauge(
		"merge.velocity.merges_per_day",
		metric.WithDescription("PRs merged in the rolling last 24 hours"),
		metric.WithUnit("{pr}/d"),
	)
	mv.openPRCount, _ = m.Int64ObservableGauge(
		"merge.velocity.open_pr_count",
		metric.WithDescription("Current count of open PRs (snapshot)"),
		metric.WithUnit("{pr}"),
	)
	mv.createdVsMergedDelta, _ = m.Float64ObservableGauge(
		"merge.velocity.created_vs_merged_delta",
		metric.WithDescription("PRs created/day minus merged/day; positive means backlog is growing"),
		metric.WithUnit("{pr}/d"),
	)

	// Histogram is push-based: each RecordMergeVelocity call appends a sample.
	mv.medianTimeToMerge, _ = m.Float64Histogram(
		"merge.velocity.median_time_to_merge_hours",
		metric.WithDescription("Median hours from PR open to merge, computed over last 20 merged PRs"),
		metric.WithUnit("h"),
	)

	// Register the observable callback for the three gauges.
	_, _ = m.RegisterCallback(
		func(_ context.Context, obs metric.Observer) error {
			mv.mu.Lock()
			snapshot := make(map[string]MergeVelocityData, len(mv.last))
			maps.Copy(snapshot, mv.last)
			mv.mu.Unlock()
			for repo, d := range snapshot {
				attr := metric.WithAttributes(attribute.String("repo", repo))
				obs.ObserveFloat64(mv.mergesPerDay, d.MergesPerDay, attr)
				obs.ObserveInt64(mv.openPRCount, d.OpenPRCount, attr)
				obs.ObserveFloat64(mv.createdVsMergedDelta, d.CreatedVsMergedDelta, attr)
			}
			return nil
		},
		mv.mergesPerDay,
		mv.openPRCount,
		mv.createdVsMergedDelta,
	)

	return mv
}

// RecordMergeVelocity records a merge-velocity snapshot for repo. It updates
// the observable gauges (merges_per_day, open_pr_count, created_vs_merged_delta)
// and appends the median time-to-merge as a histogram sample.
//
// Safe to call with the default no-op provider (before InitMeter or when no
// endpoint is configured).
func RecordMergeVelocity(ctx context.Context, repo string, data MergeVelocityData) {
	mv := MergeVelocity()
	if mv == nil {
		return
	}
	mv.mu.Lock()
	mv.last[repo] = data
	mv.mu.Unlock()

	if mv.medianTimeToMerge != nil {
		mv.medianTimeToMerge.Record(ctx, data.MedianTimeToMergeHours,
			metric.WithAttributes(attribute.String("repo", repo)))
	}
}

// ghPR is the minimal shape we need from the GitHub API list-pulls response.
type ghPR struct {
	Number    int    `json:"number"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
	MergedAt  string `json:"merged_at"`
	ClosedAt  string `json:"closed_at"`
}

// CollectMergeVelocity queries the GitHub API via the gh CLI and returns a
// MergeVelocityData snapshot for repo (format: "owner/repo"). token is unused
// when the gh CLI is authenticated; pass "" to rely on gh's own auth.
//
// Metrics collected:
//   - MergesPerDay: PRs whose merged_at falls in the last 24 h
//   - OpenPRCount: live open PR count
//   - MedianTimeToMergeHours: median open→merged duration for the last 20 PRs
//   - CreatedVsMergedDelta: PRs created in last 24 h minus merged in last 24 h
func CollectMergeVelocity(ctx context.Context, repo, _ string) (MergeVelocityData, error) {
	now := time.Now().UTC()
	cutoff24h := now.Add(-24 * time.Hour)

	// Fetch recent PRs: closed (merged) + open. We page through closed PRs to
	// find those merged in the last 24 h and collect up to 20 for the median.
	closed, err := fetchPRs(ctx, repo, "closed", 100)
	if err != nil {
		return MergeVelocityData{}, fmt.Errorf("merge-velocity: fetch closed PRs for %s: %w", repo, err)
	}
	open, err := fetchPRs(ctx, repo, "open", 100)
	if err != nil {
		return MergeVelocityData{}, fmt.Errorf("merge-velocity: fetch open PRs for %s: %w", repo, err)
	}

	var (
		mergedLast24h  int64
		createdLast24h int64
		mergeDurations []float64
	)

	for _, pr := range closed {
		created, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		if created.After(cutoff24h) {
			createdLast24h++
		}
		if pr.MergedAt == "" {
			continue
		}
		merged, err := time.Parse(time.RFC3339, pr.MergedAt)
		if err != nil {
			continue
		}
		if merged.After(cutoff24h) {
			mergedLast24h++
		}
		// Collect merge durations for the most recent 20 merged PRs.
		if len(mergeDurations) < 20 {
			mergeDurations = append(mergeDurations, merged.Sub(created).Hours())
		}
	}

	for _, pr := range open {
		created, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		if created.After(cutoff24h) {
			createdLast24h++
		}
	}

	median := medianFloat64(mergeDurations)

	return MergeVelocityData{
		MergesPerDay:           float64(mergedLast24h),
		OpenPRCount:            int64(len(open)),
		MedianTimeToMergeHours: median,
		CreatedVsMergedDelta:   float64(createdLast24h) - float64(mergedLast24h),
	}, nil
}

// fetchPRs calls `gh api` to list PRs for repo in the given state.
func fetchPRs(ctx context.Context, repo, state string, perPage int) ([]ghPR, error) {
	path := fmt.Sprintf("repos/%s/pulls?state=%s&per_page=%d&sort=updated&direction=desc", repo, state, perPage)
	out, err := exec.CommandContext(ctx, "gh", "api", path).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %w", path, err)
	}
	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("unmarshal PRs: %w", err)
	}
	return prs, nil
}

// medianFloat64 returns the median of a sorted copy of vs. Returns 0 for an
// empty slice. NaN values in vs are excluded before the median is computed.
func medianFloat64(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	clean := make([]float64, 0, len(vs))
	for _, v := range vs {
		if !math.IsNaN(v) {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return 0
	}
	sort.Float64s(clean)
	n := len(clean)
	if n%2 == 1 {
		return clean[n/2]
	}
	return (clean[n/2-1] + clean[n/2]) / 2
}
