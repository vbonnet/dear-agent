# Engram Plugin Runtime Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/internal/plugin` loads executable Engram plugins, verifies integrity
metadata, executes plugin commands through the security sandbox, wires EventBus
subscriptions, and exposes structured plugin runtime logging.

## Requirements

**ENGRAM-PLUGIN-01** When the loader scans multiple search paths, the system shall load each search path concurrently and return plugins sorted by plugin name.

**ENGRAM-PLUGIN-02** When a search path does not exist, the system shall log the miss and continue loading other search paths.

**ENGRAM-PLUGIN-03** When one plugin directory has an invalid manifest, the system shall log that plugin failure and continue loading sibling plugins.

**ENGRAM-PLUGIN-04** When a plugin name appears in the disabled list, the system shall skip that plugin after manifest loading.

**ENGRAM-PLUGIN-05** When a plugin manifest has integrity files, the system shall support only `sha256` integrity verification.

**ENGRAM-PLUGIN-06** When an integrity file is missing or has a mismatched hash, the system shall reject the plugin.

**ENGRAM-PLUGIN-07** When no integrity files are declared, the system shall skip integrity verification for legacy plugin compatibility.

**ENGRAM-PLUGIN-08** When executing a plugin command, the system shall enforce the executor timeout with a context deadline.

**ENGRAM-PLUGIN-09** When the parent context is cancelled during execution, the system shall return a cancellation error instead of a timeout error.

**ENGRAM-PLUGIN-10** When a requested command is not declared by the plugin manifest, the system shall return a command-not-found error.

**ENGRAM-PLUGIN-11** When plugin permissions are invalid, the system shall reject execution before applying the sandbox.

**ENGRAM-PLUGIN-12** When command execution succeeds, the system shall return the combined command output.

**ENGRAM-PLUGIN-13** When a plugin subscribes to EventBus events, the system shall register one handler per subscribed event type.

**ENGRAM-PLUGIN-14** When an EventBus handler executes, the system shall call the plugin command named `handler`.

**ENGRAM-PLUGIN-15** When listing loaded plugins through the enforcement adapter, the system shall return plugin name and command metadata.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
