/**
 * Cache Validation Test Suite for Sub-Agent Orchestrator
 *
 * Tests cache behavior, sub-agent isolation, and parallel execution
 * as specified in Task 2.2 (bead: oss-4e17)
 *
 * Test Coverage:
 * - Cache hit/miss detection
 * - Cache key generation and invalidation
 * - Sub-agent context isolation
 * - Parallel execution with caching
 * - Graceful degradation
 * - Cache statistics tracking
 */

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
  SubAgentFactory,
  SubAgentPool,
  SubAgentError,
  type SubAgent,
  type SubAgentConfig,
  type ApiUsageStats,
} from '../../src/sub-agent-orchestrator.js';
import type { Persona, PersonaReviewInput } from '../../src/types.js';

// Mock types for Anthropic SDK (for type safety)
type MockMessage = {
  id: string;
  type: 'message';
  role: 'assistant';
  content: Array<{ type: 'text'; text: string }>;
  model: string;
  stop_reason: string | null;
  stop_sequence: string | null;
  usage: ApiUsageStats;
};

// Mock Anthropic SDK completely (must be hoisted, so can't use external variables)
vi.mock('@anthropic-ai/sdk', () => {
  // Mock APIError class
  class MockAnthropicAPIError extends Error {
    status: number;
    constructor(status: number, body: any, message?: string, headers?: any) {
      super(message || JSON.stringify(body));
      this.status = status;
      this.name = 'APIError';
    }
  }

  // Mock Anthropic constructor
  const MockAnthropicConstructor = vi.fn();

  return {
    default: MockAnthropicConstructor,
    APIError: MockAnthropicAPIError
  };
});

// Import mocked Anthropic for use in tests
import Anthropic, { APIError } from '@anthropic-ai/sdk';

// Get access to the mocked constructor for test setup
const mockAnthropicConstructor = Anthropic as unknown as ReturnType<typeof vi.fn>;

// ============================================================================
// Mock Setup
// ============================================================================

/**
 * Creates a mock Anthropic API response with cache metadata
 */
function createMockResponse(
  text: string,
  usage: ApiUsageStats
): MockMessage {
  return {
    id: 'msg_test_' + Math.random().toString(36).slice(2),
    type: 'message',
    role: 'assistant',
    content: [
      {
        type: 'text',
        text,
      },
    ],
    model: 'claude-3-5-sonnet-20241022',
    stop_reason: 'end_turn',
    stop_sequence: null,
    usage,
  };
}

/**
 * Creates a mock Anthropic client for testing
 */
function createMockAnthropicClient(responses: MockMessage[]) {
  let callCount = 0;

  const mockClient = {
    messages: {
      create: vi.fn(async () => {
        if (callCount >= responses.length) {
          throw new Error('No more mock responses available');
        }
        return responses[callCount++];
      }),
    },
  };

  return mockClient;
}

// ============================================================================
// Test Data
// ============================================================================

const securityPersona: Persona = {
  name: 'security-engineer',
  displayName: 'Security Engineer',
  version: '1.0.0',
  description: 'Focuses on security vulnerabilities',
  focusAreas: ['security', 'authentication', 'authorization'],
  prompt: 'You are a security expert. Find security issues in code. ' +
    'Respond with JSON array of findings.',
  severityLevels: ['critical', 'high', 'medium', 'low'],
};

const performancePersona: Persona = {
  name: 'performance-engineer',
  displayName: 'Performance Engineer',
  version: '1.0.0',
  description: 'Focuses on performance optimization',
  focusAreas: ['performance', 'scalability', 'resource-usage'],
  prompt: 'You are a performance expert. Find performance issues in code. ' +
    'Respond with JSON array of findings.',
  severityLevels: ['high', 'medium', 'low'],
};

const testConfig: SubAgentConfig = {
  apiKey: 'test-api-key-mock',
  model: 'claude-3-5-sonnet-20241022',
  maxTokens: 4096,
  temperature: 0.0,
};

