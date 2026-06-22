package override

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// llmJudgeModel is the small classifier model used to vet P0/P1 override
// reasons. It is deliberately a *separate, cheap* model — distinct from the
// agent requesting the bypass — so the party that wants the override is never
// the party that judges it.
const llmJudgeModel = "claude-haiku-4-5-20251001"

// LLMJudge classifies an override request via a language-model call. It is the
// seam for the model layer: ClaudeHaikuJudge wraps the real Anthropic call,
// FakeJudge wraps a canned response for deterministic tests. The shape mirrors
// the prompt contract — (command, reason, context) → allow/deny+explanation.
type LLMJudge interface {
	// Classify renders a verdict on whether `reason` is a legitimate, specific
	// justification for the bypass described by `cmd` in `context`. A non-nil
	// error means the model could not be consulted; callers degrade safely
	// (fall through to the deterministic floor) rather than block.
	Classify(ctx context.Context, cmd, reason, context string) (JudgeVerdict, error)
}

// ClaudeHaikuJudge is a [Judge] that layers an [LLMJudge] classifier on top of
// the deterministic [DefaultJudge] floor. Order of operations on Evaluate:
//
//  1. Run the deterministic floor. If it denies (empty/junk/too-short reason),
//     deny immediately — never spend a model call, and never let the model
//     allow what the floor already rejected.
//  2. Otherwise consult the model. If it is configured and answers, its verdict
//     wins (this is how P0 vetting catches plausible-but-empty reasons the floor
//     waves through).
//  3. If the model is unconfigured (no API key) or errors, fall through to the
//     floor's verdict. The override is never blocked merely because the model
//     was unreachable — degrade safely, fail open to the deterministic decision.
type ClaudeHaikuJudge struct {
	llm      LLMJudge // model classifier; nil ⇒ deterministic floor only
	fallback Judge    // floor + degradation target; nil ⇒ DefaultJudge{}
}

// NewClaudeHaikuJudge builds a ClaudeHaikuJudge backed by the live Anthropic
// API when ANTHROPIC_API_KEY is set. When the key is absent it returns a judge
// whose model layer is nil — i.e. it behaves exactly like DefaultJudge, so CI
// and key-less environments are never blocked by a missing credential.
func NewClaudeHaikuJudge() *ClaudeHaikuJudge {
	return &ClaudeHaikuJudge{llm: newAnthropicClassifierFromEnv(), fallback: DefaultJudge{}}
}

// NewClaudeHaikuJudgeWith builds a ClaudeHaikuJudge around an explicit
// classifier (e.g. a [FakeJudge] in tests, or a pre-built client). A nil llm
// degrades to the deterministic floor.
func NewClaudeHaikuJudgeWith(llm LLMJudge) *ClaudeHaikuJudge {
	return &ClaudeHaikuJudge{llm: llm, fallback: DefaultJudge{}}
}

// Name implements Judge.
func (j *ClaudeHaikuJudge) Name() string {
	if j.llm == nil {
		// No model layer wired (no API key): the deterministic floor is what
		// actually decided, so the audit log should say so.
		return "default"
	}
	return "claude-haiku"
}

// Evaluate implements Judge: deterministic floor first, then the model layer,
// degrading safely to the floor's verdict on any model trouble.
func (j *ClaudeHaikuJudge) Evaluate(ctx context.Context, req JudgeRequest) (JudgeVerdict, error) {
	fb := j.fallback
	if fb == nil {
		fb = DefaultJudge{}
	}

	base, err := fb.Evaluate(ctx, req)
	if err != nil {
		// The floor itself failed to decide; propagate so Require fails closed.
		return base, err
	}
	// Floor denied → deny. The model never gets a vote to loosen the floor.
	if !base.Allow {
		return base, nil
	}
	// No model configured (key absent) → the floor's allow stands.
	if j.llm == nil {
		return base, nil
	}

	cmd := strings.TrimSpace(req.Tool + " " + req.Flag)
	verdict, lerr := j.llm.Classify(ctx, cmd, req.Reason, req.Gate)
	if lerr != nil {
		// Degrade safely: a model that cannot be reached must not block a bypass
		// that already cleared the deterministic floor. Returning the floor's
		// verdict (not the error) is the whole point — fail open to the
		// deterministic decision, never harder than it.
		return base, nil //nolint:nilerr // intentional safe degradation to the floor verdict
	}
	return verdict, nil
}

