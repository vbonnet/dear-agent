# AGM Deep Research Workflow Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/workflow/deepresearch` implements the AGM deep-research workflow
for the Gemini CLI harness. It extracts URLs from prompts, invokes the
Gemini deep-research tool, records resumable markdown logs, and generates
research-derived improvement proposals.

## Requirements

**AGM-DEEPRESEARCH-01** When the workflow is constructed, the system shall prefer `GEMINI_DR_PATH` and otherwise fall back to the conventional tool path or `gemini-deep-research` on `PATH`.

**AGM-DEEPRESEARCH-02** When the workflow advertises supported harnesses, the system shall return only `gemini-cli`.

**AGM-DEEPRESEARCH-03** When execution receives a prompt with no URLs, the system shall return an unsuccessful result and an error.

**AGM-DEEPRESEARCH-04** When execution receives one URL, the system shall run one deep-research command and return one `research-report` artifact.

**AGM-DEEPRESEARCH-05** When execution receives multiple URLs, the system shall create a crash-resilient research log for the session.

**AGM-DEEPRESEARCH-06** When a previous log marks URLs completed, the system shall skip those URLs and reuse their report artifacts.

**AGM-DEEPRESEARCH-07** When researching multiple pending URLs, the system shall launch one research worker per pending URL and collect exactly one result from each worker.

**AGM-DEEPRESEARCH-08** When a research worker panics, the system shall convert the panic into a failed research result instead of deadlocking collection.

**AGM-DEEPRESEARCH-09** When at least one URL succeeds, the system shall return a successful workflow result with URL counts and errors in metadata.

**AGM-DEEPRESEARCH-10** When research reports are available, the system shall generate markdown proposals and append them to the research log.

**AGM-DEEPRESEARCH-11** When no research reports are available to the applicator, the system shall return an error instead of generating empty proposals.

## BDD Traceability

- `agm/test/bdd/features/workflow_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
