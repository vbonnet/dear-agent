package main

import (
	"context"
	"errors"
	"sort"
	"strings"
)

func (evidence treeIdentityEvidence) safeAIReviewTestPath(path string) bool {
	safe, _ := evidence.aiReviewTestPathSafety(path)
	return safe
}

func (evidence treeIdentityEvidence) aiReviewTestPathSafety(path string) (bool, string) {
	if !isAIReviewGoTestPath(path) || !normalizedPathRelated(path, "cmd/ai-review") {
		return false, "path is not an exact lowercase-ASCII Go test under cmd/ai-review"
	}
	components := pathAndAncestors(path)
	for index, component := range components {
		identity := normalizedPathIdentity(component)
		basePeers := evidence.base.byIdentity[identity]
		headPeers := evidence.head.byIdentity[identity]
		if len(basePeers) == 0 && len(headPeers) == 0 {
			return false, "changed test identity is absent from both authenticated revisions (" + identity + ")"
		}
		if index < len(components)-1 {
			if reason := stableTreeAncestorIdentity(identity, basePeers, headPeers); reason != "" {
				return false, reason
			}
			continue
		}
		if reason := aiReviewTestLeafIdentityRisk(identity, basePeers, headPeers); reason != "" {
			return false, reason
		}
	}
	return true, ""
}

func aiReviewTestLeafIdentityRisk(identity string, basePeers, headPeers []treeIdentityEntry) string {
	if len(basePeers) > 1 || len(headPeers) > 1 {
		return "review test identity has multiple authenticated peers (" + identity + ")"
	}
	for _, peers := range [][]treeIdentityEntry{basePeers, headPeers} {
		if len(peers) == 0 {
			continue
		}
		entry := peers[0]
		if entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") || !isAIReviewGoTestPath(entry.Path) {
			return "review test identity includes a production or non-file peer (" + identity + ")"
		}
	}
	return ""
}

// checkoutIdentityTriggers applies the filesystem-identity invariant to every
// changed path, not just today's enumerated trust roots. Tree-only aliases can
// merge safely, but a tree/non-tree conflict or colliding non-tree peer can
// shadow arbitrary RBAC, hook, schema, migration, or deployment owners.
func checkoutIdentityTriggers(ctx context.Context, evidence treeIdentityEvidence, changedPaths []string) ([]string, error) {
	reasons := make(map[string]bool)
	for pathIndex, path := range changedPaths {
		if pathIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for _, component := range pathAndAncestors(path) {
			identity := normalizedPathIdentity(component)
			if reason := checkoutIdentityComponentRisk(identity, evidence.base.byIdentity[identity], evidence.head.byIdentity[identity]); reason != "" {
				reasons[reason] = true
				if len(reasons) > maxDeterministicEscalationTriggers {
					return nil, errors.New("checkout identity trigger count exceeds the review limit")
				}
				break
			}
		}
	}
	ordered := make([]string, 0, len(reasons))
	for reason := range reasons {
		ordered = append(ordered, "changed path has unsafe checkout identity ("+reason+")")
	}
	sort.Strings(ordered)
	return ordered, nil
}

func checkoutIdentityComponentRisk(identity string, basePeers, headPeers []treeIdentityEntry) string {
	baseTrees, baseFiles := partitionTreeIdentityPeers(basePeers)
	headTrees, headFiles := partitionTreeIdentityPeers(headPeers)
	if (len(baseTrees) > 0 && len(baseFiles) > 0) || (len(headTrees) > 0 && len(headFiles) > 0) {
		return "tree and non-tree peers share identity " + identity
	}
	if len(baseFiles) > 1 || len(headFiles) > 1 {
		return "multiple non-tree peers share identity " + identity
	}
	if (len(baseTrees) > 0 && len(headFiles) > 0) || (len(baseFiles) > 0 && len(headTrees) > 0) {
		return "tree/non-tree identity changed across revisions " + identity
	}
	return ""
}

func partitionTreeIdentityPeers(peers []treeIdentityEntry) (trees, files []treeIdentityEntry) {
	for _, peer := range peers {
		if peer.Type == "tree" {
			trees = append(trees, peer)
		} else {
			files = append(files, peer)
		}
	}
	return trees, files
}

func unsafeAIReviewTestIdentityTriggers(evidence treeIdentityEvidence, changedPaths []string) []string {
	reasons := make(map[string]bool)
	for _, path := range changedPaths {
		if !isAIReviewGoTestPath(path) {
			continue
		}
		if safe, reason := evidence.aiReviewTestPathSafety(path); !safe {
			reasons[reason] = true
		}
	}
	triggers := make([]string, 0, len(reasons))
	for reason := range reasons {
		triggers = append(triggers, "review test-only path has unsafe checkout identity ("+reason+")")
	}
	sort.Strings(triggers)
	return triggers
}

func (evidence treeIdentityEvidence) hookOwnerAutomatedPathSafety(path string) (bool, string) {
	if !isHookOwnerAutomatedPath(path) {
		return false, "path is not an exact hook-package test or canonical hook-owner SPEC"
	}
	components := pathAndAncestors(path)
	for index, component := range components {
		identity := normalizedPathIdentity(component)
		basePeers := evidence.base.byIdentity[identity]
		headPeers := evidence.head.byIdentity[identity]
		if len(basePeers) == 0 && len(headPeers) == 0 {
			return false, "changed hook path identity is absent from both authenticated revisions (" + identity + ")"
		}
		if index < len(components)-1 {
			if reason := exactHookTreeAncestorRisk(component, basePeers, headPeers); reason != "" {
				return false, reason
			}
			continue
		}
		if reason := exactHookAutomatedLeafRisk(path, basePeers, headPeers); reason != "" {
			return false, reason
		}
	}
	return true, ""
}

