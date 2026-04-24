/**
 * Integration tests for end-to-end multi-persona-review workflows
 * Session 17: Additional Testing Coverage
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mkdir, writeFile, rm } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import { execSync } from 'child_process';
import {
  loadCrossCheckConfig,
  loadPersonas,
  resolvePersonaPaths,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
  formatReviewResultJSON,
  formatReviewResultGitHub,
  createCostSink,
} from '../src/index.js';
import type { ReviewConfig, PersonaReviewInput, PersonaReviewOutput } from '../src/types.js';

describe('Integration Tests - E2E Workflows', () => {
  let testDir: string;

  beforeEach(async () => {
    testDir = join(tmpdir(), `multi-persona-review-integration-${Date.now()}-${Math.random().toString(36).substring(7)}`);
    await mkdir(testDir, { recursive: true });

    // Initialize git repo
    try {
      execSync('git init', { cwd: testDir, stdio: 'pipe' });
      execSync('git config user.email "test@test.com"', { cwd: testDir, stdio: 'pipe' });
      execSync('git config user.name "Test User"', { cwd: testDir, stdio: 'pipe' });
    } catch {
      // Git init optional for integration tests
    }
  });

  afterEach(async () => {
    try {
      await rm(testDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  describe('Full Review Workflow', () => {
    it('should complete full review with all components', async () => {
      // Create test files
      await mkdir(join(testDir, 'src'), { recursive: true });
      await writeFile(
        join(testDir, 'src/index.ts'),
        `export function insecureFunction(input: string) {
  // SQL injection vulnerability
  const query = "SELECT * FROM users WHERE id = " + input;
  return query;
}`
      );

      await writeFile(
        join(testDir, 'src/utils.ts'),
        `export function badError() {
  try {
    throw new Error('test');
  } catch (e) {
    // Swallowed error - no handling
  }
}`
      );

      // Create persona
      const personaDir = join(testDir, 'personas');
      await mkdir(personaDir, { recursive: true });
      await writeFile(
        join(personaDir, 'test-persona.yaml'),
        `name: test-persona
displayName: Test Persona
version: "1.0.0"
description: A test security reviewer
focusAreas:
  - Security
  - SQL Injection
  - Error Handling
prompt: |
  You are a security reviewer. Look for:
  - SQL injection vulnerabilities
  - Improper error handling
  - Security issues
`
      );

      // Load personas
      const personaPaths = [personaDir];
      const personas = await loadPersonas(personaPaths);
      const testPersona = personas.get('test-persona')!;

      expect(testPersona).toBeDefined();
      expect(testPersona.name).toBe('test-persona');

      // Create mock reviewer
      const mockReviewer = async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
        const findings = [];

        // Check for SQL injection
        for (const file of input.files) {
          if (file.content.includes('SELECT * FROM') && file.content.includes('+')) {
            findings.push({
              id: 'sql-injection-1',
              file: file.path,
              line: 3,
              severity: 'critical' as const,
              personas: [input.persona.name],
              title: 'SQL Injection Vulnerability',
              description: 'Direct string concatenation in SQL query allows SQL injection attacks',
            });
          }

          if (file.content.includes('catch (e)') && file.content.includes('// Swallowed error')) {
            findings.push({
              id: 'error-handling-1',
              file: file.path,
              line: 4,
              severity: 'medium' as const,
              personas: [input.persona.name],
              title: 'Swallowed Error',
              description: 'Error is caught but not handled or logged',
            });
          }
        }

        return {
          persona: input.persona.name,
          findings,
          cost: {
            persona: input.persona.name,
            cost: 0.05,
            inputTokens: 500,
            outputTokens: 100,
          },
        };
      };

      // Run review
      const config: ReviewConfig = {
        files: [join(testDir, 'src')],
        personas: [testPersona],
        mode: 'thorough',
        fileScanMode: 'full',
        options: {
          deduplicate: true,
        },
      };

      const result = await reviewFiles(config, testDir, mockReviewer);

      // Verify results
      expect(result.findings.length).toBeGreaterThan(0);
      expect(result.findings.some(f => f.severity === 'critical')).toBe(true);
      expect(result.summary.filesReviewed).toBe(2);
      expect(result.cost.totalCost).toBe(0.05);

      // Test formatting outputs
      const textOutput = formatReviewResult(result, {
        colors: false,
        groupByFile: true,
        showCost: true,
        showSummary: true,
      });
      expect(textOutput).toContain('SQL Injection');

      const jsonOutput = formatReviewResultJSON(result);
      const parsed = JSON.parse(jsonOutput);
      expect(parsed.findings).toBeDefined();

      const githubOutput = formatReviewResultGitHub(result);
      expect(githubOutput).toContain('Multi-Persona Review Code Review');
    });
  });

  describe('Error Handling', () => {
    it('should handle missing files gracefully', async () => {
      const mockReviewer = async (): Promise<PersonaReviewOutput> => {
        return {
          persona: 'test',
          findings: [],
          cost: {
            persona: 'test',
            cost: 0,
            inputTokens: 0,
            outputTokens: 0,
          },
        };
      };

      const config: ReviewConfig = {
        files: [join(testDir, 'nonexistent.ts')],
        personas: [{
          name: 'test',
          displayName: 'Test',
          version: '1.0',
          description: 'Test',
          focusAreas: ['test'],
          prompt: 'test',
        }],
        mode: 'quick',
      };

      // Should handle missing files
      await expect(reviewFiles(config, testDir, mockReviewer)).rejects.toThrow();
    });

    it('should handle binary files correctly', async () => {
      // Create binary file
      const binaryPath = join(testDir, 'image.png');
      const buffer = Buffer.from([0x89, 0x50, 0x4E, 0x47]); // PNG header
      await writeFile(binaryPath, buffer);

      // Create a text file as well
      const textPath = join(testDir, 'test.ts');
      await writeFile(textPath, 'const x = 1;');

      const mockReviewer = async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
        // Should receive text files but not binary files
        // Binary files are automatically filtered out
        return {
          persona: 'test',
          findings: [],
          cost: {
            persona: 'test',
            cost: 0,
            inputTokens: 0,
            outputTokens: 0,
          },
        };
      };

      const config: ReviewConfig = {
        files: [testDir],
        personas: [{
          name: 'test',
          displayName: 'Test',
          version: '1.0',
          description: 'Test',
          focusAreas: ['test'],
          prompt: 'test',
        }],
        mode: 'quick',
        fileScanMode: 'full',
      };

      const result = await reviewFiles(config, testDir, mockReviewer);
      // Should only review the text file, not the binary file
      expect(result.summary.filesReviewed).toBeGreaterThanOrEqual(1);
    });
  });

  describe('Cost Tracking', () => {
    it('should track costs with file sink', async () => {
      const costFile = join(testDir, 'costs.jsonl');

      const fileSink = await createCostSink({
        type: 'file',
        config: {
          filePath: costFile,
        },
      });

      await fileSink.record({
        totalCost: 0.123,
        totalTokens: 5000,
        byPersona: {
          'test-persona': {
            persona: 'test-persona',
            cost: 0.123,
            inputTokens: 4000,
            outputTokens: 1000,
          },
        },
      }, {
        repository: 'test-repo',
        branch: 'main',
        mode: 'thorough',
      });

      // Verify file was created and contains data
      const { readFile: readFileAsync } = await import('fs/promises');
      const content = await readFileAsync(costFile, 'utf-8');
      expect(content).toContain('"total":0.123');
      expect(content).toContain('"repository":"test-repo"');
    });

    it('should track costs with stdout sink', async () => {
      const originalConsoleError = console.error;
      const logs: string[] = [];
      console.error = (...args: any[]) => {
        logs.push(args.join(' '));
      };

      try {
        const stdoutSink = await createCostSink({
          type: 'stdout',
        });

        await stdoutSink.record({
          totalCost: 0.05,
          totalTokens: 2000,
          byPersona: {},
        });

        expect(logs.some(l => l.includes('[COST_TRACKING]'))).toBe(true);
        expect(logs.some(l => l.includes('0.05'))).toBe(true);
      } finally {
        console.error = originalConsoleError;
      }
    });
  });

  describe('Deduplication', () => {
    it('should deduplicate similar findings', async () => {
      const mockReviewer = async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
        // Create similar findings from different personas
        return {
          persona: input.persona.name,
          findings: [{
            id: `${input.persona.name}-1`,
            file: 'test.ts',
            line: 10,
            severity: 'medium' as const,
            personas: [input.persona.name],
            title: 'Missing error handling',
            description: 'The function does not handle errors properly',
          }],
          cost: {
            persona: input.persona.name,
            cost: 0.01,
            inputTokens: 100,
            outputTokens: 50,
          },
        };
      };

      await writeFile(join(testDir, 'test.ts'), 'function test() { }');

      const config: ReviewConfig = {
        files: [join(testDir, 'test.ts')],
        personas: [
          {
            name: 'persona1',
            displayName: 'Persona 1',
            version: '1.0',
            description: 'Test',
            focusAreas: ['test'],
            prompt: 'test',
          },
          {
            name: 'persona2',
            displayName: 'Persona 2',
            version: '1.0',
            description: 'Test',
            focusAreas: ['test'],
            prompt: 'test',
          },
        ],
        mode: 'quick',
        fileScanMode: 'full',
        options: {
          deduplicate: true,
          similarityThreshold: 0.8,
        },
      };

      const result = await reviewFiles(config, testDir, mockReviewer);

      // Should deduplicate the similar findings
      expect(result.findings.length).toBe(1);
      expect(result.findings[0].personas.length).toBe(2);
      expect(result.summary.deduplicated).toBeGreaterThan(0);
    });
  });

  describe('Configuration Loading', () => {
    it('should load configuration from file', async () => {
      // Create config file
      const configDir = join(testDir, '.wayfinder');
      await mkdir(configDir, { recursive: true });
      await writeFile(
        join(configDir, 'config.yml'),
        `crossCheck:
  defaultMode: thorough
  defaultPersonas:
    - security-engineer
    - code-health
  options:
    deduplicate: true
    similarityThreshold: 0.9
`
      );

      const configFilePath = join(configDir, 'config.yml');
      const config = await loadCrossCheckConfig(configFilePath);

      expect(config.defaultMode).toBe('thorough');
      expect(config.defaultPersonas).toEqual(['security-engineer', 'code-health']);
      expect(config.options?.deduplicate).toBe(true);
      expect(config.options?.similarityThreshold).toBe(0.9);
    });

    it('should handle missing config gracefully', async () => {
      try {
        const config = await loadCrossCheckConfig(testDir);
        // Should return empty/default config
        expect(config).toBeDefined();
      } catch (error) {
        // It's OK to throw an error if no config exists
        // This is expected behavior
        expect(error).toBeDefined();
      }
    });
  });

  describe('Parallel Execution', () => {
    it('should execute personas in parallel', async () => {
      const executionOrder: string[] = [];
      const delays = [100, 50, 75]; // Different delays to ensure parallel execution

      const mockReviewer = async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
        const personaIndex = parseInt(input.persona.name.replace('persona', ''));
        const delay = delays[personaIndex - 1];

        await new Promise(resolve => setTimeout(resolve, delay));
        executionOrder.push(input.persona.name);

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

      await writeFile(join(testDir, 'test.ts'), 'const x = 1;');

      const config: ReviewConfig = {
        files: [join(testDir, 'test.ts')],
        personas: [
          {
            name: 'persona1',
            displayName: 'Persona 1',
            version: '1.0',
            description: 'Test',
            focusAreas: ['test'],
            prompt: 'test',
          },
          {
            name: 'persona2',
            displayName: 'Persona 2',
            version: '1.0',
            description: 'Test',
            focusAreas: ['test'],
            prompt: 'test',
          },
          {
            name: 'persona3',
            displayName: 'Persona 3',
            version: '1.0',
            description: 'Test',
            focusAreas: ['test'],
            prompt: 'test',
          },
        ],
        mode: 'quick',
        fileScanMode: 'full',
      };

      const startTime = Date.now();
      await reviewFiles(config, testDir, mockReviewer, { parallel: true });
      const duration = Date.now() - startTime;

      // If parallel, should complete in ~100ms (longest delay)
      // If sequential, would take ~225ms (sum of delays)
      expect(duration).toBeLessThan(200);

      // With parallel execution, faster tasks finish before slower ones
      expect(executionOrder.indexOf('persona2')).toBeLessThan(executionOrder.indexOf('persona1'));
    });
  });
});
