package escalation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// defaultAdjudicatorModel is the model used to score answered escalations. It is
// a *separate* classifier from any agent in the escalation chain — the party
// that gave the answer is never the party that judges it. Haiku is the default
// for cost (a backfill may score many events); override with
// AGM_ESCALATION_ADJUDICATOR_MODEL for a stronger pass.
const defaultAdjudicatorModel = "claude-haiku-4-5-20251001"

// adjudicatorModelEnv names the env var that overrides the adjudicator model.
const adjudicatorModelEnv = "AGM_ESCALATION_ADJUDICATOR_MODEL"

// LLMAdjudicator is the model-call seam: anthropicAdjudicator wraps the real
// Anthropic call, FakeAdjudicator wraps a canned response for deterministic
// tests. The shape mirrors the prompt contract — request → outcome verdict.
type LLMAdjudicator interface {
	// Score renders a verdict on the answer in req. A non-nil error means the
	// model could not be consulted; callers degrade to the deterministic floor.
	Score(ctx context.Context, req AdjudicationRequest) (Adjudication, error)
}

// ClaudeAdjudicator layers an [LLMAdjudicator] on top of the deterministic
// [DefaultAdjudicator] floor. Order of operations on Adjudicate:
//
//  1. If the deterministic floor renders a verdict (e.g. a non-answer is
//     decidably incorrect), use it — never spend a model call on a case the
//     floor already settled.
//  2. Otherwise consult the model. Its verdict wins.
//  3. If the model is unconfigured (no API key) or errors, fall through to the
//     floor's verdict — which for a substantive answer is the empty "could not
//     assess" result, so the backfill simply leaves the event for a later pass.
//     A model outage never mislabels an answer.
type ClaudeAdjudicator struct {
	llm      LLMAdjudicator // model layer; nil ⇒ deterministic floor only
	fallback Adjudicator    // floor + degradation target; nil ⇒ DefaultAdjudicator{}
}

// NewClaudeAdjudicator builds a ClaudeAdjudicator backed by the live Anthropic
// API when ANTHROPIC_API_KEY is set. When the key is absent its model layer is
// nil — it behaves exactly like DefaultAdjudicator, so CI and key-less
// environments are never blocked by a missing credential.
func NewClaudeAdjudicator() *ClaudeAdjudicator {
	return &ClaudeAdjudicator{llm: newAnthropicAdjudicatorFromEnv(), fallback: DefaultAdjudicator{}}
}

// NewClaudeAdjudicatorWith builds a ClaudeAdjudicator around an explicit model
// layer (e.g. a [FakeAdjudicator] in tests). A nil llm degrades to the floor.
func NewClaudeAdjudicatorWith(llm LLMAdjudicator) *ClaudeAdjudicator {
	return &ClaudeAdjudicator{llm: llm, fallback: DefaultAdjudicator{}}
}

// Name implements Adjudicator.
func (a *ClaudeAdjudicator) Name() string {
	if a.llm == nil {
		// No model layer wired (no API key): the deterministic floor is what
		// actually decided, so the log should say so.
		return "default"
	}
	return "claude"
}

// Adjudicate implements Adjudicator: floor first, then model, degrading to the
// floor's verdict on any model trouble.
func (a *ClaudeAdjudicator) Adjudicate(ctx context.Context, req AdjudicationRequest) (Adjudication, error) {
	fb := a.fallback
	if fb == nil {
		fb = DefaultAdjudicator{}
	}

	base, err := fb.Adjudicate(ctx, req)
	if err != nil {
		return base, err
	}
	// Floor already rendered a verdict (e.g. a non-answer) → keep it; no model call.
	if base.Outcome != "" {
		return base, nil
	}
	// No model configured → the floor's "could not assess" stands.
	if a.llm == nil {
		return base, nil
	}

	verdict, lerr := a.llm.Score(ctx, req)
	if lerr != nil {
		// Degrade safely: a model that cannot be reached must not invent a
		// verdict. Return the floor's (empty) result so the backfill leaves the
		// event for a later pass.
		return base, nil //nolint:nilerr // intentional safe degradation to the floor verdict
	}
	return verdict, nil
}

// FakeAdjudicator is a deterministic [LLMAdjudicator] for tests. It returns a
// fixed verdict (or error) and records every call so tests can assert the mapped
// request was passed through.
type FakeAdjudicator struct {
	Verdict Adjudication
	Err     error

	Calls []AdjudicationRequest
}

