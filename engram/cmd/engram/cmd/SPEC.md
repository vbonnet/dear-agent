# Engram CLI Commands - Technical Specification

<!-- Last audited at: NEEDS-AUDIT -->

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
