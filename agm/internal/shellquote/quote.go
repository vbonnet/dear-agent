// Package shellquote owns AGM's dependency-neutral POSIX shell quoting.
package shellquote

import "strings"

// Quote wraps a value for safe interpolation as one POSIX shell word.
func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
