/**
 * JSON output formatter for multi-persona-review reviews
 */

import type { ReviewResult } from '../types.js';

/**
 * Formats review result as JSON
 */
export function formatReviewResultJSON(result: ReviewResult): string {
  return JSON.stringify(result, null, 2);
}
