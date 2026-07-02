package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/codexcontrol"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	reconcileCodexExecute bool
	reconcileCodexLimit   int
)

var adminReconcileCodexCmd = &cobra.Command{
	Use:   "reconcile-codex",
	Short: "Import missing Codex app-server threads into AGM metadata",
	Long: `List non-archived Codex app-server threads and create AGM metadata for
threads that are not already tracked by Codex thread id.

This command does not create tmux sessions, archive Codex threads, or delete
resources. It only records AGM metadata so Codex-originated work can appear in
AGM lists and later be explicitly resumed or archived.

Examples:
  agm admin reconcile-codex
  agm admin reconcile-codex --execute
  agm admin reconcile-codex --limit 50 --execute`,
	RunE: runAdminReconcileCodex,
}

func init() {
	adminCmd.AddCommand(adminReconcileCodexCmd)
	adminReconcileCodexCmd.Flags().BoolVar(&reconcileCodexExecute, "execute", false, "Create AGM records for missing Codex threads")
	adminReconcileCodexCmd.Flags().IntVar(&reconcileCodexLimit, "limit", 200, "Maximum Codex threads to inspect")
}

func runAdminReconcileCodex(cmd *cobra.Command, args []string) error {
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer func() { _ = adapter.Close() }()

	client := codexcontrol.New()
	if err := client.StartRemoteControl(context.Background()); err != nil {
		return err
	}
	archived := false
	threads, err := client.ListThreads(context.Background(), codexcontrol.ListThreadsOptions{
		Archived: &archived,
		Limit:    reconcileCodexLimit,
	})
	if err != nil {
		return err
	}

	var imported, skipped int
	for _, thread := range threads.Data {
		if thread.ID == "" {
			continue
		}
		existing, err := adapter.GetSessionByUUID(thread.ID)
		if err != nil {
			return fmt.Errorf("check existing AGM session for Codex thread %s: %w", thread.ID, err)
		}
		if existing != nil {
			skipped++
			continue
		}
		name := codexThreadAGMName(thread)
		if !reconcileCodexExecute {
			fmt.Printf("Would import Codex thread %s as %s (cwd: %s)\n", thread.ID, name, thread.CWD)
			imported++
			continue
		}
		if err := adapter.CreateSession(codexThreadManifest(thread, name, cfg.Workspace)); err != nil {
			return fmt.Errorf("import Codex thread %s: %w", thread.ID, err)
		}
		fmt.Printf("Imported Codex thread %s as %s\n", thread.ID, name)
		imported++
	}

	if reconcileCodexExecute {
		ui.PrintSuccess(fmt.Sprintf("Codex reconcile complete: imported %d, skipped %d", imported, skipped))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Codex reconcile dry-run complete: %d import candidate(s), skipped %d", imported, skipped))
		fmt.Println("Re-run with --execute to create AGM records.")
	}
	return nil
}

func codexThreadAGMName(thread codexcontrol.Thread) string {
	for _, candidate := range []string{thread.Name, thread.Preview, "codex-" + shortID(thread.ID)} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		name := tmux.SanitizeSessionName(candidate)
		if name != "" {
			return name
		}
	}
	return "codex-" + shortID(thread.ID)
}

func codexThreadManifest(thread codexcontrol.Thread, name, workspace string) *manifest.Manifest {
	createdAt := time.Now()
	if thread.CreatedAt > 0 {
		createdAt = time.Unix(thread.CreatedAt, 0)
	}
	updatedAt := time.Now()
	if thread.UpdatedAt > 0 {
		updatedAt = time.Unix(thread.UpdatedAt, 0)
	}
	return &manifest.Manifest{
		SchemaVersion:    manifest.SchemaVersion,
		SessionID:        uuid.New().String(),
		Name:             name,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		Lifecycle:        "",
		Workspace:        workspace,
		Harness:          "codex-cli",
		Model:            agent.HarnessDefaults["codex-cli"],
		WorkingDirectory: thread.CWD,
		Context: manifest.Context{
			Project: thread.CWD,
			Tags:    []string{"source:codex-reconcile"},
		},
		Codex: &manifest.Codex{
			SessionID:      thread.ID,
			TranscriptPath: thread.Path,
		},
		Tmux: manifest.Tmux{
			SessionName: name,
		},
	}
}
