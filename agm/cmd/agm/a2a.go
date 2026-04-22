package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	agma2a "github.com/vbonnet/dear-agent/agm/internal/a2a"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var a2aCmd = &cobra.Command{
	Use:   "a2a",
	Short: "A2A Agent Card management",
	Long: `A2A commands manage Agent Cards for AGM sessions.

Agent Cards expose session metadata in the A2A protocol format,
enabling external tools to discover AGM-managed agents.

Cards are stored as JSON files at ~/.agm/a2a/cards/.

Examples:
  agm a2a list-cards              # List all Agent Cards
  agm a2a show-card my-session    # Show card for a session
  agm a2a sync                    # Sync cards from active sessions`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var a2aListCardsCmd = &cobra.Command{
	Use:   "list-cards",
	Short: "List all A2A Agent Cards",
	Long:  `List all Agent Cards in the registry. Shows name, description, and skill count.`,
	RunE:  runA2AListCards,
}

var a2aShowCardCmd = &cobra.Command{
	Use:   "show-card [session-name]",
	Short: "Show Agent Card for a session",
	Long:  `Display the full A2A Agent Card JSON for a given session.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runA2AShowCard,
}

var a2aSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Agent Cards from active sessions",
	Long: `Generate or update Agent Cards for all active sessions.
Removes cards for archived or deleted sessions.`,
	RunE: runA2ASync,
}

func init() {
	rootCmd.AddCommand(a2aCmd)
	a2aCmd.AddCommand(a2aListCardsCmd)
	a2aCmd.AddCommand(a2aShowCardCmd)
	a2aCmd.AddCommand(a2aSyncCmd)
}

func getCardRegistry() (*agma2a.Registry, error) {
	cardsDir, err := agma2a.DefaultCardsDir()
	if err != nil {
		return nil, err
	}
	return agma2a.NewRegistry(cardsDir)
}

func runA2AListCards(_ *cobra.Command, _ []string) error {
	reg, err := getCardRegistry()
	if err != nil {
		return fmt.Errorf("failed to open card registry: %w", err)
	}

	cards, err := reg.ListCards()
	if err != nil {
		return fmt.Errorf("failed to list cards: %w", err)
	}

	if len(cards) == 0 {
		fmt.Println("No Agent Cards found. Run 'agm a2a sync' to generate cards from active sessions.")
		return nil
	}

	if outputFormat == "json" {
		data, err := json.MarshalIndent(cards, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal cards: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tSKILLS\tVERSION")
	for _, card := range cards {
		desc := card.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", card.Name, desc, len(card.Skills), card.ProtocolVersion)
	}
	w.Flush()

	return nil
}

func runA2AShowCard(_ *cobra.Command, args []string) error {
	sessionName := args[0]

	reg, err := getCardRegistry()
	if err != nil {
		return fmt.Errorf("failed to open card registry: %w", err)
	}

	card, err := reg.GetCard(sessionName)
	if err != nil {
		return fmt.Errorf("no card found for session %q: %w", sessionName, err)
	}

	data, err := agma2a.CardJSON(*card)
	if err != nil {
		return fmt.Errorf("failed to serialize card: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

func runA2ASync(_ *cobra.Command, _ []string) error {
	// Get storage adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	// List all sessions
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Filter to non-archived sessions for card generation
	var active []*manifest.Manifest
	for _, m := range manifests {
		if m.Lifecycle != manifest.LifecycleArchived {
			active = append(active, m)
		}
	}

	reg, err := getCardRegistry()
	if err != nil {
		return fmt.Errorf("failed to open card registry: %w", err)
	}

	if err := reg.SyncFromManifests(manifests); err != nil {
		return fmt.Errorf("failed to sync cards: %w", err)
	}

	fmt.Printf("Synced %d Agent Cards (%d active sessions)\n", len(active), len(active))
	return nil
}