// FakeJudge is a deterministic [LLMJudge] for tests. It returns a fixed verdict
// (or a fixed error), and records the last call so tests can assert the mapped
// (cmd, reason, context) tuple was passed through correctly.
type FakeJudge struct {
	Verdict JudgeVerdict
	Err     error

	// Calls captures every Classify invocation, in order.
	Calls []FakeJudgeCall
}

// FakeJudgeCall is one recorded Classify invocation.
type FakeJudgeCall struct {
	Cmd     string
	Reason  string
	Context string
}

// Classify implements LLMJudge with the canned verdict/error.
func (f *FakeJudge) Classify(_ context.Context, cmd, reason, context string) (JudgeVerdict, error) {
	f.Calls = append(f.Calls, FakeJudgeCall{Cmd: cmd, Reason: reason, Context: context})
	if f.Err != nil {
		return JudgeVerdict{}, f.Err
	}
	return f.Verdict, nil
}

// anthropicClassifier is the live LLMJudge: a single, non-streaming messages
// call to claude-haiku that returns a JSON allow/deny verdict.
type anthropicClassifier struct {
	client anthropic.Client
	model  string
}

// newAnthropicClassifierFromEnv returns a live classifier when ANTHROPIC_API_KEY
// is set, or nil when it is absent (signalling "no model layer").
func newAnthropicClassifierFromEnv() LLMJudge {
	key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if key == "" {
		return nil
	}
	return &anthropicClassifier{
		client: anthropic.NewClient(option.WithAPIKey(key)),
		model:  llmJudgeModel,
	}
}

// llmVerdictJSON is the JSON contract the model is asked to emit.
type llmVerdictJSON struct {
	Allow       bool   `json:"allow"`
	Explanation string `json:"explanation"`
}

// classifyPrompt builds the single-turn prompt sent to the judge model.
func classifyPrompt(cmd, reason, context string) string {
	return fmt.Sprintf("Given command='%s', reason='%s', context='%s': "+
		"Is this a legitimate, specific, non-empty reason for overriding a safety check? "+
		"Answer JSON: {\"allow\": bool, \"explanation\": string}",
		cmd, reason, context)
}

// Classify implements LLMJudge with a live Anthropic messages call.
func (c *anthropicClassifier) Classify(ctx context.Context, cmd, reason, context string) (JudgeVerdict, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(classifyPrompt(cmd, reason, context))),
		},
	})
	if err != nil {
		return JudgeVerdict{}, fmt.Errorf("claude-haiku judge call failed: %w", err)
	}
	if len(resp.Content) == 0 {
		return JudgeVerdict{}, fmt.Errorf("claude-haiku judge returned no content")
	}

	text := strings.TrimSpace(resp.Content[0].AsText().Text)
	parsed, perr := parseVerdictJSON(text)
	if perr != nil {
		return JudgeVerdict{}, perr
	}
	return JudgeVerdict(parsed), nil
}

// parseVerdictJSON extracts the {allow, explanation} object from a model reply,
// tolerating prose or code fences around the JSON object.
func parseVerdictJSON(text string) (llmVerdictJSON, error) {
	var v llmVerdictJSON
	// Fast path: the whole reply is the JSON object.
	if err := json.Unmarshal([]byte(text), &v); err == nil {
		return v, nil
	}
	// Slow path: carve out the first {...} span and parse that.
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return v, fmt.Errorf("claude-haiku judge reply was not JSON: %q", text)
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &v); err != nil {
		return v, fmt.Errorf("claude-haiku judge reply was not valid JSON: %w", err)
	}
	return v, nil
}

// Compile-time checks.
var (
	_ Judge    = (*ClaudeHaikuJudge)(nil)
	_ LLMJudge = (*FakeJudge)(nil)
	_ LLMJudge = (*anthropicClassifier)(nil)
)
