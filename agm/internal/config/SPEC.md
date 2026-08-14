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

**CONFIG-01** When AGM loads configuration without a file, the system shall return defaults for sessions, timeouts, locks, health checks, adapters, auto-resume, status line, sandbox, and budget settings.

**CONFIG-02** When a configuration file overrides only some settings, the system shall preserve default values for unspecified settings.

**CONFIG-03** When OpenCode adapter configuration is enabled, the system shall require a server URL and valid reconnect delays.

**CONFIG-04** When OpenCode adapter configuration is disabled, the system shall tolerate incomplete adapter fields without failing configuration load.

**CONFIG-05** When centralized storage mode is configured, the system shall resolve the workspace before deriving AGM storage paths.

**CONFIG-06** When centralized storage mode is active, the system shall bootstrap `~/.agm` as a symlink to the centralized storage path without deleting existing data.

**CONFIG-07** When workspace detection runs in test mode, the system shall prefer `ENGRAM_TEST_WORKSPACE` over environment and filesystem discovery.

**CONFIG-08** When workspace detection cannot resolve a workspace, the system shall return an actionable error instead of prompting interactively.

**CONFIG-09** When adapter, sandbox, budget, or status-line defaults are changed, the system shall keep active harnesses on shared defaults unless a harness-specific setting is explicit.

**CONFIG-10** When AGM reads an existing shared configuration file, the system shall decode exactly one non-empty YAML mapping onto established core and operator-UI defaults with known fields enforced at every declared nested struct, and shall reject unknown fields, malformed known values, and every second YAML document.

**CONFIG-11** When no explicit configuration source is selected and the canonical source is ordinarily absent, the system shall retain defaults; when an explicit source is absent, any source path is dangling, or any other selected-source read fails, the system shall return no usable configuration before sandbox repository resolution.

**CONFIG-12** When an existing selected configuration source is read, the system shall authenticate one regular-file snapshot of at most 1 MiB before decoding it.

**CONFIG-13** When sandbox configuration is present, the system shall require a canonical mapping, canonical true or false `enabled`, non-empty canonical string `provider`, and canonical sequences of non-empty strings for `repos` and `writable_dirs`, while preserving aliases, YAML merge precedence, registered provider extensibility, and explicit `repos: []` compatibility.

**CONFIG-14** When sandbox repository or writable-directory paths use exact `~` or `~/...`, the system shall expand them against one physical HOME path selection, reject dot components, and require absolute effective paths before sandbox consumers run.

**CONFIG-15** When sandbox configuration selects a provider, the system shall project it into the effective command configuration unless an explicitly changed provider flag takes precedence.

**CONFIG-16** When configuration loading succeeds, the system shall retain one opaque, structurally immutable tuple of physically normalized HOME, storage, and sandbox-root paths selected from that snapshot, and later changes to HOME, workspace discovery, working directory, or public storage fields shall not replace those paths; default, directly constructed, and zero-value configurations shall provide no runtime authority.

**CONFIG-17** When a retained storage or sandbox path is projected for use, the system shall revalidate its existing filesystem components at projection time and reject dangling links, physical escape, or post-load symlink substitution; destructive consumers shall still require an operation-local filesystem check, while an existing dotfile-mode `~/.agm` symlink shall retain its resolved physical target for compatibility.

**CONFIG-18** When centralized storage is bootstrapped or verified, the configuration module shall derive the compatibility-link location and target from the same retained runtime authority, repair a wrong compatibility link without deleting its target, and return any bootstrap or integrity failure instead of claiming a dotfile fallback.

## BDD Traceability

- `agm/test/bdd/features/config_directory_parity.feature`
- `agm/test/bdd/features/harness_parity.feature`

## Package Test Traceability

- `agm/internal/config/config_test.go`
- `agm/internal/config/config_strict_test.go`
- `agm/internal/config/runtime_authority_test.go`
- `agm/internal/config/storage_test.go`
- `agm/internal/config/parser_golden_test.go`
- `agm/internal/config/fuzz_test.go`
