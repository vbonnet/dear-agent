package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildGeminiResearchArgs(t *testing.T) {
	args := buildGeminiResearchArgs("what is RAG?", geminiResearchOptions{
		effort: "high", model: "gemini-x", timeout: 90 * time.Second,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--print", "--dangerously-skip-permissions", "--disable-slash-commands",
		"--print-timeout 1m30s", "--effort high", "--model gemini-x", "-p",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	// The prompt must be the final argument (after -p), never interpreted as a flag.
	if args[len(args)-1] != "what is RAG?" || args[len(args)-2] != "-p" {
		t.Errorf("prompt must trail as `-p <prompt>`, got %v", args)
	}
}

func TestBuildGeminiResearchArgsOmitsUnsetOptionals(t *testing.T) {
	args := buildGeminiResearchArgs("hi", geminiResearchOptions{})
	joined := strings.Join(args, " ")
	for _, unwanted := range []string{"--effort", "--model", "--print-timeout"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("args %q should omit %q when unset", joined, unwanted)
		}
	}
}

func TestRunGeminiResearchSuccess(t *testing.T) {
	var gotAgy string
	var gotArgs []string
	run := func(_ context.Context, agyPath string, args []string, _ string) ([]byte, error) {
		gotAgy = agyPath
		gotArgs = args
		return []byte("  RAG augments an LLM with retrieved context. DONE-RAG\n"), nil
	}
	out, err := runGeminiResearch(context.Background(), "  what is RAG?  ", geminiResearchOptions{
		agyPath: "/fake/agy", timeout: time.Minute,
	}, run)
	if err != nil {
		t.Fatalf("runGeminiResearch: %v", err)
	}
	if out != "RAG augments an LLM with retrieved context. DONE-RAG" {
		t.Errorf("output not trimmed: %q", out)
	}
	if gotAgy != "/fake/agy" {
		t.Errorf("agy path = %q, want /fake/agy", gotAgy)
	}
	// Prompt is trimmed before being passed through.
	if gotArgs[len(gotArgs)-1] != "what is RAG?" {
		t.Errorf("prompt arg = %q, want trimmed 'what is RAG?'", gotArgs[len(gotArgs)-1])
	}
}

func TestRunGeminiResearchEmptyPrompt(t *testing.T) {
	called := false
	run := func(context.Context, string, []string, string) ([]byte, error) {
		called = true
		return nil, nil
	}
	if _, err := runGeminiResearch(context.Background(), "   ", geminiResearchOptions{agyPath: "/fake/agy"}, run); err == nil {
		t.Fatal("expected error for empty prompt")
	}
	if called {
		t.Fatal("must not invoke agy for an empty prompt")
	}
}

func TestRunGeminiResearchExecErrorSurfacesOutput(t *testing.T) {
	run := func(context.Context, string, []string, string) ([]byte, error) {
		return []byte("account eligibility verification pending"), errors.New("exit status 1")
	}
	out, err := runGeminiResearch(context.Background(), "p", geminiResearchOptions{agyPath: "/fake/agy"}, run)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "eligibility") {
		t.Errorf("error should include agy output, got %v", err)
	}
	if out != "account eligibility verification pending" {
		t.Errorf("partial output = %q", out)
	}
}

func TestResolveGeminiResearchPrompt(t *testing.T) {
	if got, _ := resolveGeminiResearchPrompt([]string{"what", "is", "RAG"}, "", strings.NewReader("")); got != "what is RAG" {
		t.Errorf("args prompt = %q", got)
	}
	if got, _ := resolveGeminiResearchPrompt(nil, "", strings.NewReader("piped prompt")); got != "piped prompt" {
		t.Errorf("stdin prompt = %q", got)
	}
}
