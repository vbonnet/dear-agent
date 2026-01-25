#!/usr/bin/env tsx
/**
 * Context Usage Benchmark
 *
 * Measures token count for eager vs lazy loading across test scenarios.
 *
 * Usage: tsx benchmarks/context-usage.bench.ts
 */

import { SchemaFilter } from '../src/lib/schema-filter';
import { IntentAnalyzer } from '../src/lib/intent-analyzer';
import { countTokens, getTokenCountingMethod } from './lib/token-counter';
import { scenarios } from './lib/scenarios';

interface ContextResult {
  scenario: string;
  baselineTokens: number;
  lazyTokens: number;
  reductionPercent: number;
}

async function main() {
  console.log('=== Context Usage Benchmark ===\n');
  console.log(`Token counting method: ${getTokenCountingMethod()}\n`);

  const schemaFilter = new SchemaFilter();
  const intentAnalyzer = new IntentAnalyzer();

  // Load all schemas (eager loading baseline)
  // Note: In a real implementation, you would load actual MCP schemas here
  // For this benchmark, we'll create mock schemas
  const allSchemas = createMockSchemas();
  const baselineJson = JSON.stringify(allSchemas);
  const baselineResult = countTokens(baselineJson);

  console.log(`Baseline (all schemas): ${baselineResult.count} tokens\n`);

  const results: ContextResult[] = [];

  for (const scenario of scenarios) {
    console.log(`Scenario: ${scenario.name}`);
    console.log(`  Intent: "${scenario.intent}"`);

    // Apply intent filtering
    const intent = await intentAnalyzer.analyze(scenario.intent);
    const filteredSchemas = schemaFilter.filter(allSchemas, intent);

    const lazyJson = JSON.stringify(filteredSchemas);
    const lazyResult = countTokens(lazyJson);

    const reduction =
      ((baselineResult.count - lazyResult.count) / baselineResult.count) * 100;

    console.log(`  Filtered schemas: ${lazyResult.count} tokens`);
    console.log(`  Reduction: ${reduction.toFixed(2)}%\n`);

    results.push({
      scenario: scenario.name,
      baselineTokens: baselineResult.count,
      lazyTokens: lazyResult.count,
      reductionPercent: reduction,
    });
  }

  // Output results as JSON for report generator
  console.log('\n=== Results (JSON) ===');
  console.log(JSON.stringify(results, null, 2));

  return results;
}

/**
 * Create mock MCP schemas for benchmarking
 *
 * In a real implementation, this would load actual schemas from MCP servers.
 * For benchmarking purposes, we create representative mock data.
 */
function createMockSchemas(): any {
  const schemas: any = {};

  // Mock googledocs tools
  schemas.googledocs = {
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
      {
        name: 'appendToGoogleDoc',
        description: 'Append text to a Google Document',
        inputSchema: {
          type: 'object',
          properties: {
            documentId: { type: 'string' },
            textToAppend: { type: 'string' },
          },
          required: ['documentId', 'textToAppend'],
        },
      },
    ],
  };

  // Mock atlassian tools
  schemas.atlassian = {
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
      {
        name: 'getJiraIssue',
        description: 'Get details of a Jira issue',
        inputSchema: {
          type: 'object',
          properties: {
            issueKey: { type: 'string' },
          },
          required: ['issueKey'],
        },
      },
    ],
  };

  // Mock slack tools
  schemas.slack = {
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
      {
        name: 'getChannelHistory',
        description: 'Get message history from a Slack channel',
        inputSchema: {
          type: 'object',
          properties: {
            channel: { type: 'string' },
            limit: { type: 'number' },
          },
          required: ['channel'],
        },
      },
    ],
  };

  return schemas;
}

// Run if executed directly
if (require.main === module) {
  main().catch((error) => {
    console.error('Benchmark failed:', error);
    process.exit(1);
  });
}

export { main as runContextUsageBenchmark };
