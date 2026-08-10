package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

const maxDependabotModuleBlobBytes = 2 * 1024 * 1024

// dependabotModuleOnlyCandidate proves that an authenticated Git delta contains
// only an ordinary Go dependency version update. Authentication of the actor
// and source repository deliberately remains a caller responsibility; this
// helper examines committed Git objects only.
//
// The accepted path/status sets are exactly:
//
//	M go.mod
//
// or:
//
//	M go.mod
//	M go.sum
//
// Both go.mod objects must be regular, non-executable text blobs. Their module,
// go, toolchain, exclude, retract, and every other non-require token must be
// identical. Requirement membership and indirect markers may change, but at
// least one requirement present on both sides must change version. A replace
// directive on either side is never eligible, even when it is unchanged.
func dependabotModuleOnlyCandidate(ctx context.Context, mergeBase, head string) (bool, error) {
	actualMergeBase, err := resolveMergeBase(ctx, mergeBase, head)
	if err != nil {
		return false, fmt.Errorf("authenticate dependency delta revisions: %w", err)
	}
	if actualMergeBase != mergeBase {
		return false, errors.New("dependency delta base is not the authenticated merge-base")
	}

	paths, candidate, err := dependabotModuleCandidatePaths(ctx, mergeBase, head)
	if err != nil || !candidate {
		return false, err
	}
	baseBlobs, headBlobs, candidate, err := dependabotModuleCandidateBlobs(ctx, mergeBase, head, paths)
	if err != nil || !candidate {
		return false, err
	}
	candidate, err = moduleDependencyVersionLedChange(baseBlobs["go.mod"], headBlobs["go.mod"])
	if err != nil || !candidate || len(paths) == 1 {
		return candidate, err
	}
	return goSumChangeSafe(baseBlobs["go.sum"], headBlobs["go.sum"])
}

