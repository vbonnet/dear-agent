// Package githooks holds behavioural tests for the repo's shell git hooks
// (scripts/git-hooks/*). The hooks themselves are POSIX shell — these tests
// exec them in a throwaway git repo with stubbed `agm` / `go` on PATH so the
// hook's control flow is covered without touching a real worktree, invoking the
// real reaper, or running a real build:
//   - Stage 1 (binary rebuild): which source changes rebuild which binaries,
//     the docs/tests/config fast-path, opt-out, branch gate, foreign-repo no-op.
//   - Stage 2 (worktree sweep): which branch triggers a sweep, opt-out,
//     fail-safe exit.
package githooks
