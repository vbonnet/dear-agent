# Pi Harness

<!-- Last audited at: 2026-07-21 against Pi 0.81.1 -->

AGM supports [Pi](https://github.com/earendil-works/pi) as the canonical
`pi-cli` harness. `pi` is accepted as an input alias and normalized before
storage. Claude Code remains the reference harness, but Pi participates in the
same active lifecycle, model-family, permission, hook, MCP, Engram, Wayfinder,
marketplace, config-directory, quota, and BDD parity contracts.

## Install and create

```bash
agm admin install-harness pi
agm session new pi-work --harness pi-cli -C /absolute/project/path
agm session new pi-plan --harness pi-cli --permission-mode plan -C /absolute/project/path
agm send msg pi-work --prompt "Inspect the failing test"
agm session resume pi-work
```

AGM installs the canonical npm package
`@earendil-works/pi-coding-agent` and verifies the `pi` executable. Provider
authentication remains Pi-owned; AGM does not copy, inspect, or translate Pi
credentials.

## Native identity and storage

New sessions use AGM's session ID as Pi's exact `--session-id`. AGM creates an
owner-only session directory and launches Pi with an explicit `--session-dir`.
The manifest persists:

```yaml
pi:
  session_id: <exact-native-id>
  session_dir: <absolute-private-directory>
  transcript_path: <exact-jsonl-path-when-created>
```

Resume, history, import, export, Engram indexing, and quota collection validate
the JSONL header. Discovery is bounded, does not follow symlinks, rejects
duplicate IDs, and never selects a transcript because it is newest.

To register an existing native session, use `agm session import --harness
pi-cli --session-id <native-id>`. AGM discovers Pi's default project-grouped
session tree, validates the header and absolute working directory, and copies
the transcript into AGM-private storage before registering it. Import preserves
the latest native provider/model when the transcript establishes one; otherwise
AGM leaves the model override empty so Pi resumes the saved native selection.
Before Pi has persisted a transcript (which it defers until the first assistant
message), no saved selection exists; cold resume therefore preserves the
configured model or uses the Pi harness default.

## Permission model

Pi project trust and tool authorization are separate decisions. AGM launches
Pi with explicit project approval only after selecting the requested working
directory. It also installs a dependency-free authorization extension in
AGM-owned private storage and passes it with `--extension`; repository files
cannot replace that mandatory extension.

Resolved allowlists are written atomically to a per-session owner-only policy
file beside the managed extension. The launch command passes only that path,
which avoids macOS's bounded terminal input queue; a missing or malformed file
keeps the managed status out of `ready` and blocks tool calls.

| Mode | Active native tools | Unmatched tool call |
|---|---|---|
| `plan` | `read`, `grep`, `find`, `ls` | Mutating calls are blocked before allowlist evaluation |
| `default` | all built-in tools | Ask in an interactive UI; block without one |
| `auto` | all built-in tools | Allow after repository guardrails run |

The extension maps Pi's `bash`, `read`, `edit`, `write`, `grep`, `find`, and
`ls` calls to AGM permission categories. Extension tools receive a stable
PascalCase category (for example, `plugin_deploy` becomes `PluginDeploy`) so
they can be pre-approved without weakening plan mode. Patterns are anchored;
wildcards must be explicit. Bash calls containing unquoted command chaining,
redirection, or command substitution are never pre-approved by an allowlist;
default mode asks interactively and a non-interactive caller fails closed.
Runtime transitions use `agm send mode <mode> <session-name>`,
which sends the managed `/agm-mode plan|default|auto` command. Model transitions use
`/agm-model provider/model` and are persisted only after AGM observes the
managed transition result.

The stable footer token `AGM <mode>/ready <launch-id>` is AGM's send-safety
boundary. Create and cold resume require the current launch ID before they can
report readiness, so an older footer retained in tmux history cannot authorize
a new process. Every cold-resume entry point also proves Pi-specific process
identity before attaching. This recognizes the canonical npm package's Node
entrypoint, including installations beneath npm prefixes containing spaces,
and permits supported Node runtime flags and whitespace-bearing preload paths
without treating an option value, unrelated `node` process, or later Pi-looking
argument as Pi. AGM reads lossless process argv from the host OS for this
decision; flattened tmux or `ps` display text is not liveness evidence.
When Pi has exited, AGM relaunches only in a positively classified bare
shell and rejects retained pane start-command metadata or any other foreground
process. Ctrl-C and root shutdown cancel both identity and pane-classification
scans before command delivery or attachment.
Routine sends use the latest managed mode/state; `working`,
permission, model-selection, and other overlays are not ready.

## Repository instructions, skills, and hooks

Pi reads the repository's root `AGENTS.md` directly. `.pi/settings.json`
discovers the living AGM and Wayfinder skill trees; it does not copy those
skills. `.pi/hooks.json` maps native Pi events to the repository's shared
lifecycle and tool guardrails. The wrappers under `.pi/guardrails/` reuse the living
OpenCode shell policies so rule fixes have one implementation owner.

Repository hooks load only from the explicitly approved working directory.
`PreToolUse` failures block the native call before auto or allowlist decisions.
Every invocation receives the shared event name, native Pi session ID, approved
working directory, loop state, and native event payload. Structured hook
decisions are honored even when the command exits successfully. A hook
execution error, timeout, signal, or nonzero exit status takes precedence over
partial stderr and advisory context in its fail-closed diagnostic. In
particular,
a blocked `UserPromptSubmit` is consumed before the model sees it, a blocked
`PreCompact` cancels compaction, and a blocking `Stop` result is delivered back
to Pi as a follow-up user turn so the shared bounded guardrail-feedback loop can
finish its remediation. Completion of Pi's conventional `subagent` extension
tool projects `SubagentStop`; because the tool runs isolated child Pi processes,
blocking remediation returns to the parent turn. Until Beads exposes a
Pi-specific hook entrypoint, lifecycle events use its
behaviorally equivalent `codex-hook` adapter with Dolt auto-commit enabled,
matching the other non-Claude hook manifests.

Wayfinder is available through Pi's native skill discovery and the
`wayfinder-session` CLI. Its status and temporal artifacts remain
harness-neutral.

## Models, usage, and limitations

Pi exposes the shared `fable`, `opus`, `sonnet`, and `haiku` tiers plus
`gpt-frontier`, `gpt`, and `gpt-fast`. Its aliases cover every active AGM model family. Anthropic, OpenAI, and Gemini
aliases use their native Pi providers; GLM, DeepSeek, Nemotron, and Qwen use
their canonical OpenRouter routes. Provider availability still depends on the
models and authentication configured in Pi.

AGM reads Pi's native JSONL usage and provider-reported cost. It uses the latest
assistant prompt footprint for context, the audited Pi 0.81.1 model-catalog
window for the recorded direct or nested OpenRouter model route, and sums
native cost records for the session. Route identity matters: Pi's nested
OpenRouter OpenAI entries can expose a different context window than the same
model through direct OpenAI. Provider and model IDs remain case-sensitive and
separate; a custom raw model ID may repeat its provider prefix without AGM
collapsing that opaque segment.
When `PI_CODING_AGENT_DIR` selects a custom Pi configuration, AGM validates
the directory before creating or importing a session, records the absolute
non-symlink path with Pi's native identity, and quotes it into both create and
cold-resume commands. This explicit forwarding is required for sessions
started through an already-running tmux server, whose environment may predate
the invoking shell. Sessions without an explicit directory continue to use
Pi's native default discovery because AGM explicitly clears any stale copy of
the variable inherited from the tmux server.
For models declared in Pi's `models.json`, AGM reads only the bounded model ID,
`contextWindow`, and `modelOverrides` data: it never evaluates credential or
command fields. A custom model with an omitted window uses Pi's 128000-token
default. Mathematically integral JSON decimal and exponent spellings are
accepted exactly like Pi, while explicit null, fractional, or otherwise invalid
windows are rejected without floating-point rounding. An override
matching the exact provider-qualified route recorded by Pi applies even when
the provider or model is newer than AGM's static tables; unqualified orphan
overrides remain conservative. Malformed, oversized, symlinked,
group- or other-writable, ambiguous, or invalid catalog data retains AGM's
conservative 200000-token fallback. An unqualified legacy model is accepted
only when exactly one provider matches; a second match remains ambiguous even
when both windows are equal or one declaration is invalid.
Pi does not expose a provider quota/rate-limit API, so those fields are
reported as unavailable rather than populated with Claude-specific values.

Pi 0.81.1 adds native retry lifecycle events for compaction and branch
summarization and defers background model-catalog refresh until after
interactive startup. AGM does not reinterpret native retry output as a
readiness transition: the latest managed `AGM <mode>/<state> <launch-id>`
status remains the sole readiness authority.

Pi does not require a background server. Archive stops the managed tmux
process while preserving its private native transcript. Deleting transcript
data remains an explicit storage operation, not an archive side effect.

## Troubleshooting

- `pi binary not found`: run `agm admin install-harness pi` and confirm `pi
  --version` is visible in AGM's `PATH`.
- `no provider models available`: configure a provider through Pi; AGM will not
  read credential files to diagnose this.
- custom provider missing only under AGM: confirm `PI_CODING_AGENT_DIR` names
  an existing non-symlink directory when the session is created; AGM persists
  that path for cold resume instead of relying on tmux's global environment.
- `Pi transcript not found`: inspect the manifest's `pi` block; do not replace
  it with the most recent JSONL path.
- no `AGM <mode>/ready <launch-id>` footer: treat the session as not sendable and inspect
  the managed extension load error in the pane.
- project hook rejection: run the named `.pi/guardrails` wrapper from the same
  project directory and preserve the fail-closed behavior while fixing it.

## Verification boundaries

Pure tests cover command construction, identity, parsing, permission decisions,
hook execution, persistence, and parity matrices. Contract tests require the
real `pi` binary. A provider-authenticated prompt/response test must be skipped
with an explicit reason when this machine has no configured Pi models; startup,
extension loading, mode transitions, project skills, and fail-closed tool
guardrails remain locally verifiable without provider credentials.
