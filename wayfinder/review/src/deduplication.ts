/**
 * Finding deduplication engine
 * Groups and merges similar findings from multiple personas
 */

import type { Finding } from './types.js';

/**
 * Calculates Levenshtein distance between two strings
 * Returns the minimum number of single-character edits (insertions, deletions, substitutions)
 */
function levenshteinDistance(str1: string, str2: string): number {
  const len1 = str1.length;
  const len2 = str2.length;

  // Quick returns for edge cases
  if (len1 === 0) return len2;
  if (len2 === 0) return len1;
  if (str1 === str2) return 0;

  // Create a 2D array for dynamic programming
  // We only need current and previous row, so optimize memory
  let prevRow = new Array(len2 + 1);
  let currRow = new Array(len2 + 1);

  // Initialize first row
  for (let j = 0; j <= len2; j++) {
    prevRow[j] = j;
  }

  // Calculate distances
  for (let i = 1; i <= len1; i++) {
    currRow[0] = i;

    for (let j = 1; j <= len2; j++) {
      const cost = str1[i - 1] === str2[j - 1] ? 0 : 1;

      currRow[j] = Math.min(
        currRow[j - 1] + 1,      // insertion
        prevRow[j] + 1,          // deletion
        prevRow[j - 1] + cost    // substitution
      );
    }

    // Swap rows
    [prevRow, currRow] = [currRow, prevRow];
  }

  return prevRow[len2];
}

/**
 * Calculates similarity between two strings using normalized Levenshtein distance
 * Returns a value between 0.0 (completely different) and 1.0 (identical)
 */
function calculateSimilarity(str1: string, str2: string): number {
  const len1 = str1.length;
  const len2 = str2.length;

  if (len1 === 0) return len2 === 0 ? 1.0 : 0.0;
  if (len2 === 0) return 0.0;
  if (str1 === str2) return 1.0;

  const maxLen = Math.max(len1, len2);
  const distance = levenshteinDistance(str1, str2);

  // Convert distance to similarity (0 = identical, maxLen = completely different)
  return 1.0 - (distance / maxLen);
}

/**
 * Checks if two findings are similar enough to be considered duplicates
 */
export function areSimilarFindings(
  finding1: Finding,
  finding2: Finding,
  threshold: number = 0.8
): boolean {
  // Must be same file
  if (finding1.file !== finding2.file) return false;

  // Must be close in line numbers (within 5 lines)
  if (finding1.line && finding2.line) {
    const lineDiff = Math.abs(finding1.line - finding2.line);
    if (lineDiff > 5) return false;
  }

  // Check title similarity
  const titleSimilarity = calculateSimilarity(
    finding1.title.toLowerCase(),
    finding2.title.toLowerCase()
  );

  if (titleSimilarity >= threshold) return true;

  // Check description similarity
  const descSimilarity = calculateSimilarity(
    finding1.description.toLowerCase().substring(0, 200),
    finding2.description.toLowerCase().substring(0, 200)
  );

  return descSimilarity >= threshold;
}

/**
 * Merges similar findings into a single finding
 */
export function mergeFindings(findings: Finding[]): Finding {
  if (findings.length === 0) {
    throw new Error('Cannot merge empty findings array');
  }

  if (findings.length === 1) {
    return findings[0];
  }

  // Use the finding with highest confidence as base
  const sorted = [...findings].sort((a, b) =>
    (b.confidence || 0) - (a.confidence || 0)
  );

  const base = sorted[0];

  // Combine personas
  const allPersonas = Array.from(
    new Set(findings.flatMap(f => f.personas))
  );

  // Use highest severity
  const severities: Record<string, number> = {
    critical: 4,
    high: 3,
    medium: 2,
    low: 1,
    info: 0,
  };

  const highestSeverity = findings.reduce((max, f) => {
    return severities[f.severity] > severities[max.severity] ? f : max;
  }, findings[0]);

  // Average confidence
  const avgConfidence = findings.reduce((sum, f) => sum + (f.confidence || 0.5), 0) / findings.length;

  // Combine categories
  const allCategories = Array.from(
    new Set(findings.flatMap(f => f.categories || []))
  );

  return {
    ...base,
    personas: allPersonas,
    severity: highestSeverity.severity,
    confidence: avgConfidence,
    categories: allCategories.length > 0 ? allCategories : undefined,
  };
}

/**
 * Deduplicates findings using similarity threshold
 */
export function deduplicateFindings(
  findings: Finding[],
  threshold: number = 0.8
): { findings: Finding[]; duplicatesRemoved: number } {
  if (findings.length === 0) {
    return { findings: [], duplicatesRemoved: 0 };
  }

  const groups: Finding[][] = [];

  for (const finding of findings) {
    // Find existing group that this finding belongs to
    let foundGroup = false;

    for (const group of groups) {
      // Check similarity with first finding in group
      if (areSimilarFindings(group[0], finding, threshold)) {
        group.push(finding);
        foundGroup = true;
        break;
      }
    }

    // Create new group if no match found
    if (!foundGroup) {
      groups.push([finding]);
    }
  }

  // Merge each group
  const deduplicated = groups.map(group => mergeFindings(group));

  const duplicatesRemoved = findings.length - deduplicated.length;

  return {
    findings: deduplicated,
    duplicatesRemoved,
  };
}
