// Package vcs provides version control for engram memory files (.ai.md and .why.md).
//
// It creates and manages a git repository for memory storage, automatically
// committing changes and pushing them to a remote for backup. The package
// implements a two-layer defense:
//
//   - Layer 1: Direct integration via the MemoryVCS facade called by engram CLI
//   - Layer 2: Claude Code hooks that catch changes bypassing the CLI
//
// Key features:
//   - Automatic companion file tracking (.ai.md <-> .why.md)
//   - Pre-commit validation (pairing enforcement, frontmatter linting)
//   - Configurable push strategies (immediate, async, batched, manual)
//   - Non-blocking async push with retry
//   - Full commit history and file restoration
package vcs
