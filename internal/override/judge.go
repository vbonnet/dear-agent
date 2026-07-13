package override

import (
	"context"
	"regexp"
	"strings"
)

// JudgeRequest is the input an override Judge evaluates.
type JudgeRequest struct {
	Tool   string
	Flag   string
	Gate   string
	Risk   Risk
	Reason string
}

// JudgeVerdict is a Judge's decision on whether the reason justifies the bypass.
type JudgeVerdict struct {
	Allow       bool
	Explanation string // why (especially on deny); surfaced to the caller and audit log
}

// Judge gives an independent verdict on whether a reason justifies an override.
//
// The default ([DefaultJudge]) is deterministic: it rejects missing or
// obviously low-effort reasons. [LLMOverrideJudge] can call a small model as a
// separate classifier, distinct from the agent asking for the bypass, to vet
// high-risk overrides. The interface keeps that provider choice replaceable
// and deterministic in tests without changing call sites.
type Judge interface {
	// Name identifies the policy judge in the audit log (e.g. "default",
	// "llm-override") without exposing provider-specific routing.
	Name() string
	// Evaluate returns a verdict. A non-nil error means the judge could not
	// decide; callers fail closed (refuse the bypass) in that case.
	Evaluate(ctx context.Context, req JudgeRequest) (JudgeVerdict, error)
}

// minReasonLen is the shortest a justification may be after trimming. Short
// enough not to be annoying for an honest one-liner ("gemini bot is down"),
// long enough to reject reflexive single tokens.
const minReasonLen = 12

// junkReasons are reasons that are technically non-empty but carry no
// justification — the lazy bypass an LLM types to get past the gate. Matched
// case-insensitively against the whole trimmed reason.
var junkReasons = map[string]bool{
	"x": true, "xx": true, "xxx": true,
	"force": true, "skip": true, "override": true, "bypass": true,
	"because": true, "reason": true, "reasons": true,
	"test": true, "testing": true, "tmp": true, "temp": true,
	"na": true, "n/a": true, "none": true, "nil": true, "null": true,
	"yes": true, "y": true, "ok": true, "go": true, "do it": true,
	"just do it": true, "trust me": true, "please": true,
	"i need to": true, "needed": true, "need": true, "wip": true,
}

// allNonWord matches a reason made only of whitespace/punctuation (e.g. "...").
var allNonWord = regexp.MustCompile(`^[\W_]*$`)

// isRepeatedChar reports whether s is one character repeated (e.g. "aaaa").
// RE2 has no backreferences, so this is done by hand.
func isRepeatedChar(s string) bool {
	rs := []rune(s)
	if len(rs) == 0 {
		return false
	}
	for _, r := range rs[1:] {
		if r != rs[0] {
			return false
		}
	}
	return true
}

// DefaultJudge is a deterministic, offline reason-quality check. It catches the
// laziest bypass attempts (empty, junk tokens, single repeated char) without a
// network call, so it is safe to run everywhere including CI. It is the floor,
// not the ceiling: P0 overrides should layer an independent model Judge on top.
type DefaultJudge struct{}

// Name implements Judge.
func (DefaultJudge) Name() string { return "default" }

// Evaluate implements Judge with deterministic reason-quality heuristics.
func (DefaultJudge) Evaluate(_ context.Context, req JudgeRequest) (JudgeVerdict, error) {
	r := strings.TrimSpace(req.Reason)
	switch {
	case r == "":
		return JudgeVerdict{Allow: false, Explanation: "no --reason was provided"}, nil
	case len([]rune(r)) < minReasonLen:
		return JudgeVerdict{Allow: false,
			Explanation: "reason too short to be a real justification (need at least a brief sentence)"}, nil
	case allNonWord.MatchString(r):
		return JudgeVerdict{Allow: false, Explanation: "reason contains no words"}, nil
	case isRepeatedChar(r):
		return JudgeVerdict{Allow: false, Explanation: "reason is a single repeated character"}, nil
	case junkReasons[strings.ToLower(r)]:
		return JudgeVerdict{Allow: false,
			Explanation: "reason is a filler token, not a justification — say what is actually unusual about this case"}, nil
	}
	return JudgeVerdict{Allow: true, Explanation: "reason passed deterministic quality checks"}, nil
}

// Compile-time check.
var _ Judge = DefaultJudge{}
