package tmux

import (
	"regexp"
	"strings"
)

// Claude Code 2.1.234 renders the trust dialog's options unnumbered and lists
// "No, exit" first, so the numbered, Yes-first patterns in prompt_detector.go
// match nothing against a live pane. Rather than loosen those patterns — their
// prose-body validation is what stops a composer draft from being answered
// (ce-wn4qe) — the current layout is recognized by the one part of the dialog
// that terminal wrapping cannot reshape: the option block at the tail.
//
// The prose above the options wraps at the pane width, so any validator keyed
// to the body's line structure is width-dependent and fails on a real capture.
// The option rows never wrap.
var (
	claudeTrustOptionAffirmativePattern = regexp.MustCompile(`(?i)^(?:❯[ \t]*)?(?:\d+[.)][ \t]+)?Yes,[ \t]+(?:I trust this folder|proceed)[ \t\r]*$`)
	claudeTrustOptionNegativePattern    = regexp.MustCompile(`(?i)^(?:❯[ \t]*)?(?:\d+[.)][ \t]+)?No,[ \t]+(?:exit|quit)[ \t\r]*$`)
)

// claudeTrustOptionBlock locates the trust dialog's two option rows within a
// capture and records which one currently carries the selector marker.
type claudeTrustOptionBlock struct {
	affirmativeIndex int
	negativeIndex    int
	// selectorIndex is the row carrying "❯". It always equals either
	// affirmativeIndex or negativeIndex.
	selectorIndex int
}

// findClaudeTrustOptionBlock reports the trust dialog's option block when, and
// only when, that block owns the tail of the capture.
//
// The proof required is deliberately narrow, because whatever this returns can
// authorize a keystroke:
//
//   - the capture's last significant rows are exactly the two option rows,
//     followed by nothing but dialog chrome (blank rows, borders, the
//     confirm/cancel hint);
//   - both options are present, so a partial redraw is not answered;
//   - exactly one row carries the selector marker, so a torn redraw showing two
//     is refused rather than guessed at;
//   - the trust question appears above them, so option-shaped text alone (or a
//     historical, already-answered dialog scrolled up with newer content below)
//     is not mistaken for a live dialog.
//
// Option order is not assumed: the legacy layout lists the affirmative first
// and the current one lists it second.
func findClaudeTrustOptionBlock(lines []string) (claudeTrustOptionBlock, bool) {
	optionIndexes, above, ok := trustOptionRowsOwningTail(lines)
	if !ok {
		return claudeTrustOptionBlock{}, false
	}
	block, ok := resolveTrustOptionBlock(lines, optionIndexes)
	if !ok {
		return claudeTrustOptionBlock{}, false
	}

	// The dialog's own question has to be above the options, and everything
	// between the two has to be the dialog's own prose. Requiring only that the
	// question appear somewhere above would let a historical question authorize
	// a newer, unrelated selector — the ce-wn4qe failure mode.
	for index := above; index >= 0; index-- {
		if !containsClaudeTrustQuestion(strings.TrimSpace(stripANSI(lines[index]))) {
			continue
		}
		// The question row is included so its own wrapped continuation joins
		// back onto it instead of being validated as body prose.
		return block, validUnwrappedClaudeTrustBody(lines[index : above+1])
	}
	return claudeTrustOptionBlock{}, false
}

// trustOptionRowsOwningTail returns the indexes of the two option rows that own
// the tail of the capture, plus the index of the last row above them. It fails
// when anything other than dialog chrome follows the options, or when fewer
// than two option rows are present.
func trustOptionRowsOwningTail(lines []string) ([]int, int, bool) {
	index := len(lines) - 1
	for index >= 0 && isClaudeTrustDialogTailChrome(lines[index]) {
		index--
	}

	optionIndexes := make([]int, 0, 2)
	for index >= 0 && len(optionIndexes) < 2 {
		plain := strings.TrimSpace(stripANSI(lines[index]))
		if plain == "" {
			index--
			continue
		}
		if !isClaudeTrustOptionRow(plain) {
			return nil, 0, false
		}
		optionIndexes = append(optionIndexes, index)
		index--
	}
	if len(optionIndexes) != 2 {
		return nil, 0, false
	}
	return optionIndexes, index, true
}

