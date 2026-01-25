/**
 * Intent Analyzer - Context Broker Implementation
 *
 * Parses user intent using keyword matching to generate Requirement Envelopes.
 * Target: 85-90% accuracy, <10ms p99 performance
 */

export type Action = 'CREATE' | 'READ' | 'UPDATE' | 'DELETE' | 'SEARCH' | 'UNKNOWN';
export type Service = 'atlassian' | 'googledocs' | 'slack' | 'glean';

/**
 * Requirement Envelope - structured representation of user intent
 */
export interface RequirementEnvelope {
  action: Action;
  target?: string;
  service?: Service;
  scope?: string[];
  confidence: number;
  raw_intent: string;
  parsed_at: string;
  fallback_to_all: boolean;
}

/**
 * Action keyword patterns (CRUD + SEARCH operations)
 */
const ACTIONS: Record<Action, RegExp> = {
  CREATE: /\b(create|add|new|make|generate|send|post)\b/i,
  READ: /\b(read|get|show|view|display|fetch|list)\b/i,
  UPDATE: /\b(update|edit|modify|change|set)\b/i,
  DELETE: /\b(delete|remove|destroy|cancel)\b/i,
  SEARCH: /\b(search|find|query|lookup|filter)\b/i,
  UNKNOWN: /^$/,  // Never matches, placeholder only
};

/**
 * Service keyword patterns (downstream MCPs)
 * Note: More specific patterns first, then generic ones
 * "document" can be ambiguous, so only match when clearly a service reference
 */
const SERVICES: Record<Service, RegExp> = {
  glean: /\b(glean|knowledge\s*base|kb)\b/i,
  atlassian: /\b(jira|confluence|atlassian|ticket|issue)\b/i,
  slack: /\b(slack|message|channel|dm)\b/i,
  googledocs: /\b(google\s*docs?|gdocs|(?:my|the|a|recent)\s+documents?|(?:my|the|recent)\s+doc)\b/i,
};

/**
 * Target extraction patterns (entities being acted upon)
 * Priority: Specific patterns should come before generic ones
 */
const TARGET_PATTERNS: Record<string, RegExp> = {
  // Named entity (high priority to catch "named X Y Z")
  named: /\b(?:named|called)\s+([A-Z][\w]*(?:\s+[\w]+)*)/i,

  // Atlassian targets (must come after named to avoid false matches)
  ticket: /\b(?:ticket|issue)\s+([\w-]+)(?!\s+[A-Z])/i,

  // Slack targets (preserve #)
  channel: /\b(?:to\s+(?:channel\s+)?)?(#[\w-]+)/i,
  user: /\b(?:user|@)\s*(@?[\w-]+)/i,

  // GoogleDocs targets (match multi-word titles after "document"/"doc")
  document: /\b(?:document|doc)\s+((?:[A-Z][\w]*\s+)*[A-Z][\w]*)/i,
};

/**
 * Matches first pattern group against raw text
 * @param patterns - Map of pattern name to regex
 * @param text - Raw user input
 * @returns Matched key or undefined
 */
function matchFirst<T extends string>(
  patterns: Record<T, RegExp>,
  text: string
): T | undefined {
  for (const [key, pattern] of Object.entries(patterns) as [T, RegExp][]) {
    if (pattern.test(text)) {
      return key;
    }
  }
  return undefined;
}

/**
 * Extracts target entity from raw text
 * @param text - Raw user input
 * @returns Extracted target string or undefined
 */
function extractTarget(text: string): string | undefined {
  for (const [, pattern] of Object.entries(TARGET_PATTERNS)) {
    const match = pattern.exec(text);
    if (match) {
      // Find first non-empty capture group
      for (let i = 1; i < match.length; i++) {
        if (match[i]) {
          return match[i].trim();
        }
      }
    }
  }
  return undefined;
}

/**
 * Extracts scope indicators from raw text (e.g., "all", "my", "recent")
 * @param text - Raw user input
 * @returns Array of scope indicators
 */
function extractScope(text: string): string[] {
  const scope: string[] = [];

  if (/\b(all|every)\b/i.test(text)) {
    scope.push('all');
  }
  if (/\b(my|mine)\b/i.test(text)) {
    scope.push('user');
  }
  if (/\b(recent|latest)\b/i.test(text)) {
    scope.push('recent');
  }
  if (/\b(open|active)\b/i.test(text)) {
    scope.push('active');
  }

  return scope;
}

/**
 * Calculates confidence score based on matched patterns
 * @param action - Detected action
 * @param service - Detected service
 * @param target - Detected target
 * @returns Confidence score (0.0-1.0)
 */
function calculateConfidence(
  action: Action,
  service: Service | undefined,
  target: string | undefined
): number {
  let confidence = 0.0;

  // Action detection adds base confidence
  if (action !== 'UNKNOWN') {
    confidence += 0.4;
  }

  // Service detection adds significant confidence
  if (service !== undefined) {
    confidence += 0.4;
  }

  // Target detection adds additional confidence
  if (target !== undefined) {
    confidence += 0.1;
  }

  // Cap at 0.9 (perfect intent is rare with keyword matching)
  return Math.min(confidence, 0.9);
}

/**
 * Analyzes user intent from raw text input
 *
 * @param raw - User's raw message text
 * @returns RequirementEnvelope with parsed intent
 *
 * @example
 * analyzeIntent("Create Jira ticket")
 * // => { action: 'CREATE', service: 'atlassian', confidence: 0.8, ... }
 *
 * @example
 * analyzeIntent("Show me the document")
 * // => { action: 'READ', service: 'googledocs', confidence: 0.8, ... }
 */
export function analyzeIntent(raw: string): RequirementEnvelope {
  // Normalize input
  const normalized = raw.trim();

  // Match patterns
  const action = matchFirst(ACTIONS, normalized) || 'UNKNOWN';
  const service = matchFirst(SERVICES, normalized);
  const target = extractTarget(normalized);
  const scope = extractScope(normalized);

  // Calculate confidence
  const confidence = calculateConfidence(action, service, target);

  // Determine fallback behavior (threshold: 0.5)
  const fallback_to_all = confidence < 0.5;

  return {
    action,
    service,
    target,
    scope: scope.length > 0 ? scope : undefined,
    confidence,
    raw_intent: raw,
    parsed_at: new Date().toISOString(),
    fallback_to_all,
  };
}

/**
 * Batch analyzes multiple intents (for testing/validation)
 * @param intents - Array of raw intent strings
 * @returns Array of RequirementEnvelopes
 */
export function analyzeIntentBatch(intents: string[]): RequirementEnvelope[] {
  return intents.map(analyzeIntent);
}

/**
 * Validates if an envelope meets minimum confidence threshold
 * @param envelope - RequirementEnvelope to validate
 * @param threshold - Minimum confidence (default: 0.5)
 * @returns True if confidence meets threshold
 */
export function isConfident(envelope: RequirementEnvelope, threshold = 0.5): boolean {
  return envelope.confidence >= threshold;
}

/**
 * Exports action patterns for testing/validation
 */
export const PATTERNS = {
  ACTIONS,
  SERVICES,
} as const;
