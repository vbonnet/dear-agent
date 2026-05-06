package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/intent"
)

var intentCmd = &cobra.Command{
	Use:   "intent",
	Short: "Declare and inspect agent coordination intents",
	Long: `Intents are non-blocking coordination signals: an agent declares
which files or packages it plans to touch, and other agents can read
the board to see what's in flight.

This is information, not a lock. Two agents may declare overlapping
intents and both proceed — the board exists so they (and the human
reviewing them) know it's happening. Use 'agm intent list --overlapping'
to surface conflicts.

Intents auto-expire after their TTL (default 30m). Run 'agm intent expire'
to garbage-collect the board, or pass --include-expired to inspect
old rows for debugging.

Examples:
  agm intent declare --session=worker-1 --file=pkg/foo/bar.go --description="add tests"
  agm intent list
  agm intent list --session=worker-1
  agm intent list --overlapping
  agm intent expire
  agm intent remove <id>`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var intentDeclareCmd = &cobra.Command{
	Use:   "declare",
	Short: "Declare a new intent",
	RunE:  runIntentDeclare,
}

var intentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List intents on the board",
	RunE:  runIntentList,
}

var intentExpireCmd = &cobra.Command{
	Use:   "expire",
	Short: "Garbage-collect expired intents",
	RunE:  runIntentExpire,
}

var intentRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove an intent by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runIntentRemove,
}

var (
	intentDeclareSession     string
	intentDeclareFiles       []string
	intentDeclarePackages    []string
	intentDeclareDescription string
	intentDeclareTTL         time.Duration

	intentListSession     string
	intentListIncludeAll  bool
	intentListOverlapping bool

	intentBoardDir    string
	intentOutputForm  string
)

func init() {
	rootCmd.AddCommand(intentCmd)
	intentCmd.AddCommand(intentDeclareCmd)
	intentCmd.AddCommand(intentListCmd)
	intentCmd.AddCommand(intentExpireCmd)
	intentCmd.AddCommand(intentRemoveCmd)

	intentCmd.PersistentFlags().StringVar(&intentBoardDir, "board-dir", "", "intent board directory (default: ~/.agm/intents)")
	intentCmd.PersistentFlags().StringVarP(&intentOutputForm, "output", "o", "", "output format: text (default), json")

	intentDeclareCmd.Flags().StringVar(&intentDeclareSession, "session", "", "owning session ID (required; defaults to AGM_SESSION_NAME)")
	intentDeclareCmd.Flags().StringSliceVar(&intentDeclareFiles, "file", nil, "file path the agent intends to modify (repeatable)")
	intentDeclareCmd.Flags().StringSliceVar(&intentDeclarePackages, "package", nil, "package the agent intends to modify (repeatable)")
	intentDeclareCmd.Flags().StringVar(&intentDeclareDescription, "description", "", "human-readable description of the work")
	intentDeclareCmd.Flags().DurationVar(&intentDeclareTTL, "ttl", 0, "intent lifetime (default 30m)")

	intentListCmd.Flags().StringVar(&intentListSession, "session", "", "filter by session ID")
	intentListCmd.Flags().BoolVar(&intentListIncludeAll, "include-expired", false, "show expired intents too")
	intentListCmd.Flags().BoolVar(&intentListOverlapping, "overlapping", false, "show only intents that overlap another")
}

// resolveIntentBoard returns a FileBoard rooted at --board-dir or
// $HOME/.agm/intents.
func resolveIntentBoard() (*intent.FileBoard, error) {
	dir := intentBoardDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("intent: resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".agm", "intents")
	}
	return intent.NewFileBoard(dir)
}

func runIntentDeclare(_ *cobra.Command, _ []string) error {
	session := intentDeclareSession
	if session == "" {
		session = os.Getenv("AGM_SESSION_NAME")
	}
	if session == "" {
		return errors.New("--session is required (or set AGM_SESSION_NAME)")
	}
	if len(intentDeclareFiles) == 0 && len(intentDeclarePackages) == 0 {
		return errors.New("must declare at least one --file or --package")
	}

	board, err := resolveIntentBoard()
	if err != nil {
		return err
	}
	in, err := board.Declare(intent.DeclareOpts{
		SessionID:   session,
		Files:       intentDeclareFiles,
		Packages:    intentDeclarePackages,
		Description: intentDeclareDescription,
		TTL:         intentDeclareTTL,
	})
	if err != nil {
		return err
	}

	overlaps, err := board.Overlaps(in)
	if err != nil {
		// Declare succeeded; an overlap-check failure is informational.
		fmt.Fprintf(os.Stderr, "warning: could not check overlaps: %v\n", err)
	}

	format := resolveIntentFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Intent   intent.Intent   `json:"intent"`
			Overlaps []intent.Intent `json:"overlaps,omitempty"`
		}{Intent: in, Overlaps: overlaps})
	}

	fmt.Printf("Declared intent %s for session %s (expires %s)\n", in.ID, in.SessionID, in.ExpiresAt.Format(time.RFC3339))
	if len(overlaps) > 0 {
		fmt.Printf("\nWarning: %d overlapping intent(s) in flight (informational, not blocking):\n", len(overlaps))
		printIntentTable(overlaps)
	}
	return nil
}

func runIntentList(_ *cobra.Command, _ []string) error {
	board, err := resolveIntentBoard()
	if err != nil {
		return err
	}
	rows, err := board.List(intent.ListFilter{
		SessionID:      intentListSession,
		IncludeExpired: intentListIncludeAll,
	})
	if err != nil {
		return err
	}

	if intentListOverlapping {
		rows = filterOverlapping(rows)
	}

	format := resolveIntentFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		// Always a slice, never nil, so consumers can iterate without
		// a nil check.
		if rows == nil {
			rows = []intent.Intent{}
		}
		return enc.Encode(rows)
	}

	if len(rows) == 0 {
		fmt.Println("No intents on the board.")
		return nil
	}
	printIntentTable(rows)
	return nil
}

func runIntentExpire(_ *cobra.Command, _ []string) error {
	board, err := resolveIntentBoard()
	if err != nil {
		return err
	}
	n, err := board.Expire()
	if err != nil {
		return err
	}
	fmt.Printf("Removed %d expired intent(s).\n", n)
	return nil
}

func runIntentRemove(_ *cobra.Command, args []string) error {
	board, err := resolveIntentBoard()
	if err != nil {
		return err
	}
	if err := board.Remove(args[0]); err != nil {
		return err
	}
	fmt.Printf("Removed intent %s.\n", args[0])
	return nil
}

// filterOverlapping returns the subset of rows that overlap at least
// one other row. Used by `agm intent list --overlapping`.
func filterOverlapping(rows []intent.Intent) []intent.Intent {
	out := make([]intent.Intent, 0)
	for i, a := range rows {
		for j, b := range rows {
			if i == j {
				continue
			}
			if a.Overlaps(b) {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

func printIntentTable(rows []intent.Intent) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSESSION\tFILES\tPACKAGES\tEXPIRES\tDESCRIPTION")
	for _, r := range rows {
		files := strings.Join(r.Files, ",")
		if files == "" {
			files = "-"
		}
		packages := strings.Join(r.Packages, ",")
		if packages == "" {
			packages = "-"
		}
		desc := r.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.SessionID, files, packages, r.ExpiresAt.Format(time.RFC3339), desc)
	}
	_ = w.Flush()
}

func resolveIntentFormat() string {
	if intentOutputForm != "" {
		return intentOutputForm
	}
	return outputFormat
}
