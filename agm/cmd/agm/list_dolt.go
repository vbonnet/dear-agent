package main

import (
	"fmt"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
)

var (
	listJSON    bool
	listAll     bool
	listTrust   bool
	listTestEnv string
	listTags    []string
	listFilters []string
)

// listCmdDolt is the Dolt-backed version of the list command
var listCmdDolt = &cobra.Command{
	Use:   "list",
	Short: "List AGM sessions from Dolt database",
	Long: `List AGM sessions from Dolt database.

By default, shows only running sessions (active tmux). Stopped and archived
sessions are hidden to reduce noise from stale OFFLINE sessions.
Use --all to show all sessions including stopped and archived.

Examples:
  agm session list                         # List running sessions only
  agm session list --all                   # List all sessions (stopped + archived)
  agm session list --json                  # Output as JSON
  agm session list --filter role:worker    # Filter by role tag
  agm session list --tag cap:claude-code   # Filter by capability tag`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load named test environment if --test-env flag is set
		if listTestEnv != "" {
			tc := testcontext.LoadNamed(listTestEnv)
			if err := tc.SetEnv(); err != nil {
				return fmt.Errorf("failed to activate test environment %q: %w", listTestEnv, err)
			}
		}

		// Construct OpContext with storage
		opCtx, cleanup, err := newOpContextWithStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer cleanup()

		// Determine status filter
		status := "active"
		if listAll {
			status = "all"
		}

		// Merge --filter and --tag values; both filter by context tags
		tags := append(append([]string(nil), listTags...), listFilters...)

		// Call ops layer
		// By default, hide stopped (OFFLINE) sessions to reduce noise.
		// Use --all to see everything including stopped and archived.
		result, err := ops.ListSessions(opCtx, &ops.ListSessionsRequest{
			Status:         status,
			Tags:           tags,
			Limit:          1000,
			ExcludeStopped: !listAll,
		})
		if err != nil {
			return handleError(err)
		}

		if len(result.Sessions) == 0 {
			if !listAll {
				ui.PrintWarning("No running sessions found")
				fmt.Println("\nUse --all to see stopped and archived sessions")
			} else {
				ui.PrintWarning("No sessions found")
				fmt.Println("\nCreate your first session with: agm session new")
			}
			return nil
		}

		// Output using cliframe
		writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())

		if listJSON {
			// Use cliframe JSON formatter
			formatter, fmtErr := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
			if fmtErr != nil {
				return fmtErr
			}
			writer = writer.WithFormatter(formatter)
			return writer.Output(result)
		}

		// For table output, use ops result summaries
		// Print a simple table from SessionSummary data
		printSessionSummaryTable(cmd, result.Sessions, listTrust)

		// Show orphan tmux sessions if any
		if len(result.OrphanTmuxSessions) > 0 {
			fmt.Fprintln(cmd.OutOrStdout())
			ui.PrintWarning("Orphan tmux sessions (no AGM counterpart):")
			for _, name := range result.OrphanTmuxSessions {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", name)
			}
		}
		return nil
	},
}

func init() {
	// Register with session command
	sessionCmd.AddCommand(listCmdDolt)
	listCmdDolt.Flags().BoolVar(&listAll, "all", false, "show all sessions including stopped and archived")
	listCmdDolt.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
	listCmdDolt.Flags().StringSliceVar(&listTags, "tag", nil, "filter by context tag (repeatable, e.g., --tag role:worker)")
	listCmdDolt.Flags().StringSliceVar(&listFilters, "filter", nil, "filter by context tag (alias for --tag, e.g., --filter role:worker)")
	listCmdDolt.Flags().BoolVar(&listTrust, "trust", false, "show trust score column")
	listCmdDolt.Flags().StringVar(&listTestEnv, "test-env", "", "Use named test environment")
}

// shortStatus maps session status and attachment to compact display icons.
func shortStatus(s ops.SessionSummary) string {
	switch s.Status {
	case "active":
		if s.Attached {
			return "●" // active & attached
		}
		return "◐" // active & detached
	case "stopped":
		return "○"
	default:
		return "?"
	}
}

// shortHarness maps harness names to compact display codes.
func shortHarness(harness string) string {
	switch harness {
	case "claude-code":
		return "cc"
	case "gemini-cli":
		return "gem"
	case "codex-cli":
		return "cdx"
	case "opencode-cli":
		return "oc"
	default:
		return harness
	}
}

// compactProject replaces the home directory prefix with ~/ and truncates
// long paths with an ellipsis in the middle to preserve both prefix and suffix.
func compactProject(project string) string {
	if project == "" {
		return ""
	}
	// Replace home directory with ~/
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		project = strings.Replace(project, u.HomeDir, "~", 1)
	}
	project = filepath.ToSlash(project)

	const maxLen = 36
	if len(project) <= maxLen {
		return project
	}

	// Keep prefix and suffix, join with ellipsis
	half := (maxLen - 1) / 2 // -1 for the …
	return project[:half] + "…" + project[len(project)-half:]
}

// printSessionSummaryTable prints a compact table of session summaries.
func printSessionSummaryTable(cmd *cobra.Command, sessions []ops.SessionSummary, showTrust bool) {
	out := cmd.OutOrStdout()

	// Sort alphabetically by name
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	// Legend on separate lines
	fmt.Fprintln(out, "Status  (S): ●=active & attached  ◐=active & detached  ○=stopped")
	fmt.Fprintln(out, "Harness (H): cc=claude  gem=gemini  cdx=codex  oc=opencode")
	fmt.Fprintln(out)

	// Header
	if showTrust {
		fmt.Fprintf(out, "%-28s %s %-3s %5s %-24s %s\n",
			"NAME", "S", "H", "TRUST", "PROJECT", "TAGS")
		fmt.Fprintf(out, "%-28s %s %-3s %5s %-24s %s\n",
			"---", "-", "--", "-----", "-------", "----")
	} else {
		fmt.Fprintf(out, "%-28s %s %-3s %-24s %s\n",
			"NAME", "S", "H", "PROJECT", "TAGS")
		fmt.Fprintf(out, "%-28s %s %-3s %-24s %s\n",
			"---", "-", "--", "-------", "----")
	}

	for _, s := range sessions {
		name := s.Name
		if len(name) > 27 {
			name = name[:24] + "..."
		}
		project := compactProject(s.Project)
		tags := strings.Join(s.Tags, ",")
		if len(tags) > 32 {
			tags = tags[:29] + "..."
		}
		if showTrust {
			trustScore := lookupTrustScore(s.Name)
			fmt.Fprintf(out, "%-28s %s %-3s %5d %-24s %s\n",
				name, shortStatus(s), shortHarness(s.Harness), trustScore, project, tags)
		} else {
			fmt.Fprintf(out, "%-28s %s %-3s %-24s %s\n",
				name, shortStatus(s), shortHarness(s.Harness), project, tags)
		}
	}
}

// lookupTrustScore returns the trust score for a session, defaulting to the
// base score if no trust data exists.
func lookupTrustScore(sessionName string) int {
	result, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: sessionName})
	if err != nil {
		return 50 // base score fallback
	}
	return result.Score
}
