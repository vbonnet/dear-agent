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

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/scratchpad/sandbox_test.go`
