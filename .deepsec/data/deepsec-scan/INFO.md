# dear-agent

## What this codebase does

A Go meta-harness for AI coding agents. Three main components: **AGM**
(`agm/` — tmux sessions, named recurring "loops" stored in SQLite, plugin
orchestration), **Engram** (`engram/` — cue-based persistent memory across
sessions), **Wayfinder** (`wayfinder/` — YAML-DAG workflow engine with
validation gates). ~50 binaries under `cmd/` (`workflow-*`, `dear-agent-*`,
MCP servers). Substrate is tmux + SQLite + files; users are developers
running it on their own dev machine, but plugins/loops execute arbitrary
shell, so the local trust boundary is real.

## Auth shape

- No web auth surface. AGM is a local CLI; the dear-agent-api / MCP
  servers bind to localhost.
- External-tool credentials are pulled from env (`ANTHROPIC_AUTH_TOKEN`,
  `gcloud auth application-default print-access-token`, GitHub via `gh`).
- Plugin executor (`engram/internal/plugin/executor.go`) is the main trust
  boundary: it runs plugin-declared commands inside a sandbox
  (`engram/internal/scratchpad/sandbox.go` uses docker;
  `engram/internal/security/sandbox_linux.go` uses apparmor).

## Threat model

Primary risk is **local privilege/scope escalation via the harness**: a
malicious loop spec, plugin manifest, workflow YAML, or MCP-supplied
argument tricks AGM into running attacker-chosen shell with the user's
ambient creds (gh token, gcloud ADC, AWS profile, repo write access).
Secondary risk is **substrate poisoning**: SQL injection into SQLite
stores (`~/.agm/loops.db`, engram memory DBs) corrupts cross-session
memory or hides activity from audit. Workflow/plugin YAML is the most
dangerous input — it's project-scoped and easy to commit.

## Project-specific patterns to flag

- **`exec.Command` / `exec.CommandContext` with non-literal argv**: AGM's
  whole job is running shell. Hardcoded program + hardcoded args = safe
  (already marked `//nolint:gosec G204`). Flag any callsite where the
  program or any argv element flows from a YAML/JSON field, MCP arg,
  loop spec, or workflow step without an allowlist.
- **SQLite string-interpolated queries**: many `*.db` stores (loops,
  engram, audit, eventbus). Flag `fmt.Sprintf` / `"+"` / `strings.Replace`
  built into a SQL string; canonical form is `?`-placeholders.
- **Sandbox escape**: in `engram/internal/scratchpad/sandbox.go` and
  `engram/internal/plugin/executor.go`, flag any path that builds a
  `docker run` argv from plugin/manifest fields without validating
  volume mounts (`-v`), `--privileged`, `--cap-add`, `--network host`,
  or `--pid host`.
- **Path traversal via worktree/session roots**: AGM creates worktrees
  and session dirs from user/plugin-supplied names. Flag any
  `filepath.Join(root, untrustedName)` that doesn't reject `..`,
  absolute paths, or symlink targets escaping `root`.
- **Hook / signal command injection**: `pkg/stophook`, `pkg/signals`,
  `engram/hooks*`, agm-hooks read JSON from stdin and may shell out.
  Flag any `sh -c` / `bash -c` with interpolated stdin fields.

## Known false-positives

- `exec.Command("git", ...)`, `("docker", ...)`, `("tmux", ...)`,
  `("gh", ...)` with **all-literal args**: these are deliberate and
  often already carry a `//nolint:gosec G204` + "Security:" comment.
- Anything under `**/*_test.go`, `testutil/`, `tests/`,
  `internal/benchmark/`, `samples/`, `codegen/` fixtures —
  hardcoded test creds and dummy tokens are expected.
- `internal/sandbox/` and `engram/internal/security/` are the security
  primitives themselves; they intentionally invoke `apparmor_parser`,
  `docker`, etc. — flag only if the invocation surfaces external input.
- `cmd/workflow-codemod`, `cmd/workflow-migrate` invoke `go run` /
  `gofmt` over repo paths; argv is from CLI flags the user typed, not
  remote input.
