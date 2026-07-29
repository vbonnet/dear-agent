package tmux

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidatePastedText rejects terminal controls and invalid UTF-8 before a
// caller-controlled value crosses tmux's terminal-paste boundary. Empty
// optional fields are permitted.
func ValidatePastedText(name, value string) error {
	if value == "" {
		return nil
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("invalid harness launch request: %s contains invalid UTF-8", name)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("invalid harness launch request: %s contains control characters", name)
	}
	return nil
}
