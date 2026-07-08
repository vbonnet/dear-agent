# Engram Corpus Callosum Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`engram/internal/corpus` publishes Engram's schemas to corpus callosum and
queries related AGM, Wayfinder, and swarm data through the same optional
integration. The package keeps Engram discoverable without making corpus
callosum a hard dependency for local Engram operation.

Registration is deliberately gated by `CORPUS_CALLOSUM_BIN`; the package does
not auto-detect `cc` from `PATH` because `cc` is commonly the system C
compiler on macOS.

## EARS Requirements

**ECO-01** When the Engram schema is requested, the system shall identify the component as `engram`, advertise the current component version, declare backward compatibility, and include bead, document, memory trace, and ecphory result schemas.

**ECO-02** When the document schema is requested, the system shall describe stateless versioned documents separately from mutable memory traces.

**ECO-03** When `CORPUS_CALLOSUM_BIN` is unset or does not resolve to an executable, the system shall treat corpus callosum as unavailable and shall not auto-detect the system `cc` binary.

**ECO-04** When schema registration runs while corpus callosum is unavailable, the system shall skip registration without returning an error.

**ECO-05** When schema registration runs with a workspace, the system shall include that workspace in both the schema payload and the registration command.

**ECO-06** When schema registration invokes corpus callosum, the system shall write the schema to a temporary JSON file and call `register` with the Engram component name, component version, and schema path.

**ECO-07** When schema registration or unregistration fails, the system shall return an error that includes the corpus callosum command output.

**ECO-08** When registration status is checked while corpus callosum is unavailable, the system shall report not registered without returning an error.

**ECO-09** When AGM sessions are queried, the system shall query component `agm`, schema `session`, the requested workspace, and any caller-provided filter.

**ECO-10** When Wayfinder projects are queried, the system shall query component `wayfinder`, schema `project`, the requested workspace, and any caller-provided filter.

**ECO-11** When swarm projects are queried, the system shall query component `wayfinder`, schema `swarm`, preserving the merged Wayfinder ownership of swarm project data.

**ECO-12** When corpus callosum query output is not successful JSON, the system shall return a parse or query error instead of returning partial data.

**ECO-13** When components are discovered, the system shall parse the JSON response and return only the component names.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_knowledge_guardrails.feature`
- Package tests: `engram/internal/corpus/schema_test.go`
- Package tests: `engram/internal/corpus/register_test.go`

