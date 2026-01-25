/**
 * Token Counter
 *
 * Counts tokens using tiktoken (BPE encoding) with fallback to heuristic.
 *
 * @module benchmarks/lib/token-counter
 */

export interface TokenCountResult {
  count: number;
  method: 'tiktoken' | 'heuristic';
}

let encoder: any = null;
let useTiktoken = true;

// Try initializing tiktoken
try {
  const { encodingForModel } = require('tiktoken');
  encoder = encodingForModel('gpt-4');
  console.log('[token-counter] tiktoken initialized successfully');
} catch (error: any) {
  console.warn('[token-counter] tiktoken WASM failed, using heuristic fallback');
  console.warn(`[token-counter] Error: ${error.message}`);
  useTiktoken = false;
}

/**
 * Count tokens in text using tiktoken or heuristic fallback
 *
 * @param text - Text to count tokens for
 * @returns Token count and method used
 */
export function countTokens(text: string): TokenCountResult {
  if (useTiktoken && encoder) {
    try {
      const tokens = encoder.encode(text);
      return { count: tokens.length, method: 'tiktoken' };
    } catch (error: any) {
      console.warn('[token-counter] tiktoken encoding failed, using heuristic');
      useTiktoken = false;
    }
  }

  // Heuristic: chars / 4 (approximation for GPT tokenization)
  return {
    count: Math.ceil(text.length / 4),
    method: 'heuristic',
  };
}

/**
 * Get the token counting method being used
 *
 * @returns 'tiktoken' or 'heuristic'
 */
export function getTokenCountingMethod(): 'tiktoken' | 'heuristic' {
  return useTiktoken ? 'tiktoken' : 'heuristic';
}

// Clean up on exit
process.on('exit', () => {
  if (encoder) {
    try {
      encoder.free();
      console.log('[token-counter] tiktoken encoder freed');
    } catch (error) {
      // Ignore cleanup errors
    }
  }
});
