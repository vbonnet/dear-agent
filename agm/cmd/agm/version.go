package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
)

var (
	// Version information (can be set via ldflags at build time)
	Version   = "2.0.0-dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	BuiltBy   = "unknown"
)

// populateVersionFromBuildInfo fills version vars from Go's embedded VCS info
// when ldflags were not used (e.g., plain `go install`).
func populateVersionFromBuildInfo() {
	if GitCommit != "unknown" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			GitCommit = s.Value
			if len(GitCommit) > 12 {
				GitCommit = GitCommit[:12]
			}
		case "vcs.time":
			BuildDate = s.Value
		case "vcs.modified":
			if s.Value == "true" && GitCommit != "unknown" {
				GitCommit += "-dirty"
			}
		}
	}
	if GitCommit != "unknown" {
		BuiltBy = "go install"
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version, git commit, build date, and Go version of agm.`,
	Run: func(cmd *cobra.Command, args []string) {
		executable, err := os.Executable()
		if err != nil {
			executable = "unknown"
		}
		fmt.Printf("agm version %s\n", Version)
		fmt.Printf("  binary: %s\n", executable)
		fmt.Printf("  git commit: %s\n", GitCommit)
		fmt.Printf("  built: %s by %s\n", BuildDate, BuiltBy)
		fmt.Printf("  go version: %s\n", runtime.Version())

		// Show freshness status
		repoPath, err := freshness.FindRepoPath()
		if err != nil {
			fmt.Printf("  freshness: unknown (%v)\n", err)
			return
		}
		result := freshness.Check(repoPath, GitCommit)
		if result.Error != nil {
			fmt.Printf("  freshness: unknown (%v)\n", result.Error)
		} else if result.Stale {
			fmt.Printf("  freshness: STALE (repo HEAD is %s)\n", result.RepoHEAD)
			fmt.Printf("             Run: make -C %s install\n", result.RepoPath)
		} else {
			fmt.Printf("  freshness: OK\n")
		}
	},
}

func init() {
	populateVersionFromBuildInfo()
	rootCmd.AddCommand(versionCmd)

	// Enable `agm --version` in addition to `agm version`
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate(fmt.Sprintf("agm version %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate))
}
