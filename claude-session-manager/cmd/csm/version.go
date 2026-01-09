package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version information (can be set via ldflags at build time)
	Version   = "2.0.0-dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version, git commit, build date, and Go version of csm.`,
	Run: func(cmd *cobra.Command, args []string) {
		executable, err := os.Executable()
		if err != nil {
			executable = "unknown"
		}
		fmt.Printf("csm version %s\n", Version)
		fmt.Printf("  binary: %s\n", executable)
		fmt.Printf("  git commit: %s\n", GitCommit)
		fmt.Printf("  built: %s\n", BuildDate)
		fmt.Printf("  go version: %s\n", runtime.Version())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
