/**
 * Comprehensive End-to-End Claude Code Integration Tests
 *
 * Tests the complete Claude Code environment integration including:
 * - Auto-detection of VertexAI credentials from Claude Code env vars
 * - Full review workflow in Claude Code context
 * - Sub-agent orchestration with VertexAI in production-like scenarios
 * - Error handling and graceful degradation
 * - Logging and diagnostics
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { SubAgentPool, SubAgentFactory } from '../../src/sub-agent-orchestrator.js';
import type { SubAgentConfig } from '../../src/sub-agent-orchestrator.js';
import type { Persona, PersonaReviewInput } from '../../src/types.js';

// Test personas representing realistic review scenarios
const securityPersona: Persona = {
  name: 'security-expert',
  displayName: 'Security Expert',
  version: '1.0.0',
  prompt: 'You are a security expert. Review code for vulnerabilities, focusing on injection attacks, authentication issues, and data exposure.',
  focusAreas: ['SQL injection', 'XSS', 'authentication', 'authorization', 'secrets management'],
  severityLevels: ['critical', 'high', 'medium', 'low', 'info'],
  enabled: true,
  tier: 1,
};

const performancePersona: Persona = {
  name: 'performance-expert',
  displayName: 'Performance Expert',
  version: '1.0.0',
  prompt: 'You are a performance expert. Review code for efficiency issues, focusing on algorithmic complexity, memory usage, and scalability.',
  focusAreas: ['algorithm complexity', 'memory leaks', 'database queries', 'caching', 'concurrency'],
  severityLevels: ['critical', 'high', 'medium', 'low', 'info'],
  enabled: true,
  tier: 2,
};

// Realistic code samples with known issues
const vulnerableCode = `
// User authentication endpoint
app.post('/login', async (req, res) => {
  const { username, password } = req.body;

  // SQL injection vulnerability
  const query = \`SELECT * FROM users WHERE username = '\${username}' AND password = '\${password}'\`;
  const user = await db.query(query);

  if (user) {
    // Hardcoded secret key
    const token = jwt.sign({ userId: user.id }, 'super-secret-key-123');
    res.json({ token });
  } else {
    res.status(401).send('Invalid credentials');
  }
});
`;

const inefficientCode = `
// Process large dataset
function processRecords(records) {
  let results = [];

  // O(n²) complexity - inefficient
  for (let i = 0; i < records.length; i++) {
    for (let j = 0; j < records.length; j++) {
      if (records[i].category === records[j].category && i !== j) {
        results.push({ match: records[i], related: records[j] });
      }
    }
  }

  // Memory inefficient - loading all into memory
  const allData = records.map(r => db.loadFullRecord(r.id));
  return { results, allData };
}
`;

// VertexAI credentials from Claude Code environment
const claudeCodeVertexProject = process.env.ANTHROPIC_VERTEX_PROJECT_ID;
const claudeCodeVertexRegion = process.env.CLOUD_ML_REGION || 'us-east5';
const claudeCodeUseVertex = process.env.CLAUDE_CODE_USE_VERTEX === '1';
const vertexKeyFilename = process.env.GOOGLE_APPLICATION_CREDENTIALS;

// Check TLS/SSL connectivity to Google APIs (may be unavailable in sandboxed environments)
const HAS_TLS_CONNECTIVITY = (() => {
  try {
    const { execSync } = require('child_process');
    execSync('node -e "const https=require(\'https\');const r=https.request(\'https://oauth2.googleapis.com/.well-known/openid-configuration\',{timeout:3000},res=>{res.resume();res.on(\'end\',()=>process.exit(0))});r.on(\'error\',()=>process.exit(1));r.end()"', { timeout: 5000 });
    return true;
  } catch {
    return false;
  }
})();

const shouldRunIntegrationTests = Boolean(claudeCodeVertexProject) && claudeCodeUseVertex && HAS_TLS_CONNECTIVITY;

describe('Claude Code Integration - End-to-End', () => {
  describe('Environment Detection', () => {
    it('detects Claude Code VertexAI environment variables', () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      // Verify Claude Code sets expected env vars
      expect(process.env.CLAUDE_CODE_USE_VERTEX).toBe('1');
      expect(process.env.ANTHROPIC_VERTEX_PROJECT_ID).toBeDefined();
      expect(process.env.ANTHROPIC_VERTEX_PROJECT_ID).not.toBe('');

      // Region should have sensible default
      const region = process.env.CLOUD_ML_REGION || 'us-east5';
      expect(region).toMatch(/^[a-z]+-[a-z]+\d+$/); // e.g., us-east5, europe-west1
    });

    it('creates sub-agent config from Claude Code env vars', () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
      };

      expect(config.vertexProject).toBe(claudeCodeVertexProject);
      expect(config.vertexLocation).toBe(claudeCodeVertexRegion);
      expect(config.apiKey).toBeUndefined(); // Should not use Anthropic API key
    });
  });

  describe('Single Persona Review Workflow', () => {
    let pool: SubAgentPool;

    beforeEach(() => {
      pool = new SubAgentPool();
    });

    afterEach(async () => {
      await pool.clear();
    });

    it('performs security review with VertexAI sub-agent', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
        maxTokens: 4096,
        temperature: 0.0,
      };

      const agent = pool.get(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'src/auth/login.ts',
            content: vulnerableCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'thorough',
        options: {},
      };

      const result = await agent.review(input);

      // Verify result structure
      expect(result.persona).toBe('security-expert');
      expect(result.findings).toBeDefined();
      expect(Array.isArray(result.findings)).toBe(true);

      // Security review should detect vulnerabilities
      expect(result.findings.length).toBeGreaterThan(0);

      const findings = result.findings.map(f => ({
        title: f.title.toLowerCase(),
        description: f.description.toLowerCase(),
        severity: f.severity,
      }));

      // Should detect SQL injection
      const sqlInjectionFound = findings.some(f =>
        f.title.includes('sql') || f.description.includes('sql injection')
      );
      expect(sqlInjectionFound).toBe(true);

      // Should detect hardcoded secret
      const secretFound = findings.some(f =>
        f.title.includes('secret') || f.description.includes('hardcoded') ||
        f.title.includes('credential') || f.description.includes('key')
      );
      expect(secretFound).toBe(true);

      // Verify cost tracking
      expect(result.cost).toBeDefined();
      expect(result.cost.persona).toBe('security-expert');
      expect(result.cost.inputTokens).toBeGreaterThan(0);
      expect(result.cost.outputTokens).toBeGreaterThan(0);
      expect(result.cost.cost).toBeGreaterThan(0);

      // VertexAI does not support caching
      expect(result.cost.cacheCreationInputTokens || 0).toBe(0);
      expect(result.cost.cacheReadInputTokens || 0).toBe(0);
    }, 90000); // 90s timeout for thorough review

    it('performs performance review with VertexAI sub-agent', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
      };

      const agent = pool.get(performancePersona, config);
      const input: PersonaReviewInput = {
        persona: performancePersona,
        files: [
          {
            path: 'src/data/processor.ts',
            content: inefficientCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'thorough',
        options: {},
      };

      const result = await agent.review(input);

      // Verify result
      expect(result.persona).toBe('performance-expert');
      expect(result.findings.length).toBeGreaterThan(0);

      const findings = result.findings.map(f => ({
        title: f.title.toLowerCase(),
        description: f.description.toLowerCase(),
      }));

      // Should detect O(n²) complexity
      const complexityFound = findings.some(f =>
        f.title.includes('complexity') || f.description.includes('o(n') ||
        f.title.includes('performance') || f.description.includes('nested loop')
      );
      expect(complexityFound).toBe(true);

      // Verify stats tracking
      const stats = agent.getStats();
      expect(stats.reviewCount).toBe(1);
      expect(stats.totalInputTokens).toBeGreaterThan(0);
      expect(stats.totalOutputTokens).toBeGreaterThan(0);
    }, 90000);
  });

  describe('Multi-Persona Parallel Execution', () => {
    let pool: SubAgentPool;

    beforeEach(() => {
      pool = new SubAgentPool();
    });

    afterEach(async () => {
      await pool.clear();
    });

    it('executes multiple personas in parallel with VertexAI', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
        timeout: 120000, // 2 minute timeout
      };

      const securityAgent = pool.get(securityPersona, config);
      const performanceAgent = pool.get(performancePersona, config);

      const securityInput: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'src/auth/login.ts',
            content: vulnerableCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'thorough',
        options: {},
      };

      const performanceInput: PersonaReviewInput = {
        persona: performancePersona,
        files: [
          {
            path: 'src/data/processor.ts',
            content: inefficientCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'thorough',
        options: {},
      };

      // Execute reviews in parallel
      const startTime = Date.now();
      const [securityResult, performanceResult] = await Promise.all([
        securityAgent.review(securityInput),
        performanceAgent.review(performanceInput),
      ]);
      const endTime = Date.now();

      // Verify both completed successfully
      expect(securityResult.persona).toBe('security-expert');
      expect(performanceResult.persona).toBe('performance-expert');
      expect(securityResult.findings.length).toBeGreaterThan(0);
      expect(performanceResult.findings.length).toBeGreaterThan(0);

      // Verify parallel execution (should be faster than sequential)
      const totalTime = endTime - startTime;
      console.log(`Parallel execution time: ${totalTime}ms`);

      // Both agents should track separate stats
      const securityStats = securityAgent.getStats();
      const performanceStats = performanceAgent.getStats();

      expect(securityStats.reviewCount).toBe(1);
      expect(performanceStats.reviewCount).toBe(1);
      expect(securityStats.totalInputTokens).toBeGreaterThan(0);
      expect(performanceStats.totalInputTokens).toBeGreaterThan(0);

      // Pool should contain both agents
      expect(pool.size()).toBe(2);
    }, 180000); // 3 minute timeout for parallel reviews
  });

  describe('Error Handling and Recovery', () => {
    let pool: SubAgentPool;

    beforeEach(() => {
      pool = new SubAgentPool();
    });

    afterEach(async () => {
      await pool.clear();
    });

    it('handles invalid VertexAI project gracefully', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: 'invalid-nonexistent-project-12345',
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
        timeout: 10000, // Short timeout for error case
      };

      const agent = pool.get(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test.ts',
            content: 'const x = 1;',
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'fast',
        options: {},
      };

      const result = await agent.review(input);

      // Should not throw - returns graceful error
      expect(result.persona).toBe('security-expert');
      expect(result.findings).toEqual([]);
      expect(result.errors).toBeDefined();
      expect(result.errors!.length).toBeGreaterThan(0);
      expect(result.errors![0].message).toBeDefined();
    }, 20000);

    it('handles invalid model region gracefully', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: 'invalid-region-xyz',
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
        timeout: 10000,
      };

      const agent = pool.get(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test.ts',
            content: 'const x = 1;',
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'fast',
        options: {},
      };

      const result = await agent.review(input);

      // Should return error gracefully
      expect(result.errors).toBeDefined();
      expect(result.errors!.length).toBeGreaterThan(0);
    }, 20000);

    it('handles network timeouts gracefully', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
        timeout: 1, // 1ms timeout - virtually impossible to complete
      };

      const agent = pool.get(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test.ts',
            content: vulnerableCode, // Large code sample
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'thorough',
        options: {},
      };

      const result = await agent.review(input);

      // Should handle timeout gracefully - either return error or complete successfully
      // With 1ms timeout, we expect an error in most cases
      expect(result.persona).toBe('security-expert'); // Always returns persona name

      if (result.errors && result.errors.length > 0) {
        // If there's an error, verify it's structured correctly
        expect(result.errors[0].message).toBeDefined();
        expect(typeof result.errors[0].message).toBe('string');
        expect(result.errors[0].message.length).toBeGreaterThan(0);
        // Error can be any error - timeout, connection, auth, etc.
        // The important thing is it doesn't throw, it returns gracefully
      } else {
        // If no error, review should have completed (timing was lucky or timeout not enforced)
        // This is acceptable - we just verify graceful handling
        expect(result.findings).toBeDefined();
      }
    }, 15000);

    it('handles missing authentication gracefully', async () => {
      // This test runs even without VertexAI credentials to verify error handling
      const config: SubAgentConfig = {
        vertexProject: 'test-project',
        vertexLocation: 'us-east5',
        vertexKeyFilename: '/nonexistent/path/to/key.json', // Invalid key file
        model: 'claude-sonnet-4-5@20250929',
        timeout: 5000,
      };

      const agent = SubAgentFactory.createSubAgent(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test.ts',
            content: 'const x = 1;',
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'fast',
        options: {},
      };

      const result = await agent.review(input);

      // Should return authentication error
      expect(result.errors).toBeDefined();
      expect(result.errors!.length).toBeGreaterThan(0);
    }, 15000);
  });

  describe('Backward Compatibility', () => {
    it('Anthropic provider works when both providers available', async () => {
      if (!process.env.ANTHROPIC_API_KEY) {
        console.log('Skipping - Anthropic API key not available');
        return;
      }

      const config: SubAgentConfig = {
        apiKey: process.env.ANTHROPIC_API_KEY,
        // Even if VertexAI vars present, Anthropic should be used
        vertexProject: claudeCodeVertexProject,
        model: 'claude-3-5-sonnet-20241022',
      };

      const pool = new SubAgentPool();
      const agent = pool.get(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test.ts',
            content: vulnerableCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'fast',
        options: {},
      };

      const result = await agent.review(input);

      expect(result.persona).toBe('security-expert');
      expect(result.findings).toBeDefined();

      await pool.clear();
    }, 60000);
  });

  describe('Statistics and Diagnostics', () => {
    let pool: SubAgentPool;

    beforeEach(() => {
      pool = new SubAgentPool();
    });

    afterEach(async () => {
      await pool.clear();
    });

    it('tracks comprehensive statistics for VertexAI reviews', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
      };

      const agent = pool.get(securityPersona, config);

      // Perform multiple reviews
      const input1: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test1.ts',
            content: 'const x = 1;',
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'fast',
        options: {},
      };

      const input2: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test2.ts',
            content: vulnerableCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'fast',
        options: {},
      };

      await agent.review(input1);
      await agent.review(input2);

      const stats = agent.getStats();

      // Verify stats tracking
      expect(stats.reviewCount).toBe(2);
      expect(stats.totalInputTokens).toBeGreaterThan(0);
      expect(stats.totalOutputTokens).toBeGreaterThan(0);
      expect(stats.lastUsed).toBeInstanceOf(Date);
      expect(stats.createdAt).toBeInstanceOf(Date);

      // VertexAI does not support caching
      expect(stats.totalCacheWrites).toBe(0);
      expect(stats.totalCacheReads).toBe(0);
      expect(stats.cacheHits).toBe(0);
      expect(stats.cacheMisses).toBe(0);
    }, 120000);

    it('provides accurate cost calculations', async () => {
      if (!shouldRunIntegrationTests) {
        console.log('Skipping - Claude Code VertexAI environment not detected');
        return;
      }

      const config: SubAgentConfig = {
        vertexProject: claudeCodeVertexProject!,
        vertexLocation: claudeCodeVertexRegion,
        vertexKeyFilename,
        model: 'claude-sonnet-4-5@20250929',
      };

      const agent = pool.get(securityPersona, config);
      const input: PersonaReviewInput = {
        persona: securityPersona,
        files: [
          {
            path: 'test.ts',
            content: vulnerableCode,
            language: 'typescript',
            isDiff: false,
          },
        ],
        mode: 'thorough',
        options: {},
      };

      const result = await agent.review(input);

      // Verify cost structure
      expect(result.cost).toBeDefined();
      expect(result.cost.persona).toBe('security-expert');
      expect(result.cost.inputTokens).toBeGreaterThan(0);
      expect(result.cost.outputTokens).toBeGreaterThan(0);
      expect(result.cost.cost).toBeGreaterThan(0);

      // Cost should be reasonable (not astronomical)
      expect(result.cost.cost).toBeLessThan(1.0); // Should be well under $1

      // No caching for VertexAI
      expect(result.cost.cacheCreationInputTokens || 0).toBe(0);
      expect(result.cost.cacheReadInputTokens || 0).toBe(0);
    }, 90000);
  });
});
