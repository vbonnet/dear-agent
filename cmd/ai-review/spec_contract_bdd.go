package main

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	gherkin "github.com/cucumber/gherkin/go/v26"
	messages "github.com/cucumber/messages/go/v21"
)

const (
	maxFeatureScenarios          = 512
	maxFeatureExecutableCases    = 4096
	maxFeatureExecutableSteps    = 16 * 1024
	maxFeatureInterpolationWork  = 64 * 1024 * 1024
	maxFeatureInterpolationBytes = 16 * 1024 * 1024
	maxFeatureSteps              = 4096
)

type bddFeatureEvidence struct {
	Path      string                `json:"path"`
	Language  string                `json:"language"`
	Name      string                `json:"name"`
	Tags      []string              `json:"tags"`
	Scenarios []bddScenarioEvidence `json:"scenarios"`
	Content   string                `json:"content"`
}

type bddScenarioEvidence struct {
	Rule    string   `json:"rule"`
	Keyword string   `json:"keyword"`
	Name    string   `json:"name"`
	Tags    []string `json:"tags"`
	Steps   []string `json:"steps"`
}

//nolint:gocyclo // Gherkin AST validation keeps all authenticated evidence bounds in one ordered pass.
func parseBDDFeature(path string, blob []byte) (bddFeatureEvidence, error) {
	ids := &messages.Incrementing{}
	builder := gherkin.NewAstBuilder(ids.NewId)
	parser := gherkin.NewParser(builder)
	parser.StopAtFirstError(true)
	matcher := gherkin.NewLanguageMatcher(gherkin.DialectsBuiltin(), gherkin.DefaultDialect)
	if err := parser.Parse(gherkin.NewScanner(bytes.NewReader(blob)), matcher); err != nil {
		return bddFeatureEvidence{}, errors.New("invalid Gherkin")
	}
	document := builder.GetGherkinDocument()
	if document == nil || document.Feature == nil || strings.TrimSpace(document.Feature.Name) == "" {
		return bddFeatureEvidence{}, errors.New("missing named Gherkin feature")
	}
	dialect := gherkin.DialectsBuiltin().GetDialect(document.Feature.Language)
	if dialect == nil {
		return bddFeatureEvidence{}, errors.New("unsupported Gherkin dialect")
	}
	evidence := bddFeatureEvidence{
		Path:      path,
		Language:  document.Feature.Language,
		Name:      strings.TrimSpace(document.Feature.Name),
		Tags:      bddTags(document.Feature.Tags),
		Scenarios: []bddScenarioEvidence{},
		Content:   string(blob),
	}
	stepCount := 0
	executableCases := 0
	executionBudget := gherkinExecutionBudget{}
	scenarioIDs := make(map[string]bool)
	appendScenario := func(rule string, scenario *messages.Scenario, background []*messages.Step) error {
		if scenario == nil || scenario.Id == "" || strings.TrimSpace(scenario.Name) == "" {
			return errors.New("unnamed Gherkin scenario")
		}
		if scenarioIDs[scenario.Id] {
			return errors.New("duplicate Gherkin scenario identity")
		}
		steps := make([]string, 0, len(scenario.Steps))
		for _, step := range scenario.Steps {
			if step == nil || strings.TrimSpace(step.Text) == "" {
				return errors.New("empty Gherkin step")
			}
			steps = append(steps, strings.TrimSpace(step.Keyword)+" "+strings.TrimSpace(step.Text))
			stepCount++
			if stepCount > maxFeatureSteps {
				return errors.New("too many Gherkin steps")
			}
		}
		if len(steps) == 0 {
			return errors.New("gherkin scenario has no runnable steps")
		}
		executions, err := scenarioExecutionCount(scenario, dialect, background, &executionBudget)
		if err != nil {
			return err
		}
		if executableCases > maxFeatureExecutableCases-executions {
			return errors.New("too many executable Gherkin cases")
		}
		executableCases += executions
		scenarioIDs[scenario.Id] = true
		evidence.Scenarios = append(evidence.Scenarios, bddScenarioEvidence{
			Rule:    strings.TrimSpace(rule),
			Keyword: strings.TrimSpace(scenario.Keyword),
			Name:    strings.TrimSpace(scenario.Name),
			Tags:    bddTags(scenario.Tags),
			Steps:   steps,
		})
		if len(evidence.Scenarios) > maxFeatureScenarios {
			return errors.New("too many Gherkin scenarios")
		}
		return nil
	}
	featureBackgroundSteps := []*messages.Step{}
	for _, child := range document.Feature.Children {
		if child == nil {
			continue
		}
		if child.Background != nil {
			steps, err := validateGherkinBackground(child.Background, &stepCount)
			if err != nil {
				return bddFeatureEvidence{}, err
			}
			featureBackgroundSteps = append(featureBackgroundSteps, steps...)
		}
		if child.Scenario != nil {
			if err := appendScenario("", child.Scenario, featureBackgroundSteps); err != nil {
				return bddFeatureEvidence{}, err
			}
		}
		if child.Rule == nil {
			continue
		}
		ruleBackgroundSteps := append([]*messages.Step(nil), featureBackgroundSteps...)
		for _, ruleChild := range child.Rule.Children {
			if ruleChild == nil {
				continue
			}
			if ruleChild.Background != nil {
				steps, err := validateGherkinBackground(ruleChild.Background, &stepCount)
				if err != nil {
					return bddFeatureEvidence{}, err
				}
				ruleBackgroundSteps = append(ruleBackgroundSteps, steps...)
			}
			if ruleChild.Scenario == nil {
				continue
			}
			if err := appendScenario(child.Rule.Name, ruleChild.Scenario, ruleBackgroundSteps); err != nil {
				return bddFeatureEvidence{}, err
			}
		}
	}
	if len(evidence.Scenarios) == 0 || stepCount == 0 {
		return bddFeatureEvidence{}, errors.New("gherkin feature has no runnable scenario steps")
	}
	return evidence, nil
}

