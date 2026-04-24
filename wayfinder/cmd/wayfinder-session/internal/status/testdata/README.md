# Test Data for Wayfinder V2 Schema

This directory contains test fixtures for Wayfinder V2 YAML parser and validator.

## Files

- `valid-v2.yaml` - Example of a valid V2 WAYFINDER-STATUS.md file with all features:
  - Project metadata (name, type, risk level)
  - Phase history with phase-specific metadata
  - Roadmap with tasks and dependencies
  - Quality metrics

## V2 Schema Features

The V2 schema includes:

1. **9-Phase Consolidation**: W0, D1, D2, D3, D4, S6, S7, S8, S11
   - D4 includes S4 stakeholder approval
   - S6 includes S5 research
   - S8 includes S9 validation and S10 deployment

2. **Native Roadmap**: Tasks tracked directly in WAYFINDER-STATUS.md
   - Task dependencies (depends_on, blocks)
   - Effort estimates (effort_days)
   - Status tracking per task

3. **Quality Metrics**: Built-in quality tracking
   - Test coverage
   - Assertion density
   - Multi-persona review scores
   - Issue counts (P0/P1/P2)

## Usage

```go
// Parse V2 file
status, err := ParseV2("testdata/valid-v2.yaml")

// Validate
err = ValidateV2(status)

// Write V2 file
err = WriteV2(status, "output.yaml")
```
