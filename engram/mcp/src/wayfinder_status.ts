import { isAlias, parseDocument, visit } from 'yaml';

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
const TASK_STATUSES = ['pending', 'in-progress', 'completed', 'blocked'] as const;
const TASK_PRIORITIES = ['P0', 'P1', 'P2'] as const;
const VALIDATION_STATUSES = ['pending', 'in-progress', 'passed', 'failed'] as const;
const DEPLOYMENT_STATUSES = ['pending', 'in-progress', 'deployed', 'rolled-back'] as const;
const WAYPOINT_OUTCOMES = ['success', 'partial', 'skipped'] as const;
const SKIPPABLE_WAYPOINTS = ['DESIGN', 'SPEC', 'PLAN'] as const;
const LIFECYCLE_STATUSES: Record<string, string> = {
  working: 'in-progress',
  'input-required': 'blocked',
  'dependency-blocked': 'blocked',
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
  const lines = content.split('\n');
  if (lines[0] !== '---') {
    throw new Error('invalid Wayfinder V2 status: must start with ---');
  }
  const closingLine = lines.indexOf('---', 1);
  if (closingLine < 0) {
    throw new Error('invalid Wayfinder V2 status: missing closing ---');
  }
  if (lines.slice(closingLine + 1).some((line) => line.trim() !== '')) {
    throw new Error('invalid Wayfinder V2 status: content after closing --- is not allowed');
  }
  return lines.slice(1, closingLine).join('\n');
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

function isCanonicalTimestamp(value: unknown): value is string {
  if (typeof value !== 'string') return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (match === null) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, , offsetHourText, offsetMinuteText] = match;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  const hour = Number(hourText);
  const minute = Number(minuteText);
  const second = Number(secondText);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const daysInMonth = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > daysInMonth[month - 1]) return false;
  if (hour > 23 || minute > 59 || second > 59) return false;
  if (offsetHourText !== undefined && (Number(offsetHourText) > 23 || Number(offsetMinuteText) > 59)) return false;
  return true;
}

function requireTimestamp(record: RecordValue, key: string): void {
  const value = record[key];
  if (!isCanonicalTimestamp(value)) {
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

function assertKnownFields(record: RecordValue, path: string, knownFields: readonly string[]): void {
  const known = new Set(knownFields);
  for (const key of Object.keys(record)) {
    if (!known.has(key)) {
      throw new Error(`invalid Wayfinder V2 status: unknown field ${JSON.stringify(`${path}.${key}`)}`);
    }
  }
}

function optionalString(record: RecordValue, key: string, path: string): void {
  const value = record[key];
  if (value !== undefined && value !== null && typeof value !== 'string') {
    throw new Error(`invalid Wayfinder V2 status: ${path}.${key} must be a string`);
  }
}

function optionalTimestamp(record: RecordValue, key: string, path: string): void {
  const value = record[key];
  if (value === undefined || value === null) return;
  if (!isCanonicalTimestamp(value)) {
    throw new Error(`invalid Wayfinder V2 status: ${path}.${key} must be a timestamp`);
  }
}

function optionalNumber(record: RecordValue, key: string, path: string): number | undefined {
  const value = record[key];
  if (value === undefined || value === null) return undefined;
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`invalid Wayfinder V2 status: ${path}.${key} must be a number`);
  }
  return value;
}

function optionalInteger(record: RecordValue, key: string, path: string): number | undefined {
  const value = optionalNumber(record, key, path);
  if (value !== undefined && !Number.isInteger(value)) {
    throw new Error(`invalid Wayfinder V2 status: ${path}.${key} must be an integer`);
  }
  return value;
}

function optionalBoolean(record: RecordValue, key: string, path: string): void {
  const value = record[key];
  if (value !== undefined && value !== null && typeof value !== 'boolean') {
    throw new Error(`invalid Wayfinder V2 status: ${path}.${key} must be a boolean`);
  }
}

function validateStringArray(record: RecordValue, key: string, path: string): string[] {
  const value = record[key];
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) {
    throw new Error(`invalid Wayfinder V2 status: ${path}.${key} must be a string array`);
  }
  return value as string[];
}

function validateOptionalTopLevelFields(record: RecordValue): void {
  for (const key of ['description', 'repository', 'branch', 'blocked_reason', 'lifecycle_state', 'blocked_on', 'error_message', 'input_needed']) {
    optionalString(record, key, 'document');
  }
  validateStringArray(record, 'tags', 'document');
  validateStringArray(record, 'beads', 'document');
  optionalTimestamp(record, 'completion_date', 'document');
}

