// sessionstart-chezmoi-drift detects when ~/.claude/settings.json has drifted
// from the chezmoi template. Runs as a SessionStart hook and warns via stderr
// if drift is detected. Always exits 0 (never blocks session start).
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	run()
}

func run() {
	// Hard timeout to avoid blocking session start
	done := make(chan struct{}, 1)
	go func() {
		check()
		done <- struct{}{}
	}()

	select {
	case <-done:
		return
	case <-time.After(3 * time.Second):
		return // timeout silently
	}
}

func check() {
	// Check if chezmoi is available
	chezmoiPath, err := exec.LookPath("chezmoi")
	if err != nil {
		return // chezmoi not installed, skip silently
	}

	// Run chezmoi diff on settings.json
	cmd := exec.Command(chezmoiPath, "diff", "--no-pager", os.ExpandEnv("$HOME/.claude/settings.json"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// chezmoi diff exits 0 if no diff, non-zero if diff exists; either way the diff is in stdout
	_ = cmd.Run()

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return // no drift
	}

	// Count changed lines (rough estimate)
	lines := strings.Split(output, "\n")
	additions := 0
	deletions := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}

	fmt.Fprintf(os.Stderr, "[chezmoi-drift] settings.json has drifted from chezmoi template (+%d/-%d lines). Run 'chezmoi diff ~/.claude/settings.json' to review, 'chezmoi apply --force ~/.claude/settings.json' to sync.\n",
		additions, deletions)
}
