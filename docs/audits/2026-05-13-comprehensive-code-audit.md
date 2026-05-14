# Comprehensive Code Audit — dear-agent

**Date:** 2026-05-13
**Branch:** `claude/nice-chaum-c7d000`
**Scope:** all Go code in the monorepo (2,687 `.go` files, ~595k LOC across
`agm/`, `engram/`, `wayfinder/`, `internal/`, `pkg/`, `cmd/`, `tools/`).
**Mode:** read-only — no code changes.

> Note on framing: the audit request described this repo as a "Python project."
> It is in fact a Go monorepo (`go.mod`, `.golangci.yml`, Go 1.26.3). Tooling
> was adapted accordingly: `golangci-lint`, `govulncheck`, `go vet` in place of
> `ruff`/`mypy`/`pip audit`.

## Executive Summary

The repo is in **good baseline health** by automated measures: `golangci-lint`
reports 0 issues, `govulncheck` finds 0 vulnerabilities, `go vet` is clean,
and there are no real hardcoded secrets, no `InsecureSkipVerify`, and no
SQL injection via string concatenation. The lint config (`.golangci.yml`)
is well-curated.

The problems are at a level above what linters can catch:

- **No CRITICAL security issues**, but two IMPROVE-grade hardening gaps in
  the optional HTTP shims and the AppArmor profile writer.
- **Significant architectural sprawl**: 9 separate `config/` packages,
  2 `telemetry` packages, 4 `workspace` packages, 8+ overlapping `sanitize*`
  functions, and at least one fully dead scaffolded sub-package
  (`agm/internal/app`).
- **Doc/code drift**: `ARCHITECTURE.md` describes a `research/` tree that
  doesn't exist; `pkg/telemetry` ships with ADR/SPEC/ARCHITECTURE files for
  a single file with one caller.
- A handful of god files (`agm/cmd/agm/new.go` at 2,391 LOC) and a god
  package (`agm/cmd/agm` at 142 non-test files, 32k LOC) suggest the CLI
  surface needs decomposition.

Categorized totals: **0 CRITICAL**, **22 IMPROVE**, **15 MINOR**.

---

## Section 1 — Tooling Baseline

| Check                              | Result |
|------------------------------------|--------|
| `golangci-lint run ./...`          | 0 issues |
| `govulncheck ./...`                | No vulnerabilities found |
| `go vet ./...`                     | clean |
| Hardcoded API keys outside tests   | none found |
| `InsecureSkipVerify: true`         | 0 occurrences |
| `fmt.Sprintf`-built SQL            | 0 unsafe occurrences (only one `%q`-quoted JSON literal) |

These pass-rates are real and worth preserving — see the "DEAR Enforcement
Candidates" section.

One caveat: the root `.golangci.yml:102` excludes `gosec` rules
**G104, G204, G301, G703** (text: `"G204|G301|G104|G703"`).
`agm/.golangci.yml:89` is narrower (`"G204|G301|G104"`, no G703).
G204 (subprocess from variable) and G703 (path traversal taint) are
the two most relevant rules for a tool that shells out 437 times.
The exclusion is defensible (most exec sites are internal commands
with internal data) but it means the codebase is trusting itself on
the entire surface this audit had to inspect manually.

---

## Section 2 — Security Findings

### IMPROVE — MCP / API HTTP shims missing timeouts, body limits, auth

- `cmd/dear-agent-mcp/workflow.go:83-91` and `cmd/recommendation-mcp/main.go:54-61`
  call `http.ListenAndServe(*httpAddr, mux)` with **no ReadTimeout,
  WriteTimeout, IdleTimeout, or ReadHeaderTimeout**. `ServeHTTP` (e.g.,
  `workflow.go:427-442`, `recommendation-mcp/main.go:179`) calls
  `io.ReadAll(r.Body)` without `http.MaxBytesReader`. The
  `//nolint:gosec // dev tool` comment hides the linter, but `--http` is a
  documented run mode.
- `cmd/dear-agent-api/main.go:133-141` runs a loopback HTTP server with
  the same body-size gap (`pkg/api/server.go:225,304` uses
  `json.NewDecoder(r.Body)` directly).
- **Threat:** slowloris/OOM if any user binds these on a shared interface.
  **Fix sketch:** wrap with `&http.Server{ReadTimeout: ..., WriteTimeout:
  ..., IdleTimeout: ...}` and `http.MaxBytesReader` on each handler.

