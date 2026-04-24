package analyzer

import (
	"encoding/json"
	"os"
)

// alternativeTools are the tools that indicate the agent switched strategy
// rather than retrying the denied Bash command.
var alternativeTools = map[string]bool{
	"Read":  true,
	"Grep":  true,
	"Glob":  true,
	"Write": true,
	"Edit":  true,
}

// ClassifyDenials correlates each denial with its transcript and classifies
// the outcome based on what the agent did after the denial.
func ClassifyDenials(denials []DenialEntry, cache *TranscriptCache) []ClassifiedDenial {
	results := make([]ClassifiedDenial, 0, len(denials))
	for _, d := range denials {
		cd := classifyOne(d, cache)
		results = append(results, cd)
	}
	return results
}

func classifyOne(denial DenialEntry, cache *TranscriptCache) ClassifiedDenial {
	cd := ClassifiedDenial{
		Denial: denial,
	}

	// Check if transcript file exists.
	if _, err := os.Stat(denial.TranscriptPath); os.IsNotExist(err) {
		cd.Outcome = OutcomeTranscriptMissing
		return cd
	}

	entries, err := cache.Get(denial.TranscriptPath)
	if err != nil {
		cd.Outcome = OutcomeTranscriptMissing
		return cd
	}

	// Find the index of the entry containing the denied tool_use_id.
	deniedIdx := findToolUseIndex(entries, denial.ToolUseID)
	if deniedIdx < 0 {
		cd.Outcome = OutcomeUnknown
		return cd
	}

	// Look at the next 5 entries after the denial for tool_use content blocks.
	limit := deniedIdx + 6 // deniedIdx + 1..5 inclusive
	if limit > len(entries) {
		limit = len(entries)
	}

	// Collect subsequent tool uses and results from entries after the denied one.
	var actions []toolAction
	for i := deniedIdx + 1; i < limit; i++ {
		blocks := parseContentBlocks(entries[i])
		for _, b := range blocks {
			if b.Type == "tool_use" {
				ta := toolAction{name: b.Name, toolUseID: b.ID}
				// Check if this tool_use was subsequently denied (has an error result).
				ta.hasError = hasErrorResult(entries[i+1:limit], b.ID)
				actions = append(actions, ta)
			}
		}
	}

	if len(actions) == 0 {
		cd.Outcome = OutcomeGaveUp
		cd.Confidence = 0.3
		cd.IsFalsePositive = false
		return cd
	}

	first := actions[0]

	switch {
	case first.name == "Bash" && !first.hasError:
		cd.Outcome = OutcomeRetrySuccess
		cd.NextToolName = "Bash"
		cd.IsFalsePositive = true
		cd.Confidence = 0.8
		cd.WastedCalls = countConsecutiveBashRetries(actions)

	case alternativeTools[first.name]:
		cd.Outcome = OutcomeSwitchedTool
		cd.NextToolName = first.name
		cd.IsFalsePositive = false
		cd.Confidence = 0.7

	case first.name == "Bash" && first.hasError:
		cd.Outcome = OutcomeRetryDenied
		cd.NextToolName = "Bash"
		cd.IsFalsePositive = true
		cd.Confidence = 0.6
		cd.WastedCalls = countConsecutiveBashRetries(actions)

	default:
		cd.Outcome = OutcomeUnknown
		cd.NextToolName = first.name
	}

	cd.RetryCount = countBashRetries(actions)
	return cd
}

// findToolUseIndex locates the transcript entry that contains a content block
// referencing the given toolUseID (either as tool_use id or tool_result tool_use_id).
func findToolUseIndex(entries []TranscriptEntry, toolUseID string) int {
	for i, e := range entries {
		blocks := parseContentBlocks(e)
		for _, b := range blocks {
			if b.ID == toolUseID || b.ToolUseID == toolUseID {
				return i
			}
		}
	}
	return -1
}

// parseContentBlocks extracts ContentBlocks from a TranscriptEntry's message.
func parseContentBlocks(entry TranscriptEntry) []ContentBlock {
	if len(entry.Message) == 0 {
		return nil
	}
	var msg TranscriptMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return nil
	}
	if len(msg.Content) == 0 {
		return nil
	}

	// Content can be a string (for user messages) or an array of ContentBlocks.
	// Try array first.
	var blocks []ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// hasErrorResult checks if any entry in the slice contains a tool_result
// with the given tool_use_id (indicating a denial/error).
func hasErrorResult(entries []TranscriptEntry, toolUseID string) bool {
	for _, e := range entries {
		blocks := parseContentBlocks(e)
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID == toolUseID {
				return true
			}
		}
	}
	return false
}

// countConsecutiveBashRetries counts consecutive Bash actions from the start.
func countConsecutiveBashRetries(actions []toolAction) int {
	count := 0
	for _, a := range actions {
		if a.name != "Bash" {
			break
		}
		count++
	}
	return count
}

// countBashRetries counts all Bash actions in the list.
func countBashRetries(actions []toolAction) int {
	count := 0
	for _, a := range actions {
		if a.name == "Bash" {
			count++
		}
	}
	return count
}

// toolAction represents a tool invocation found after a denial.
type toolAction struct {
	name      string
	toolUseID string
	hasError  bool
}
