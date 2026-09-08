package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowDelegatesIssueLifecycleToCommand(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "security-audit.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	hygieneStart := strings.Index(workflow, "  workflow-hygiene:")
	if hygieneStart < 0 {
		t.Fatal("workflow-hygiene job is absent")
	}
	hygiene := workflow[hygieneStart:]
	reconcileStart := strings.Index(hygiene, "      - name: Reconcile security-audit issue")
	if reconcileStart < 0 {
		t.Fatal("workflow reconciliation step is absent")
	}
	reconcileStep := hygiene[reconcileStart:]
	if got := strings.Count(hygiene, "go run ./tools/security-audit-issues"); got != 1 {
		t.Fatalf("workflow command delegation count = %d, want 1", got)
	}
	if !strings.Contains(reconcileStep, "if: ${{ github.event_name == 'schedule' || github.ref == format('refs/heads/{0}', github.event.repository.default_branch) }}") {
		t.Error("workflow reconciliation is not restricted to the repository default branch")
	}
	for _, forbidden := range []string{"gh label create", "gh issue create", "gh issue comment", "gh issue edit", "gh issue close"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("workflow still owns provider mutation %q", forbidden)
		}
	}
	for _, required := range []string{
		"actions/setup-go@v7",
		"GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
		"REPO: ${{ github.repository }}",
	} {
		if !strings.Contains(hygiene, required) {
			t.Errorf("workflow omits %q", required)
		}
	}
	for _, required := range []string{"group: security-audit-issue-reconciliation-${{ github.ref }}", "cancel-in-progress: false"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow omits %q", required)
		}
	}
}
