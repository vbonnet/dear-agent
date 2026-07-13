# Deployment Drift Check Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/drift-check` compares reviewed source artifacts with deployed host copies
and reports merged-but-not-deployed gaps.

## EARS Requirements

**DDC-01** When no configuration file is supplied, the command shall use its embedded deploy-target configuration.

**DDC-02** When configuration is unreadable or malformed, the command shall return an error before evaluating targets.

**DDC-03** When no repository root is supplied, the command shall resolve Git's top-level directory within the shared 30-second timeout.

**DDC-04** When a Git ref is supplied, the command shall compare deployed artifacts with committed source from that ref rather than mutable working-tree content.

**DDC-05** When required deployed content is stale or missing, the command shall report remediation and return exit code 2.

**DDC-06** When a target cannot be evaluated, the command shall return an error rather than treating unknown evidence as drift-free.

**DDC-07** When audit mode is enabled, the command shall append actionable drift evidence without hiding drift if audit persistence fails.

**DDC-08** When JSON output is selected, the command shall emit the complete structured report; when quiet text output is selected, it shall suppress only passing target lines.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Package tests: `cmd/drift-check/*_test.go`