const sampleInput: PersonaReviewInput = {
  persona: securityPersona,
  files: [
    {
      path: 'auth.ts',
      content: 'export function login(password: string) { return password; }',
      isDiff: false,
    },
  ],
  mode: 'quick',
  options: {},
};

const sampleFindings = [
  {
    severity: 'high',
    file: 'auth.ts',
    line: 1,
    title: 'Password returned in plaintext',
    description: 'Password should not be returned from login function',
    confidence: 0.95,
    categories: ['security'],
  },
];

// ============================================================================
// Cache Hit Tests
// ============================================================================

describe('Cache Hit Tests', () => {
  it('should detect cache creation on first review', async () => {
    // First review creates cache (cache_creation_input_tokens > 0)
    const mockResponse = createMockResponse(
      JSON.stringify(sampleFindings),
      {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800, // Persona prompt cached
        cache_read_input_tokens: 0,
      }
    );

    // Mock Anthropic SDK
    const originalAnthropicConstructor = Anthropic;
    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([mockResponse])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const result = await agent.review(sampleInput);

    const stats = agent.getStats();

    // Verify cache creation
    expect(stats.totalCacheWrites).toBe(800);
    expect(stats.totalCacheReads).toBe(0);
    expect(stats.cacheMisses).toBe(1); // First call is a miss
    expect(stats.cacheHits).toBe(0);
    expect(result.findings.length).toBeGreaterThan(0);

    vi.restoreAllMocks();
  });

  it('should detect cache hit on second review with same persona', async () => {
    const firstResponse = createMockResponse(
      JSON.stringify(sampleFindings),
      {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      }
    );

    const secondResponse = createMockResponse(
      JSON.stringify(sampleFindings),
      {
        input_tokens: 700, // Only user prompt (new content)
        output_tokens: 180,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800, // Persona prompt read from cache
      }
    );

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([firstResponse, secondResponse])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    // First review
    await agent.review(sampleInput);

    // Second review should hit cache
    await agent.review({ ...sampleInput, files: [{ ...sampleInput.files[0], content: 'new code' }] });

    const stats = agent.getStats();

    // Verify cache hit
    expect(stats.totalCacheWrites).toBe(800); // Only from first call
    expect(stats.totalCacheReads).toBe(800); // From second call
    expect(stats.cacheMisses).toBe(1);
    expect(stats.cacheHits).toBe(1);
    expect(stats.reviewCount).toBe(2);

    vi.restoreAllMocks();
  });

  it('should calculate cache hit rate accurately', async () => {
    const responses = [
      // First call: cache miss (creation)
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      }),
      // Second call: cache hit
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 700,
        output_tokens: 180,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800,
      }),
      // Third call: cache hit
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 700,
        output_tokens: 190,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800,
      }),
    ];

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient(responses)
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    await agent.review(sampleInput);
    await agent.review(sampleInput);
    await agent.review(sampleInput);

    const stats = agent.getStats();

    // Cache hit rate = cacheHits / reviewCount = 2/3 = 66.67%
    const hitRate = stats.cacheHits / stats.reviewCount;
    expect(stats.cacheHits).toBe(2);
    expect(stats.cacheMisses).toBe(1);
    expect(stats.reviewCount).toBe(3);
    expect(hitRate).toBeCloseTo(0.667, 2);

    vi.restoreAllMocks();
  });

  it('should track cache reads independently from cache writes', async () => {
    const response = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 700,
      output_tokens: 200,
      cache_creation_input_tokens: 0,
      cache_read_input_tokens: 1200, // Large cache hit
    });

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([response])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    await agent.review(sampleInput);

    const stats = agent.getStats();

    expect(stats.totalCacheReads).toBe(1200);
    expect(stats.totalCacheWrites).toBe(0);
    expect(stats.cacheHits).toBe(1);

    vi.restoreAllMocks();
  });
});

