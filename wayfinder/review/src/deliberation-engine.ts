/**
 * Deliberation engine for adversarial debate between review personas.
 *
 * Orchestrates structured rounds of challenges and responses to surface
 * tensions, resolve disagreements, and produce a final deliberation memo.
 */

import type {
  Challenge,
  ChallengeResponse,
  DeliberationBrief,
  DeliberationConfig,
  DeliberationMemo,
  DeliberationRound,
  DeliberationStats,
  Tension,
} from './deliberation-types.js';
import type { Finding, PersonaReviewOutput } from './types.js';

// ============================================================================
// Default Configuration
// ============================================================================

/** Default deliberation configuration with all fields required */
export const DEFAULT_DELIBERATION_CONFIG: Required<DeliberationConfig> = {
  enabled: false,
  maxRounds: 3,
  maxDeliberationTokens: 50000,
  timeoutMs: 120000,
  convergenceThreshold: 0.8,
  leadReviewerModel: 'claude-3-opus-20240229',
};

// ============================================================================
// Interfaces
// ============================================================================

/** An agent that can respond to deliberation challenges */
export interface Challengeable {
  /** Send a challenge prompt and receive a text response */
  challenge(prompt: string): Promise<string>;
}

/** Lead reviewer that orchestrates the deliberation process */
export interface LeadReviewer {
  /** Identify tensions from the initial brief */
  identifyTensions(brief: DeliberationBrief): Promise<Tension[]>;

  /** Formulate challenges for unresolved tensions in a given round */
  formulateChallenges(tensions: Tension[], round: number): Promise<Challenge[]>;

  /** Synthesize a final memo from the brief and all rounds */
  synthesizeMemo(
    brief: DeliberationBrief,
    rounds: DeliberationRound[],
  ): Promise<DeliberationMemo>;

  /** Return total tokens consumed by this reviewer */
  getTokensUsed(): number;
}

// ============================================================================
// Deliberation Engine
// ============================================================================

/** Orchestrates adversarial deliberation between review personas */
export class DeliberationEngine {
  private readonly config: Required<DeliberationConfig>;
  private startTime: number = 0;
  private totalTokens: number = 0;

  constructor(config?: Partial<DeliberationConfig>) {
    this.config = { ...DEFAULT_DELIBERATION_CONFIG, ...config };
  }

  /**
   * Run the full deliberation process.
   *
   * @param sessionId - Session identifier
   * @param personaResults - Review outputs from each persona
   * @param codeContext - The code being reviewed
   * @param leadReviewer - Lead reviewer agent
   * @param personaAgents - Map of persona name to challengeable agent
   * @returns Final deliberation memo
   */
  async run(
    sessionId: string,
    personaResults: PersonaReviewOutput[],
    codeContext: string,
    leadReviewer: LeadReviewer,
    personaAgents: Map<string, Challengeable>,
  ): Promise<DeliberationMemo> {
    this.startTime = Date.now();
    this.totalTokens = 0;

    // 1. Compile the initial brief
    const brief = this.compileBrief(sessionId, personaResults, codeContext);

    // 2. Have the lead reviewer identify tensions
    try {
      const tensions = await leadReviewer.identifyTensions(brief);
      brief.tensions = tensions;
    } catch (err) {
      // Graceful degradation: proceed with no tensions
      brief.tensions = [];
    }

    // 3. Run deliberation rounds
    const rounds: DeliberationRound[] = [];

    for (let round = 1; round <= this.config.maxRounds; round++) {
      if (!this.shouldContinue(rounds, brief.tensions)) {
        break;
      }

      const roundStart = Date.now();
      let roundTokens = 0;

      // Get unresolved tensions
      const unresolvedTensions = brief.tensions.filter((t) => !t.resolved);
      if (unresolvedTensions.length === 0) {
        break;
      }

      // Formulate challenges
      let challenges: Challenge[];
      try {
        challenges = await leadReviewer.formulateChallenges(
          unresolvedTensions,
          round,
        );
      } catch (err) {
        // Graceful degradation: skip this round
        break;
      }

      if (challenges.length === 0) {
        break;
      }

      // Send challenges to personas in parallel
      const responsePromises = challenges.map(async (challenge) => {
        const agent = personaAgents.get(challenge.toPersona);
        if (!agent) {
          return this.defaultChallengeResponse(challenge);
        }

        try {
          const responseText = await agent.challenge(challenge.question);
          return this.parseChallengeResponse(challenge, responseText);
        } catch (err) {
          return this.defaultChallengeResponse(challenge);
        }
      });

      const responses = await Promise.all(responsePromises);

      // Estimate token usage for this round
      roundTokens = this.estimateRoundTokens(challenges, responses);
      this.totalTokens += roundTokens;

      // Update tensions based on responses
      this.updateTensions(brief.tensions, responses);

      rounds.push({
        roundNumber: round,
        challenges,
        responses,
        tokenUsage: roundTokens,
        durationMs: Date.now() - roundStart,
      });
    }

    // 4. Synthesize final memo
    try {
      const memo = await leadReviewer.synthesizeMemo(brief, rounds);
      memo.totalTokens = this.totalTokens + leadReviewer.getTokensUsed();
      memo.durationMs = Date.now() - this.startTime;
      return memo;
    } catch (err) {
      // Graceful degradation: return a basic memo
      return {
        sessionId,
        decision: 'CONDITIONAL',
        summary: 'Deliberation completed but memo synthesis failed.',
        findings: personaResults.flatMap((r) => r.findings),
        tensions: brief.tensions,
        recommendations: [],
        rounds,
        totalTokens: this.totalTokens,
        totalCost: 0,
        durationMs: Date.now() - this.startTime,
      };
    }
  }

