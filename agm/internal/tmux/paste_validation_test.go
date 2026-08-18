package tmux

import (
	"strings"
	"testing"
)

// AGM-SHELLQUOTE-05: a NUL byte can be represented in a Go string but not in a
// POSIX shell command or an OS argv entry, so shellquote.Quote deliberately
// scopes its round-trip contract to NUL-free input. That is only sound if the
// terminal-paste boundary rejects NUL before Quote is ever reached, so this
// test pins the boundary rather than leaving the exclusion as prose.
func TestValidatePastedTextRejectsNUL(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		value string
	}{
		{name: "leading", value: "\x00prompt"},
		{name: "embedded", value: "pro\x00mpt"},
		{name: "trailing", value: "prompt\x00"},
		{name: "only", value: "\x00"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePastedText("prompt", testCase.value)
			if err == nil {
				t.Fatalf("ValidatePastedText accepted a NUL-bearing value %q", testCase.value)
			}
			if !strings.Contains(err.Error(), "control characters") {
				t.Fatalf("err = %v, want a control-character rejection", err)
			}
		})
	}
}

func TestValidatePastedTextAcceptsOrdinaryText(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"",
		"plain prompt",
		"quotes ' and \" stay inert",
		"unicode ✓ é 漢字",
		"metacharacters $(id) `id` && || ; > <",
	} {
		if err := ValidatePastedText("prompt", value); err != nil {
			t.Fatalf("ValidatePastedText(%q) = %v, want nil", value, err)
		}
	}
}
