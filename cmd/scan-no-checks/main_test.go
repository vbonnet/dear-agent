package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFlags_Defaults(t *testing.T) {
	cfg, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags(nil) error: %v", err)
	}
	if cfg.limit != 100 {
		t.Errorf("default limit = %d, want 100", cfg.limit)
	}
	if cfg.trigger || cfg.dryRun || cfg.jsonOut {
		t.Errorf("boolean flags should default false, got %+v", cfg)
	}
}

func TestParseFlags_Values(t *testing.T) {
	cfg, err := parseFlags([]string{"--repo", "a/b", "--limit", "5", "--trigger", "--json", "--dry-run"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if cfg.repo != "a/b" || cfg.limit != 5 || !cfg.trigger || !cfg.jsonOut || !cfg.dryRun {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
}

func TestParseFlags_BadLimit(t *testing.T) {
	if _, err := parseFlags([]string{"--limit", "0"}); err == nil {
		t.Error("expected error for --limit 0")
	}
	if _, err := parseFlags([]string{"--limit", "abc"}); err == nil {
		t.Error("expected error for non-integer --limit")
	}
}

func TestParseFlags_UnexpectedArg(t *testing.T) {
	if _, err := parseFlags([]string{"extra"}); err == nil {
		t.Error("expected error for unexpected positional argument")
	}
}

func TestParseRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/vbonnet/dear-agent.git": "vbonnet/dear-agent",
		"https://github.com/vbonnet/dear-agent":     "vbonnet/dear-agent",
		"git@github.com:vbonnet/dear-agent.git":     "vbonnet/dear-agent",
	}
	for raw, want := range cases {
		got, err := parseRepoFromURL(raw)
		if err != nil {
			t.Errorf("parseRepoFromURL(%q) error: %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("parseRepoFromURL(%q) = %q, want %q", raw, got, want)
		}
	}
	if _, err := parseRepoFromURL("https://gitlab.com/x/y.git"); err == nil {
		t.Error("expected error for non-GitHub URL")
	}
}

func TestReport_Table(t *testing.T) {
	var buf bytes.Buffer
	stuck := []result{
		{Number: 582, Title: "fix thing", HeadRefName: "feat/x", HeadSHA: "cebb82eb05bea83", Triggered: true},
		{Number: 581, Title: "other", HeadRefName: "feat/y", HeadSHA: "deadbeefcafe", Error: "boom"},
	}
	if err := report(&buf, config{}, stuck); err != nil {
		t.Fatalf("report error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "#582") || !strings.Contains(out, "re-triggered") {
		t.Errorf("table missing triggered row: %s", out)
	}
	if !strings.Contains(out, "re-trigger FAILED: boom") {
		t.Errorf("table missing failure row: %s", out)
	}
}

func TestReport_TableEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := report(&buf, config{}, nil); err != nil {
		t.Fatalf("report error: %v", err)
	}
	if !strings.Contains(buf.String(), "no stuck PRs") {
		t.Errorf("empty table should say no stuck PRs, got: %s", buf.String())
	}
}

func TestReport_JSONEmptyIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := report(&buf, config{jsonOut: true}, nil); err != nil {
		t.Fatalf("report error: %v", err)
	}
	var got []result
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON output is not a valid array: %v\n%s", err, buf.String())
	}
	if len(got) != 0 {
		t.Errorf("expected empty array, got %d entries", len(got))
	}
}

func TestReport_JSONRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	stuck := []result{{Number: 1, Title: "t", HeadRefName: "b", HeadSHA: "abc1234", Triggered: true}}
	if err := report(&buf, config{jsonOut: true}, stuck); err != nil {
		t.Fatalf("report error: %v", err)
	}
	var got []result
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 || got[0].Number != 1 || !got[0].Triggered {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("cebb82eb05bea83"); got != "cebb82e" {
		t.Errorf("shortSHA long = %q, want cebb82e", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("shortSHA short = %q, want abc", got)
	}
}
