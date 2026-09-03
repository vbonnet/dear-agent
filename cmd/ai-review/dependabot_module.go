package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
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
// identical. Only indirect requirement membership and indirect markers on
// retained requirements may change, but at least one requirement present on
// both sides must change version. A replace directive on either side is never
// eligible, even when it is unchanged.
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
	cause, err := moduleDependencyUpdateCause(base, head)
	return err == nil && cause == "", err
}

// moduleDependencyUpdateCause returns the empty string when the two manifests
// differ only by an ordinary dependency version update, and otherwise a short
// phrase naming the first foundational difference it found. The phrase is
// governance-facing evidence: it tells a contributor exactly which part of the
// delta requires maintainer judgment rather than only that one was required.
func moduleDependencyUpdateCause(base, head []byte) (string, error) {
	baseModule, err := parseModuleVersionState(base)
	if err != nil {
		return "", fmt.Errorf("parse base go.mod: %w", err)
	}
	headModule, err := parseModuleVersionState(head)
	if err != nil {
		return "", fmt.Errorf("parse head go.mod: %w", err)
	}
	switch {
	case baseModule.hasReplace || headModule.hasReplace:
		return "declares a replace directive", nil
	case baseModule.goDirective != headModule.goDirective:
		return "changes the go directive", nil
	case baseModule.toolchain != headModule.toolchain:
		return "changes the toolchain directive", nil
	case !bytes.Equal(baseModule.withoutRequireDirectives, headModule.withoutRequireDirectives):
		return "changes module identity, an exclude or retract directive, or other non-require syntax", nil
	case !equalModuleAnnotatedRequireBlocks(baseModule.annotatedRequireBlocks, headModule.annotatedRequireBlocks):
		return "changes a policy-annotated require block", nil
	}
	return moduleRequirementDeltaCause(baseModule.requirements, headModule.requirements), nil
}

// moduleRequirementDeltaCause classifies the require-block delta. Adding or
// dropping an unannotated indirect requirement is the ordinary consequence of
// resolving a version bump, so it stays routine; a new or removed direct
// requirement is a new dependency decision, and a major-version move is an
// API-surface decision. Both belong to a maintainer.
func moduleRequirementDeltaCause(base, head map[string]moduleRequirement) string {
	versionChanged := false
	for _, path := range slices.Sorted(maps.Keys(base)) {
		baseRequirement := base[path]
		headRequirement, retained := head[path]
		if !retained {
			if !baseRequirement.indirect || len(baseRequirement.annotations) != 0 {
				return fmt.Sprintf("removes direct requirement %s", path)
			}
			continue
		}
		if !slices.Equal(baseRequirement.annotations, headRequirement.annotations) {
			return fmt.Sprintf("changes the annotations on requirement %s", path)
		}
		if baseRequirement.version == headRequirement.version {
			continue
		}
		baseMajor, headMajor := semver.Major(baseRequirement.version), semver.Major(headRequirement.version)
		if baseMajor == "" || headMajor == "" {
			return fmt.Sprintf("gives requirement %s a non-canonical version", path)
		}
		if baseMajor != headMajor {
			return fmt.Sprintf("changes the major version of requirement %s", path)
		}
		versionChanged = true
	}
	for _, path := range slices.Sorted(maps.Keys(head)) {
		headRequirement := head[path]
		if _, existed := base[path]; !existed &&
			(!headRequirement.indirect || len(headRequirement.annotations) != 0) {
			return fmt.Sprintf("adds direct requirement %s", path)
		}
	}
	if !versionChanged {
		return "changes no existing requirement version"
	}
	return ""
}

type moduleVersionState struct {
	requirements             map[string]moduleRequirement
	annotatedRequireBlocks   []moduleAnnotatedRequireBlock
	withoutRequireDirectives []byte
	goDirective              string
	toolchain                string
	hasReplace               bool
}

type moduleRequirement struct {
	version     string
	indirect    bool
	annotations []string
}

type moduleAnnotatedRequireBlock struct {
	index   int
	block   moduleComments
	lparen  moduleComments
	rparen  moduleComments
	members []string
}

type moduleComments struct {
	before []string
	suffix []string
	after  []string
}

