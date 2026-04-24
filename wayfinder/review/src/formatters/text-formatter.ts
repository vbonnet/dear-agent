/**
 * Text output formatter for multi-persona-review reviews
 * Formats review results as human-readable text
 */

import type { ReviewResult, Finding, Severity } from '../types.js';
import type { CacheAlert } from '../cache-alert-tracker.js';
import {
  aggregateCacheMetrics,
  calculateCacheHitRate,
  calculateCostSavings,
} from '../cost-sink.js';

/**
 * ANSI color codes for terminal output
 */
const COLORS = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',

  // Severity colors
  critical: '\x1b[41m\x1b[37m',  // White on red background
  high: '\x1b[31m',              // Red
  medium: '\x1b[33m',            // Yellow
  low: '\x1b[36m',               // Cyan
  info: '\x1b[90m',              // Gray

  // Other colors
  green: '\x1b[32m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  yellow: '\x1b[33m',
  red: '\x1b[31m',
};

/**
 * Severity symbols
 */
const SEVERITY_SYMBOLS: Record<Severity, string> = {
  critical: '🔴',
  high: '🟠',
  medium: '🟡',
  low: '🔵',
  info: 'ℹ️ ',
};

/**
 * Format options
 */
export interface TextFormatOptions {
  /** Use colors (default: true) */
  colors?: boolean;

  /** Show file paths as relative (default: true) */
  relative?: boolean;

  /** Group findings by file (default: true) */
  groupByFile?: boolean;

  /** Show cost information (default: true) */
  showCost?: boolean;

  /** Show summary statistics (default: true) */
  showSummary?: boolean;
}

/**
 * Formats a severity level with color
 */
function formatSeverity(severity: Severity, useColors: boolean): string {
  const symbol = SEVERITY_SYMBOLS[severity];
  const text = severity.toUpperCase().padEnd(8);

  if (!useColors) {
    return `${symbol} ${text}`;
  }

  const color = COLORS[severity];
  return `${symbol} ${color}${text}${COLORS.reset}`;
}

/**
 * Formats a file path
 */
function formatFilePath(path: string, useColors: boolean): string {
  if (!useColors) {
    return path;
  }

  return `${COLORS.blue}${path}${COLORS.reset}`;
}

/**
 * Formats a line number
 */
function formatLineNumber(line: number | undefined, useColors: boolean): string {
  if (line === undefined) {
    return '';
  }

  if (!useColors) {
    return `:${line}`;
  }

  return `${COLORS.dim}:${line}${COLORS.reset}`;
}

/**
 * Formats a single finding
 */
function formatFinding(finding: Finding, useColors: boolean): string {
  const lines: string[] = [];

  // Header: severity + file:line
  const location = `${formatFilePath(finding.file, useColors)}${formatLineNumber(finding.line, useColors)}`;
  const severity = formatSeverity(finding.severity, useColors);

  lines.push(`${severity} ${location}`);

  // Title
  const title = useColors
    ? `${COLORS.bold}${finding.title}${COLORS.reset}`
    : finding.title;
  lines.push(`  ${title}`);

  // Description
  if (finding.description) {
    const descLines = finding.description.split('\n');
    for (const line of descLines) {
      lines.push(`  ${line}`);
    }
  }

  // Categories
  if (finding.categories && finding.categories.length > 0) {
    const categories = finding.categories.join(', ');
    const categoriesText = useColors
      ? `${COLORS.dim}Categories: ${categories}${COLORS.reset}`
      : `Categories: ${categories}`;
    lines.push(`  ${categoriesText}`);
  }

  // Confidence
  if (finding.confidence !== undefined) {
    const confidence = `${Math.round(finding.confidence * 100)}%`;
    const confidenceText = useColors
      ? `${COLORS.dim}Confidence: ${confidence}${COLORS.reset}`
      : `Confidence: ${confidence}`;
    lines.push(`  ${confidenceText}`);
  }

  // Decision (Agency-Agents voting)
  if (finding.decision) {
    const decision = finding.decision;
    const decisionColor = decision === 'GO' ? COLORS.green : COLORS.red;
    const decisionText = useColors
      ? `${decisionColor}Decision: ${decision}${COLORS.reset}`
      : `Decision: ${decision}`;
    lines.push(`  ${decisionText}`);
  }

  // Alternatives (Agency-Agents lateral thinking)
  if (finding.alternatives && finding.alternatives.length > 0) {
    const altHeader = useColors
      ? `${COLORS.dim}Alternatives:${COLORS.reset}`
      : `Alternatives:`;
    lines.push(`  ${altHeader}`);
    for (const alt of finding.alternatives) {
      lines.push(`    • ${alt}`);
    }
  }

  // Out of scope flag (Agency-Agents expertise boundary)
  if (finding.outOfScope) {
    const outOfScopeText = useColors
      ? `${COLORS.yellow}⚠️  Out of Scope${COLORS.reset}`
      : `⚠️  Out of Scope`;
    lines.push(`  ${outOfScopeText}`);
  }

  lines.push(''); // Empty line between findings

  return lines.join('\n');
}

