# ADR-026: Reconcile Claude UI session archival locally

Status: Accepted (2026-05-30; amended 2026-07-17)

## Context

AGM lifecycle records and Claude's local UI session records are different
namespaces. Archiving one does not hide the other from Claude's session list.
Using undocumented web APIs or browser credentials would expand the security
boundary.

## Decision

`agm session archive-ui` and its shared operation reconcile supported local
Claude UI records by changing only their archive field. The operation is
schema-aware, dry-run capable, reversible, and never deletes transcripts or
harvests credentials. It refuses unknown shapes rather than guessing.

Bulk and scheduled callers use the same operation. AGM session archival remains
owned by ADR-016 and does not imply provider-record archival.

## Alternatives

Cookie-backed web automation is undocumented and credential-sensitive. Deleting
local files destroys data. Treating UI records as AGM sessions conflates two
identities.

## Consequences

The adapter is provider-specific and may need updates when Claude changes its
local schema. Claude UI, operations, command, and schedule tests verify safety
and reconciliation.
