/**
 * Tests for automatic cache TTL selection
 * Verifies the auto-TTL heuristic and session detection logic
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  selectCacheTTL,
  detectSessionReviewCount,
} from '../../src/sub-agent-orchestrator.js';

describe('Automatic Cache TTL Selection', () => {
  let originalEnv: NodeJS.ProcessEnv;

  beforeEach(() => {
    // Save original environment
    originalEnv = { ...process.env };
  });

  afterEach(() => {
    // Restore original environment
    process.env = originalEnv;
  });

  describe('selectCacheTTL', () => {
    it('should return 5min for 1 review (conservative)', () => {
      expect(selectCacheTTL(1)).toBe('5min');
    });

    it('should return 5min for 2 reviews', () => {
      expect(selectCacheTTL(2)).toBe('5min');
    });

    it('should return 5min for 3 reviews', () => {
      expect(selectCacheTTL(3)).toBe('5min');
    });

    it('should return 1h for 4 reviews (break-even point, aggressive)', () => {
      expect(selectCacheTTL(4)).toBe('1h');
    });

    it('should return 1h for 5 reviews', () => {
      expect(selectCacheTTL(5)).toBe('1h');
    });

    it('should return 1h for 10 reviews', () => {
      expect(selectCacheTTL(10)).toBe('1h');
    });

    it('should return 1h for 100 reviews', () => {
      expect(selectCacheTTL(100)).toBe('1h');
    });

    it('should handle edge case: 0 reviews', () => {
      expect(selectCacheTTL(0)).toBe('5min');
    });
  });

  describe('detectSessionReviewCount', () => {
    it('should return 1 by default (no environment variables)', () => {
      delete process.env.MULTI_PERSONA_REVIEW_COUNT;
      delete process.env.MULTI_PERSONA_BATCH_MODE;
      delete process.env.CI;

      expect(detectSessionReviewCount()).toBe(1);
    });

    it('should use explicit MULTI_PERSONA_REVIEW_COUNT', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '7';
      expect(detectSessionReviewCount()).toBe(7);
    });

    it('should handle large MULTI_PERSONA_REVIEW_COUNT', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '50';
      expect(detectSessionReviewCount()).toBe(50);
    });

    it('should handle invalid MULTI_PERSONA_REVIEW_COUNT and fall back', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = 'invalid';
      expect(detectSessionReviewCount()).toBe(1);
    });

    it('should handle negative MULTI_PERSONA_REVIEW_COUNT and fall back', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '-5';
      expect(detectSessionReviewCount()).toBe(1);
    });

    it('should handle zero MULTI_PERSONA_REVIEW_COUNT and fall back', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '0';
      expect(detectSessionReviewCount()).toBe(1);
    });

    it('should detect batch mode', () => {
      process.env.MULTI_PERSONA_BATCH_MODE = 'true';
      expect(detectSessionReviewCount()).toBe(5);
    });

    it('should ignore batch mode when set to false', () => {
      process.env.MULTI_PERSONA_BATCH_MODE = 'false';
      expect(detectSessionReviewCount()).toBe(1);
    });

    it('should detect CI environment', () => {
      process.env.CI = 'true';
      expect(detectSessionReviewCount()).toBe(4);
    });

    it('should ignore CI when set to false', () => {
      process.env.CI = 'false';
      expect(detectSessionReviewCount()).toBe(1);
    });

    it('should prioritize explicit count over batch mode', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '10';
      process.env.MULTI_PERSONA_BATCH_MODE = 'true';
      expect(detectSessionReviewCount()).toBe(10);
    });

    it('should prioritize explicit count over CI', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '8';
      process.env.CI = 'true';
      expect(detectSessionReviewCount()).toBe(8);
    });

    it('should prioritize batch mode over CI', () => {
      process.env.MULTI_PERSONA_BATCH_MODE = 'true';
      process.env.CI = 'true';
      expect(detectSessionReviewCount()).toBe(5);
    });

    it('should prioritize explicit count over all other signals', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '15';
      process.env.MULTI_PERSONA_BATCH_MODE = 'true';
      process.env.CI = 'true';
      expect(detectSessionReviewCount()).toBe(15);
    });
  });

  describe('Integration: Auto-TTL with session detection', () => {
    it('should select conservative strategy for single review session', () => {
      delete process.env.MULTI_PERSONA_REVIEW_COUNT;
      delete process.env.MULTI_PERSONA_BATCH_MODE;
      delete process.env.CI;

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(1);
      expect(strategy).toBe('5min');
    });

    it('should select aggressive strategy for batch mode session', () => {
      process.env.MULTI_PERSONA_BATCH_MODE = 'true';

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(5);
      expect(strategy).toBe('1h');
    });

    it('should select aggressive strategy for CI environment', () => {
      process.env.CI = 'true';

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(4);
      expect(strategy).toBe('1h');
    });

    it('should select aggressive strategy for explicit large count', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '20';

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(20);
      expect(strategy).toBe('1h');
    });

    it('should select conservative strategy for explicit small count', () => {
      process.env.MULTI_PERSONA_REVIEW_COUNT = '2';

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(2);
      expect(strategy).toBe('5min');
    });

    it('should handle realistic CI/CD scenario', () => {
      // Simulate GitHub Actions environment
      process.env.CI = 'true';
      process.env.GITHUB_ACTIONS = 'true';

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(4); // CI detection
      expect(strategy).toBe('1h'); // Aggressive caching
    });

    it('should handle realistic batch review scenario', () => {
      // Simulate batch review script
      process.env.MULTI_PERSONA_BATCH_MODE = 'true';
      process.env.MULTI_PERSONA_REVIEW_COUNT = '10';

      const reviewCount = detectSessionReviewCount();
      const strategy = selectCacheTTL(reviewCount);

      expect(reviewCount).toBe(10); // Explicit count takes precedence
      expect(strategy).toBe('1h'); // Aggressive caching
    });
  });

  describe('Cost optimization scenarios', () => {
    it('should recommend conservative for break-even threshold (3 reviews)', () => {
      const strategy = selectCacheTTL(3);
      expect(strategy).toBe('5min');
    });

    it('should recommend aggressive at break-even threshold (4 reviews)', () => {
      const strategy = selectCacheTTL(4);
      expect(strategy).toBe('1h');
    });

    it('should be conservative for single ad-hoc review', () => {
      const strategy = selectCacheTTL(1);
      expect(strategy).toBe('5min');
    });

    it('should be aggressive for large batch (50+ reviews)', () => {
      const strategy = selectCacheTTL(50);
      expect(strategy).toBe('1h');
    });
  });
});
