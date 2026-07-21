import { parseDocument } from 'yaml';

const WAYPOINTS = [
  'CHARTER',
  'PROBLEM',
  'RESEARCH',
  'DESIGN',
  'SPEC',
  'PLAN',
  'SETUP',
  'BUILD',
  'RETRO',
] as const;
const PROJECT_TYPES = ['feature', 'research', 'infrastructure', 'refactor', 'bugfix'] as const;
const RISK_LEVELS = ['XS', 'S', 'M', 'L', 'XL'] as const;
const PROJECT_STATUSES = ['planning', 'in-progress', 'blocked', 'completed', 'abandoned'] as const;
const WAYPOINT_STATUSES = ['pending', 'completed', 'in-progress', 'blocked', 'skipped'] as const;
const SKIPPABLE_WAYPOINTS = ['DESIGN', 'SPEC', 'PLAN'] as const;
const LIFECYCLE_STATUSES: Record<string, string> = {
  working: 'in-progress',
  input_required: 'blocked',
  dependency_blocked: 'blocked',
  validating: 'in-progress',
  completed: 'completed',
  failed: 'blocked',
  canceled: 'abandoned',
};
const KNOWN_FIELDS = new Set([
  'schema_version',
  'project_name',
  'project_type',
  'risk_level',
  'current_waypoint',
  'status',
  'created_at',
  'updated_at',
  'description',
  'repository',
  'branch',
  'tags',
  'beads',
  'completion_date',
  'blocked_reason',
  'lifecycle_state',
  'blocked_on',
  'error_message',
  'input_needed',
  'skip_roadmap',
  'skip_phases',
  'waypoint_history',
  'roadmap',
  'quality_metrics',
]);

export interface WayfinderStatusSummary {
  phase: string;
  progress: string;
  status: string;
}

type RecordValue = Record<string, unknown>;

function extractFrontmatter(content: string): string {
  const normalized = content.replace(/^\uFEFF/, '').replace(/\r\n/g, '\n');
  if (!normalized.startsWith('---\n')) {
    throw new Error('invalid Wayfinder V2 status: must start with ---');
  }
  const closing = normalized.indexOf('\n---', 4);
  if (closing < 0) {
    throw new Error('invalid Wayfinder V2 status: missing closing ---');
  }
  const remainder = normalized.slice(closing + 4).trim();
  if (remainder !== '') {
    throw new Error('invalid Wayfinder V2 status: content after closing --- is not allowed');
  }
  return normalized.slice(4, closing);
}

function asRecord(value: unknown, path: string): RecordValue {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`invalid Wayfinder V2 status: ${path} must be a mapping`);
  }
  return value as RecordValue;
}

function requiredString(record: RecordValue, key: string): string {
  const value = record[key];
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`invalid Wayfinder V2 status: ${key} is required`);
  }
  return value;
}

function requireEnum(record: RecordValue, key: string, allowed: readonly string[]): string {
  const value = requiredString(record, key);
  if (!allowed.includes(value)) {
    throw new Error(`invalid Wayfinder V2 status: invalid ${key} ${JSON.stringify(value)}`);
  }
  return value;
}

function requireTimestamp(record: RecordValue, key: string): void {
  const value = record[key];
  if (!(typeof value === 'string' || value instanceof Date) || Number.isNaN(new Date(value).getTime())) {
    throw new Error(`invalid Wayfinder V2 status: ${key} is required`);
  }
}

function optionalStringArray(record: RecordValue, key: string): string[] {
  const value = record[key];
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) {
    throw new Error(`invalid Wayfinder V2 status: ${key} must be a string array`);
  }
  return value as string[];
}

function validateSkipPhases(record: RecordValue): string[] {
  const phases = optionalStringArray(record, 'skip_phases');
  const unique = new Set(phases);
  if (unique.size !== phases.length || phases.some((phase) => !SKIPPABLE_WAYPOINTS.includes(phase as never))) {
    throw new Error('invalid Wayfinder V2 status: skip_phases may contain DESIGN, SPEC, and PLAN once each');
  }
  if (record.skip_roadmap !== undefined && typeof record.skip_roadmap !== 'boolean') {
    throw new Error('invalid Wayfinder V2 status: skip_roadmap must be a boolean');
  }
  return phases;
}

