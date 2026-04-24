package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestGroupRunE_UnknownSubcommand(t *testing.T) {
	// Create a parent command with groupRunE
	parent := &cobra.Command{
		Use:  "parent",
		Args: cobra.ArbitraryArgs,
		RunE: groupRunE,
	}

	// Add a valid child
	child := &cobra.Command{
		Use:  "valid",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	parent.AddCommand(child)

	// Test: unknown subcommand should return an error
	err := groupRunE(parent, []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}
	if got := err.Error(); got != `unknown command "bogus" for "parent"` {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestGroupRunE_NoArgs(t *testing.T) {
	// Create a parent command with groupRunE
	parent := &cobra.Command{
		Use:  "parent",
		Args: cobra.ArbitraryArgs,
		RunE: groupRunE,
	}

	// Test: no args should not return an error (just prints help)
	err := groupRunE(parent, []string{})
	if err != nil {
		t.Fatalf("expected nil error for no args, got: %v", err)
	}
}
