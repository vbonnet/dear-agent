package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/internal/prlinkify"
)

func main() {
	repo := flag.String("repo", "", "default GitHub repo as OWNER/REPO (default: vbonnet/dear-agent)")
	hookMode := flag.Bool("hook", false, "hook adapter mode: read hook JSON from stdin, emit context JSON")
	summary := flag.Bool("summary", false, "output a summary of found PR refs instead of linkified text")
	flag.Parse()

	if envRepo := os.Getenv("PR_LINKIFY_REPO"); envRepo != "" && *repo == "" {
		*repo = envRepo
	}

	cfg := prlinkify.Config{DefaultRepo: *repo}

	if *hookMode {
		runHook(cfg)
		return
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pr-linkify: read stdin: %v\n", err)
		os.Exit(1)
	}

	text := string(data)
	if *summary {
		printSummary(text, cfg)
		return
	}
	fmt.Print(prlinkify.Linkify(text, cfg))
}

func runHook(cfg prlinkify.Config) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}

	var input struct {
		Message    string `json:"message"`
		Transcript string `json:"transcript"`
		Output     string `json:"output"`
		ToolInput  struct {
			Message    string `json:"message"`
			Transcript string `json:"transcript"`
			Output     string `json:"output"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return
	}

	text := firstNonEmpty(
		input.Transcript, input.Output, input.Message,
		input.ToolInput.Transcript, input.ToolInput.Output, input.ToolInput.Message,
	)
	if text == "" {
		return
	}

	refs := prlinkify.FindRefs(text, cfg)
	if len(refs) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("PR links from this session:\n")
	for _, ref := range refs {
		fmt.Fprintf(&sb, "- [PR #%s](%s)\n", ref.Number, ref.URL)
	}

	hookOut := map[string]any{
		"additional_context": sb.String(),
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "Stop",
			"additionalContext": sb.String(),
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(hookOut)
}

func printSummary(text string, cfg prlinkify.Config) {
	refs := prlinkify.FindRefs(text, cfg)
	if len(refs) == 0 {
		return
	}
	for _, ref := range refs {
		fmt.Printf("[PR #%s](%s)\n", ref.Number, ref.URL)
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
