# Canonical status fixtures

`valid-v2.yaml` exercises the schema 2.0 parser and validator with project
metadata, named phase history, roadmap tasks, lifecycle evidence, and quality
metrics.

The fixture uses the only supported phase sequence:

`CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO`

Tests parse it with `ParseV2`, validate it with `ValidateV2`, and verify that
serialization preserves canonical fields.
