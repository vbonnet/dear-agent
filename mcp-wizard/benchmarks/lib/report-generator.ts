/**
 * Report Generator
 *
 * Generates markdown performance report from benchmark results.
 *
 * @module benchmarks/lib/report-generator
 */

import * as fs from 'fs';
import * as path from 'path';

export interface ContextUsageResult {
  scenario: string;
  baselineTokens: number;
  lazyTokens: number;
  reductionPercent: number;
}

export interface LatencyResult {
  operation: string;
  p50: number;
  p95: number;
  p99: number;
}

export interface StartupResult {
  gateway: { p50: number; p95: number; p99: number };
  direct: { p50: number; p95: number; p99: number };
}

export interface MemoryResult {
  gateway: { rss: number; heapUsed: number };
  direct: { rss: number; heapUsed: number };
  overhead: { rssPercent: number; heapPercent: number };
}

export interface ReportData {
  contextUsage: ContextUsageResult[];
  latency: LatencyResult[];
  startup?: StartupResult;
  memory?: MemoryResult;
  tokenCountingMethod: 'tiktoken' | 'heuristic';
  timestamp: string;
}

/**
 * Generate markdown performance report
 *
 * @param data - Benchmark results
 * @param outputPath - Path to write report (default: benchmarks/PERFORMANCE-REPORT.md)
 */
export function generateReport(
  data: ReportData,
  outputPath = path.join(__dirname, '..', 'PERFORMANCE-REPORT.md')
): void {
  const markdown = buildMarkdown(data);

  fs.writeFileSync(outputPath, markdown, 'utf-8');
  console.log(`\nPerformance report generated: ${outputPath}`);
}

function buildMarkdown(data: ReportData): string {
  const sections: string[] = [];

  // Header
  sections.push('# MCP Performance Benchmark Report\n');
  sections.push(`**Generated**: ${data.timestamp}\n`);
  sections.push(`**Token Counting Method**: ${data.tokenCountingMethod}\n`);

  // Executive Summary
  sections.push('## Executive Summary\n');
  sections.push(buildExecutiveSummary(data));

  // Context Usage
  sections.push('## Context Usage (Token Count)\n');
  sections.push(buildContextUsageTable(data.contextUsage));

  // Latency
  sections.push('## Latency Percentiles\n');
  sections.push(buildLatencyTable(data.latency));

  // Startup (if available)
  if (data.startup) {
    sections.push('## Startup Time\n');
    sections.push(buildStartupTable(data.startup));
  }

  // Memory (if available)
  if (data.memory) {
    sections.push('## Memory Overhead\n');
    sections.push(buildMemoryTable(data.memory));
  }

  // Recommendations
  sections.push('## Recommendations\n');
  sections.push(buildRecommendations(data));

  // Methodology
  sections.push('## Methodology\n');
  sections.push(buildMethodology(data));

  return sections.join('\n');
}

function buildExecutiveSummary(data: ReportData): string {
  const avgReduction =
    data.contextUsage.reduce((sum, r) => sum + r.reductionPercent, 0) /
    data.contextUsage.length;

  const maxLatency = Math.max(...data.latency.map((l) => l.p99));

  const lines: string[] = [
    `- **Context Reduction**: ${avgReduction.toFixed(1)}% average (lazy vs eager loading)`,
    `- **Latency p99**: ${maxLatency.toFixed(2)}ms (max across operations)`,
  ];

  if (data.startup) {
    const overhead =
      data.startup.gateway.p99 - data.startup.direct.p99;
    lines.push(
      `- **Startup Overhead**: ${overhead.toFixed(2)}ms (gateway vs direct)`
    );
  }

  if (data.memory) {
    lines.push(
      `- **Memory Overhead**: ${data.memory.overhead.rssPercent.toFixed(1)}% RSS, ${data.memory.overhead.heapPercent.toFixed(1)}% heap`
    );
  }

  return lines.join('\n') + '\n';
}

function buildContextUsageTable(results: ContextUsageResult[]): string {
  const headers = ['Scenario', 'Baseline Tokens', 'Lazy Tokens', 'Reduction %'];
  const rows = results.map((r) => [
    r.scenario,
    r.baselineTokens.toString(),
    r.lazyTokens.toString(),
    r.reductionPercent.toFixed(2) + '%',
  ]);

  return generateMarkdownTable(headers, rows) + '\n';
}

