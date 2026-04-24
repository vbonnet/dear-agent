/**
 * Tests for review engine
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mkdir, writeFile, rm } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import { execSync } from 'child_process';
import {
  reviewFiles,
  prepareContext,
  runPersonaReview,
  aggregateResults,
  createSummary,
  groupFindingsByFile,
  validateReviewConfig,
  ReviewEngineError,
  REVIEW_ENGINE_ERROR_CODES,
  type ReviewerFunction,
  type ReviewContext,
} from '../src/review-engine.js';
import type {
  ReviewConfig,
  PersonaReviewInput,
  PersonaReviewOutput,
  Finding,
  Persona,
} from '../src/types.js';

// Mock Anthropic SDK for sub-agent tests
vi.mock('@anthropic-ai/sdk', () => {
  class MockAPIError extends Error {
    status: number;
    constructor(status: number, body: any, message?: string) {
      super(message || JSON.stringify(body));
      this.status = status;
      this.name = 'APIError';
    }
  }

  const MockAnthropic = vi.fn().mockImplementation((config) => {
    const mockCreate = vi.fn().mockImplementation(async () => {
      // Fail for invalid API keys
      if (config?.apiKey === 'invalid-key') {
        throw new MockAPIError(401, { error: { message: 'Invalid API key' } }, '401 Invalid API key');
      }

      // Success for valid keys
      return {
        id: 'msg_test',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: '[]' }],
        model: 'claude-3-5-sonnet-20241022',
        stop_reason: 'end_turn',
        stop_sequence: null,
        usage: {
          input_tokens: 1500,
          output_tokens: 200,
          cache_creation_input_tokens: 800,
          cache_read_input_tokens: 0,
        },
      };
    });

    return {
      messages: {
        create: mockCreate
      }
    };
  });

  return {
    default: MockAnthropic,
    APIError: MockAPIError
  };
});

describe('review-engine', () => {
  let testDir: string;

  // Mock persona for testing
  const mockPersona: Persona = {
    name: 'test-persona',
    displayName: 'Test Persona',
    version: '1.0.0',
    description: 'Test persona for unit tests',
    focusAreas: ['testing'],
    prompt: 'You are a test persona',
  };

  beforeEach(async () => {
    testDir = join(tmpdir(), `multi-persona-review-test-${Date.now()}-${Math.random().toString(36).substring(7)}`);
    await mkdir(testDir, { recursive: true });

    // Initialize git repo
    try {
      execSync('git init', { cwd: testDir, stdio: 'pipe' });
      execSync('git config user.email "test@test.com"', { cwd: testDir, stdio: 'pipe' });
      execSync('git config user.name "Test User"', { cwd: testDir, stdio: 'pipe' });
    } catch (error) {
      // Git init can fail in some test environments, that's OK
      console.warn('Git init failed in test setup:', error);
    }
  });

  afterEach(async () => {
    try {
      await rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  describe('validateReviewConfig', () => {
    it('should validate correct config', () => {
      const config: ReviewConfig = {
        files: ['test.ts'],
        personas: [mockPersona],
        mode: 'quick',
      };

      expect(() => validateReviewConfig(config)).not.toThrow();
    });

    it('should throw on empty files', () => {
      const config: ReviewConfig = {
        files: [],
        personas: [mockPersona],
        mode: 'quick',
      };

      expect(() => validateReviewConfig(config)).toThrow(ReviewEngineError);
      expect(() => validateReviewConfig(config)).toThrow(/at least one file/);
    });

    it('should throw on empty personas', () => {
      const config: ReviewConfig = {
        files: ['test.ts'],
        personas: [],
        mode: 'quick',
      };

      expect(() => validateReviewConfig(config)).toThrow(ReviewEngineError);
      expect(() => validateReviewConfig(config)).toThrow(/at least one persona/);
    });

    it('should throw on missing mode', () => {
      const config = {
        files: ['test.ts'],
        personas: [mockPersona],
      } as ReviewConfig;

      expect(() => validateReviewConfig(config)).toThrow(ReviewEngineError);
      expect(() => validateReviewConfig(config)).toThrow(/must specify a mode/);
    });
  });

  describe('prepareContext', () => {
    it('should prepare context for full file scan', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
      };

      const context = await prepareContext(config, testDir);

      expect(context.files).toHaveLength(1);
      expect(context.files[0].path).toBe('test.ts');
      expect(context.files[0].content).toBe('const x = 1;');
      expect(context.files[0].isDiff).toBe(false);
      expect(context.sessionId).toBeDefined();
      expect(context.startTime).toBeInstanceOf(Date);
    });

    it('should throw on no files found after filtering', async () => {
      // Create a directory with only binary files
      const binDir = join(testDir, 'bin');
      await mkdir(binDir);
      await writeFile(join(binDir, 'binary.bin'), Buffer.from([0x00, 0x01, 0x02]));

      const config: ReviewConfig = {
        files: [binDir],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
      };

      // Binary files are filtered out, so no files will be found
      await expect(prepareContext(config, testDir)).rejects.toThrow(
        ReviewEngineError
      );
      await expect(prepareContext(config, testDir)).rejects.toThrow(/No files found/);
    });

    it.skip('should use changed mode for quick reviews by default', async () => {
      // Skip this test due to git signing environment issues
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');
      execSync('git add .', { cwd: testDir });
      execSync('git commit -m "initial"', { cwd: testDir });

      // Modify file
      await writeFile(filePath, 'const x = 2;');

      const config: ReviewConfig = {
        files: [testDir],
        personas: [mockPersona],
        mode: 'quick',
        // No fileScanMode specified - should default to 'changed'
      };

      const context = await prepareContext(config, testDir);

      // Should find the changed file
      expect(context.files).toHaveLength(1);
      expect(context.files[0].isDiff).toBe(true); // Changed mode uses diffs
    });
  });

  describe('runPersonaReview', () => {
    it('should run successful review', async () => {
      const mockReviewer: ReviewerFunction = async (
        input: PersonaReviewInput
      ): Promise<PersonaReviewOutput> => {
        const finding: Finding = {
          id: 'test-1',
          file: input.files[0].path,
          line: 1,
          severity: 'medium',
          personas: [input.persona.name],
          title: 'Test finding',
          description: 'Test description',
        };

        return {
          persona: input.persona.name,
          findings: [finding],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      const input: PersonaReviewInput = {
        persona: mockPersona,
        files: [
          {
            path: 'test.ts',
            content: 'const x = 1;',
            isDiff: false,
          },
        ],
        mode: 'quick',
        options: {},
      };

      const result = await runPersonaReview('test-persona', input, mockReviewer);

      expect(result.persona).toBe('test-persona');
      expect(result.findings).toHaveLength(1);
      expect(result.findings[0].title).toBe('Test finding');
      expect(result.cost.cost).toBe(0.01);
      expect(result.errors).toBeUndefined();
    });

    it('should handle reviewer errors gracefully', async () => {
      const mockReviewer: ReviewerFunction = async () => {
        throw new Error('API error');
      };

      const input: PersonaReviewInput = {
        persona: mockPersona,
        files: [
          {
            path: 'test.ts',
            content: 'const x = 1;',
            isDiff: false,
          },
        ],
        mode: 'quick',
        options: {},
      };

      const result = await runPersonaReview('test-persona', input, mockReviewer);

      expect(result.persona).toBe('test-persona');
      expect(result.findings).toHaveLength(0);
      expect(result.errors).toHaveLength(1);
      expect(result.errors![0].message).toContain('API error');
    });
  });

  describe('createSummary', () => {
    it('should create correct summary', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'test.ts',
          severity: 'critical',
          personas: ['test'],
          title: 'Critical issue',
          description: 'desc',
        },
        {
          id: '2',
          file: 'test.ts',
          severity: 'medium',
          personas: ['test'],
          title: 'Medium issue',
          description: 'desc',
        },
        {
          id: '3',
          file: 'test.ts',
          severity: 'medium',
          personas: ['test'],
          title: 'Another medium',
          description: 'desc',
        },
      ];

      const summary = createSummary(2, findings);

      expect(summary.filesReviewed).toBe(2);
      expect(summary.totalFindings).toBe(3);
      expect(summary.findingsBySeverity.critical).toBe(1);
      expect(summary.findingsBySeverity.high).toBe(0);
      expect(summary.findingsBySeverity.medium).toBe(2);
      expect(summary.findingsBySeverity.low).toBe(0);
      expect(summary.findingsBySeverity.info).toBe(0);
    });
  });

  describe('groupFindingsByFile', () => {
    it('should group findings by file', () => {
      const findings: Finding[] = [
        {
          id: '1',
          file: 'file1.ts',
          severity: 'critical',
          personas: ['test'],
          title: 'Issue 1',
          description: 'desc',
        },
        {
          id: '2',
          file: 'file2.ts',
          severity: 'medium',
          personas: ['test'],
          title: 'Issue 2',
          description: 'desc',
        },
        {
          id: '3',
          file: 'file1.ts',
          severity: 'low',
          personas: ['test'],
          title: 'Issue 3',
          description: 'desc',
        },
      ];

      const grouped = groupFindingsByFile(findings);

      expect(Object.keys(grouped)).toHaveLength(2);
      expect(grouped['file1.ts']).toHaveLength(2);
      expect(grouped['file2.ts']).toHaveLength(1);
      expect(grouped['file1.ts'][0].id).toBe('1');
      expect(grouped['file1.ts'][1].id).toBe('3');
    });
  });

  describe('aggregateResults', () => {
    it('should aggregate results from multiple personas', () => {
      const config: ReviewConfig = {
        files: ['test.ts'],
        personas: [mockPersona],
        mode: 'quick',
        options: {
          deduplicate: false, // Disable deduplication for this test
        },
      };

      const context: ReviewContext = {
        files: [
          {
            path: 'test.ts',
            content: 'const x = 1;',
            isDiff: false,
          },
        ],
        cwd: testDir,
        sessionId: 'test-session',
        startTime: new Date(),
      };

      const personaResults: PersonaReviewOutput[] = [
        {
          persona: 'persona1',
          findings: [
            {
              id: '1',
              file: 'test.ts',
              severity: 'high',
              personas: ['persona1'],
              title: 'Finding 1',
              description: 'desc',
            },
          ],
          cost: {
            persona: 'persona1',
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        },
        {
          persona: 'persona2',
          findings: [
            {
              id: '2',
              file: 'test.ts',
              severity: 'medium',
              personas: ['persona2'],
              title: 'Finding 2',
              description: 'desc',
            },
          ],
          cost: {
            persona: 'persona2',
            cost: 0.02,
            inputTokens: 200,
            outputTokens: 100,
          },
        },
      ];

      const result = aggregateResults(config, context, personaResults);

      expect(result.sessionId).toBe('test-session');
      expect(result.findings).toHaveLength(2);
      expect(result.summary.totalFindings).toBe(2);
      expect(result.cost.totalCost).toBe(0.03);
      expect(result.cost.totalTokens).toBe(450);
      expect(result.cost.byPersona['persona1'].cost).toBe(0.01);
      expect(result.cost.byPersona['persona2'].cost).toBe(0.02);
    });
  });

  describe('reviewFiles', () => {
    it('should complete full review workflow', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
      };

      const mockReviewer: ReviewerFunction = async (
        input: PersonaReviewInput
      ): Promise<PersonaReviewOutput> => {
        return {
          persona: input.persona.name,
          findings: [
            {
              id: 'test-1',
              file: input.files[0].path,
              line: 1,
              severity: 'low',
              personas: [input.persona.name],
              title: 'Test finding',
              description: 'Test description',
            },
          ],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      const result = await reviewFiles(config, testDir, mockReviewer);

      expect(result.findings).toHaveLength(1);
      expect(result.findings[0].title).toBe('Test finding');
      expect(result.summary.filesReviewed).toBe(1);
      expect(result.summary.totalFindings).toBe(1);
      expect(result.cost.totalCost).toBe(0.01);
    });

    it('should throw when all personas fail', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
      };

      const mockReviewer: ReviewerFunction = async () => {
        throw new Error('All personas failed');
      };

      await expect(reviewFiles(config, testDir, mockReviewer)).rejects.toThrow(
        ReviewEngineError
      );
      await expect(reviewFiles(config, testDir, mockReviewer)).rejects.toThrow(
        /All personas failed/
      );
    });

    it('should handle multiple personas sequentially', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const persona2: Persona = {
        ...mockPersona,
        name: 'persona2',
        displayName: 'Persona 2',
      };

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona, persona2],
        mode: 'quick',
        fileScanMode: 'full',
        options: {
          deduplicate: false, // Disable deduplication for this test
        },
      };

      let callCount = 0;
      const mockReviewer: ReviewerFunction = async (
        input: PersonaReviewInput
      ): Promise<PersonaReviewOutput> => {
        callCount++;
        return {
          persona: input.persona.name,
          findings: [
            {
              id: `finding-${callCount}`,
              file: input.files[0].path,
              severity: 'low',
              personas: [input.persona.name],
              title: `Finding from ${input.persona.name}`,
              description: 'desc',
            },
          ],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      const result = await reviewFiles(config, testDir, mockReviewer);

      expect(callCount).toBe(2);
      expect(result.findings).toHaveLength(2);
      expect(result.findings[0].title).toBe('Finding from test-persona');
      expect(result.findings[1].title).toBe('Finding from persona2');
    });

    it('should use sub-agents when API key is provided', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
        options: {
          apiKey: 'test-api-key', // Provide API key to enable sub-agents
        },
      };

      const mockReviewer: ReviewerFunction = async (
        input: PersonaReviewInput
      ): Promise<PersonaReviewOutput> => {
        return {
          persona: input.persona.name,
          findings: [],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      // This test will fall back to legacy reviewer since we're using a mock API key
      // The important part is that it doesn't crash and completes successfully
      const result = await reviewFiles(config, testDir, mockReviewer, {
        useSubAgents: true,
        apiKey: 'test-api-key',
      });

      expect(result.findings).toBeDefined();
      expect(result.summary.filesReviewed).toBe(1);
    });

    it('should fall back to legacy when useSubAgents is false', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
      };

      let legacyReviewerCalled = false;
      const mockReviewer: ReviewerFunction = async (
        input: PersonaReviewInput
      ): Promise<PersonaReviewOutput> => {
        legacyReviewerCalled = true;
        return {
          persona: input.persona.name,
          findings: [],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      const result = await reviewFiles(config, testDir, mockReviewer, {
        useSubAgents: false, // Explicitly disable sub-agents
      });

      expect(legacyReviewerCalled).toBe(true);
      expect(result.findings).toBeDefined();
      expect(result.summary.filesReviewed).toBe(1);
    });

    it('should handle sub-agent errors and fall back gracefully', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'const x = 1;');

      const config: ReviewConfig = {
        files: [filePath],
        personas: [mockPersona],
        mode: 'quick',
        fileScanMode: 'full',
        options: {
          apiKey: 'invalid-key', // Invalid key will cause sub-agent to fail
        },
      };

      let legacyFallbackCalled = false;
      const mockReviewer: ReviewerFunction = async (
        input: PersonaReviewInput
      ): Promise<PersonaReviewOutput> => {
        legacyFallbackCalled = true;
        return {
          persona: input.persona.name,
          findings: [],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      const result = await reviewFiles(config, testDir, mockReviewer, {
        useSubAgents: true,
        apiKey: 'invalid-key',
      });

      // Should fall back to legacy reviewer and complete successfully
      expect(legacyFallbackCalled).toBe(true);
      expect(result.findings).toBeDefined();
      expect(result.summary.filesReviewed).toBe(1);
    });
  });
});
