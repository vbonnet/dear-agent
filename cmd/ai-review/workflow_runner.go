package main

import (
	"regexp"

	"go.yaml.in/yaml/v3"
)

var (
	workflowMatrixRunner = regexp.MustCompile(`^\$\{\{\s*matrix\.([A-Za-z0-9_-]+)\s*\}\}$`)
)

// githubHostedRunnerLabels is the finite standard public-repository label set
// documented by GitHub on 2026-08-10. A shape regex is unsafe because a
// self-hosted runner may claim any custom label, including an invented
// ubuntu-NN.NN spelling. Unknown and future labels fail closed until this
// source-pinned list is deliberately refreshed.
// Source: https://docs.github.com/en/actions/reference/runners/github-hosted-runners
var githubHostedRunnerLabels = map[string]bool{
	"ubuntu-slim":           true,
	"ubuntu-latest":         true,
	"ubuntu-22.04":          true,
	"ubuntu-24.04":          true,
	"ubuntu-26.04":          true,
	"ubuntu-22.04-arm":      true,
	"ubuntu-24.04-arm":      true,
	"ubuntu-26.04-arm":      true,
	"windows-latest":        true,
	"windows-2022":          true,
	"windows-2025":          true,
	"windows-2025-vs2026":   true,
	"windows-11-arm":        true,
	"windows-11-vs2026-arm": true,
	"macos-latest":          true,
	"macos-14":              true,
	"macos-15":              true,
	"macos-26":              true,
	"macos-15-intel":        true,
	"macos-26-intel":        true,
}

func workflowRunnerReason(job, runner *yaml.Node) string {
	check := func(value string) string {
		value = asciiLower(value)
		if githubHostedRunnerLabels[value] {
			return ""
		}
		match := workflowMatrixRunner.FindStringSubmatch(value)
		if len(match) != 2 || !workflowMatrixUsesOnlyGitHubHostedRunners(job, match[1]) {
			return "workflow runner authority is self-hosted, custom, or dynamic"
		}
		return ""
	}
	switch runner.Kind {
	case yaml.ScalarNode:
		return check(runner.Value)
	case yaml.SequenceNode:
		// Actions treats a runner sequence as an all-label conjunction. Even
		// when every spelling is a standard hosted label, no hosted runner is
		// proven to satisfy multiple labels while a self-hosted runner may claim
		// them all. Only one proven hosted selector is safe.
		if len(runner.Content) != 1 {
			return "workflow runner authority is ambiguous"
		}
		label := runner.Content[0]
		if label.Kind != yaml.ScalarNode || check(label.Value) != "" {
			return "workflow runner authority is self-hosted, custom, or dynamic"
		}
		return ""
	case yaml.DocumentNode, yaml.MappingNode, yaml.AliasNode:
		return "workflow runner authority is ambiguous"
	default:
		return "workflow runner authority is ambiguous"
	}
}

func workflowMatrixUsesOnlyGitHubHostedRunners(job *yaml.Node, key string) bool {
	matrix, ok := workflowRunnerMatrix(job)
	if !ok {
		return false
	}
	if !workflowMatrixAxisUsesOnlyGitHubHostedRunners(matrix, key) {
		return false
	}
	return workflowMatrixIncludesUseOnlyGitHubHostedRunners(matrix, key)
}

func workflowRunnerMatrix(job *yaml.Node) (*yaml.Node, bool) {
	strategy, ok := mappingNodeValue(job, "strategy")
	if !ok || strategy.Kind != yaml.MappingNode {
		return nil, false
	}
	matrix, ok := mappingNodeValue(strategy, "matrix")
	if !ok || matrix.Kind != yaml.MappingNode {
		return nil, false
	}
	return matrix, true
}

func workflowMatrixAxisUsesOnlyGitHubHostedRunners(matrix *yaml.Node, key string) bool {
	values, ok, ambiguous := workflowMatrixAxisValue(matrix, key)
	if ambiguous || !ok || values.Kind != yaml.SequenceNode || len(values.Content) == 0 {
		return false
	}
	for _, value := range values.Content {
		if value.Kind != yaml.ScalarNode || !githubHostedRunnerLabels[asciiLower(value.Value)] {
			return false
		}
	}
	return true
}

func workflowMatrixAxisValue(matrix *yaml.Node, key string) (*yaml.Node, bool, bool) {
	wanted := asciiLower(key)
	var value *yaml.Node
	for i := 0; i < len(matrix.Content); i += 2 {
		rawKey := matrix.Content[i].Value
		// The runner converter recognizes only exact lowercase include/exclude
		// as controls. Case variants are ordinary matrix axes.
		if rawKey == "include" || rawKey == "exclude" || asciiLower(rawKey) != wanted {
			continue
		}
		if value != nil {
			return nil, false, true
		}
		value = matrix.Content[i+1]
	}
	return value, value != nil, false
}

func workflowMatrixIncludesUseOnlyGitHubHostedRunners(matrix *yaml.Node, key string) bool {
	include, ok := mappingNodeValue(matrix, "include")
	if !ok {
		return true
	}
	if include.Kind != yaml.SequenceNode {
		return false
	}
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode {
			return false
		}
		override, ok, ambiguous := mappingNodeValueASCIIFold(entry, key)
		if ambiguous {
			return false
		}
		if !ok {
			continue
		}
		if override.Kind != yaml.ScalarNode || !githubHostedRunnerLabels[asciiLower(override.Value)] {
			return false
		}
	}
	return true
}

func mappingNodeValueASCIIFold(mapping *yaml.Node, key string) (*yaml.Node, bool, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false, false
	}
	wanted := asciiLower(key)
	var value *yaml.Node
	for i := 0; i < len(mapping.Content); i += 2 {
		if asciiLower(mapping.Content[i].Value) != wanted {
			continue
		}
		if value != nil {
			return nil, false, true
		}
		value = mapping.Content[i+1]
	}
	return value, value != nil, false
}
