package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/user"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	listJSON        bool
	listAll         bool
	listTrust       bool
	listTestEnv     string
	listTags        []string
	listFilters     []string
	listLimit       int
	listOffset      int
	listStableOrder bool
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
  agm session list --all --limit 1000      # Paginate complete history
  agm session list --filter role:worker    # Filter by role tag
  agm session list --tag cap:claude-code   # Filter by capability tag`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateListPagination(listLimit, listOffset); err != nil {
			return err
		}
		// Load named test environment if --test-env flag is set
		if listTestEnv != "" {
			tc, err := testcontext.LoadNamed(listTestEnv)
			if err != nil {
				return fmt.Errorf("invalid test environment %q: %w", listTestEnv, err)
			}
			if err := tc.SetEnv(); err != nil {
				return fmt.Errorf("failed to activate test environment %q: %w", listTestEnv, err)
			}
		}

		ctx, span := startListSpan(cmd.Context(), listAll, len(listTags)+len(listFilters))
		defer span.End()

		// Construct OpContext with storage
		opCtx, cleanup, err := newOpContextWithStorage()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer cleanup()
		_ = ctx // span propagation to future sub-operations

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
			Limit:          listLimit,
			Offset:         listOffset,
			StableOrder:    listStableOrder,
			ExcludeStopped: !listAll,
		})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return handleError(err)
		}

		span.SetAttributes(attribute.Int("session.count", len(result.Sessions)))

		// JSON output: route through the central printJSON path so the global
		// --output json and --fields flags both work. --json is kept as a hidden
		// backward-compatible alias for --output json. Emit JSON even when the
		// result set is empty so programmatic consumers always get a parseable
		// object rather than a human-readable warning.
		if isJSONOutput() || listJSON {
			if outputMode == ModeAgent {
				applyAgentListDefaults(result, fieldsFlag)
			}
			return printListJSON(result)
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

		// For table output, use ops result summaries
		// Print a simple table from SessionSummary data. Show the OUTCOME column
		// with --all so the archive pile is triage-legible (archived rows carry
		// completed|crashed|killed|gc-stale instead of an indistinguishable '?').
		printSessionSummaryTable(cmd, result.Sessions, listTrust, listAll)

		// Show orphan tmux sessions if any
		if len(result.OrphanTmuxSessions) > 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			ui.PrintWarning("Orphan tmux sessions (no AGM counterpart):")
			for _, name := range result.OrphanTmuxSessions {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", name)
			}
		}
		return nil
	},
}

func validateListPagination(limit, offset int) error {
	if limit < 1 || limit > 1000 {
		return fmt.Errorf("--limit must be between 1 and 1000")
	}
	if offset < 0 {
		return fmt.Errorf("--offset must be non-negative")
	}
	return nil
}

var listTopLevelFields = map[string]bool{
	"operation":            true,
	"sessions":             true,
	"total":                true,
	"limit":                true,
	"offset":               true,
	"orphan_tmux_sessions": true,
}

var listSessionFields = map[string]bool{
	"id":             true,
	"name":           true,
	"status":         true,
	"outcome":        true,
	"attached":       true,
	"harness":        true,
	"workspace":      true,
	"project":        true,
	"tags":           true,
	"created_at":     true,
	"updated_at":     true,
	"estimated_cost": true,
}

// printListJSON keeps `agm session list --fields name,status,...` useful for
// agents. The global field-mask helper intentionally filters only top-level
// objects; list accepts per-session fields too, so row fields are applied inside
// the sessions envelope instead of collapsing the whole result to `{}`.
func printListJSON(result *ops.ListSessionsResult) error {
	if len(fieldsFlag) == 0 || !fieldsAreOnlySessionRows(fieldsFlag) {
		return printJSON(result)
	}

	rows := make([]map[string]any, 0, len(result.Sessions))
	for _, s := range result.Sessions {
		rows = append(rows, filterSessionSummaryFields(s, fieldsFlag))
	}
	return printJSONNoFieldMask(map[string]any{
		"sessions": rows,
		"total":    result.Total,
	})
}

func fieldsAreOnlySessionRows(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if listTopLevelFields[f] || !listSessionFields[f] {
			return false
		}
	}
	return true
}

func filterSessionSummaryFields(s ops.SessionSummary, fields []string) map[string]any {
	source := map[string]any{}
	data, err := json.Marshal(s)
	if err == nil {
		_ = json.Unmarshal(data, &source)
	}

	row := map[string]any{}
	for _, f := range fields {
		if v, ok := source[f]; ok {
			row[f] = v
		}
	}
	return row
}

func printJSONNoFieldMask(v any) error {
	return printJSONUnmasked(v)
}

// startListSpan starts an OTel span for the session list operation.
func startListSpan(ctx context.Context, all bool, filterCount int) (context.Context, trace.Span) {
	return otel.Tracer("agm").Start(ctx, "agm.session.list",
		trace.WithAttributes(
			attribute.String("operation", "list"),
			attribute.Bool("filter.all", all),
			attribute.Int("filter.tag_count", filterCount),
		))
}