func validateGherkinBackground(background *messages.Background, stepCount *int) ([]*messages.Step, error) {
	if background == nil || len(background.Steps) == 0 {
		return nil, errors.New("gherkin background has no runnable steps")
	}
	for _, step := range background.Steps {
		if step == nil || strings.TrimSpace(step.Text) == "" {
			return nil, errors.New("empty Gherkin background step")
		}
		(*stepCount)++
		if *stepCount > maxFeatureSteps {
			return nil, errors.New("too many Gherkin steps")
		}
	}
	return append([]*messages.Step(nil), background.Steps...), nil
}

type gherkinExecutionBudget struct {
	steps           int
	workBytes       int
	allocationBytes int
}

// scenarioExecutionCount validates executable cases directly on the bounded
// AST. It deliberately does not materialize Gherkin pickles: explicit bounded
// Examples substitution supplies the evidence this gate needs without letting
// untrusted outline rows amplify step arguments in memory.
func scenarioExecutionCount(scenario *messages.Scenario, dialect *gherkin.Dialect, backgroundSteps []*messages.Step, budget *gherkinExecutionBudget) (int, error) {
	if budget == nil {
		return 0, errors.New("missing Gherkin execution budget")
	}
	if !isGherkinKeyword(scenario.Keyword, dialect.ScenarioOutlineKeywords()) {
		if len(scenario.Examples) != 0 {
			return 0, errors.New("non-outline Gherkin scenario has examples")
		}
		if err := addGherkinBudget(&budget.steps, len(backgroundSteps)+len(scenario.Steps), maxFeatureExecutableSteps, "too many executable Gherkin steps"); err != nil {
			return 0, err
		}
		return 1, nil
	}
	if len(scenario.Examples) == 0 {
		return 0, errors.New("gherkin scenario outline has no executable examples")
	}
	rowCount := 0
	for _, examples := range scenario.Examples {
		rows, err := executableExampleRows(examples)
		if err != nil {
			return 0, err
		}
		if rows == 0 {
			continue
		}
		if rowCount > maxFeatureExecutableCases-rows {
			return 0, errors.New("too many executable Gherkin cases")
		}
		rowCount += rows
		needles, err := gherkinExampleNeedles(examples.TableHeader.Cells)
		if err != nil {
			return 0, err
		}
		for _, row := range examples.TableBody {
			if err := validateOutlineExecution(backgroundSteps, scenario.Steps, needles, row.Cells, budget); err != nil {
				return 0, err
			}
		}
	}
	if rowCount == 0 {
		return 0, errors.New("gherkin scenario outline has no executable examples")
	}
	return rowCount, nil
}

func gherkinExampleNeedles(variables []*messages.TableCell) ([]string, error) {
	needles := make([]string, len(variables))
	for i, variable := range variables {
		if variable == nil || len(variable.Value) > maxFeatureInterpolationBytes-2 {
			return nil, errors.New("gherkin scenario outline has an invalid interpolation cell")
		}
		needles[i] = "<" + variable.Value + ">"
	}
	return needles, nil
}

func validateOutlineExecution(backgroundSteps, steps []*messages.Step, needles []string, values []*messages.TableCell, budget *gherkinExecutionBudget) error {
	if len(needles) != len(values) {
		return errors.New("gherkin scenario outline has a malformed examples row")
	}
	if budget == nil {
		return errors.New("missing Gherkin execution budget")
	}
	if err := addGherkinBudget(&budget.steps, len(backgroundSteps)+len(steps), maxFeatureExecutableSteps, "too many executable Gherkin steps"); err != nil {
		return err
	}
	validateSteps := func(steps []*messages.Step) error {
		for _, step := range steps {
			if step == nil {
				return errors.New("empty Gherkin background step")
			}
			text, err := boundedGherkinSubstitution(step.Text, needles, values, budget)
			if err != nil {
				return err
			}
			if strings.TrimSpace(text) == "" {
				return errors.New("gherkin scenario outline has an empty executable step")
			}
		}
		return nil
	}
	if err := validateSteps(backgroundSteps); err != nil {
		return err
	}
	if err := validateSteps(steps); err != nil {
		return err
	}
	return nil
}

