package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
)

// Version, GitCommit, BuildDate, BuiltBy are package-level aliases kept for
// backward compatibility with code inside this cmd that references them
// (admin_verify_deployment.go, main.go). Their values are set by init() after
// pkg/version vars are populated from ldflags or build info.
var (
	Version   string
	GitCommit string
	BuildDate string
	BuiltBy   string
)

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
	// Populate pkg/version from embedded VCS info when ldflags were not used.
	pkgversion.PopulateFromBuildInfo()

	// Mirror pkg/version vars into the local aliases used by this cmd.
	Version = pkgversion.Version
	GitCommit = pkgversion.GitCommit
	BuildDate = pkgversion.BuildDate
	BuiltBy = pkgversion.BuiltBy

	rootCmd.AddCommand(versionCmd)

	// Enable `agm --version` in addition to `agm version`
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate(fmt.Sprintf("agm version %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate))
}
