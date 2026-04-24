/**
 * Unit tests for the DeliberationEngine class.
 *
 * Tests cover:
 * - compileBrief() — compiles findings from persona results
 * - shouldContinue() — timeout, token budget, convergence threshold
 * - parseChallengeResponse() — stance/confidence parsing, missing fields
 * - updateTensions() — position updates, tension resolution
 * - run() — full deliberation flow with mocked agents
 * - getStats() — statistics calculation
 * - Edge cases: empty results, immediate resolution, timeout, budget exceeded
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  DeliberationEngine,
  DEFAULT_DELIBERATION_CONFIG,
  type LeadReviewer,
  type Challengeable,
} from '../../src/deliberation-engine.js';
import type {
  Challenge,
  ChallengeResponse,
  DeliberationBrief,
  DeliberationMemo,
  DeliberationRound,
  Tension,
} from '../../src/deliberation-types.js';
import type { Finding, PersonaReviewOutput } from '../../src/types.js';

// ============================================================================
// Test Helpers
// ============================================================================

function makeFinding(overrides: Partial<Finding> = {}): Finding {
  return {
    id: 'f-1',
    file: 'src/main.ts',
    severity: 'medium',
    personas: ['security'],
    title: 'Potential issue',
    description: 'Something might be wrong',
    ...overrides,
  };
}

function makePersonaResult(overrides: Partial<PersonaReviewOutput> = {}): PersonaReviewOutput {
  return {
    persona: 'security',
    findings: [makeFinding()],
    cost: { persona: 'security', cost: 0.01, inputTokens: 500, outputTokens: 100 },
    ...overrides,
  };
}

function makeTension(overrides: Partial<Tension> = {}): Tension {
  return {
    id: 't-1',
    topic: 'error handling strategy',
    positions: [
      { persona: 'security', stance: 'oppose', argument: 'Too permissive', confidence: 0.8 },
      { persona: 'perf', stance: 'support', argument: 'Performance first', confidence: 0.7 },
    ],
    resolved: false,
    relatedFindings: ['f-1'],
    ...overrides,
  };
}

function makeChallenge(overrides: Partial<Challenge> = {}): Challenge {
  return {
    id: 'c-1',
    toPersona: 'security',
    tensionId: 't-1',
    question: 'Can you justify your position?',
    ...overrides,
  };
}

function makeRound(overrides: Partial<DeliberationRound> = {}): DeliberationRound {
  return {
    roundNumber: 1,
    challenges: [makeChallenge()],
    responses: [
      {
        challengeId: 'c-1',
        persona: 'security',
        tensionId: 't-1',
        stance: 'support',
        argument: 'Reconsidered, now agree.',
        confidence: 0.9,
      },
    ],
    tokenUsage: 500,
    durationMs: 1000,
    ...overrides,
  };
}

function createMockLeadReviewer(overrides: Partial<LeadReviewer> = {}): LeadReviewer {
  return {
    identifyTensions: vi.fn().mockResolvedValue([makeTension()]),
    formulateChallenges: vi.fn().mockResolvedValue([makeChallenge()]),
    synthesizeMemo: vi.fn().mockResolvedValue({
      sessionId: 'test-session',
      decision: 'GO',
      summary: 'All tensions resolved.',
      findings: [makeFinding()],
      tensions: [makeTension({ resolved: true, resolution: 'Resolved' })],
      recommendations: ['Ship it'],
      rounds: [],
      totalTokens: 1000,
      totalCost: 0.05,
      durationMs: 500,
    } satisfies DeliberationMemo),
    getTokensUsed: vi.fn().mockReturnValue(800),
    ...overrides,
  };
}

function createMockChallengeable(responseText: string): Challengeable {
  return {
    challenge: vi.fn().mockResolvedValue(responseText),
  };
}

// ============================================================================
// Tests
// ============================================================================

describe('DeliberationEngine', () => {
  let engine: DeliberationEngine;

  beforeEach(() => {
    engine = new DeliberationEngine({ enabled: true, maxRounds: 3 });
  });

  // --------------------------------------------------------------------------
  // DEFAULT_DELIBERATION_CONFIG
  // --------------------------------------------------------------------------

  describe('DEFAULT_DELIBERATION_CONFIG', () => {
    it('should have expected default values', () => {
      expect(DEFAULT_DELIBERATION_CONFIG.enabled).toBe(false);
      expect(DEFAULT_DELIBERATION_CONFIG.maxRounds).toBe(3);
      expect(DEFAULT_DELIBERATION_CONFIG.maxDeliberationTokens).toBe(50000);
      expect(DEFAULT_DELIBERATION_CONFIG.timeoutMs).toBe(120000);
      expect(DEFAULT_DELIBERATION_CONFIG.convergenceThreshold).toBe(0.8);
      expect(DEFAULT_DELIBERATION_CONFIG.leadReviewerModel).toBe('claude-3-opus-20240229');
    });
  });

  // --------------------------------------------------------------------------
  // compileBrief()
  // --------------------------------------------------------------------------

  describe('compileBrief()', () => {
    it('should compile findings grouped by persona', () => {
      const results: PersonaReviewOutput[] = [
        makePersonaResult({ persona: 'security', findings: [makeFinding({ id: 'f-1' })] }),
        makePersonaResult({ persona: 'perf', findings: [makeFinding({ id: 'f-2' })] }),
      ];

      const brief = engine.compileBrief('session-1', results, 'const x = 1;');

      expect(brief.sessionId).toBe('session-1');
      expect(brief.codeContext).toBe('const x = 1;');
      expect(Object.keys(brief.findingsByPersona)).toHaveLength(2);
      expect(brief.findingsByPersona['security']).toHaveLength(1);
      expect(brief.findingsByPersona['perf']).toHaveLength(1);
      expect(brief.tensions).toEqual([]);
    });

    it('should handle empty persona results', () => {
      const brief = engine.compileBrief('session-2', [], 'code');

      expect(brief.sessionId).toBe('session-2');
      expect(Object.keys(brief.findingsByPersona)).toHaveLength(0);
      expect(brief.tensions).toEqual([]);
    });

    it('should handle persona with no findings', () => {
      const results = [makePersonaResult({ persona: 'empty-persona', findings: [] })];
      const brief = engine.compileBrief('s', results, 'code');

      expect(brief.findingsByPersona['empty-persona']).toEqual([]);
    });

    it('should handle multiple findings per persona', () => {
      const findings = [
        makeFinding({ id: 'f-1', title: 'Issue 1' }),
        makeFinding({ id: 'f-2', title: 'Issue 2' }),
        makeFinding({ id: 'f-3', title: 'Issue 3' }),
      ];
      const results = [makePersonaResult({ persona: 'sec', findings })];
      const brief = engine.compileBrief('s', results, 'code');

      expect(brief.findingsByPersona['sec']).toHaveLength(3);
    });
  });

  // --------------------------------------------------------------------------
  // shouldContinue()
  // --------------------------------------------------------------------------

  describe('shouldContinue()', () => {
    it('should return true when no limits are hit', () => {
      // Need to set startTime by simulating run start
      (engine as any).startTime = Date.now();
      (engine as any).totalTokens = 0;

      const result = engine.shouldContinue([], []);
      expect(result).toBe(true);
    });

    it('should return false when timeout is exceeded', () => {
      (engine as any).startTime = Date.now() - 200000; // well past 120s default
      (engine as any).totalTokens = 0;

      expect(engine.shouldContinue([], [])).toBe(false);
    });

    it('should return false when token budget is exceeded', () => {
      (engine as any).startTime = Date.now();
      (engine as any).totalTokens = 60000; // over 50000 default

      expect(engine.shouldContinue([], [])).toBe(false);
    });

    it('should return false when convergence threshold is reached', () => {
      (engine as any).startTime = Date.now();
      (engine as any).totalTokens = 0;

      const tensions: Tension[] = [
        makeTension({ id: 't-1', resolved: true }),
        makeTension({ id: 't-2', resolved: true }),
        makeTension({ id: 't-3', resolved: true }),
        makeTension({ id: 't-4', resolved: true }),
        makeTension({ id: 't-5', resolved: false }),
      ];

      // 4/5 = 0.8, which equals the default threshold
      expect(engine.shouldContinue([], tensions)).toBe(false);
    });

    it('should return true when convergence is below threshold', () => {
      (engine as any).startTime = Date.now();
      (engine as any).totalTokens = 0;

      const tensions: Tension[] = [
        makeTension({ id: 't-1', resolved: true }),
        makeTension({ id: 't-2', resolved: false }),
        makeTension({ id: 't-3', resolved: false }),
      ];

      // 1/3 = 0.33, below 0.8 threshold
      expect(engine.shouldContinue([], tensions)).toBe(true);
    });

    it('should return true when tensions array is empty', () => {
      (engine as any).startTime = Date.now();
      (engine as any).totalTokens = 0;

      expect(engine.shouldContinue([], [])).toBe(true);
    });

    it('should return true when tensions is undefined', () => {
      (engine as any).startTime = Date.now();
      (engine as any).totalTokens = 0;

      expect(engine.shouldContinue([])).toBe(true);
    });

    it('should respect custom timeout', () => {
      const customEngine = new DeliberationEngine({ timeoutMs: 1000 });
      (customEngine as any).startTime = Date.now() - 1500;
      (customEngine as any).totalTokens = 0;

      expect(customEngine.shouldContinue([], [])).toBe(false);
    });

    it('should respect custom token budget', () => {
      const customEngine = new DeliberationEngine({ maxDeliberationTokens: 100 });
      (customEngine as any).startTime = Date.now();
      (customEngine as any).totalTokens = 150;

      expect(customEngine.shouldContinue([], [])).toBe(false);
    });

    it('should respect custom convergence threshold', () => {
      const customEngine = new DeliberationEngine({ convergenceThreshold: 0.5 });
      (customEngine as any).startTime = Date.now();
      (customEngine as any).totalTokens = 0;

      const tensions: Tension[] = [
        makeTension({ id: 't-1', resolved: true }),
        makeTension({ id: 't-2', resolved: false }),
      ];

      // 1/2 = 0.5, equals threshold
      expect(customEngine.shouldContinue([], tensions)).toBe(false);
    });
  });

  // --------------------------------------------------------------------------
  // parseChallengeResponse()
  // --------------------------------------------------------------------------

  describe('parseChallengeResponse()', () => {
    const challenge = makeChallenge();

    it('should parse support stance and confidence', () => {
      const text = 'I agree. **Stance**: support\n**Confidence**: 0.9';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.challengeId).toBe('c-1');
      expect(response.persona).toBe('security');
      expect(response.tensionId).toBe('t-1');
      expect(response.stance).toBe('support');
      expect(response.confidence).toBe(0.9);
      expect(response.argument).toBe(text);
    });

    it('should parse oppose stance', () => {
      const text = '**Stance**: oppose\n**Confidence**: 0.85';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('oppose');
      expect(response.confidence).toBe(0.85);
    });

    it('should parse revise stance', () => {
      const text = '**Stance**: revise\n**Confidence**: 0.6';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('revise');
      expect(response.confidence).toBe(0.6);
    });

    it('should parse withdraw stance', () => {
      const text = '**Stance**: withdraw\n**Confidence**: 0.3';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('withdraw');
      expect(response.confidence).toBe(0.3);
    });

    it('should default stance to support when missing', () => {
      const text = 'Some response without stance markers. **Confidence**: 0.7';
      const response = engine.parseChallengeResponse(challenge, text);

      // When stance regex doesn't match, defaults to 'neutral', then
      // validStances check passes 'neutral' but it's not in the parsed
      // stanceMatch so it falls through to 'support' default
      expect(response.stance).toBe('support');
      expect(response.confidence).toBe(0.7);
    });

    it('should default confidence to 0.5 when missing', () => {
      const text = '**Stance**: oppose\nNo confidence marker here.';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('oppose');
      expect(response.confidence).toBe(0.5);
    });

    it('should default both stance and confidence when text has no markers', () => {
      const text = 'Just a plain text response with no structured markers.';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('support');
      expect(response.confidence).toBe(0.5);
    });

    it('should clamp confidence above 1 to 1', () => {
      const text = '**Stance**: support\n**Confidence**: 1.5';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.confidence).toBe(1);
    });

    it('should default to 0.5 when confidence has a negative sign (regex does not match negatives)', () => {
      const text = '**Stance**: support\n**Confidence**: -0.3';
      const response = engine.parseChallengeResponse(challenge, text);

      // The regex [\d.]+ cannot match starting with '-',
      // so the entire confidence regex fails and default 0.5 is used
      expect(response.confidence).toBe(0.5);
    });

    it('should be case-insensitive for stance parsing', () => {
      const text = '**Stance**: OPPOSE\n**Confidence**: 0.7';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('oppose');
    });

    it('should be case-insensitive for confidence parsing', () => {
      const text = '**stance**: support\n**confidence**: 0.75';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.stance).toBe('support');
      expect(response.confidence).toBe(0.75);
    });

    it('should store original text as argument', () => {
      const text = 'Long argument with details.\n**Stance**: revise\n**Confidence**: 0.6';
      const response = engine.parseChallengeResponse(challenge, text);

      expect(response.argument).toBe(text);
    });

    it('should use challenge persona and tensionId', () => {
      const customChallenge = makeChallenge({
        id: 'c-99',
        toPersona: 'architect',
        tensionId: 't-42',
      });
      const text = '**Stance**: support\n**Confidence**: 0.8';
      const response = engine.parseChallengeResponse(customChallenge, text);

      expect(response.challengeId).toBe('c-99');
      expect(response.persona).toBe('architect');
      expect(response.tensionId).toBe('t-42');
    });
  });

  // --------------------------------------------------------------------------
  // updateTensions()
  // --------------------------------------------------------------------------

  describe('updateTensions()', () => {
    it('should update existing position for a persona', () => {
      const tensions = [makeTension()];
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-1',
          stance: 'support',
          argument: 'Changed my mind',
          confidence: 0.9,
        },
      ];

      engine.updateTensions(tensions, responses);

      const securityPos = tensions[0].positions.find((p) => p.persona === 'security');
      expect(securityPos?.stance).toBe('support');
      expect(securityPos?.argument).toBe('Changed my mind');
      expect(securityPos?.confidence).toBe(0.9);
    });

    it('should add new position for unknown persona', () => {
      const tensions = [makeTension()];
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'architect',
          tensionId: 't-1',
          stance: 'support',
          argument: 'I agree',
          confidence: 0.7,
        },
      ];

      engine.updateTensions(tensions, responses);

      expect(tensions[0].positions).toHaveLength(3); // original 2 + new 1
      const architectPos = tensions[0].positions.find((p) => p.persona === 'architect');
      expect(architectPos?.stance).toBe('support');
    });

    it('should resolve tension when no positions have oppose stance', () => {
      const tensions = [makeTension()]; // security has 'oppose'
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-1',
          stance: 'support', // changing from oppose to support
          argument: 'I now agree',
          confidence: 0.85,
        },
      ];

      engine.updateTensions(tensions, responses);

      expect(tensions[0].resolved).toBe(true);
      expect(tensions[0].resolution).toContain('Resolved after deliberation');
    });

    it('should not resolve tension when opposing positions remain', () => {
      const tensions = [makeTension()];
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'perf',
          tensionId: 't-1',
          stance: 'oppose', // perf now opposes (was support)
          argument: 'Changed my mind to oppose',
          confidence: 0.6,
        },
      ];

      engine.updateTensions(tensions, responses);

      // Both security and perf now oppose
      expect(tensions[0].resolved).toBe(false);
    });

    it('should map revise stance to neutral', () => {
      const tensions = [makeTension()]; // security opposes
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-1',
          stance: 'revise',
          argument: 'Revised position',
          confidence: 0.7,
        },
      ];

      engine.updateTensions(tensions, responses);

      const securityPos = tensions[0].positions.find((p) => p.persona === 'security');
      expect(securityPos?.stance).toBe('neutral');
    });

    it('should map withdraw stance to neutral', () => {
      const tensions = [makeTension()]; // security opposes
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-1',
          stance: 'withdraw',
          argument: 'Withdrawing',
          confidence: 0.3,
        },
      ];

      engine.updateTensions(tensions, responses);

      const securityPos = tensions[0].positions.find((p) => p.persona === 'security');
      expect(securityPos?.stance).toBe('neutral');
      // With security now neutral, no oppose remains -> resolved
      expect(tensions[0].resolved).toBe(true);
    });

    it('should skip responses for unknown tension IDs', () => {
      const tensions = [makeTension({ id: 't-1' })];
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-nonexistent',
          stance: 'support',
          argument: 'Irrelevant',
          confidence: 0.5,
        },
      ];

      engine.updateTensions(tensions, responses);

      // Original positions unchanged
      const securityPos = tensions[0].positions.find((p) => p.persona === 'security');
      expect(securityPos?.stance).toBe('oppose');
    });

    it('should not re-resolve already resolved tensions', () => {
      const tensions = [makeTension({ resolved: true, resolution: 'Already done' })];
      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-1',
          stance: 'oppose',
          argument: 'Reopening',
          confidence: 0.9,
        },
      ];

      engine.updateTensions(tensions, responses);

      // Position gets updated but resolved status preserved
      expect(tensions[0].resolved).toBe(true);
      expect(tensions[0].resolution).toBe('Already done');
    });

    it('should handle empty responses array', () => {
      const tensions = [makeTension()];
      engine.updateTensions(tensions, []);

      expect(tensions[0].positions).toHaveLength(2);
      expect(tensions[0].resolved).toBe(false);
    });

    it('should handle multiple responses for multiple tensions', () => {
      const tensions = [
        makeTension({ id: 't-1' }),
        makeTension({
          id: 't-2',
          positions: [
            { persona: 'arch', stance: 'oppose', argument: 'Bad idea', confidence: 0.7 },
          ],
        }),
      ];

      const responses: ChallengeResponse[] = [
        {
          challengeId: 'c-1',
          persona: 'security',
          tensionId: 't-1',
          stance: 'support',
          argument: 'Agree now',
          confidence: 0.9,
        },
        {
          challengeId: 'c-2',
          persona: 'arch',
          tensionId: 't-2',
          stance: 'revise',
          argument: 'OK, revised',
          confidence: 0.6,
        },
      ];

      engine.updateTensions(tensions, responses);

      expect(tensions[0].resolved).toBe(true); // security no longer opposes
      expect(tensions[1].resolved).toBe(true); // arch now neutral (from revise)
    });
  });

  // --------------------------------------------------------------------------
  // run()
  // --------------------------------------------------------------------------

  describe('run()', () => {
    it('should run full deliberation flow', async () => {
      const leadReviewer = createMockLeadReviewer();
      const securityAgent = createMockChallengeable(
        '**Stance**: support\n**Confidence**: 0.9\nI now agree.',
      );
      const personaAgents = new Map<string, Challengeable>([
        ['security', securityAgent],
      ]);

      const results = [makePersonaResult()];
      const memo = await engine.run('s-1', results, 'code', leadReviewer, personaAgents);

      expect(memo.sessionId).toBe('test-session');
      expect(memo.decision).toBe('GO');
      expect(leadReviewer.identifyTensions).toHaveBeenCalledOnce();
      expect(leadReviewer.formulateChallenges).toHaveBeenCalled();
      expect(leadReviewer.synthesizeMemo).toHaveBeenCalled();
    });

    it('should handle lead reviewer identifyTensions failure gracefully', async () => {
      const leadReviewer = createMockLeadReviewer({
        identifyTensions: vi.fn().mockRejectedValue(new Error('API error')),
        formulateChallenges: vi.fn().mockResolvedValue([]),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      // Should still produce a memo (graceful degradation)
      expect(memo).toBeDefined();
      expect(leadReviewer.synthesizeMemo).toHaveBeenCalled();
    });

    it('should handle lead reviewer formulateChallenges failure', async () => {
      const leadReviewer = createMockLeadReviewer({
        formulateChallenges: vi.fn().mockRejectedValue(new Error('Failed')),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      // Should still synthesize a memo even with no rounds
      expect(memo).toBeDefined();
      expect(leadReviewer.synthesizeMemo).toHaveBeenCalled();
    });

    it('should handle lead reviewer synthesizeMemo failure', async () => {
      const leadReviewer = createMockLeadReviewer({
        identifyTensions: vi.fn().mockResolvedValue([]),
        synthesizeMemo: vi.fn().mockRejectedValue(new Error('Synthesis failed')),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      // Graceful fallback memo
      expect(memo.sessionId).toBe('s-1');
      expect(memo.decision).toBe('CONDITIONAL');
      expect(memo.summary).toContain('memo synthesis failed');
    });

    it('should use default response when persona agent is not in map', async () => {
      const challenge = makeChallenge({ toPersona: 'nonexistent-persona' });
      const leadReviewer = createMockLeadReviewer({
        formulateChallenges: vi.fn().mockResolvedValue([challenge]),
      });
      const personaAgents = new Map<string, Challengeable>(); // empty

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
    });

    it('should use default response when persona agent throws', async () => {
      const leadReviewer = createMockLeadReviewer();
      const failingAgent: Challengeable = {
        challenge: vi.fn().mockRejectedValue(new Error('Agent down')),
      };
      const personaAgents = new Map<string, Challengeable>([
        ['security', failingAgent],
      ]);

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
      expect(failingAgent.challenge).toHaveBeenCalled();
    });

    it('should stop when no unresolved tensions remain', async () => {
      // identifyTensions returns all resolved
      const leadReviewer = createMockLeadReviewer({
        identifyTensions: vi.fn().mockResolvedValue([
          makeTension({ resolved: true, resolution: 'Pre-resolved' }),
        ]),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
      // formulateChallenges should not be called since all tensions are resolved
      expect(leadReviewer.formulateChallenges).not.toHaveBeenCalled();
    });

    it('should stop when formulateChallenges returns empty', async () => {
      const leadReviewer = createMockLeadReviewer({
        formulateChallenges: vi.fn().mockResolvedValue([]),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
      expect(leadReviewer.synthesizeMemo).toHaveBeenCalled();
    });

    it('should run multiple rounds when tensions persist', async () => {
      let callCount = 0;
      const leadReviewer = createMockLeadReviewer({
        identifyTensions: vi.fn().mockResolvedValue([
          makeTension({ id: 't-1' }),
        ]),
        formulateChallenges: vi.fn().mockImplementation(async () => {
          callCount++;
          if (callCount >= 3) {
            return []; // stop after 2 rounds
          }
          return [makeChallenge()];
        }),
      });

      // Agent always opposes, keeping tension alive
      const stubbornAgent = createMockChallengeable(
        '**Stance**: oppose\n**Confidence**: 0.95',
      );
      const personaAgents = new Map<string, Challengeable>([
        ['security', stubbornAgent],
      ]);

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
      // formulateChallenges called at least twice
      expect((leadReviewer.formulateChallenges as any).mock.calls.length).toBeGreaterThanOrEqual(2);
    });

    it('should respect maxRounds limit', async () => {
      const limitedEngine = new DeliberationEngine({ maxRounds: 1 });
      const leadReviewer = createMockLeadReviewer();

      const stubbornAgent = createMockChallengeable(
        '**Stance**: oppose\n**Confidence**: 0.95',
      );
      const personaAgents = new Map<string, Challengeable>([
        ['security', stubbornAgent],
      ]);

      const memo = await limitedEngine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
      // Only 1 round should have happened
      expect((leadReviewer.formulateChallenges as any).mock.calls.length).toBeLessThanOrEqual(1);
    });

    it('should handle empty persona results', async () => {
      const leadReviewer = createMockLeadReviewer({
        identifyTensions: vi.fn().mockResolvedValue([]),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-empty', [], 'code', leadReviewer, personaAgents);

      expect(memo).toBeDefined();
    });

    it('should update totalTokens and durationMs on the memo', async () => {
      const leadReviewer = createMockLeadReviewer({
        identifyTensions: vi.fn().mockResolvedValue([]),
        getTokensUsed: vi.fn().mockReturnValue(500),
      });
      const personaAgents = new Map<string, Challengeable>();

      const memo = await engine.run('s-1', [makePersonaResult()], 'code', leadReviewer, personaAgents);

      // totalTokens should include lead reviewer usage
      expect(memo.totalTokens).toBeGreaterThanOrEqual(500);
      expect(memo.durationMs).toBeGreaterThanOrEqual(0);
    });
  });

  // --------------------------------------------------------------------------
  // getStats()
  // --------------------------------------------------------------------------

  describe('getStats()', () => {
    it('should compute correct statistics for resolved tensions', () => {
      const rounds = [makeRound()];
      const tensions = [
        makeTension({ id: 't-1', resolved: true }),
        makeTension({ id: 't-2', resolved: false }),
      ];

      const stats = engine.getStats(rounds, tensions);

      expect(stats.totalRounds).toBe(1);
      expect(stats.totalChallenges).toBe(1);
      expect(stats.tensionsResolved).toBe(1);
      expect(stats.tensionsUnresolved).toBe(1);
      expect(stats.convergenceRatio).toBe(0.5);
    });

    it('should count revised and withdrawn findings', () => {
      const rounds: DeliberationRound[] = [
        makeRound({
          responses: [
            {
              challengeId: 'c-1',
              persona: 'sec',
              tensionId: 't-1',
              stance: 'revise',
              argument: 'Revised',
              confidence: 0.7,
            },
            {
              challengeId: 'c-2',
              persona: 'perf',
              tensionId: 't-1',
              stance: 'withdraw',
              argument: 'Withdrawn',
              confidence: 0.2,
            },
            {
              challengeId: 'c-3',
              persona: 'arch',
              tensionId: 't-1',
              stance: 'support',
              argument: 'Agree',
              confidence: 0.9,
            },
          ],
        }),
      ];

      const stats = engine.getStats(rounds, []);

      expect(stats.findingsRevised).toBe(1);
      expect(stats.findingsWithdrawn).toBe(1);
    });

    it('should compute per-persona token usage', () => {
      const rounds: DeliberationRound[] = [
        makeRound({
          responses: [
            {
              challengeId: 'c-1',
              persona: 'sec',
              tensionId: 't-1',
              stance: 'support',
              argument: 'A'.repeat(400), // 100 tokens
              confidence: 0.9,
            },
            {
              challengeId: 'c-2',
              persona: 'perf',
              tensionId: 't-1',
              stance: 'support',
              argument: 'B'.repeat(200), // 50 tokens
              confidence: 0.8,
            },
          ],
          tokenUsage: 200,
        }),
      ];

      const stats = engine.getStats(rounds, []);

      expect(stats.tokenUsage.personas['sec']).toBe(100);
      expect(stats.tokenUsage.personas['perf']).toBe(50);
      expect(stats.tokenUsage.total).toBe(200);
    });

    it('should handle empty rounds', () => {
      const stats = engine.getStats([], []);

      expect(stats.totalRounds).toBe(0);
      expect(stats.totalChallenges).toBe(0);
      expect(stats.tensionsResolved).toBe(0);
      expect(stats.tensionsUnresolved).toBe(0);
      expect(stats.findingsRevised).toBe(0);
      expect(stats.findingsWithdrawn).toBe(0);
      expect(stats.convergenceRatio).toBe(1); // 0 tensions = fully converged
      expect(stats.tokenUsage.total).toBe(0);
    });

    it('should handle all tensions resolved', () => {
      const tensions = [
        makeTension({ resolved: true }),
        makeTension({ id: 't-2', resolved: true }),
      ];

      const stats = engine.getStats([], tensions);

      expect(stats.convergenceRatio).toBe(1);
      expect(stats.tensionsResolved).toBe(2);
      expect(stats.tensionsUnresolved).toBe(0);
    });

    it('should handle no tensions resolved', () => {
      const tensions = [
        makeTension({ resolved: false }),
        makeTension({ id: 't-2', resolved: false }),
      ];

      const stats = engine.getStats([], tensions);

      expect(stats.convergenceRatio).toBe(0);
      expect(stats.tensionsResolved).toBe(0);
      expect(stats.tensionsUnresolved).toBe(2);
    });

    it('should aggregate across multiple rounds', () => {
      const rounds: DeliberationRound[] = [
        makeRound({
          roundNumber: 1,
          challenges: [makeChallenge({ id: 'c-1' }), makeChallenge({ id: 'c-2' })],
          responses: [
            {
              challengeId: 'c-1',
              persona: 'sec',
              tensionId: 't-1',
              stance: 'revise',
              argument: 'Round 1',
              confidence: 0.6,
            },
          ],
          tokenUsage: 300,
        }),
        makeRound({
          roundNumber: 2,
          challenges: [makeChallenge({ id: 'c-3' })],
          responses: [
            {
              challengeId: 'c-3',
              persona: 'sec',
              tensionId: 't-1',
              stance: 'withdraw',
              argument: 'Round 2',
              confidence: 0.3,
            },
          ],
          tokenUsage: 200,
        }),
      ];

      const stats = engine.getStats(rounds, []);

      expect(stats.totalRounds).toBe(2);
      expect(stats.totalChallenges).toBe(3); // 2 + 1
      expect(stats.findingsRevised).toBe(1);
      expect(stats.findingsWithdrawn).toBe(1);
      expect(stats.tokenUsage.total).toBe(500);
    });

    it('should compute lead reviewer token usage as remainder', () => {
      const rounds: DeliberationRound[] = [
        makeRound({
          responses: [
            {
              challengeId: 'c-1',
              persona: 'sec',
              tensionId: 't-1',
              stance: 'support',
              argument: 'X'.repeat(80), // 20 tokens
              confidence: 0.9,
            },
          ],
          tokenUsage: 100,
        }),
      ];

      const stats = engine.getStats(rounds, []);

      expect(stats.tokenUsage.leadReviewer).toBe(80); // 100 - 20
      expect(stats.tokenUsage.personas['sec']).toBe(20);
    });
  });
});