function buildLatencyTable(results: LatencyResult[]): string {
  const headers = ['Operation', 'p50 (ms)', 'p95 (ms)', 'p99 (ms)'];
  const rows = results.map((r) => [
    r.operation,
    r.p50.toFixed(2),
    r.p95.toFixed(2),
    r.p99.toFixed(2),
  ]);

  return generateMarkdownTable(headers, rows) + '\n';
}

function buildStartupTable(startup: StartupResult): string {
  const headers = ['Mode', 'p50 (ms)', 'p95 (ms)', 'p99 (ms)'];
  const rows = [
    [
      'Gateway',
      startup.gateway.p50.toFixed(2),
      startup.gateway.p95.toFixed(2),
      startup.gateway.p99.toFixed(2),
    ],
    [
      'Direct',
      startup.direct.p50.toFixed(2),
      startup.direct.p95.toFixed(2),
      startup.direct.p99.toFixed(2),
    ],
  ];

  return generateMarkdownTable(headers, rows) + '\n';
}

function buildMemoryTable(memory: MemoryResult): string {
  const headers = ['Mode', 'RSS (MB)', 'Heap Used (MB)'];
  const rows = [
    [
      'Gateway',
      memory.gateway.rss.toString(),
      memory.gateway.heapUsed.toString(),
    ],
    [
      'Direct',
      memory.direct.rss.toString(),
      memory.direct.heapUsed.toString(),
    ],
    [
      'Overhead',
      `${memory.overhead.rssPercent.toFixed(1)}%`,
      `${memory.overhead.heapPercent.toFixed(1)}%`,
    ],
  ];

  return generateMarkdownTable(headers, rows) + '\n';
}

function buildRecommendations(data: ReportData): string {
  const recommendations: string[] = [];

  // Context reduction
  const avgReduction =
    data.contextUsage.reduce((sum, r) => sum + r.reductionPercent, 0) /
    data.contextUsage.length;

  if (avgReduction > 80) {
    recommendations.push(
      `✅ **Context reduction exceeds 80%** (${avgReduction.toFixed(1)}%) - lazy loading is highly effective`
    );
  } else if (avgReduction > 50) {
    recommendations.push(
      `⚠️ **Context reduction is moderate** (${avgReduction.toFixed(1)}%) - consider improving intent analysis accuracy`
    );
  } else {
    recommendations.push(
      `❌ **Context reduction is low** (${avgReduction.toFixed(1)}%) - intent filtering may not be working as expected`
    );
  }

  // Latency
  const maxLatency = Math.max(...data.latency.map((l) => l.p99));
  if (maxLatency < 50) {
    recommendations.push(
      `✅ **Latency p99 < 50ms** (${maxLatency.toFixed(2)}ms) - meets target`
    );
  } else {
    recommendations.push(
      `⚠️ **Latency p99 > 50ms** (${maxLatency.toFixed(2)}ms) - consider caching or optimization`
    );
  }

  // Token counting method
  if (data.tokenCountingMethod === 'heuristic') {
    recommendations.push(
      '⚠️ **Using heuristic token counting** - consider installing tiktoken for accurate measurements'
    );
  }

  return recommendations.map((r) => `- ${r}`).join('\n') + '\n';
}

function buildMethodology(data: ReportData): string {
  return `
- **Iterations**: 100 per benchmark (warm-up: 10 iterations)
- **Timing**: \`performance.now()\` (high-resolution)
- **Percentile Calculation**: Sort samples, index at percentile threshold
- **Token Counting**: ${data.tokenCountingMethod === 'tiktoken' ? 'tiktoken (BPE encoding)' : 'Heuristic (chars / 4)'}
- **Test Scenarios**: 4 synthetic cases (googledocs, atlassian, slack, ambiguous)
`;
}

function generateMarkdownTable(headers: string[], rows: string[][]): string {
  const headerRow = `| ${headers.join(' | ')} |`;
  const separator = `|${headers.map(() => '---').join('|')}|`;
  const dataRows = rows.map((row) => `| ${row.join(' | ')} |`).join('\n');

  return `${headerRow}\n${separator}\n${dataRows}`;
}
