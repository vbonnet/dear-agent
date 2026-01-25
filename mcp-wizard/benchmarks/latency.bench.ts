#!/usr/bin/env tsx
/**
 * Latency Benchmark
 *
 * Measures p95/p99 latency for schema filtering and intent analysis.
 *
 * Usage: tsx benchmarks/latency.bench.ts
 */

import { SchemaFilter } from '../src/lib/schema-filter';
import { IntentAnalyzer } from '../src/lib/intent-analyzer';
import { runBenchmark } from './lib/harness';
import { scenarios } from './lib/scenarios';

interface LatencyResult {
  operation: string;
  p50: number;
  p95: number;
  p99: number;
}

async function main() {
  console.log('=== Latency Benchmark ===\n');

  const schemaFilter = new SchemaFilter();
  const intentAnalyzer = new IntentAnalyzer();

  const results: LatencyResult[] = [];

  // Mock schemas for benchmarking
  const mockSchemas = createMockSchemas();

  for (const scenario of scenarios) {
    console.log(`\n--- Scenario: ${scenario.name} ---`);

    // Benchmark intent analysis
    const intentResult = await runBenchmark(
      `Intent Analysis (${scenario.name})`,
      async () => {
        await intentAnalyzer.analyze(scenario.intent);
      }
    );

    results.push({
      operation: `intent-analyzer-${scenario.name}`,
      p50: intentResult.p50,
      p95: intentResult.p95,
      p99: intentResult.p99,
    });

    // Benchmark schema filtering
    // Note: We analyze intent once first, then benchmark filtering
    const intent = await intentAnalyzer.analyze(scenario.intent);
    const filterResult = await runBenchmark(
      `Schema Filtering (${scenario.name})`,
      async () => {
        schemaFilter.filter(mockSchemas, intent);
      }
    );

    results.push({
      operation: `schema-filter-${scenario.name}`,
      p50: filterResult.p50,
      p95: filterResult.p95,
      p99: filterResult.p99,
    });
  }

  // Output results
  console.log('\n\n=== Summary ===\n');
  console.log('Operation                        | p50 (ms) | p95 (ms) | p99 (ms)');
  console.log('-------------------------------- | -------- | -------- | --------');

  for (const result of results) {
    const opName = result.operation.padEnd(32);
    console.log(
      `${opName} | ${result.p50.toFixed(2).padStart(8)} | ${result.p95.toFixed(2).padStart(8)} | ${result.p99.toFixed(2).padStart(8)}`
    );
  }

  console.log('\n=== Results (JSON) ===');
  console.log(JSON.stringify(results, null, 2));

  // Check against targets
  console.log('\n=== Target Validation ===');
  const schemaFilterP99 = Math.max(
    ...results
      .filter((r) => r.operation.startsWith('schema-filter'))
      .map((r) => r.p99)
  );
  const intentP99 = Math.max(
    ...results
      .filter((r) => r.operation.startsWith('intent-analyzer'))
      .map((r) => r.p99)
  );

  console.log(
    `Schema Filter p99: ${schemaFilterP99.toFixed(2)}ms (target: <20ms) ${schemaFilterP99 < 20 ? '✅' : '⚠️'}`
  );
  console.log(
    `Intent Analyzer p99: ${intentP99.toFixed(2)}ms (target: <50ms) ${intentP99 < 50 ? '✅' : '⚠️'}`
  );

  return results;
}

function createMockSchemas(): any {
  // Same mock schemas as context-usage.bench.ts
  return {
    googledocs: {
      tools: [
        {
          name: 'readGoogleDoc',
          description: 'Read content from a Google Document',
          inputSchema: {
            type: 'object',
            properties: {
              documentId: { type: 'string' },
              format: { type: 'string', enum: ['text', 'json', 'markdown'] },
            },
            required: ['documentId'],
          },
        },
        {
          name: 'listDocumentTabs',
          description: 'List all tabs in a Google Document',
          inputSchema: {
            type: 'object',
            properties: {
              documentId: { type: 'string' },
            },
            required: ['documentId'],
          },
        },
      ],
    },
    atlassian: {
      tools: [
        {
          name: 'searchJiraIssues',
          description: 'Search for Jira issues using JQL',
          inputSchema: {
            type: 'object',
            properties: {
              jql: { type: 'string' },
              maxResults: { type: 'number' },
            },
            required: ['jql'],
          },
        },
      ],
    },
    slack: {
      tools: [
        {
          name: 'searchMessages',
          description: 'Search Slack messages',
          inputSchema: {
            type: 'object',
            properties: {
              query: { type: 'string' },
              count: { type: 'number' },
            },
            required: ['query'],
          },
        },
      ],
    },
  };
}

// Run if executed directly
if (require.main === module) {
  main().catch((error) => {
    console.error('Benchmark failed:', error);
    process.exit(1);
  });
}

export { main as runLatencyBenchmark };
