package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/coordinator"
)

var (
	coordMaxConcurrent   int
	coordSandboxDir      string
	coordMonitorInterval time.Duration
	coordNoSandbox       bool
)

// coordCmd is the root command for coordination
var coordCmd = &cobra.Command{
	Use:   "coord",
	Short: "Coordinate multiple wayfinder projects",
	Long:  "Execute and monitor multiple wayfinder projects concurrently with sandbox isolation",
}

// coordStartCmd starts multiple projects
var coordStartCmd = &cobra.Command{
	Use:   "start <project1> <project2> [project3...]",
	Short: "Start multiple projects concurrently",
	Long: `Start multiple wayfinder projects concurrently with automatic sandbox isolation.

Examples:
  wayfinder-session coord start oss-wp12/ oss-wp13/
  wayfinder-session coord start --max-concurrent=2 project1/ project2/ project3/
  wayfinder-session coord start --no-sandbox project1/ project2/`,
	Args: cobra.MinimumNArgs(2),
	RunE: runCoordStart,
}

// coordStatusCmd shows status of running projects
var coordStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all running projects",
	Long:  "Display current status, phase, and progress of all coordinated projects",
	RunE:  runCoordStatus,
}

// coordStopCmd stops all running projects
var coordStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running projects gracefully",
	Long:  "Send SIGTERM to all running projects and wait for graceful shutdown",
	RunE:  runCoordStop,
}

func init() {
	// Add flags to coordStartCmd
	coordStartCmd.Flags().IntVar(&coordMaxConcurrent, "max-concurrent", 4, "Maximum concurrent projects")
	coordStartCmd.Flags().StringVar(&coordSandboxDir, "sandbox-dir", filepath.Join(os.Getenv("HOME"), ".wayfinder", "sandboxes"), "Sandbox directory")
	coordStartCmd.Flags().DurationVar(&coordMonitorInterval, "monitor-interval", 10*time.Second, "Status polling interval")
	coordStartCmd.Flags().BoolVar(&coordNoSandbox, "no-sandbox", false, "Disable sandbox isolation")

	// Add subcommands
	coordCmd.AddCommand(coordStartCmd)
	coordCmd.AddCommand(coordStatusCmd)
	coordCmd.AddCommand(coordStopCmd)

	// Register with root command (assumes rootCmd is defined in root.go)
	// This will be called by main.go's init
}

// GetCoordCmd returns the coord command for registration
func GetCoordCmd() *cobra.Command {
	return coordCmd
}

func runCoordStart(cmd *cobra.Command, args []string) error {
	projectDirs := args

	// Validate project directories
	for _, dir := range projectDirs {
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("invalid project directory %s: %w", dir, err)
		}
	}

	// Create config
	cfg := coordinator.Config{
		MaxConcurrent:   coordMaxConcurrent,
		SandboxDir:      coordSandboxDir,
		MonitorInterval: coordMonitorInterval,
		NoSandbox:       coordNoSandbox,
	}

	// Create sandbox manager (simplified for MVP)
	var sandboxMgr coordinator.SandboxManager
	if !coordNoSandbox {
		sandboxMgr = newSimpleSandboxManager(coordSandboxDir)
	}

	// Create coordinator
	coord := coordinator.NewCoordinator(cfg, sandboxMgr)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt signal, stopping projects...")
		cancel()
		coord.Stop(context.Background())
		os.Exit(130)
	}()

	// Print startup message
	fmt.Printf("Starting %d projects with max concurrency: %d\n\n", len(projectDirs), coordMaxConcurrent)

	// Subscribe to events for terminal output
	coord.GetMonitor().Subscribe(coordinator.EventProjectStarted, func(e coordinator.Event) {
		fmt.Printf("✓ Started: %s\n", filepath.Base(e.ProjectDir))
	})

	coord.GetMonitor().Subscribe(coordinator.EventProjectCompleted, func(e coordinator.Event) {
		fmt.Printf("✓ Completed: %s\n", filepath.Base(e.ProjectDir))
	})

	coord.GetMonitor().Subscribe(coordinator.EventProjectFailed, func(e coordinator.Event) {
		fmt.Printf("✗ Failed: %s - %v\n", filepath.Base(e.ProjectDir), e.Error)
	})

	// Start status display (simple for MVP)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				displayStatus(coord)
			}
		}
	}()

	// Start coordinator
	err := coord.Start(ctx, projectDirs)

	// Print final status
	fmt.Println("\n" + strings.Repeat("=", 60))
	displayFinalStatus(coord)

	return err
}

