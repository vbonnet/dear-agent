# User Testing Script - CLI Prompt Parameter Pattern

## Overview

This script is designed for S9 validation of the CLI prompt parameter pattern implementation.

**Target participants**: 3 new users (unfamiliar with gemini-deep-research)

**Total time per participant**: ~20 minutes

**Success criteria**:
- Scenario 1: <2 minutes to understand from --help
- Scenario 2: <5 minutes to customize one stage
- Scenario 3: <10 minutes to use @file syntax

---

## Pre-Session Setup

### Facilitator Checklist

- [ ] gemini-deep-research installed and working
- [ ] Test API key configured (or use mock mode if available)
- [ ] Clean test directory prepared
- [ ] Timer ready
- [ ] Data collection template prepared
- [ ] Screen recording enabled (optional, with consent)

### Participant Setup

**Prerequisites**:
- Participant has basic command-line experience
- Has never used gemini-deep-research before
- Has 20 minutes available

**Introduction** (say to participant):

> "Thank you for participating in this user testing session. We're testing a new feature for customizing AI research prompts. I'll ask you to complete 3 tasks. Please think aloud as you work, and ask questions if anything is unclear. There are no wrong answers - we're testing the tool, not you."

---

## Scenario 1: Understanding from --help

**Goal**: Verify users can understand the new flags within 2 minutes

**Instructions to participant**:

> "Imagine you want to customize how gemini-deep-research analyzes content. Your task is to figure out how to do this using only the --help output. Please read the help and explain back to me what you would do."

**Facilitator actions**:

1. Start timer
2. Ask participant to run: `gemini-deep-research --help`
3. Observe participant reading help output
4. Stop timer when participant explains understanding OR at 2 minutes

**Data to collect**:

- Time to comprehension: ______ seconds
- Participant explanation (paraphrased):
- Did participant identify the three stage flags? (Y/N)
- Did participant understand precedence (CLI > config)? (Y/N)
- Questions asked:
- Confusion points:

**Success metric**: <2 minutes to explain which flags customize prompts

---

## Scenario 2: Customize One Stage

**Goal**: Verify users can override one prompt stage within 5 minutes

**Instructions to participant**:

> "Now, actually run the tool on this test URL with a custom analyze prompt: 'Focus only on security topics'. Don't worry about the research running - we'll cancel it after it starts."

**Test URL**: `https://example.com/test-article`

**Facilitator actions**:

1. Start timer
2. Observe participant constructing command
3. Help if stuck >3 minutes (note what help was needed)
4. Stop timer when command is executed OR at 5 minutes

**Expected command**:
```bash
gemini-deep-research https://example.com/test-article --analyze-prompt "Focus only on security topics"
```

**Data to collect**:

- Time to execute: ______ seconds
- Command used:
- Did participant succeed without help? (Y/N)
- If helped, what was unclear?
- Errors encountered:
- Questions asked:

**Success metric**: <5 minutes to execute custom prompt command

---

## Scenario 3: Use @file Syntax

**Goal**: Verify users can use @file syntax within 10 minutes

**Setup for facilitator**:

Create a prompt file in test directory:

```bash
cat > custom-extract.txt << 'EOF'
Extract the following from the content:
1. Main technical concepts
2. Key technologies mentioned
3. Research questions posed
EOF
```

**Instructions to participant**:

> "We've created a file called 'custom-extract.txt' with a custom extraction prompt. Your task is to run gemini-deep-research using this file as the extract prompt instead of typing it directly. Use the --help if needed."

**Facilitator actions**:

1. Show participant the file: `cat custom-extract.txt`
2. Start timer
3. Observe participant discovering @file syntax
4. Help if stuck >5 minutes (note what was needed)
5. Stop timer when command is executed OR at 10 minutes

**Expected command**:
```bash
gemini-deep-research https://example.com/test-article --extract-prompt @custom-extract.txt
```

**Data to collect**:

- Time to execute: ______ seconds
- Command used:
- Did participant find @file syntax in --help? (Y/N)
- Did participant succeed without help? (Y/N)
- If helped, what was unclear?
- Questions asked:
- Would participant use this feature? (Y/N)

**Success metric**: <10 minutes to use @file syntax successfully

---

## Post-Scenario Questions

After all scenarios, ask:

### Usability Questions

1. "On a scale of 1-5, how easy was it to understand how to customize prompts?"
   - Rating: ______
   - Comments:

2. "Was the --help output clear enough, or did you need additional documentation?"
   - Response:

3. "Would you use the @file syntax for your own prompts?"
   - Y/N and why:

