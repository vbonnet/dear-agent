package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Manage recurring background loops",
	Long: `Manage persistent, named loops that run a command on a cadence.

Loops are the primary UX for recurring background work: babysit PRs, watch CI,
scan dependencies. Each loop stores its definition and run history in SQLite
(~/.agm/loops.db), so every execution is queryable.

Examples:
  agm loop new babysit-prs --cadence 5m --cmd "gh pr list --author @me"
  agm loop list
  agm loop run babysit-prs        # run immediately (once)
  agm loop logs babysit-prs       # recent run history
  agm loop pause babysit-prs
  agm loop resume babysit-prs
  agm loop tick                   # run all due loops (wire to cron)
  agm loop delete babysit-prs`,
}

var (
	loopNewCadence     string
	loopNewCmdFlag     string
	loopNewDescription string
	loopLogsLimit      int
)

var loopNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new recurring loop",
	Long: `Create a named, recurring loop that runs a bash command on a cadence.

The cadence is a Go duration string (5m, 1h, 30s, 24h, ...).
The command runs via /bin/sh -c, inheriting the caller's environment.

The loop does not run immediately — use 'agm loop run <name>' to trigger it
now, or wait for the cadence to fire via 'agm loop tick'.

Examples:
  agm loop new babysit-prs --cadence 5m --cmd "gh pr list"
  agm loop new nightly-scan --cadence 8h --cmd "./scripts/vuln-scan.sh" \
      --description "Nightly dependency vulnerability scan"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cadence, err := time.ParseDuration(loopNewCadence)
		if err != nil {
			return fmt.Errorf("invalid cadence %q: must be a Go duration like 5m, 1h, 30s", loopNewCadence)
		}

		store, err := openLoopStore()
		if err != nil {
			return err
		}
		defer store.Close()

		l, err := store.CreateLoop(cmd.Context(), name, loopNewDescription, loopNewCmdFlag, cadence)
		if err != nil {
			return err
		}

		fmt.Printf("Loop %q created.\n", l.Name)
		fmt.Printf("  cadence:     %s\n", l.Cadence)
		fmt.Printf("  cmd:         %s\n", l.Cmd)
		if l.Description != "" {
			fmt.Printf("  description: %s\n", l.Description)
		}
		if l.NextRunAt != nil {
			fmt.Printf("  first due:   %s\n", l.NextRunAt.Format(time.RFC3339))
		}
		fmt.Printf("\nRun it now:   agm loop run %s\n", name)
		fmt.Printf("Wire to cron: * * * * * agm loop tick\n")
		return nil
	},
}

var loopListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all loops",
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := openLoopStore()
		if err != nil {
			return err
		}
		defer store.Close()

		loops, err := store.ListLoops(cmd.Context())
		if err != nil {
			return err
		}

		if len(loops) == 0 {
			fmt.Println("No loops defined.")
			fmt.Println(`Create one: agm loop new <name> --cadence 5m --cmd "..."`)
			return nil
		}

		for _, l := range loops {
			lastRun := "never"
			if l.LastRunAt != nil {
				lastRun = l.LastRunAt.Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-22s  %-6s  cadence=%-8s  runs=%-4d  last=%s\n",
				l.Name, l.Status, l.Cadence, l.RunCount, lastRun)
			if l.Description != "" {
				fmt.Printf("  %s\n", l.Description)
			}
		}
		return nil
	},
}

var loopRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Execute a loop immediately (once)",
	Long: `Run a loop's command right now, outside its normal cadence.

Stdout and stderr stream to the terminal. next_run_at advances by one cadence
period from when this run finishes, so the scheduled cadence resumes correctly.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		store, err := openLoopStore()
		if err != nil {
			return err
		}
		defer store.Close()

		fmt.Printf("Running loop %q...\n", name)
		r, err := store.RunLoop(cmd.Context(), name)
		if err != nil {
			return err
		}

		if r.Stdout != "" {
			fmt.Print(r.Stdout)
		}
		if r.Stderr != "" {
			fmt.Fprint(os.Stderr, r.Stderr)
		}

		if r.Success {
			dur := ""
			if r.FinishedAt != nil {
				dur = " in " + r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond).String()
			}
			fmt.Printf("Loop run succeeded%s.\n", dur)
			return nil
		}
		code := -1
		if r.ExitCode != nil {
			code = *r.ExitCode
		}
		return fmt.Errorf("loop run failed (exit %d)", code)
	},
}

var loopLogsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Show recent run history for a loop",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		store, err := openLoopStore()
		if err != nil {
			return err
		}
		defer store.Close()

		runs, err := store.GetRuns(cmd.Context(), name, loopLogsLimit)
		if err != nil {
			return err
		}

		if len(runs) == 0 {
			fmt.Printf("No runs recorded for loop %q.\n", name)
			return nil
		}

		for _, r := range runs {
			status := "[OK]  "
			if !r.Success {
				status = "[FAIL]"
			}
			dur := "    "
			if r.FinishedAt != nil {
				dur = r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond).String()
			}
			exitStr := ""
			if !r.Success && r.ExitCode != nil {
				exitStr = fmt.Sprintf(" exit=%d", *r.ExitCode)
			}
			fmt.Printf("%s  %s  %s%s\n",
				status, r.StartedAt.Format("2006-01-02 15:04:05"), dur, exitStr)
			if !r.Success && r.Stderr != "" {
				for _, line := range strings.Split(strings.TrimSpace(r.Stderr), "\n") {
					fmt.Printf("         stderr: %s\n", line)
				}
			}
		}
		return nil
	},
}

var loopPauseCmd = &cobra.Command{
	Use:   "pause <name>",
	Short: "Pause a loop (skip by tick until resumed)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setLoopStatus(cmd, args[0], ops.LoopStatusPaused)
	},
}

var loopResumeCmd = &cobra.Command{
	Use:   "resume <name>",
	Short: "Resume a paused loop",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setLoopStatus(cmd, args[0], ops.LoopStatusActive)
	},
}

var loopDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a loop and its entire run history",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		store, err := openLoopStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.DeleteLoop(cmd.Context(), name); err != nil {
			return err
		}
		fmt.Printf("Loop %q deleted.\n", name)
		return nil
	},
}

var loopTickCmd = &cobra.Command{
	Use:   "tick",
	Short: "Run all active loops that are due (wire to cron)",
	Long: `Check all active loops and execute any whose next_run_at is in the past.

Designed to be called from a cron job or launchd timer. Exit code is non-zero
if any loop fails, so the scheduler can detect and alert on failures.

crontab entry (run tick every minute, execute whatever is due):
  * * * * * agm loop tick

macOS launchd: set StartInterval to 60 in the plist.

Tick is silent when there is nothing due, making it safe to call frequently.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := openLoopStore()
		if err != nil {
			return err
		}
		defer store.Close()

		due, err := store.DueLoops(cmd.Context())
		if err != nil {
			return err
		}
		if len(due) == 0 {
			return nil // nothing due; exit 0 cleanly
		}

		var failed int
		for _, l := range due {
			r, runErr := store.RunLoop(cmd.Context(), l.Name)
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "loop %q: run error: %v\n", l.Name, runErr)
				failed++
				continue
			}
			if !r.Success {
				code := -1
				if r.ExitCode != nil {
					code = *r.ExitCode
				}
				fmt.Fprintf(os.Stderr, "loop %q: exited %d\n", l.Name, code)
				failed++
			}
		}

		if failed > 0 {
			return fmt.Errorf("%d loop(s) failed", failed)
		}
		return nil
	},
}

func setLoopStatus(cmd *cobra.Command, name string, status ops.LoopStatus) error {
	store, err := openLoopStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.SetStatus(cmd.Context(), name, status); err != nil {
		return err
	}
	fmt.Printf("Loop %q is now %s.\n", name, status)
	return nil
}

func openLoopStore() (*ops.LoopStore, error) {
	s, err := ops.OpenLoopStore(ops.LoopStorePath())
	if err != nil {
		return nil, fmt.Errorf("cannot open loop store: %w", err)
	}
	return s, nil
}

func init() {
	rootCmd.AddCommand(loopCmd)
	loopCmd.AddCommand(loopNewCmd)
	loopCmd.AddCommand(loopListCmd)
	loopCmd.AddCommand(loopRunCmd)
	loopCmd.AddCommand(loopLogsCmd)
	loopCmd.AddCommand(loopPauseCmd)
	loopCmd.AddCommand(loopResumeCmd)
	loopCmd.AddCommand(loopDeleteCmd)
	loopCmd.AddCommand(loopTickCmd)

	loopNewCmd.Flags().StringVar(&loopNewCadence, "cadence", "", "How often to run (Go duration: 5m, 1h, 30s) [required]")
	_ = loopNewCmd.MarkFlagRequired("cadence")
	loopNewCmd.Flags().StringVar(&loopNewCmdFlag, "cmd", "", "Bash command to run [required]")
	_ = loopNewCmd.MarkFlagRequired("cmd")
	loopNewCmd.Flags().StringVar(&loopNewDescription, "description", "", "Human-readable description")

	loopLogsCmd.Flags().IntVar(&loopLogsLimit, "limit", 20, "Maximum number of runs to show")
}