func init() {
	// Register with session command
	sessionCmd.AddCommand(listCmdDolt)
	listCmdDolt.Flags().BoolVar(&listAll, "all", false, "show all sessions including stopped and archived")
	listCmdDolt.Flags().BoolVar(&listJSON, "json", false, "output as JSON (alias for --output json)")
	// --json is a backward-compatible alias for the global --output json flag.
	_ = listCmdDolt.Flags().MarkHidden("json")
	listCmdDolt.Flags().StringSliceVar(&listTags, "tag", nil, "filter by context tag (repeatable, e.g., --tag role:worker)")
	listCmdDolt.Flags().StringSliceVar(&listFilters, "filter", nil, "filter by context tag (alias for --tag, e.g., --filter role:worker)")
	listCmdDolt.Flags().BoolVar(&listTrust, "trust", false, "show trust score column")
	listCmdDolt.Flags().StringVar(&listTestEnv, "test-env", "", "Use named test environment")
	listCmdDolt.Flags().IntVar(&listLimit, "limit", 1000, "maximum sessions to return (1-1000)")
	listCmdDolt.Flags().IntVar(&listOffset, "offset", 0, "sessions to skip for pagination")
	listCmdDolt.Flags().BoolVar(&listStableOrder, "stable-order", false, "order by immutable creation key for reliable offset pagination")
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
	case "archived":
		return "▪"
	default:
		return "?"
	}
}

// outcomeCell renders the per-row OUTCOME column value. Non-archived sessions
// have no outcome, shown as "-".
func outcomeCell(s ops.SessionSummary) string {
	if s.Outcome == "" {
		return "-"
	}
	return s.Outcome
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
	case "agy", "antigravity", "agy-cli":
		return "agy"
	case "pi-cli", "pi":
		return "pi"
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

// sanitizeSandboxPath shrinks a verbose project path for token-efficient
// agent-mode output. A sandbox path like
// /Users/.../sandboxes/<uuid>/merged collapses to just <uuid>; any other
// absolute path is reduced to its basename.
func sanitizeSandboxPath(p string) string {
	if p == "" {
		return ""
	}
	if _, rest, found := strings.Cut(p, "/sandboxes/"); found {
		return strings.TrimSuffix(rest, "/merged")
	}
	return filepath.Base(p)
}

// applyAgentListDefaults rewrites a list result in place for agent-mode output:
// it drops each session's raw UUID unless the caller explicitly asked for it via
// --fields id, and basenames verbose sandbox project paths. Human-mode output
// never passes through here, so it is unaffected.
func applyAgentListDefaults(result *ops.ListSessionsResult, fields []string) {
	if result == nil {
		return
	}
	includeID := slices.Contains(fields, "id")
	for i := range result.Sessions {
		if !includeID {
			result.Sessions[i].ID = ""
		}
		result.Sessions[i].Project = sanitizeSandboxPath(result.Sessions[i].Project)
	}
}

// printSessionSummaryTable prints a compact table of session summaries.
// When showOutcome is true (e.g. `list --all`), an OUTCOME column is inserted
// so archived rows are distinguishable by how they ended.
func printSessionSummaryTable(cmd *cobra.Command, sessions []ops.SessionSummary, showTrust, showOutcome bool) {
	out := cmd.OutOrStdout()

	// Sort alphabetically by name
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	// Legend on separate lines
	_, _ = fmt.Fprintln(out, "Status  (S): ●=active & attached  ◐=active & detached  ○=stopped  ▪=archived")
	_, _ = fmt.Fprintln(out, "Harness (H): cc=claude  gem=gemini  cdx=codex  oc=opencode")
	_, _ = fmt.Fprintln(out)

	// Build the format string and headers dynamically so the optional OUTCOME
	// and TRUST columns compose without a 2x2 explosion of printf literals.
	header := []any{"NAME", "S", "H"}
	rule := []any{"---", "-", "--"}
	format := "%-28s %s %-3s"
	if showOutcome {
		format += " %-9s"
		header = append(header, "OUTCOME")
		rule = append(rule, "-------")
	}
	if showTrust {
		format += " %5s"
		header = append(header, "TRUST")
		rule = append(rule, "-----")
	}
	format += " %-24s %s\n"
	header = append(header, "PROJECT", "TAGS")
	rule = append(rule, "-------", "----")

	_, _ = fmt.Fprintf(out, format, header...)
	_, _ = fmt.Fprintf(out, format, rule...)

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
		row := []any{name, shortStatus(s), shortHarness(s.Harness)}
		if showOutcome {
			row = append(row, outcomeCell(s))
		}
		if showTrust {
			row = append(row, fmt.Sprintf("%5d", lookupTrustScore(s.Name)))
		}
		row = append(row, project, tags)
		_, _ = fmt.Fprintf(out, format, row...)
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
