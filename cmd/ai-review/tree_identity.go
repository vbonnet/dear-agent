package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

const (
	maxTreeIdentityEntries           = 2 * maxHeadPaths
	maxTreeIdentityPathComponents    = 256
	maxTreeIdentityPeers             = 32
	maxSpecControlIdentityComponents = 64 * 1024
)

type treeIdentityEntry struct {
	Path     string
	Mode     string
	Type     string
	ObjectID string
}

type treeIdentityIndex struct {
	byIdentity map[string][]treeIdentityEntry
	byPath     map[string]treeIdentityEntry
}

type treeIdentityEvidence struct {
	base treeIdentityIndex
	head treeIdentityIndex
}

type treeIdentityLimits struct {
	entries    int
	peers      int
	components int
}

var productionTreeIdentityLimits = treeIdentityLimits{
	entries:    maxTreeIdentityEntries,
	peers:      maxTreeIdentityPeers,
	components: maxTreeIdentityPathComponents,
}

// loadTreeIdentityEvidence authenticates the complete merge-base and head
// trees, including directory entries. Leaf-only scans cannot distinguish a
// harmless path from a file/directory or case-fold alias that a supported
// checkout would materialize at the same filesystem identity.
func loadTreeIdentityEvidence(ctx context.Context, base, head string) (treeIdentityEvidence, error) {
	baseIndex, err := loadTreeIdentityIndex(ctx, base)
	if err != nil {
		return treeIdentityEvidence{}, fmt.Errorf("load merge-base tree identities: %w", err)
	}
	headIndex, err := loadTreeIdentityIndex(ctx, head)
	if err != nil {
		return treeIdentityEvidence{}, fmt.Errorf("load HEAD tree identities: %w", err)
	}
	return treeIdentityEvidence{base: baseIndex, head: headIndex}, nil
}

func loadTreeIdentityIndex(ctx context.Context, revision string) (treeIdentityIndex, error) {
	if !validObjectID(revision) {
		return treeIdentityIndex{}, errors.New("invalid tree identity revision")
	}
	out, err := gitOutputBounded(ctx, maxGitMetadataBytes, "ls-tree", "--full-tree", "-r", "-t", "-z", revision)
	if err != nil {
		return treeIdentityIndex{}, err
	}
	return parseTreeIdentityIndex(ctx, out, productionTreeIdentityLimits)
}

func parseTreeIdentityIndex(ctx context.Context, out []byte, limits treeIdentityLimits) (treeIdentityIndex, error) {
	if limits.entries < 1 || limits.peers < 1 || limits.components < 1 {
		return treeIdentityIndex{}, errors.New("invalid tree identity limits")
	}
	if len(out) > 0 && out[len(out)-1] != 0 {
		return treeIdentityIndex{}, errors.New("tree identity inventory has unterminated NUL framing")
	}
	fields := bytesSplitNUL(out)
	if len(fields) > limits.entries {
		return treeIdentityIndex{}, errors.New("tree identity entry count exceeds the review limit")
	}
	index, err := parseTreeIdentityEntries(ctx, fields, limits)
	if err != nil {
		return treeIdentityIndex{}, err
	}
	if err := index.validateAncestry(ctx); err != nil {
		return treeIdentityIndex{}, err
	}
	sortTreeIdentityPeers(index.byIdentity)
	return index, nil
}

func parseTreeIdentityEntries(ctx context.Context, fields [][]byte, limits treeIdentityLimits) (treeIdentityIndex, error) {
	index := treeIdentityIndex{
		byIdentity: make(map[string][]treeIdentityEntry),
		byPath:     make(map[string]treeIdentityEntry),
	}
	for fieldIndex, raw := range fields {
		if fieldIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return treeIdentityIndex{}, err
			}
		}
		entry, err := parseTreeIdentityEntry(raw, limits.components)
		if err != nil {
			return treeIdentityIndex{}, err
		}
		path := entry.Path
		if _, duplicate := index.byPath[path]; duplicate {
			return treeIdentityIndex{}, errors.New("tree identity inventory contains a duplicate path")
		}
		index.byPath[path] = entry
		identity := normalizedPathIdentity(path)
		index.byIdentity[identity] = append(index.byIdentity[identity], entry)
		if len(index.byIdentity[identity]) > limits.peers {
			return treeIdentityIndex{}, errors.New("tree identity peer count exceeds the review limit")
		}
	}
	return index, nil
}

func parseTreeIdentityEntry(raw []byte, maxComponents int) (treeIdentityEntry, error) {
	metadata, rawPath, ok := bytes.Cut(raw, []byte{'\t'})
	parts := strings.Fields(string(metadata))
	path := string(rawPath)
	if !ok || len(parts) != 3 || !validObjectID(parts[2]) || !safeGitPath(path) ||
		strings.Count(path, "/")+1 > maxComponents ||
		!validTreeIdentityMode(parts[0], parts[1]) {
		return treeIdentityEntry{}, errors.New("tree identity inventory contains unauthenticated metadata")
	}
	return treeIdentityEntry{Path: path, Mode: parts[0], Type: parts[1], ObjectID: parts[2]}, nil
}

