package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/trace"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	traceFilesWorkspace string
	traceFilesSince     string
	traceFilesJSON      bool
)

var traceFilesCmd = &cobra.Command{
	Use:   "trace-files <file1> [file2] ...",
	Short: "Find which sessions modified specific files",
	Long: `Trace file provenance by searching history.jsonl for conversations that modified files.

This command helps you understand which AGM sessions worked on specific files by searching
through Claude history for file modification records. Useful for:
  • Understanding file provenance and context
  • Finding the session that introduced a change
  • Recovering lost context for file modifications

Examples:
  agm admin trace-files ~/src/project/README.md
  agm admin trace-files file1.go file2.go --since 2024-02-01
  agm admin trace-files *.md --workspace oss --json

Output shows:
  • Session UUID and name
  • Timestamps of each modification
  • Workspace (if applicable)

Notes:
  • Searches all workspace history.jsonl files
  • Handles corrupted history gracefully (skips bad entries)
  • Supports exact and substring path matching
  • Shows orphaned sessions (no manifest) as "<no manifest>"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runTraceFiles,
}

func init() {
	adminCmd.AddCommand(traceFilesCmd)

	traceFilesCmd.Flags().StringVar(&traceFilesWorkspace, "workspace", "",
		"Filter results to specific workspace")
	traceFilesCmd.Flags().StringVar(&traceFilesSince, "since", "",
		"Only show modifications after this date (RFC3339 format: 2024-02-19T10:00:00Z)")
	traceFilesCmd.Flags().BoolVar(&traceFilesJSON, "json", false,
		"Output results in JSON format")
}

func runTraceFiles(cmd *cobra.Command, args []string) error {
	// Get sessions directory
	sessionsDir := cfg.SessionsDir
	if sessionsDir == "" {
		homeDir, _ := os.UserHomeDir()
		sessionsDir = homeDir + "/sessions"
	}

	// Parse --since flag if provided
	var sinceTime *time.Time
	if traceFilesSince != "" {
		parsed, err := time.Parse(time.RFC3339, traceFilesSince)
		if err != nil {
			return fmt.Errorf("invalid --since date format (expected RFC3339, e.g., 2024-02-19T10:00:00Z): %w", err)
		}
		sinceTime = &parsed
	}

	// Create tracer
	tracer := trace.NewTracer(sessionsDir)

	// Trace files
	opts := trace.TraceOptions{
		FilePaths:   args,
		Since:       sinceTime,
		Workspace:   traceFilesWorkspace,
		SessionsDir: sessionsDir,
	}

	results, err := tracer.TraceFiles(opts)
	if err != nil {
		return fmt.Errorf("trace failed: %w", err)
	}

	// Output results
	if traceFilesJSON {
		return outputTraceJSON(results)
	}

	return outputTraceHuman(results, opts)
}

// outputTraceJSON outputs trace results in JSON format
func outputTraceJSON(results []*trace.TraceResult) error {
	// Convert to JSON-friendly format
	output := struct {
		Files []struct {
			Path     string `json:"path"`
			Sessions []struct {
				UUID          string `json:"uuid"`
				Name          string `json:"name"`
				Workspace     string `json:"workspace,omitempty"`
				Modifications []struct {
					Timestamp string `json:"timestamp"`
				} `json:"modifications"`
			} `json:"sessions"`
		} `json:"files"`
	}{}

	output.Files = make([]struct {
		Path     string `json:"path"`
		Sessions []struct {
			UUID          string `json:"uuid"`
			Name          string `json:"name"`
			Workspace     string `json:"workspace,omitempty"`
			Modifications []struct {
				Timestamp string `json:"timestamp"`
			} `json:"modifications"`
		} `json:"sessions"`
	}, len(results))

	for i, result := range results {
		output.Files[i].Path = result.FilePath
		output.Files[i].Sessions = make([]struct {
			UUID          string `json:"uuid"`
			Name          string `json:"name"`
			Workspace     string `json:"workspace,omitempty"`
			Modifications []struct {
				Timestamp string `json:"timestamp"`
			} `json:"modifications"`
		}, len(result.Sessions))

		for j, session := range result.Sessions {
			output.Files[i].Sessions[j].UUID = session.SessionID
			output.Files[i].Sessions[j].Name = session.SessionName
			output.Files[i].Sessions[j].Workspace = session.Workspace
			output.Files[i].Sessions[j].Modifications = make([]struct {
				Timestamp string `json:"timestamp"`
			}, len(session.Modifications))

			for k, mod := range session.Modifications {
				output.Files[i].Sessions[j].Modifications[k].Timestamp = mod.Timestamp.Format(time.RFC3339)
			}
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// outputTraceHuman outputs trace results in human-readable format
func outputTraceHuman(results []*trace.TraceResult, opts trace.TraceOptions) error {
	// Header
	fmt.Println(ui.Blue("═══ File Provenance Trace ═══"))
	fmt.Println()

	// Show filters if any
	if opts.Workspace != "" {
		fmt.Printf("Workspace filter: %s\n", opts.Workspace)
	}
	if opts.Since != nil {
		fmt.Printf("Since: %s\n", opts.Since.Format("2006-01-02 15:04:05"))
	}
	if opts.Workspace != "" || opts.Since != nil {
		fmt.Println()
	}

	// Track if we found any results
	foundAny := false

	for _, result := range results {
		if len(result.Sessions) == 0 {
			printNoSessions(result.FilePath, opts.Workspace)
			continue
		}
		foundAny = true
		printTraceTable(result)
	}

	// Summary
	if !foundAny {
		fmt.Println(ui.Yellow("No file modifications found in history."))
		fmt.Println()
		fmt.Println("Possible reasons:")
		fmt.Println("  • Files were never modified in tracked sessions")
		fmt.Println("  • history.jsonl doesn't contain file modification records")
		fmt.Println("  • File paths don't match (try absolute paths)")
		if opts.Since != nil {
			fmt.Println("  • All modifications were before --since date")
		}
	} else {
		totalSessions := 0
		for _, result := range results {
			totalSessions += len(result.Sessions)
		}
		fmt.Printf(ui.Green("✓ Found %d session(s) across %d file(s)\n"), totalSessions, len(results))
	}

	return nil
}

// printNoSessions prints the "no sessions found" line for a single file in
// human-readable trace output, qualifying with the workspace filter if set.
func printNoSessions(filePath, workspace string) {
	fmt.Println(ui.Yellow("File: " + filePath))
	if workspace != "" {
		fmt.Printf("  %s\n", ui.Yellow("No sessions found (in workspace "+workspace+")"))
	} else {
		fmt.Printf("  %s\n", ui.Yellow("No sessions found"))
	}
	fmt.Println()
}

// printTraceTable prints the per-file table of sessions and modification
// timestamps for human-readable trace output.
func printTraceTable(result *trace.TraceResult) {
	fmt.Println(ui.Green("File: " + result.FilePath))
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Session UUID\tSession Name\tWorkspace\tModifications\t")
	fmt.Fprintln(w, "------------\t------------\t---------\t-------------\t")

	for _, session := range result.Sessions {
		workspace := session.Workspace
		if workspace == "" {
			workspace = "-"
		}
		var timestamps []string
		for _, mod := range session.Modifications {
			timestamps = append(timestamps, mod.Timestamp.Format("2006-01-02 15:04:05"))
		}
		if len(timestamps) > 0 {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n",
				session.SessionID,
				session.SessionName,
				workspace,
				timestamps[0])
		}
		for i := 1; i < len(timestamps); i++ {
			fmt.Fprintf(w, "\t\t\t%s\t\n", timestamps[i])
		}
	}
	w.Flush()
	fmt.Println()
}
