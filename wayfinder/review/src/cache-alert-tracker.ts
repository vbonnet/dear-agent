/**
 * Cache Hit Rate Alert Tracker
 *
 * Monitors cache hit rates per persona and alerts when performance degrades.
 * Target: ≥80% cache hit rate per persona
 * Alert: If hit rate <50% for 3 consecutive reviews
 */

import type { CacheMetrics } from './types.js';
import { calculateCacheHitRate } from './cost-sink.js';

/**
 * Alert severity levels
 */
export type AlertSeverity = 'warning' | 'critical';

/**
 * Cache performance alert
 */
export interface CacheAlert {
  /** Persona name */
  persona: string;

  /** Alert severity */
  severity: AlertSeverity;

  /** Current cache hit rate (percentage) */
  currentHitRate: number;

  /** Target cache hit rate (percentage) */
  targetHitRate: number;

  /** Number of consecutive reviews below threshold */
  consecutiveFailures: number;

  /** Message describing the issue */
  message: string;

  /** Suggestions for remediation */
  suggestions: string[];

  /** Timestamp when alert was triggered */
  timestamp: Date;
}

/**
 * Per-persona cache performance history
 */
interface PersonaCacheHistory {
  /** Persona name */
  persona: string;

  /** Rolling window of recent hit rates (last N reviews) */
  recentHitRates: number[];

  /** Number of consecutive reviews below threshold */
  consecutiveFailures: number;

  /** Last alert timestamp (to avoid spam) */
  lastAlertTime?: Date;

  /** Total reviews tracked */
  totalReviews: number;
}

/**
 * Configuration for cache alert tracker
 */
export interface CacheAlertConfig {
  /** Target cache hit rate (default: 80%) */
  targetHitRate?: number;

  /** Alert threshold (default: 50%) */
  alertThreshold?: number;

  /** Number of consecutive failures to trigger alert (default: 3) */
  consecutiveFailuresThreshold?: number;

  /** Window size for tracking recent reviews (default: 10) */
  windowSize?: number;

  /** Minimum time between alerts for same persona (ms, default: 5 minutes) */
  alertCooldown?: number;
}

/**
 * Default configuration values
 */
const DEFAULT_CONFIG: Required<CacheAlertConfig> = {
  targetHitRate: 80,
  alertThreshold: 50,
  consecutiveFailuresThreshold: 3,
  windowSize: 10,
  alertCooldown: 5 * 60 * 1000, // 5 minutes
};

/**
 * Cache Alert Tracker
 *
 * Tracks cache performance per persona and generates alerts when
 * performance degrades below acceptable thresholds.
 */
export class CacheAlertTracker {
  private config: Required<CacheAlertConfig>;
  private history: Map<string, PersonaCacheHistory>;
  private alerts: CacheAlert[];

  constructor(config?: CacheAlertConfig) {
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.history = new Map();
    this.alerts = [];
  }

  /**
   * Track cache metrics for a persona review
   * @param persona Persona name
   * @param metrics Cache metrics from the review
   * @returns Alert if performance is degraded, undefined otherwise
   */
  track(persona: string, metrics: CacheMetrics): CacheAlert | undefined {
    const hitRate = calculateCacheHitRate(metrics);

    // Get or create history for this persona
    let personaHistory = this.history.get(persona);
    if (!personaHistory) {
      personaHistory = {
        persona,
        recentHitRates: [],
        consecutiveFailures: 0,
        totalReviews: 0,
      };
      this.history.set(persona, personaHistory);
    }

    // Update history
    personaHistory.recentHitRates.push(hitRate);
    personaHistory.totalReviews++;

    // Maintain rolling window
    if (personaHistory.recentHitRates.length > this.config.windowSize) {
      personaHistory.recentHitRates.shift();
    }

    // Track consecutive failures
    if (hitRate < this.config.alertThreshold) {
      personaHistory.consecutiveFailures++;
    } else {
      personaHistory.consecutiveFailures = 0;
    }

    // Check if we should generate an alert
    if (personaHistory.consecutiveFailures >= this.config.consecutiveFailuresThreshold) {
      // Check cooldown to avoid alert spam
      if (personaHistory.lastAlertTime) {
        const timeSinceLastAlert = Date.now() - personaHistory.lastAlertTime.getTime();
        if (timeSinceLastAlert < this.config.alertCooldown) {
          return undefined; // Still in cooldown period
        }
      }

      // Generate alert
      const alert = this.generateAlert(persona, hitRate, personaHistory.consecutiveFailures);
      this.alerts.push(alert);
      personaHistory.lastAlertTime = new Date();

      return alert;
    }

    return undefined;
  }