// ============================================================================
// Isolation Tests
// ============================================================================

describe('Sub-Agent Isolation Tests', () => {
  it('should maintain independent contexts for different personas', async () => {
    const securityResponse = createMockResponse(
      JSON.stringify([{ ...sampleFindings[0], categories: ['security'] }]),
      {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      }
    );

    const performanceResponse = createMockResponse(
      JSON.stringify([{
        severity: 'medium',
        file: 'auth.ts',
        line: 1,
        title: 'Inefficient password handling',
        description: 'Consider using async password hashing',
        confidence: 0.85,
        categories: ['performance'],
      }]),
      {
        input_tokens: 1500,
        output_tokens: 180,
        cache_creation_input_tokens: 750,
        cache_read_input_tokens: 0,
      }
    );

    const mockClientSecurity = createMockAnthropicClient([securityResponse]);
    const mockClientPerformance = createMockAnthropicClient([performanceResponse]);

    let clientCount = 0;
    mockAnthropicConstructor.mockImplementation(() => {
      clientCount++;
      return clientCount === 1 ? mockClientSecurity : mockClientPerformance;
    });

    const securityAgent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const performanceAgent = SubAgentFactory.createSubAgent(performancePersona, testConfig);

    const securityResult = await securityAgent.review({
      ...sampleInput,
      persona: securityPersona,
    });

    const performanceResult = await performanceAgent.review({
      ...sampleInput,
      persona: performancePersona,
    });

    // Verify independent findings
    expect(securityResult.findings[0].categories).toContain('security');
    expect(performanceResult.findings[0].categories).toContain('performance');

    // Verify independent cache keys
    expect(securityAgent.cacheKey).not.toBe(performanceAgent.cacheKey);
    expect(securityAgent.cacheKey).toContain('security-engineer');
    expect(performanceAgent.cacheKey).toContain('performance-engineer');

    vi.restoreAllMocks();
  });

  it('should prevent context leakage between personas', async () => {
    const pool = new SubAgentPool();

    const responses1 = [
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      }),
    ];

    const responses2 = [
      createMockResponse(JSON.stringify([{ ...sampleFindings[0], title: 'Different finding' }]), {
        input_tokens: 1400,
        output_tokens: 190,
        cache_creation_input_tokens: 750,
        cache_read_input_tokens: 0,
      }),
    ];

    let callOrder = 0;
    mockAnthropicConstructor.mockImplementation(() => {
      callOrder++;
      return callOrder === 1
        ? createMockAnthropicClient(responses1)
        : createMockAnthropicClient(responses2);
    });

    const agent1 = pool.get(securityPersona, testConfig);
    const agent2 = pool.get(performancePersona, testConfig);

    const result1 = await agent1.review({ ...sampleInput, persona: securityPersona });
    const result2 = await agent2.review({ ...sampleInput, persona: performancePersona });

    // Verify separate conversation contexts
    expect(result1.findings[0].title).not.toBe(result2.findings[0].title);
    expect(agent1.getStats().reviewCount).toBe(1);
    expect(agent2.getStats().reviewCount).toBe(1);

    vi.restoreAllMocks();
  });

  it('should maintain independent statistics per sub-agent', async () => {
    const response1 = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1500,
      output_tokens: 200,
      cache_creation_input_tokens: 800,
      cache_read_input_tokens: 0,
    });

    const response2a = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1400,
      output_tokens: 180,
      cache_creation_input_tokens: 750,
      cache_read_input_tokens: 0,
    });

    const response2b = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 700,
      output_tokens: 185,
      cache_creation_input_tokens: 0,
      cache_read_input_tokens: 750,
    });

    let agentCallCount = 0;
    mockAnthropicConstructor.mockImplementation(() => {
      agentCallCount++;
      if (agentCallCount === 1) {
        return createMockAnthropicClient([response1]);
      } else {
        return createMockAnthropicClient([response2a, response2b]);
      }
    });

    const agent1 = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(performancePersona, testConfig);

    await agent1.review({ ...sampleInput, persona: securityPersona });
    await agent2.review({ ...sampleInput, persona: performancePersona });
    await agent2.review({ ...sampleInput, persona: performancePersona });

    const stats1 = agent1.getStats();
    const stats2 = agent2.getStats();

    // Agent 1: 1 review, 1 cache miss
    expect(stats1.reviewCount).toBe(1);
    expect(stats1.cacheMisses).toBe(1);
    expect(stats1.cacheHits).toBe(0);

    // Agent 2: 2 reviews, 1 miss, 1 hit
    expect(stats2.reviewCount).toBe(2);
    expect(stats2.cacheMisses).toBe(1);
    expect(stats2.cacheHits).toBe(1);

    vi.restoreAllMocks();
  });
});

