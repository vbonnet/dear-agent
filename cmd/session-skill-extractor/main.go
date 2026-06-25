// session-skill-extractor reads a completed session transcript and proposes a
// new SKILL candidate by calling the model. Includes dedup against existing
// skills and an anti-pattern guard (what does the new skill replace/reduce?).
//
// Output is always a reviewable candidate — never auto-committed.
//
// Usage: session-skill-extractor [--session <id>] [--output <dir>] [--dry-run]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("session-skill-extractor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	home := os.Getenv("HOME")
	var (
		sessionID   = fs.String("session", "", "session ID to analyze (default: latest session in projects-dir)")
		outputDir   = fs.String("output", "/tmp/skill-candidates/", "directory to write candidate skill file")
		dryRun      = fs.Bool("dry-run", false, "print proposal without writing any files")
		skillsDir   = fs.String("skills-dir", home+"/.claude/skills", "directory containing existing skills for dedup")
		projectsDir = fs.String("projects-dir", home+"/.claude/projects", "root directory containing session transcripts")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: session-skill-extractor [--session <id>] [--output <dir>] [--dry-run]\n\n"+
			"Analyzes a session transcript and proposes a new reusable SKILL candidate.\n"+
			"Deduplicates against existing skills. Identifies what the new skill could\n"+
			"replace or retire (anti-pattern guard). Output is never auto-committed.\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(stderr, "error: ANTHROPIC_API_KEY environment variable is required")
		return 1
	}

	cfg := Config{
		SessionID:   *sessionID,
		SkillsDir:   *skillsDir,
		ProjectsDir: *projectsDir,
		OutputDir:   *outputDir,
		DryRun:      *dryRun,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := Extract(ctx, cfg, newAnthropicCaller(apiKey))
	if errors.Is(err, ErrEmptyTranscript) {
		fmt.Fprintln(stdout, "nothing to extract: transcript is empty")
		return 0
	}
	if errors.Is(err, ErrNothingNew) {
		fmt.Fprintln(stdout, "model says: nothing new — all procedures are already covered by existing skills")
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "skill:    %s\n", result.SkillName)
	fmt.Fprintf(stdout, "replaces: %s\n", result.Replaces)
	fmt.Fprintf(stdout, "why-new:  %s\n", result.WhyNew)
	if *dryRun {
		fmt.Fprintf(stdout, "\n--- SKILL.md (dry-run, not written) ---\n%s\n--- END ---\n", result.Content)
	} else {
		fmt.Fprintf(stdout, "written:  %s\n", result.OutputPath)
	}
	return 0
}