/**
 * Formats findings grouped by file
 */
function formatByFile(result: ReviewResult, useColors: boolean): string {
  const lines: string[] = [];

  for (const [filePath, findings] of Object.entries(result.findingsByFile)) {
    // File header
    const fileHeader = useColors
      ? `${COLORS.bold}${COLORS.blue}${filePath}${COLORS.reset} ${COLORS.dim}(${findings.length} finding${findings.length === 1 ? '' : 's'})${COLORS.reset}`
      : `${filePath} (${findings.length} finding${findings.length === 1 ? '' : 's'})`;

    lines.push(fileHeader);
    lines.push('─'.repeat(80));
    lines.push('');

    // Findings
    for (const finding of findings) {
      lines.push(formatFinding(finding, useColors));
    }
  }

  return lines.join('\n');
}

/**
 * Formats findings as a flat list
 */
function formatFlat(result: ReviewResult, useColors: boolean): string {
  const lines: string[] = [];

  for (const finding of result.findings) {
    lines.push(formatFinding(finding, useColors));
  }

  return lines.join('\n');
}

/**
 * Formats summary statistics
 */
function formatSummary(result: ReviewResult, useColors: boolean): string {
  const lines: string[] = [];
  const { summary } = result;

  // Header
  const header = useColors
    ? `${COLORS.bold}Review Summary${COLORS.reset}`
    : 'Review Summary';

  lines.push('');
  lines.push('═'.repeat(80));
  lines.push(header);
  lines.push('═'.repeat(80));
  lines.push('');

  // Files reviewed
  lines.push(`Files reviewed:     ${summary.filesReviewed}`);

  // Total findings
  lines.push(`Total findings:     ${summary.totalFindings}`);

  // Findings by severity
  lines.push('');
  lines.push('Findings by severity:');
  for (const severity of ['critical', 'high', 'medium', 'low', 'info'] as Severity[]) {
    const count = summary.findingsBySeverity[severity];
    if (count > 0) {
      const severityLabel = formatSeverity(severity, useColors);
      lines.push(`  ${severityLabel} ${count}`);
    }
  }

  return lines.join('\n');
}

/**
 * Formats cost information
 */
function formatCost(result: ReviewResult, useColors: boolean): string {
  const lines: string[] = [];
  const { cost } = result;

  lines.push('');
  lines.push('─'.repeat(80));

  const costHeader = useColors
    ? `${COLORS.bold}Cost Information${COLORS.reset}`
    : 'Cost Information';

  lines.push(costHeader);
  lines.push('─'.repeat(80));
  lines.push('');

  // Total cost
  const totalCost = `$${cost.totalCost.toFixed(4)}`;
  lines.push(`Total cost:         ${totalCost}`);
  lines.push(`Total tokens:       ${cost.totalTokens.toLocaleString()}`);

  // Per-persona breakdown
  if (Object.keys(cost.byPersona).length > 1) {
    lines.push('');
    lines.push('By persona:');
    for (const [persona, personaCost] of Object.entries(cost.byPersona)) {
      const costStr = `$${personaCost.cost.toFixed(4)}`;
      const tokensStr = `${(personaCost.inputTokens + personaCost.outputTokens).toLocaleString()} tokens`;
      lines.push(`  ${persona.padEnd(30)} ${costStr.padEnd(10)} ${tokensStr}`);
    }
  }

  return lines.join('\n');
}

/**
 * Formats cache performance information
 */
