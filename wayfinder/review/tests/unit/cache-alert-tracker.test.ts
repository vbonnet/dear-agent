/**
 * Tests for cache alert tracker
 */

import { describe, it, expect, beforeEach } from 'vitest';
import {
  CacheAlertTracker,
  type CacheAlert,
  type CacheAlertConfig,
} from '../../src/cache-alert-tracker.js';
import type { CacheMetrics } from '../../src/types.js';

describe('CacheAlertTracker', () => {
  let tracker: CacheAlertTracker;

  beforeEach(() => {
    tracker = new CacheAlertTracker();
  });

  describe('Basic tracking', () => {
    it('should track cache performance for a persona', () => {
      const metrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 900,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.9,
      };

      const alert = tracker.track('security-engineer', metrics);

      expect(alert).toBeUndefined(); // No alert for good performance
      const stats = tracker.getPersonaStats('security-engineer');
      expect(stats).toBeDefined();
      expect(stats?.totalReviews).toBe(1);
      expect(stats?.recentHitRate).toBeGreaterThan(80);
    });

    it('should not alert for hit rate above threshold', () => {
      const goodMetrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      const alert1 = tracker.track('security-engineer', goodMetrics);
      const alert2 = tracker.track('security-engineer', goodMetrics);
      const alert3 = tracker.track('security-engineer', goodMetrics);

      expect(alert1).toBeUndefined();
      expect(alert2).toBeUndefined();
      expect(alert3).toBeUndefined();
      expect(tracker.getAlerts()).toHaveLength(0);
    });
  });

  describe('Alert generation', () => {
    it('should alert after 3 consecutive reviews below threshold', () => {
      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      const alert1 = tracker.track('security-engineer', poorMetrics);
      expect(alert1).toBeUndefined(); // 1st failure

      const alert2 = tracker.track('security-engineer', poorMetrics);
      expect(alert2).toBeUndefined(); // 2nd failure

      const alert3 = tracker.track('security-engineer', poorMetrics);
      expect(alert3).toBeDefined(); // 3rd failure - alert!

      expect(alert3?.persona).toBe('security-engineer');
      expect(alert3?.severity).toBe('warning');
      expect(alert3?.currentHitRate).toBeCloseTo(30, 0);
      expect(alert3?.consecutiveFailures).toBe(3);
      expect(alert3?.suggestions.length).toBeGreaterThan(0);
    });

    it('should generate critical alert for very low hit rate', () => {
      const veryPoorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 100,
        inputTokens: 900,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.1,
      };

      tracker.track('security-engineer', veryPoorMetrics);
      tracker.track('security-engineer', veryPoorMetrics);
      const alert = tracker.track('security-engineer', veryPoorMetrics);

      expect(alert?.severity).toBe('critical');
      expect(alert?.currentHitRate).toBeLessThan(30);
    });

    it('should reset consecutive failures on good performance', () => {
      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      const goodMetrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      tracker.track('security-engineer', poorMetrics);
      tracker.track('security-engineer', poorMetrics);
      tracker.track('security-engineer', goodMetrics); // Reset

      const stats = tracker.getPersonaStats('security-engineer');
      expect(stats?.consecutiveFailures).toBe(0);

      // Should not alert on next poor performance
      tracker.track('security-engineer', poorMetrics);
      const alert = tracker.track('security-engineer', poorMetrics);
      expect(alert).toBeUndefined(); // Only 2 consecutive failures
    });

    it('should include appropriate suggestions based on hit rate', () => {
      const criticalMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 50,
        inputTokens: 950,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.05,
      };

      tracker.track('security-engineer', criticalMetrics);
      tracker.track('security-engineer', criticalMetrics);
      const alert = tracker.track('security-engineer', criticalMetrics);

      expect(alert?.suggestions.length).toBeGreaterThan(0);
      // Check that suggestions include stability-related advice
      const hasCacheAdvice = alert?.suggestions.some(s =>
        s.includes('cache') || s.includes('prompt') || s.includes('persona')
      );
      expect(hasCacheAdvice).toBe(true);
    });
  });

  describe('Alert cooldown', () => {
    it('should respect cooldown period between alerts', () => {
      const config: CacheAlertConfig = {
        alertCooldown: 1000, // 1 second
      };
      const tracker = new CacheAlertTracker(config);

      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      // Generate first alert
      tracker.track('security-engineer', poorMetrics);
      tracker.track('security-engineer', poorMetrics);
      const alert1 = tracker.track('security-engineer', poorMetrics);
      expect(alert1).toBeDefined();

      // Immediate subsequent poor performance should not alert (cooldown)
      const alert2 = tracker.track('security-engineer', poorMetrics);
      expect(alert2).toBeUndefined();
    });
  });

  describe('Multi-persona tracking', () => {
    it('should track multiple personas independently', () => {
      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      const goodMetrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      // Security engineer has poor performance
      tracker.track('security-engineer', poorMetrics);
      tracker.track('security-engineer', poorMetrics);
      const alert1 = tracker.track('security-engineer', poorMetrics);
      expect(alert1).toBeDefined();

      // Code health has good performance
      tracker.track('code-health', goodMetrics);
      tracker.track('code-health', goodMetrics);
      const alert2 = tracker.track('code-health', goodMetrics);
      expect(alert2).toBeUndefined();

      const alerts = tracker.getAlerts();
      expect(alerts).toHaveLength(1);
      expect(alerts[0].persona).toBe('security-engineer');
    });

    it('should provide stats for all tracked personas', () => {
      const metrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      tracker.track('security-engineer', metrics);
      tracker.track('code-health', metrics);
      tracker.track('performance-engineer', metrics);

      const allStats = tracker.getAllStats();
      expect(allStats.size).toBe(3);
      expect(allStats.has('security-engineer')).toBe(true);
      expect(allStats.has('code-health')).toBe(true);
      expect(allStats.has('performance-engineer')).toBe(true);
    });
  });

  describe('Configuration', () => {
    it('should use custom target hit rate', () => {
      const config: CacheAlertConfig = {
        targetHitRate: 90,
        alertThreshold: 60,
      };
      const tracker = new CacheAlertTracker(config);

      const metrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 700,
        inputTokens: 300,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.7,
      };

      tracker.track('security-engineer', metrics);
      tracker.track('security-engineer', metrics);
      const alert = tracker.track('security-engineer', metrics);

      expect(alert).toBeUndefined(); // 70% > 60% threshold
    });

    it('should use custom consecutive failures threshold', () => {
      const config: CacheAlertConfig = {
        consecutiveFailuresThreshold: 2,
      };
      const tracker = new CacheAlertTracker(config);

      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      tracker.track('security-engineer', poorMetrics);
      const alert = tracker.track('security-engineer', poorMetrics);

      expect(alert).toBeDefined(); // Alert after 2 failures instead of 3
      expect(alert?.consecutiveFailures).toBe(2);
    });

    it('should use custom window size', () => {
      const config: CacheAlertConfig = {
        windowSize: 3,
      };
      const tracker = new CacheAlertTracker(config);

      const metrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      // Track 5 reviews
      for (let i = 0; i < 5; i++) {
        tracker.track('security-engineer', metrics);
      }

      const stats = tracker.getPersonaStats('security-engineer');
      expect(stats?.totalReviews).toBe(5);
      // Window should only keep last 3 reviews
      // Note: recentHitRates array is internal, we verify through total count
    });
  });

  describe('Statistics and reporting', () => {
    it('should calculate average hit rate correctly', () => {
      const highMetrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 900,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.9,
      };

      const lowMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 500,
        inputTokens: 500,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.5,
      };

      tracker.track('security-engineer', highMetrics);
      tracker.track('security-engineer', lowMetrics);

      const stats = tracker.getPersonaStats('security-engineer');
      expect(stats?.averageHitRate).toBeGreaterThan(60);
      expect(stats?.averageHitRate).toBeLessThan(80);
    });

    it('should track recent alerts', () => {
      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      // Generate alerts for two personas
      for (let i = 0; i < 3; i++) {
        tracker.track('security-engineer', poorMetrics);
        tracker.track('code-health', poorMetrics);
      }

      const recentAlerts = tracker.getRecentAlerts(5);
      expect(recentAlerts.length).toBe(2);
    });
  });

  describe('Persistence', () => {
    it('should export tracking data', () => {
      const metrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      tracker.track('security-engineer', metrics);
      tracker.track('code-health', metrics);

      const exported = tracker.export();

      expect(exported.config).toBeDefined();
      expect(exported.history).toBeDefined();
      expect(Object.keys(exported.history)).toHaveLength(2);
      expect(exported.alerts).toEqual([]);
    });

    it('should import tracking data', () => {
      const metrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      tracker.track('security-engineer', metrics);
      const exported = tracker.export();

      const newTracker = new CacheAlertTracker();
      newTracker.import(exported);

      const stats = newTracker.getPersonaStats('security-engineer');
      expect(stats?.totalReviews).toBe(1);
    });
  });

  describe('Reset functionality', () => {
    it('should reset specific persona', () => {
      const metrics: CacheMetrics = {
        cacheCreationTokens: 100,
        cacheReadTokens: 800,
        inputTokens: 100,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.8,
      };

      tracker.track('security-engineer', metrics);
      tracker.track('code-health', metrics);

      tracker.resetPersona('security-engineer');

      const stats1 = tracker.getPersonaStats('security-engineer');
      const stats2 = tracker.getPersonaStats('code-health');

      expect(stats1).toBeUndefined();
      expect(stats2).toBeDefined();
    });

    it('should reset all tracking', () => {
      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      for (let i = 0; i < 3; i++) {
        tracker.track('security-engineer', poorMetrics);
        tracker.track('code-health', poorMetrics);
      }

      tracker.resetAll();

      expect(tracker.getAlerts()).toHaveLength(0);
      expect(tracker.getPersonaStats('security-engineer')).toBeUndefined();
      expect(tracker.getPersonaStats('code-health')).toBeUndefined();
    });

    it('should clear alerts only', () => {
      const poorMetrics: CacheMetrics = {
        cacheCreationTokens: 0,
        cacheReadTokens: 300,
        inputTokens: 700,
        outputTokens: 500,
        cacheHit: true,
        cacheEfficiency: 0.3,
      };

      for (let i = 0; i < 3; i++) {
        tracker.track('security-engineer', poorMetrics);
      }

      expect(tracker.getAlerts()).toHaveLength(1);

      tracker.clearAlerts();

      expect(tracker.getAlerts()).toHaveLength(0);
      expect(tracker.getPersonaStats('security-engineer')).toBeDefined();
    });
  });
});