func exactHookTreeAncestorRisk(component string, basePeers, headPeers []treeIdentityEntry) string {
	identity := normalizedPathIdentity(component)
	for _, peers := range [][]treeIdentityEntry{basePeers, headPeers} {
		if len(peers) > 1 {
			return "hook carveout ancestry has normalized alias peers (" + identity + ")"
		}
		for _, entry := range peers {
			if entry.Path != component || entry.Mode != "040000" || entry.Type != "tree" {
				return "hook carveout ancestry is noncanonical or non-directory (" + identity + ")"
			}
		}
	}
	return ""
}

func exactHookAutomatedLeafRisk(path string, basePeers, headPeers []treeIdentityEntry) string {
	identity := normalizedPathIdentity(path)
	if len(basePeers) > 1 || len(headPeers) > 1 {
		return "hook carveout leaf has normalized alias peers (" + identity + ")"
	}
	if isHookGoTestPath(path) {
		return exactHookTestLeafRisk(path, basePeers, headPeers)
	}
	return exactHookSpecLeafRisk(path, basePeers, headPeers)
}

func exactHookTestLeafRisk(path string, basePeers, headPeers []treeIdentityEntry) string {
	identity := normalizedPathIdentity(path)
	owner, _ := hookGoPackageOwner(path)
	for _, peers := range [][]treeIdentityEntry{basePeers, headPeers} {
		if len(peers) == 0 {
			continue
		}
		entry := peers[0]
		peerOwner, peerIsTest := hookGoPackageOwner(entry.Path)
		if !peerIsTest || peerOwner != owner || !strings.HasSuffix(entry.Path, "_test.go") || entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
			return "hook test carveout leaf is not a regular exact-suffix test in the same canonical Go package owner (" + identity + ")"
		}
	}
	return ""
}

func exactHookSpecLeafRisk(path string, basePeers, headPeers []treeIdentityEntry) string {
	identity := normalizedPathIdentity(path)
	for _, peers := range [][]treeIdentityEntry{basePeers, headPeers} {
		if len(peers) == 0 {
			continue
		}
		entry := peers[0]
		if entry.Path != path || entry.Type != "blob" {
			return "hook carveout leaf is noncanonical or non-file (" + identity + ")"
		}
		if entry.Mode != "100644" {
			return "hook SPEC carveout leaf is not a regular non-executable blob (" + identity + ")"
		}
	}
	return ""
}

func unsafeHookOwnerAutomatedPathTriggers(evidence treeIdentityEvidence, changedPaths []string) []string {
	reasons := make(map[string]bool)
	for _, path := range changedPaths {
		if !isHookOwnerAutomatedPath(path) {
			continue
		}
		if safe, reason := evidence.hookOwnerAutomatedPathSafety(path); !safe {
			reasons[reason] = true
		}
	}
	triggers := make([]string, 0, len(reasons))
	for reason := range reasons {
		triggers = append(triggers, "hook test or SPEC path has unsafe checkout identity ("+reason+")")
	}
	sort.Strings(triggers)
	return triggers
}

// changedSpecControlIdentityRisks uses the same authenticated tree identities
// for SPEC controls. A stable direct control-file change remains reviewable;
// an alias peer, type transition, or changed ancestor/descendant of an actual
// control blob fails closed.
func changedSpecControlIdentityRisks(ctx context.Context, evidence treeIdentityEvidence, changedPaths []string) (map[string]string, error) {
	controlIdentities, controlAncestors, err := evidence.specControlIdentityIndex(ctx)
	if err != nil {
		return nil, err
	}
	risks := make(map[string]string)
	for index, path := range changedPaths {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		controlName, _ := specControlBasename(path)
		if controlName != "" {
			if safe, reason := evidence.stableRegularPath(path); !safe {
				risks[path] = "SPEC control path type evidence or checkout identity requires maintainer review (" + reason + ")"
				continue
			}
		}
		if identity, related := relatedSpecControlIdentity(path, controlIdentities, controlAncestors); related {
			risks[path] = "SPEC control ancestor or descendant identity requires maintainer review (" + path + " aliases " + identity + ")"
		}
	}
	return risks, nil
}

func (evidence treeIdentityEvidence) specControlIdentityIndex(ctx context.Context) (map[string]bool, map[string]string, error) {
	identities := make(map[string]bool)
	ancestors := make(map[string]string)
	components := 0
	for _, tree := range []treeIdentityIndex{evidence.base, evidence.head} {
		paths := make([]string, 0, len(tree.byPath))
		for path := range tree.byPath {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for index, path := range paths {
			if index%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, nil, err
				}
			}
			entry := tree.byPath[path]
			name, _ := specControlBasename(entry.Path)
			if name != "" && entry.Type != "tree" {
				identity := normalizedPathIdentity(entry.Path)
				if !identities[identity] {
					identities[identity] = true
					if len(identities) > maxHeadSpecFiles {
						return nil, nil, errors.New("SPEC control identity count exceeds the review limit")
					}
					for _, ancestor := range pathAncestors(identity) {
						components++
						if components > maxSpecControlIdentityComponents {
							return nil, nil, errors.New("SPEC control ancestor count exceeds the review limit")
						}
						if _, exists := ancestors[ancestor]; !exists {
							ancestors[ancestor] = identity
						}
					}
				}
			}
		}
	}
	return identities, ancestors, nil
}

func relatedSpecControlIdentity(path string, identities map[string]bool, ancestors map[string]string) (string, bool) {
	identity := normalizedPathIdentity(path)
	if control, ok := ancestors[identity]; ok {
		return control, true
	}
	for _, prefix := range pathAncestors(identity) {
		if identities[prefix] {
			return prefix, true
		}
	}
	return "", false
}