function validateBuildMetrics(value: unknown, path: string): void {
  if (value === undefined || value === null) return;
  const metrics = asRecord(value, path);
  assertKnownFields(metrics, path, [
    'tests_passed', 'tests_failed', 'coverage_percent', 'assertion_density', 'build_duration_secs',
  ]);
  optionalInteger(metrics, 'tests_passed', path);
  optionalInteger(metrics, 'tests_failed', path);
  optionalNumber(metrics, 'coverage_percent', path);
  optionalNumber(metrics, 'assertion_density', path);
  optionalInteger(metrics, 'build_duration_secs', path);
}

function validateQualityMetrics(value: unknown): void {
  if (value === undefined || value === null) return;
  const metrics = asRecord(value, 'quality_metrics');
  const fields = [
    'coverage_percent', 'coverage_target', 'assertion_density', 'assertion_density_target',
    'multi_persona_score', 'security_score', 'performance_score', 'reliability_score',
    'maintainability_score', 'p0_issues', 'p1_issues', 'p2_issues',
    'estimated_effort_hours', 'actual_effort_hours', 'effort_variance',
  ] as const;
  assertKnownFields(metrics, 'quality_metrics', fields);

  for (const key of fields) optionalNumber(metrics, key, 'quality_metrics');
  for (const key of ['coverage_percent', 'coverage_target', 'multi_persona_score', 'security_score', 'performance_score', 'reliability_score', 'maintainability_score'] as const) {
    const score = optionalNumber(metrics, key, 'quality_metrics');
    if (score !== undefined && (score < 0 || score > 100)) {
      throw new Error(`invalid Wayfinder V2 status: quality_metrics.${key} must be 0-100`);
    }
  }
  for (const key of ['assertion_density', 'assertion_density_target', 'estimated_effort_hours', 'actual_effort_hours'] as const) {
    const amount = optionalNumber(metrics, key, 'quality_metrics');
    if (amount !== undefined && amount < 0) {
      throw new Error(`invalid Wayfinder V2 status: quality_metrics.${key} cannot be negative`);
    }
  }
  for (const key of ['p0_issues', 'p1_issues', 'p2_issues'] as const) {
    const count = optionalNumber(metrics, key, 'quality_metrics');
    if (count !== undefined && (!Number.isInteger(count) || count < 0)) {
      throw new Error(`invalid Wayfinder V2 status: quality_metrics.${key} must be a non-negative integer`);
    }
  }
}

