package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maxWorkflowBlobBytes     = 256 * 1024
	maxWorkflowEvidenceBytes = 4 * 1024 * 1024
	maxWorkflowEvidenceFiles = 512
)

type workflowBlobEvidence struct {
	path string
	blob []byte
}

func changedWorkflowIdentities(changedPaths []string) map[string][]string {
	identities := make(map[string][]string)
	for _, path := range changedPaths {
		path = strings.TrimSpace(path)
		identity, ok := workflowIdentity(path)
		if !ok {
			continue
		}
		identities[identity] = append(identities[identity], path)
	}
	for identity := range identities {
		sort.Strings(identities[identity])
		identities[identity] = compactStrings(identities[identity])
	}
	return identities
}

func workflowIdentity(path string) (string, bool) {
	lower := normalizedPathIdentity(path)
	const prefix = ".github/workflows/"
	if !strings.HasPrefix(lower, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(lower, prefix)
	if name == "" || strings.Contains(name, "/") ||
		(!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
		return "", false
	}
	return lower, true
}

func normalizedPathIdentity(value string) string {
	// Supported case-insensitive filesystems can also fold non-ASCII code
	// points and canonical normalization variants onto the same checkout path.
	// Normalize before and after full Unicode case folding so all such peers
	// share one authenticated Git-tree identity.
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(value)))
}

func asciiLower(value string) string {
	buf := []byte(value)
	for i, b := range buf {
		if b >= 'A' && b <= 'Z' {
			buf[i] = b + ('a' - 'A')
		}
	}
	return string(buf)
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func workflowEvidenceFailure(changed map[string][]string) string {
	paths := make([]string, 0, len(changed))
	for _, candidates := range changed {
		paths = append(paths, candidates[0])
	}
	sort.Strings(paths)
	return fmt.Sprintf("privileged workflow evidence cannot be authenticated within bounds (%s)", paths[0])
}

// loadWorkflowEvidence enumerates the complete bounded tree so case-fold
// aliases are grouped with their canonical peer even when the alias is the
// only changed path. Only identities present in the diff are materialized.
func loadWorkflowEvidence(ctx context.Context, tree treeIdentityIndex, wanted map[string][]string) (map[string][]workflowBlobEvidence, error) {
	requests, pathsByIdentity, err := workflowBlobRequests(tree, wanted)
	if err != nil {
		return nil, err
	}
	blobs, err := gitTextBlobsBounded(ctx, requests, maxWorkflowBlobBytes, maxWorkflowEvidenceBytes)
	if err != nil {
		return nil, err
	}
	return assembleWorkflowEvidence(pathsByIdentity, blobs)
}

func workflowBlobRequests(tree treeIdentityIndex, wanted map[string][]string) ([]gitBlobRequest, map[string][]string, error) {
	requests := make([]gitBlobRequest, 0)
	pathsByIdentity := make(map[string][]string)
	paths := make([]string, 0, len(tree.byPath))
	for path := range tree.byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		entry := tree.byPath[path]
		identity, isWorkflow := workflowIdentity(path)
		if !isWorkflow {
			continue
		}
		if _, needed := wanted[identity]; !needed {
			continue
		}
		if entry.Mode != "100644" || entry.Type != "blob" {
			return nil, nil, fmt.Errorf("workflow is not a regular non-executable blob (%s)", path)
		}
		requests = append(requests, gitBlobRequest{Path: path, ObjectID: entry.ObjectID})
		pathsByIdentity[identity] = append(pathsByIdentity[identity], path)
		if len(requests) > maxWorkflowEvidenceFiles {
			return nil, nil, errors.New("workflow evidence file count exceeds the review limit")
		}
	}
	return requests, pathsByIdentity, nil
}

func assembleWorkflowEvidence(pathsByIdentity map[string][]string, blobs map[string][]byte) (map[string][]workflowBlobEvidence, error) {
	evidence := make(map[string][]workflowBlobEvidence, len(pathsByIdentity))
	for identity, paths := range pathsByIdentity {
		sort.Strings(paths)
		for _, path := range paths {
			blob, ok := blobs[path]
			if !ok {
				return nil, fmt.Errorf("workflow blob is unavailable (%s)", path)
			}
			evidence[identity] = append(evidence[identity], workflowBlobEvidence{path: path, blob: blob})
		}
	}
	return evidence, nil
}