func sortTreeIdentityPeers(peers map[string][]treeIdentityEntry) {
	for identity := range peers {
		sort.Slice(peers[identity], func(i, j int) bool {
			left, right := peers[identity][i], peers[identity][j]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			if left.Type != right.Type {
				return left.Type < right.Type
			}
			return left.Mode < right.Mode
		})
	}
}

func validTreeIdentityMode(mode, objectType string) bool {
	switch mode {
	case "040000":
		return objectType == "tree"
	case "100644", "100755", "120000":
		return objectType == "blob"
	case gitlinkMode:
		return objectType == "commit"
	default:
		return false
	}
}

func (index treeIdentityIndex) validateAncestry(ctx context.Context) error {
	pathIndex := 0
	for path := range index.byPath {
		if pathIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		pathIndex++
		for _, ancestor := range pathAncestors(path) {
			entry, ok := index.byPath[ancestor]
			if !ok || entry.Mode != "040000" || entry.Type != "tree" {
				return errors.New("tree identity inventory has unauthenticated directory ancestry")
			}
		}
	}
	return nil
}

func pathAncestors(path string) []string {
	ancestors := make([]string, 0, strings.Count(path, "/"))
	for offset := strings.IndexByte(path, '/'); offset >= 0; {
		ancestors = append(ancestors, path[:offset])
		next := strings.IndexByte(path[offset+1:], '/')
		if next < 0 {
			break
		}
		offset += next + 1
	}
	return ancestors
}

func pathAndAncestors(path string) []string {
	return append(pathAncestors(path), path)
}

// normalizedPathRelated reports a slash-boundary ancestor, descendant, or
// exact relationship after the checkout identity normalization. It is
// symmetric so a protected file catches an added child and a protected
// directory catches a changed slashless node.
func normalizedPathRelated(left, right string) bool {
	left = normalizedPathIdentity(strings.TrimSuffix(left, "/"))
	right = normalizedPathIdentity(strings.TrimSuffix(right, "/"))
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func normalizedPathRelatedToAny(path string, protected []string) bool {
	return slices.ContainsFunc(protected, func(candidate string) bool {
		return normalizedPathRelated(path, candidate)
	})
}

// stableRegularPath proves that a changed regular file and every directory in
// its ancestry have one compatible checkout identity across both revisions.
// Additions and deletions may be absent on one side; any peer, spelling drift,
// file/directory transition, symlink, gitlink, or executable mode fails closed.
func (evidence treeIdentityEvidence) stableRegularPath(path string) (bool, string) {
	components := pathAndAncestors(path)
	for index, component := range components {
		leaf := index == len(components)-1
		basePeers := evidence.base.byIdentity[normalizedPathIdentity(component)]
		headPeers := evidence.head.byIdentity[normalizedPathIdentity(component)]
		reason := stableTreeIdentityComponent(component, leaf, basePeers, headPeers)
		if reason != "" {
			return false, reason
		}
	}
	return true, ""
}

func stableTreeIdentityComponent(component string, leaf bool, basePeers, headPeers []treeIdentityEntry) string {
	identity := normalizedPathIdentity(component)
	if len(basePeers) == 0 && len(headPeers) == 0 {
		return "changed path identity is absent from both authenticated revisions (" + identity + ")"
	}
	if !leaf {
		return stableTreeAncestorIdentity(identity, basePeers, headPeers)
	}
	return stableTreeLeafIdentity(identity, basePeers, headPeers)
}

func stableTreeAncestorIdentity(identity string, basePeers, headPeers []treeIdentityEntry) string {
	for _, peers := range [][]treeIdentityEntry{basePeers, headPeers} {
		for _, entry := range peers {
			if entry.Mode != "040000" || entry.Type != "tree" {
				return "changed path ancestry has a file/directory type conflict (" + identity + ")"
			}
		}
	}
	// Multiple normalized directory spellings merge on supported
	// case-insensitive checkouts. They are compatible when every peer is a
	// tree; leaf peers below them remain adjudicated independently.
	return ""
}

func stableTreeLeafIdentity(identity string, basePeers, headPeers []treeIdentityEntry) string {
	if len(basePeers) > 1 || len(headPeers) > 1 {
		return "checkout file identity has multiple authenticated peers (" + identity + ")"
	}
	for _, peers := range [][]treeIdentityEntry{basePeers, headPeers} {
		if len(peers) == 0 {
			continue
		}
		entry := peers[0]
		if entry.Mode != "100644" || entry.Type != "blob" {
			return "changed file identity is not a regular non-executable blob (" + identity + ")"
		}
	}
	if len(basePeers) == 1 && len(headPeers) == 1 {
		baseEntry, headEntry := basePeers[0], headPeers[0]
		if baseEntry.Path != headEntry.Path {
			return "checkout identity spelling changed across revisions (" + identity + ")"
		}
		if baseEntry.Mode != headEntry.Mode || baseEntry.Type != headEntry.Type {
			return "checkout identity type changed across revisions (" + identity + ")"
		}
	}
	return ""
}
