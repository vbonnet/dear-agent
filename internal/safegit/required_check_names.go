package safegit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// RequiredCheckNamesForBranch returns the complete effective set of unscoped
// required status-check context names for branch. A non-nil empty set means
// both policy sources were read successfully and require no status checks.
// Discovery errors, required-workflow rules, and integration-scoped identities
// fail closed because a name-only consumer cannot prove those requirements.
func RequiredCheckNamesForBranch(ctx context.Context, repo, branch string) (map[string]bool, error) {
	policy, err := discoverRequiredChecksContext(ctx, repo, branch)
	if err != nil {
		return nil, err
	}
	if policy.HasRequiredWorkflows {
		return nil, fmt.Errorf(
			"required workflow rules apply to %s; name-only required-check projection cannot prove whether CI fired",
			branch,
		)
	}
	scoped := make(map[string]bool)
	for identity := range policy.Identities {
		if identity.Scoped {
			scoped[identity.Context] = true
		}
	}
	if len(scoped) > 0 {
		contexts := make([]string, 0, len(scoped))
		for context := range scoped {
			contexts = append(contexts, context)
		}
		sort.Strings(contexts)
		return nil, fmt.Errorf(
			"integration-scoped required checks apply to %s: %s; name-only required-check projection cannot prove provider identity",
			branch,
			strings.Join(contexts, ", "),
		)
	}
	return policy.contexts(), nil
}

func promptDisabledGHCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)
	return cmd
}
