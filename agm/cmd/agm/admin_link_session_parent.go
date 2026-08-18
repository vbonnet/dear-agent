package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	linkChildID     string
	linkParentID    string
	linkInheritName bool
)

var linkSessionParentCmd = &cobra.Command{
	Use:   "link-session-parent",
	Short: "Link a child session to its parent session",
	Long: `Link a child session to its parent session by setting parent_session_id.

This command establishes the parent-child relationship in the database and
optionally inherits the parent's name for the child session.

When --inherit-name is set and the child name is "Unknown", the child will
be renamed to "<parent-name>-exec" to maintain naming continuity.`,
	Example: `  # Link with name inheritance
  agm admin link-session-parent \
    --child 80c10f57-d114-477b-ad6e-53cfe23a580f \
    --parent 593e6716-d717-4039-a47f-0b0f92a8500f \
    --inherit-name

  # Link without name changes
  agm admin link-session-parent \
    --child 80c10f57-d114-477b-ad6e-53cfe23a580f \
    --parent 593e6716-d717-4039-a47f-0b0f92a8500f`,
	RunE: runLinkSessionParent,
}

func init() {
	adminCmd.AddCommand(linkSessionParentCmd)

	linkSessionParentCmd.Flags().StringVar(&linkChildID, "child", "",
		"Child session UUID (required)")
	linkSessionParentCmd.Flags().StringVar(&linkParentID, "parent", "",
		"Parent session UUID (required)")
	linkSessionParentCmd.Flags().BoolVar(&linkInheritName, "inherit-name", false,
		"Inherit parent name if child name is 'Unknown'")

	_ = linkSessionParentCmd.MarkFlagRequired("child")
	_ = linkSessionParentCmd.MarkFlagRequired("parent")
}

func runLinkSessionParent(cmd *cobra.Command, args []string) error {
	// Connect to Dolt storage
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer func() { _ = adapter.Close() }()

	// Validate child session exists
	child, err := adapter.GetSession(linkChildID)
	if err != nil {
		ui.PrintError(err,
			"Failed to load child session",
			"  • Verify child session ID exists\n"+
				"  • Run 'agm session list' to see available sessions")
		return fmt.Errorf("child session not found: %w", err)
	}

	// Validate parent session exists
	parent, err := adapter.GetSession(linkParentID)
	if err != nil {
		ui.PrintError(err,
			"Failed to load parent session",
			"  • Verify parent session ID exists\n"+
				"  • Run 'agm session list' to see available sessions")
		return fmt.Errorf("parent session not found: %w", err)
	}

	// Prevent circular references
	if child.SessionID == parent.SessionID {
		return fmt.Errorf("cannot link session to itself")
	}

	// Check if parent already has a parent (would create multi-level hierarchy)
	// For safety, warn but allow it
	if parent.ParentSessionID != nil && *parent.ParentSessionID != "" {
		fmt.Printf("%s Warning: Parent session '%s' also has a parent (%s)\n",
			ui.Yellow("⚠"), parent.Name, *parent.ParentSessionID)
		fmt.Println(ui.Yellow("  This creates a multi-level hierarchy"))
	}

	// Handle name inheritance
	var inheritedName *string
	if linkInheritName && (child.Name == "" || child.Name == "Unknown") {
		if parent.Name != "" {
			name := parent.Name + "-exec"
			inheritedName = &name
			child.Name = name
			fmt.Printf("%s Inherited name: '%s' → '%s'\n",
				ui.Blue("ℹ"), parent.Name, child.Name)
		}
	}

	// Parent assignment and optional name inheritance are one explicit identity
	// CAS. A concurrent writer is reported instead of silently dropping the
	// inherited name while claiming the link succeeded.
	if err := persistSessionParentLink(cmd.Context(), adapter, child, linkParentID, inheritedName); err != nil {
		return fmt.Errorf("failed to link child session in Dolt: %w", err)
	}

	// Success feedback
	ui.PrintSuccess(fmt.Sprintf(
		"Linked session parent: %s ('%s') → %s ('%s')",
		child.SessionID[:8], child.Name,
		parent.SessionID[:8], parent.Name))

	return nil
}
