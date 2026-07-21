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

  it('rejects a non-canonical schema version', () => {
    assert.throws(() => parseWayfinderStatus(canonical.replace('"2.0"', '"1.0"')), /schema_version must be/);
  });

  it('rejects unknown fields instead of silently accepting schema drift', () => {
    assert.throws(
      () => parseWayfinderStatus(canonical.replace('project_name:', 'mystery: true\nproject_name:')),
      /unknown field "mystery"/,
    );
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
});
