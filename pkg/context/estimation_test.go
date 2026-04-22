package context

import (
	"strings"
	"testing"
)

func TestEstimateTokens_Empty(t *testing.T) {
	got := EstimateTokens("")
	if got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTokens_ShortText(t *testing.T) {
	got := EstimateTokens("hi")
	if got < 1 {
		t.Errorf("EstimateTokens(\"hi\") = %d, want >= 1", got)
	}
}

func TestEstimateTokens_EnglishProse(t *testing.T) {
	// "The quick brown fox jumps over the lazy dog" = 9 words ≈ 10 tokens (GPT/Claude)
	// 43 chars -> base=10 -> padded=13. With padding the estimate is intentionally conservative.
	text := "The quick brown fox jumps over the lazy dog"
	got := EstimateTokens(text)
	actual := 13 // 43/4 * 4/3 = 14, actual ~10 tokens but padded estimate is ~13
	assertWithin20Pct(t, "English prose", got, actual)
}

func TestEstimateTokens_GoCode(t *testing.T) {
	// Go function: ~30 tokens actual
	code := `func main() {
	fmt.Println("hello world")
	for i := 0; i < 10; i++ {
		fmt.Println(i)
	}
}`
	got := EstimateTokens(code)
	actual := 35 // approximate actual token count
	assertWithin20Pct(t, "Go code", got, actual)
}

func TestEstimateTokens_JSON(t *testing.T) {
	json := `{"name": "test", "value": 42, "items": ["a", "b", "c"], "nested": {"key": "val"}}`
	got := EstimateTokens(json)
	actual := 30 // approximate actual token count
	assertWithin20Pct(t, "JSON", got, actual)
}

func TestEstimateTokens_LongText(t *testing.T) {
	// 4000 chars -> base=1000 -> padded=1333
	text := strings.Repeat("word ", 800) // 4000 chars
	got := EstimateTokens(text)
	actual := 1333 // padded estimate: 4000/4 * 4/3
	assertWithin20Pct(t, "long text", got, actual)
}

func TestEstimateTokens_Unicode(t *testing.T) {
	// Unicode: each char is multi-byte, so byte-based estimate will be higher
	// "こんにちは世界" = 7 chars, ~7 tokens actual, but 21 bytes -> base=5 -> padded=6
	text := "こんにちは世界"
	got := EstimateTokens(text)
	if got < 1 {
		t.Errorf("EstimateTokens(unicode) = %d, want >= 1", got)
	}
}

func TestEstimateImageTokens(t *testing.T) {
	got := EstimateImageTokens()
	if got != 2000 {
		t.Errorf("EstimateImageTokens() = %d, want 2000", got)
	}
}

func assertWithin20Pct(t *testing.T, label string, got, actual int) {
	t.Helper()
	low := actual * 80 / 100
	high := actual * 120 / 100
	if got < low || got > high {
		t.Errorf("%s: EstimateTokens = %d, want within 20%% of %d (range %d-%d)",
			label, got, actual, low, high)
	}
}