  /**
   * Compile a deliberation brief from persona review outputs.
   *
   * @param sessionId - Session identifier
   * @param personaResults - Review outputs from each persona
   * @param codeContext - The code being reviewed
   * @returns Compiled deliberation brief
   */
  compileBrief(
    sessionId: string,
    personaResults: PersonaReviewOutput[],
    codeContext: string,
  ): DeliberationBrief {
    const findingsByPersona: Record<string, Finding[]> = {};

    for (const result of personaResults) {
      findingsByPersona[result.persona] = result.findings;
    }

    return {
      sessionId,
      codeContext,
      findingsByPersona,
      tensions: [],
    };
  }

  /**
   * Determine whether deliberation should continue.
   *
   * Checks timeout, token budget, and convergence threshold.
   *
   * @param rounds - Rounds completed so far
   * @param tensions - Current tensions
   * @returns Whether to continue deliberation
   */
  shouldContinue(rounds: DeliberationRound[], tensions?: Tension[]): boolean {
    // Check timeout
    if (Date.now() - this.startTime >= this.config.timeoutMs) {
      return false;
    }

    // Check token budget
    if (this.totalTokens >= this.config.maxDeliberationTokens) {
      return false;
    }

    // Check convergence
    if (tensions && tensions.length > 0) {
      const resolved = tensions.filter((t) => t.resolved).length;
      const ratio = resolved / tensions.length;
      if (ratio >= this.config.convergenceThreshold) {
        return false;
      }
    }

    return true;
  }

  /**
   * Update tensions based on challenge responses.
   *
   * Updates positions and marks tensions as resolved when no positions
   * have stance 'oppose'.
   *
   * @param tensions - Tensions to update
   * @param responses - Responses from this round
   */
  updateTensions(tensions: Tension[], responses: ChallengeResponse[]): void {
    for (const response of responses) {
      const tension = tensions.find((t) => t.id === response.tensionId);
      if (!tension) {
        continue;
      }

      // Update or add position for this persona
      const existingIdx = tension.positions.findIndex(
        (p) => p.persona === response.persona,
      );

      const newPosition = {
        persona: response.persona,
        stance: response.stance === 'revise' || response.stance === 'withdraw'
          ? ('neutral' as const)
          : (response.stance as 'support' | 'oppose' | 'neutral'),
        argument: response.argument,
        confidence: response.confidence,
      };

      if (existingIdx >= 0) {
        tension.positions[existingIdx] = newPosition;
      } else {
        tension.positions.push(newPosition);
      }
    }

    // Check if tensions are resolved (no opposing positions)
    for (const tension of tensions) {
      if (tension.resolved) {
        continue;
      }

      const hasOppose = tension.positions.some((p) => p.stance === 'oppose');
      if (!hasOppose && tension.positions.length > 0) {
        tension.resolved = true;
        tension.resolution =
          `Resolved after deliberation. Final positions: ${tension.positions.map((p) => `${p.persona}: ${p.stance}`).join(', ')}`;
      }
    }
  }

