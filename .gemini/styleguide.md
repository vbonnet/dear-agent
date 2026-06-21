# dear-agent — Gemini Code Assist style guide

Rules for Gemini Code Assist to apply when reviewing pull requests in this
repository. These mirror the engineering principles in `.claude/CLAUDE.md`;
treat that file and `ARCHITECTURE.md` as the source of truth if they conflict.

## Language & toolchain

- **Go is the default and only sanctioned language.** Flag any new Python for
  code we own and ship — it is not permitted. Rust or TypeScript are allowed
  only with an explicit, stated justification in the PR.
- Code must pass `make preflight` (vet + build + lint). Call out anything that
  would fail `go vet` or `golangci-lint`.
- Prefer the standard library and existing internal packages over new
  third-party dependencies; question new modules in `go.mod`.

## Correctness & safety

- Errors must be handled or explicitly wrapped with `fmt.Errorf("...: %w", err)`;
  flag swallowed errors and bare `_ =` discards of error returns.
- Flag data races, unguarded shared state, and goroutines without a clear
  lifecycle (no context/cancellation or wait).
- Flag missing `context.Context` propagation on I/O and subprocess calls.
- Security: flag command injection, unvalidated paths, SSRF, and secrets or
  tokens written to logs. `gosec`-class findings (e.g. G704) are blocking.
- Subprocess/network calls should be bounded by a timeout and must not hang on
  interactive prompts (`GIT_TERMINAL_PROMPT=0` for git over HTTPS).

## Scope & reviewability

- **One PR, one scoped change.** Flag scope creep — unrelated fixes bundled into
  a feature PR should be split out.
- Prefer small, focused diffs. Note when a change mixes refactor + behavior
  change in a way that makes review hard.

## Tests & docs

- New behavior needs tests; bug fixes need a regression test. Flag untested
  logic in non-trivial changes.
- **Living docs live next to the code** (package `doc.go` / `README` /
  `ARCHITECTURE.md` / `docs/adr/`). Flag temporal artifacts (design docs, plans,
  audits, DEAR retros, research) committed into this repo — those belong in the
  knowledge base, not here.
- If a change alters documented behavior, the co-located doc should be updated
  in the same PR.

## Style

- Follow standard Go formatting (`gofmt`) and idiom; defer to existing patterns
  in the surrounding package over personal preference.
- Comments explain *why*, not *what*. Flag redundant restate-the-code comments.
- Commit messages follow Conventional Commits (`feat(scope): ...`,
  `fix(scope): ...`).

## Tone

Be concise and specific. Cite `file:line`. Prefer actionable suggestions over
general observations. Skip nits already handled by `gofmt`/`golangci-lint`.
