import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { parseWayfinderStatus } from './wayfinder_status.js';

const canonical = `---
schema_version: "2.0"
project_name: typescript-reader
project_type: feature
risk_level: XS
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:03:00Z
skip_roadmap: true
skip_phases: [DESIGN, SPEC, PLAN]
waypoint_history:
  - {name: CHARTER, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}
  - {name: PROBLEM, status: completed, started_at: 2026-07-20T00:01:00Z, completed_at: 2026-07-20T00:02:00Z}
  - {name: RESEARCH, status: completed, started_at: 2026-07-20T00:02:00Z, completed_at: 2026-07-20T00:03:00Z}
---
`;

describe('parseWayfinderStatus', () => {
  it('reads canonical V2 fields and derives skipped-phase progress', () => {
    assert.deepEqual(parseWayfinderStatus(canonical), {
      phase: 'BUILD',
      progress: '77%',
      status: 'in-progress',
    });
  });

  it('rejects legacy Markdown labels', () => {
    assert.throws(
      () => parseWayfinderStatus('Current Phase: **BUILD**\nProgress: 60%\nStatus: In Progress\n'),
      /must start with ---/,
    );
  });

  it('rejects BOM and CRLF framing that the Go reader rejects', () => {
    assert.throws(() => parseWayfinderStatus(`\uFEFF${canonical}`), /must start with ---/);
    assert.throws(() => parseWayfinderStatus(canonical.replaceAll('\n', '\r\n')), /must start with ---/);
  });

  it('requires the closing delimiter to occupy the complete line', () => {
    assert.throws(() => parseWayfinderStatus(canonical.replace('\n---\n', '\n---   \n')), /missing closing ---/);
  });

  it('rejects a non-canonical schema version', () => {
    assert.throws(() => parseWayfinderStatus(canonical.replace('"2.0"', '"1.0"')), /schema_version must be/);
  });

  it('rejects unknown fields instead of silently accepting schema drift', () => {
    assert.throws(
      () => parseWayfinderStatus(canonical.replace('project_name:', 'mystery: true\nproject_name:')),
      /unknown field "mystery"/,
    );
  });

  it('rejects wrong types for optional top-level string arrays', () => {
    assert.throws(
      () => parseWayfinderStatus(canonical.replace('skip_roadmap:', 'beads: ce-123\nskip_roadmap:')),
      /document\.beads must be a string array/,
    );
    assert.throws(
      () => parseWayfinderStatus(canonical.replace('skip_roadmap:', 'tags: release\nskip_roadmap:')),
      /document\.tags must be a string array/,
    );
  });

  it('rejects YAML aliases before resolving the document', () => {
    const invalid = canonical.replace(
      'skip_roadmap:',
      'tags: &shared [release]\nbeads: *shared\nskip_roadmap:',
    );
    assert.throws(() => parseWayfinderStatus(invalid), /YAML aliases are not allowed/);
  });

  it('rejects wrong types in optional waypoint history fields', () => {
    const invalid = canonical.replace('status: completed, started_at:', 'status: completed, deliverables: output.md, started_at:');
    assert.throws(() => parseWayfinderStatus(invalid), /deliverables must be a string array/);
  });

  it('rejects non-RFC3339 and impossible timestamps', () => {
    assert.throws(() => parseWayfinderStatus(canonical.replace('2026-07-20T00:00:00Z', '"1"')), /created_at is required/);
    assert.throws(() => parseWayfinderStatus(canonical.replace('2026-07-20T00:00:00Z', '2026-02-30T00:00:00Z')), /created_at is required/);
  });

  it('rejects invalid BUILD metadata enums', () => {
    const invalid = canonical.replace(
      '  - {name: CHARTER,',
      '  - {name: BUILD, validation_status: typo, deployment_status: typo,',
    );
    assert.throws(() => parseWayfinderStatus(invalid), /validation_status is invalid/);
  });

  it('rejects duplicate waypoint history names', () => {
    const duplicate = canonical.replace(
      '  - {name: PROBLEM,',
      '  - {name: CHARTER,',
    );
    assert.throws(() => parseWayfinderStatus(duplicate), /duplicate waypoint_history name "CHARTER"/);
  });

  it('rejects invalid waypoint outcomes', () => {
    const invalid = canonical.replace('status: completed, started_at:', 'status: completed, outcome: typo, started_at:');
    assert.throws(() => parseWayfinderStatus(invalid), /outcome is invalid/);
  });

  it('rejects skipped history for mandatory phases', () => {
    const invalid = canonical.replace(
      '  - {name: CHARTER, status: completed,',
      '  - {name: BUILD, status: skipped,',
    );
    assert.throws(() => parseWayfinderStatus(invalid), /cannot skip mandatory waypoint "BUILD"/);
  });

  it('rejects active history for a configured skip', () => {
    const invalid = canonical.replace(
      '\n---\n',
      '\n  - {name: DESIGN, status: in-progress, started_at: 2026-07-20T00:03:00Z}\n---\n',
    );
    assert.throws(() => parseWayfinderStatus(invalid), /configured skipped waypoint "DESIGN" cannot have active status "in-progress"/);
  });

  it('rejects a configured skip as the unresolved current waypoint', () => {
    const invalid = canonical.replace('current_waypoint: BUILD', 'current_waypoint: DESIGN');
    assert.throws(() => parseWayfinderStatus(invalid), /current_waypoint "DESIGN" is configured to be skipped/);
  });

  it('rejects history that bypasses mandatory predecessors', () => {
    const invalid = `---
schema_version: "2.0"
project_name: bypass
project_type: feature
risk_level: M
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:01:00Z
waypoint_history:
  - {name: BUILD, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}
---
`;
    assert.throws(() => parseWayfinderStatus(invalid), /requires completed predecessor "CHARTER"/);
  });

  it('rejects a later current waypoint when history is missing', () => {
    const invalid = canonical.replace(/waypoint_history:[\s\S]*?\n---\n$/, '---\n');
    assert.throws(() => parseWayfinderStatus(invalid), /requires completed predecessor "CHARTER"/);
  });

  it('rejects history entries ahead of the current waypoint', () => {
    const invalid = canonical.replace('current_waypoint: BUILD', 'current_waypoint: CHARTER');
    assert.throws(() => parseWayfinderStatus(invalid), /cannot be ahead of current_waypoint/);
  });

  it('requires blocked_reason for blocked status', () => {
    assert.throws(
      () => parseWayfinderStatus(canonical.replace('status: in-progress', 'status: blocked')),
      /blocked_reason is required/,
    );
  });

  it('enforces lifecycle and status consistency', () => {
    assert.throws(
      () => parseWayfinderStatus(canonical.replace('status: in-progress', 'status: in-progress\nlifecycle_state: completed')),
      /lifecycle_state "completed" requires status "completed"/,
    );
  });

  it('rejects completed status until every required waypoint is complete', () => {
    const incomplete = canonical.replace(
      'status: in-progress',
      'status: completed\ncompletion_date: 2026-07-20T00:04:00Z',
    );
    assert.throws(() => parseWayfinderStatus(incomplete), /required Wayfinder phases are incomplete: BUILD, RETRO/);
  });

  it('accepts canonical hyphenated lifecycle state names', () => {
    const blocked = canonical.replace(
      'status: in-progress',
      'status: blocked\nblocked_reason: waiting for input\nlifecycle_state: input-required\ninput_needed: choose a database',
    );
    assert.equal(parseWayfinderStatus(blocked).status, 'blocked');
  });

  it('rejects out-of-range nested quality metrics', () => {
    const invalid = canonical.replace('waypoint_history:', 'quality_metrics: {coverage_percent: 150}\nwaypoint_history:');
    assert.throws(() => parseWayfinderStatus(invalid), /quality_metrics\.coverage_percent must be 0-100/);
  });

  it('rejects a roadmap that is not a mapping', () => {
    const invalid = canonical.replace('waypoint_history:', 'roadmap: []\nwaypoint_history:');
    assert.throws(() => parseWayfinderStatus(invalid), /roadmap must be a mapping/);
  });

  it('validates nested roadmap task references', () => {
    const invalid = canonical.replace(
      'waypoint_history:',
      `roadmap:
  phases:
    - id: BUILD
      name: Implementation
      status: in-progress
      tasks:
        - id: task-1
          title: Implement
          status: blocked
          depends_on: [missing-task]
waypoint_history:`,
    );
    assert.throws(() => parseWayfinderStatus(invalid), /depends_on references missing task/);
  });

  it('rejects negative roadmap effort', () => {
    const invalid = canonical.replace(
      'waypoint_history:',
      `roadmap:
  phases:
    - id: BUILD
      status: in-progress
      tasks:
        - id: task-1
          status: pending
          effort_days: -1
waypoint_history:`,
    );
    assert.throws(() => parseWayfinderStatus(invalid), /effort_days cannot be negative/);
  });

  it('rejects duplicate roadmap phase IDs', () => {
    const invalid = canonical.replace(
      'waypoint_history:',
      `roadmap:
  phases:
    - {id: BUILD, status: in-progress}
    - {id: BUILD, status: pending}
waypoint_history:`,
    );
    assert.throws(() => parseWayfinderStatus(invalid), /duplicate roadmap phase id "BUILD"/);
  });
});