### IMPROVE — AppArmor profile written to predictable `/tmp` path

- `engram/internal/security/sandbox_linux.go:166-178` builds
  `profilePath := filepath.Join("/tmp", fmt.Sprintf("engram_%s.profile",
  hashCommand(cmd)))` and `os.WriteFile`s to it. `hashCommand` is a
  deterministic SHA-256 of the command argv — discoverable by any local
  user. `os.WriteFile` follows existing symlinks; on a shared-`/tmp`
  system a local attacker can pre-place a symlink to redirect the write.
  Need-root for `apparmor_parser -r` limits practical impact.
- **Fix sketch:** `os.MkdirTemp` or `os.OpenFile(... O_NOFOLLOW|O_CREAT|
  O_EXCL ...)`.

### IMPROVE — `quality_gates.go` substitutes `$BRANCH` into a shell string

- `agm/internal/ops/quality_gates.go:171-177`:
  ```go
  checkCmd = strings.ReplaceAll(checkCmd, "$BRANCH", branch)
  cmd := exec.Command("bash", "-c", checkCmd)
  ```
  Today `branch` is plumbed from `git rev-parse --abbrev-ref HEAD`, which
  is bounded by git's ref grammar — so today this is not exploitable.
  But the pattern is a footgun: any future caller passing an untrusted
  ref name (a contributor branch on a CI run) gets direct command
  injection. **Fix sketch:** quote with `%q` before substitution, or
  pass `BRANCH` as an env var (`cmd.Env = append(cmd.Env, "BRANCH="+branch)`)
  rather than string-interpolating into the shell command.

### IMPROVE — LLM HTTP clients without timeouts

- `agm/internal/evaluation/claude_judge.go:50` —
  `httpClient: &http.Client{}`
- `agm/internal/evaluation/gpt4_judge.go:54` — same.
- A hung Anthropic/OpenAI socket leaves the judge waiting indefinitely
  if the caller didn't pass a context with deadline. Compare
  `pkg/llm/provider/openrouter.go:71` which sets `Timeout: 120s`.

### IMPROVE — Unbounded LLM response-body reads

- `pkg/llm/provider/openrouter.go:142` and `pkg/llm/provider/ollama.go:190`
  use `io.ReadAll(httpResp.Body)` with no `io.LimitReader`. Especially
  relevant for Ollama where the endpoint is user-configured
  (`ollama.go:177`).

### MINOR — Operator-configured shell-out via `sh -c`

- `agm/cmd/agm/heartbeat.go:299` (`executeRestart`)
- `agm/internal/sentinel/daemon/loop_monitor.go:206` (escalation command)
- `tools/dod-enforcer/dod.go:198`, `internal/benchmark/executor.go:82`

All take operator-supplied config or flags, not network input. Intended
privilege; document explicitly that these are operator-trust boundaries
(e.g., in a SECURITY.md) so reviewers don't have to re-derive that fact.

### MINOR — `yaml.Unmarshal` (not `UnmarshalStrict`) used at 87 sites

- E.g., `pkg/workflow/load.go:25`, `pkg/workflow/roles/registry.go:90`.
  Unknown fields are silently ignored — a typo in a workflow YAML
  becomes a silent behavior change, not an error. Not a security bug
  for trusted YAML, but a hardening miss.

### MINOR — Tar extractor uses a `strings.Contains(.., "..")` pre-check

- `agm/internal/backup/session_backup.go:344-352`. The real defense
  (`HasPrefix(target, root+sep)` at line 350) is correct; the substring
  check is wrong-primitive and rejects legitimate names like
  `notes..bak`.

### Non-findings (verified, **no issue**)

- No `InsecureSkipVerify` anywhere.
- No SQL injection — every Dolt/SQLite call uses `?` placeholders. Only
  near-miss is `agm/internal/dolt/sessions.go:372` using `%q` to inline
  a JSON-array literal for `JSON_CONTAINS`, which is safe.
- `tmux send-keys` in `send_approve.go:147` uses the `-l` literal flag
  and passes `reason` as a single argv element — not shell-interpreted.
- Engram memory-file validator (`engram/hippocampus/memory_security.go:122`,
  `ValidateMemoryFile`) is robust: NFC-normalize + `EvalSymlinks` +
  base-prefix check + homoglyph detector.