func dependabotModuleCandidatePaths(ctx context.Context, mergeBase, head string) ([]string, bool, error) {
	out, err := gitOutputBounded(ctx, maxGitMetadataBytes, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-status", "-z", mergeBase, head)
	if err != nil {
		return nil, false, fmt.Errorf("read dependency delta paths: %w", err)
	}
	fields := bytesSplitNUL(out)
	if len(fields)%2 != 0 {
		return nil, false, errors.New("dependency delta has malformed Git name-status evidence")
	}
	if len(fields)/2 > maxChangedPaths {
		return nil, false, errors.New("dependency delta path count exceeds the review limit")
	}

	statuses := make(map[string]string, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		status, path := string(fields[i]), string(fields[i+1])
		if len(status) != 1 || !safeGitPath(path) {
			return nil, false, errors.New("dependency delta has unsafe Git name-status evidence")
		}
		if _, duplicate := statuses[path]; duplicate {
			return nil, false, errors.New("dependency delta repeats a changed path")
		}
		statuses[path] = status
	}

	if statuses["go.mod"] != "M" || (len(statuses) != 1 && len(statuses) != 2) {
		return nil, false, nil
	}
	paths := []string{"go.mod"}
	if len(statuses) == 2 {
		if statuses["go.sum"] != "M" {
			return nil, false, nil
		}
		paths = append(paths, "go.sum")
	}
	return paths, true, nil
}

func dependabotModuleCandidateBlobs(ctx context.Context, mergeBase, head string, paths []string) (map[string][]byte, map[string][]byte, bool, error) {
	baseBlobs, err := gitRegularTextBlobsBounded(ctx, mergeBase, paths, maxDependabotModuleBlobBytes, len(paths)*maxDependabotModuleBlobBytes)
	if err != nil {
		return nil, nil, false, fmt.Errorf("read base dependency manifests: %w", err)
	}
	headBlobs, err := gitRegularTextBlobsBounded(ctx, head, paths, maxDependabotModuleBlobBytes, len(paths)*maxDependabotModuleBlobBytes)
	if err != nil {
		return nil, nil, false, fmt.Errorf("read head dependency manifests: %w", err)
	}
	if len(baseBlobs) != len(paths) || len(headBlobs) != len(paths) {
		return nil, nil, false, nil
	}
	return baseBlobs, headBlobs, true, nil
}

func moduleDependencyVersionLedChange(base, head []byte) (bool, error) {
	baseModule, err := parseModuleVersionState(base)
	if err != nil {
		return false, fmt.Errorf("parse base go.mod: %w", err)
	}
	headModule, err := parseModuleVersionState(head)
	if err != nil {
		return false, fmt.Errorf("parse head go.mod: %w", err)
	}
	if baseModule.hasReplace || headModule.hasReplace {
		return false, nil
	}
	if !bytes.Equal(baseModule.withoutRequireDirectives, headModule.withoutRequireDirectives) {
		return false, nil
	}

	versionChanged := false
	for path, baseRequirement := range baseModule.requirements {
		headRequirement, ok := headModule.requirements[path]
		if !ok {
			if len(baseRequirement.annotations) != 0 {
				return false, nil
			}
			continue
		}
		if !slices.Equal(baseRequirement.annotations, headRequirement.annotations) {
			return false, nil
		}
		if baseRequirement.version != headRequirement.version {
			versionChanged = true
		}
	}
	for path, headRequirement := range headModule.requirements {
		if _, existed := baseModule.requirements[path]; !existed && len(headRequirement.annotations) != 0 {
			return false, nil
		}
	}
	return versionChanged, nil
}

type moduleVersionState struct {
	requirements             map[string]moduleRequirement
	withoutRequireDirectives []byte
	hasReplace               bool
}

type moduleRequirement struct {
	version     string
	annotations []string
}

// parseModuleVersionState removes parsed require directives before formatting.
// Equality of the resulting bytes is therefore a strict proof that all
// non-require syntax and comments stayed fixed. Requirement annotations are
// authenticated separately, with the tool-managed indirect marker normalized
// away so directness may change without admitting arbitrary comment changes.
func parseModuleVersionState(raw []byte) (moduleVersionState, error) {
	file, err := modfile.Parse("go.mod", raw, nil)
	if err != nil {
		return moduleVersionState{}, err
	}
	if file.Module == nil || file.Module.Mod.Path == "" {
		return moduleVersionState{}, errors.New("go.mod has no module directive")
	}

	state := moduleVersionState{
		requirements: make(map[string]moduleRequirement, len(file.Require)),
		hasReplace:   len(file.Replace) != 0,
	}
	for _, requirement := range file.Require {
		path, parsed, err := parseModuleRequirement(requirement)
		if err != nil {
			return moduleVersionState{}, err
		}
		if _, duplicate := state.requirements[path]; duplicate {
			return moduleVersionState{}, fmt.Errorf("go.mod repeats requirement path %q", path)
		}
		state.requirements[path] = parsed
	}
	for path := range state.requirements {
		if err := file.DropRequire(path); err != nil {
			return moduleVersionState{}, fmt.Errorf("remove go.mod requirement %q: %w", path, err)
		}
	}
	file.Cleanup()
	state.withoutRequireDirectives, err = file.Format()
	if err != nil {
		return moduleVersionState{}, fmt.Errorf("normalize go.mod: %w", err)
	}
	return state, nil
}

func parseModuleRequirement(requirement *modfile.Require) (string, moduleRequirement, error) {
	if requirement == nil || requirement.Mod.Path == "" || requirement.Mod.Version == "" || requirement.Syntax == nil {
		return "", moduleRequirement{}, errors.New("go.mod has malformed requirement evidence")
	}
	tokens := requirement.Syntax.Token
	if (len(tokens) != 2 && len(tokens) != 3) || tokens[len(tokens)-2] != requirement.Mod.Path || tokens[len(tokens)-1] != requirement.Mod.Version {
		return "", moduleRequirement{}, fmt.Errorf("go.mod requirement %q has ambiguous syntax", requirement.Mod.Path)
	}
	if len(tokens) == 3 && tokens[0] != "require" {
		return "", moduleRequirement{}, fmt.Errorf("go.mod requirement %q has ambiguous directive syntax", requirement.Mod.Path)
	}
	annotations, err := moduleRequirementAnnotations(requirement)
	if err != nil {
		return "", moduleRequirement{}, err
	}
	return requirement.Mod.Path, moduleRequirement{
		version:     requirement.Mod.Version,
		annotations: annotations,
	}, nil
}

func moduleRequirementAnnotations(requirement *modfile.Require) ([]string, error) {
	annotations := make([]string, 0, len(requirement.Syntax.Before)+len(requirement.Syntax.Suffix)+len(requirement.Syntax.After))
	for _, comment := range requirement.Syntax.Before {
		annotations = append(annotations, "before:"+comment.Token)
	}
	for index, comment := range requirement.Syntax.Suffix {
		token := comment.Token
		if index == 0 && requirement.Indirect {
			var err error
			token, err = commentWithoutIndirectMarker(token)
			if err != nil {
				return nil, fmt.Errorf("go.mod requirement %q has ambiguous indirect annotation: %w", requirement.Mod.Path, err)
			}
		}
		if token != "" {
			annotations = append(annotations, "suffix:"+token)
		}
	}
	for _, comment := range requirement.Syntax.After {
		annotations = append(annotations, "after:"+comment.Token)
	}
	return annotations, nil
}

func commentWithoutIndirectMarker(token string) (string, error) {
	text := strings.TrimSpace(strings.TrimPrefix(token, "//"))
	switch {
	case text == "indirect":
		return "", nil
	case strings.HasPrefix(text, "indirect;"):
		remainder := strings.TrimSpace(strings.TrimPrefix(text, "indirect;"))
		if remainder == "" {
			return "", nil
		}
		return "// " + remainder, nil
	default:
		return "", errors.New("missing canonical indirect marker")
	}
}

func goSumChangeSafe(base, head []byte) (bool, error) {
	baseSums, err := parseGoSum(base)
	if err != nil {
		return false, fmt.Errorf("parse base go.sum: %w", err)
	}
	headSums, err := parseGoSum(head)
	if err != nil {
		return false, fmt.Errorf("parse head go.sum: %w", err)
	}
	for key, baseHash := range baseSums {
		if headHash, retained := headSums[key]; retained && headHash != baseHash {
			return false, nil
		}
	}
	return true, nil
}

func parseGoSum(raw []byte) (map[string]string, error) {
	sums := make(map[string]string)
	for number, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, " ")
		if len(fields) != 3 || fields[0] == "" || fields[1] == "" || fields[2] == "" {
			return nil, fmt.Errorf("line %d does not have the exact module version hash shape", number+1)
		}
		modulePath, versionToken, hash := fields[0], fields[1], fields[2]
		version := strings.TrimSuffix(versionToken, "/go.mod")
		if version == "" || module.CanonicalVersion(version) != version {
			return nil, fmt.Errorf("line %d has a non-canonical module version", number+1)
		}
		if err := module.Check(modulePath, version); err != nil {
			return nil, fmt.Errorf("line %d has an invalid module/version pair: %w", number+1, err)
		}
		if !canonicalGoSumHash(hash) {
			return nil, fmt.Errorf("line %d has a non-canonical h1 SHA-256", number+1)
		}
		key := modulePath + " " + versionToken
		if _, duplicate := sums[key]; duplicate {
			return nil, fmt.Errorf("line %d repeats checksum key %q", number+1, key)
		}
		sums[key] = hash
	}
	return sums, nil
}

func canonicalGoSumHash(value string) bool {
	encoded, ok := strings.CutPrefix(value, "h1:")
	if !ok || encoded == "" {
		return false
	}
	digest, err := base64.StdEncoding.DecodeString(encoded)
	return err == nil && len(digest) == sha256.Size && base64.StdEncoding.EncodeToString(digest) == encoded
}
