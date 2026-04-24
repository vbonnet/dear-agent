/**
 * Tests for sub-agent orchestrator
 * Tests SubAgent, SubAgentFactory, SubAgentPool with cache_control integration
 */

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
  SubAgentFactory,
  SubAgentPool,
  SubAgentError,
  SUB_AGENT_ERROR_CODES,
  selectCacheTTL,
  detectSessionReviewCount,
  type SubAgent,
  type SubAgentConfig,
  type SubAgentStats,
} from '../../src/sub-agent-orchestrator.js';
import type { Persona, PersonaReviewInput } from '../../src/types.js';

// Mock Anthropic SDK so that auth-error tests work without real API calls
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
        create: mockCreate,
      },
    };
  });

  return {
    default: MockAnthropic,
    APIError: MockAPIError,
  };
});

describe('sub-agent-orchestrator', () => {
  // Mock persona for testing
  const mockPersona: Persona = {
    name: 'test-persona',
    displayName: 'Test Persona',
    version: '1.0.0',
    description: 'Test persona for unit tests',
    focusAreas: ['testing', 'quality'],
    prompt: 'You are a test persona focused on code quality and testing. Always respond with valid JSON array of findings.',
    severityLevels: ['critical', 'high', 'medium', 'low'],
  };

  const mockConfig: SubAgentConfig = {
    apiKey: 'test-api-key-12345',
    model: 'claude-3-5-sonnet-20241022',
    maxTokens: 4096,
    temperature: 0.0,
  };

  const mockInput: PersonaReviewInput = {
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

  describe('SubAgentFactory', () => {
    it('should throw error when API key is missing', () => {
      const invalidConfig: SubAgentConfig = { ...mockConfig, apiKey: '' };
      expect(() => SubAgentFactory.createSubAgent(mockPersona, invalidConfig)).toThrow(SubAgentError);
      expect(() => SubAgentFactory.createSubAgent(mockPersona, invalidConfig)).toThrow(/Missing provider credentials/);
    });

    it('should create sub-agent with valid config', () => {
      const agent = SubAgentFactory.createSubAgent(mockPersona, mockConfig);
      expect(agent).toBeDefined();
      expect(agent.persona.name).toBe('test-persona');
      expect(agent.cacheKey).toContain('persona:test-persona:1.0.0:');
    });

    it('should create sub-agent with cache key containing version hash', () => {
      const agent = SubAgentFactory.createSubAgent(mockPersona, mockConfig);
      // Cache key format: persona:{name}:{version}:{hash}
      expect(agent.cacheKey).toMatch(/^persona:test-persona:1\.0\.0:[a-f0-9]{16}$/);
    });

    it('should create different cache keys for different persona versions', () => {
      const agent1 = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      const persona2: Persona = {
        ...mockPersona,
        version: '2.0.0',
      };
      const agent2 = SubAgentFactory.createSubAgent(persona2, mockConfig);

      expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
    });

    it('should create different cache keys for different persona prompts', () => {
      const agent1 = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      const persona2: Persona = {
        ...mockPersona,
        prompt: 'Different prompt content',
      };
      const agent2 = SubAgentFactory.createSubAgent(persona2, mockConfig);

      expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
    });

    it('should initialize sub-agent with default stats', () => {
      const agent = SubAgentFactory.createSubAgent(mockPersona, mockConfig);
      const stats = agent.getStats();

      expect(stats.reviewCount).toBe(0);
      expect(stats.cacheHits).toBe(0);
      expect(stats.cacheMisses).toBe(0);
      expect(stats.totalInputTokens).toBe(0);
      expect(stats.totalOutputTokens).toBe(0);
      expect(stats.totalCacheWrites).toBe(0);
      expect(stats.totalCacheReads).toBe(0);
      expect(stats.createdAt).toBeInstanceOf(Date);
      expect(stats.lastUsed).toBeInstanceOf(Date);
    });
  });

  describe('SubAgentPool', () => {
    it('should create empty pool with default max size', () => {
      const pool = new SubAgentPool();
      expect(pool.size()).toBe(0);
    });

    it('should create pool with custom max size', () => {
      const pool = new SubAgentPool(5);
      expect(pool.size()).toBe(0);
    });

    it('should get or create sub-agent', () => {
      const pool = new SubAgentPool();
      const agent1 = pool.get(mockPersona, mockConfig);
      expect(pool.size()).toBe(1);

      // Getting same persona should return cached agent
      const agent2 = pool.get(mockPersona, mockConfig);
      expect(pool.size()).toBe(1);
      expect(agent1).toBe(agent2);
      expect(agent1.cacheKey).toBe(agent2.cacheKey);
    });

    it('should cache sub-agents by persona version hash', () => {
      const pool = new SubAgentPool();

      const persona1 = { ...mockPersona };
      const persona2 = { ...mockPersona, version: '2.0.0' };

      const agent1 = pool.get(persona1, mockConfig);
      const agent2 = pool.get(persona2, mockConfig);

      expect(pool.size()).toBe(2);
      expect(agent1).not.toBe(agent2);
      expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
    });

    it('should enforce maximum pool size with LRU eviction', () => {
      const pool = new SubAgentPool(2); // Max 2 agents

      const persona1 = { ...mockPersona, name: 'persona1' };
      const persona2 = { ...mockPersona, name: 'persona2' };
      const persona3 = { ...mockPersona, name: 'persona3' };

      pool.get(persona1, mockConfig);
      pool.get(persona2, mockConfig);
      expect(pool.size()).toBe(2);

      // Adding third persona should evict least recently used (persona1)
      pool.get(persona3, mockConfig);
      expect(pool.size()).toBe(2);

      const cachedPersonas = pool.getCachedPersonas();
      expect(cachedPersonas).toContain('persona2');
      expect(cachedPersonas).toContain('persona3');
      expect(cachedPersonas).not.toContain('persona1');
    });

    it('should destroy specific persona from pool', async () => {
      const pool = new SubAgentPool();
      pool.get(mockPersona, mockConfig);
      expect(pool.size()).toBe(1);

      await pool.destroy(mockPersona);
      expect(pool.size()).toBe(0);
    });

    it('should clear all sub-agents from pool', async () => {
      const pool = new SubAgentPool();

      const persona1 = { ...mockPersona, name: 'persona1' };
      const persona2 = { ...mockPersona, name: 'persona2' };

      pool.get(persona1, mockConfig);
      pool.get(persona2, mockConfig);
      expect(pool.size()).toBe(2);

      await pool.clear();
      expect(pool.size()).toBe(0);
    });

    it('should return list of cached persona names', () => {
      const pool = new SubAgentPool();

      const persona1 = { ...mockPersona, name: 'security-engineer' };
      const persona2 = { ...mockPersona, name: 'code-quality' };

      pool.get(persona1, mockConfig);
      pool.get(persona2, mockConfig);

      const cachedPersonas = pool.getCachedPersonas();
      expect(cachedPersonas).toHaveLength(2);
      expect(cachedPersonas).toContain('security-engineer');
      expect(cachedPersonas).toContain('code-quality');
    });

    it('should return all stats from pooled agents', () => {
      const pool = new SubAgentPool();

      const persona1 = { ...mockPersona, name: 'persona1' };
      const persona2 = { ...mockPersona, name: 'persona2' };

      pool.get(persona1, mockConfig);
      pool.get(persona2, mockConfig);

      const allStats = pool.getAllStats();
      const statKeys = Object.keys(allStats);

      expect(statKeys.length).toBe(2);
      expect(statKeys.every(key => key.startsWith('persona:'))).toBe(true);
    });
  });

  describe('SubAgent', () => {
    it('should have persona and cache key properties', () => {
      const agent = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      expect(agent.persona).toEqual(mockPersona);
      expect(agent.cacheKey).toBeDefined();
      expect(typeof agent.cacheKey).toBe('string');
    });

    it('should implement destroy method', async () => {
      const agent = SubAgentFactory.createSubAgent(mockPersona, mockConfig);
      await expect(agent.destroy()).resolves.toBeUndefined();
    });

    it('should track statistics across reviews', () => {
      const agent = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      const statsBefore = agent.getStats();
      expect(statsBefore.reviewCount).toBe(0);

      // Note: Actual review calls would require API key and are tested in integration tests
    });

    it('should throw on authentication errors (to trigger review-engine fallback)', async () => {
      // Using invalid API key to trigger authentication error
      const invalidConfig: SubAgentConfig = {
        ...mockConfig,
        apiKey: 'invalid-key',
      };
      const agent = SubAgentFactory.createSubAgent(mockPersona, invalidConfig);

      // Should throw on auth errors to allow review-engine to fall back
      await expect(agent.review(mockInput)).rejects.toThrow();
    });
  });

  describe('Parallel Execution Support', () => {
    // Note: Parallel execution is comprehensively tested in sub-agent-cache.test.ts
    // with proper Anthropic SDK mocking. This test would require either:
    // 1. Valid API key (making it an integration test)
    // 2. Anthropic SDK mocking (duplicating sub-agent-cache.test.ts)
    it.skip('should support parallel reviews with Promise.all (see sub-agent-cache.test.ts)', async () => {
      const pool = new SubAgentPool();

      const persona1 = { ...mockPersona, name: 'persona1' };
      const persona2 = { ...mockPersona, name: 'persona2' };

      const agent1 = pool.get(persona1, mockConfig);
      const agent2 = pool.get(persona2, mockConfig);

      // Both agents can be invoked in parallel
      const promises = [
        agent1.review({ ...mockInput, persona: persona1 }),
        agent2.review({ ...mockInput, persona: persona2 }),
      ];

      const results = await Promise.all(promises);

      expect(results).toHaveLength(2);
      expect(results[0].persona).toBe('persona1');
      expect(results[1].persona).toBe('persona2');
    });
  });

  describe('Cache Key Generation', () => {
    it('should generate stable cache keys for identical personas', () => {
      const agent1 = SubAgentFactory.createSubAgent(mockPersona, mockConfig);
      const agent2 = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      expect(agent1.cacheKey).toBe(agent2.cacheKey);
    });

    it('should generate different cache keys when prompt changes', () => {
      const agent1 = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      const modifiedPersona: Persona = {
        ...mockPersona,
        prompt: mockPersona.prompt + ' Additional instructions.',
      };
      const agent2 = SubAgentFactory.createSubAgent(modifiedPersona, mockConfig);

      expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
    });

    it('should generate different cache keys when focus areas change', () => {
      const agent1 = SubAgentFactory.createSubAgent(mockPersona, mockConfig);

      const modifiedPersona: Persona = {
        ...mockPersona,
        focusAreas: [...mockPersona.focusAreas, 'performance'],
      };
      const agent2 = SubAgentFactory.createSubAgent(modifiedPersona, mockConfig);

      expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
    });
  });

  describe('Error Handling', () => {
    it('should create SubAgentError with correct code and message', () => {
      const error = new SubAgentError(
        SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
        'API key is required'
      );

      expect(error.code).toBe(SUB_AGENT_ERROR_CODES.API_KEY_MISSING);
      expect(error.message).toBe('API key is required');
      expect(error.name).toBe('SubAgentError');
    });

    it('should include error details', () => {
      const details = { statusCode: 401, reason: 'Unauthorized' };
      const error = new SubAgentError(
        SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
        'Review failed',
        details
      );

      expect(error.details).toEqual(details);
    });

    it('should define all expected error codes', () => {
      expect(SUB_AGENT_ERROR_CODES.API_KEY_MISSING).toBe('SUBAGENT_6001');
      expect(SUB_AGENT_ERROR_CODES.REVIEW_FAILED).toBe('SUBAGENT_6002');
      expect(SUB_AGENT_ERROR_CODES.PARSE_ERROR).toBe('SUBAGENT_6003');
      expect(SUB_AGENT_ERROR_CODES.CACHE_ERROR).toBe('SUBAGENT_6004');
      expect(SUB_AGENT_ERROR_CODES.CREATION_FAILED).toBe('SUBAGENT_6005');
    });
  });

  describe('Automatic Cache TTL Selection', () => {
    let originalEnv: NodeJS.ProcessEnv;

    beforeEach(() => {
      // Save original environment
      originalEnv = { ...process.env };
    });

    afterEach(() => {
      // Restore original environment
      process.env = originalEnv;
    });

    describe('selectCacheTTL', () => {
      it('should return 5min for 1 review', () => {
        expect(selectCacheTTL(1)).toBe('5min');
      });

      it('should return 5min for 2 reviews', () => {
        expect(selectCacheTTL(2)).toBe('5min');
      });

      it('should return 5min for 3 reviews', () => {
        expect(selectCacheTTL(3)).toBe('5min');
      });

      it('should return 1h for 4 reviews (break-even point)', () => {
        expect(selectCacheTTL(4)).toBe('1h');
      });

      it('should return 1h for 5 reviews', () => {
        expect(selectCacheTTL(5)).toBe('1h');
      });

      it('should return 1h for 10 reviews', () => {
        expect(selectCacheTTL(10)).toBe('1h');
      });

      it('should return 1h for 100 reviews', () => {
        expect(selectCacheTTL(100)).toBe('1h');
      });
    });

    describe('detectSessionReviewCount', () => {
      it('should return 1 by default (no environment variables)', () => {
        delete process.env.MULTI_PERSONA_REVIEW_COUNT;
        delete process.env.MULTI_PERSONA_BATCH_MODE;
        delete process.env.CI;

        expect(detectSessionReviewCount()).toBe(1);
      });

      it('should use explicit MULTI_PERSONA_REVIEW_COUNT', () => {
        process.env.MULTI_PERSONA_REVIEW_COUNT = '7';
        expect(detectSessionReviewCount()).toBe(7);
      });

      it('should handle invalid MULTI_PERSONA_REVIEW_COUNT and fall back', () => {
        process.env.MULTI_PERSONA_REVIEW_COUNT = 'invalid';
        expect(detectSessionReviewCount()).toBe(1);
      });

      it('should handle negative MULTI_PERSONA_REVIEW_COUNT and fall back', () => {
        process.env.MULTI_PERSONA_REVIEW_COUNT = '-5';
        expect(detectSessionReviewCount()).toBe(1);
      });

      it('should detect batch mode', () => {
        process.env.MULTI_PERSONA_BATCH_MODE = 'true';
        expect(detectSessionReviewCount()).toBe(5);
      });

      it('should detect CI environment', () => {
        process.env.CI = 'true';
        expect(detectSessionReviewCount()).toBe(4);
      });

      it('should prioritize explicit count over batch mode', () => {
        process.env.MULTI_PERSONA_REVIEW_COUNT = '10';
        process.env.MULTI_PERSONA_BATCH_MODE = 'true';
        expect(detectSessionReviewCount()).toBe(10);
      });

      it('should prioritize explicit count over CI', () => {
        process.env.MULTI_PERSONA_REVIEW_COUNT = '8';
        process.env.CI = 'true';
        expect(detectSessionReviewCount()).toBe(8);
      });

      it('should prioritize batch mode over CI', () => {
        process.env.MULTI_PERSONA_BATCH_MODE = 'true';
        process.env.CI = 'true';
        expect(detectSessionReviewCount()).toBe(5);
      });
    });

    describe('Integration: Auto-TTL with session detection', () => {
      it('should select 5min TTL for single review session', () => {
        delete process.env.MULTI_PERSONA_REVIEW_COUNT;
        delete process.env.MULTI_PERSONA_BATCH_MODE;
        delete process.env.CI;

        const reviewCount = detectSessionReviewCount();
        const ttl = selectCacheTTL(reviewCount);

        expect(reviewCount).toBe(1);
        expect(ttl).toBe('5min');
      });

      it('should select 1h TTL for batch mode session', () => {
        process.env.MULTI_PERSONA_BATCH_MODE = 'true';

        const reviewCount = detectSessionReviewCount();
        const ttl = selectCacheTTL(reviewCount);

        expect(reviewCount).toBe(5);
        expect(ttl).toBe('1h');
      });

      it('should select 1h TTL for CI environment', () => {
        process.env.CI = 'true';

        const reviewCount = detectSessionReviewCount();
        const ttl = selectCacheTTL(reviewCount);

        expect(reviewCount).toBe(4);
        expect(ttl).toBe('1h');
      });

      it('should select 1h TTL for explicit large count', () => {
        process.env.MULTI_PERSONA_REVIEW_COUNT = '20';

        const reviewCount = detectSessionReviewCount();
        const ttl = selectCacheTTL(reviewCount);

        expect(reviewCount).toBe(20);
        expect(ttl).toBe('1h');
      });
    });
  });
});
