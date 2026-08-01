package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/internal/markdownvisible"
)

type markdownVisibilityStateKey struct{}

type markdownVisibilityState struct {
	document string
	lines    []markdownvisible.Line
}

// RegisterMarkdownVisibilitySteps registers provider-neutral Markdown
// visibility behavior used by SPEC governance tools.
func RegisterMarkdownVisibilitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, markdownVisibilityStateKey{}, &markdownVisibilityState{}), nil
	})
	ctx.Step(`^a SPEC document containing visible requirements and hidden CommonMark examples$`, aSPECWithVisibleRequirementsAndHiddenExamples)
	ctx.Step(`^AGM selects normative Markdown prose$`, agmSelectsNormativeMarkdownProse)
	ctx.Step(`^hidden examples should be excluded without changing visible source line alignment$`, hiddenExamplesAreExcludedWithoutShiftingVisibleProse)
}

func aSPECWithVisibleRequirementsAndHiddenExamples(ctx context.Context) error {
	state, err := getMarkdownVisibilityState(ctx)
	if err != nil {
		return err
	}
	state.document = strings.Join([]string{
		"# Contract",
		"<!-- **HIDDEN-01** When copied, the system shall ignore it. -->",
		"> > ```markdown",
		"> > **HIDDEN-02** When copied, the system shall ignore it.",
		"> > ```",
		"`<!--` remains code and visible prose remains aligned.",
		"**VISIBLE-01** When checked, the system shall preserve normative prose.",
	}, "\n")
	return nil
}

func agmSelectsNormativeMarkdownProse(ctx context.Context) error {
	state, err := getMarkdownVisibilityState(ctx)
	if err != nil {
		return err
	}
	state.lines = markdownvisible.Lines([]byte(state.document))
	return nil
}

func hiddenExamplesAreExcludedWithoutShiftingVisibleProse(ctx context.Context) error {
	state, err := getMarkdownVisibilityState(ctx)
	if err != nil {
		return err
	}
	if len(state.lines) != strings.Count(state.document, "\n")+1 {
		return fmt.Errorf("classified line count = %d, want source line count", len(state.lines))
	}
	visible := make([]string, 0, len(state.lines))
	for _, line := range state.lines {
		if line.Visible {
			visible = append(visible, line.Text)
		}
	}
	text := strings.Join(visible, "\n")
	if strings.Contains(text, "HIDDEN-01") || strings.Contains(text, "HIDDEN-02") || strings.Contains(text, "```") {
		return fmt.Errorf("hidden CommonMark example remained normative: %q", text)
	}
	for _, index := range []int{2, 3, 4} {
		if state.lines[index].Visible || strings.TrimSpace(state.lines[index].Text) != "" {
			return fmt.Errorf("container-nested fence line %d was not completely excluded: %#v", index, state.lines[index])
		}
	}
	if !strings.Contains(text, "<!--") || !strings.Contains(text, "VISIBLE-01") || state.lines[6].Text != "**VISIBLE-01** When checked, the system shall preserve normative prose." {
		return fmt.Errorf("visible prose or line alignment was lost: %#v", state.lines)
	}
	return nil
}

func getMarkdownVisibilityState(ctx context.Context) (*markdownVisibilityState, error) {
	state, ok := ctx.Value(markdownVisibilityStateKey{}).(*markdownVisibilityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("Markdown visibility scenario state is unavailable")
	}
	return state, nil
}
