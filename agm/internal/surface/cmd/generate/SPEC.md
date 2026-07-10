# Surface Metadata Generator Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/surface/cmd/generate` generates maintained command-surface
metadata from AGM's canonical registry.

## Requirements

**SGN-01** When the generator runs, the system shall load the canonical command surface rather than maintaining a second handwritten registry.

**SGN-02** When generated output is written, the system shall format deterministic Go source suitable for version control.

**SGN-03** When registry loading, generation, formatting, or writing fails, the system shall exit non-zero and leave the failure visible.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/internal/surface/cmd/generate/*_test.go`