// ============================================================================
// Cache Invalidation Tests
// ============================================================================

describe('Cache Invalidation Tests', () => {
  it('should invalidate cache when persona version changes', () => {
    const v1Persona: Persona = { ...securityPersona, version: '1.0.0' };
    const v2Persona: Persona = { ...securityPersona, version: '2.0.0' };

    const agent1 = SubAgentFactory.createSubAgent(v1Persona, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(v2Persona, testConfig);

    // Different versions should have different cache keys
    expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
    expect(agent1.cacheKey).toContain('1.0.0');
    expect(agent2.cacheKey).toContain('2.0.0');
  });

  it('should invalidate cache when persona prompt changes', () => {
    const originalPrompt = securityPersona.prompt;
    const modifiedPrompt = originalPrompt + ' Focus on SQL injection.';

    const persona1: Persona = { ...securityPersona, prompt: originalPrompt };
    const persona2: Persona = { ...securityPersona, prompt: modifiedPrompt };

    const agent1 = SubAgentFactory.createSubAgent(persona1, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(persona2, testConfig);

    // Different prompts should generate different hash -> different cache keys
    expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
  });

  it('should invalidate cache when focus areas change', () => {
    const persona1: Persona = {
      ...securityPersona,
      focusAreas: ['security', 'authentication'],
    };

    const persona2: Persona = {
      ...securityPersona,
      focusAreas: ['security', 'authentication', 'encryption'],
    };

    const agent1 = SubAgentFactory.createSubAgent(persona1, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(persona2, testConfig);

    expect(agent1.cacheKey).not.toBe(agent2.cacheKey);
  });

  it('should include version hash in cache key', () => {
    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    // Cache key format: persona:{name}:{version}:{hash}
    const cacheKeyPattern = /^persona:security-engineer:1\.0\.0:[a-f0-9]{16}$/;
    expect(agent.cacheKey).toMatch(cacheKeyPattern);
  });

  it('should generate stable hash for identical persona configurations', () => {
    const persona1: Persona = { ...securityPersona };
    const persona2: Persona = { ...securityPersona };

    const agent1 = SubAgentFactory.createSubAgent(persona1, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(persona2, testConfig);

    // Identical personas should have identical cache keys
    expect(agent1.cacheKey).toBe(agent2.cacheKey);
  });

  it('should not invalidate cache for non-cacheable field changes', () => {
    // displayName and description are not part of cache key calculation
    const persona1: Persona = {
      ...securityPersona,
      displayName: 'Security Expert',
      description: 'Original description',
    };

    const persona2: Persona = {
      ...securityPersona,
      displayName: 'Security Specialist',
      description: 'Modified description',
    };

    const agent1 = SubAgentFactory.createSubAgent(persona1, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(persona2, testConfig);

    // Cache key should be the same (only name, version, prompt, focusAreas, severityLevels matter)
    expect(agent1.cacheKey).toBe(agent2.cacheKey);
  });
});

// ============================================================================
// Parallel Execution Tests
// ============================================================================

describe('Parallel Execution with Caching Tests', () => {
  it('should maintain cache benefits when running personas in parallel', async () => {
    const securityResponse = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1500,
      output_tokens: 200,
      cache_creation_input_tokens: 800,
      cache_read_input_tokens: 0,
    });

    const performanceResponse = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1400,
      output_tokens: 180,
      cache_creation_input_tokens: 750,
      cache_read_input_tokens: 0,
    });

    let clientNumber = 0;
    mockAnthropicConstructor.mockImplementation(() => {
      clientNumber++;
      return clientNumber === 1
        ? createMockAnthropicClient([securityResponse])
        : createMockAnthropicClient([performanceResponse]);
    });

    const agent1 = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const agent2 = SubAgentFactory.createSubAgent(performancePersona, testConfig);

    // Execute reviews in parallel
    const results = await Promise.all([
      agent1.review({ ...sampleInput, persona: securityPersona }),
      agent2.review({ ...sampleInput, persona: performancePersona }),
    ]);

    expect(results).toHaveLength(2);

    // Both should have created their own caches
    const stats1 = agent1.getStats();
    const stats2 = agent2.getStats();

    expect(stats1.totalCacheWrites).toBe(800);
    expect(stats2.totalCacheWrites).toBe(750);

    vi.restoreAllMocks();
  });

  it('should not interfere with caching when multiple personas run concurrently', async () => {
    const pool = new SubAgentPool();

    const responses = [
      // Security persona responses
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      }),
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 700,
        output_tokens: 190,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800,
      }),
      // Performance persona responses
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1400,
        output_tokens: 180,
        cache_creation_input_tokens: 750,
        cache_read_input_tokens: 0,
      }),
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 650,
        output_tokens: 175,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 750,
      }),
    ];

    let responseIndex = 0;
    let agentIndex = 0;
    mockAnthropicConstructor.mockImplementation(() => {
      agentIndex++;
      if (agentIndex === 1) {
        // Security agent gets responses 0 and 1
        return createMockAnthropicClient([responses[0], responses[1]]);
      } else {
        // Performance agent gets responses 2 and 3
        return createMockAnthropicClient([responses[2], responses[3]]);
      }
    });

    const agent1 = pool.get(securityPersona, testConfig);
    const agent2 = pool.get(performancePersona, testConfig);

    // Run first round in parallel
    await Promise.all([
      agent1.review({ ...sampleInput, persona: securityPersona }),
      agent2.review({ ...sampleInput, persona: performancePersona }),
    ]);

    // Run second round in parallel (should hit cache)
    await Promise.all([
      agent1.review({ ...sampleInput, persona: securityPersona }),
      agent2.review({ ...sampleInput, persona: performancePersona }),
    ]);

    const stats1 = agent1.getStats();
    const stats2 = agent2.getStats();

    // Each agent should have 1 miss, 1 hit
    expect(stats1.cacheMisses).toBe(1);
    expect(stats1.cacheHits).toBe(1);
    expect(stats2.cacheMisses).toBe(1);
    expect(stats2.cacheHits).toBe(1);

    vi.restoreAllMocks();
  });

  it('should handle Promise.all with mixed cache hits and misses', async () => {
    const createResponse = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1500,
      output_tokens: 200,
      cache_creation_input_tokens: 800,
      cache_read_input_tokens: 0,
    });

    const hitResponse = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 700,
      output_tokens: 190,
      cache_creation_input_tokens: 0,
      cache_read_input_tokens: 800,
    });

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([createResponse, hitResponse])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    // First review creates cache, second hits cache
    const results = await Promise.all([
      agent.review(sampleInput),
      agent.review(sampleInput),
    ]);

    expect(results).toHaveLength(2);

    const stats = agent.getStats();
    expect(stats.reviewCount).toBe(2);
    expect(stats.totalCacheWrites).toBeGreaterThan(0);
    expect(stats.totalCacheReads).toBeGreaterThan(0);

    vi.restoreAllMocks();
  });
});