// parseModuleVersionState removes parsed require directives before formatting.
// Equality of the resulting bytes is therefore a strict proof that all
// non-require syntax and comments stayed fixed. Requirement and require-block
// annotations are authenticated separately, with the tool-managed indirect
// marker normalized away so directness may change without admitting arbitrary
// comment changes.
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
	// Capture both language directives before Cleanup and Format, which are
	// free to rewrite the syntax tree these fields describe.
	if file.Go != nil {
		state.goDirective = file.Go.Version
	}
	if file.Toolchain != nil {
		state.toolchain = file.Toolchain.Name
	}
	requirementPaths := make(map[*modfile.Line]string, len(file.Require))
	for _, requirement := range file.Require {
		path, parsed, err := parseModuleRequirement(requirement)
		if err != nil {
			return moduleVersionState{}, err
		}
		if _, duplicate := state.requirements[path]; duplicate {
			return moduleVersionState{}, fmt.Errorf("go.mod repeats requirement path %q", path)
		}
		state.requirements[path] = parsed
		requirementPaths[requirement.Syntax] = path
	}
	state.annotatedRequireBlocks, err = parseModuleAnnotatedRequireBlocks(file.Syntax, requirementPaths)
	if err != nil {
		return moduleVersionState{}, err
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
		indirect:    requirement.Indirect,
		annotations: annotations,
	}, nil
}

func parseModuleAnnotatedRequireBlocks(syntax *modfile.FileSyntax, requirementPaths map[*modfile.Line]string) ([]moduleAnnotatedRequireBlock, error) {
	if syntax == nil {
		return nil, nil
	}
	var result []moduleAnnotatedRequireBlock
	requireBlockIndex := 0
	for _, statement := range syntax.Stmt {
		block, ok := statement.(*modfile.LineBlock)
		if !ok || len(block.Token) != 1 || block.Token[0] != "require" {
			continue
		}
		annotatedBlock := moduleAnnotatedRequireBlock{
			index:  requireBlockIndex,
			block:  parseModuleComments(block.Comments),
			lparen: parseModuleComments(block.LParen.Comments),
			rparen: parseModuleComments(block.RParen.Comments),
		}
		requireBlockIndex++
		if annotatedBlock.block.empty() && annotatedBlock.lparen.empty() && annotatedBlock.rparen.empty() {
			continue
		}
		annotatedBlock.members = make([]string, 0, len(block.Line))
		for _, line := range block.Line {
			path, ok := requirementPaths[line]
			if !ok {
				return nil, errors.New("annotated go.mod require block contains unparsed requirement evidence")
			}
			annotatedBlock.members = append(annotatedBlock.members, path)
		}
		slices.Sort(annotatedBlock.members)
		result = append(result, annotatedBlock)
	}
	return result, nil
}

func parseModuleComments(comments modfile.Comments) moduleComments {
	return moduleComments{
		before: moduleCommentTokens(comments.Before),
		suffix: moduleCommentTokens(comments.Suffix),
		after:  moduleCommentTokens(comments.After),
	}
}

func moduleCommentTokens(comments []modfile.Comment) []string {
	tokens := make([]string, len(comments))
	for index, comment := range comments {
		tokens[index] = comment.Token
	}
	return tokens
}

func (comments moduleComments) empty() bool {
	return len(comments.before) == 0 && len(comments.suffix) == 0 && len(comments.after) == 0
}

func equalModuleAnnotatedRequireBlocks(base, head []moduleAnnotatedRequireBlock) bool {
	return slices.EqualFunc(base, head, func(base, head moduleAnnotatedRequireBlock) bool {
		return base.index == head.index &&
			equalModuleComments(base.block, head.block) &&
			equalModuleComments(base.lparen, head.lparen) &&
			equalModuleComments(base.rparen, head.rparen) &&
			slices.Equal(base.members, head.members)
	})
}

func equalModuleComments(base, head moduleComments) bool {
	return slices.Equal(base.before, head.before) &&
		slices.Equal(base.suffix, head.suffix) &&
		slices.Equal(base.after, head.after)
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
		// Go's own go.sum reader tokenizes fields and therefore accepts CRLF.
		// Preserve the otherwise exact line grammar while accepting that one
		// canonical cross-platform line ending.
		line = strings.TrimSuffix(line, "\r")
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