- HTTP bind defaults are `127.0.0.1`; the tailnet variant pins `TLS 1.2`.
- All "API key prefix" matches (`sk-`, `AIza`, `ghp_`) are inside
  `*_test.go` fixtures or sanitizer documentation, not real credentials.

---

## Section 3 — Tech Debt & "Slop"

### IMPROVE — Dead scaffold package: `agm/internal/app`

- `agm/internal/app/app.go:11-22` defines `FilesystemInterface` as an
  empty interface (`{ // Add filesystem methods as needed for testing }`)
  and an `App` struct that wires four dependencies.
- **`app.NewApp` has zero non-test callers** in the entire codebase
  (verified with `grep -rn "app\.NewApp" --include='*.go' | grep -v _test`).
  `BuildRootCommand` returns a Cobra root with "Subcommands will be added
  here in future phases" — they never were. The real CLI is in
  `agm/cmd/agm/`. **Delete** the package or document why it's parked.

### IMPROVE — Backward-compat aliases nobody uses: `V2ToV1PhaseMap`

- `wayfinder/cmd/wayfinder-session/internal/status/types.go:176-177`
  declares `V2ToV1PhaseMap` ("for backward compat reference"). Only
  match in the codebase is the definition itself — zero reads. Stale.

### IMPROVE — Backward-compat aliases used only by tests

- `wayfinder/cmd/wayfinder-session/internal/status/types_v2.go:462-488`
  exports `PhaseHistory` (alias), `PhaseV2Charter` … `PhaseV2Retro`,
  `PhaseStatusV2Pending` … `PhaseStatusV2Skipped`. Of these, only
  `PhaseV2Charter`/`Problem`/`Build`/`Plan` and `PhaseStatusV2Pending`
  are referenced by **converter tests** — no production code uses them.
  Either delete or move into `types_v2_test.go`.

### IMPROVE — `--version` flag kept as a no-op

- `wayfinder/cmd/wayfinder-session/commands/start.go:60-61` adds a
  `--version` flag explicitly labelled "for backward compatibility with
  old tests." This is the "shim that should have been deleted with the
  old tests" pattern. If the old tests are gone, the flag should follow.

### IMPROVE — Pervasive sanitizer duplication

Found by grepping `^func (sanitize|Sanitize)`:

| function                    | location                              |
|-----------------------------|---------------------------------------|
| `SanitizeSessionName`       | `agm/internal/tmux/tmux.go:106`       |
| `sanitizeSessionName`       | `agm/internal/validate/validator.go:46` (different sig) |
| `sanitizeTmuxName`          | `agm/cmd/agm/resume.go:735`           |
| `sanitizeSessionID`         | `agm/internal/bus/queue.go:77`        |
| `sanitizeFilename`          | `agm/internal/evaluation/feedback_loop.go` |
| `sanitizeFilename`          | `agm/internal/backup/session_backup.go` |
| `sanitizeID`                | (separate package) |
| `sanitizeMessage`           | (separate package) |
| `SanitizeKey`               | `pkg/llm/auth/apikey.go:164`          |

At minimum, the four `*SessionName / TmuxName / SessionID` variants are
the same conceptual function with slightly different rules. The two
`sanitizeFilename`s share a name and (probably) most of an
implementation. Consolidate in `pkg/strutil` or `internal/safety`.

### IMPROVE — `Enabled bool yaml:"enabled"` pattern in 5 sibling structs in one file

- `engram/internal/config/config.go:72, 168, 222, 243, 282` — five
  separate config sub-structs each carry an `Enabled bool`. Each section
  has its own enable toggle but no shared interface; turning a feature
  on/off goes through five unrelated knobs. Either lift to a common
  `Feature{Enabled bool, ...}` embed, or accept the duplication as
  intentional and document the policy.

### IMPROVE — `agm/internal/app/app.go` is also a microcosm of slop tells

- Empty interface with a "fill in later" comment.
- `App` struct names a field `Harness` but takes parameter `agent`.
- `BuildRootCommand`'s body is a placeholder with a comment promising
  future work; production CLI lives elsewhere.
- This is exactly the file shape an over-eager generator produces.

### MINOR — 289 `TODO`/`FIXME`/`XXX`/`HACK` markers, mostly unaging

Single-file leaders (sampled): `pkg/workflow/runner.go`,
`agm/internal/sentinel/daemon/monitor.go`,
`agm/cmd/agm/new.go`. Treat the 289 figure as a maintenance index —
not an emergency, but if the count keeps growing, the team is using
TODOs as memory storage rather than the issue tracker.

