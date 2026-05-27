// dear-agent-bumblebee — installer + scan wrapper for the Bumblebee endpoint
// scanner. Replaces the bash trio (bumblebee-install.sh, bumblebee-scan.sh,
// install-bumblebee-launchagent.sh) that ran afoul of the repo's
// bash-20-line-limit policy. Per ADR-027.
//
// Subcommands:
//
//	install              fetch a pinned, SHA-256-verified Bumblebee release
//	install-launchagent  install (or refresh) the macOS LaunchAgent
//	scan                 run a Bumblebee scan with dear-agent defaults
//	version              print the pinned Bumblebee version
//	help                 print this help
//
// Each subcommand accepts `-h` for details.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "install":
		err = runInstall(os.Args[2:])
	case "install-launchagent":
		err = runInstallLaunchAgent(os.Args[2:])
	case "scan":
		err = runScan(os.Args[2:])
	case "version", "--version":
		fmt.Println(BumblebeeVersion)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `dear-agent-bumblebee — installer + scan wrapper for Bumblebee (ADR-027).

Usage:
  dear-agent-bumblebee <subcommand> [flags]

Subcommands:
  install              Fetch a pinned, SHA-256-verified Bumblebee release.
                       Flags: --prefix DIR   (default $BUMBLEBEE_PREFIX or ~/.local/bin)
                              --uninstall    Remove the installed binary instead.

  install-launchagent  Install (or refresh) the per-user macOS LaunchAgent.
                       Flags: --uninstall    Remove the LaunchAgent.
                              --status       Show launchctl state and exit.

  scan                 Run a Bumblebee scan and write NDJSON to the per-user
                       data dir.
                       Flags: --profile NAME  (default "project")
                              --root DIR
                              --output FILE

  version              Print the pinned Bumblebee version.
`)
}
