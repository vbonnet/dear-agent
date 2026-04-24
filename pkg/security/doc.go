// Package security provides utilities for secure file operations.
//
// This package prevents common security vulnerabilities including:
//   - Path traversal attacks (../ sequences)
//   - Symlink following vulnerabilities
//   - File size DoS attacks
//   - Invalid file extensions
//   - Unsafe directory creation
//
// All diagram-as-code skills MUST use these utilities for file operations.
package security
