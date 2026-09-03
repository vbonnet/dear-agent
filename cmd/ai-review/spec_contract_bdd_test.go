package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestBuildReviewPlan_RequiresExactReciprocalBDDBacklink(t *testing.T) {
	for _, tc := range []struct {
		name      string
		feature   string
		wantHuman bool
	}{
		{name: "exact SPEC backlink", feature: featureDocument("# SPEC: module/SPEC.md\n", "contract")},
		{name: "exact related backlink", feature: featureDocument("# RELATED-SPEC: module/SPEC.md\n", "contract")},
		{name: "wrong SPEC backlink", feature: featureDocument("# SPEC: other/SPEC.md\n", "contract"), wantHuman: true},
		{name: "substring is not backlink", feature: featureDocument("# mentions module/SPEC.md\n", "contract"), wantHuman: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, "module/SPEC.md", specDocument("MOD-01", "When checked, the system shall report it.", "features/module.feature"))
			writeReviewFile(t, repo, "features/module.feature", tc.feature)
			gittest.Run(t, repo, "add", "module/SPEC.md", "features/module.feature")
			gittest.Run(t, repo, "commit", "-m", "add contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)
			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if plan.needsHuman() != tc.wantHuman {
				t.Fatalf("HumanReasons = %v, want human=%t", plan.HumanReasons, tc.wantHuman)
			}
		})
	}
}

func TestBuildReviewPlan_RejectsNonBacktickedFeaturePath(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	spec := "# Contract\n\n**MOD-01** When checked, the system shall report it.\n\n## BDD Traceability\n\n- Feature: features/module.feature\n"
	writeReviewFile(t, repo, "module/SPEC.md", spec)
	writeReviewFile(t, repo, "features/module.feature", featureDocument("# SPEC: module/SPEC.md\n", "contract"))
	gittest.Run(t, repo, "add", "module/SPEC.md", "features/module.feature")
	gittest.Run(t, repo, "commit", "-m", "add contract")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)
	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "lacks a BDD feature") {
		t.Fatalf("non-backticked feature path was accepted: %v", plan.HumanReasons)
	}
}

func TestBuildReviewPlan_AllowsAuthenticatedDeterministicNoBDDEvidence(t *testing.T) {
	for _, test := range []struct {
		line        string
		heading     string
		consequence string
	}{
		{line: "- No BDD change, with reason: deterministic unit coverage proves the private parser seam.", heading: "## BDD traceability", consequence: "No BDD change, with reason: deterministic unit coverage proves the private parser seam."},
		{line: "- Test consequence: Deterministic schema test validates the private protocol boundary.", heading: "## BDD Traceability", consequence: "Deterministic schema test validates the private protocol boundary."},
	} {
		t.Run(test.consequence, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			spec := "# Contract\n\n**MOD-01** When checked, the system shall report it.\n\n" + test.heading + "\n\n" + test.line + "\n"
			writeReviewFile(t, repo, "module/SPEC.md", spec)
			gittest.Run(t, repo, "add", "module/SPEC.md")
			gittest.Run(t, repo, "commit", "-m", "add deterministic contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if plan.needsHuman() || len(plan.Contracts) != 1 || plan.Contracts[0].TestConsequence != test.consequence || len(plan.Contracts[0].Features) != 0 {
				t.Fatalf("deterministic no-BDD evidence was not retained: %#v, reasons=%v", plan.Contracts, plan.HumanReasons)
			}
		})
	}
}

func TestBuildReviewPlan_RejectsMalformedCanonicalNoBDDEvidence(t *testing.T) {
	for _, line := range []string{
		"- No BDD change with reason: parser coverage exists.",
		"- No BDD change, with reason:",
		"- no bdd change, with reason: parser coverage exists.",
	} {
		t.Run(line, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			spec := "# Contract\n\n**MOD-01** When checked, the system shall report it.\n\n## BDD Traceability\n\n" + line + "\n"
			writeReviewFile(t, repo, "module/SPEC.md", spec)
			gittest.Run(t, repo, "add", "module/SPEC.md")
			gittest.Run(t, repo, "commit", "-m", "add malformed no-BDD evidence")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)
			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "lacks a BDD feature") {
				t.Fatalf("malformed canonical no-BDD declaration was accepted: %#v", plan)
			}
		})
	}
}

func TestBuildReviewPlan_RequiresRegularBDDFeatureObject(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	writeReviewFile(t, repo, "module/SPEC.md", specDocument("MOD-01", "When checked, the system shall report it.", "features/module.feature"))
	writeReviewFile(t, repo, "features/module.feature", "# SPEC: module/SPEC.md\n")
	gittest.Run(t, repo, "add", "module/SPEC.md", "features/module.feature")
	featureObject := strings.TrimSpace(gittest.Run(t, repo, "hash-object", "features/module.feature"))
	gittest.Run(t, repo, "update-index", "--cacheinfo", "120000,"+featureObject+",features/module.feature")
	gittest.Run(t, repo, "commit", "-m", "add symlink-shaped feature evidence")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "unavailable or non-regular") {
		t.Fatalf("non-regular BDD evidence was accepted: %#v", plan)
	}
}