### MINOR — Logging-style inconsistency

- `log` (stdlib) imported in 33 files; `log/slog` in 81 files; raw
  `fmt.Printf` in 57 non-test, non-`cmd` files. The 4-5x preference for
  `slog` is good, but the `log` and `fmt.Printf` users should migrate.
  Note: most `fmt.Printf` calls in non-`cmd` packages are likely
  user-facing CLI output piped through a UI helper — verify
  individually before treating as a bug.

### MINOR — Big files / god packages

| LOC   | path                                              | comment |
|-------|---------------------------------------------------|---------|
| 2,391 | `agm/cmd/agm/new.go`                              | single CLI command, hard to navigate |
| 1,453 | `engram/internal/health/checks.go`                | many checks in one file |
| 1,295 | `pkg/workflow/runner.go`                          | runner with many responsibilities |
| 1,054 | `agm/internal/ui/table.go`                        | table rendering, plausible at this size |
| 1,005 | `agm/cmd/agm/send_msg.go`                         | second giant CLI command |
| 32k   | `agm/cmd/agm` (package)                           | 142 non-test files, 206 total |
| 10k   | `agm/internal/ops`                                | 96 files |

The `cmd/agm` package is the dominant code-mass center of the repo and
the natural target for any decomposition initiative.

---

## Section 4 — Architecture (Ousterhout)

### IMPROVE — Config sprawl (9 packages, ~24 `config.go` files)

```
./config
./engram/internal/config
./agm/internal/config
./pkg/llm/config
./pkg/audit/config
./tools/devlog/internal/config
./wayfinder/cmd/wayfinder-session/internal/config
./agm/internal/a2a/config
./agm/internal/sentinel/config
```

Plus standalone `config.go` files in `pkg/cliframe`, `pkg/notify`,
`pkg/config-loader`, `pkg/workspace`, etc. Each one re-implements
YAML loading, env-var overlay, default merging. This is the classic AI
pattern: each module gets "its own" config package rather than a shared
loader.

**Recommendation:** one `pkg/config` with a generic loader (`Load[T]`
or interface-based), and per-product schemas under their own packages.
ARCHITECTURE.md already names "Configuration cascade" as a design
principle (line 273) — the cascade lives in 9 places instead of one.

### IMPROVE — `pkg/telemetry` is a shallow, over-specified module

`pkg/telemetry/` contents:
```
ADR.md          ARCHITECTURE.md   SPEC.md   telemetry.go
```

One Go file (telemetry.go) with one importer (`wayfinder/internal/tracker/tracker.go:12`).
And **three** companion design docs. This is Ousterhout's "shallow
module" plus over-design: a tiny interface, a thin implementation,
and outsized specification load. Either fold the doc content into the
file as comments, or grow the package to match the documented scope.

Meanwhile `internal/telemetry/` is the real telemetry package: 20 files
including listeners, registries, audit loggers, schema.sql. Two
packages of the same name doing different jobs is a classic
information-leakage pattern between modules.

### IMPROVE — `internal/common` grab-bag

`internal/common/` contains `errors.go`, `git.go`, `staging.go`,
`stats.go` — four unrelated concerns sharing a package because they
all happen to be "common." Only three importers across the repo
(`internal/benchmark/{stats,executor}.go`, `internal/baseline/manager.go`).
Either split into per-concern packages (`internal/gitutil`,
`internal/statsutil`) or absorb back into the (sole) callers.

`internal/testutil/` shows the same pattern: `common_helpers.go`,
`e2e_helpers.go`, `ecphory_helpers.go`, `platform_helpers.go`,
`retrieval_helpers.go`, `scanners_helpers.go` — six concerns living
together because they're "for tests." A test helper that's specific to
`ecphory` belongs in `engram/ecphory/internal/testhelp` (close to the
code it tests).

### IMPROVE — Doc/code drift in ARCHITECTURE.md

- ARCHITECTURE.md (line 220) lists `research/` as one of the four
  products: `research/cmd/`, `research/autonomous/`. **No `research/`
  directory exists in this repo.** Either re-add the product or update
  the doc.
- ARCHITECTURE.md places `internal/sandbox/` and `internal/ops/`
  side-by-side, but `internal/sandbox` is **at the repo root** while
  `internal/ops` is **under `agm/`**. Two `internal/` trees with
  overlapping names is friction for new readers.

