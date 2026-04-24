/**
 * Tests for cost sink implementations
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mkdtemp, rm, readFile } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import {
  StdoutCostSink,
  FileCostSink,
  createCostSink,
  CostSinkError,
  COST_SINK_ERROR_CODES,
} from '../src/cost-sink.js';
import type { CostInfo, PersonaCost, CostSinkConfig } from '../src/types.js';

describe('StdoutCostSink', () => {
  let originalConsoleError: typeof console.error;
  let consoleOutput: string[] = [];

  beforeEach(() => {
    consoleOutput = [];
    originalConsoleError = console.error;
    console.error = (...args: any[]) => {
      consoleOutput.push(args.join(' '));
    };
  });

  afterEach(() => {
    console.error = originalConsoleError;
  });

  it('should record cost to stdout', async () => {
    const sink = new StdoutCostSink();

    const cost: CostInfo = {
      totalCost: 0.123,
      totalTokens: 5000,
      byPersona: {
        'security-engineer': {
          persona: 'security-engineer',
          cost: 0.123,
          inputTokens: 4000,
          outputTokens: 1000,
        },
      },
    };

    await sink.record(cost);

    expect(consoleOutput.length).toBe(1);
    expect(consoleOutput[0]).toContain('[COST_TRACKING]');
    expect(consoleOutput[0]).toContain('"total":0.123');
    expect(consoleOutput[0]).toContain('"tokens":5000');
  });

  it('should include metadata in output', async () => {
    const sink = new StdoutCostSink();

    const cost: CostInfo = {
      totalCost: 0.05,
      totalTokens: 2000,
      byPersona: {},
    };

    await sink.record(cost, {
      repository: 'test-repo',
      branch: 'main',
      mode: 'quick',
    });

    expect(consoleOutput[0]).toContain('"repository":"test-repo"');
    expect(consoleOutput[0]).toContain('"branch":"main"');
    expect(consoleOutput[0]).toContain('"mode":"quick"');
  });

  it('should track cache alerts and report them', async () => {
    const sink = new StdoutCostSink();

    // Poor cache performance
    const poorCost: CostInfo = {
      totalCost: 0.05,
      totalTokens: 2000,
      byPersona: {
        'security-engineer': {
          persona: 'security-engineer',
          cost: 0.05,
          inputTokens: 1500,
          outputTokens: 500,
          cacheReadInputTokens: 100, // Very low
        },
      },
    };

    // Track 3 times to trigger alert
    await sink.record(poorCost);
    await sink.record(poorCost);
    await sink.record(poorCost);

    // Should have generated an alert
    const hasAlert = consoleOutput.some(output => output.includes('[CACHE_ALERT]'));
    expect(hasAlert).toBe(true);

    // Check getCacheAlerts method
    const alerts = sink.getCacheAlerts();
    expect(alerts.length).toBeGreaterThan(0);
    expect(alerts[0].persona).toBe('security-engineer');
    expect(alerts[0].consecutiveFailures).toBe(3);
  });
});

describe('FileCostSink', () => {
  let tempDir: string;

  beforeEach(async () => {
    tempDir = await mkdtemp(join(tmpdir(), 'cost-sink-test-'));
  });

  afterEach(async () => {
    await rm(tempDir, { recursive: true, force: true });
  });

  it('should throw error if filePath is not provided', () => {
    expect(() => new FileCostSink({})).toThrow(CostSinkError);
    expect(() => new FileCostSink({})).toThrow('requires filePath');
  });

  it('should write cost records to file', async () => {
    const filePath = join(tempDir, 'costs.jsonl');
    const sink = new FileCostSink({ filePath });

    const cost: CostInfo = {
      totalCost: 0.25,
      totalTokens: 10000,
      byPersona: {
        'security-engineer': {
          persona: 'security-engineer',
          cost: 0.15,
          inputTokens: 6000,
          outputTokens: 2000,
        },
        'performance-engineer': {
          persona: 'performance-engineer',
          cost: 0.10,
          inputTokens: 2000,
          outputTokens: 1000,
        },
      },
    };

    await sink.record(cost);

    const content = await readFile(filePath, 'utf-8');
    const lines = content.trim().split('\n');
    expect(lines.length).toBe(1);

    const record = JSON.parse(lines[0]);
    expect(record.cost.total).toBe(0.25);
    expect(record.cost.tokens).toBe(10000);
    expect(record.cost.byPersona['security-engineer'].cost).toBe(0.15);
  });

  it('should append multiple records to file', async () => {
    const filePath = join(tempDir, 'costs.jsonl');
    const sink = new FileCostSink({ filePath });

    const cost1: CostInfo = {
      totalCost: 0.10,
      totalTokens: 3000,
      byPersona: {},
    };

    const cost2: CostInfo = {
      totalCost: 0.15,
      totalTokens: 5000,
      byPersona: {},
    };

    await sink.record(cost1);
    await sink.record(cost2);

    const content = await readFile(filePath, 'utf-8');
    const lines = content.trim().split('\n');
    expect(lines.length).toBe(2);

    const record1 = JSON.parse(lines[0]);
    const record2 = JSON.parse(lines[1]);
    expect(record1.cost.total).toBe(0.10);
    expect(record2.cost.total).toBe(0.15);
  });

  it('should include metadata in file records', async () => {
    const filePath = join(tempDir, 'costs.jsonl');
    const sink = new FileCostSink({ filePath });

    const cost: CostInfo = {
      totalCost: 0.05,
      totalTokens: 2000,
      byPersona: {},
    };

    await sink.record(cost, {
      repository: 'my-repo',
      branch: 'feature-branch',
      commit: 'abc123',
      pullRequest: 42,
    });

    const content = await readFile(filePath, 'utf-8');
    const record = JSON.parse(content.trim());
    expect(record.metadata.repository).toBe('my-repo');
    expect(record.metadata.branch).toBe('feature-branch');
    expect(record.metadata.commit).toBe('abc123');
    expect(record.metadata.pullRequest).toBe(42);
  });
});

describe('createCostSink', () => {
  it('should create StdoutCostSink for stdout type', async () => {
    const config: CostSinkConfig = {
      type: 'stdout',
    };

    const sink = await createCostSink(config);
    expect(sink).toBeInstanceOf(StdoutCostSink);
  });

  it('should create FileCostSink for file type', async () => {
    const config: CostSinkConfig = {
      type: 'file',
      config: {
        filePath: '/tmp/test-costs.jsonl',
      },
    };

    const sink = await createCostSink(config);
    expect(sink).toBeInstanceOf(FileCostSink);
  });

  it('should throw error for AWS sink (not implemented)', async () => {
    const config: CostSinkConfig = {
      type: 'aws',
    };

    await expect(createCostSink(config)).rejects.toThrow(CostSinkError);
    await expect(createCostSink(config)).rejects.toThrow('AWS cost sink not implemented');
  });

  it('should throw error for Datadog sink (not implemented)', async () => {
    const config: CostSinkConfig = {
      type: 'datadog',
    };

    await expect(createCostSink(config)).rejects.toThrow(CostSinkError);
    await expect(createCostSink(config)).rejects.toThrow('Datadog cost sink not implemented');
  });

  it('should throw error for webhook sink (not implemented)', async () => {
    const config: CostSinkConfig = {
      type: 'webhook',
    };

    await expect(createCostSink(config)).rejects.toThrow(CostSinkError);
    await expect(createCostSink(config)).rejects.toThrow('Webhook cost sink not implemented');
  });

  it('should throw error for unknown sink type', async () => {
    const config: any = {
      type: 'unknown',
    };

    await expect(createCostSink(config)).rejects.toThrow(CostSinkError);
    await expect(createCostSink(config)).rejects.toThrow('Unknown cost sink type');
  });
});