function formatCachePerformance(result: ReviewResult, useColors: boolean): string {
  const lines: string[] = [];
  const { cost } = result;

  // Aggregate cache metrics across all personas
  const cacheMetrics = aggregateCacheMetrics(cost);

  // Only show cache section if there's cache activity
  if (cacheMetrics.cacheReadTokens === 0 && cacheMetrics.cacheCreationTokens === 0) {
    return '';
  }

  lines.push('');
  lines.push('─'.repeat(80));

  const cacheHeader = useColors
    ? `${COLORS.bold}Cache Performance${COLORS.reset}`
    : 'Cache Performance';

  lines.push(cacheHeader);
  lines.push('─'.repeat(80));
  lines.push('');

  // Hit rate
  const hitRate = calculateCacheHitRate(cacheMetrics);
  const cacheHits = cacheMetrics.cacheReadTokens;
  const cacheMisses = cacheMetrics.inputTokens;
  const hitRateStr = useColors
    ? `${COLORS.green}${hitRate.toFixed(1)}%${COLORS.reset}`
    : `${hitRate.toFixed(1)}%`;
  lines.push(`Hit Rate:           ${hitRateStr} (${cacheHits.toLocaleString()} hits, ${cacheMisses.toLocaleString()} misses)`);

  // Tokens saved
  const tokensSaved = cacheMetrics.cacheReadTokens;
  lines.push(`Tokens Saved:       ${tokensSaved.toLocaleString()} (cache reads)`);

  // Cost savings
  const costSavings = calculateCostSavings(cacheMetrics);
  const savingsStr = useColors
    ? `${COLORS.green}$${costSavings.savings.toFixed(4)}${COLORS.reset}`
    : `$${costSavings.savings.toFixed(4)}`;
  const savingsPercent = useColors
    ? `${COLORS.green}${costSavings.savingsPercent.toFixed(1)}%${COLORS.reset}`
    : `${costSavings.savingsPercent.toFixed(1)}%`;
  lines.push(`Cost Savings:       ${savingsStr} (${savingsPercent} reduction)`);

  return lines.join('\n');
}

/**
 * Formats cache alerts
 */
export function formatCacheAlerts(alerts: CacheAlert[], useColors: boolean): string {
  if (alerts.length === 0) {
    return '';
  }

  const lines: string[] = [];
  lines.push('');
  lines.push('─'.repeat(80));

  const alertHeader = useColors
    ? `${COLORS.bold}${COLORS.yellow}⚠ Cache Performance Alerts${COLORS.reset}`
    : '⚠ Cache Performance Alerts';

  lines.push(alertHeader);
  lines.push('─'.repeat(80));
  lines.push('');

  for (const alert of alerts) {
    // Severity indicator
    const severityStr = alert.severity === 'critical'
      ? (useColors ? `${COLORS.red}${COLORS.bold}CRITICAL${COLORS.reset}` : 'CRITICAL')
      : (useColors ? `${COLORS.yellow}WARNING${COLORS.reset}` : 'WARNING');

    lines.push(`[${severityStr}] ${alert.persona}`);
    lines.push('');

    // Hit rate
    const hitRateColor = alert.currentHitRate < 30 ? COLORS.red : COLORS.yellow;
    const hitRateStr = useColors
      ? `${hitRateColor}${alert.currentHitRate.toFixed(1)}%${COLORS.reset}`
      : `${alert.currentHitRate.toFixed(1)}%`;
    lines.push(`  Cache Hit Rate:     ${hitRateStr} (target: ${alert.targetHitRate}%)`);
    lines.push(`  Consecutive Issues: ${alert.consecutiveFailures} reviews`);
    lines.push('');

    // Message
    lines.push(`  ${alert.message}`);
    lines.push('');

    // Suggestions
    if (alert.suggestions.length > 0) {
      lines.push('  Suggestions:');
      for (const suggestion of alert.suggestions) {
        lines.push(`    • ${suggestion}`);
      }
      lines.push('');
    }
  }

  return lines.join('\n');
}

/**
 * Formats a review result as human-readable text
 */
export function formatReviewResult(
  result: ReviewResult,
  options: TextFormatOptions = {}
): string {
  const {
    colors = true,
    groupByFile = true,
    showCost = true,
    showSummary = true,
  } = options;

  const sections: string[] = [];

  // Main findings section
  if (result.findings.length > 0) {
    if (groupByFile) {
      sections.push(formatByFile(result, colors));
    } else {
      sections.push(formatFlat(result, colors));
    }
  } else {
    const noFindings = colors
      ? `${COLORS.green}${COLORS.bold}✓ No issues found!${COLORS.reset}`
      : '✓ No issues found!';
    sections.push(noFindings);
    sections.push('');
  }

  // Summary section
  if (showSummary) {
    sections.push(formatSummary(result, colors));
  }

  // Cost section
  if (showCost) {
    sections.push(formatCost(result, colors));
  }

  // Cache performance section
  if (showCost) {
    const cacheSection = formatCachePerformance(result, colors);
    if (cacheSection) {
      sections.push(cacheSection);
    }
  }

  return sections.join('\n');
}