func isClaudeTrustOptionRow(plain string) bool {
	return claudeTrustOptionAffirmativePattern.MatchString(plain) ||
		claudeTrustOptionNegativePattern.MatchString(plain)
}

// resolveTrustOptionBlock assigns the two option rows to their roles and locates
// the selector. It refuses a capture that repeats a role or that shows two
// selector markers, both of which mean the pane was captured mid-redraw.
func resolveTrustOptionBlock(lines []string, optionIndexes []int) (claudeTrustOptionBlock, bool) {
	block := claudeTrustOptionBlock{affirmativeIndex: -1, negativeIndex: -1, selectorIndex: -1}
	selectors := 0
	for _, optionIndex := range optionIndexes {
		plain := strings.TrimSpace(stripANSI(lines[optionIndex]))
		role := &block.negativeIndex
		if claudeTrustOptionAffirmativePattern.MatchString(plain) {
			role = &block.affirmativeIndex
		}
		if *role >= 0 {
			return claudeTrustOptionBlock{}, false
		}
		*role = optionIndex
		if strings.HasPrefix(plain, "❯") {
			selectors++
			block.selectorIndex = optionIndex
		}
	}
	if block.affirmativeIndex < 0 || block.negativeIndex < 0 || selectors != 1 {
		return claudeTrustOptionBlock{}, false
	}
	return block, true
}

// claudeTrustCapabilitySentencePattern matches the consequence sentence Claude
// Code renders between the trust question and the permission warning. It is
// absent from older builds, so it is optional.
var claudeTrustCapabilitySentencePattern = regexp.MustCompile(`(?i)^Claude Code(?:'ll| will) be able to read, edit,? and execute files here\.?$`)

// claudeTrustBodyMarkerPattern locates the fixed phrases that close out the
// trust dialog's body. Terminal wrapping can merge them onto the same physical
// row as the free-form permission summary, so they are split back out before
// the body is validated.
var claudeTrustBodyMarkerPattern = regexp.MustCompile(`(?i)(These will apply without asking|Only proceed if you trust this configuration|Security guide)`)

// validUnwrappedClaudeTrustBody reports whether the rows between the trust
// question and the option block are the dialog's own body.
//
// It works on unwrapped text because the body wraps at the pane width: the
// question, the permission warning and the rule summary all continue across
// physical rows, so any line-shaped validation is width-dependent and rejects
// real captures. Joining continuations back together first makes the check
// depend on the dialog's wording rather than on the terminal's geometry.
//
// The permission summary itself is free-form — it lists whatever rules the
// project pre-approves — but it is bounded on both sides by required phrases,
// so unrelated output cannot slip through in its place.
// The first row passed in is the question itself; everything after it is body.
func validUnwrappedClaudeTrustBody(lines []string) bool {
	body := unwrapClaudeTrustBody(lines)
	if len(body) == 0 || !containsClaudeTrustQuestion(body[0]) {
		return false
	}
	body = body[1:]
	if len(body) > 0 && claudeTrustCapabilitySentencePattern.MatchString(body[0]) {
		body = body[1:]
	}
	if len(body) == 0 {
		// No permission warning at all: a folder that pre-approves nothing.
		return true
	}
	if !strings.Contains(strings.ToLower(body[0]), "this folder pre-approves") {
		return false
	}

	// Every rule summary row has to look like a list of permission rules, so
	// unrelated output cannot ride along inside the summary (ce-wn4qe).
	index, ok := validTrustSummaryRegion(body)
	if !ok {
		return false
	}
	return onlyTrustClosingPhrases(body[index:])
}

