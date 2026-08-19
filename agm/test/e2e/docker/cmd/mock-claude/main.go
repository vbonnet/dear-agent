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
	"io"
	"os"
	"strings"
	"time"
)

// prompt is U+276F, the character the reaper's prompt detection matches.
const prompt = "❯ "

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

	serve(os.Stdin, os.Stdout)
}

// serve is the harness contract the reaper depends on: a banner, the U+276F
// prompt its prompt detection looks for, and a clean exit on the native
// shutdown command. Split out from main so the contract is testable without a
// pty.
func serve(in io.Reader, out io.Writer) {
	fmt.Fprintln(out, "Mock Claude Code v1.0 (E2E Test)")
	fmt.Fprintln(out, "Type /exit to quit")
	fmt.Fprintln(out)
	fmt.Fprint(out, prompt)

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		switch strings.TrimSpace(scanner.Text()) {
		case "/exit", "/quit":
			fmt.Fprintln(out, "Goodbye")
			return
		default:
			fmt.Fprint(out, prompt)
		}
	}
}