function completedWaypoints(record: RecordValue): Set<string> {
  const history = record.waypoint_history;
  if (history === undefined || history === null) return new Set();
  if (!Array.isArray(history)) {
    throw new Error('invalid Wayfinder V2 status: waypoint_history must be an array');
  }

  const complete = new Set<string>();
  for (const [index, entry] of history.entries()) {
    const waypoint = asRecord(entry, `waypoint_history[${index}]`);
    const name = requireEnum(waypoint, 'name', WAYPOINTS);
    const status = requireEnum(waypoint, 'status', WAYPOINT_STATUSES);
    requireTimestamp(waypoint, 'started_at');
    if (status === 'completed') {
      requireTimestamp(waypoint, 'completed_at');
    }
    if (status === 'completed' || status === 'skipped') {
      complete.add(name);
    }
  }
  return complete;
}

function validateConditionalStatus(record: RecordValue, status: string): void {
  if (status === 'completed') {
    requireTimestamp(record, 'completion_date');
  }
  if (status === 'blocked') {
    requiredString(record, 'blocked_reason');
  }

  const lifecycle = record.lifecycle_state;
  if (lifecycle === undefined || lifecycle === null || lifecycle === '') return;
  if (typeof lifecycle !== 'string' || LIFECYCLE_STATUSES[lifecycle] === undefined) {
    throw new Error(`invalid Wayfinder V2 status: invalid lifecycle_state ${JSON.stringify(lifecycle)}`);
  }
  const expectedStatus = LIFECYCLE_STATUSES[lifecycle];
  if (status !== expectedStatus) {
    throw new Error(`invalid Wayfinder V2 status: lifecycle_state ${JSON.stringify(lifecycle)} requires status ${JSON.stringify(expectedStatus)}`);
  }
  if (lifecycle === 'input_required') requiredString(record, 'input_needed');
  if (lifecycle === 'dependency_blocked') requiredString(record, 'blocked_on');
  if (lifecycle === 'failed') requiredString(record, 'error_message');
}

export function parseWayfinderStatus(content: string): WayfinderStatusSummary {
  const document = parseDocument(extractFrontmatter(content), { strict: true, uniqueKeys: true });
  if (document.errors.length > 0) {
    throw new Error(`invalid Wayfinder V2 status: ${document.errors[0].message}`);
  }
  const record = asRecord(document.toJS(), 'document');
  for (const key of Object.keys(record)) {
    if (!KNOWN_FIELDS.has(key)) {
      throw new Error(`invalid Wayfinder V2 status: unknown field ${JSON.stringify(key)}`);
    }
  }

  const schemaVersion = requiredString(record, 'schema_version');
  if (schemaVersion !== '2.0') {
    throw new Error(`invalid Wayfinder V2 status: schema_version must be "2.0", got ${JSON.stringify(schemaVersion)}`);
  }
  requiredString(record, 'project_name');
  requireEnum(record, 'project_type', PROJECT_TYPES);
  requireEnum(record, 'risk_level', RISK_LEVELS);
  const phase = requireEnum(record, 'current_waypoint', WAYPOINTS);
  const status = requireEnum(record, 'status', PROJECT_STATUSES);
  requireTimestamp(record, 'created_at');
  requireTimestamp(record, 'updated_at');
	validateConditionalStatus(record, status);

  const complete = completedWaypoints(record);
  for (const skipped of validateSkipPhases(record)) complete.add(skipped);
  if (record.skip_roadmap === true) complete.add('SETUP');

  return {
    phase,
    progress: status === 'completed' ? '100%' : `${Math.floor((complete.size * 100) / WAYPOINTS.length)}%`,
    status,
  };
}