// validTrustSummaryRegion consumes the rule summary and returns the index of
// the closing warning. At least one summary row is required, and the warning
// must be present: a summary that simply runs off the end is not a dialog.
func validTrustSummaryRegion(body []string) (int, bool) {
	index := 1
	for index < len(body) && !strings.Contains(strings.ToLower(body[index]), "these will apply without asking") {
		if !validTruncatedTrustPermissionSummary(body[index]) {
			return 0, false
		}
		index++
	}
	if index == 1 || index == len(body) {
		return 0, false
	}
	return index, true
}

func onlyTrustClosingPhrases(body []string) bool {
	for _, line := range body {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "these will apply without asking") ||
			strings.Contains(lower, "only proceed if you trust this configuration") ||
			lower == "security guide" {
			continue
		}
		return false
	}
	return true
}

// claudeTrustSummaryOverflowPattern matches the "and 51 more" tail Claude
// appends when it elides the rest of the pre-approved rules.
var claudeTrustSummaryOverflowPattern = regexp.MustCompile(`(?i)^and[ \t]+\d+[ \t]+more$`)

// validTruncatedTrustPermissionSummary is the width-tolerant sibling of
// validClaudeTrustPermissionSummary. Claude elides long rules with a horizontal
// ellipsis to fit the pane, which leaves parentheses unclosed mid-rule, so the
// strict validator rejects every real capture that has more rules than fit.
// Truncation is accepted here, but only as a suffix: the part of each rule that
// did survive still has to be a well-formed permission name.
func validTruncatedTrustPermissionSummary(line string) bool {
	line = strings.TrimSpace(line)
	if remainder, ok := strings.CutPrefix(line, "- "); ok {
		line = strings.TrimSpace(remainder)
	} else if remainder, ok := strings.CutPrefix(line, "• "); ok {
		line = strings.TrimSpace(remainder)
	}
	if line == "" || len(line) > 4096 {
		return false
	}
	rules, ok := splitTruncatedTrustPermissionRules(line)
	if !ok || len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if claudeTrustSummaryOverflowPattern.MatchString(rule) {
			continue
		}
		if truncated, wasTruncated := strings.CutSuffix(rule, "…"); wasTruncated {
			if !validTruncatedTrustPermissionRule(truncated) {
				return false
			}
			continue
		}
		if !validClaudeTrustPermissionRule(rule) {
			return false
		}
	}
	return true
}

// validTruncatedTrustPermissionRule validates the surviving prefix of an elided
// rule: its name, up to the argument list the ellipsis cut short.
func validTruncatedTrustPermissionRule(rule string) bool {
	if rule == "" || strings.TrimSpace(rule) != rule {
		return false
	}
	name := rule
	if open := strings.IndexByte(rule, '('); open >= 0 {
		if open == 0 {
			return false
		}
		name = rule[:open]
	}
	if strings.HasPrefix(name, "mcp__") {
		return validClaudePermissionName(name, true)
	}
	return name[0] >= 'A' && name[0] <= 'Z' && validClaudePermissionName(name, false)
}

// splitTruncatedTrustPermissionRules splits a summary row on its top-level
// commas. An ellipsis closes whatever parenthesis the elision cut off, which is
// what lets a truncated row be split at all.
func splitTruncatedTrustPermissionRules(line string) ([]string, bool) {
	const maxNesting = 8
	rules := make([]string, 0, 4)
	start := 0
	depth := 0
	for i, char := range line {
		switch char {
		case '(':
			depth++
			if depth > maxNesting {
				return nil, false
			}
		case ')':
			depth--
			if depth < 0 {
				return nil, false
			}
		case '…':
			depth = 0
		case ',':
			if depth == 0 {
				rules = append(rules, strings.TrimSpace(line[start:i]))
				start = i + 1
			}
		case '\r', '\n':
			return nil, false
		}
	}
	if depth != 0 {
		return nil, false
	}
	return append(rules, strings.TrimSpace(line[start:])), true
}

