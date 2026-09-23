package safegit

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// checkAllCI verifies that every effective required CI check has passed.
func checkAllCI(prNum int, repo string) error {
	return checkAllCIContext(context.Background(), prNum, repo)
}

func checkAllCIContext(ctx context.Context, prNum int, repo string) error {
	return checkAllCIContextForBase(ctx, prNum, repo, "")
}

func checkAllCIContextForBase(ctx context.Context, prNum int, repo, expectedBase string) error {
	projection, err := projectRequiredCheckRuns(ctx, prNum, repo)
	if err != nil {
		return err
	}
	if expectedBase != "" && projection.BaseBranch != expectedBase {
		return fmt.Errorf("PR #%d base changed before CI policy evaluation (expected %s, got %s); retry",
			prNum, expectedBase, projection.BaseBranch)
	}
	allChecks, err := fetchAllCheckRuns(ctx, prNum, repo, projection.AuthoritativeEmpty)
	if err != nil {
		return err
	}

	required := projection.Runs
	if projection.AuthoritativeEmpty {
		required = allChecks
		fmt.Fprintln(os.Stderr, "safe-merge: base branch has no required CI contexts; validating all reported checks (conservative policy)")
	} else {
		requiredNames := make([]string, 0, len(required))
		for _, check := range required {
			requiredNames = append(requiredNames, check.Name)
		}
		informationalRuns := informationalCheckRuns(allChecks, projection.ProviderRuns)
		informational := make([]string, 0, len(informationalRuns))
		for _, check := range informationalRuns {
			informational = append(informational, fmt.Sprintf("%s (%s)", check.Name, check.State))
		}
		sort.Strings(requiredNames)
		sort.Strings(informational)
		fmt.Fprintf(os.Stderr, "safe-merge: provider-effective required checks: %s\n", strings.Join(requiredNames, ", "))
		if len(informational) > 0 {
			fmt.Fprintf(os.Stderr, "safe-merge: informational checks (not merge-gating): %s\n", strings.Join(informational, ", "))
		}
	}
	return validateCheckRuns(required)
}
