// Package githooks holds behavioural tests for the repo's shell git hooks
// (scripts/git-hooks/*). The hooks themselves are POSIX shell — these tests
// exec them in a throwaway git repo with a stubbed `agm` on PATH so the hook's
// control flow (which branch triggers a sweep, opt-out, fail-safe exit) is
// covered without touching a real worktree or invoking the real reaper.
package githooks