// unwrapClaudeTrustBody rejoins rows the terminal wrapped, then splits the
// dialog's fixed closing phrases back onto their own rows so the caller can
// reason about the body's structure independently of the pane width.
//
// A row is treated as a continuation of the previous one when the previous row
// does not end at a sentence boundary. That is exactly the shape hard wrapping
// produces, and it is what keeps unrelated output — which does end at a
// boundary, or starts after one — from being absorbed into the question.
func unwrapClaudeTrustBody(lines []string) []string {
	var joined []string
	for _, line := range lines {
		plain := strings.TrimSpace(stripANSI(line))
		if plain == "" || strings.Trim(plain, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
			continue
		}
		if len(joined) > 0 && !endsAtSentenceBoundary(joined[len(joined)-1]) {
			joined[len(joined)-1] += " " + plain
			continue
		}
		joined = append(joined, plain)
	}

	var body []string
	for _, line := range joined {
		body = append(body, splitAtClaudeTrustBodyMarkers(line)...)
	}
	return body
}

func endsAtSentenceBoundary(line string) bool {
	if line == "" {
		return true
	}
	switch line[len(line)-1] {
	case '.', '?', '!', ':':
		return true
	}
	return false
}

func splitAtClaudeTrustBodyMarkers(line string) []string {
	locations := claudeTrustBodyMarkerPattern.FindAllStringIndex(line, -1)
	if len(locations) == 0 {
		return []string{line}
	}
	var parts []string
	previous := 0
	for _, location := range locations {
		if location[0] == 0 {
			continue
		}
		if segment := strings.TrimSpace(line[previous:location[0]]); segment != "" {
			parts = append(parts, segment)
		}
		previous = location[0]
	}
	if segment := strings.TrimSpace(line[previous:]); segment != "" {
		parts = append(parts, segment)
	}
	return parts
}

// isClaudeTrustDialogTailChrome reports whether a row below the option block
// still belongs to the dialog. It is deliberately stricter than
// isClaudeTrustDialogChrome, which also accepts option rows: here an option row
// must be counted as an option, not skipped as chrome.
func isClaudeTrustDialogTailChrome(line string) bool {
	plain := strings.TrimSpace(stripANSI(line))
	if plain == "" {
		return true
	}
	if strings.Trim(plain, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	return claudeTrustDialogHintPattern.MatchString(plain)
}

// trustAffirmativeNavigationKey reports the arrow key that moves the trust
// dialog's selector onto "Yes, I trust this folder", and whether such a move is
// needed and unambiguous.
//
// This exists because the dialog opens with the *negative* option selected. A
// detector that only observes the selector, as AGM's did, therefore waits out
// its budget on a dialog that will never answer itself, and an operator who
// blind-presses Enter selects "No, exit" and kills the harness.
//
// It reports no move once the affirmative option is already selected: pressing
// Enter is then the caller's decision, gated separately.
func trustAffirmativeNavigationKey(content string) (string, bool) {
	block, ok := findClaudeTrustOptionBlock(strings.Split(content, "\n"))
	if !ok || block.selectorIndex == block.affirmativeIndex {
		return "", false
	}
	if block.affirmativeIndex > block.selectorIndex {
		return "Down", true
	}
	return "Up", true
}

// classifyCurrentTrustOptionBlock maps the option block onto the selection
// states the numbered-layout classifier reports, so callers keep one vocabulary.
func classifyCurrentTrustOptionBlock(content string) claudeTrustSelection {
	block, ok := findClaudeTrustOptionBlock(strings.Split(content, "\n"))
	if !ok {
		return claudeTrustNotSelected
	}
	if block.selectorIndex == block.affirmativeIndex {
		return claudeTrustAffirmativeSelected
	}
	return claudeTrustNegativeSelected
}
