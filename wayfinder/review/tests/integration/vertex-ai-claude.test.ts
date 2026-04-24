/**
 * Integration tests for VertexAI Claude client
 *
 * These tests require:
 * - VERTEX_PROJECT_ID environment variable
 * - VERTEX_LOCATION environment variable (optional, defaults to us-east5)
 * - Google Application Default Credentials configured
 *
 * Run with: npm test -- tests/integration/vertex-ai-claude.test.ts
 *
 * Skip if credentials not available: SKIP_INTEGRATION=1 npm test
 */

import { describe, it, expect, beforeAll } from 'vitest';
import { createVertexAIClaudeReviewer } from '../../src/vertex-ai-claude-client.js';
import type { Persona, PersonaReviewInput } from '../../src/types.js';

const SKIP_INTEGRATION = process.env.SKIP_INTEGRATION === '1';
const VERTEX_PROJECT_ID = process.env.VERTEX_PROJECT_ID;
const VERTEX_LOCATION = process.env.VERTEX_LOCATION || 'us-east5';
const VERTEX_MODEL = process.env.VERTEX_MODEL || 'claude-sonnet-4-5@20250929';

// Check if TLS/SSL connectivity to Google APIs is available.
// In sandboxed environments the CA certs may be missing, causing
// UNABLE_TO_GET_ISSUER_CERT_LOCALLY errors. Skip gracefully.
const HAS_TLS_CONNECTIVITY = (() => {
  try {
    const { execSync } = require('child_process');
    execSync('node -e "const https=require(\'https\');const r=https.request(\'https://oauth2.googleapis.com/.well-known/openid-configuration\',{timeout:3000},res=>{res.resume();res.on(\'end\',()=>process.exit(0))});r.on(\'error\',()=>process.exit(1));r.end()"', { timeout: 5000 });
    return true;
  } catch {
    return false;
  }
})();

const describeIntegration = SKIP_INTEGRATION || !VERTEX_PROJECT_ID || !HAS_TLS_CONNECTIVITY
  ? describe.skip
  : describe;

describeIntegration('VertexAI Claude Integration', () => {
  const mockPersona: Persona = {
    name: 'tech-lead',
    displayName: 'Tech Lead',
    version: '1.0.0',
    description: 'Technical lead reviewing code quality',
    focusAreas: ['code-quality', 'best-practices'],
    prompt: `You are a technical lead reviewing code quality.

Focus on:
- Code clarity and maintainability
- Potential bugs or edge cases
- Best practices and conventions

Return findings in JSON format with severity, message, file, line, category, and suggestion fields.`,
  };

  let reviewer: ReturnType<typeof createVertexAIClaudeReviewer>;

  beforeAll(() => {
    reviewer = createVertexAIClaudeReviewer({
      projectId: VERTEX_PROJECT_ID!,
      location: VERTEX_LOCATION,
      model: VERTEX_MODEL,
    });
  });

  it('should successfully review simple code', async () => {
    const input: PersonaReviewInput = {
      persona: mockPersona,
      files: [
        {
          path: 'test.py',
          content: `def add(a, b):
    return a + b

# TODO: add input validation`,
        },
      ],
      mode: 'quick',
    };

    const result = await reviewer(input);

    expect(result).toBeDefined();
    expect(result.persona).toBe('tech-lead');
    expect(result.findings).toBeInstanceOf(Array);
    expect(result.cost).toBeDefined();
    expect(result.cost.persona).toBe('tech-lead');
    expect(result.cost.inputTokens).toBeGreaterThan(0);
    expect(result.cost.outputTokens).toBeGreaterThan(0);
    expect(result.cost.cost).toBeGreaterThan(0);
  }, 30000); // 30s timeout for API call

  it('should detect code issues', async () => {
    const input: PersonaReviewInput = {
      persona: mockPersona,
      files: [
        {
          path: 'buggy.js',
          content: `function divide(a, b) {
    return a / b;  // No division by zero check
}

const result = divide(10, 0);  // Will return Infinity`,
        },
      ],
      mode: 'quick',
    };

    const result = await reviewer(input);

    expect(result.findings.length).toBeGreaterThan(0);

    // Should detect division by zero issue
    // Finding fields: title, description (mapped from raw message), categories (array, mapped from category)
    const hasDivisionIssue = result.findings.some(
      f => f.title?.toLowerCase().includes('division') ||
           f.title?.toLowerCase().includes('zero') ||
           f.description?.toLowerCase().includes('division') ||
           f.description?.toLowerCase().includes('zero') ||
           f.categories?.some((c: string) => c.includes('error-handling'))
    );

    expect(hasDivisionIssue).toBe(true);
  }, 30000);

  it('should handle empty file gracefully', async () => {
    const input: PersonaReviewInput = {
      persona: mockPersona,
      files: [
        {
          path: 'empty.txt',
          content: '',
        },
      ],
      mode: 'quick',
    };

    const result = await reviewer(input);

    expect(result).toBeDefined();
    expect(result.findings).toBeInstanceOf(Array);
    // Empty file should have no findings or minimal findings
    expect(result.findings.length).toBeLessThan(3);
  }, 30000);

  it('should track costs accurately', async () => {
    const input: PersonaReviewInput = {
      persona: mockPersona,
      files: [
        {
          path: 'simple.py',
          content: 'print("hello")',
        },
      ],
      mode: 'quick',
    };

    const result = await reviewer(input);

    expect(result.cost.inputTokens).toBeGreaterThan(0);
    expect(result.cost.outputTokens).toBeGreaterThan(0);

    // Verify cost calculation (Claude Sonnet 4.5 pricing)
    const expectedCost =
      (result.cost.inputTokens * 3.0 / 1_000_000) +
      (result.cost.outputTokens * 15.0 / 1_000_000);

    expect(result.cost.cost).toBeCloseTo(expectedCost, 8);
  }, 30000);

  it('should support multiple files in single review', async () => {
    const input: PersonaReviewInput = {
      persona: mockPersona,
      files: [
        {
          path: 'file1.js',
          content: 'const x = 1;',
        },
        {
          path: 'file2.js',
          content: 'const y = 2;',
        },
      ],
      mode: 'quick',
    };

    const result = await reviewer(input);

    expect(result).toBeDefined();
    expect(result.findings).toBeInstanceOf(Array);
  }, 30000);

  it('should respect temperature setting for consistency', async () => {
    const deterministicReviewer = createVertexAIClaudeReviewer({
      projectId: VERTEX_PROJECT_ID!,
      location: VERTEX_LOCATION,
      model: VERTEX_MODEL,
      temperature: 0.0, // Deterministic
    });

    const input: PersonaReviewInput = {
      persona: mockPersona,
      files: [
        {
          path: 'test.py',
          content: 'def foo(): pass',
        },
      ],
      mode: 'quick',
    };

    const result1 = await deterministicReviewer(input);
    const result2 = await deterministicReviewer(input);

    // With temperature=0, results should be very similar
    expect(result1.findings.length).toBe(result2.findings.length);
  }, 60000);
});
