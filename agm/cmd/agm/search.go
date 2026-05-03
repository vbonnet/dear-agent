package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/llm"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	searchCache      *llm.SearchCache
	searchMaxResults int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Find archived sessions using semantic search",
	Long: `Find archived Claude sessions by conversational content using AI-powered semantic search.

This command:
  1. Searches your conversation history (~/.claude/history.jsonl)
  2. Uses Google Vertex AI (Claude Haiku) for semantic matching
  3. Shows ranked results with interactive selection
  4. Auto-restores the selected session

Authentication:
  • Uses Google Cloud Application Default Credentials (ADC)
  • Run 'gcloud auth application-default login' to authenticate
  • Requires GOOGLE_CLOUD_PROJECT env var or gcloud default project

Rate limiting: 10 searches per minute
Cache: Results cached for 5 minutes

Examples:
  # Find session about Composio
  agm session search "that conversation about Composio"

  # Find OAuth implementation work
  agm session search "OAuth integration with MCP"

  # Find session from last week
  agm session search "last week's debugging session"

Fallback:
  If search doesn't find what you need, try pattern matching:
  agm session unarchive *composio*`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().IntVar(&searchMaxResults, "max-results", 10, "Maximum number of results to return")
	sessionCmd.AddCommand(searchCmd)

	// Initialize search cache (5-minute TTL)
	searchCache = llm.NewSearchCache(5 * time.Minute)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	// Get Dolt storage adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Check cache first
	if cachedResults := searchCache.Get(query); cachedResults != nil {
		fmt.Printf("Using cached results\n")
		return handleSearchResults(adapter, query, cachedResults)
	}

	// Parse conversation history
	historyParser := history.NewParser("")
	sessions, err := historyParser.ReadConversations(1000) // Limit to last 1000 entries
	if err != nil {
		ui.PrintError(err, "Failed to read conversation history",
			"  • Check ~/.claude/history.jsonl exists\n"+
				"  • Verify file permissions")
		return err
	}

	if len(sessions) == 0 {
		fmt.Printf("No conversation history found\n")
		fmt.Printf("\nHistory file appears to be empty or doesn't exist.\n")
		fmt.Printf("Try pattern matching instead: agm session unarchive *<pattern>*\n")
		return nil
	}

	fmt.Printf("Searching %d sessions from history...\n", len(sessions))

	// Get list of archived sessions to filter results
	archivedSessions, err := session.FindArchived(cfg.SessionsDir, "*", nil)
	if err != nil {
		ui.PrintError(err,
			"Failed to list archived sessions",
			"  • Check archive directory: ls -la "+filepath.Join(cfg.SessionsDir, ".archive-old-format")+"\n"+
				"  • Verify sessions directory: ls -ld "+cfg.SessionsDir+"\n"+
				"  • List active sessions only: agm session list")
		return err
	}

	if len(archivedSessions) == 0 {
		fmt.Printf("No archived sessions found\n")
		fmt.Printf("\nYou don't have any archived sessions to search.\n")
		fmt.Printf("Use 'agm session list' to see active sessions.\n")
		return nil
	}

	// Create session ID set for filtering
	archivedIDs := make(map[string]bool)
	for _, s := range archivedSessions {
		archivedIDs[s.SessionID] = true
	}

	// Build metadata for LLM search (only archived sessions)
	var sessionMetadata []llm.SessionMetadata
	for _, s := range sessions {
		if archivedIDs[s.SessionID] {
			// Find archived session details
			var archivedSession *session.ArchivedSession
			for _, a := range archivedSessions {
				if a.SessionID == s.SessionID {
					archivedSession = a
					break
				}
			}

			if archivedSession != nil {
				sessionMetadata = append(sessionMetadata, llm.SessionMetadata{
					SessionID: s.SessionID,
					Name:      archivedSession.Name,
					Tags:      archivedSession.Tags,
					Project:   s.Project,
				})
			}
		}
	}

	if len(sessionMetadata) == 0 {
		fmt.Printf("No archived sessions with conversation history found\n")
		fmt.Printf("\nArchived sessions don't have conversation history in ~/.claude/history.jsonl.\n")
		fmt.Printf("Try pattern matching: agm session unarchive *<pattern>*\n")
		return nil
	}

	fmt.Printf("Querying AI for %d archived sessions...\n", len(sessionMetadata))

	// Get GCP project ID
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		// Try reading from gcloud config
		projectID = getGCloudProject()
	}

	if projectID == "" {
		return fmt.Errorf("GCP project ID not configured\n\nPlease set one of:\n  • GOOGLE_CLOUD_PROJECT environment variable\n  • Run: gcloud config set project <project-id>\n  • Run: gcloud auth application-default login")
	}

	// Create LLM client
	llmClient, err := llm.NewClient(llm.ClientConfig{
		ProjectID: projectID,
		Location:  "us-central1",
		ModelID:   "claude-3-5-haiku@20241022",
		RateLimit: 10,
	})
	if err != nil {
		ui.PrintError(err,
			"Failed to create LLM client for semantic search",
			"  • Set GCP project: export GOOGLE_CLOUD_PROJECT=<project-id>\n"+
				"  • Or set project ID: gcloud config set project <project-id>\n"+
				"  • Verify credentials: gcloud auth application-default login\n"+
				"  • Enable Vertex AI API: gcloud services enable aiplatform.googleapis.com")
		return err
	}

	// Perform search
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := llmClient.Search(ctx, llm.SearchRequest{
		Query:      query,
		Sessions:   sessionMetadata,
		MaxResults: searchMaxResults,
	})
	if err != nil {
		ui.PrintError(err, "Search failed",
			"Possible causes:\n"+
				"  • GCP credentials not configured (run 'gcloud auth application-default login')\n"+
				"  • Vertex AI API not enabled (enable at console.cloud.google.com)\n"+
				"  • Network connectivity issue\n\n"+
				"Try pattern matching as fallback: agm session unarchive *<pattern>*")
		return err
	}

	// Cache results
	searchCache.Set(query, results)

	return handleSearchResults(adapter, query, results)
}