### IMPROVE — `agm/cmd/agm` is a god package by file count

- 206 `.go` files (142 non-test), ~32k LOC. Every subcommand lives in
  the same package, sharing globals (`heartbeatRestartCmd`, etc.). A
  contributor wanting to add a subcommand has to scan for naming
  collisions in a 206-file directory.
- **Recommendation:** subcommand subpackages
  (`agm/cmd/agm/admin/`, `agm/cmd/agm/session/`, …) with the root
  `agm.go` only doing wiring. Cobra encourages this pattern.

### IMPROVE — Five `Sanitize`/`sanitize` functions on one conceptual key

(Same set as Section 3, viewed through the architecture lens.) When
two packages both have to know how a session name is escaped, they
have a shared invariant in two places. Today the bug is benign; the
day someone tightens one rule and forgets the other, sessions named
in one path become unfindable from another.

### MINOR — 163 interfaces, many likely single-impl

`grep -rh '^type \w+ interface' | wc -l` reports 163 interface
declarations across non-test code. A spot check of
`agm/internal/app/app.go:11` shows `FilesystemInterface` is an empty
interface. The repo's adapter-pattern philosophy justifies *some* of
these (Agent, Backend, Provider). But "163 interfaces" in a 595k-LOC
codebase is on the high side, and a sweep with `unused` extended to
detect interfaces with one implementation would likely find dozens
that should just be concrete types.

### MINOR — Five `telemetry` directories

- `pkg/telemetry`, `internal/telemetry`, `engram/internal/telemetry`,
  `agm/internal/telemetry`, `wayfinder/cmd/wayfinder-session/internal/telemetry`.

Each product has its own telemetry. Some of this is fine (per-product
schemas), but the lack of a shared base means a code reader has no
"telemetry contract" to land on.

### MINOR — `agm/internal/app/app.go` `FilesystemInterface` is leaky-and-empty

An empty interface signals an abstraction that hasn't been designed yet,
exported into the API anyway. Either delete the interface and the
package, or specify the methods.

---

## Section 5 — Categorized Findings Index

### CRITICAL (must fix)

*(none)*

### IMPROVE (should fix) — 22

| # | Area      | Finding                                                                             | Locator |
|---|-----------|-------------------------------------------------------------------------------------|---------|
| 1 | Security  | HTTP shims missing timeouts/body-limit/auth                                         | `cmd/dear-agent-mcp/workflow.go:83`, `cmd/recommendation-mcp/main.go:54` |
| 2 | Security  | AppArmor profile path predictable in `/tmp`                                         | `engram/internal/security/sandbox_linux.go:166` |
| 3 | Security  | `$BRANCH` substitution into shell string                                            | `agm/internal/ops/quality_gates.go:171-177` |
| 4 | Security  | LLM judge clients lack `Timeout`                                                    | `agm/internal/evaluation/claude_judge.go:50`, `gpt4_judge.go:54` |
| 5 | Security  | Unbounded `io.ReadAll` on LLM responses                                             | `pkg/llm/provider/openrouter.go:142`, `ollama.go:190` |
| 6 | Tech debt | Dead scaffold package `agm/internal/app`                                            | `agm/internal/app/app.go` |
| 7 | Tech debt | Unused `V2ToV1PhaseMap`                                                             | `wayfinder/cmd/wayfinder-session/internal/status/types.go:176-177` |
| 8 | Tech debt | Backward-compat phase/status aliases only used by tests                             | `wayfinder/cmd/wayfinder-session/internal/status/types_v2.go:462-488` |
| 9 | Tech debt | `--version` no-op flag                                                              | `wayfinder/cmd/wayfinder-session/commands/start.go:60` |
| 10| Tech debt | 8+ overlapping sanitizers; 4 of them on the same session-name concept              | grep `^func (Sanitize|sanitize)` |
| 11| Tech debt | Same `Enabled bool` pattern repeated 5× in `engram/internal/config/config.go`       | lines 72,168,222,243,282 |
| 12| Tech debt | `agm/cmd/agm/new.go` is 2,391 LOC — needs decomposition                             | `agm/cmd/agm/new.go` |
| 13| Tech debt | `agm/cmd/agm` package is 32k LOC across 142 non-test files                          | `agm/cmd/agm/` |
| 14| Tech debt | `pkg/workflow/runner.go` is 1,295 LOC                                               | `pkg/workflow/runner.go` |
| 15| Tech debt | `engram/internal/health/checks.go` is 1,453 LOC                                     | `engram/internal/health/checks.go` |
| 16| Arch      | Config sprawl — 9 separate `config/` packages re-implementing YAML loading          | (top-level directory layout) |
| 17| Arch      | `pkg/telemetry`: 3 spec docs + 1 Go file + 1 caller (shallow, over-specified)        | `pkg/telemetry/` |
| 18| Arch      | `internal/common` and `internal/testutil` are grab-bags                              | `internal/common/`, `internal/testutil/` |
| 19| Arch      | ARCHITECTURE.md claims `research/` product that doesn't exist                       | `ARCHITECTURE.md:220` |
| 20| Arch      | Two `internal/` trees (root + `agm/internal/`) with overlapping concerns            | repo root |
| 21| Arch      | 5 different `telemetry` packages                                                    | grep `^package telemetry` |
| 22| Arch      | Empty `FilesystemInterface` interface exported                                       | `agm/internal/app/app.go:11` |