  /**
   * Parse a challenge response from raw text.
   *
   * Looks for `**Stance**: support|oppose|revise|withdraw` and
   * `**Confidence**: 0.X` patterns in the response text.
   *
   * @param challenge - The challenge that was issued
   * @param responseText - Raw text response from the persona
   * @returns Parsed challenge response
   */
  parseChallengeResponse(
    challenge: Challenge,
    responseText: string,
  ): ChallengeResponse {
    // Parse stance
    const stanceMatch = responseText.match(
      /\*\*Stance\*\*:\s*(support|oppose|revise|withdraw)/i,
    );
    const stance = (stanceMatch?.[1]?.toLowerCase() ?? 'neutral') as
      | 'support'
      | 'oppose'
      | 'revise'
      | 'withdraw';

    // Parse confidence
    const confidenceMatch = responseText.match(
      /\*\*Confidence\*\*:\s*([\d.]+)/i,
    );
    const confidence = confidenceMatch
      ? Math.min(1, Math.max(0, parseFloat(confidenceMatch[1])))
      : 0.5;

    // If stance didn't match any valid value, default to 'support'
    const validStances = ['support', 'oppose', 'revise', 'withdraw'];
    const finalStance = validStances.includes(stance) ? stance : 'support';

    return {
      challengeId: challenge.id,
      persona: challenge.toPersona,
      tensionId: challenge.tensionId,
      stance: finalStance as 'support' | 'oppose' | 'revise' | 'withdraw',
      argument: responseText,
      confidence,
    };
  }

  /**
   * Compute aggregate statistics for a deliberation session.
   *
   * @param rounds - All deliberation rounds
   * @param tensions - All tensions
   * @returns Deliberation statistics
   */
  getStats(rounds: DeliberationRound[], tensions: Tension[]): DeliberationStats {
    const totalChallenges = rounds.reduce(
      (sum, r) => sum + r.challenges.length,
      0,
    );

    const resolved = tensions.filter((t) => t.resolved).length;
    const unresolved = tensions.filter((t) => !t.resolved).length;

    let findingsRevised = 0;
    let findingsWithdrawn = 0;
    const personaTokens: Record<string, number> = {};

    for (const round of rounds) {
      for (const response of round.responses) {
        if (response.stance === 'revise') {
          findingsRevised++;
        }
        if (response.stance === 'withdraw') {
          findingsWithdrawn++;
        }

        // Estimate per-persona token usage from argument length
        const tokens = Math.ceil(response.argument.length / 4);
        personaTokens[response.persona] =
          (personaTokens[response.persona] ?? 0) + tokens;
      }
    }

    const totalRoundTokens = rounds.reduce((sum, r) => sum + r.tokenUsage, 0);

    return {
      totalRounds: rounds.length,
      totalChallenges,
      tensionsResolved: resolved,
      tensionsUnresolved: unresolved,
      findingsRevised,
      findingsWithdrawn,
      convergenceRatio:
        tensions.length > 0 ? resolved / tensions.length : 1,
      tokenUsage: {
        leadReviewer: Math.max(
          0,
          totalRoundTokens -
            Object.values(personaTokens).reduce((a, b) => a + b, 0),
        ),
        personas: personaTokens,
        total: totalRoundTokens,
      },
    };
  }

  /**
   * Create a default challenge response for when a persona agent is
   * unavailable or errors.
   */
  private defaultChallengeResponse(challenge: Challenge): ChallengeResponse {
    return {
      challengeId: challenge.id,
      persona: challenge.toPersona,
      tensionId: challenge.tensionId,
      stance: 'support' as const,
      argument: 'No response available.',
      confidence: 0.5,
    };
  }

  /**
   * Estimate token usage for a round using char/4 approximation.
   */
  private estimateRoundTokens(
    challenges: Challenge[],
    responses: ChallengeResponse[],
  ): number {
    let chars = 0;

    for (const challenge of challenges) {
      chars += challenge.question.length;
    }

    for (const response of responses) {
      chars += response.argument.length;
    }

    return Math.ceil(chars / 4);
  }
}
