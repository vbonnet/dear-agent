# Engram Scratchpad Sandbox Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/scratchpad` executes bounded probe code in an isolated Docker
container and cleans up all temporary resources.

## EARS Requirements

**ESP-01** When a scratchpad is created, the system shall launch a container with no network, a read-only root filesystem, bounded CPU, memory, processes, and temporary storage, and a read-only probe mount.

**ESP-02** When supported Python, Bash, or Node code is submitted, the system shall write a private temporary source file and execute the matching interpreter in the sandbox.

**ESP-03** When an unsupported language is submitted, the system shall reject execution with an explicit error.

**ESP-04** When a request timeout or caller cancellation occurs, the system shall terminate the execution command and return the failure.

**ESP-05** When execution completes or fails, the system shall return captured output, exit status, duration, and any execution error.

**ESP-06** When the configured execution limit is reached, the system shall reject further probes without running them.

**ESP-07** When sandbox cleanup is requested, the system shall remove the container and temporary working directory.

**ESP-08** When concurrent execution or cleanup calls occur, the system shall serialize sandbox state changes.

**ESP-09** When scratchpad container startup succeeds, the system shall return a sandbox for code execution only after a private creation-time value is visible unchanged inside the execution environment.

**ESP-10** If the private creation-time value is absent or differs, the system shall return no sandbox and report that bind capability is unavailable before any interpreter execution.

**ESP-11** When sandbox creation fails after container startup begins, the system shall attempt cleanup of the preassigned container identity despite an ambiguous launch result and cleanup of the temporary working directory independently of creation-request cancellation, bound container cleanup by the cleanup time limit, and return cleanup failures together with the creation failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_scratchpad_bind_visibility.feature`
- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/scratchpad/sandbox_test.go`
- Test consequence: Deterministic integration tests exercise visible, missing, mismatched, canceled, ambiguous-launch, cleanup-deadline, and cleanup-failing Docker paths before interpreter execution.
