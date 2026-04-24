/**
 * Tests for Agency-Agents collaboration patterns
 * Covers voting, confidence, lateral thinking, and scope detection
 */

import { describe, it, expect } from 'vitest';
import { getVoteWeight, aggregateVotes } from '../../src/review-engine.js';
import type { Finding, Persona } from '../../src/types.js';

describe('agency-agents patterns', () => {
  describe('getVoteWeight', () => {
    it('should return 3.0 for tier 1 personas', () => {
      expect(getVoteWeight(1)).toBe(3.0);
    });

    it('should return 1.0 for tier 2 personas', () => {
      expect(getVoteWeight(2)).toBe(1.0);
    });

    it('should return 0.5 for tier 3 personas', () => {
      expect(getVoteWeight(3)).toBe(0.5);
    });

    it('should return 1.0 for undefined tier (default)', () => {
      expect(getVoteWeight(undefined)).toBe(1.0);
    });
  });

  describe('aggregateVotes', () => {
    const tier1Persona: Persona = {
      name: 'security-engineer',
      displayName: 'Security Engineer',
      version: '1.0.0',
      description: 'Security expert',
      focusAreas: ['security'],
      prompt: 'You are a security expert',
      tier: 1,
    };

    const tier2Persona: Persona = {
      name: 'code-health',
      displayName: 'Code Health',
      version: '1.0.0',
      description: 'Code quality expert',
      focusAreas: ['quality'],
      prompt: 'You are a code quality expert',
      tier: 2,
    };

    const tier3Persona: Persona = {
      name: 'junior-reviewer',
      displayName: 'Junior Reviewer',
      version: '1.0.0',
      description: 'Junior reviewer',
      focusAreas: ['general'],
      prompt: 'You are a junior reviewer',
      tier: 3,
    };

    it('should aggregate GO votes with tier weighting', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
        {
          id: '2',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['code-health'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
      ];

      const personas = [tier1Persona, tier2Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      expect(result[0].decision).toBe('GO');
      expect(result[0].metadata?.voteAggregation).toBeDefined();
      expect(result[0].metadata?.voteAggregation.goVotes).toBe(4.0); // 3.0 + 1.0
      expect(result[0].metadata?.voteAggregation.totalWeight).toBe(4.0);
    });

    it('should aggregate NO-GO votes with tier weighting', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'critical',
          personas: ['security-engineer'],
          title: 'Security issue',
          description: 'Critical security flaw',
          decision: 'NO-GO',
        },
      ];

      const personas = [tier1Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      expect(result[0].decision).toBe('NO-GO');
      expect(result[0].metadata?.voteAggregation.noGoVotes).toBe(3.0);
    });

    it('should use tier weighting to determine final decision', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'Test finding',
          description: 'Test',
          decision: 'NO-GO',
        },
        {
          id: '2',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['code-health'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
        {
          id: '3',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['junior-reviewer'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
      ];

      const personas = [tier1Persona, tier2Persona, tier3Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      // Tier 1 NO-GO (3.0) vs Tier 2 GO (1.0) + Tier 3 GO (0.5) = 3.0 NO-GO, 1.5 GO
      // Total weight = 4.5, GO ratio = 1.5/4.5 = 0.33 < 0.5, so NO-GO
      expect(result[0].decision).toBe('NO-GO');
      expect(result[0].metadata?.voteAggregation.goVotes).toBe(1.5);
      expect(result[0].metadata?.voteAggregation.noGoVotes).toBe(3.0);
      expect(result[0].metadata?.voteAggregation.totalWeight).toBe(4.5);
    });

    it('should respect custom vote threshold', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['code-health'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
      ];

      const personas = [tier2Persona];
      // With threshold 0.8, a 100% GO vote should still be GO
      const result = aggregateVotes(findings, personas, 0.8);

      expect(result).toHaveLength(1);
      expect(result[0].decision).toBe('GO');
      expect(result[0].metadata?.voteAggregation.goRatio).toBe(1.0);
    });

    it('should default to GO for findings without decision', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'info',
          personas: ['code-health'],
          title: 'Test finding',
          description: 'Test',
          // No decision field
        },
      ];

      const personas = [tier2Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      expect(result[0].decision).toBe('GO');
    });

    it('should merge personas from multiple findings', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
        {
          id: '2',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['code-health'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
      ];

      const personas = [tier1Persona, tier2Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      expect(result[0].personas).toContain('security-engineer');
      expect(result[0].personas).toContain('code-health');
      expect(result[0].personas).toHaveLength(2);
    });

    it('should group findings by file, line, and title', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'Issue A',
          description: 'Test',
          decision: 'GO',
        },
        {
          id: '2',
          file: 'test.ts',
          line: 20,
          severity: 'medium',
          personas: ['code-health'],
          title: 'Issue B',
          description: 'Test',
          decision: 'GO',
        },
        {
          id: '3',
          file: 'test.ts',
          line: 10,
          severity: 'medium',
          personas: ['code-health'],
          title: 'Issue A',
          description: 'Test',
          decision: 'GO',
        },
      ];

      const personas = [tier1Persona, tier2Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      // Should have 2 findings: one for line 10/Issue A (merged), one for line 20/Issue B
      expect(result).toHaveLength(2);

      const finding1 = result.find(f => f.line === 10 && f.title === 'Issue A');
      const finding2 = result.find(f => f.line === 20 && f.title === 'Issue B');

      expect(finding1).toBeDefined();
      expect(finding2).toBeDefined();
      expect(finding1!.personas).toHaveLength(2); // Merged from security-engineer and code-health
      expect(finding2!.personas).toHaveLength(1); // Only code-health
    });

    it('should handle missing line numbers', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'Test finding',
          description: 'Test',
          decision: 'GO',
        },
      ];

      const personas = [tier1Persona];
      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      expect(result[0].decision).toBe('GO');
    });
  });

  describe('confidence filtering', () => {
    it('should include findings above minimum confidence', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          severity: 'medium',
          personas: ['code-health'],
          title: 'High confidence',
          description: 'Test',
          confidence: 0.9,
        },
        {
          id: '2',
          file: 'test.ts',
          severity: 'medium',
          personas: ['code-health'],
          title: 'Low confidence',
          description: 'Test',
          confidence: 0.5,
        },
      ];

      const minConfidence = 0.7;
      const filtered = findings.filter(f => {
        const confidence = f.confidence ?? 1.0;
        return confidence >= minConfidence;
      });

      expect(filtered).toHaveLength(1);
      expect(filtered[0].title).toBe('High confidence');
    });

    it('should default to 1.0 confidence if not specified', () => {
      const finding: Finding = {
        id: '1',
        file: 'test.ts',
        severity: 'medium',
        personas: ['code-health'],
        title: 'No confidence',
        description: 'Test',
      };

      const confidence = finding.confidence ?? 1.0;
      expect(confidence).toBe(1.0);
    });
  });

  describe('lateral thinking alternatives', () => {
    it('should store alternatives in findings', () => {
      const finding: Finding = {
        id: '1',
        file: 'test.ts',
        line: 10,
        severity: 'medium',
        personas: ['security-engineer'],
        title: 'Security issue',
        description: 'Test',
        decision: 'NO-GO',
        alternatives: [
          'Use parameterized queries',
          'Implement input validation',
          'Use ORM with built-in escaping',
        ],
      };

      expect(finding.alternatives).toBeDefined();
      expect(finding.alternatives).toHaveLength(3);
      expect(finding.alternatives![0]).toBe('Use parameterized queries');
    });

    it('should handle missing alternatives gracefully', () => {
      const finding: Finding = {
        id: '1',
        file: 'test.ts',
        severity: 'medium',
        personas: ['code-health'],
        title: 'Test',
        description: 'Test',
      };

      expect(finding.alternatives).toBeUndefined();
    });
  });

  describe('expertise boundary detection', () => {
    it('should flag findings as out of scope', () => {
      const finding: Finding = {
        id: '1',
        file: 'test.ts',
        line: 10,
        severity: 'medium',
        personas: ['security-engineer'],
        title: 'Performance issue',
        description: 'This is outside security scope',
        decision: 'GO',
        outOfScope: true,
      };

      expect(finding.outOfScope).toBe(true);
    });

    it('should be undefined if not specified', () => {
      const finding: Finding = {
        id: '1',
        file: 'test.ts',
        severity: 'medium',
        personas: ['code-health'],
        title: 'Test',
        description: 'Test',
      };

      expect(finding.outOfScope).toBeUndefined();
    });

    it('should filter out-of-scope findings if needed', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'In scope explicit',
          description: 'Test',
          outOfScope: false,
        },
        {
          id: '2',
          file: 'test.ts',
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'In scope implicit',
          description: 'Test',
          // outOfScope undefined = in scope
        },
        {
          id: '3',
          file: 'test.ts',
          severity: 'medium',
          personas: ['security-engineer'],
          title: 'Out of scope',
          description: 'Test',
          outOfScope: true,
        },
      ];

      const inScope = findings.filter(f => f.outOfScope !== true);
      expect(inScope).toHaveLength(2);
      expect(inScope.map(f => f.title)).toContain('In scope explicit');
      expect(inScope.map(f => f.title)).toContain('In scope implicit');
    });
  });

  describe('integrated agency-agents workflow', () => {
    it('should support all agency-agents fields together', () => {
      const finding: Finding = {
        id: '1',
        file: 'test.ts',
        line: 10,
        severity: 'high',
        personas: ['security-engineer'],
        title: 'SQL Injection vulnerability',
        description: 'Direct string concatenation in SQL query',
        confidence: 0.95,
        decision: 'NO-GO',
        alternatives: [
          'Use parameterized queries with prepared statements',
          'Switch to an ORM like Prisma or TypeORM',
          'Implement input sanitization library',
        ],
        outOfScope: false,
      };

      expect(finding.confidence).toBe(0.95);
      expect(finding.decision).toBe('NO-GO');
      expect(finding.alternatives).toHaveLength(3);
      expect(finding.outOfScope).toBe(false);
    });

    it('should preserve agency-agents metadata through vote aggregation', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          line: 10,
          severity: 'high',
          personas: ['security-engineer'],
          title: 'Security issue',
          description: 'Test',
          decision: 'NO-GO',
          alternatives: ['Alt 1', 'Alt 2', 'Alt 3'],
          confidence: 0.9,
          outOfScope: false,
        },
      ];

      const personas: Persona[] = [
        {
          name: 'security-engineer',
          displayName: 'Security Engineer',
          version: '1.0.0',
          description: 'Security expert',
          focusAreas: ['security'],
          prompt: 'You are a security expert',
          tier: 1,
        },
      ];

      const result = aggregateVotes(findings, personas, 0.5);

      expect(result).toHaveLength(1);
      expect(result[0].alternatives).toEqual(['Alt 1', 'Alt 2', 'Alt 3']);
      expect(result[0].confidence).toBe(0.9);
      expect(result[0].outOfScope).toBe(false);
      expect(result[0].decision).toBe('NO-GO');
    });
  });
});
