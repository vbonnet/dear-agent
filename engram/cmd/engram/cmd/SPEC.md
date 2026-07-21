# Engram CLI Commands - Technical Specification

## EARS Requirements

**ECMDR-01** When the Engram root command is built, the command package shall register the maintained context, memory, retrieval, and governance subcommands.

**ECMDR-02** When a command accepts a context budget, the command package shall validate the value before invoking storage or retrieval behavior.

**ECMDR-03** When `engram validate` selects a validator, the command package shall expose only maintained Engram, content, link, and YAML token contracts and shall not advertise retired Wayfinder schemas.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_cli_support_guardrails.feature`
- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

<!-- Last audited at: 2026-07-19 -->

**Status:** Living documentation
**Scope:** `engram/cmd/engram/cmd`

## Overview

This package defines the Cobra command tree for the `engram` CLI. It wires
user-facing subcommands to the underlying Engram packages without owning the
domain logic itself.

## Requirements

### FR1: Root Command Wiring

- The package MUST expose the root `engram` command.
- Subcommands MUST be registered from package initialization or explicit
  command construction.
- Command help MUST point to living in-repo documentation only.

### FR2: Context Budget Commands

- The `context` command group MUST expose context-window budget checks and
  model threshold inspection.
- Context budget behavior MUST delegate to `pkg/context`.
- User-facing help MUST direct library/API readers to `pkg/context/README.md`.

### FR3: Documentation Hygiene

- Command help MUST NOT link to temporal research reports, old phase
  deliverables, or deleted archive artifacts.
- Historical benchmark or research material belongs in `engram-research`, not
  in this command package.

## Non-Goals

- This package does not define the context budget algorithm.
- This package does not store benchmark datasets or research reports.
- This package does not own model threshold policy beyond command-line
  presentation.
