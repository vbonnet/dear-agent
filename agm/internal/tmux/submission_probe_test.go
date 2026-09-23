package tmux

import (
	"strings"
	"testing"
)

func TestClassifyExactCommandSubmissionStopsRetryAcrossGenericBusyHarnesses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		harness string
		parked  string
		running string
	}{
		{
			name:    "AGY",
			harness: "agy",
			parked:  "> /compact",
			running: "> /compact\nprocessing request\nresponse chunk",
		},
		{
			name:    "Gemini",
			harness: "gemini-cli",
			parked:  "│ > /compact │\n╰────────╯\n? for shortcuts",
			running: "│ > /compact │\nprocessing request",
		},
		{
			name:    "OpenCode",
			harness: "opencode-cli",
			parked:  "> /compact",
			running: "> /compact\nWorking on request",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyExactCommandSubmission(test.parked, test.harness, "/compact"); got != submissionStillParked {
				t.Fatalf("parked classifier = %v, want still parked", got)
			}
			if got := classifyExactCommandSubmission(test.running, test.harness, "/compact"); got != submissionObserved {
				t.Fatalf("running classifier = %v, want observed", got)
			}
		})
	}
}

func TestClassifyExactCommandSubmissionBindsPromptGlyphInsidePayload(t *testing.T) {
	t.Parallel()
	command := "/compact preserve this exact context\n>"
	parked := "❯ /compact preserve this exact context\n>\n────────────────\n? for shortcuts"
	if got := classifyExactCommandSubmission(parked, "claude-code", command); got != submissionStillParked {
		t.Fatalf("payload prompt glyph classifier = %v, want still parked", got)
	}
}

func TestClassifyExactCommandSubmissionRetainsMoreThanFiveHundredParkedLines(t *testing.T) {
	t.Parallel()
	commandLines := make([]string, 0, 601)
	commandLines = append(commandLines, "/compact preserve all context")
	for range 600 {
		commandLines = append(commandLines, "- retained evidence")
	}
	command := strings.Join(commandLines, "\n")
	parked := "❯ " + command + "\n────────────────\n? for shortcuts"
	if got := classifyExactCommandSubmission(parked, "claude-code", command); got != submissionStillParked {
		t.Fatalf("long parked command classifier = %v, want still parked", got)
	}
}

func TestClassifyExactCommandSubmissionMeasuredPasteMarker(t *testing.T) {
	t.Parallel()
	command := "/compact preserve\n- project: audit"
	parked := "❯ [Pasted text #1 +2 lines]\n" + command + "\n────────────────\n? for shortcuts"
	if got := classifyExactCommandSubmission(parked, "claude-code", command); got != submissionStillParked {
		t.Fatalf("measured paste classifier = %v, want still parked", got)
	}

	wrongExtent := strings.Replace(parked, "+2 lines", "+3 lines", 1)
	if got := classifyExactCommandSubmission(wrongExtent, "claude-code", command); got != submissionAmbiguous {
		t.Fatalf("wrong paste extent classifier = %v, want ambiguous", got)
	}
}

func TestVerifyingEnterStrictAmbiguousComposerStopsAfterOneEnter(t *testing.T) {
	t.Parallel()
	enters := 0
	config := fastConfig()
	config.requireObservedSubmission = true
	config.classifySubmission = func(string) submissionObservation { return submissionAmbiguous }
	err := verifyingEnter(func() error {
		enters++
		return nil
	}, func() (string, error) {
		return "> unrelated human draft", nil
	}, config)
	if err == nil || !PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("ambiguous composer error = %v, want submission uncertainty", err)
	}
	if enters != 1 {
		t.Fatalf("ambiguous composer received %d Enter attempts, want exactly one", enters)
	}
}

func TestClassifyExactCommandSubmissionPartialOverlapIsAmbiguous(t *testing.T) {
	t.Parallel()
	if got := classifyExactCommandSubmission("> /com", "agy", "/compact preserve"); got != submissionAmbiguous {
		t.Fatalf("partial command overlap classifier = %v, want ambiguous", got)
	}
}

func TestClassifyExactCommandSubmissionAbsenceOrConcurrentClearIsAmbiguous(t *testing.T) {
	t.Parallel()
	for _, content := range []string{
		"processing request without a retained command echo",
		">\n? for shortcuts",
	} {
		if got := classifyExactCommandSubmission(content, "agy", "/compact preserve"); got != submissionAmbiguous {
			t.Fatalf("cleared/absent command classifier for %q = %v, want ambiguous", content, got)
		}
	}
}

func TestClassifyExactCommandSubmissionDoesNotBorrowHistoricalIdenticalCommandAfterClear(t *testing.T) {
	t.Parallel()
	command := "/compact"
	baseline := "❯ /compact\nold compaction output\n❯\n────────────────\n? for shortcuts"
	baselineAnchors := countExactCommandSubmissionAnchors(baseline, "claude-code", command)
	if baselineAnchors != 1 {
		t.Fatalf("baseline anchors = %d, want 1", baselineAnchors)
	}

	// The newly pasted command was cleared before Enter, so the post-Enter pane
	// still contains only the old echo and the current empty composer.
	if got := classifyExactCommandSubmissionAfterBaseline(
		baseline, "claude-code", command, baselineAnchors,
	); got != submissionAmbiguous {
		t.Fatalf("historical repeated-command classifier = %v, want ambiguous", got)
	}
}

func TestClassifyExactCommandSubmissionAcceptsNewAnchorAfterBaseline(t *testing.T) {
	t.Parallel()
	command := "/compact"
	baseline := "❯ /compact\nold compaction output\n❯\n────────────────\n? for shortcuts"
	postEnter := "❯ /compact\nold compaction output\n❯ /compact\nnew compaction output\n❯\n────────────────\n? for shortcuts"
	if got := classifyExactCommandSubmissionAfterBaseline(
		postEnter, "claude-code", command,
		countExactCommandSubmissionAnchors(baseline, "claude-code", command),
	); got != submissionObserved {
		t.Fatalf("new repeated-command classifier = %v, want observed", got)
	}
}
