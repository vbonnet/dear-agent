/**
 * Type definitions for the adversarial deliberation feature.
 *
 * Deliberation enables structured debate between personas to surface
 * tensions, challenge assumptions, and converge on high-confidence findings.
 */

import type { Finding } from './types.js';

// ============================================================================
// Deliberation Configuration
// ============================================================================

/** Configuration for the adversarial deliberation process */
export interface DeliberationConfig {
  /** Whether deliberation is enabled */
  enabled: boolean;

  /** Maximum number of deliberation rounds (default 3) */
  maxRounds?: number;

  /** Maximum total tokens to spend on deliberation */
  maxDeliberationTokens?: number;

  /** Timeout in milliseconds for the entire deliberation */
  timeoutMs?: number;

  /** Convergence threshold (0-1); deliberation stops when reached */
  convergenceThreshold?: number;

  /** Model to use for the lead reviewer */
  leadReviewerModel?: string;
}

// ============================================================================
// Deliberation Brief
// ============================================================================

/** Initial brief compiled from persona review results for deliberation */
export interface DeliberationBrief {
  /** Session identifier */
  sessionId: string;

  /** Code context being reviewed */
  codeContext: string;

  /** Findings grouped by persona name */
  findingsByPersona: Record<string, Finding[]>;

  /** Identified tensions between persona positions */
  tensions: Tension[];
}

// ============================================================================
// Tension & Position Types
// ============================================================================

/** A tension represents a point of disagreement between personas */
export interface Tension {
  /** Unique identifier */
  id: string;

  /** Topic of the tension (e.g., 'error handling strategy') */
  topic: string;

  /** Positions held by each persona on this tension */
  positions: PersonaPosition[];

  /** Whether the tension has been resolved through deliberation */
  resolved: boolean;

  /** Resolution summary (when resolved) */
  resolution?: string;

  /** IDs of findings related to this tension */
  relatedFindings: string[];
}

/** A persona's position on a particular tension */
export interface PersonaPosition {
  /** Persona name */
  persona: string;

  /** Stance on the topic */
  stance: 'support' | 'oppose' | 'neutral';

  /** Argument supporting the stance */
  argument: string;

  /** Confidence level (0-1) */
  confidence: number;
}

// ============================================================================
// Deliberation Round Types
// ============================================================================

/** Record of a single deliberation round */
export interface DeliberationRound {
  /** Round number (1-indexed) */
  roundNumber: number;

  /** Challenges issued by the lead reviewer */
  challenges: Challenge[];

  /** Responses from challenged personas */
  responses: ChallengeResponse[];

  /** Tokens consumed in this round */
  tokenUsage: number;

  /** Duration of this round in milliseconds */
  durationMs: number;
}

/** A challenge issued by the lead reviewer to a persona */
export interface Challenge {
  /** Unique identifier */
  id: string;

  /** Persona being challenged */
  toPersona: string;

  /** Tension this challenge relates to */
  tensionId: string;

  /** The challenge question */
  question: string;
}

/** A persona's response to a challenge */
export interface ChallengeResponse {
  /** ID of the challenge being responded to */
  challengeId: string;

  /** Persona responding */
  persona: string;

  /** Tension this response relates to */
  tensionId: string;

  /** Updated stance after considering the challenge */
  stance: 'support' | 'oppose' | 'revise' | 'withdraw';

  /** Argument supporting the updated stance */
  argument: string;

  /** Updated confidence level (0-1) */
  confidence: number;

  /** Revised finding (when stance is 'revise') */
  revisedFinding?: Finding;
}

// ============================================================================
// Deliberation Output Types
// ============================================================================

/** Final memo produced by the lead reviewer after deliberation */
export interface DeliberationMemo {
  /** Session identifier */
  sessionId: string;

  /** Overall decision */
  decision: 'GO' | 'NO-GO' | 'CONDITIONAL';

  /** Human-readable summary of the deliberation outcome */
  summary: string;

  /** Final set of findings after deliberation */
  findings: Finding[];

  /** All tensions (resolved and unresolved) */
  tensions: Tension[];

  /** Actionable recommendations */
  recommendations: string[];

  /** Record of all deliberation rounds */
  rounds: DeliberationRound[];

  /** Total tokens consumed across all rounds */
  totalTokens: number;

  /** Total cost in USD */
  totalCost: number;

  /** Total deliberation duration in milliseconds */
  durationMs: number;
}

/** Aggregate statistics for a deliberation session */
export interface DeliberationStats {
  /** Total number of rounds executed */
  totalRounds: number;

  /** Total number of challenges issued */
  totalChallenges: number;

  /** Number of tensions that were resolved */
  tensionsResolved: number;

  /** Number of tensions that remain unresolved */
  tensionsUnresolved: number;

  /** Number of findings that were revised */
  findingsRevised: number;

  /** Number of findings that were withdrawn */
  findingsWithdrawn: number;

  /** Ratio of resolved tensions to total tensions (0-1) */
  convergenceRatio: number;

  /** Token usage breakdown */
  tokenUsage: {
    /** Tokens used by the lead reviewer */
    leadReviewer: number;

    /** Tokens used by each persona */
    personas: Record<string, number>;

    /** Total tokens used */
    total: number;
  };
}
