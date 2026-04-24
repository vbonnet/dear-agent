package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/reservation"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	reservePaths   string
	reserveSession string
	reserveTTL     string
)

var sessionReserveCmd = &cobra.Command{
	Use:   "reserve",
	Short: "Reserve file paths for exclusive editing by a session",
	Long: `Reserve file paths (exact or glob patterns) for a session.

Other agents can check reservations before editing to avoid conflicts.
Reservations are advisory (not enforced) and auto-expire after the TTL.

Examples:
  agm session reserve --paths "pkg/dag/*.go" --session my-session
  agm session reserve --paths "main.go,pkg/api/*.go" --session task-1
  agm session reserve --paths "*.go" --session s1 --ttl 30m`,
	RunE: runSessionReserve,
}

func init() {
	sessionReserveCmd.Flags().StringVar(&reservePaths, "paths", "",
		"Comma-separated file paths or glob patterns to reserve")
	sessionReserveCmd.Flags().StringVar(&reserveSession, "session", "",
		"Session ID claiming the reservation")
	sessionReserveCmd.Flags().StringVar(&reserveTTL, "ttl", "2h",
		"Time-to-live for reservations (e.g. 30m, 2h, 24h)")

	_ = sessionReserveCmd.MarkFlagRequired("paths")
	_ = sessionReserveCmd.MarkFlagRequired("session")

	sessionCmd.AddCommand(sessionReserveCmd)
}

func runSessionReserve(_ *cobra.Command, _ []string) error {
	patterns := splitPaths(reservePaths)
	if len(patterns) == 0 {
		return fmt.Errorf("no paths specified")
	}

	ttl, err := time.ParseDuration(reserveTTL)
	if err != nil {
		return fmt.Errorf("invalid TTL %q: %w", reserveTTL, err)
	}

	store := reservation.NewStore(reservation.DefaultStorePath())
	created, err := store.Reserve(reserveSession, patterns, ttl)
	if err != nil {
		return fmt.Errorf("failed to reserve: %w", err)
	}

	fmt.Printf("%s Reserved %d path(s) for session %s (TTL: %s)\n",
		ui.Green("[OK]"), len(created), ui.Yellow(reserveSession), ttl)
	for _, r := range created {
		fmt.Printf("  %s %s (expires %s)\n",
			ui.Green("+"), r.Pattern, r.ExpiresAt.Format(time.RFC3339))
	}

	return nil
}

func splitPaths(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// --- check-reservation command ---

var (
	checkPath    string
	checkSession string
	checkJSON    bool
)

var sessionCheckReservationCmd = &cobra.Command{
	Use:   "check-reservation",
	Short: "Check if a file path is reserved by another session",
	Long: `Check whether a specific file path is reserved by another session.

Returns the owner session ID if reserved, or indicates the path is free.
Use --session to exclude your own reservations from the check.

Examples:
  agm session check-reservation --path "pkg/dag/resolver.go"
  agm session check-reservation --path "main.go" --session my-session
  agm session check-reservation --path "pkg/dag/types.go" --json`,
	RunE: runSessionCheckReservation,
}

func init() {
	sessionCheckReservationCmd.Flags().StringVar(&checkPath, "path", "",
		"File path to check")
	sessionCheckReservationCmd.Flags().StringVar(&checkSession, "session", "",
		"Current session ID (to exclude own reservations)")
	sessionCheckReservationCmd.Flags().BoolVar(&checkJSON, "json", false,
		"Output result as JSON")

	_ = sessionCheckReservationCmd.MarkFlagRequired("path")

	sessionCmd.AddCommand(sessionCheckReservationCmd)
}

func runSessionCheckReservation(_ *cobra.Command, _ []string) error {
	store := reservation.NewStore(reservation.DefaultStorePath())
	result, err := store.Check(checkPath, checkSession)
	if err != nil {
		return fmt.Errorf("failed to check reservation: %w", err)
	}

	if checkJSON {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if result.Reserved {
		fmt.Printf("%s Path %s is reserved by session %s (pattern: %s, expires: %s)\n",
			ui.Yellow("[RESERVED]"),
			ui.Yellow(checkPath),
			ui.Yellow(result.Owner),
			result.Pattern,
			result.ExpiresAt.Format(time.RFC3339))
	} else {
		fmt.Printf("%s Path %s is available\n",
			ui.Green("[FREE]"),
			checkPath)
	}

	return nil
}

// --- release command ---

var releaseSession string

var sessionReleaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release all file reservations for a session",
	Long: `Release all file path reservations held by a session.

This is automatically done when reservations expire (TTL), but can be
called explicitly when a session completes its work early.

Examples:
  agm session release --session my-session`,
	RunE: runSessionRelease,
}

func init() {
	sessionReleaseCmd.Flags().StringVar(&releaseSession, "session", "",
		"Session ID whose reservations to release")

	_ = sessionReleaseCmd.MarkFlagRequired("session")

	sessionCmd.AddCommand(sessionReleaseCmd)
}

func runSessionRelease(_ *cobra.Command, _ []string) error {
	store := reservation.NewStore(reservation.DefaultStorePath())
	released, err := store.Release(releaseSession)
	if err != nil {
		return fmt.Errorf("failed to release: %w", err)
	}

	if released > 0 {
		fmt.Printf("%s Released %d reservation(s) for session %s\n",
			ui.Green("[OK]"), released, ui.Yellow(releaseSession))
	} else {
		fmt.Printf("%s No active reservations found for session %s\n",
			ui.Yellow("[NONE]"), releaseSession)
	}

	return nil
}

// --- list-reservations command ---

var listReservationsSession string

var sessionListReservationsCmd = &cobra.Command{
	Use:   "list-reservations",
	Short: "List all active file reservations",
	Long: `List all active (non-expired) file path reservations.

Optionally filter by session ID.

Examples:
  agm session list-reservations
  agm session list-reservations --session my-session`,
	RunE: runSessionListReservations,
}

func init() {
	sessionListReservationsCmd.Flags().StringVar(&listReservationsSession, "session", "",
		"Filter to specific session ID")

	sessionCmd.AddCommand(sessionListReservationsCmd)
}

func runSessionListReservations(_ *cobra.Command, _ []string) error {
	store := reservation.NewStore(reservation.DefaultStorePath())
	reservations, err := store.List(listReservationsSession)
	if err != nil {
		return fmt.Errorf("failed to list reservations: %w", err)
	}

	if len(reservations) == 0 {
		fmt.Println(ui.Yellow("No active reservations"))
		return nil
	}

	fmt.Printf("%s Active reservations:\n\n", ui.Blue("==="))
	for _, r := range reservations {
		remaining := time.Until(r.ExpiresAt).Truncate(time.Second)
		fmt.Printf("  Session: %s\n", ui.Yellow(r.SessionID))
		fmt.Printf("  Pattern: %s\n", r.Pattern)
		fmt.Printf("  Expires: %s (%s remaining)\n", r.ExpiresAt.Format(time.RFC3339), remaining)
		fmt.Println()
	}

	return nil
}
