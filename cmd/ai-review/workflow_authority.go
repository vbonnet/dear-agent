package main

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

var criticalWorkflowWriteScopes = map[string]bool{
	"actions":       true,
	"attestations":  true,
	"checks":        true,
	"contents":      true,
	"deployments":   true,
	"id-token":      true,
	"packages":      true,
	"pages":         true,
	"pull-requests": true,
	// security-events can update/dismiss code-scanning alerts and replace a
	// commit/category's SARIF results; GitHub code-scanning rules can use those
	// results to allow or block merges. It is therefore gate authority, not a
	// low-blast reporting permission.
	// Sources (verified 2026-08-10):
	// https://docs.github.com/en/rest/code-scanning/code-scanning
	// https://docs.github.com/en/code-security/concepts/code-scanning/merge-protection
	// https://docs.github.com/en/enterprise-cloud@latest/code-security/how-tos/find-and-fix-code-vulnerabilities/integrate-with-existing-tools/upload-sarif-file
	"security-events": true,
	"statuses":        true,
	// issues: write is review-control authority, not a low-blast reporting
	// permission. GitHub models pull requests as issues, so the issues API
	// adds and removes *pull request* labels; the labels in
	// [mergeloop.DefaultBlockLabels] are the human-only merge block. A
	// workflow holding this scope can therefore strip needs-security-review
	// (or any peer) before the merge loop classifies the PR, which is exactly
	// the bypass this classifier exists to close.
	// Sources (verified 2026-08-18):
	// https://docs.github.com/en/rest/issues/labels
	// https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps
	"issues": true,
}

// lowBlastWorkflowWriteScopes are write scopes that cannot reach source,
// review, gate, deployment, signing, or merge-gate state. A scope belongs here
// only when no API it unlocks can mutate a pull request's labels, checks,
// statuses, reviews, or contents. TestMergeGateLabelScopesAreNeverLowBlast
// keeps that invariant honest against the merge loop's own block labels.
var lowBlastWorkflowWriteScopes = map[string]bool{
	"discussions": true,
}

// workflowPrivilegeReason classifies only authority-bearing behavior. Routine
// read-only workflows remain automated; malformed or ambiguous workflow input
// is itself privileged because treating unreadable authority as neutral would
// recreate the bypass this classifier closes.
func workflowPrivilegeReason(blob []byte) (string, bool) {
	root, _, err := parseWorkflowYAML(blob)
	if err != nil {
		return "malformed or ambiguous workflow YAML", true
	}
	reason := workflowRootAuthorityReason(root)
	return reason, reason != ""
}