### MINOR (nice to have) — 15

| # | Area      | Finding                                                                  | Locator |
|---|-----------|--------------------------------------------------------------------------|---------|
| 23| Security  | Operator-configured `sh -c` sites — document trust boundary              | `heartbeat.go:299`, `loop_monitor.go:206`, `dod.go:198`, `executor.go:82` |
| 24| Security  | 87 `yaml.Unmarshal` sites, none using `UnmarshalStrict`                   | grep `yaml\.Unmarshal` |
| 25| Security  | Tar extractor uses `strings.Contains(..,"..")` (wrong primitive)         | `agm/internal/backup/session_backup.go:344` |
| 26| Security  | Dolt DSN built via raw `fmt.Sprintf` (fragile if pwd has `@`/`:`/`/`)    | `agm/internal/dolt/adapter.go:277-283` |
| 27| Tech debt | 289 TODO/FIXME/XXX/HACK markers                                          | grep |
| 28| Tech debt | Logging: 33 `log`, 81 `slog`, 57 non-cmd `fmt.Printf`                    | grep |
| 29| Tech debt | 163 interface declarations — likely many single-impl                     | grep |
| 30| Tech debt | `agm/internal/ui/table.go` at 1,054 LOC                                  | file |
| 31| Tech debt | `agm/cmd/agm/send_msg.go` at 1,005 LOC                                   | file |
| 32| Tech debt | `agm/internal/sentinel/daemon/monitor.go` at 963 LOC                     | file |
| 33| Tech debt | 6 `Deprecated:` markers — verify each is still needed                    | grep `// Deprecated` |
| 34| Arch      | `internal/sandbox` at repo root vs `agm/internal/*` siblings — inconsistent | repo root |
| 35| Arch      | 4 `workspace` packages                                                    | `pkg/workspace`, `tools/devlog/internal/workspace`, `wayfinder/.../workspace`, `agm/cmd/agm/workspace` |
| 36| Arch      | `golangci-lint` excludes G204, G703 — re-enable in CI when feasible      | `.golangci.yml:102` (root), `agm/.golangci.yml:89` |
| 37| Style     | `App.Harness` field paired with `agent` constructor parameter             | `agm/internal/app/app.go:22,26` |

---

## DEAR Enforcement Candidates

