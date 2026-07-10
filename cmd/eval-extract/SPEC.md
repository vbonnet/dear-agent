# Evaluation Extraction Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/eval-extract` converts real trace failures into deterministic regression
cases independent of the originating harness or model family.

## EARS Requirements

**EEC-01** When no input path is supplied, the command shall reject extraction before creating an eval store.

**EEC-02** When input is a JSON object, JSON array, JSONL file, or directory, the command shall decode traces according to the documented format.

**EEC-03** When a directory is scanned, the command shall process only JSON and JSONL files in lexical order.

**EEC-04** When any input record is malformed, the command shall return the file and line context rather than silently dropping evidence.

**EEC-05** When traces are classified, the command shall apply configured score, retry, and memory-relevance thresholds through the shared classifier.

**EEC-06** When dry-run mode is selected, the command shall report problematic traces without writing eval cases.

**EEC-07** When extraction runs, the command shall persist discoverable cases through the shared eval pipeline and report generated and skipped counts.

**EEC-08** When no traces are present, the command shall report that condition and exit without creating fabricated cases.

**EEC-09** When traces originate from different harnesses or model families, the command shall classify the shared trace schema identically.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_intelligence_command_guardrails.feature`
- Package tests: `cmd/eval-extract/*_test.go`
