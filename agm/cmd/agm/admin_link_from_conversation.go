package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var linkFromConversationCmd = &cobra.Command{
	Use:   "link-from-conversation",
	Short: "Link sessions to parents by reading parentUuid from conversation files",
	Long: `Read conversation.jsonl files to discover parentUuid relationships and set up
parent_session_id links in the database.

This command:
1. Scans all sessions in the database
2. For each session, reads its conversation.jsonl file
3. Looks for parentUuid in the first 20 lines
4. If found, links child.parent_session_id = parent.id

This works for sessions created days/weeks ago where time-based detection won't work.`,
	RunE: runLinkFromConversation,
}

var (
	linkFromConvApply  bool
	linkFromConvDryRun bool
)

func init() {
	adminCmd.AddCommand(linkFromConversationCmd)
	linkFromConversationCmd.Flags().BoolVar(&linkFromConvApply, "apply", false, "Execute changes and update database")
	linkFromConversationCmd.Flags().BoolVar(&linkFromConvDryRun, "dry-run", false, "Preview changes without modifying database")
}

type conversationLine struct {
	ParentUUID string `json:"parentUuid"`
	Type       string `json:"type"`
}

func runLinkFromConversation(cmd *cobra.Command, args []string) error {
	// Default to dry-run if neither flag specified
	if !linkFromConvApply && !linkFromConvDryRun {
		linkFromConvDryRun = true
	}

	mode := "dry-run"
	if linkFromConvApply {
		mode = "apply"
	}

	fmt.Printf("Scanning sessions for parentUuid in conversation files (%s mode)...\n\n", mode)

	// Connect to storage
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	// Get sessions dir - use default Claude Code location
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	sessionsDir := filepath.Join(home, ".claude", "projects", "-home-user-src")

	// Get all sessions from database
	allSessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Build map of claude_uuid -> session for lookups
	sessionsByUUID := make(map[string]*manifest.Manifest)
	for _, s := range allSessions {
		if s.Claude.UUID != "" {
			sessionsByUUID[s.Claude.UUID] = s
		}
	}

	relationships := buildLinkRelationships(allSessions, sessionsByUUID, sessionsDir)

	if len(relationships) == 0 {
		fmt.Println("✓ No orphaned execution sessions found")
		return nil
	}

	fmt.Printf("Found %d execution sessions to link:\n\n", len(relationships))

	for i, rel := range relationships {
		fmt.Printf("%d. Execution: %s (%s)\n", i+1, rel.ChildName, rel.ChildUUID[:8])
		fmt.Printf("   Planning:  %s (%s)\n", rel.ParentName, rel.ParentUUID[:8])
		fmt.Printf("   Action:    Set parent_session_id = %s\n\n", rel.ParentID)
	}

	if linkFromConvDryRun {
		fmt.Println("Run with --apply to execute changes")
		return nil
	}

	// Apply changes
	fmt.Println("Applying changes...")
	for _, rel := range relationships {
		// Update parent_session_id using SQL update
		query := `UPDATE agm_sessions SET parent_session_id = ? WHERE session_id = ?`
		if err := adapter.ExecSQL(cmd.Context(), query, rel.ParentID, rel.ChildID); err != nil {
			fmt.Printf("  ✗ Failed to link %s: %v\n", rel.ChildName, err)
			continue
		}

		fmt.Printf("  ✓ Linked: %s → %s\n", rel.ChildName, rel.ParentName)
	}

	fmt.Printf("\n✓ Successfully linked %d sessions\n", len(relationships))
	return nil
}

// linkRelationship records a discovered child→parent link awaiting application.
type linkRelationship struct {
	ChildUUID  string
	ChildName  string
	ChildID    string
	ParentUUID string
	ParentName string
	ParentID   string
}

// buildLinkRelationships scans each candidate session's conversation file for
// a parentUuid and pairs it with the matching session in sessionsByUUID.
func buildLinkRelationships(allSessions []*manifest.Manifest, sessionsByUUID map[string]*manifest.Manifest, sessionsDir string) []linkRelationship {
	var rels []linkRelationship
	for _, session := range allSessions {
		if session.Claude.UUID == "" {
			continue
		}
		if session.ParentSessionID != nil && *session.ParentSessionID != "" {
			continue
		}
		conversationFile := filepath.Join(sessionsDir, session.Claude.UUID+".jsonl")
		if _, err := os.Stat(conversationFile); os.IsNotExist(err) {
			continue
		}
		parentUUID, err := extractParentUUID(conversationFile)
		if err != nil || parentUUID == "" {
			continue
		}
		parentSession, exists := sessionsByUUID[parentUUID]
		if !exists {
			continue
		}
		rels = append(rels, linkRelationship{
			ChildUUID:  session.Claude.UUID,
			ChildName:  session.Name,
			ChildID:    session.SessionID,
			ParentUUID: parentUUID,
			ParentName: parentSession.Name,
			ParentID:   parentSession.SessionID,
		})
	}
	return rels
}

func extractParentUUID(conversationFile string) (string, error) {
	f, err := os.Open(conversationFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineCount := 0

	for scanner.Scan() && lineCount < 20 {
		lineCount++

		var line conversationLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		if line.ParentUUID != "" {
			return line.ParentUUID, nil
		}
	}

	return "", scanner.Err()
}
