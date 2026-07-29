# Agent Harness and Model Parity Specification

<!-- Last audited at: 2026-07-22 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** AGM harness adapters, active harness parity, and model-family routing

## Overview

`agm/internal/agent` owns harness identity, capabilities, model routing, and
the adapter contract for harness-native lifecycle primitives. Cross-surface
create, kill, archive, and message-delivery ordering belongs to
`agm/internal/ops`; the root CLI retains the focused transactional resume
workflow. The agent package also owns the model alias registry used by CLI
creation flows, OpenCode model selection, and cross-harness tier aliases.

Claude Code is the reference implementation. Codex CLI, AGY, OpenCode, and Pi are
active parity harnesses. Gemini CLI is accepted only for deprecated
compatibility.

## EARS Requirements

### Harness Parity

**AGP-01** When AGM enumerates active harnesses, the system shall return `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli` in canonical parity order.

**AGP-02** When AGM validates a deprecated compatibility harness, the system shall accept `gemini-cli` without adding it to the active parity set.

**AGP-03** When a user supplies a legacy Antigravity harness spelling, the system shall normalize `antigravity` and `agy-cli` to `agy` before validation, factory lookup, or model lookup.

**AGP-04** When AGM resolves an active harness adapter, the system shall return a concrete adapter whose `Name()` matches the normalized harness identifier.

**AGP-05** When AGM builds OpenCode model choices, the system shall include model aliases from every active harness and the OpenRouter-compatible model family source while excluding deprecated-only Gemini CLI aliases.

**AGP-13** When AGM validates active harness adapter conformance, the system shall run the same non-I/O adapter contract across every active harness and require canonical identity, non-empty version, sane capabilities, default model coverage, test model coverage, model aliases, and model family coverage.

**AGP-54** When AGM resolves the `codex-cli` harness, the system shall use `CodexCLIAdapter` and shall not route Codex terminal status through the OpenAI API adapter.

**AGP-56** When a CLI or MCP surface exposes create, kill, archive, or message-delivery behavior, the surface shall delegate lifecycle ordering, rollback, and verified postconditions to `agm/internal/ops` rather than implement a competing surface-specific lifecycle.

### Model Families

**AGP-06** When AGM enumerates supported model families, the system shall return Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen in priority order.

**AGP-07** When AGM maps a supported model family to a default model, the system shall return at least one syntactically safe model identifier for each family.

**AGP-08** When AGM exposes OpenRouter-compatible model aliases, the system shall include GLM 5.2, DeepSeek V4, Nemotron, and Qwen family aliases in that priority order.

**AGP-18** When AGM resolves Nemotron or Qwen family defaults, the system shall use the canonical OpenRouter slugs `nvidia/nemotron-3-ultra-550b-a55b` and `qwen/qwen3.6-max-preview`.

**AGP-09** When AGM validates an unknown future model identifier, the system shall allow syntactically safe identifiers and reject shell metacharacters before any value can be interpolated into a tmux command.

### Cross-Harness Tier Aliases

**AGP-10** When a user selects a Claude tier alias for another active harness, the system shall resolve the tier to that harness's closest native model alias.

**AGP-11** When a user selects an active harness's test mode, the system shall choose a low-cost test model for `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`.

### Pi Native Session Lifecycle

**AGP-39** When AGM creates a Pi session, the system shall invoke `pi` with AGM's session ID as Pi's exact `--session-id`, an AGM-owned private `--session-dir`, the managed authorization `--extension`, explicit project approval, and the resolved model and active-tool set.

**AGP-40** When AGM resumes a Pi session, the system shall use the persisted Pi session ID and session directory and shall reject unsafe or ambiguous native identity rather than substitute newest-file discovery.

**AGP-41** When AGM sends a prompt, mode transition, or model transition to a running Pi session, the system shall wait for the managed `AGM <mode>/ready` status and shall route the operation through Pi's native composer or AGM extension commands.

**AGP-42** When AGM reads, imports, exports, or resumes Pi history, the system shall resolve the transcript whose header contains the exact persisted Pi session ID, bound every file and tree walk, reject duplicate native IDs, and preserve the identity in both manifest and Dolt metadata.

**AGP-43** When AGM evaluates Pi capabilities, the system shall expose native AGENTS.md loading, project skills, exact model routing, resumable JSONL sessions, managed hooks, and bridged tool authorization without claiming native quota or rate-limit telemetry that Pi does not provide.

**AGP-44** When AGM exports Pi history through the shared Agent message model, the system shall include user and assistant text without mislabeling tool results as assistant speech, while native export shall preserve the original JSONL.

**AGP-45** When OpenCode resolves an alias aggregated from multiple harness catalogs, the system shall use stable explicit precedence and prefer provider-qualified routes instead of depending on map iteration order.