func boundedGherkinSubstitution(text string, needles []string, values []*messages.TableCell, budget *gherkinExecutionBudget) (string, error) {
	if budget == nil || len(needles) != len(values) {
		return "", errors.New("gherkin scenario outline has malformed interpolation evidence")
	}
	current := text
	for i, needle := range needles {
		if needle == "" || values[i] == nil {
			return "", errors.New("gherkin scenario outline has an invalid interpolation cell")
		}
		if err := addGherkinBudget(&budget.workBytes, len(current), maxFeatureInterpolationWork, "gherkin interpolation exceeds the review work limit"); err != nil {
			return "", err
		}
		matches := strings.Count(current, needle)
		if matches == 0 {
			continue
		}
		nextBytes, err := gherkinReplacementBytes(len(current), len(needle), len(values[i].Value), matches)
		if err != nil {
			return "", err
		}
		if err := addGherkinBudget(&budget.workBytes, len(current), maxFeatureInterpolationWork, "gherkin interpolation exceeds the review work limit"); err != nil {
			return "", err
		}
		if err := addGherkinBudget(&budget.allocationBytes, nextBytes, maxFeatureInterpolationBytes, "gherkin interpolation exceeds the review allocation limit"); err != nil {
			return "", err
		}
		current = strings.ReplaceAll(current, needle, values[i].Value)
		if len(current) != nextBytes {
			return "", errors.New("invalid Gherkin interpolation length")
		}
	}
	return current, nil
}

func gherkinReplacementBytes(current, needle, replacement, matches int) (int, error) {
	if current < 0 || current > maxFeatureInterpolationBytes || needle < 1 || replacement < 0 || matches < 1 {
		return 0, errors.New("invalid Gherkin interpolation size")
	}
	if replacement >= needle {
		growth := replacement - needle
		if growth > 0 && matches > (maxFeatureInterpolationBytes-current)/growth {
			return 0, errors.New("gherkin interpolation exceeds the review allocation limit")
		}
		return current + matches*growth, nil
	}
	shrink := needle - replacement
	if matches > current/shrink {
		return 0, errors.New("invalid Gherkin interpolation size")
	}
	return current - matches*shrink, nil
}

func addGherkinBudget(total *int, add, limit int, message string) error {
	if total == nil || add < 0 || limit < 0 || add > limit || *total < 0 || *total > limit-add {
		return errors.New(message)
	}
	*total += add
	return nil
}

func executableExampleRows(examples *messages.Examples) (int, error) {
	if examples == nil {
		return 0, errors.New("gherkin scenario outline has a malformed examples block")
	}
	if examples.TableHeader == nil {
		if len(examples.TableBody) == 0 {
			return 0, nil
		}
		return 0, errors.New("gherkin scenario outline has a malformed examples block")
	}
	if len(examples.TableHeader.Cells) == 0 {
		return 0, errors.New("gherkin scenario outline has an invalid examples header")
	}
	headerNames := make(map[string]bool, len(examples.TableHeader.Cells))
	for _, cell := range examples.TableHeader.Cells {
		if cell == nil {
			return 0, errors.New("gherkin scenario outline has an invalid examples header")
		}
		name := strings.TrimSpace(cell.Value)
		if name == "" || headerNames[name] {
			return 0, errors.New("gherkin scenario outline has an invalid examples header")
		}
		headerNames[name] = true
	}
	if len(examples.TableBody) == 0 {
		return 0, nil
	}
	for _, row := range examples.TableBody {
		if !validExampleRow(row, len(examples.TableHeader.Cells)) {
			return 0, errors.New("gherkin scenario outline has a malformed examples row")
		}
	}
	return len(examples.TableBody), nil
}

func validExampleRow(row *messages.TableRow, width int) bool {
	if row == nil || len(row.Cells) != width {
		return false
	}
	return !slices.Contains(row.Cells, nil)
}

func isGherkinKeyword(keyword string, candidates []string) bool {
	keyword = strings.TrimSpace(keyword)
	return slices.ContainsFunc(candidates, func(candidate string) bool {
		return strings.TrimSpace(candidate) == keyword
	})
}

func bddTags(tags []*messages.Tag) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != nil && strings.TrimSpace(tag.Name) != "" {
			result = append(result, strings.TrimSpace(tag.Name))
		}
	}
	return result
}
