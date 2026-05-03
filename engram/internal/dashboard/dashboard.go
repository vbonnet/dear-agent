// Package dashboard provides dashboard-related functionality.
package dashboard

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry/agent"
)

// DisplayOptions configures dashboard display.
type DisplayOptions struct {
	Metric string    // "all", "specificity", "examples", "efficiency", "trends"
	Since  time.Time // Filter launches since this date
	Until  time.Time // Filter launches until this date
	Format string    // "table", "csv", "json"
	DBPath string    // Telemetry database path
}

// Display shows dashboard metrics.
func Display(opts DisplayOptions) error {
	// Validate metric selection first (before database operations)
	validMetrics := map[string]bool{
		"all": true, "specificity": true, "examples": true, "efficiency": true, "trends": true,
	}
	if !validMetrics[opts.Metric] {
		return fmt.Errorf("invalid metric: %s (use all|specificity|examples|efficiency|trends)", opts.Metric)
	}

	// Open database
	storage, err := openStorage(opts.DBPath)
	if err != nil {
		return err
	}
	defer storage.Close()

	// Check for data
	if err := checkDataExists(storage); err != nil {
		return err
	}

	// Display metrics based on selection
	ctx := context.Background()
	switch opts.Metric {
	case "all":
		return displayAllMetrics(ctx, storage, opts)
	case "specificity":
		return displaySpecificity(ctx, storage, opts)
	case "examples":
		return displayExamples(ctx, storage, opts)
	case "efficiency":
		return displayEfficiency(ctx, storage, opts)
	case "trends":
		return displayTrends(ctx, storage, opts)
	default:
		return fmt.Errorf("invalid metric: %s (use all|specificity|examples|efficiency|trends)", opts.Metric)
	}
}

// openStorage opens the telemetry database.
func openStorage(path string) (*agent.Storage, error) {
	var storage *agent.Storage
	var err error

	if path == "" {
		storage, err = agent.NewStorage()
	} else {
		storage, err = agent.NewStorageAt(path)
	}

	if err != nil {
		// Check if file not found
		if os.IsNotExist(err) || (path != "" && !fileExists(path)) {
			dbPath := path
			if dbPath == "" {
				dbPath = agent.DefaultDatabasePath()
			}
			//nolint:revive // multi-line CLI-facing help text
			return nil, fmt.Errorf("telemetry database not found: %s\n\nRun 'engram-telemetry' to initialize telemetry collection.", dbPath)
		}
		return nil, fmt.Errorf("failed to open telemetry database: %w", err)
	}

	return storage, nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// checkDataExists verifies the database has data.
func checkDataExists(storage *agent.Storage) error {
	ctx := context.Background()
	stats, err := storage.Stats(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to check database stats: %w", err)
	}

	if stats.Total == 0 {
		//nolint:revive // multi-line CLI-facing help text
		return fmt.Errorf("no agent launches found in telemetry database.\n\nRun 'engram-telemetry' to start collecting data.")
	}

	return nil
}

// displayAllMetrics displays all 4 metrics.
func displayAllMetrics(ctx context.Context, storage *agent.Storage, opts DisplayOptions) error {
	// Display header
	if opts.Format == "table" || opts.Format == "markdown" {
		fmt.Println("Agent Success Metrics")
		fmt.Println("=====================")
		fmt.Println()
	}

	// Display each metric
	if err := displaySpecificity(ctx, storage, opts); err != nil {
		return err
	}

	if err := displayExamples(ctx, storage, opts); err != nil {
		return err
	}

	if err := displayEfficiency(ctx, storage, opts); err != nil {
		return err
	}

	if err := displayTrends(ctx, storage, opts); err != nil {
		return err
	}

	return nil
}

// displaySpecificity displays success rate by specificity.
func displaySpecificity(ctx context.Context, storage *agent.Storage, opts DisplayOptions) error {
	metrics, err := QuerySuccessBySpecificity(ctx, storage.DB(), opts.Since, opts.Until)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		if opts.Format == "table" || opts.Format == "markdown" {
			fmt.Println("No data available for specificity metrics.")
			fmt.Println()
		}
		return nil
	}

	output, err := FormatSpecificityTable(metrics, opts.Format)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

// displayExamples displays success rate by example presence.
func displayExamples(ctx context.Context, storage *agent.Storage, opts DisplayOptions) error {
	metrics, err := QuerySuccessByExamples(ctx, storage.DB(), opts.Since, opts.Until)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		if opts.Format == "table" || opts.Format == "markdown" {
			fmt.Println("No data available for example metrics.")
			fmt.Println()
		}
		return nil
	}

	output, err := FormatExampleTable(metrics, opts.Format)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

// displayEfficiency displays token efficiency metrics.
func displayEfficiency(ctx context.Context, storage *agent.Storage, opts DisplayOptions) error {
	metrics, err := QueryTokenEfficiency(ctx, storage.DB(), opts.Since, opts.Until)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		if opts.Format == "table" || opts.Format == "markdown" {
			fmt.Println("No data available for efficiency metrics.")
			fmt.Println()
		}
		return nil
	}

	output, err := FormatEfficiencyTable(metrics, opts.Format)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

// displayTrends displays trends over time.
func displayTrends(ctx context.Context, storage *agent.Storage, opts DisplayOptions) error {
	metrics, err := QueryTrendsOverTime(ctx, storage.DB(), opts.Since, opts.Until)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		if opts.Format == "table" || opts.Format == "markdown" {
			fmt.Println("No data available for trend metrics.")
			fmt.Println()
		}
		return nil
	}

	output, err := FormatTrendTable(metrics, opts.Format)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}
