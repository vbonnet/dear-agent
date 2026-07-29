// Package pastevalidate owns validation for caller-controlled text that may
// cross tmux's terminal-paste boundary.
package pastevalidate

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Text rejects terminal controls and invalid UTF-8. Empty optional fields are
// permitted so callers can use the same primitive before deciding whether a
// paste is needed.
func Text(name, value string) error {
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
