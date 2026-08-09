package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	deepsecSameRepositoryAtom = "github.event.pull_request.head.repo.full_name == github.repository"
	deepsecFullCILabelAtom    = "contains(github.event.pull_request.labels.*.name, 'full-ci')"
	deepsecNotLabeledAtom     = "github.event.action != 'labeled'"
	deepsecAddedFullCIAtom    = "github.event.label.name == 'full-ci'"
)

type deepsecWorkflowContract struct {
	On   map[string]yaml.Node        `yaml:"on"`
	Jobs deepsecWorkflowContractJobs `yaml:"jobs"`
}

type deepsecWorkflowContractJobs struct {
	CheckKey deepsecWorkflowCheckKeyJob `yaml:"check-key"`
	Scan     deepsecWorkflowScanJob     `yaml:"scan"`
}

type deepsecWorkflowCheckKeyJob struct {
	If string `yaml:"if"`
}

type deepsecWorkflowScanJob struct {
	Needs string                `yaml:"needs"`
	If    string                `yaml:"if"`
	Steps []deepsecWorkflowStep `yaml:"steps"`
}

type deepsecWorkflowStep struct {
	Uses string `yaml:"uses"`
	With struct {
		Ref        string `yaml:"ref"`
		FetchDepth *int   `yaml:"fetch-depth"`
	} `yaml:"with"`
}

type deepsecPullRequestTrigger struct {
	Branches []string `yaml:"branches"`
	Types    []string `yaml:"types"`
}

type deepsecAdmissionEvent struct {
	HeadRepository string
	Repository     string
	Action         string
	AddedLabel     string
	Labels         []string
}

func TestDeepsecWorkflowContract(t *testing.T) {
	t.Parallel()

	path := filepath.Join(packageSpecBDDRepoRoot(), ".github", "workflows", "deepsec.yml")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var workflow deepsecWorkflowContract
	if err := yaml.Unmarshal(source, &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	if got := sortedDeepsecKeys(workflow.On); !slices.Equal(got, []string{"pull_request"}) {
		t.Fatalf("deepsec triggers = %v, want only pull_request", got)
	}
	pullRequestNode := workflow.On["pull_request"]
	var triggerFields map[string]yaml.Node
	if err := pullRequestNode.Decode(&triggerFields); err != nil {
		t.Fatalf("decode pull_request trigger fields: %v", err)
	}
	if got := sortedDeepsecKeys(triggerFields); !slices.Equal(got, []string{"branches", "types"}) {
		t.Fatalf("pull_request trigger fields = %v, want only branches and types", got)
	}
	var trigger deepsecPullRequestTrigger
	if err := pullRequestNode.Decode(&trigger); err != nil {
		t.Fatalf("decode pull_request trigger: %v", err)
	}
	if !equalDeepsecStringSet(trigger.Branches, []string{"main"}) {
		t.Fatalf("pull_request branches = %v, want [main]", trigger.Branches)
	}
	if !equalDeepsecStringSet(trigger.Types, []string{"opened", "synchronize", "reopened", "labeled"}) {
		t.Fatalf("pull_request types = %v, want opened, synchronize, reopened, and labeled", trigger.Types)
	}

	expression := normalizeDeepsecExpression(workflow.Jobs.CheckKey.If)
	fixtures := []struct {
		name  string
		event deepsecAdmissionEvent
		want  bool
	}{
		{
			name: "adding full-ci admits an unchanged same-repository head",
			event: deepsecAdmissionEvent{
				HeadRepository: "vbonnet/dear-agent",
				Repository:     "vbonnet/dear-agent",
				Action:         "labeled",
				AddedLabel:     "full-ci",
				Labels:         []string{"full-ci"},
			},
			want: true,
		},
		{
			name: "synchronize admits a currently labeled same-repository head",
			event: deepsecAdmissionEvent{
				HeadRepository: "vbonnet/dear-agent",
				Repository:     "vbonnet/dear-agent",
				Action:         "synchronize",
				Labels:         []string{"full-ci"},
			},
			want: true,
		},
		{
			name: "opened without full-ci is rejected",
			event: deepsecAdmissionEvent{
				HeadRepository: "vbonnet/dear-agent",
				Repository:     "vbonnet/dear-agent",
				Action:         "opened",
			},
		},
		{
			name: "fork with full-ci is rejected",
			event: deepsecAdmissionEvent{
				HeadRepository: "contributor/dear-agent",
				Repository:     "vbonnet/dear-agent",
				Action:         "synchronize",
				Labels:         []string{"full-ci"},
			},
		},
		{
			name: "fork adding full-ci is rejected",
			event: deepsecAdmissionEvent{
				HeadRepository: "contributor/dear-agent",
				Repository:     "vbonnet/dear-agent",
				Action:         "labeled",
				AddedLabel:     "full-ci",
				Labels:         []string{"full-ci"},
			},
		},
		{
			name: "unrelated added label does not duplicate a paid scan",
			event: deepsecAdmissionEvent{
				HeadRepository: "vbonnet/dear-agent",
				Repository:     "vbonnet/dear-agent",
				Action:         "labeled",
				AddedLabel:     "documentation",
				Labels:         []string{"full-ci", "documentation"},
			},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			got, err := evaluateDeepsecAdmission(expression, fixture.event)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.want {
				t.Fatalf("admission = %t, want %t for expression %q", got, fixture.want, expression)
			}
		})
	}

	if workflow.Jobs.Scan.Needs != "check-key" {
		t.Fatalf("scan.needs = %q, want check-key", workflow.Jobs.Scan.Needs)
	}
	if got := normalizeDeepsecExpression(workflow.Jobs.Scan.If); got != "needs.check-key.outputs.has_key == 'true'" {
		t.Fatalf("scan.if = %q, want credential output gate", got)
	}

	checkoutCount := 0
	for _, step := range workflow.Jobs.Scan.Steps {
		if !strings.HasPrefix(step.Uses, "actions/checkout@") {
			continue
		}
		checkoutCount++
		if step.With.Ref != "${{ github.event.pull_request.head.sha }}" {
			t.Errorf("checkout ref = %q, want exact pull-request head", step.With.Ref)
		}
		if step.With.FetchDepth == nil {
			t.Error("checkout fetch-depth is absent, want explicit 0")
		} else if *step.With.FetchDepth != 0 {
			t.Errorf("checkout fetch-depth = %d, want 0", *step.With.FetchDepth)
		}
	}
	if checkoutCount != 1 {
		t.Fatalf("checkout steps = %d, want exactly 1", checkoutCount)
	}

	for _, staleClaim := range []string{"merge_group", "workflow_dispatch", "success_if_findings"} {
		if strings.Contains(string(source), staleClaim) {
			t.Errorf("workflow retains unsupported claim %q", staleClaim)
		}
	}
}