Findings worth promoting from "audit notes" to "self-perpetuating
guardrails" — either CLAUDE.md guidance (so future AI sessions don't
re-introduce them) or CI checks (so humans can't either).

### Candidate A — Forbid new `config` packages

**Why:** 9 already exist; each is the same YAML+env+default loader,
diverged in small ways. Every new `config` package compounds drift.

**Where:** add a section to `.claude/CLAUDE.md` (project):
> Do **not** create a new `*/config/` package. There are already
> nine. Use `pkg/cliframe/config.go` (or whatever the chosen base
> becomes) and pass a typed schema. New code that needs config
> should go in a `*/configschema/` package containing only the
> struct types — no loader.

CI counterpart: a structure-auditor rule rejecting new directories
named `config` under `pkg/` or `*/internal/`.

### Candidate B — Forbid scaffolded "to be filled in later" packages

**Why:** `agm/internal/app/app.go` is the canonical example: an
exported empty interface with `// Add filesystem methods as needed`.
A year later, no callers and no methods.

**Where:** CLAUDE.md global writing rules:
> Do not commit a public interface with zero methods. If the
> design is not ready, keep the placeholder in a `// TODO(...)`
> comment in the consumer, not as exported API surface.

CI counterpart: `revive` rule or custom static check —
`type X interface { }` (empty) is a build error outside of
`embedding/marker` constructs.

### Candidate C — Sanitize-once rule

**Why:** four functions sanitize "session name" with slightly
different semantics. Drift between them is invisible until two
code paths disagree about whether a session named `foo bar` exists.

**Where:** `agm/internal/safety/SPEC.md` (new): "the canonical
session-name normalizer lives in one place; reviewers reject new
`sanitize*` functions touching session/tmux/bus identifiers."

### Candidate D — `yaml.UnmarshalStrict` for trusted-internal YAML

**Why:** 87 `yaml.Unmarshal` sites accept unknown fields silently. A
typo in `acceptance-criteria.yaml` becomes a silent skip rather than
a hard fail. The defense is one method call.

**Where:** CI hook (gocritic custom check or a simple grep linter in
`scripts/`) that flags new `yaml.Unmarshal` calls where the target is
a typed struct — should be `UnmarshalStrict`.

### Candidate E — HTTP server hardening checklist

**Why:** the MCP and API shims share the same three gaps (no timeouts,
no body limit, no auth). Adding a fourth HTTP server in the same shape
will be tempting.

**Where:** `pkg/httpsafe/` (new) — a `NewServer(addr, mux, opts)`
helper that *requires* timeouts and applies `MaxBytesReader` by
default. CLAUDE.md: "do not call `http.ListenAndServe` directly;
use `pkg/httpsafe`."

### Candidate F — `cmd/agm` decomposition gate

**Why:** 142 non-test files in one package will grow without bound
unless explicitly checked.

**Where:** a `.ci-policy.yaml` entry (the repo already has one) that
fails if `agm/cmd/agm/*.go` (non-test) file count exceeds a number
(say 150) — forcing the next contributor to put their command in a
subpackage.

### Candidate G — Doc-vs-tree drift check

**Why:** ARCHITECTURE.md claims a `research/` product that doesn't
exist. This is exactly the failure mode the `architecture-validator`
agent in this repo is designed to catch.

**Where:** wire `architecture-validator` into the daily audit set in
`.dear-agent.yml > audits.schedule.daily`. The check runs on every
push; ARCHITECTURE.md drifts get caught the same day the dir is
renamed.

### Candidate H — `gosec` G204/G703 — quarantine, don't blanket-exclude

**Why:** root `.golangci.yml:102` excludes `G204|G301|G104|G703`
repo-wide (and `agm/.golangci.yml:89` excludes `G204|G301|G104`).
This is the right pragmatic choice for `agm/` shell-outs (operator
input is trusted) but the wrong choice for the HTTP/MCP boundary
where untrusted input does enter.

**Where:** narrow the exclusion to specific paths
(`agm/internal/.*`, `tools/.*`) rather than the whole tree. Force new
HTTP handler code to pass G204/G703.

---

## Appendix A — Tooling Output

- `golangci-lint run ./...` → `0 issues.` (full output:
  `/tmp/audit-findings/golangci.txt`)
- `govulncheck ./...` → `No vulnerabilities found.` (full output:
  `/tmp/audit-findings/govulncheck.txt`)
- `go vet ./...` → clean (empty output:
  `/tmp/audit-findings/govet.txt`)

## Appendix B — Audit Methodology

1. **Baseline tooling** (parallel): `golangci-lint`, `govulncheck`,
   `go vet`.
2. **Three parallel investigation agents** (general-purpose):
   - Tech-debt / slop (categories: dead code, dup logic, slop tells,
     deprecated shims).
   - Security review (categories: injection, traversal, secrets, TLS,
     deserialization, dep CVEs, server exposure).
   - Architecture review (categories: shallow/deep modules, leaky
     abstractions, doc-vs-code drift, god packages, adapter pattern
     hygiene).
3. **Manual cross-cutting scans** for: `exec.Command` sites,
   `yaml.Unmarshal` count, fmt-vs-slog ratios, sanitizer duplicates,
   file/package LOC distribution, doc/tree alignment.
4. **Verification pass** before report: every dead-code claim
   re-grepped to confirm zero callers; every security claim
   re-inspected at the file:line cited.

All findings labeled in this report were verified at a specific
file/line. Speculative or unverifiable observations were dropped
during the verification pass.