4. "What was most confusing about the new flags?"
   - Response:

5. "What would you improve about the help documentation?"
   - Response:

### Feature Understanding

6. "Can you explain back to me what the three stages (extract, analyze, research) do?"
   - Participant explanation:
   - Accuracy: (Correct / Partially correct / Incorrect)

7. "How would you customize all three stages at once?"
   - Participant answer:

8. "If you wanted project-wide custom prompts, how would you set that up?"
   - Participant answer:
   - Did they mention config files? (Y/N)

---

## Data Collection Template

**Participant ID**: ______

**Date**: ______

**Facilitator**: ______

### Timing Results

| Scenario | Target | Actual | Pass/Fail |
|----------|--------|--------|-----------|
| 1: --help comprehension | <2 min | _____ | _____ |
| 2: Customize one stage | <5 min | _____ | _____ |
| 3: @file syntax | <10 min | _____ | _____ |

### Qualitative Results

**Ease of Use** (1-5): ______

**Confusing aspects**:
-
-
-

**Suggestions for improvement**:
-
-
-

**Would use in real work** (Y/N): ______

---

## Analysis Guidelines

### Success Criteria

**Overall pass**: All 3 scenarios meet time targets for 2+ out of 3 participants

**NFR2 Validation**: Average comprehension time (Scenario 1) <2 minutes across all participants

### Red Flags

- Participant unable to complete Scenario 1 in 5 minutes → Help text unclear
- Participant unable to find @file syntax in Scenario 3 → Needs better documentation
- Multiple participants ask same question → Common confusion point, fix docs
- Ease of Use <3 average → Major usability issue

### Follow-up Actions

If any red flags occur:
1. Document specific pain points
2. Update --help text or add examples
3. Consider adding error messages with suggestions
4. Re-test with 1-2 additional participants

---

## Example Session Recording Template

```
Participant: P1
Date: 2026-02-03
Facilitator: [Name]

SCENARIO 1 (--help comprehension)
[00:00] Started reading --help
[00:35] Participant said: "I see there are three flags for different stages"
[00:52] Participant explained: "I would use --analyze-prompt to customize the analysis"
[00:52] COMPLETE - 52 seconds ✓

SCENARIO 2 (Customize one stage)
[00:00] Started constructing command
[00:18] Typed: gemini-deep-research https://example.com/test-article --analyze-prompt "Focus only on security topics"
[00:22] Executed command
[00:22] COMPLETE - 22 seconds ✓

SCENARIO 3 (@file syntax)
[00:00] Read --help looking for file option
[01:23] Asked: "Is there a way to use a file instead of typing?"
[01:30] Found @file syntax in --help examples
[02:05] Typed: gemini-deep-research https://example.com/test-article --extract-prompt @custom-extract.txt
[02:10] Executed command
[02:10] COMPLETE - 2min 10sec ✓

POST-SCENARIO
Ease of Use: 4/5
Comments: "Pretty straightforward once I saw the examples. The @ syntax is clever."
Confusing: "Wasn't sure at first if I could combine multiple custom prompts"
Would use: Yes
```

---

## Appendix: Mock Test Mode (Optional)

If API access is limited, prepare a mock mode:

```bash
# Mock script that simulates tool output without API call
export GEMINI_API_KEY="test-key"
export MOCK_MODE=true

# Tool should detect MOCK_MODE and return sample output quickly
```

This allows testing the CLI interface without consuming API quota.

---

## Notes for Facilitator

**Do**:
- Encourage thinking aloud
- Remain neutral (don't nod or shake head)
- Take detailed notes on confusion points
- Time each scenario accurately
- Ask follow-up "why" questions

**Don't**:
- Lead the participant to the answer
- Explain features before they attempt
- Interrupt their process
- Judge their approach

**If participant is stuck**:
- Wait at least 60 seconds of silence
- Ask: "What are you thinking right now?"
- If still stuck >3 minutes: Provide minimal hint (e.g., "Check the --help examples section")

---

## Expected Outcomes

**Successful validation**:
- 2+ participants complete all scenarios within time limits
- Average ease of use rating ≥3.5/5
- No major usability red flags
- NFR2 requirement met (<2 min comprehension)

**If validation fails**:
- Document specific failure modes
- Prioritize fixes (P0: blockers, P1: confusion, P2: nice-to-have)
- Implement fixes
- Re-test with new participants

**Deliverable for S9**:
- Completed data collection forms (3 participants)
- Summary report with pass/fail per NFR
- List of action items if validation failed
- Recommendation: Ship / Fix and re-test