// ============================================================================
// Graceful Degradation Tests
// ============================================================================

describe('Graceful Degradation Tests', () => {
  it('should handle missing cache_control gracefully', async () => {
    // Response without cache metadata (e.g., from older API or non-cached request)
    const response = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1500,
      output_tokens: 200,
      // No cache_creation_input_tokens or cache_read_input_tokens
    });

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([response])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const result = await agent.review(sampleInput);

    // Should still process successfully
    expect(result.findings).toBeDefined();
    expect(result.findings.length).toBeGreaterThan(0);

    const stats = agent.getStats();
    expect(stats.totalCacheWrites).toBe(0);
    expect(stats.totalCacheReads).toBe(0);
    expect(stats.cacheMisses).toBe(0);
    expect(stats.cacheHits).toBe(0);

    vi.restoreAllMocks();
  });

  it('should throw on authentication errors (to trigger review-engine fallback)', async () => {
    const mockClient = {
      messages: {
        create: vi.fn().mockRejectedValue(
          new APIError(401, { message: 'Invalid API key' }, 'Unauthorized', {})
        ),
      },
    };

    mockAnthropicConstructor.mockImplementation(() => mockClient);

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    // Should throw SubAgentError (wrapping the APIError) with improved error message
    await expect(agent.review(sampleInput)).rejects.toThrow(SubAgentError);
    await expect(agent.review(sampleInput)).rejects.toThrow(/Authentication failed for anthropic provider/);

    vi.restoreAllMocks();
  });

  it('should handle rate limit errors gracefully', async () => {
    const mockClient = {
      messages: {
        create: vi.fn().mockRejectedValue(
          new APIError(429, { message: 'Rate limit exceeded' }, 'Rate Limit', {})
        ),
      },
    };

    mockAnthropicConstructor.mockImplementation(() => mockClient);

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const result = await agent.review(sampleInput);

    expect(result.findings).toEqual([]);
    expect(result.errors).toBeDefined();
    expect(result.errors![0].code).toBe('RATE_LIMIT');

    vi.restoreAllMocks();
  });

  it('should handle malformed response gracefully', async () => {
    // Response with invalid JSON
    const response = createMockResponse('Not valid JSON at all', {
      input_tokens: 1500,
      output_tokens: 200,
      cache_creation_input_tokens: 800,
      cache_read_input_tokens: 0,
    });

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([response])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    const result = await agent.review(sampleInput);

    // Should return error, not crash
    expect(result.findings).toEqual([]);
    expect(result.errors).toBeDefined();
    expect(result.errors!.length).toBeGreaterThan(0);

    vi.restoreAllMocks();
  });
});

