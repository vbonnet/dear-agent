package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// The SPEC reviewer builds from the repository's Go dependency graph, so a
// change to that graph can change the reviewer that reviews it. Treating "the
// file changed at all" as the boundary, however, escalates every routine
// dependency bump to a sleeping human: PR 1434 (an OpenTelemetry family bump)
// and PR 1441 (the x/crypto and fast-uri security patches) were both forced to
// needs-human-review with no finding other than "go.mod changed", which is a
// throughput killer and, for a security patch, a availability risk of its own.
//
// This file draws the boundary at the parsed delta instead. A version-only
// update to modules the manifest already required, with no new direct
// requirement, no major-version movement, no replace directive, and no go or
// toolchain change, is ordinary work: it still faces green CI, the
// five-dimension review, and the vulnerability scans, but it no longer needs a
// maintainer decision. Every other dependency-graph change keeps the
// fail-closed maintainer requirement it has today.

// reviewerDependencyReasonPrefix is the stable leading text of every
// dependency-graph escalation. onlyReviewerDependencyReasons and the trusted
// workflow classify by it, so the path and cause may only follow it.
const reviewerDependencyReasonPrefix = "SPEC reviewer dependency graph change requires maintainer review ("

// dependencyGraphDelta is the dependency-graph slice of one authenticated diff.
// Routine is true only when every dependency path in the delta is proven to be
// an ordinary dependency version update; Causes records, in path order, why a
// delta failed that proof so the escalation says something actionable.
type dependencyGraphDelta struct {
	Paths   []string `json:"paths"`
	Routine bool     `json:"routine"`
	Causes  []string `json:"causes"`
}

// cause returns the recorded refusal for one dependency path, or the empty
// string when that path is part of a proven routine update.
func (delta dependencyGraphDelta) cause(path string) string {
	index := slices.Index(delta.Paths, path)
	if delta.Routine || index < 0 || index >= len(delta.Causes) {
		return ""
	}
	return delta.Causes[index]
}

// classifyDependencyGraphDelta proves whether the dependency-graph paths in an
// authenticated delta are an ordinary dependency version update. It is
// deliberately fail-closed on everything it cannot prove: unreadable blobs,
// unparsable manifests, workspace or vendored inputs, and any go.mod status
// other than a plain modification all stay maintainer decisions. Only a
// genuine Git or bounds failure is returned as an error.
func classifyDependencyGraphDelta(ctx context.Context, mergeBase, head string, statuses map[string]string) (dependencyGraphDelta, error) {
	delta := dependencyGraphDelta{Paths: []string{}, Causes: []string{}}
	for _, path := range slices.Sorted(maps.Keys(statuses)) {
		if specReviewDependencyPath(path) {
			delta.Paths = append(delta.Paths, path)
		}
	}
	if len(delta.Paths) == 0 {
		return delta, nil
	}

	manifests := make([]string, 0, len(delta.Paths))
	for _, path := range delta.Paths {
		cause := dependencyManifestCause(path, statuses[path])
		delta.Causes = append(delta.Causes, cause)
		if cause == "" {
			manifests = append(manifests, path)
		}
	}
	// A go.sum without its go.mod proves nothing about the requirement graph,
	// and an empty manifest set means every path already failed above.
	if !slices.Contains(manifests, "go.mod") {
		markUnprovenDependencyPaths(&delta, "cannot be proven without a modified go.mod")
		return delta, nil
	}

	blobs, err := dependencyManifestBlobs(ctx, mergeBase, head, manifests)
	if err != nil {
		return dependencyGraphDelta{}, err
	}
	if blobs == nil {
		markUnprovenDependencyPaths(&delta, "is not a regular text blob in both revisions")
		return delta, nil
	}

	cause, err := moduleDependencyUpdateCause(blobs.base["go.mod"], blobs.head["go.mod"])
	if err != nil {
		cause = "does not parse as a Go module manifest"
	}
	if cause == "" && slices.Contains(manifests, "go.sum") {
		safe, err := goSumChangeSafe(blobs.base["go.sum"], blobs.head["go.sum"])
		if err != nil {
			cause = "has an unparsable go.sum"
		} else if !safe {
			cause = "rewrites the checksum of a retained module version"
		}
	}
	if cause != "" {
		markUnprovenDependencyPaths(&delta, cause)
		return delta, nil
	}
	// The proof is whole-delta: a routine manifest bump does not clear a
	// workspace or vendored path that was already refused above.
	delta.Routine = !slices.ContainsFunc(delta.Causes, func(cause string) bool { return cause != "" })
	return delta, nil
}

// dependencyManifestCause rejects every dependency-graph path that is not a
// plainly modified root Go manifest. Workspace files and vendored trees carry
// build inputs this proof does not model, and an added, deleted, or retyped
// manifest has no version delta to compare.
func dependencyManifestCause(path, status string) string {
	identity := normalizedPathIdentity(path)
	if identity != "go.mod" && identity != "go.sum" {
		return "is a workspace or vendored build input rather than a module manifest"
	}
	if status != "M" {
		return "is added, deleted, or retyped rather than modified"
	}
	return ""
}

// markUnprovenDependencyPaths records one shared refusal on every dependency
// path that had not already failed for a more specific reason. A refusal is
// whole-delta by construction: the manifests are proven together.
func markUnprovenDependencyPaths(delta *dependencyGraphDelta, cause string) {
	for index, existing := range delta.Causes {
		if existing == "" {
			delta.Causes[index] = cause
		}
	}
}

type dependencyManifestPair struct {
	base map[string][]byte
	head map[string][]byte
}

// dependencyManifestBlobs reads both revisions of the proven manifest paths.
// A nil pair means a path was absent or not a regular text blob, which the
// caller turns into a refusal rather than an error.
func dependencyManifestBlobs(ctx context.Context, mergeBase, head string, paths []string) (*dependencyManifestPair, error) {
	baseBlobs, err := gitRegularTextBlobsBounded(ctx, mergeBase, paths, maxDependabotModuleBlobBytes, len(paths)*maxDependabotModuleBlobBytes)
	if err != nil {
		return nil, fmt.Errorf("read merge-base dependency manifests: %w", err)
	}
	headBlobs, err := gitRegularTextBlobsBounded(ctx, head, paths, maxDependabotModuleBlobBytes, len(paths)*maxDependabotModuleBlobBytes)
	if err != nil {
		return nil, fmt.Errorf("read head dependency manifests: %w", err)
	}
	if len(baseBlobs) != len(paths) || len(headBlobs) != len(paths) {
		return nil, nil
	}
	return &dependencyManifestPair{base: baseBlobs, head: headBlobs}, nil
}