  /**
   * Generate an alert for degraded cache performance
   */
  private generateAlert(
    persona: string,
    currentHitRate: number,
    consecutiveFailures: number
  ): CacheAlert {
    const severity: AlertSeverity = currentHitRate < 30 ? 'critical' : 'warning';

    const suggestions: string[] = [];

    // Analyze the issue and provide suggestions
    if (currentHitRate < 10) {
      suggestions.push('Cache may not be functioning - check persona prompt stability');
      suggestions.push('Verify that persona version and prompt have not changed');
      suggestions.push('Check if cache_control breakpoints are properly configured');
    } else if (currentHitRate < 30) {
      suggestions.push('Cache hit rate is critically low - investigate prompt changes');
      suggestions.push('Consider stabilizing persona definition to improve caching');
    } else {
      suggestions.push('Cache performance is below target - review recent changes');
      suggestions.push('Ensure persona prompts are stable across reviews');
    }

    // Add general suggestions
    suggestions.push('Check if persona version has been incremented (invalidates cache)');
    suggestions.push('Review persona focusAreas and prompt for unintended modifications');

    const message = `Persona "${persona}" has cache hit rate of ${currentHitRate.toFixed(1)}% ` +
      `(target: ${this.config.targetHitRate}%, threshold: ${this.config.alertThreshold}%). ` +
      `This has occurred for ${consecutiveFailures} consecutive reviews. ` +
      `Poor cache performance increases API costs and latency.`;

    return {
      persona,
      severity,
      currentHitRate,
      targetHitRate: this.config.targetHitRate,
      consecutiveFailures,
      message,
      suggestions,
      timestamp: new Date(),
    };
  }

  /**
   * Get all alerts generated
   */
  getAlerts(): CacheAlert[] {
    return [...this.alerts];
  }

  /**
   * Get recent alerts (last N)
   */
  getRecentAlerts(count: number = 5): CacheAlert[] {
    return this.alerts.slice(-count);
  }

  /**
   * Clear all alerts
   */
  clearAlerts(): void {
    this.alerts = [];
  }

  /**
   * Get cache performance statistics for a persona
   */
  getPersonaStats(persona: string): {
    persona: string;
    averageHitRate: number;
    recentHitRate: number;
    totalReviews: number;
    consecutiveFailures: number;
  } | undefined {
    const history = this.history.get(persona);
    if (!history) {
      return undefined;
    }

    const averageHitRate = history.recentHitRates.length > 0
      ? history.recentHitRates.reduce((sum, rate) => sum + rate, 0) / history.recentHitRates.length
      : 0;

    const recentHitRate = history.recentHitRates.length > 0
      ? history.recentHitRates[history.recentHitRates.length - 1]
      : 0;

    return {
      persona: history.persona,
      averageHitRate,
      recentHitRate,
      totalReviews: history.totalReviews,
      consecutiveFailures: history.consecutiveFailures,
    };
  }

  /**
   * Get statistics for all tracked personas
   */
  getAllStats(): Map<string, ReturnType<CacheAlertTracker['getPersonaStats']>> {
    const stats = new Map();
    for (const persona of this.history.keys()) {
      const personaStats = this.getPersonaStats(persona);
      if (personaStats) {
        stats.set(persona, personaStats);
      }
    }
    return stats;
  }

  /**
   * Reset tracking for a specific persona
   */
  resetPersona(persona: string): void {
    this.history.delete(persona);
  }

  /**
   * Reset all tracking data
   */
  resetAll(): void {
    this.history.clear();
    this.alerts = [];
  }

  /**
   * Export tracking data (for persistence)
   */
  export(): {
    config: Required<CacheAlertConfig>;
    history: Record<string, PersonaCacheHistory>;
    alerts: CacheAlert[];
  } {
    const historyObj: Record<string, PersonaCacheHistory> = {};
    for (const [persona, history] of this.history.entries()) {
      historyObj[persona] = { ...history };
    }

    return {
      config: this.config,
      history: historyObj,
      alerts: [...this.alerts],
    };
  }

  /**
   * Import tracking data (for persistence)
   */
  import(data: {
    config?: Required<CacheAlertConfig>;
    history: Record<string, PersonaCacheHistory>;
    alerts: CacheAlert[];
  }): void {
    if (data.config) {
      this.config = data.config;
    }

    this.history.clear();
    for (const [persona, history] of Object.entries(data.history)) {
      this.history.set(persona, { ...history });
    }

    this.alerts = [...data.alerts];
  }
}
