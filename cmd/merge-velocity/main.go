// Command merge-velocity records merge-pipeline throughput as OTel metrics.
//
// It queries the GitHub API for a repository, computes:
//   - merges_per_day: PRs merged in the last 24 h
//   - open_pr_count: live open PR count
//   - median_time_to_merge_hours: median hours from open to merge (last 20 PRs)
//   - created_vs_merged_delta: PRs created/day − merged/day (positive = growing backlog)
//
// and emits them via the existing internal/telemetry OTel infrastructure.
//
// Usage:
//
//	merge-velocity --repo owner/repo [--otlp-endpoint host:port]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	repo := flag.String("repo", "", "Repository in owner/repo format (required)")
	endpoint := flag.String("otlp-endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "OTLP gRPC endpoint (host:port)")
	flag.Parse()

	if *repo == "" {
		flag.Usage()
		return fmt.Errorf("merge-velocity: --repo is required")
	}

	shutdown, err := telemetry.InitMeter("merge-velocity", telemetry.WithEndpoint(*endpoint))
	if err != nil {
		return fmt.Errorf("init meter: %w", err)
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		if err := shutdown(flushCtx); err != nil {
			log.Printf("metrics shutdown: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	data, err := telemetry.CollectMergeVelocity(ctx, *repo, "")
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}

	telemetry.RecordMergeVelocity(ctx, *repo, data)

	fmt.Printf("repo:                       %s\n", *repo)
	fmt.Printf("merges_per_day:             %.1f\n", data.MergesPerDay)
	fmt.Printf("open_pr_count:              %d\n", data.OpenPRCount)
	fmt.Printf("median_time_to_merge_hours: %.2f\n", data.MedianTimeToMergeHours)
	fmt.Printf("created_vs_merged_delta:    %.1f\n", data.CreatedVsMergedDelta)
	return nil
}
