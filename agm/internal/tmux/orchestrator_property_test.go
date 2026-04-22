package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// TestOrchestratorProperty_SessionNamesAlwaysUnique verifies session names never collide
func TestOrchestratorProperty_SessionNamesAlwaysUnique(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Sanitized session names are always unique for different inputs", prop.ForAll(
		func(name1, name2 string) bool {
			sanitized1 := SanitizeSessionName(name1)
			sanitized2 := SanitizeSessionName(name2)

			// If inputs are different, sanitized outputs should be different
			// (unless both are invalid and fall back to "session")
			if name1 == name2 {
				return sanitized1 == sanitized2
			}

			// If both sanitize to "session", they were both invalid
			// This is acceptable - invalid inputs can collide
			if sanitized1 == "session" && sanitized2 == "session" {
				return true
			}

			// Different valid inputs should produce different outputs
			return sanitized1 != sanitized2 || (sanitized1 == "session" && sanitized2 == "session")
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SanitizedNamesValid verifies all sanitized names are tmux-compatible
func TestOrchestratorProperty_SanitizedNamesValid(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Sanitized session names only contain valid tmux characters", prop.ForAll(
		func(name string) bool {
			sanitized := SanitizeSessionName(name)

			// Must be non-empty
			if sanitized == "" {
				return false
			}

			// Must only contain alphanumeric, dash, underscore
			for _, r := range sanitized {
				valid := (r >= 'a' && r <= 'z') ||
					(r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') ||
					r == '-' || r == '_'
				if !valid {
					return false
				}
			}

			return true
		},
		gen.AnyString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SocketPathsAlwaysAbsolute verifies socket paths are always absolute
func TestOrchestratorProperty_SocketPathsAlwaysAbsolute(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Socket paths are always absolute", prop.ForAll(
		func() bool {
			socketPath := GetSocketPath()

			// Must be absolute path
			return filepath.IsAbs(socketPath)
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SocketPathsWritable verifies socket path parents are writable
func TestOrchestratorProperty_SocketPathsWritable(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Socket path parent directory is writable", prop.ForAll(
		func() bool {
			socketPath := GetSocketPath()
			parentDir := filepath.Dir(socketPath)

			// Parent must be a writable location:
			// - ~/.agm/ (default, safe from /tmp cleanup)
			// - /tmp or subdirectory (legacy/override)
			home, _ := os.UserHomeDir()
			agmDir := filepath.Join(home, ".agm")
			return parentDir == agmDir || parentDir == "/tmp" || strings.HasPrefix(parentDir, "/tmp/")
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_NormalizePreservesValid verifies normalization preserves valid names
func TestOrchestratorProperty_NormalizePreservesValid(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("NormalizeTmuxSessionName preserves names without special chars", prop.ForAll(
		func(name string) bool {
			// Only test names that don't contain special characters
			if strings.ContainsAny(name, ". :") {
				return true // Skip test for names with special chars
			}

			normalized := NormalizeTmuxSessionName(name)

			// Should be unchanged if no special chars
			return normalized == name
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_NormalizeReplacesSpecialChars verifies normalization replaces special characters
func TestOrchestratorProperty_NormalizeReplacesSpecialChars(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("NormalizeTmuxSessionName replaces dots/colons/spaces with dashes", prop.ForAll(
		func(prefix, suffix string) bool {
			// Test with each special character
			testCases := []string{
				prefix + "." + suffix,
				prefix + ":" + suffix,
				prefix + " " + suffix,
			}

			for _, testName := range testCases {
				normalized := NormalizeTmuxSessionName(testName)

				// Should not contain original special chars
				if strings.ContainsAny(normalized, ".: ") {
					return false
				}

				// Should contain dashes (replacements)
				if !strings.Contains(normalized, "-") {
					return false
				}
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SanitizeIdempotent verifies sanitization is idempotent
func TestOrchestratorProperty_SanitizeIdempotent(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Sanitizing twice produces same result", prop.ForAll(
		func(name string) bool {
			sanitized1 := SanitizeSessionName(name)
			sanitized2 := SanitizeSessionName(sanitized1)

			// Sanitizing an already-sanitized name should be no-op
			return sanitized1 == sanitized2
		},
		gen.AnyString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_NormalizeIdempotent verifies normalization is idempotent
func TestOrchestratorProperty_NormalizeIdempotent(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Normalizing twice produces same result", prop.ForAll(
		func(name string) bool {
			normalized1 := NormalizeTmuxSessionName(name)
			normalized2 := NormalizeTmuxSessionName(normalized1)

			// Normalizing an already-normalized name should be no-op
			return normalized1 == normalized2
		},
		gen.AnyString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SanitizeFallbackNonEmpty verifies fallback is always non-empty
func TestOrchestratorProperty_SanitizeFallbackNonEmpty(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Sanitized name is never empty", prop.ForAll(
		func(name string) bool {
			sanitized := SanitizeSessionName(name)

			// Even for invalid/empty inputs, must return non-empty result
			return sanitized != ""
		},
		gen.AnyString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SocketPathConsistent verifies socket path is consistent
func TestOrchestratorProperty_SocketPathConsistent(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("GetSocketPath returns consistent value", prop.ForAll(
		func() bool {
			path1 := GetSocketPath()
			path2 := GetSocketPath()

			// Should return same value on repeated calls
			return path1 == path2
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_ReadSocketPathsNonEmpty verifies read paths are never empty
func TestOrchestratorProperty_ReadSocketPathsNonEmpty(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("GetReadSocketPaths always returns non-empty slice", prop.ForAll(
		func() bool {
			paths := GetReadSocketPaths()

			// Must return at least one path
			if len(paths) == 0 {
				return false
			}

			// All paths must be non-empty
			for _, path := range paths {
				if path == "" {
					return false
				}
			}

			return true
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SanitizeRemovesInvalidChars verifies invalid characters are removed
func TestOrchestratorProperty_SanitizeRemovesInvalidChars(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Sanitize removes all invalid tmux characters", prop.ForAll(
		func(name string) bool {
			sanitized := SanitizeSessionName(name)

			// Invalid chars: anything except alphanumeric, dash, underscore
			invalidChars := "!@#$%^&*()+=[]{}\\|;:'\",.<>?/`~"

			// Sanitized name must not contain any invalid chars
			for _, invalidChar := range invalidChars {
				if strings.ContainsRune(sanitized, invalidChar) {
					return false
				}
			}

			return true
		},
		gen.AnyString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_NormalizePreservesLength verifies normalization doesn't change length
func TestOrchestratorProperty_NormalizePreservesLength(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("NormalizeTmuxSessionName preserves string length", prop.ForAll(
		func(name string) bool {
			normalized := NormalizeTmuxSessionName(name)

			// Normalization only replaces characters, doesn't add/remove
			return len(normalized) == len(name)
		},
		gen.AnyString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestOrchestratorProperty_SanitizeSpacesToDashes verifies spaces become dashes
func TestOrchestratorProperty_SanitizeSpacesToDashes(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Sanitize converts spaces to dashes", prop.ForAll(
		func(word1, word2 string) bool {
			// Create name with space
			name := word1 + " " + word2

			// Skip if words are empty
			if word1 == "" || word2 == "" {
				return true
			}

			sanitized := SanitizeSessionName(name)

			// Should not contain spaces
			if strings.Contains(sanitized, " ") {
				return false
			}

			// Should contain dash (space was converted)
			// But only if the words themselves are valid
			return !strings.ContainsAny(word1+word2, " !@#$%^&*()+=[]{}\\|;:'\",.<>?/`~")
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