func handleSearchResults(adapter *dolt.Adapter, query string, results []llm.SearchResult) error {
	switch len(results) {
	case 0:
		fmt.Printf("No matching sessions found\n")
		fmt.Printf("\nNo archived sessions match your query: \"%s\"\n\n", query)
		fmt.Printf("Suggestions:\n")
		fmt.Printf("  • Try a different query\n")
		fmt.Printf("  • Use pattern matching: agm session unarchive *<pattern>*\n")
		fmt.Printf("  • List all archived: agm session list --all\n")
		return nil

	case 1:
		// Single result - show details and confirm
		result := results[0]
		archivedSession, err := findArchivedByID(result.SessionID)
		if err != nil {
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Found 1 match: %s", archivedSession.Name))
		fmt.Printf("\nSession details:\n")
		fmt.Printf("  • Name: %s\n", archivedSession.Name)
		fmt.Printf("  • Project: %s\n", archivedSession.Project)
		if len(archivedSession.Tags) > 0 {
			fmt.Printf("  • Tags: %v\n", archivedSession.Tags)
		}
		fmt.Printf("  • Archived: %s\n", archivedSession.ArchivedAt)
		fmt.Printf("  • Relevance: %s\n", result.Reason)
		fmt.Printf("\n")

		// Confirm before restoring
		var confirmed bool
		err = huh.NewConfirm().
			Title("Restore this session?").
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			WithTheme(ui.GetTheme()).
			Run()
		if err != nil || !confirmed {
			fmt.Printf("\nRestore cancelled.\n")
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}

		return restoreArchivedSession(adapter, archivedSession)

	default:
		// Multiple results - show interactive selection
		ui.PrintSuccess(fmt.Sprintf("Found %d matches", len(results)))

		// Load archived session details
		var sessionInfos []ui.ArchivedSessionInfo
		for _, result := range results {
			archivedSession, err := findArchivedByID(result.SessionID)
			if err != nil {
				continue // Skip if not found
			}

			sessionInfos = append(sessionInfos, ui.ArchivedSessionInfo{
				SessionID:  archivedSession.SessionID,
				Name:       fmt.Sprintf("%s (relevance: %.1f)", archivedSession.Name, result.Relevance),
				ArchivedAt: archivedSession.ArchivedAt,
				Tags:       archivedSession.Tags,
				Project:    archivedSession.Project,
			})
		}

		if len(sessionInfos) == 0 {
			return fmt.Errorf("no valid sessions found (internal error)")
		}

		// Show picker
		selectedID, err := ui.ArchivedSessionPicker(sessionInfos)
		if err != nil {
			fmt.Printf("\nRestore cancelled.\n")
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}

		// Find and restore selected session
		archivedSession, err := findArchivedByID(selectedID)
		if err != nil {
			return err
		}

		return restoreArchivedSession(adapter, archivedSession)
	}
}

// findArchivedByID finds an archived session by session ID
func findArchivedByID(sessionID string) (*session.ArchivedSession, error) {
	// Search both locations
	archivedSessions, err := session.FindArchived(cfg.SessionsDir, "*", nil)
	if err != nil {
		return nil, err
	}

	for _, s := range archivedSessions {
		if s.SessionID == sessionID {
			return s, nil
		}
	}

	return nil, fmt.Errorf("archived session not found: %s", sessionID)
}

// getGCloudProject attempts to read the default project from gcloud config
func getGCloudProject() string {
	// Try reading from gcloud config file
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configPath := home + "/.config/gcloud/configurations/config_default"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	// Simple parsing - look for "project = <project-id>"
	lines := string(data)
	for _, line := range splitLinesStr(lines) {
		if len(line) > 10 && line[:8] == "project " {
			parts := splitOnEquals(line)
			if len(parts) == 2 {
				return trimSpace(parts[1])
			}
		}
	}

	return ""
}

func splitLinesStr(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitOnEquals(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
