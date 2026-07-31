---
name: write-spec
description: "Write or revise SPEC.md as an observable, implementation- and harness-neutral contract. Use when behavior needs normative EARS requirements, cross-harness consistency, an explicit capability variation, or BDD traceability. Do not use for behavior-preserving refactors or non-normative prose edits."
---

# Write a specification

This skill is the harness-neutral entry point for the repository's canonical
SPEC authoring workflow. It deliberately does not restate the rules, commands,
or stop conditions: [`docs/spec-authoring.md`](../../../docs/spec-authoring.md)
owns them.

## Workflow

1. Read [`docs/spec-authoring.md`](../../../docs/spec-authoring.md) completely,
   including the focused contract-model and EARS/BDD references it links.
2. Follow its **Before editing a specification** workflow against live source,
   specifications, inventory, and tests.
3. Apply its ownership, capability, adapter, traceability, stop, and maintainer
   boundaries without adding harness-local variants of this skill.
4. Run the exact checks in its **Review and maintainer boundary** section.
5. Report the outcome and delivery states required by that guide.

## Verify

Complete every item in the canonical guide's **Review and maintainer
boundary** section. Do not treat this skill as an alternative checklist.

## Canonical owner

- [Repository SPEC authoring guide](../../../docs/spec-authoring.md)
