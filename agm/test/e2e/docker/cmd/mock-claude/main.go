// Command mock-claude stands in for the Claude Code harness in the reaper E2E.
//
// It must be built to a file literally named `claude`. AGM decides whether a
// session is alive by reading the pane process table and matching the process
// COMM against the harness (expectedHarnessProcessMatcher, "claude-code" ->
// base name "claude"). Linux takes COMM from the executable's file name, so an
// interpreted mock reports `python3` no matter what the script is called: the
// session computes as "zombie", `agm session archive --async` refuses with
// "--async should only be used for active sessions", and the reaper spawn the
// E2E means to exercise is never reached.
//
// Behaviour mirrors the harness contract the reaper depends on: print a
// banner, show the U+276F prompt the reaper's prompt detection looks for, and
// exit 0 on the native /exit command.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	stuck := flag.Bool("stuck", false, "Never show the prompt, to exercise the reaper's prompt-detection timeout")
	flag.Parse()
	if *stuck {
		fmt.Println("Mock Claude Code v1.0 (Stuck Simulation)")
		fmt.Println("Processing request... (will never finish)")
		for {
			time.Sleep(time.Hour)
		}
	}

	fmt.Println("Mock Claude Code v1.0 (E2E Test)")
	fmt.Println("Type /exit to quit")
	fmt.Println()
	fmt.Print("❯ ")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "/exit", "/quit":
			fmt.Println("Goodbye")
			return
		default:
			fmt.Print("❯ ")
		}
	}
}
