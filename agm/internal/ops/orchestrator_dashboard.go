package ops

import (
	"fmt"
	"sort"
	"time"
)

// OrchestratorDashboardRequest defines input for the orchestrator dashboard.
type OrchestratorDashboardRequest struct {
	// Window for throughput metrics (defaults to 1 hour).
	Window time.Duration `json:"window,omitempty"`
}

// OrchestratorTrustSummary shows top and bottom performers.
type OrchestratorTrustSummary struct {
	Top    []TrustLeaderboardEntry `json:"top"`
	Bottom []TrustLeaderboardEntry `json:"bottom"`
	Total  int                     `json:"total"`
}

// OrchestratorDashboardResult contains unified orchestrator view.
type OrchestratorDashboardResult struct {
	Operation string                   `json:"operation"`
	Timestamp string                   `json:"timestamp"`
	Sessions  SessionMetrics           `json:"sessions"`
	Metrics   ThroughputMetrics        `json:"throughput"`
	Resources ResourceMetrics          `json:"resources"`
	Alerts    []Alert                  `json:"alerts"`
	Trust     OrchestratorTrustSummary `json:"trust"`
}

// OrchestratorDashboard returns a unified view for orchestrators.
func OrchestratorDashboard(ctx *OpContext, req *OrchestratorDashboardRequest) (*OrchestratorDashboardResult, error) {
	if req == nil {
		req = &OrchestratorDashboardRequest{
			Window: time.Hour,
		}
	}

	result := &OrchestratorDashboardResult{
		Operation: "orchestrator_dashboard",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// 1. Get system metrics
	metricsReq := &MetricsRequest{Window: req.Window}
	metricsResult, err := GetMetrics(ctx, metricsReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}
	result.Sessions = metricsResult.Sessions
	result.Metrics = metricsResult.Throughput
	result.Resources = metricsResult.Resources
	result.Alerts = metricsResult.Alerts

	// 2. Get trust leaderboard
	trustResult, err := TrustLeaderboard(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get trust leaderboard: %w", err)
	}
	result.Trust = extractTrustSummary(trustResult)

	return result, nil
}

// extractTrustSummary returns top 5 and bottom 5 from leaderboard.
func extractTrustSummary(lb *TrustLeaderboardResult) OrchestratorTrustSummary {
	summary := OrchestratorTrustSummary{
		Top:    []TrustLeaderboardEntry{},
		Bottom: []TrustLeaderboardEntry{},
		Total:  len(lb.Entries),
	}

	if len(lb.Entries) == 0 {
		return summary
	}

	// Sort descending by score
	sorted := make([]TrustLeaderboardEntry, len(lb.Entries))
	copy(sorted, lb.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// Top 5
	top := 5
	if len(sorted) < 5 {
		top = len(sorted)
	}
	summary.Top = sorted[:top]

	// Bottom 5
	bottom := 5
	if len(sorted) < 5 {
		bottom = len(sorted)
	}
	if len(sorted) > bottom {
		summary.Bottom = sorted[len(sorted)-bottom:]
	}

	return summary
}
