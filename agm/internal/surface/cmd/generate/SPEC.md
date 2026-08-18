# Surface Metadata Generator Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/surface/cmd/generate` generates maintained CLI and MCP surface
metadata from AGM's canonical operation registry.

## Requirements

**SGN-01** When the generator runs, the system shall load the canonical command surface rather than maintaining a second handwritten registry.

**SGN-02** When generated output is written, the system shall format deterministic Go source suitable for version control.

**SGN-03** When registry loading, generation, formatting, or writing fails, the system shall exit non-zero and leave the failure visible.

**SGN-04** While installed plugin commands are owned by the live Cobra tree, this generator shall not emit a second plugin Markdown source.

**SGN-05** When repository preflight or CI runs, the system shall regenerate the checked-in ignored surface artifacts and fail if their bytes change.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/internal/surface/cmd/generate/*_test.go`
