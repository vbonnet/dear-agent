package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/search"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	searchWorkspace     string
	searchRegex         bool
	searchCaseSensitive bool
)

var sessionSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search conversation content for keywords or patterns",
	Long: `Search conversation content in history.jsonl files for keywords or regex patterns.

This command searches through all conversation messages to find sessions containing
specific text. It's useful for finding past conversations about particular topics
or recovering work when you can't remember which session you used.

Search behavior:
- Case-insensitive by default (use --case-sensitive for exact match)
- Searches all text content in conversation messages
- Supports regex patterns with --regex flag
- Returns session UUID, name, match count, and context snippet

Examples:
  agm session search "docker compose"              # Find sessions about docker compose
  agm session search "error.*timeout" --regex      # Regex pattern search
  agm session search "API" --case-sensitive        # Case-sensitive search
  agm session search "kubernetes" --workspace oss  # Search only oss workspace

Output:
  UUID       | Session Name    | Matches | Workspace | Context
  -----------|----------------|---------|-----------|------------------
  abc123...  | fix-auth-bug   | 5       | oss       | "...error timeout..."`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionSearch,
}

func init() {
	sessionCmd.AddCommand(sessionSearchCmd)

	sessionSearchCmd.Flags().StringVar(&searchWorkspace, "workspace", "",
		"Filter search to specific workspace")
	sessionSearchCmd.Flags().BoolVar(&searchRegex, "regex", false,
		"Treat query as regex pattern")
	sessionSearchCmd.Flags().BoolVar(&searchCaseSensitive, "case-sensitive", false,
		"Enable case-sensitive search")
}

func runSessionSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	fmt.Println(ui.Blue("=== Session Content Search ===\n"))
	fmt.Printf("Query: %s\n", ui.Yellow(query))
	if searchRegex {
		fmt.Println("Mode: Regex")
	} else {
		fmt.Println("Mode: Keyword")
	}
	if searchCaseSensitive {
		fmt.Println("Case: Sensitive")
	} else {
		fmt.Println("Case: Insensitive")
	}
	if searchWorkspace != "" {
		fmt.Printf("Workspace filter: %s\n", searchWorkspace)
	}
	fmt.Println()

	// Get Dolt storage adapter
	// Note: Content search uses the search package directly (not ops.SearchSessions
	// which does name-based search). The search.NewSearcher requires *dolt.Adapter
	// so we use getStorage() here. The ops.SearchSessions function provides
	// name-matching via the ops layer for MCP/CLI parity.
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Create searcher
	searcher := search.NewSearcher(adapter)

	// Perform search
	results, err := searcher.Search(search.SearchOptions{
		Query:         query,
		UseRegex:      searchRegex,
		CaseSensitive: searchCaseSensitive,
		Workspace:     searchWorkspace,
	})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Display results
	if len(results) == 0 {
		fmt.Println(ui.Yellow("No matching sessions found"))
		fmt.Println()
		fmt.Println("Tips:")
		fmt.Println("  • Try a different search query")
		fmt.Println("  • Remove workspace filter to search all workspaces")
		fmt.Println("  • Use --regex for pattern matching")
		return nil
	}

	fmt.Printf("Found %s matching session(s)\n\n", ui.Green(fmt.Sprintf("%d", len(results))))
	displaySearchResults(results)

	return nil
}

func displaySearchResults(results []*search.SearchResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "UUID\tSession Name\tMatches\tWorkspace\tContext")
	fmt.Fprintln(w, "----\t------------\t-------\t---------\t-------")

	// Rows
	for _, r := range results {
		uuid := r.SessionUUID
		if len(uuid) > 8 {
			uuid = uuid[:8] + "..."
		}

		sessionName := r.SessionName
		if sessionName == "" {
			sessionName = "(no manifest)"
		}
		// Truncate long names
		if len(sessionName) > 20 {
			sessionName = sessionName[:17] + "..."
		}

		workspace := r.Workspace
		if workspace == "" {
			workspace = "-"
		}

		context := r.ContextSnippet
		if context == "" {
			context = "(no snippet)"
		}
		// Truncate context for table display
		if len(context) > 40 {
			context = context[:37] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			uuid, sessionName, r.MatchCount, workspace, context)
	}
}