func workflowRootAuthorityReason(root *yaml.Node) string {
	if nodeContainsCustomSecretReference(root) {
		return "workflow consumes a non-default or dynamic secret"
	}
	on, ok := mappingNodeValue(root, "on")
	if !ok {
		return "workflow trigger evidence is missing"
	}
	if reason := privilegedWorkflowEventReason(on); reason != "" {
		return reason
	}
	if reason := workflowCallSecretsReason(on); reason != "" {
		return reason
	}
	rootPermissions, hasRootPermissions := mappingNodeValue(root, "permissions")
	if hasRootPermissions && !validWorkflowPermissionsShape(rootPermissions) {
		return "workflow permissions evidence is ambiguous"
	}
	jobs, ok := mappingNodeValue(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode || len(jobs.Content) == 0 {
		return "workflow jobs evidence is missing or malformed"
	}
	return workflowJobsAuthorityReason(jobs, rootPermissions)
}

func workflowJobsAuthorityReason(jobs, rootPermissions *yaml.Node) string {
	for i := 1; i < len(jobs.Content); i += 2 {
		if reason := workflowJobAuthorityReason(jobs.Content[i], rootPermissions); reason != "" {
			return reason
		}
	}
	return ""
}

func workflowJobAuthorityReason(job, rootPermissions *yaml.Node) string {
	if job.Kind != yaml.MappingNode {
		return "workflow job evidence is malformed"
	}
	if reason := workflowJobPermissionsAuthorityReason(job, rootPermissions); reason != "" {
		return reason
	}
	if _, reusable := mappingNodeValue(job, "uses"); reusable {
		return "workflow delegates reusable-workflow or runner authority"
	}
	if reason := workflowJobEnvironmentAuthorityReason(job); reason != "" {
		return reason
	}
	if reason := workflowJobRunnerAuthorityReason(job); reason != "" {
		return reason
	}
	if nodeContainsCredentialsAuthority(job) {
		return "workflow supplies container, service, or action credentials"
	}
	if secrets, ok := mappingNodeValue(job, "secrets"); ok {
		return workflowJobSecretsReason(secrets)
	}
	return ""
}

func workflowJobPermissionsAuthorityReason(job, rootPermissions *yaml.Node) string {
	permissions, ok := mappingNodeValue(job, "permissions")
	if !ok {
		permissions = rootPermissions
	}
	if permissions == nil {
		return "workflow effective permissions are not explicit"
	}
	return workflowPermissionsReason(permissions)
}

func workflowJobEnvironmentAuthorityReason(job *yaml.Node) string {
	environment, ok := mappingNodeValue(job, "environment")
	if !ok {
		return ""
	}
	if environment.Kind != yaml.ScalarNode || strings.TrimSpace(environment.Value) != "" {
		return "workflow owns protected environment or deployment authority"
	}
	return "workflow environment evidence is ambiguous"
}

func workflowJobRunnerAuthorityReason(job *yaml.Node) string {
	runner, ok := mappingNodeValue(job, "runs-on")
	if !ok {
		return "workflow runner authority is missing or ambiguous"
	}
	return workflowRunnerReason(job, runner)
}

func nodeContainsCredentialsAuthority(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == "credentials" {
				value := node.Content[i+1]
				if value.Kind != yaml.MappingNode || len(value.Content) > 0 {
					return true
				}
			}
		}
	}
	return slices.ContainsFunc(node.Content, nodeContainsCredentialsAuthority)
}

