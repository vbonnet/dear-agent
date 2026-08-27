// Package specpackage stages and validates the fixed, portable SPEC governance
// distribution. Validation proves structural closure and content identity; it
// does not authenticate installation ancestry or a running process image.
package specpackage

import (
	"context"
	"fmt"
)

// SchemaVersion identifies the canonical staged-package receipt format.
const SchemaVersion = "spec-governance-package/v1"

// FileReceipt identifies one payload file independently of its host path.
type FileReceipt struct {
	Path        string `json:"path"`
	Role        string `json:"role"`
	LogicalMode string `json:"logical_mode"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// Receipt is the recomputed identity of a structurally valid package.
type Receipt struct {
	SchemaVersion  string        `json:"schema_version"`
	ManifestSHA256 string        `json:"manifest_sha256"`
	Files          []FileReceipt `json:"files"`
}

// StagedPackage is a unique private staging root and its validated receipt.
type StagedPackage struct {
	Root    string  `json:"root"`
	Receipt Receipt `json:"receipt"`
}

// RetainedStagingRootError reports the originally allocated path left untouched
// after a failed Stage call. Root is diagnostic, not authenticated. Only an
// IdentityVerified value of true proves that the original root was still
// visible at that path when Stage returned. Cleanup remains a separately
// authorized, liveness-aware lifecycle operation in either case.
type RetainedStagingRootError struct {
	Root             string
	IdentityVerified bool
}

func (failure *RetainedStagingRootError) Error() string {
	if failure.IdentityVerified {
		return fmt.Sprintf("failed private staging root retained without cleanup with verified identity: %s", failure.Root)
	}
	return fmt.Sprintf("allocated staging path retained without cleanup with unverified identity: %s", failure.Root)
}

// Stage creates and validates one unique private distribution beneath
// stagingParent. The caller supplies an already-built specaudit executable.
// After an allocated attempt fails, Stage leaves the private tree untouched
// and joins a RetainedStagingRootError whose IdentityVerified field reports
// whether the original root remained visible; lifecycle cleanup is deliberately
// outside this operation.
func Stage(ctx context.Context, sourceRoot, artifactPath, stagingParent string) (StagedPackage, error) {
	return stage(ctx, sourceRoot, artifactPath, stagingParent)
}

// Validate recomputes the structural closure and receipt of distributionRoot.
// The returned digest is evidence for later trusted installation; this call
// does not itself authenticate the root or bind a running image.
func Validate(ctx context.Context, distributionRoot string) (Receipt, error) {
	return validate(ctx, distributionRoot)
}