function validateRoadmap(value: unknown): void {
  if (value === undefined || value === null) return;
  const roadmap = asRecord(value, 'roadmap');
  assertKnownFields(roadmap, 'roadmap', ['phases']);
  const phasesValue = roadmap.phases;
  if (phasesValue === undefined || phasesValue === null) return;
  if (!Array.isArray(phasesValue)) {
    throw new Error('invalid Wayfinder V2 status: roadmap.phases must be an array');
  }

  const tasks = new Map<string, RecordValue>();
  const phaseIds = new Set<string>();
  for (const [phaseIndex, phaseValue] of phasesValue.entries()) {
    const path = `roadmap.phases[${phaseIndex}]`;
    const phase = asRecord(phaseValue, path);
    assertKnownFields(phase, path, ['id', 'name', 'status', 'started_at', 'completed_at', 'tasks']);
    const phaseId = requireEnum(phase, 'id', WAYPOINTS);
    if (phaseIds.has(phaseId)) {
      throw new Error(`invalid Wayfinder V2 status: duplicate roadmap phase id ${JSON.stringify(phaseId)}`);
    }
    phaseIds.add(phaseId);
    requireEnum(phase, 'status', WAYPOINT_STATUSES);
    optionalString(phase, 'name', path);
    optionalTimestamp(phase, 'started_at', path);
    optionalTimestamp(phase, 'completed_at', path);

    const phaseTasks = phase.tasks;
    if (phaseTasks === undefined || phaseTasks === null) continue;
    if (!Array.isArray(phaseTasks)) {
      throw new Error(`invalid Wayfinder V2 status: ${path}.tasks must be an array`);
    }
    for (const [taskIndex, taskValue] of phaseTasks.entries()) {
      const taskPath = `${path}.tasks[${taskIndex}]`;
      const task = asRecord(taskValue, taskPath);
      assertKnownFields(task, taskPath, [
        'id', 'title', 'effort_days', 'status', 'deliverables', 'tests_status', 'depends_on',
        'description', 'priority', 'assigned_to', 'blocks', 'acceptance_criteria', 'started_at',
        'completed_at', 'bead_id', 'notes', 'verify_command', 'verify_expected', 'verified_at',
        'verify_result',
      ]);
      const id = requiredString(task, 'id');
      if (tasks.has(id)) {
        throw new Error(`invalid Wayfinder V2 status: duplicate roadmap task id ${JSON.stringify(id)}`);
      }
      requireEnum(task, 'status', TASK_STATUSES);
      for (const key of ['title', 'tests_status', 'description', 'assigned_to', 'bead_id', 'notes', 'verify_command', 'verify_expected', 'verify_result']) {
        optionalString(task, key, taskPath);
      }
      const effortDays = optionalNumber(task, 'effort_days', taskPath);
      if (effortDays !== undefined && effortDays < 0) {
        throw new Error(`invalid Wayfinder V2 status: ${taskPath}.effort_days cannot be negative`);
      }
      optionalTimestamp(task, 'started_at', taskPath);
      optionalTimestamp(task, 'completed_at', taskPath);
      optionalTimestamp(task, 'verified_at', taskPath);
      for (const key of ['deliverables', 'depends_on', 'blocks', 'acceptance_criteria']) {
        validateStringArray(task, key, taskPath);
      }
      const priority = task.priority;
      if (priority !== undefined && priority !== null && priority !== '') {
        if (typeof priority !== 'string' || !TASK_PRIORITIES.includes(priority as never)) {
          throw new Error(`invalid Wayfinder V2 status: ${taskPath}.priority is invalid`);
        }
      }
      tasks.set(id, task);
    }
  }

  for (const [id, task] of tasks) {
    for (const key of ['depends_on', 'blocks'] as const) {
      for (const reference of validateStringArray(task, key, `roadmap task ${id}`)) {
        if (!tasks.has(reference)) {
          throw new Error(`invalid Wayfinder V2 status: task ${JSON.stringify(id)} ${key} references missing task ${JSON.stringify(reference)}`);
        }
      }
    }
  }

  const visiting = new Set<string>();
  const visited = new Set<string>();
  const visit = (id: string): void => {
    if (visiting.has(id)) throw new Error(`invalid Wayfinder V2 status: cyclic roadmap dependency at ${JSON.stringify(id)}`);
    if (visited.has(id)) return;
    visiting.add(id);
    for (const dependency of validateStringArray(tasks.get(id)!, 'depends_on', `roadmap task ${id}`)) visit(dependency);
    visiting.delete(id);
    visited.add(id);
  };
  for (const id of tasks.keys()) visit(id);
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

function completedWaypoints(
  record: RecordValue,
  configuredSkips: ReadonlySet<string>,
  currentPosition: number,
): Set<string> {
  const history = record.waypoint_history;
  if (history === undefined || history === null) return new Set();
  if (!Array.isArray(history)) {
    throw new Error('invalid Wayfinder V2 status: waypoint_history must be an array');
  }

  const complete = new Set<string>();
  const seen = new Set<string>();
  let lastPosition = -1;
  for (const [index, entry] of history.entries()) {
    const path = `waypoint_history[${index}]`;
    const waypoint = asRecord(entry, path);
    assertKnownFields(waypoint, path, [
      'name', 'status', 'started_at', 'completed_at', 'deliverables', 'notes', 'outcome',
      'stakeholder_approved', 'stakeholder_notes', 'research_notes', 'tests_feature_created',
      'validation_status', 'deployment_status', 'build_iterations', 'build_metrics',
    ]);
    const name = requireEnum(waypoint, 'name', WAYPOINTS);
    if (seen.has(name)) {
      throw new Error(`invalid Wayfinder V2 status: duplicate waypoint_history name ${JSON.stringify(name)}`);
    }
    seen.add(name);
    const status = requireEnum(waypoint, 'status', WAYPOINT_STATUSES);
    if (status === 'skipped' && !configuredSkips.has(name)) {
      throw new Error(`invalid Wayfinder V2 status: ${path} cannot skip mandatory waypoint ${JSON.stringify(name)}`);
    }
    if (configuredSkips.has(name) && status !== 'skipped' && status !== 'completed') {
      throw new Error(`invalid Wayfinder V2 status: ${path} configured skipped waypoint ${JSON.stringify(name)} cannot have active status ${JSON.stringify(status)}`);
    }
    requireTimestamp(waypoint, 'started_at');
    optionalTimestamp(waypoint, 'completed_at', path);
    if (status === 'completed') {
      requireTimestamp(waypoint, 'completed_at');
    }
    validateStringArray(waypoint, 'deliverables', path);
    for (const key of ['notes', 'outcome', 'stakeholder_notes', 'research_notes', 'validation_status', 'deployment_status']) {
      optionalString(waypoint, key, path);
    }
    const outcome = waypoint.outcome;
    if (outcome !== undefined && outcome !== null && !WAYPOINT_OUTCOMES.includes(outcome as never)) {
      throw new Error(`invalid Wayfinder V2 status: ${path}.outcome is invalid`);
    }
    if (name === 'BUILD') {
      const validationStatus = waypoint.validation_status;
      if (validationStatus !== undefined && validationStatus !== null && validationStatus !== '' && !VALIDATION_STATUSES.includes(validationStatus as never)) {
        throw new Error(`invalid Wayfinder V2 status: ${path}.validation_status is invalid`);
      }
      const deploymentStatus = waypoint.deployment_status;
      if (deploymentStatus !== undefined && deploymentStatus !== null && deploymentStatus !== '' && !DEPLOYMENT_STATUSES.includes(deploymentStatus as never)) {
        throw new Error(`invalid Wayfinder V2 status: ${path}.deployment_status is invalid`);
      }
    }
    optionalBoolean(waypoint, 'stakeholder_approved', path);
    optionalBoolean(waypoint, 'tests_feature_created', path);
    optionalInteger(waypoint, 'build_iterations', path);
    validateBuildMetrics(waypoint.build_metrics, `${path}.build_metrics`);
    const position = WAYPOINTS.indexOf(name as never);
    if (position > currentPosition) {
      throw new Error(`invalid Wayfinder V2 status: ${path} waypoint ${JSON.stringify(name)} cannot be ahead of current_waypoint`);
    }
    if (position < lastPosition) {
      throw new Error(`invalid Wayfinder V2 status: ${path} waypoint ${JSON.stringify(name)} is out of canonical order`);
    }
    for (const predecessor of WAYPOINTS.slice(0, position)) {
      if (!configuredSkips.has(predecessor) && !complete.has(predecessor)) {
        throw new Error(`invalid Wayfinder V2 status: ${path} requires completed predecessor ${JSON.stringify(predecessor)}`);
      }
    }
    if (status === 'completed' || status === 'skipped') {
      complete.add(name);
    }
    lastPosition = position;
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
  if (lifecycle === 'input-required') requiredString(record, 'input_needed');
  if (lifecycle === 'dependency-blocked') requiredString(record, 'blocked_on');
  if (lifecycle === 'failed') requiredString(record, 'error_message');
}

export function parseWayfinderStatus(content: string): WayfinderStatusSummary {
  const document = parseDocument(extractFrontmatter(content), { strict: true, uniqueKeys: true });
  if (document.errors.length > 0) {
    throw new Error(`invalid Wayfinder V2 status: ${document.errors[0].message}`);
  }
  visit(document, (_key, node) => {
    if (isAlias(node)) {
      throw new Error('invalid Wayfinder V2 status: YAML aliases are not allowed');
    }
  });
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
  validateOptionalTopLevelFields(record);
  validateConditionalStatus(record, status);
  validateRoadmap(record.roadmap);
  validateQualityMetrics(record.quality_metrics);

  const configuredSkips = new Set<string>(validateSkipPhases(record));
  if (record.skip_roadmap === true) configuredSkips.add('SETUP');
  const currentPosition = WAYPOINTS.indexOf(phase as never);
  const complete = completedWaypoints(record, configuredSkips, currentPosition);
  if (configuredSkips.has(phase) && !complete.has(phase)) {
    throw new Error(`invalid Wayfinder V2 status: current_waypoint ${JSON.stringify(phase)} is configured to be skipped but has no completed or skipped history`);
  }
  for (const predecessor of WAYPOINTS.slice(0, currentPosition)) {
    if (!configuredSkips.has(predecessor) && !complete.has(predecessor)) {
      throw new Error(`invalid Wayfinder V2 status: current_waypoint ${JSON.stringify(phase)} requires completed predecessor ${JSON.stringify(predecessor)}`);
    }
  }
  for (const skipped of configuredSkips) complete.add(skipped);
  if (status === 'completed') {
    const incomplete = WAYPOINTS.filter((waypoint) => !complete.has(waypoint));
    if (incomplete.length > 0) {
      throw new Error(`invalid Wayfinder V2 status: required Wayfinder phases are incomplete: ${incomplete.join(', ')}`);
    }
  }

  return {
    phase,
    progress: status === 'completed' ? '100%' : `${Math.floor((complete.size * 100) / WAYPOINTS.length)}%`,
    status,
  };
}