**AGP-46** When the Pi adapter imports a native transcript, the system shall preserve an established provider-qualified model and shall not fabricate a default override when native model provenance is absent.

**AGP-47** When AGM cold-resumes a Pi session before Pi has persisted a native transcript, the system shall preserve the configured model or use the Pi harness default; when a persisted transcript exists without model provenance, the system shall omit `--model` so Pi retains native session truth.

**AGP-48** When any Pi lifecycle entry point finds an existing tmux session during cold resume, the system shall preserve it only after proving Pi-specific process identity, including the canonical npm Node entrypoint without accepting a generic `node` process, otherwise require a positively classified restartable shell before command delivery, and fail without pane mutation when another harness, a non-shell foreground, a disappeared pane, or a liveness-scan error is observed.

**AGP-49** When AGM launches Pi for create or cold resume, the system shall generate a unique launch ID, pass it through the canonical command and managed extension, and require readiness carrying that exact ID before registration, attachment, or success.

**AGP-50** When the root AGM command classifies an existing Pi pane during resume, the system shall propagate the command context through Pi identity and generic pane-liveness scans and shall return cancellation before command delivery, attachment, or metadata mutation.

**AGP-51** When the Pi adapter resumes a session, the system shall classify exact process liveness before validating or materializing relaunch configuration; a proven live Pi process shall remain attachable when its persisted coding-agent directory is no longer available, while a required relaunch shall validate configuration and permission artifacts before creating a new tmux session.

**AGP-52** When the Pi adapter creates a session, an explicitly present `SessionContext.Environment` coding-agent directory, including an empty native-default value, shall take precedence over the adapter process environment; create and import shall persist coding-agent directory presence even for the native default, and resume shall use the caller environment only for metadata that lacks both a persisted directory and that marker.

**AGP-58** When an adapter or private harness handoff pastes a TUI command, shell launch, or set-directory command into tmux, the system shall reject invalid UTF-8 and terminal control characters in every caller-derived or generated interpolated value before invoking the tmux delivery boundary.

### AGY Model and Adapter Lifecycle

**AGP-20** When AGM resolves an AGY model alias or accepts an AGY public model label, the system shall pass an exact label exposed by the installed AGY public model catalog through `--model`, including labels containing spaces or parentheses.

**AGP-24** When AGM resumes an AGY manifest containing an unambiguous retired `2.5-pro` or `2.0-flash-lite` alias or its former full identifier, the system shall translate it to the closest current AGY public model label before constructing the resume command; the ambiguous former default `2.5-flash` on a saved conversation is governed by AGP-28.

**AGP-25** When MCP creates an AGY session, the system shall wait through first-run trust and initialization until the AGY composer is ready before delivering the required startup prompt; cancellation or readiness failure shall enter the shared creation rollback path.

**AGP-26** When the AGM process receives SIGINT or SIGTERM, the root command context shall cancel and every command-scoped active-harness readiness or monitoring wait, including create and its final liveness scan, cold-resume metadata lookup and migration, post-create prompt delivery and verification, post-resume prompt delivery, direct, fan-out, or structured send delivery, model or mode slash-command delivery, compaction delivery and monitoring, continuous scan/watch loops, and AGY metadata backfill or association retry, shall return without continuing into tmux creation or command delivery, prompt delivery or retry, attach, success reporting, or metadata mutation.

**AGP-27** When a user supplies a cross-harness tier alias with different letter case, the system shall canonicalize the alias key case-insensitively while preserving any exact case-sensitive public model label.

**AGP-28** When an imported or manually associated AGY conversation has no observable native model, the system shall leave its manifest model unset and cold-resume without `--model` so AGY retains the saved conversation selection; when a pre-provenance saved-conversation record contains the ambiguous former default `2.5-flash` or `gemini-2.5-flash`, the resume path shall clear that stored override before command construction.

**AGP-29** When `send set-model` changes a running AGY conversation, the system shall persist the selection only after observing a new confirmation that exactly names the requested public model; a stale, mismatched, or unavailable confirmation shall clear the stored model override so a later cold resume cannot force an unselected model.

**AGP-21** When the AGY adapter creates or cold-resumes a session, the system shall use the shared canonical AGY command builder, preserve the selected model, permission mode, authorized directories, native conversation ID, quoting, and process-exit policy, require native readiness before returning success or attaching, and roll back any tmux session it created when command delivery, readiness, or metadata persistence fails; when an imported conversation has no defensible model provenance, cold resume shall omit `--model` so AGY retains the saved native selection.

