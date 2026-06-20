package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
)

var (
	verifyDeploymentStrict bool
	verifyDeploymentJSON   bool
	verifyDeploymentRepo   string
	verifyDeploymentRef    string
)

var verifyDeploymentCmd = &cobra.Command{
	Use:   "verify-deployment",
	Short: "Verify the running binary was built from origin/main (deployment gate)",
	Long: `Assert that the running agm binary's embedded vcs.revision is an ancestor
of origin/main. This is the merge->binary deployment-verification gate: it
catches a binary built from a rolled-back or divergent checkout (a commit that
is not on trunk), which a plain "is the binary up to date with HEAD" check
cannot detect.

It is safe to call from a post-merge hook or CI: no network fetch is performed
(it verifies against the local origin/main tracking ref).

Exit codes:
  0 - Verified (binary commit is an ancestor of origin/main), OR
      indeterminate without --strict (no source repo / no trunk ref / git error)
  1 - FAILED: binary is NOT on origin/main, its commit is missing from the repo,
      or it has no embedded vcs.revision
  2 - Indeterminate AND --strict was given (could not verify)

Examples:
  agm admin verify-deployment
  agm admin verify-deployment --json
  agm admin verify-deployment --strict          # CI: fail on indeterminate too
  agm admin verify-deployment --repo /path/to/agm --ref origin/main`,
	RunE: runVerifyDeployment,
}

func init() {
	adminCmd.AddCommand(verifyDeploymentCmd)
	verifyDeploymentCmd.Flags().BoolVar(&verifyDeploymentStrict, "strict", false,
		"Treat indeterminate results (no repo / no trunk ref) as failure (exit 2)")
	verifyDeploymentCmd.Flags().BoolVar(&verifyDeploymentJSON, "json", false,
		"Output the result as JSON")
	verifyDeploymentCmd.Flags().StringVar(&verifyDeploymentRepo, "repo", "",
		"Source repository to verify against (default: auto-detected, AGM_SOURCE_DIR)")
	verifyDeploymentCmd.Flags().StringVar(&verifyDeploymentRef, "ref", "",
		"Trunk ref to verify against (default: origin/HEAD, then origin/main)")
}

func runVerifyDeployment(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	repoPath := verifyDeploymentRepo
	if repoPath == "" {
		p, err := freshness.FindRepoPath()
		if err != nil {
			// No source repo to verify against — indeterminate.
			return reportVerifyDeployment(freshness.AncestryResult{
				Status:       freshness.AncestryIndeterminate,
				BinaryCommit: GitCommit,
				Err:          err,
			})
		}
		repoPath = p
	}

	result := freshness.VerifyAncestry(repoPath, GitCommit, verifyDeploymentRef)
	return reportVerifyDeployment(result)
}

func reportVerifyDeployment(r freshness.AncestryResult) error {
	if verifyDeploymentJSON {
		payload := map[string]any{
			"status":        ancestryStatusString(r.Status),
			"ok":            r.OK(),
			"fail_loud":     r.FailLoud(),
			"binary_commit": r.BinaryCommit,
			"trunk_ref":     r.TrunkRef,
			"trunk_commit":  r.TrunkCommit,
			"repo_path":     r.RepoPath,
			"reason":        r.Reason(),
		}
		if r.Err != nil {
			payload["error"] = r.Err.Error()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
	}

	switch {
	case r.OK():
		if !verifyDeploymentJSON {
			fmt.Printf("✓ deployment verified: %s\n", r.Reason())
		}
		return nil
	case r.FailLoud():
		if !verifyDeploymentJSON {
			fmt.Fprintf(os.Stderr, "✗ deployment verification FAILED: %s\n", r.Reason())
		}
		return fmt.Errorf("deployment verification failed: %s", r.Reason())
	default: // indeterminate
		if !verifyDeploymentJSON {
			fmt.Fprintf(os.Stderr, "⚠ deployment verification indeterminate: %s\n", r.Reason())
		}
		if verifyDeploymentStrict {
			os.Exit(2)
		}
		return nil
	}
}

func ancestryStatusString(s freshness.AncestryStatus) string {
	switch s {
	case freshness.AncestryVerified:
		return "verified"
	case freshness.AncestryNotAncestor:
		return "not_ancestor"
	case freshness.AncestryCommitMissing:
		return "commit_missing"
	case freshness.AncestryUnknownCommit:
		return "unknown_commit"
	case freshness.AncestryIndeterminate:
		return "indeterminate"
	default:
		return "unknown"
	}
}
