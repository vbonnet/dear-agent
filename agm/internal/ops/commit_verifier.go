package ops

import (
	"fmt"
	"os/exec"
	"strings"
)

// VerifyOnMain checks whether the given commit SHA is an ancestor of the main
// branch in the repository at repoPath. This is used to confirm that a worker's
// commit has been merged to main before accepting completion.
func VerifyOnMain(repoPath string, commitSHA string) (bool, error) {
	if repoPath == "" {
		return false, fmt.Errorf("repoPath must not be empty")
	}
	if commitSHA == "" {
		return false, fmt.Errorf("commitSHA must not be empty")
	}

	cmd := exec.Command("git", "-C", repoPath, "merge-base", "--is-ancestor", commitSHA, "main")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means the commit is NOT an ancestor of main.
			if exitErr.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, fmt.Errorf("git merge-base failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return true, nil
}