// ============================================================================
// Cache Statistics and Metrics Tests
// ============================================================================

describe('Cache Statistics Tests', () => {
  it('should accurately track total cache writes across multiple reviews', async () => {
    const responses = [
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      }),
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1600,
        output_tokens: 210,
        cache_creation_input_tokens: 850, // Additional cache write
        cache_read_input_tokens: 0,
      }),
    ];

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient(responses)
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    await agent.review(sampleInput);
    await agent.review(sampleInput);

    const stats = agent.getStats();

    expect(stats.totalCacheWrites).toBe(800 + 850);
    expect(stats.totalInputTokens).toBe(1500 + 1600);
    expect(stats.totalOutputTokens).toBe(200 + 210);

    vi.restoreAllMocks();
  });

  it('should accurately track total cache reads across multiple reviews', async () => {
    const responses = [
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 700,
        output_tokens: 200,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800,
      }),
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 750,
        output_tokens: 210,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800,
      }),
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 720,
        output_tokens: 195,
        cache_creation_input_tokens: 0,
        cache_read_input_tokens: 800,
      }),
    ];

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient(responses)
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    await agent.review(sampleInput);
    await agent.review(sampleInput);
    await agent.review(sampleInput);

    const stats = agent.getStats();

    expect(stats.totalCacheReads).toBe(800 * 3);
    expect(stats.cacheHits).toBe(3);

    vi.restoreAllMocks();
  });

  it('should update lastUsed timestamp on each review', async () => {
    const response = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1500,
      output_tokens: 200,
      cache_creation_input_tokens: 800,
      cache_read_input_tokens: 0,
    });

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([response])
    );

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);

    const statsBefore = agent.getStats();
    const timeBefore = statsBefore.lastUsed.getTime();

    // Wait a bit
    await new Promise(resolve => setTimeout(resolve, 10));

    await agent.review(sampleInput);

    const statsAfter = agent.getStats();
    const timeAfter = statsAfter.lastUsed.getTime();

    expect(timeAfter).toBeGreaterThan(timeBefore);

    vi.restoreAllMocks();
  });

  it('should provide cache efficiency metrics through pool stats', async () => {
    const pool = new SubAgentPool();

    const response = createMockResponse(JSON.stringify(sampleFindings), {
      input_tokens: 1500,
      output_tokens: 200,
      cache_creation_input_tokens: 800,
      cache_read_input_tokens: 0,
    });

    mockAnthropicConstructor.mockImplementation(() =>
      createMockAnthropicClient([response])
    );

    const agent = pool.get(securityPersona, testConfig);
    await agent.review(sampleInput);

    const allStats = pool.getAllStats();
    const statsKeys = Object.keys(allStats);

    expect(statsKeys.length).toBe(1);
    expect(statsKeys[0]).toContain('security-engineer');

    const agentStats = allStats[statsKeys[0]];
    expect(agentStats.reviewCount).toBe(1);
    expect(agentStats.totalCacheWrites).toBe(800);

    vi.restoreAllMocks();
  });
});