func TestDeepsecAdmissionExpressionFailsClosed(t *testing.T) {
	t.Parallel()

	event := deepsecAdmissionEvent{
		HeadRepository: "vbonnet/dear-agent",
		Repository:     "vbonnet/dear-agent",
		Action:         "labeled",
		AddedLabel:     "full-ci",
		Labels:         []string{"full-ci"},
	}
	for _, expression := range []string{
		deepsecSameRepositoryAtom + " && github.actor == 'trusted'",
		"(" + deepsecSameRepositoryAtom,
		deepsecSameRepositoryAtom + " &&",
		"contains(github.event.pull_request.labels.*.name 'full-ci')",
	} {
		if got, err := evaluateDeepsecAdmission(expression, event); err == nil {
			t.Errorf("evaluateDeepsecAdmission(%q) = %t, want error", expression, got)
		}
	}
}

func evaluateDeepsecAdmission(expression string, event deepsecAdmissionEvent) (bool, error) {
	expression = normalizeDeepsecExpression(expression)
	if expression == "" {
		return false, fmt.Errorf("empty Deepsec admission expression")
	}

	var err error
	expression, err = stripDeepsecOuterParentheses(expression)
	if err != nil {
		return false, err
	}

	parts, err := splitDeepsecTopLevel(expression, "||")
	if err != nil {
		return false, err
	}
	if len(parts) > 1 {
		result := false
		for _, part := range parts {
			value, evalErr := evaluateDeepsecAdmission(part, event)
			if evalErr != nil {
				return false, evalErr
			}
			result = result || value
		}
		return result, nil
	}

	parts, err = splitDeepsecTopLevel(expression, "&&")
	if err != nil {
		return false, err
	}
	if len(parts) > 1 {
		result := true
		for _, part := range parts {
			value, evalErr := evaluateDeepsecAdmission(part, event)
			if evalErr != nil {
				return false, evalErr
			}
			result = result && value
		}
		return result, nil
	}

	switch expression {
	case deepsecSameRepositoryAtom:
		return event.HeadRepository == event.Repository, nil
	case deepsecFullCILabelAtom:
		return slices.Contains(event.Labels, "full-ci"), nil
	case deepsecNotLabeledAtom:
		return event.Action != "labeled", nil
	case deepsecAddedFullCIAtom:
		return event.AddedLabel == "full-ci", nil
	default:
		return false, fmt.Errorf("unsupported Deepsec admission atom %q", expression)
	}
}

func normalizeDeepsecExpression(expression string) string {
	return strings.Join(strings.Fields(expression), " ")
}

func stripDeepsecOuterParentheses(expression string) (string, error) {
	for strings.HasPrefix(expression, "(") {
		encloses, err := deepsecOuterParenthesesEncloseWholeExpression(expression)
		if err != nil {
			return "", err
		}
		if !encloses {
			break
		}
		expression = strings.TrimSpace(expression[1 : len(expression)-1])
		if expression == "" {
			return "", fmt.Errorf("empty parenthesized Deepsec expression")
		}
	}
	return expression, nil
}

func deepsecOuterParenthesesEncloseWholeExpression(expression string) (bool, error) {
	depth := 0
	var quote byte
	for i := 0; i < len(expression); i++ {
		char := expression[i]
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false, fmt.Errorf("unbalanced closing parenthesis in %q", expression)
			}
			if depth == 0 {
				return i == len(expression)-1, nil
			}
		}
	}
	if quote != 0 {
		return false, fmt.Errorf("unterminated quote in %q", expression)
	}
	return false, fmt.Errorf("unbalanced opening parenthesis in %q", expression)
}

func splitDeepsecTopLevel(expression, operator string) ([]string, error) {
	parts := make([]string, 0, 2)
	start := 0
	depth := 0
	var quote byte
	for i := 0; i < len(expression); i++ {
		char := expression[i]
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced closing parenthesis in %q", expression)
			}
		}
		if depth == 0 && strings.HasPrefix(expression[i:], operator) {
			part := strings.TrimSpace(expression[start:i])
			if part == "" {
				return nil, fmt.Errorf("empty operand before %s in %q", operator, expression)
			}
			parts = append(parts, part)
			i += len(operator) - 1
			start = i + 1
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in %q", expression)
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced parentheses in %q", expression)
	}
	last := strings.TrimSpace(expression[start:])
	if last == "" {
		return nil, fmt.Errorf("empty operand after %s in %q", operator, expression)
	}
	return append(parts, last), nil
}

func equalDeepsecStringSet(got, want []string) bool {
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	return slices.Equal(got, want)
}

func sortedDeepsecKeys(values map[string]yaml.Node) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
