# Source Workflow Bridge Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/workflowbridge` adapts a `pkg/source.Adapter` to the workflow
`SourceIndexer` interface. It keeps workflow packages independent of concrete
source backends while allowing workflow artifacts to be indexed into the shared
knowledge substrate.

## Requirements

**SOURCE-WORKFLOWBRIDGE-01** When the bridge is created with a nil source adapter, the system shall return nil.

**SOURCE-WORKFLOWBRIDGE-02** When the bridge reports its name, the system shall forward the source adapter name.

**SOURCE-WORKFLOWBRIDGE-03** When a workflow source artifact is added, the system shall map URI, title, snippet, content, and indexed timestamp into a source record.

**SOURCE-WORKFLOWBRIDGE-04** When a workflow source artifact is added, the system shall map cues and work item metadata into source metadata.

**SOURCE-WORKFLOWBRIDGE-05** When a workflow source artifact is added, the system shall mark the source metadata origin as `workflow`.

**SOURCE-WORKFLOWBRIDGE-06** When a workflow source artifact has a content type, the system shall store it in source custom metadata.

**SOURCE-WORKFLOWBRIDGE-07** When the underlying adapter add fails, the system shall return that error to the workflow caller.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