func privilegedWorkflowEventReason(on *yaml.Node) string {
	privileged := func(event string) string {
		event, exact := exactWorkflowAuthorityToken(event)
		if !exact {
			return "workflow trigger evidence is ambiguous"
		}
		switch event {
		case "pull_request_target", "workflow_run":
			return "workflow uses a privileged default-branch event"
		default:
			return ""
		}
	}
	switch on.Kind {
	case yaml.ScalarNode:
		return privileged(on.Value)
	case yaml.SequenceNode:
		for _, event := range on.Content {
			if event.Kind != yaml.ScalarNode {
				return "workflow trigger evidence is ambiguous"
			}
			if reason := privileged(event.Value); reason != "" {
				return reason
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(on.Content); i += 2 {
			if reason := privileged(on.Content[i].Value); reason != "" {
				return reason
			}
		}
	case yaml.DocumentNode, yaml.AliasNode:
		return "workflow trigger evidence is ambiguous"
	default:
		return "workflow trigger evidence is ambiguous"
	}
	return ""
}

func workflowCallSecretsReason(on *yaml.Node) string {
	if on.Kind != yaml.MappingNode {
		return ""
	}
	// AIREV-27 deliberately treats authority-bearing event identifiers as one
	// ASCII-fold equivalence class. Duplicate folded peers are ambiguous even
	// when the provider would reject one spelling.
	call, ok, ambiguous := mappingNodeValueASCIIFold(on, "workflow_call")
	if ambiguous {
		return "workflow_call authority evidence is ambiguous"
	}
	if !ok || call.Kind == yaml.ScalarNode && call.Tag == "!!null" {
		return ""
	}
	if call.Kind != yaml.MappingNode {
		return "workflow_call authority evidence is ambiguous"
	}
	secrets, ok := mappingNodeValue(call, "secrets")
	if !ok {
		return ""
	}
	if secrets.Kind == yaml.MappingNode && len(secrets.Content) == 0 {
		return ""
	}
	return "workflow_call accepts secrets"
}

func workflowJobSecretsReason(secrets *yaml.Node) string {
	switch secrets.Kind {
	case yaml.MappingNode:
		if len(secrets.Content) == 0 {
			return ""
		}
		for i := 1; i < len(secrets.Content); i += 2 {
			value := secrets.Content[i]
			if value.Kind != yaml.ScalarNode || !scalarIsExactDefaultTokenReference(value.Value) {
				return "workflow forwards a non-default or dynamic secret"
			}
		}
		return ""
	case yaml.ScalarNode:
		if asciiLower(strings.TrimSpace(secrets.Value)) == "inherit" {
			return "workflow inherits secrets"
		}
		return "workflow secrets authority is ambiguous"
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return "workflow secrets authority is ambiguous"
	default:
		return "workflow secrets authority is ambiguous"
	}
}

func workflowPermissionsReason(permissions *yaml.Node) string {
	switch permissions.Kind {
	case yaml.MappingNode:
		return workflowPermissionsMappingReason(permissions)
	case yaml.ScalarNode:
		return workflowPermissionsScalarReason(permissions.Value)
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return "workflow permissions evidence is ambiguous"
	default:
		return "workflow permissions evidence is ambiguous"
	}
}

func workflowPermissionsMappingReason(permissions *yaml.Node) string {
	for i := 0; i < len(permissions.Content); i += 2 {
		if reason := workflowPermissionEntryReason(permissions.Content[i], permissions.Content[i+1]); reason != "" {
			return reason
		}
	}
	return ""
}

func workflowPermissionEntryReason(nameNode, value *yaml.Node) string {
	name, exact := exactWorkflowAuthorityToken(nameNode.Value)
	if !exact || value.Kind != yaml.ScalarNode {
		return "workflow permissions evidence is ambiguous"
	}
	level, exact := exactWorkflowAuthorityToken(value.Value)
	if !exact {
		return "workflow permissions evidence is ambiguous"
	}
	if level == "write" {
		if criticalWorkflowWriteScopes[name] {
			return "workflow grants source, review, gate, deployment, signing, or id-token authority"
		}
		if lowBlastWorkflowWriteScopes[name] {
			return ""
		}
		return "workflow grants an unknown write permission"
	}
	if level != "read" && level != "none" || name == "id-token" && level != "none" {
		return "workflow permissions evidence is ambiguous"
	}
	return ""
}

func workflowPermissionsScalarReason(value string) string {
	level, exact := exactWorkflowAuthorityToken(value)
	if !exact {
		return "workflow permissions evidence is ambiguous"
	}
	switch level {
	case "read-all":
		return ""
	case "write-all":
		return "workflow grants write authority"
	default:
		return "workflow permissions evidence is ambiguous"
	}
}

func validWorkflowPermissionsShape(permissions *yaml.Node) bool {
	switch permissions.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(permissions.Content); i += 2 {
			name, exact := exactWorkflowAuthorityToken(permissions.Content[i].Value)
			if !exact {
				return false
			}
			value := permissions.Content[i+1]
			if value.Kind != yaml.ScalarNode {
				return false
			}
			level, exact := exactWorkflowAuthorityToken(value.Value)
			if !exact {
				return false
			}
			if level != "read" && level != "write" && level != "none" || name == "id-token" && level == "read" {
				return false
			}
		}
		return true
	case yaml.ScalarNode:
		level, exact := exactWorkflowAuthorityToken(permissions.Value)
		if !exact {
			return false
		}
		return level == "read-all" || level == "write-all"
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return false
	default:
		return false
	}
}

// exactWorkflowAuthorityToken distinguishes YAML syntax whitespace, which the
// parser already removes, from quoted whitespace, which is part of the event,
// permission scope, or permission level. The latter is not provider-canonical
// authority evidence and must fail closed rather than being silently trimmed.
func exactWorkflowAuthorityToken(value string) (string, bool) {
	if value != strings.TrimSpace(value) || strings.Contains(value, "${{") {
		return "", false
	}
	return asciiLower(value), true
}