func TestBuildReviewPlan_RequiresRunnableGherkinAndSuppliesAuthenticatedFeatureEvidence(t *testing.T) {
	for _, test := range []struct {
		name      string
		feature   string
		wantHuman bool
	}{
		{name: "feature without scenarios", feature: "# SPEC: module/SPEC.md\nFeature: contract\n", wantHuman: true},
		{name: "scenario without steps", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario: empty\n", wantHuman: true},
		{name: "outline without examples", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: empty <value>\n    Given value <value>\n", wantHuman: true},
		{name: "outline with examples heading but no table", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: empty <value>\n    Given value <value>\n\n    Examples:\n", wantHuman: true},
		{name: "outline with header but no rows", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: empty <value>\n    Given value <value>\n\n    Examples:\n      | value |\n", wantHuman: true},
		{name: "outline with malformed row", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: malformed <first> <second>\n    Given values <first> and <second>\n\n    Examples:\n      | first | second |\n      | one   |\n", wantHuman: true},
		{name: "outline with empty executable step", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: blank value\n    Given <value>\n\n    Examples:\n      | value |\n      |       |\n", wantHuman: true},
		{name: "runnable scenario does not mask empty outline", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario: runnable\n    Given the contract is exercised\n\n  Scenario Outline: empty <value>\n    Given value <value>\n", wantHuman: true},
		{name: "runnable scenario", feature: featureDocument("# SPEC: module/SPEC.md\n", "contract")},
		{name: "runnable outline", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: case <value>\n    Given value <value>\n\n    Examples:\n      | value |\n      | one   |\n"},
		{name: "runnable outline with an inert examples block", feature: "# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: case <value>\n    Given value <value>\n\n    Examples:\n\n    Examples:\n      | value |\n      | one   |\n"},
		{name: "localized runnable outline", feature: "# language: fr\n# SPEC: module/SPEC.md\nFonctionnalité: contrat\n\n  Plan du scénario: cas <valeur>\n    Soit une valeur <valeur>\n\n    Exemples:\n      | valeur |\n      | une    |\n"},
		{name: "localized outline without rows", feature: "# language: fr\n# SPEC: module/SPEC.md\nFonctionnalité: contrat\n\n  Plan du scénario: cas <valeur>\n    Soit une valeur <valeur>\n\n    Exemples:\n      | valeur |\n", wantHuman: true},
		{name: "outline exceeding bounded executable cases", feature: outlineFeatureWithRows(maxFeatureExecutableCases + 1), wantHuman: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, "module/SPEC.md", specDocument("MOD-01", "When checked, the system shall report it.", "features/module.feature"))
			writeReviewFile(t, repo, "features/module.feature", test.feature)
			gittest.Run(t, repo, "add", "module/SPEC.md", "features/module.feature")
			gittest.Run(t, repo, "commit", "-m", "add contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if plan.needsHuman() != test.wantHuman {
				t.Fatalf("HumanReasons = %v, want human=%t", plan.HumanReasons, test.wantHuman)
			}
			if !test.wantHuman {
				if len(plan.Contracts) != 1 || len(plan.Contracts[0].Features) != 1 || len(plan.Contracts[0].Features[0].Scenarios) != 1 || plan.Contracts[0].Features[0].Content != test.feature {
					t.Fatalf("authenticated Gherkin evidence = %#v", plan.Contracts)
				}
			}
		})
	}
}

func TestParseBDDFeature_PreservesCompleteEvidenceProjection(t *testing.T) {
	const source = `@feature-b @feature-a
Feature: contract

  Background:
    Given shared setup

  Rule: payment rule

    Background:
      And rule setup

    @case-b @case-a
    Scenario: first case
      When action occurs
      Then result follows

    Scenario: second case
      Given another action
`

	evidence, err := parseBDDFeature("features/contract.feature", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"path":"features/contract.feature","language":"en","name":"contract",` +
		`"tags":["@feature-b","@feature-a"],"scenarios":[` +
		`{"rule":"payment rule","keyword":"Scenario","name":"first case",` +
		`"tags":["@case-b","@case-a"],"steps":["When action occurs","Then result follows"]},` +
		`{"rule":"payment rule","keyword":"Scenario","name":"second case",` +
		`"tags":[],"steps":["Given another action"]}],"content":` + strconv.Quote(source) + "}"
	if string(got) != want {
		t.Fatalf("evidence JSON = %s, want %s", got, want)
	}
}

func outlineFeatureWithRows(rows int) string {
	var feature strings.Builder
	feature.WriteString("# SPEC: module/SPEC.md\nFeature: contract\n\n  Scenario Outline: case <value>\n    Given value <value>\n\n    Examples:\n      | value |\n")
	for i := range rows {
		fmt.Fprintf(&feature, "      | %d |\n", i)
	}
	return feature.String()
}

func TestParseBDDFeature_BoundsOrderedOutlineSubstitution(t *testing.T) {
	if _, err := parseBDDFeature("features/multi-column.feature", []byte(orderedOutlineFeature(12, false))); err != nil {
		t.Fatalf("ordinary multi-column outline was rejected: %v", err)
	}
	if _, err := parseBDDFeature("features/amplified.feature", []byte(orderedOutlineFeature(25, true))); err == nil || !strings.Contains(err.Error(), "interpolation exceeds the review allocation limit") {
		t.Fatalf("amplifying outline error = %v, want bounded interpolation failure", err)
	}
}

func TestParseBDDFeature_BoundsInheritedOutlineStepInstances(t *testing.T) {
	for _, tt := range []struct {
		name string
		rule bool
	}{
		{name: "feature background"},
		{name: "rule background", rule: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseBDDFeature("features/inherited.feature", []byte(outlineWithBackgroundSteps(129, 128, tt.rule)))
			if err == nil || !strings.Contains(err.Error(), "too many executable Gherkin steps") {
				t.Fatalf("inherited outline error = %v, want executable-step bound", err)
			}
		})
	}
	if _, err := parseBDDFeature("features/inherited-small.feature", []byte(outlineWithBackgroundSteps(2, 2, false))); err != nil {
		t.Fatalf("small inherited outline was rejected: %v", err)
	}
}

func TestParseBDDFeature_ValidatesOutlineBackgroundSubstitutions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		feature string
		want    string
	}{
		{
			name:    "feature background rejects an empty substituted step",
			feature: "Feature: inherited substitution\n\n  Background:\n    Given <value>\n\n  Scenario Outline: bounded values\n    Given stable step\n\n    Examples:\n      | value |\n      |       |\n",
			want:    "empty executable step",
		},
		{
			name:    "rule background applies ordered bounded substitutions",
			feature: outlineWithSubstitutingBackground(25, true),
			want:    "interpolation exceeds the review allocation limit",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseBDDFeature("features/inherited-substitution.feature", []byte(tt.feature))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("inherited substitution error = %v, want %q", err, tt.want)
			}
		})
	}
}

func outlineWithBackgroundSteps(backgroundSteps, rows int, rule bool) string {
	var feature strings.Builder
	feature.WriteString("Feature: inherited execution\n")
	indent := "  "
	if rule {
		feature.WriteString("\n  Rule: inherited scope\n")
		indent = "    "
	}
	feature.WriteString("\n" + indent + "Background:\n")
	for i := range backgroundSteps {
		fmt.Fprintf(&feature, "%s  Given inherited step %d\n", indent, i)
	}
	feature.WriteString("\n" + indent + "Scenario Outline: bounded <value>\n" + indent + "  Given value <value>\n\n" + indent + "Examples:\n" + indent + "  | value |\n")
	for i := range rows {
		fmt.Fprintf(&feature, "%s  | %d |\n", indent, i)
	}
	return feature.String()
}

func orderedOutlineFeature(columns int, amplify bool) string {
	var feature strings.Builder
	feature.WriteString("Feature: ordered substitution\n\n  Scenario Outline: bounded values\n    Given ")
	if amplify {
		feature.WriteString("<column-0>")
	} else {
		for i := range columns {
			fmt.Fprintf(&feature, "<column-%d> ", i)
		}
	}
	feature.WriteString("\n\n    Examples:\n      ")
	for i := range columns {
		fmt.Fprintf(&feature, "| column-%d ", i)
	}
	feature.WriteString("|\n      ")
	for i := range columns {
		value := fmt.Sprintf("value-%d", i)
		if amplify && i+1 < columns {
			value = fmt.Sprintf("<column-%d><column-%d>", i+1, i+1)
		}
		fmt.Fprintf(&feature, "| %s ", value)
	}
	feature.WriteString("|\n")
	return feature.String()
}

func outlineWithSubstitutingBackground(columns int, rule bool) string {
	var feature strings.Builder
	feature.WriteString("Feature: ordered inherited substitution\n")
	indent := "  "
	if rule {
		feature.WriteString("\n  Rule: inherited scope\n")
		indent = "    "
	}
	feature.WriteString("\n" + indent + "Background:\n" + indent + "  Given <column-0>\n\n")
	feature.WriteString(indent + "Scenario Outline: bounded values\n" + indent + "  Given stable step\n\n" + indent + "Examples:\n" + indent + "  ")
	for i := range columns {
		fmt.Fprintf(&feature, "| column-%d ", i)
	}
	feature.WriteString("|\n" + indent + "  ")
	for i := range columns {
		value := fmt.Sprintf("value-%d", i)
		if i+1 < columns {
			value = fmt.Sprintf("<column-%d><column-%d>", i+1, i+1)
		}
		fmt.Fprintf(&feature, "| %s ", value)
	}
	feature.WriteString("|\n")
	return feature.String()
}
