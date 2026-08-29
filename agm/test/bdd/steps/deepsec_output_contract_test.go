package steps

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestDeepsecGeneratedStateDoesNotDirtyWorktree(t *testing.T) {
	t.Parallel()

	ignoreSource := filepath.Join(packageSpecBDDRepoRoot(), ".deepsec", ".gitignore")
	ignoreRules, err := os.ReadFile(ignoreSource)
	if err != nil {
		t.Fatalf("read %s: %v", ignoreSource, err)
	}

	sandbox := gittest.New(t)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatalf("create temporary repository: %v", err)
	}
	sandbox.Run(t, repo, "init")

	living := map[string]string{
		".deepsec/deepsec.config.ts":             "export default { projects: [] };\n",
		".deepsec/data/deepsec-scan/INFO.md":     "# Security context\n",
		".deepsec/data/deepsec-scan/SETUP.md":    "# Agent setup\n",
		".deepsec/data/deepsec-scan/config.json": "{}\n",
	}
	writeDeepsecContractFile(t, repo, ".deepsec/.gitignore", string(ignoreRules))
	for path, contents := range living {
		writeDeepsecContractFile(t, repo, path, contents)
	}
	sandbox.Run(t, repo, "add", "--", ".deepsec")
	sandbox.Run(t, repo, "commit", "-m", "seed living Deepsec inputs")

	generated := map[string]string{
		".deepsec/data/deepsec-scan/project.json": "{}\n",
		".deepsec/data/deepsec-scan/tech.json": `{
  "detectedAt": "2026-08-29T15:06:39.192Z",
  "rootPath": "/Users/example/worktrees/dear-agent"
}
`,
		".deepsec/data/deepsec-scan/files/cmd/agent.go.json": "{}\n",
		".deepsec/data/deepsec-scan/runs/scan.json":          "{}\n",
		".deepsec/data/deepsec-scan/reports/report.md":       "# Report\n",
		".deepsec/findings/deepsec-scan.md":                  "# Findings\n",
	}
	for path, contents := range generated {
		writeDeepsecContractFile(t, repo, path, contents)
	}

	for path := range generated {
		output, checkErr := sandbox.Output(repo, "check-ignore", "--quiet", "--no-index", "--", path)
		if checkErr != nil {
			t.Errorf("generated path %s is visible to Git: %v\n%s", path, checkErr, output)
		}
	}
	if status := strings.TrimSpace(sandbox.Run(t, repo, "status", "--porcelain=v1", "--untracked-files=all", "--", ".deepsec")); status != "" {
		t.Fatalf("generated Deepsec state dirtied the worktree:\n%s", status)
	}

	for path := range living {
		output, checkErr := sandbox.Output(repo, "check-ignore", "--quiet", "--no-index", "--", path)
		if checkErr == nil {
			t.Errorf("living Deepsec input %s is ignored", path)
			continue
		}
		var exitErr *exec.ExitError
		if !errors.As(checkErr, &exitErr) || exitErr.ExitCode() != 1 {
			t.Fatalf("classify living Deepsec input %s: %v\n%s", path, checkErr, output)
		}
	}

	const callerOwnedFindings = ".deepsec/custom/findings/finding.md"
	writeDeepsecContractFile(t, repo, callerOwnedFindings, "# Caller-owned export\n")
	if status := strings.TrimSpace(sandbox.Run(t, repo, "status", "--porcelain=v1", "--untracked-files=all", "--", callerOwnedFindings)); status != "?? "+callerOwnedFindings {
		t.Fatalf("caller-owned findings status = %q, want %q", status, "?? "+callerOwnedFindings)
	}

	const unexpected = ".deepsec/data/deepsec-scan/unexpected.cache"
	writeDeepsecContractFile(t, repo, unexpected, "unclassified scanner state\n")
	if status := strings.TrimSpace(sandbox.Run(t, repo, "status", "--porcelain=v1", "--untracked-files=all", "--", unexpected)); status != "?? "+unexpected {
		t.Fatalf("unknown Deepsec state status = %q, want %q", status, "?? "+unexpected)
	}
}

func writeDeepsecContractFile(t *testing.T, root, name, contents string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create parent for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
