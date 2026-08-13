package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func specControlBasename(path string) (string, bool) {
	base := filepath.Base(path)
	switch normalizedPathIdentity(base) {
	case "spec.md":
		return "SPEC.md", base == "SPEC.md"
	case "spec.owner":
		return "SPEC.owner", base == "SPEC.owner"
	default:
		return "", false
	}
}

// specReviewOwnerPath identifies the direct protected-base owners whose future
// behavior determines whether changed SPEC contracts are reviewed at all. A
// revision cannot use this gate to approve a change to its own policy,
// workflow, implementation, or deterministic EARS parser.
func specReviewOwnerPath(path string) bool {
	return specReviewProtectedOwnerPath(path) && !isAIReviewGoTestPath(path)
}

func specReviewProtectedOwnerPath(path string) bool {
	return normalizedPathRelatedToAny(path, canonicalSpecAuthoringOwnerPaths[:]) ||
		normalizedPathRelated(path, activeHarnessRegistryPath) ||
		normalizedPathRelated(path, ".github/workflows/review.yml") ||
		normalizedPathRelated(path, ".github/rulesets/main.json") ||
		normalizedPathRelated(path, "cmd/ai-review") ||
		normalizedPathRelated(path, "internal/earslint") ||
		normalizedPathRelated(path, "internal/markdownvisible")
}

func specReviewOwnerPathAuthenticated(path string, evidence treeIdentityEvidence) bool {
	if !specReviewProtectedOwnerPath(path) {
		return false
	}
	return !evidence.safeAIReviewTestPath(path)
}

// specReviewDependencyPath identifies build inputs that can change the trusted
// reviewer without changing any SPEC contract. They remain a fail-closed
// supply-chain boundary for ordinary contributors; the workflow may separately
// recognize a narrowly authenticated Dependabot dependency-version-led update.
func specReviewDependencyPath(path string) bool {
	return normalizedPathRelated(path, "go.mod") ||
		normalizedPathRelated(path, "go.sum") ||
		normalizedPathRelated(path, "go.work") ||
		normalizedPathRelated(path, "go.work.sum") ||
		normalizedPathRelated(path, "vendor")
}

func activeHarnessMember(name string) (activeHarnessMemberEvidence, bool) {
	switch name {
	case "claude-code":
		return activeHarnessMemberEvidence{Name: name, ConfigRoot: ".claude", Aliases: []string{"claude", "claude-code"}}, true
	case "codex-cli":
		return activeHarnessMemberEvidence{Name: name, ConfigRoot: ".codex", Aliases: []string{"codex", "codex-cli"}}, true
	case "agy":
		return activeHarnessMemberEvidence{Name: name, ConfigRoot: ".agents", Aliases: []string{"agy", "agy-cli", "antigravity"}}, true
	case "opencode-cli":
		return activeHarnessMemberEvidence{Name: name, ConfigRoot: ".opencode", Aliases: []string{"opencode", "opencode-cli"}}, true
	case "pi-cli":
		return activeHarnessMemberEvidence{Name: name, ConfigRoot: ".pi", Aliases: []string{"pi", "pi-cli"}}, true
	default:
		return activeHarnessMemberEvidence{}, false
	}
}

//nolint:gocyclo // Explicit intrinsic-root handling prevents logical-seam and future-member bypasses.
func harnessLocalSpecOwner(path string, members []activeHarnessMemberEvidence) bool {
	if filepath.Base(path) != "SPEC.md" {
		return false
	}
	parts := strings.Split(normalizedPathIdentity(path), "/")
	if len(parts) < 2 {
		return false
	}
	// Dotted configuration roots, plugin roots, and explicit harness groupings
	// are registration-local by construction even beneath internal/cmd. Check
	// these intrinsic markers before allowing a logical package seam.
	for index, part := range parts[:len(parts)-1] {
		if part == "plugins" || strings.HasSuffix(part, "-plugin") {
			return true
		}
		if (part == "harness" || part == "harnesses") && index+1 < len(parts)-1 {
			return true
		}
	}
	for _, member := range members {
		aliases := make([]string, len(member.Aliases))
		for i, alias := range member.Aliases {
			aliases[i] = normalizedPathIdentity(alias)
		}
		if slices.Contains(aliases, parts[0]) {
			return true
		}
		for _, part := range parts[:len(parts)-1] {
			if part == normalizedPathIdentity(member.ConfigRoot) {
				return true
			}
			if dottedAlias, dotted := strings.CutPrefix(part, "."); dotted {
				for _, alias := range aliases {
					if dottedAlias == alias || dottedAlias == alias+"-plugin" {
						return true
					}
				}
			}
		}
	}
	return false
}

func loadHeadSpecCorpus(ctx context.Context, head string) (map[string][]byte, error) {
	tree, err := loadTreeIdentityIndex(ctx, head)
	if err != nil {
		return nil, err
	}
	return loadHeadSpecCorpusFromTree(ctx, tree)
}

func loadHeadSpecCorpusFromTree(ctx context.Context, tree treeIdentityIndex) (map[string][]byte, error) {
	requests := make([]gitBlobRequest, 0)
	paths := make([]string, 0, len(tree.byPath))
	for path := range tree.byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		entry := tree.byPath[path]
		// The authenticated tree index includes directories so checkout-identity
		// proofs can reason about file/directory conflicts. A directory whose
		// literal name is SPEC.md is not itself a normative SPEC document.
		if entry.Type == "tree" {
			continue
		}
		if filepath.Base(path) == "SPEC.md" {
			if entry.Mode != "100644" || entry.Type != "blob" {
				return nil, fmt.Errorf("HEAD SPEC is not a regular non-executable blob (%s)", path)
			}
			requests = append(requests, gitBlobRequest{Path: path, ObjectID: entry.ObjectID})
			if len(requests) > maxHeadSpecFiles {
				return nil, errors.New("HEAD contains too many SPEC files")
			}
		}
	}
	if len(requests) == 0 {
		return map[string][]byte{}, nil
	}
	return gitTextBlobsBounded(ctx, requests, maxSpecBlobBytes, maxSpecCorpusBytes)
}
