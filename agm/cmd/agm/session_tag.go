package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var tagRemove string

var sessionTagCmd = &cobra.Command{
	Use:   "tag <session> <tag>",
	Short: "Add or remove tags on a session",
	Long: `Add or remove context tags on an existing session.

Tags follow a namespace:value convention:
  role:worker, role:orchestrator, cap:web-search, cap:claude-code

Examples:
  agm session tag my-session role:worker           # Add a role tag
  agm session tag my-session cap:web-search        # Add a capability tag
  agm session tag my-session --remove role:worker  # Remove a tag`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSessionTag,
}

func init() {
	sessionCmd.AddCommand(sessionTagCmd)
	sessionTagCmd.Flags().StringVar(&tagRemove, "remove", "", "Tag to remove from the session")
}

func runSessionTag(_ *cobra.Command, args []string) error {
	identifier := args[0]

	var addTag, removeTag string
	switch {
	case tagRemove != "":
		removeTag = tagRemove
	case len(args) < 2:
		return fmt.Errorf("provide a tag to add, or use --remove <tag>")
	default:
		addTag = args[1]
	}

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	result, opErr := ops.TagSession(opCtx, &ops.TagSessionRequest{
		Identifier: identifier,
		Add:        addTag,
		Remove:     removeTag,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	switch result.Action {
	case "added":
		fmt.Printf("Added tag %q to session %s\n", result.Tag, ui.Bold(result.Name))
	case "removed":
		fmt.Printf("Removed tag %q from session %s\n", result.Tag, ui.Bold(result.Name))
	case "noop":
		ui.PrintWarning(fmt.Sprintf("Tag %q already exists on session %s", result.Tag, result.Name))
	}

	if len(result.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(result.Tags, ", "))
	} else {
		fmt.Println("Tags: (none)")
	}

	return nil
}
