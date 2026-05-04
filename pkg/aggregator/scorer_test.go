package aggregator

import (
	"math"
	"testing"
	"time"
)

func TestScorerDefaults(t *testing.T) {
	t.Parallel()
	at := time.Now()
	signals := []Signal{
		{Kind: KindSecurityAlerts, Subject: "GO-1", Value: 1, CollectedAt: at},
		{Kind: KindLintTrend, Subject: "a.go", Value: 100, CollectedAt: at},
		{Kind: KindTestCoverage, Subject: "pkg/x", Value: 70, CollectedAt: at},
	}
	scorer := Scorer{}
	scores := scorer.Score(signals)
	if len(scores) != 3 {
		t.Fatalf("Score returned %d entries, want 3", len(scores))
	}
	// Security has weight 1.0 and norm 1/10 = 0.1 → weighted 0.1.
	// Lint has weight 0.4 and norm 100/200 = 0.5 → weighted 0.2.
	// Coverage has weight 0.5 and norm 1 - 70/100 = 0.3 → weighted 0.15.
	// So lint > coverage > security on weighted score.
	if scores[0].Kind != KindLintTrend {
		t.Errorf("top score = %s, want lint_trend (weighted 0.2)", scores[0].Kind)
	}
	total := scorer.Total(scores)
	if math.Abs(total-(0.1+0.2+0.15)) > 1e-9 {
		t.Errorf("Total = %f, want %f", total, 0.45)
	}
}

func TestScorerMonotone(t *testing.T) {
	t.Parallel()
	at := time.Now()
	scorer := Scorer{}
	low := scorer.Score([]Signal{
		{Kind: KindLintTrend, Subject: "a.go", Value: 10, CollectedAt: at},
	})
	high := scorer.Score([]Signal{
		{Kind: KindLintTrend, Subject: "a.go", Value: 100, CollectedAt: at},
	})
	if low[0].Weighted >= high[0].Weighted {
		t.Errorf("monotone violation: low weighted %f, high weighted %f",
			low[0].Weighted, high[0].Weighted)
	}
}

func TestScorerCoverageInverted(t *testing.T) {
	t.Parallel()
	at := time.Now()
	scorer := Scorer{}
	highCov := scorer.Score([]Signal{
		{Kind: KindTestCoverage, Subject: "x", Value: 95, CollectedAt: at},
	})
	lowCov := scorer.Score([]Signal{
		{Kind: KindTestCoverage, Subject: "x", Value: 5, CollectedAt: at},
	})
	if lowCov[0].Weighted <= highCov[0].Weighted {
		t.Errorf("coverage drop should raise pressure: low cov weighted %f, high cov weighted %f",
			lowCov[0].Weighted, highCov[0].Weighted)
	}
}

func TestScorerWeightOverrides(t *testing.T) {
	t.Parallel()
	at := time.Now()
	signals := []Signal{
		{Kind: KindGitActivity, Subject: "/r", Value: 50, CollectedAt: at},
	}
	def := Scorer{}.Score(signals)
	custom := Scorer{Weights: map[Kind]float64{KindGitActivity: 1.0}}.Score(signals)
	if custom[0].Weighted <= def[0].Weighted {
		t.Errorf("weight override should raise weighted score: default %f, custom %f",
			def[0].Weighted, custom[0].Weighted)
	}
}

func TestScorerClampingCeiling(t *testing.T) {
	t.Parallel()
	at := time.Now()
	scorer := Scorer{}
	// 5000 commits should clamp to ceiling (100); norm == 1.
	scores := scorer.Score([]Signal{
		{Kind: KindGitActivity, Subject: "/r", Value: 5000, CollectedAt: at},
	})
	if math.Abs(scores[0].Norm-1) > 1e-9 {
		t.Errorf("Norm = %f, want clamped to 1", scores[0].Norm)
	}
}

func TestScorerEmptyInput(t *testing.T) {
	t.Parallel()
	scores := Scorer{}.Score(nil)
	if scores == nil {
		t.Error("Score(nil) should return non-nil empty slice for predictable JSON")
	}
	if len(scores) != 0 {
		t.Errorf("Score(nil) length = %d, want 0", len(scores))
	}
}
