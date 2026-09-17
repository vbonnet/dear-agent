package tmux

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var exactSubmissionPasteMarker = regexp.MustCompile(`(?i)\[(?:Pasted Content\s+(\d+)\s+chars|Pasted text(?:\s+#\d+)?\s+\+(\d+)\s+lines?)\]`)
var submissionSelectionInput = regexp.MustCompile(`^\d+[.)]\s`)

type submissionComposerCandidate struct {
	index int
	input string
}

// classifyExactCommandSubmissionAfterBaseline requires at least one more
// structural exact-command anchor than existed before the paste. Full tmux
// history can retain an earlier identical `/compact`; without this baseline, a
// concurrent clear followed by an empty Enter could borrow that old echo and
// be falsely reported as a newly submitted command.
func classifyExactCommandSubmissionAfterBaseline(content, harness, command string, baselineAnchors int) submissionObservation {
	if countExactCommandSubmissionAnchors(content, harness, command) <= baselineAnchors {
		return submissionAmbiguous
	}
	return classifyExactCommandSubmission(content, harness, command)
}

func countExactCommandSubmissionAnchors(content, harness, command string) int {
	plain := strings.ReplaceAll(stripANSI(content), "\r", "")
	lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
	commandLines := strings.Split(command, "\n")
	count := 0
	for _, line := range lines {
		if input, ok := submissionComposerInput(line, harness); ok {
			if couldBeExactDeliveryAnchor(input, commandLines, command) {
				count++
			}
			continue
		}
		// Pi and compact layouts can render a measured paste marker without a
		// prompt glyph. Count the same structural fallback accepted below.
		if markerMatchesExactCommand(cleanSubmissionLine(line), command) {
			count++
		}
	}
	return count
}

// classifyExactCommandSubmission interprets one post-Enter pane capture. It
// authorizes another Enter only when the exact delivery-owned command is still
// positively parked in the current composer. A different occupied composer or
// an incomplete rendering is ambiguous, so strict delivery stops after the
// first accepted Enter instead of risking submission of unrelated input.
func classifyExactCommandSubmission(content, harness, command string) submissionObservation {
	plain := strings.ReplaceAll(stripANSI(content), "\r", "")
	lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
	commandLines := strings.Split(command, "\n")

	candidates := findSubmissionComposerCandidates(lines, harness)
	if observation, found := classifySubmissionComposerCandidates(candidates, lines, commandLines, command, harness); found {
		return observation
	}
	if observation, found := classifyMeasuredSubmissionMarker(lines, commandLines, command, harness); found {
		return observation
	}

	// A command-looking tail without a structural composer might be the harness
	// echo after submission. Absence is not positive submission evidence either:
	// alternate-screen rendering or a concurrent human clear can remove a parked
	// command without submitting it. Both cases stop without retrying Enter.
	return submissionAmbiguous
}

func findSubmissionComposerCandidates(lines []string, harness string) []submissionComposerCandidate {
	var candidates []submissionComposerCandidate
	for i, line := range lines {
		if input, ok := submissionComposerInput(line, harness); ok {
			candidates = append(candidates, submissionComposerCandidate{index: i, input: input})
		}
	}
	return candidates
}

func classifySubmissionComposerCandidates(
	candidates []submissionComposerCandidate,
	lines, commandLines []string,
	command, harness string,
) (submissionObservation, bool) {
	latestDelivery := -1
	deliveryObserved := false
	for _, candidate := range candidates {
		if !couldBeExactDeliveryAnchor(candidate.input, commandLines, command) {
			continue
		}
		latestDelivery = candidate.index
		observation := classifySubmissionComposerRegion(
			lines[candidate.index+1:], candidate.input, commandLines, command, harness,
		)
		if observation == submissionStillParked {
			// Bind the full known command extent before considering prompt-like
			// glyphs inside its payload. A payload line containing bare ">" can
			// never replace this positively measured outer composer.
			return submissionStillParked, true
		}
		if observation == submissionAmbiguous {
			return submissionAmbiguous, true
		}
		deliveryObserved = true
	}
	if len(candidates) == 0 {
		return submissionAmbiguous, false
	}

	latest := candidates[len(candidates)-1]
	if latest.index > latestDelivery {
		if deliveryObserved && isEmptySubmissionComposer(cleanSubmissionLine(latest.input), harness) {
			return submissionObserved, true
		}
		return classifySubmissionComposerRegion(
			lines[latest.index+1:], latest.input, commandLines, command, harness,
		), true
	}
	if deliveryObserved {
		return submissionObserved, true
	}
	return classifySubmissionComposerRegion(
		lines[latest.index+1:], latest.input, commandLines, command, harness,
	), true
}

func classifyMeasuredSubmissionMarker(
	lines, commandLines []string,
	command, harness string,
) (submissionObservation, bool) {
	// Pi and some compact TUI layouts render only a measured paste marker plus
	// managed footer, without a prompt glyph. The exact marker can prove the
	// delivery-owned paste is still parked; any unexpected payload is ambiguous.
	for i, line := range slices.Backward(lines) {
		marker := cleanSubmissionLine(line)
		if markerMatchesExactCommand(marker, command) {
			tail := trimSubmissionChrome(lines[i+1:], harness)
			if len(tail) == 0 || normalizedCommandPrefix(tail, commandLines) == len(commandLines) {
				return submissionStillParked, true
			}
			return submissionAmbiguous, true
		}
	}
	return submissionAmbiguous, false
}

