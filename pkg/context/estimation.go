package context

// EstimateTokens provides a rough token count without an API call.
// Heuristic: 1 token ≈ 4 characters, with 4/3 padding for safety margin.
// Accuracy: within 20% for English text and code.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	base := len(text) / 4
	if base == 0 {
		base = 1
	}
	return base * 4 / 3 // 4/3 padding factor
}

// EstimateImageTokens returns a fixed estimate for image content.
// Images are typically ~2000 tokens regardless of size.
func EstimateImageTokens() int {
	return 2000
}
