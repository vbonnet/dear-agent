package override

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// realReason is a justification that comfortably clears the deterministic floor,
// so tests can isolate the model layer's behavior from the floor's.
const realReason = "previous migration aborted midway, re-running to finish the half-written target"

func TestClaudeHaikuJudge_ModelDenyOverridesFloorAllow(t *testing.T) {
	fake := &FakeJudge{Verdict: JudgeVerdict{Allow: false, Explanation: "reason is generic boilerplate"}}
	j := NewClaudeHaikuJudgeWith(fake)

	v, err := j.Evaluate(context.Background(), JudgeRequest{
		Tool: "agm send-compact", Flag: "--force",
		Gate: "anti-loop guard", Risk: RiskP0, Reason: realReason,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Allow {
		t.Fatal("model deny must override the floor's allow")
	}
	if v.Explanation != "reason is generic boilerplate" {
		t.Fatalf("verdict should carry the model explanation, got %q", v.Explanation)
	}
	// The classifier must have been consulted, with the mapped tuple.
	if len(fake.Calls) != 1 {
		t.Fatalf("expected exactly one Classify call, got %d", len(fake.Calls))
	}
	call := fake.Calls[0]
	if call.Cmd != "agm send-compact --force" {
		t.Fatalf("cmd should be tool+flag, got %q", call.Cmd)
	}
	if call.Reason != realReason {
		t.Fatalf("reason passed through verbatim, got %q", call.Reason)
	}
	if call.Context != "anti-loop guard" {
		t.Fatalf("context should be the gate, got %q", call.Context)
	}
}

func TestClaudeHaikuJudge_ModelAllow(t *testing.T) {
	fake := &FakeJudge{Verdict: JudgeVerdict{Allow: true, Explanation: "specific and legitimate"}}
	j := NewClaudeHaikuJudgeWith(fake)

	v, err := j.Evaluate(context.Background(), JudgeRequest{
		Tool: "engram migrate", Flag: "--force", Gate: "overwrites target", Risk: RiskP1, Reason: realReason,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.Allow {
		t.Fatalf("model allow should pass through, got deny: %s", v.Explanation)
	}
}

func TestClaudeHaikuJudge_FloorDenyShortCircuitsModel(t *testing.T) {
	fake := &FakeJudge{Verdict: JudgeVerdict{Allow: true, Explanation: "model would allow"}}
	j := NewClaudeHaikuJudgeWith(fake)

	// "x" is junk: the deterministic floor denies it. The model must not get a
	// vote to loosen the floor, and must not even be called.
	v, err := j.Evaluate(context.Background(), JudgeRequest{
		Tool: "t", Flag: "--force", Gate: "g", Risk: RiskP0, Reason: "x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Allow {
		t.Fatal("floor deny must stand even when the model would allow")
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("model must not be consulted after a floor deny, got %d calls", len(fake.Calls))
	}
}

func TestClaudeHaikuJudge_DegradesSafelyOnModelError(t *testing.T) {
	fake := &FakeJudge{Err: errors.New("anthropic API unreachable")}
	j := NewClaudeHaikuJudgeWith(fake)

	// Floor allows; model errors → degrade to the floor's allow, do not block.
	v, err := j.Evaluate(context.Background(), JudgeRequest{
		Tool: "agm send-compact", Flag: "--force", Gate: "anti-loop guard", Risk: RiskP0, Reason: realReason,
	})
	if err != nil {
		t.Fatalf("model error must not surface as an Evaluate error (degrade safely), got %v", err)
	}
	if !v.Allow {
		t.Fatal("a model error must not block a bypass that cleared the floor")
	}
}

func TestClaudeHaikuJudge_NilModelIsDeterministicFloor(t *testing.T) {
	// No model wired (the key-absent case): behaves exactly like DefaultJudge.
	j := NewClaudeHaikuJudgeWith(nil)

	allow, err := j.Evaluate(context.Background(), JudgeRequest{Reason: realReason})
	if err != nil || !allow.Allow {
		t.Fatalf("nil-model judge should allow a real reason, got allow=%v err=%v", allow.Allow, err)
	}
	deny, err := j.Evaluate(context.Background(), JudgeRequest{Reason: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deny.Allow {
		t.Fatal("nil-model judge should still apply the deterministic floor")
	}
	if j.Name() != "default" {
		t.Fatalf("nil-model judge should audit as the deterministic floor, got %q", j.Name())
	}
}

func TestClaudeHaikuJudge_NameReflectsModelPresence(t *testing.T) {
	if got := NewClaudeHaikuJudgeWith(&FakeJudge{}).Name(); got != "llm-override" {
		t.Fatalf("with model, name should be provider-neutral, got %q", got)
	}
}

func TestLLMOverrideJudge_CompatibilityConstructor(t *testing.T) {
	fake := &FakeJudge{Verdict: JudgeVerdict{Allow: true, Explanation: "specific"}}
	judge := NewLLMOverrideJudgeWith(fake)

	if judge.Name() != "llm-override" {
		t.Fatalf("provider-neutral judge name = %q", judge.Name())
	}
	if _, ok := any(NewClaudeHaikuJudgeWith(fake)).(*LLMOverrideJudge); !ok {
		t.Fatal("legacy constructor must remain compatible with LLMOverrideJudge")
	}
}

func TestNewClaudeHaikuJudge_NoAPIKeyDegradesToFloor(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	j := NewClaudeHaikuJudge()
	if j.llm != nil {
		t.Fatal("absent API key must leave the model layer nil")
	}
	// And it still enforces the floor without a network call.
	v, err := j.Evaluate(context.Background(), JudgeRequest{Reason: "force"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Allow {
		t.Fatal("key-less judge should still reject junk reasons via the floor")
	}
}

func TestRequire_UsesLLMJudgeVerdictForP0(t *testing.T) {
	// End-to-end: a Guard with an explicit ClaudeHaikuJudge whose model denies
	// must cause Require to refuse, and the audit must name the LLM judge.
	fake := &FakeJudge{Verdict: JudgeVerdict{Allow: false, Explanation: "reason is too generic for a P0 bypass"}}
	var captured []auditEntry
	g := Guard{
		Tool: "agm send-compact", Flag: "--force", Gate: "anti-loop guard", Risk: RiskP0,
		Judge:     NewClaudeHaikuJudgeWith(fake),
		auditSink: func(e auditEntry) { captured = append(captured, e) },
	}

	err := Require(context.Background(), g, realReason)
	if err == nil {
		t.Fatal("expected refusal when the LLM judge denies")
	}
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("want *DeniedError, got %T", err)
	}
	if !strings.Contains(err.Error(), "too generic") {
		t.Fatalf("error should surface the model explanation, got: %s", err.Error())
	}
	if len(captured) != 1 || captured[0].Allowed {
		t.Fatalf("LLM denial must be audited as not-allowed: %+v", captured)
	}
	if captured[0].Judge != "llm-override" {
		t.Fatalf("audit must record the LLM judge name, got %q", captured[0].Judge)
	}
}

func TestParseVerdictJSON(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantAllow bool
		wantErr   bool
	}{
		{"bare json allow", `{"allow": true, "explanation": "ok"}`, true, false},
		{"bare json deny", `{"allow": false, "explanation": "no"}`, false, false},
		{"fenced json", "```json\n{\"allow\": true, \"explanation\": \"ok\"}\n```", true, false},
		{"prose then json", `Sure! {"allow": false, "explanation": "vague"} hope that helps`, false, false},
		{"no json", "I cannot answer that.", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parseVerdictJSON(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Allow != tc.wantAllow {
				t.Fatalf("allow=%v want %v", v.Allow, tc.wantAllow)
			}
		})
	}
}

func TestClassifyPrompt_IncludesAllFields(t *testing.T) {
	p := classifyPrompt("agm send-compact --force", "stale lock", "anti-loop guard")
	for _, want := range []string{"agm send-compact --force", "stale lock", "anti-loop guard", "allow", "explanation"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q: %s", want, p)
		}
	}
}