// ============================================================================
// Cache Control Placement Validation Tests
// ============================================================================

describe('Cache Control Placement Validation', () => {
  it('should place cache_control on system prompt', async () => {
    const mockCreate = vi.fn().mockResolvedValue(
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      })
    );

    const mockClient = {
      messages: {
        create: mockCreate,
      },
    };

    mockAnthropicConstructor.mockImplementation(() => mockClient);

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    await agent.review(sampleInput);

    // Verify cache_control was placed on system prompt
    expect(mockCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        system: expect.arrayContaining([
          expect.objectContaining({
            type: 'text',
            cache_control: { type: 'ephemeral' },
          }),
        ]),
      })
    );

    vi.restoreAllMocks();
  });

  it('should not place cache_control on user messages', async () => {
    const mockCreate = vi.fn().mockResolvedValue(
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      })
    );

    const mockClient = {
      messages: {
        create: mockCreate,
      },
    };

    mockAnthropicConstructor.mockImplementation(() => mockClient);

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    await agent.review(sampleInput);

    // Get the actual call arguments
    const callArgs = mockCreate.mock.calls[0][0];

    // Verify user message doesn't have cache_control
    expect(callArgs.messages).toHaveLength(1);
    expect(callArgs.messages[0].role).toBe('user');
    expect(callArgs.messages[0]).not.toHaveProperty('cache_control');

    vi.restoreAllMocks();
  });

  it('should use ephemeral cache type', async () => {
    const mockCreate = vi.fn().mockResolvedValue(
      createMockResponse(JSON.stringify(sampleFindings), {
        input_tokens: 1500,
        output_tokens: 200,
        cache_creation_input_tokens: 800,
        cache_read_input_tokens: 0,
      })
    );

    const mockClient = {
      messages: {
        create: mockCreate,
      },
    };

    mockAnthropicConstructor.mockImplementation(() => mockClient);

    const agent = SubAgentFactory.createSubAgent(securityPersona, testConfig);
    await agent.review(sampleInput);

    const callArgs = mockCreate.mock.calls[0][0];

    expect(callArgs.system[0].cache_control).toEqual({ type: 'ephemeral' });

    vi.restoreAllMocks();
  });
});
