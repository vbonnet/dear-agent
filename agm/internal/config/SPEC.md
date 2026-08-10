# AGM Config Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/config` owns AGM runtime configuration loading, defaults, adapter
settings, centralized storage resolution, and legacy dotfile compatibility. The
package is part of harness parity because Claude Code, Codex CLI, AGY, OpenCode,
and Pi must resolve the same repository, storage, budget, sandbox, status-line,
and adapter contracts before command handlers or daemons make harness-specific
decisions.

## EARS Requirements

**CONFIG-01** When AGM loads configuration without a file, the system shall return defaults for sessions, timeouts, locks, health checks, adapters, auto-resume, status line, sandbox, budget, and notification settings.

**CONFIG-02** When a configuration file overrides only some settings, the system shall preserve default values for unspecified settings.

**CONFIG-03** When OpenCode adapter configuration is enabled, the system shall require a server URL and valid reconnect delays.

**CONFIG-04** When OpenCode adapter configuration is disabled, the system shall tolerate incomplete adapter fields without failing configuration load.

**CONFIG-05** When centralized storage mode is configured, the system shall resolve the workspace before deriving AGM storage paths.

**CONFIG-06** When centralized storage mode is active, the system shall bootstrap `~/.agm` as a symlink to the centralized storage path without deleting existing data.

**CONFIG-07** When workspace detection runs in test mode, the system shall prefer `ENGRAM_TEST_WORKSPACE` over environment and filesystem discovery.

**CONFIG-08** When workspace detection cannot resolve a workspace, the system shall return an actionable error instead of prompting interactively.

**CONFIG-09** When adapter, sandbox, budget, status-line, or notification defaults are changed, the system shall keep active harnesses on shared defaults unless a harness-specific setting is explicit.

**CONFIG-10** When notification configuration is loaded, the system shall preserve a configurable recipient label, default to a generic operator label, and allow `AGM_NOTIFY_RECIPIENT` to override only that recipient label.

**CONFIG-11** When notification dispatcher configuration is omitted, the system shall preserve zero-config completion delivery by defaulting to a local log dispatcher.

## BDD Traceability

- `agm/test/bdd/features/config_directory_parity.feature`
- `agm/test/bdd/features/harness_parity.feature`

## Package Test Traceability

- `agm/internal/config/config_test.go`
- `agm/internal/config/storage_test.go`
- `agm/internal/config/parser_golden_test.go`
- `agm/internal/config/fuzz_test.go`
