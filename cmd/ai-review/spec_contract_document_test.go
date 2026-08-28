package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMarkdownContractParsing_UsesCanonicalTraceabilityAndSkipsFences(t *testing.T) {
	document := strings.Join([]string{
		"# Contract",
		"",
		"<!--",
		"**COMMENTED-01** When hidden, the system shall ignore this requirement.",
		"## BDD Traceability",
		"- Feature: `features/commented.feature`",
		"-->",
		"",
		"```markdown",
		"**FAKE-01** When copied, the system shall ignore this example.",
		"```",
		"",
		"    **INDENTED-01** When copied, the system shall ignore this example.",
		"",
		"> ```markdown",
		"> **NESTED-01** When copied, the system shall ignore this example.",
		"> - Feature: `features/nested.feature`",
		"> ```",
		"",
		"**REAL-01** When checked, the system shall report it.",
		"",
		"## Notes",
		"",
		"- Feature: `features/outside.feature`",
		"",
		"## BDD Traceability",
		"",
		"```markdown",
		"- Feature: `features/fenced.feature`",
		"```",
		"- Feature: `features/real.feature`",
		"",
	}, "\n")
	requirements, err := parseRequirements(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 1 || requirements[0].ID != "REAL-01" {
		t.Fatalf("requirements = %#v, want only visible REAL-01", requirements)
	}
	features := exactFeaturePaths(document)
	if len(features) != 1 || features[0] != "features/real.feature" {
		t.Fatalf("features = %#v, want only canonical visible traceability", features)
	}
	if hasExactSpecBacklink("```\n# SPEC: module/SPEC.md\n```\nFeature: example\n", "module/SPEC.md") {
		t.Fatal("accepted a reciprocal backlink from fenced example text")
	}
	if hasExactSpecBacklink("<!--\n# SPEC: module/SPEC.md\n-->\nFeature: example\n", "module/SPEC.md") {
		t.Fatal("accepted a reciprocal backlink from HTML-commented text")
	}
}

func TestExactTraceability_AcceptsRepositoryTemplateWithoutBroadeningGrammar(t *testing.T) {
	templateBytes, err := os.ReadFile(filepath.Join("..", "..", "docs", "templates", "SPEC.md.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	const featurePath = "agm/test/bdd/features/template-contract.feature"
	document := strings.ReplaceAll(string(templateBytes), "{repository-relative-path-to-feature.feature}", featurePath)
	if got := exactFeaturePaths(document); !slices.Equal(got, []string{featurePath}) {
		t.Fatalf("template feature paths = %v, want [%s]", got, featurePath)
	}

	for _, tc := range []struct {
		name     string
		document string
		want     []string
	}{
		{name: "established labeled form", document: "## BDD Traceability\n\n- Feature: `features/established.feature`\n", want: []string{"features/established.feature"}},
		{name: "canonical bare form", document: "## BDD traceability\n\n- `features/canonical.feature`\n", want: []string{"features/canonical.feature"}},
		{name: "unregistered heading case", document: "## bdd traceability\n\n- `features/broad.feature`\n"},
		{name: "unregistered label case", document: "## BDD Traceability\n\n- feature: `features/broad.feature`\n"},
		{name: "bare path under established heading", document: "## BDD Traceability\n\n- `features/broad.feature`\n"},
		{name: "label under canonical heading", document: "## BDD traceability\n\n- Feature: `features/broad.feature`\n"},
		{name: "unquoted bare path", document: "## BDD traceability\n\n- features/broad.feature\n"},
		{name: "trailing annotation", document: "## BDD traceability\n\n- `features/broad.feature` canonical\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := exactFeaturePaths(tc.document); !slices.Equal(got, tc.want) {
				t.Fatalf("feature paths = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVisibleMarkdown_PreservesLinesWhileBlankingHiddenEARSProse(t *testing.T) {
	document := strings.Join([]string{
		"# Contract",
		"<!--",
		"A commented sentence shall remain hidden.",
		"-->",
		"",
		"    An indented sentence shall remain hidden.",
		"",
		"> ```text",
		"> A nested sentence shall remain hidden.",
		"> ```",
		"A visible sentence shall remain on its original line.",
	}, "\n")
	got := strings.Split(visibleMarkdown(markdownLines(document)), "\n")
	wantLines := strings.Split(document, "\n")
	if len(got) != len(wantLines) {
		t.Fatalf("visible view has %d lines, want %d", len(got), len(wantLines))
	}
	for _, hidden := range []string{"commented sentence", "indented sentence", "nested sentence"} {
		if strings.Contains(strings.Join(got, "\n"), hidden) {
			t.Fatalf("visible view retained hidden prose %q: %q", hidden, got)
		}
	}
	if got[10] != wantLines[10] {
		t.Fatalf("visible line moved or changed: line 11 = %q, want %q", got[10], wantLines[10])
	}
}
