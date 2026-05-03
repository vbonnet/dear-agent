package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"gopkg.in/yaml.v3"
)

var (
	batchSpawnManifest string
	batchSpawnDetached bool
)

// batchManifest is the top-level structure of a batch spawn YAML manifest.
type batchManifest struct {
	Workers []ops.WorkerSpec `yaml:"workers"`
}

var batchSpawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "Launch multiple workers from a YAML manifest",
	Long: `Launch multiple workers in parallel from a YAML manifest file.

Each worker is created as a detached AGM session with the specified
prompt file and model.

Manifest format:
  workers:
    - name: fix-lint
      prompt-file: /tmp/fix-lint.txt
      model: opus
    - name: add-docs
      prompt-file: /tmp/add-docs.txt
      model: sonnet

Examples:
  agm batch spawn --manifest workers.yaml
  agm batch spawn --manifest workers.yaml --detached`,
	RunE: runBatchSpawn,
}

func init() {
	batchSpawnCmd.Flags().StringVar(&batchSpawnManifest, "manifest", "", "Path to YAML manifest file (required)")
	batchSpawnCmd.Flags().BoolVar(&batchSpawnDetached, "detached", true, "Create sessions in detached mode (default: true)")
	_ = batchSpawnCmd.MarkFlagRequired("manifest")
	batchCmd.AddCommand(batchSpawnCmd)
}

func runBatchSpawn(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(batchSpawnManifest)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest batchManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	if len(manifest.Workers) == 0 {
		return fmt.Errorf("manifest contains no workers")
	}

	// Validate all workers before spawning
	for _, w := range manifest.Workers {
		if w.Name == "" {
			return fmt.Errorf("worker has empty name")
		}
		if w.PromptFile == "" {
			return fmt.Errorf("worker %q has no prompt-file", w.Name)
		}
		if _, err := os.Stat(w.PromptFile); err != nil {
			return fmt.Errorf("worker %q: prompt file %q not found: %w", w.Name, w.PromptFile, err)
		}
	}

	result := &ops.BatchSpawnResult{
		Operation: "batch_spawn",
	}

	for i, w := range manifest.Workers {
		fmt.Printf("[%d/%d] Spawning: %s (model: %s)\n", i+1, len(manifest.Workers), w.Name, w.Model)

		if err := spawnWorker(w); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to spawn %s: %v", w.Name, err))
			result.Failed = append(result.Failed, ops.FailedWorker{
				Name:  w.Name,
				Error: err.Error(),
			})
			result.Summary.Failed++
		} else {
			ui.PrintSuccess(fmt.Sprintf("Spawned: %s", w.Name))
			result.Spawned = append(result.Spawned, ops.SpawnedWorker{
				Name:  w.Name,
				Model: w.Model,
			})
			result.Summary.Success++
		}
		result.Summary.Total++
	}

	return printResult(result, func() {
		printSpawnSummary(result)
	})
}

func spawnWorker(w ops.WorkerSpec) error {
	// Read prompt from file
	promptData, err := os.ReadFile(w.PromptFile)
	if err != nil {
		return fmt.Errorf("failed to read prompt file: %w", err)
	}
	promptText := strings.TrimSpace(string(promptData))
	if promptText == "" {
		return fmt.Errorf("prompt file is empty")
	}

	// Build agm session new command
	cmdArgs := []string{"session", "new", w.Name, "--detached", "--harness=claude-code"}
	if w.Model != "" {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--model=%s", w.Model))
	}
	cmdArgs = append(cmdArgs, fmt.Sprintf("--prompt=%s", promptText))

	agmCmd := exec.Command("agm", cmdArgs...)
	agmCmd.Env = os.Environ()
	out, err := agmCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, string(out))
	}

	return nil
}

func printSpawnSummary(result *ops.BatchSpawnResult) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Batch Spawn Summary (%d workers)\n", result.Summary.Total)
	fmt.Println(strings.Repeat("=", 60))

	for _, s := range result.Spawned {
		fmt.Printf("  [OK] %s (model: %s)\n", s.Name, s.Model)
	}
	for _, f := range result.Failed {
		fmt.Printf("  [FAIL] %s: %s\n", f.Name, f.Error)
	}

	fmt.Println()
	if result.Summary.Success > 0 {
		ui.PrintSuccess(fmt.Sprintf("%d worker(s) spawned", result.Summary.Success))
	}
	if result.Summary.Failed > 0 {
		ui.PrintWarning(fmt.Sprintf("%d worker(s) failed", result.Summary.Failed))
	}
}