func runCoordStatus(cmd *cobra.Command, args []string) error {
	// For MVP, this would connect to a running coordinator instance
	// For now, just print a message
	fmt.Println("Status command not yet implemented (requires persistent coordinator)")
	fmt.Println("Use `coord start` to run projects and monitor in real-time")
	return nil
}

func runCoordStop(cmd *cobra.Command, args []string) error {
	// For MVP, this would connect to a running coordinator instance
	// For now, just print a message
	fmt.Println("Stop command not yet implemented (requires persistent coordinator)")
	fmt.Println("Use Ctrl+C to stop projects started with `coord start`")
	return nil
}

func displayStatus(coord *coordinator.Coordinator) {
	status := coord.Status()
	if len(status) == 0 {
		return
	}

	fmt.Print("\033[H\033[2J") // Clear screen
	fmt.Println("Project Status")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-30s %-15s %-10s %s\n", "Project", "Status", "Phase", "Time")
	fmt.Println(strings.Repeat("-", 80))

	for _, proj := range status {
		elapsed := ""
		if !proj.StartedAt.IsZero() {
			if proj.CompletedAt.IsZero() {
				elapsed = time.Since(proj.StartedAt).Round(time.Second).String()
			} else {
				elapsed = proj.CompletedAt.Sub(proj.StartedAt).Round(time.Second).String()
			}
		}

		// Get phase from monitor
		phase := "unknown"
		if projStatus, err := coord.GetMonitor().GetStatus(proj.ProjectDir); err == nil {
			phase = projStatus.CurrentPhase
		}

		fmt.Printf("%-30s %-15s %-10s %s\n",
			filepath.Base(proj.ProjectDir),
			proj.Status,
			phase,
			elapsed,
		)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Press Ctrl+C to stop all projects")
}

func displayFinalStatus(coord *coordinator.Coordinator) {
	status := coord.Status()

	completed := 0
	failed := 0
	cancelled := 0

	for _, proj := range status {
		switch proj.Status {
		case coordinator.StatusCompleted:
			completed++
		case coordinator.StatusFailed:
			failed++
		case coordinator.StatusCancelled:
			cancelled++
		case coordinator.StatusQueued, coordinator.StatusRunning:
			// Still in progress, not counted in final tallies
		}
	}

	fmt.Println("Final Status:")
	fmt.Printf("  Completed: %d\n", completed)
	fmt.Printf("  Failed:    %d\n", failed)
	fmt.Printf("  Cancelled: %d\n", cancelled)
	fmt.Printf("  Total:     %d\n", len(status))
}

// SimpleSandboxManager is a minimal implementation for MVP
type SimpleSandboxManager struct {
	baseDir string
}

func newSimpleSandboxManager(baseDir string) *SimpleSandboxManager {
	return &SimpleSandboxManager{baseDir: baseDir}
}

func (sm *SimpleSandboxManager) CreateSandbox(name string) (*coordinator.Sandbox, error) {
	// For MVP, just create a directory (no git worktree)
	// Full implementation would use wp11 sandbox package
	id := fmt.Sprintf("%d", time.Now().Unix())
	sandboxPath := filepath.Join(sm.baseDir, id)

	if err := os.MkdirAll(sandboxPath, 0755); err != nil {
		return nil, err
	}

	return &coordinator.Sandbox{
		ID:   id,
		Name: name,
	}, nil
}

func (sm *SimpleSandboxManager) ListSandboxes() ([]*coordinator.Sandbox, error) {
	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*coordinator.Sandbox{}, nil
		}
		return nil, err
	}

	var sandboxes []*coordinator.Sandbox
	for _, entry := range entries {
		if entry.IsDir() {
			sandboxes = append(sandboxes, &coordinator.Sandbox{
				ID:   entry.Name(),
				Name: entry.Name(),
			})
		}
	}

	return sandboxes, nil
}

func (sm *SimpleSandboxManager) CleanupSandbox(nameOrID string) error {
	sandboxPath := filepath.Join(sm.baseDir, nameOrID)
	return os.RemoveAll(sandboxPath)
}