**AGP-22** When the AGY adapter creates a fresh session, the system shall normalize the workspace to an absolute path and serialize the snapshot-through-discovery lifecycle per workspace before capturing and persisting its provider-native conversation ID after readiness and before reporting success, so concurrent creates cannot reuse or exchange the latest pre-launch workspace conversation; if the pre-launch identity snapshot cannot distinguish absence from corrupt or incomplete provider metadata, the system shall fail before creating tmux state. The adapter shall accept only safe path-component native identifiers before using them in a launch command or transcript path. When cold resume requires a new process, the system shall require that captured native ID and shall not substitute AGM's internal session ID.

**AGP-23** When the AGY adapter reports status or reads history, the system shall require an actual `agy` process for live status and shall read user/model messages from the native Antigravity brain transcript rather than a synthetic harness path.

**AGP-30** When AGY cold resume finds the recorded tmux session without a live `agy` process, the system shall verify that the pane contains only a restartable shell before command delivery and shall fail without mutation if another live harness is present or harness liveness cannot be determined.

**AGP-31** When CLI or MCP creates an AGY session through the shared operations lifecycle, the system shall correlate and persist a safe provider-native conversation ID after readiness and before registration or startup-prompt delivery, reject the pre-launch workspace identity as stale, and roll back the newly created tmux session if identity correlation fails.

**AGP-32** When the AGY adapter cold-resumes a conversation, the system shall allow the provider up to 60 seconds to become ready before treating readiness as failed, rolling back a newly created tmux session, or attaching.

**AGP-33** When the AGY adapter reads a native transcript entry whose source is absent, the system shall classify the established `USER_INPUT` and `PLANNER_RESPONSE` entry types as user and assistant messages respectively while continuing to ignore unrelated source-less entry types.

**AGP-34** When CLI or direct-adapter cold resume launches an AGY conversation, the system shall acquire the same canonical per-workspace lifecycle lock used by fresh creation before command delivery and hold it through native readiness, so a resume cannot replace AGY's workspace-global latest mapping during another operation's identity correlation.

**AGP-35** When the AGY adapter rolls back a tmux session it created after command delivery, readiness, identity discovery, or metadata persistence fails, the system shall preserve the primary failure and also report any tmux cleanup failure.

**AGP-36** When the AGY adapter creates a fresh conversation, the system shall route pre-launch snapshot and post-readiness identity correlation through the shared `agysession.CreateIdentityTracker` used by the operations lifecycle.

**AGP-37** When concurrent AGY adapter cold resumes target the same recorded tmux pane, the system shall acquire the workspace lifecycle lock before proving exact process liveness or a restartable shell and shall deliver at most one native resume command; a later caller shall observe and preserve the process launched by the earlier caller.

**AGP-38** When the AGY adapter creates or cold-resumes through a symlinked workspace, the system shall use the canonical physical workspace path consistently for locking, tmux creation, command construction, identity correlation, and newly persisted metadata.

**AGP-55** When the AGY adapter delivers an initial prompt or a later message, the system shall use AGY's harness-aware literal paste path, preserve embedded line feeds as one bracketed-paste submission, and send exactly one final Enter.

### Codex Workdir Trust (ce-cmsq)

**AGP-14** When a Codex CLI session is created or resumed through the codex-cli adapter, the system shall record the working directory as a trusted Codex project in `$CODEX_HOME/config.toml` (default `~/.codex/config.toml`) before sending the launch command, so a fresh non-git sandbox directory cannot block Codex startup on its interactive trust prompt.

**AGP-15** When the trusted-projects config already contains an entry for the working directory — at any trust level — the system shall leave the config unmodified, preserving explicit user distrust decisions.

**AGP-16** When appending a trust entry, the system shall preserve the existing config bytes, escape the directory path as a TOML basic-string key, and replace the file atomically; if the existing config fails to parse, the system shall leave the file untouched and report an error rather than risk a duplicate-table append that would break every subsequent Codex launch.

**AGP-17** If pre-trusting the working directory fails, the codex-cli adapter shall warn and still attempt the launch.

**AGP-53** When the Codex CLI adapter creates a tmux session for a fresh create or cold resume and private launch preparation or command delivery fails, the system shall clean up the session it created without terminating a pre-existing session.

### Harness Doctor Health

**AGP-19** When AGM doctor inspects an AGY session, including one stored with the legacy `agy-cli` or `antigravity` spelling, the system shall normalize the harness, derive `agy` from the shared harness binary registry, and use `$HOME/.gemini/antigravity-cli` as its advisory configuration directory rather than classify the session as unknown.

### BDD Enforcement

**AGP-12** When a new active harness or model family is added, the system shall require BDD scenarios and registry tests that cross-cut the active parity matrix before the change is complete.

**AGP-57** When AGM validates the harness parity specification, the system shall reject any `AGP` requirement identifier that does not occur exactly once so executable evidence can address one unambiguous contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/agent/agy_adapter_test.go`, `agm/internal/agent/codex_cli_adapter_test.go`, `agm/internal/agent/pi_adapter_test.go`
