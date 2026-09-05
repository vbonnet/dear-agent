package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunPRScanNoChecksPolicyErrorStopsBeforeTrigger(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun := noCheckTrigger, noCheckDryRun
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun = oldTrigger, oldDryRun
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"candidate","headRefName":"fix/x","headRefOid":"abc123","isDraft":false}]' ;;
  *rules/branches*) printf '%s\n' 'gh: provider unavailable (HTTP 500)' >&2; exit 1 ;;
  *protection/required_status_checks*) printf '%s\n' '{"contexts":["Build"]}' ;;
  *) printf '%s\n' 'unexpected mutation or check read' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = "main"
	noCheckLimit = 10
	noCheckTrigger = true
	noCheckDryRun = false
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runPRScanNoChecks(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "required checks") {
		t.Fatalf("runPRScanNoChecks() error = %v, want required-policy failure", err)
	}
	if !cmd.SilenceUsage {
		t.Fatal("policy failure should silence command usage")
	}
	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	for _, requiredCall := range []string{"rules/branches", "protection/required_status_checks"} {
		if !strings.Contains(calls, requiredCall) {
			t.Fatalf("policy discovery omitted %q:\n%s", requiredCall, calls)
		}
	}
	for _, forbidden := range []string{"/check-runs", "/git/commits", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("policy failure reached forbidden provider call %q:\n%s", forbidden, calls)
		}
	}
}

func installNoChecksFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nset -eu\n" + body
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