func couldBeExactDeliveryAnchor(input string, commandLines []string, command string) bool {
	plain := cleanSubmissionLine(input)
	if marker := exactSubmissionPasteMarker.FindString(plain); marker != "" {
		return markerMatchesExactCommand(marker, command)
	}
	if len(commandLines) == 0 {
		return false
	}
	return normalizeSubmissionText(plain) == normalizeSubmissionText(commandLines[0])
}

func classifySubmissionComposerRegion(
	tail []string,
	anchorInput string,
	commandLines []string,
	command, harness string,
) submissionObservation {
	input := cleanSubmissionLine(anchorInput)
	if marker := exactSubmissionPasteMarker.FindString(input); marker != "" {
		if !markerMatchesExactCommand(marker, command) {
			return submissionAmbiguous
		}
		trimmed := trimSubmissionChrome(tail, harness)
		if len(trimmed) == 0 {
			return submissionStillParked
		}
		matched := normalizedCommandPrefix(trimmed, commandLines)
		if matched == len(commandLines) {
			if len(trimSubmissionChrome(trimmed[matched:], harness)) == 0 {
				return submissionStillParked
			}
			return submissionObserved
		}
		return submissionAmbiguous
	}

	first := cleanSubmissionLine(input)
	if isEmptySubmissionComposer(first, harness) {
		return submissionAmbiguous
	}
	combined := append([]string{first}, tail...)
	matched := normalizedCommandPrefix(combined, commandLines)
	if matched == len(commandLines) {
		if len(trimSubmissionChrome(combined[matched:], harness)) == 0 {
			return submissionStillParked
		}
		// This shape is consistent with the exact command leaving the parked
		// composer, so another Enter would be unsafe. It is not final delivery
		// proof: an appended human line can have the same rendering. The separate
		// exact-runtime recheck must still observe READY or native PROCESSING;
		// generic BUSY turns this outcome into may-have-started uncertainty.
		return submissionObserved
	}
	// A prefix overlap is not proof that AGM's exact bytes remain parked. A
	// human may have started a new draft after the first accepted Enter; never
	// submit that draft with a retry.
	return submissionAmbiguous
}

func submissionComposerInput(line, harness string) (string, bool) {
	plain := strings.TrimSpace(stripANSI(line))
	plain = strings.TrimSpace(strings.Trim(plain, "│┃"))
	markers := []string{"❯", "›", "»"}
	if harness == "agy" || harness == "gemini-cli" || harness == "opencode-cli" || harness == "pi-cli" || harness == "codex-cli" {
		markers = append(markers, ">>", ">")
	}
	for _, marker := range markers {
		if plain == marker {
			return "", true
		}
		if strings.HasPrefix(plain, marker+" ") {
			candidate := strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.TrimPrefix(plain, marker)), "│┃"))
			// Selection dialogs also use prompt glyphs. They are not a command
			// composer and therefore remain ambiguous rather than retried.
			if submissionSelectionInput.MatchString(candidate) {
				return candidate, true
			}
			return candidate, true
		}
	}
	return "", false
}

func markerMatchesExactCommand(marker, command string) bool {
	matches := exactSubmissionPasteMarker.FindStringSubmatch(marker)
	if matches == nil {
		return false
	}
	if matches[1] != "" {
		count, err := strconv.Atoi(matches[1])
		return err == nil && count == utf8.RuneCountInString(command)
	}
	count, err := strconv.Atoi(matches[2])
	return err == nil && count == len(strings.Split(command, "\n"))
}

func normalizedCommandPrefix(lines, commandLines []string) int {
	if len(lines) < len(commandLines) {
		return -1
	}
	for i, expected := range commandLines {
		if normalizeSubmissionText(lines[i]) != normalizeSubmissionText(expected) {
			return -1
		}
	}
	return len(commandLines)
}

func trimSubmissionChrome(lines []string, harness string) []string {
	trimmed := append([]string(nil), lines...)
	for len(trimmed) > 0 && isSubmissionChrome(trimmed[len(trimmed)-1], harness) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}

func isSubmissionChrome(line, harness string) bool {
	plain := cleanSubmissionLine(line)
	if isDecorativeChromeLine(plain) {
		return true
	}
	switch harness {
	case "claude-code":
		return isClaudeComposerFooterChrome(plain)
	case "codex-cli":
		return codexFooterPattern.MatchString(plain)
	case "gemini-cli":
		return isGeminiIdleFooter(plain) || isTerminalIdleChrome(plain)
	case "opencode-cli", "agy":
		return isTerminalIdleChrome(plain)
	case "pi-cli":
		return isPiIdleComposerChrome(plain)
	default:
		return false
	}
}

func isEmptySubmissionComposer(input, harness string) bool {
	plain := strings.ToLower(strings.TrimSpace(input))
	if plain == "" {
		return true
	}
	if harness == "gemini-cli" || harness == "opencode-cli" {
		return strings.Contains(plain, "type your message") || strings.Contains(plain, "type here")
	}
	return false
}

func cleanSubmissionLine(line string) string {
	plain := strings.TrimSpace(stripANSI(line))
	plain = strings.TrimSpace(strings.Trim(plain, "│┃"))
	return plain
}

func normalizeSubmissionText(line string) string {
	return cleanSubmissionLine(line)
}