// Score implements LLMAdjudicator with the canned verdict/error.
func (f *FakeAdjudicator) Score(_ context.Context, req AdjudicationRequest) (Adjudication, error) {
	f.Calls = append(f.Calls, req)
	if f.Err != nil {
		return Adjudication{}, f.Err
	}
	return f.Verdict, nil
}

// anthropicAdjudicator is the live LLMAdjudicator: a single non-streaming
// messages call that returns a JSON outcome verdict.
type anthropicAdjudicator struct {
	client anthropic.Client
	model  string
}

// newAnthropicAdjudicatorFromEnv returns a live adjudicator when
// ANTHROPIC_API_KEY is set, or nil when it is absent ("no model layer").
func newAnthropicAdjudicatorFromEnv() LLMAdjudicator {
	key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if key == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv(adjudicatorModelEnv))
	if model == "" {
		model = defaultAdjudicatorModel
	}
	return &anthropicAdjudicator{
		client: anthropic.NewClient(option.WithAPIKey(key)),
		model:  model,
	}
}

// adjudicationJSON is the JSON contract the model is asked to emit.
type adjudicationJSON struct {
	Outcome      string `json:"outcome"`
	Misalignment string `json:"misalignment"`
	Reason       string `json:"reason"`
}

// adjudicatePrompt builds the single-turn prompt sent to the adjudicator model.
func adjudicatePrompt(req AdjudicationRequest) string {
	return fmt.Sprintf(
		"You are an independent reviewer auditing whether an answer given to an "+
			"escalated agent question was good. You did NOT write the answer.\n\n"+
			"kind: %s\n"+
			"question: %s\n"+
			"context: %s\n"+
			"answer (from role %q): %s\n\n"+
			"Judge the answer on two axes:\n"+
			"- correctness: is it accurate and does it address the question?\n"+
			"- alignment: does it steer the agent toward the right work, or pull it "+
			"off-course even if technically responsive?\n\n"+
			"Reply with ONLY a JSON object: "+
			"{\"outcome\": \"correct|incorrect|misaligned|unclear\", "+
			"\"misalignment\": \"<short note on how it steered wrong, or empty>\", "+
			"\"reason\": \"<one sentence>\"}. "+
			"Use \"unclear\" only if you genuinely lack the context to judge.",
		req.Kind, req.Question, req.Context, req.AnsweredByRole, req.Answer)
}

// Score implements LLMAdjudicator with a live Anthropic messages call.
func (c *anthropicAdjudicator) Score(ctx context.Context, req AdjudicationRequest) (Adjudication, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 512,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(adjudicatePrompt(req))),
		},
	})
	if err != nil {
		return Adjudication{}, fmt.Errorf("escalation adjudicator call failed: %w", err)
	}
	if len(resp.Content) == 0 {
		return Adjudication{}, fmt.Errorf("escalation adjudicator returned no content")
	}

	text := strings.TrimSpace(resp.Content[0].AsText().Text)
	parsed, perr := parseAdjudicationJSON(text)
	if perr != nil {
		return Adjudication{}, perr
	}
	out := Outcome(strings.ToLower(strings.TrimSpace(parsed.Outcome)))
	if !out.Valid() {
		return Adjudication{}, fmt.Errorf("escalation adjudicator returned unrecognised outcome %q", parsed.Outcome)
	}
	return Adjudication{
		Outcome:      out,
		Misalignment: strings.TrimSpace(parsed.Misalignment),
		Reason:       strings.TrimSpace(parsed.Reason),
	}, nil
}

// parseAdjudicationJSON extracts the verdict object from a model reply,
// tolerating prose or code fences around the JSON object.
func parseAdjudicationJSON(text string) (adjudicationJSON, error) {
	var v adjudicationJSON
	if err := json.Unmarshal([]byte(text), &v); err == nil {
		return v, nil
	}
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return v, fmt.Errorf("escalation adjudicator reply was not JSON: %q", text)
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &v); err != nil {
		return v, fmt.Errorf("escalation adjudicator reply was not valid JSON: %w", err)
	}
	return v, nil
}

// Compile-time checks.
var (
	_ Adjudicator    = (*ClaudeAdjudicator)(nil)
	_ LLMAdjudicator = (*FakeAdjudicator)(nil)
	_ LLMAdjudicator = (*anthropicAdjudicator)(nil)
)
