/**
 * GCP Cloud Monitoring cost sink
 * Sends cost metrics to Google Cloud Monitoring (Stackdriver)
 */

import type { CostInfo } from '../types.js';
import type { CostSink, CostMetadata } from '../cost-sink.js';
import type { CacheAlert } from '../cache-alert-tracker.js';
import { CostSinkError, COST_SINK_ERROR_CODES } from '../cost-sink.js';
import {
  aggregateCacheMetrics,
  extractCacheMetrics,
  calculateCacheHitRate,
  calculateCostSavings,
} from '../cost-sink.js';

/**
 * GCP Cost Sink configuration
 */
export interface GCPCostSinkConfig {
  /** GCP project ID */
  projectId: string;

  /** Metric name prefix (default: custom.googleapis.com/wayfinder/multi-persona-review) */
  metricPrefix?: string;

  /** Service account key file path (optional, uses ADC if not provided) */
  keyFilePath?: string;

  /** Additional labels to attach to all metrics */
  labels?: Record<string, string>;

  /** Enable debug logging */
  debug?: boolean;
}

/**
 * GCP Cloud Monitoring cost sink
 */
export class GCPCostSink implements CostSink {
  private config: GCPCostSinkConfig;
  private metricClient: any; // MetricServiceClient from @google-cloud/monitoring
  private projectPath: string;
  private metricPrefix: string;
  private alertTracker?: any; // CacheAlertTracker - lazy loaded

  constructor(config: Record<string, unknown>) {
    // Validate configuration
    if (!config.projectId || typeof config.projectId !== 'string') {
      throw new CostSinkError(
        COST_SINK_ERROR_CODES.INVALID_CONFIG,
        'GCPCostSink requires projectId in config'
      );
    }

    this.config = {
      projectId: config.projectId,
      metricPrefix: (config.metricPrefix as string) || 'custom.googleapis.com/wayfinder/multi-persona-review',
      keyFilePath: config.keyFilePath as string,
      labels: (config.labels as Record<string, string>) || {},
      debug: (config.debug as boolean) || false,
    };

    this.metricPrefix = this.config.metricPrefix!;
    this.projectPath = `projects/${this.config.projectId}`;
  }

  /**
   * Initialize the metric client (lazy loading)
   */
  private async getMetricClient() {
    if (!this.metricClient) {
      try {
        // Lazy load the @google-cloud/monitoring package using standard dynamic import
        const gcpMonitoring = await import('@google-cloud/monitoring').catch((_error: Error) => {
          throw new Error('@google-cloud/monitoring package not found. Install it with: npm install @google-cloud/monitoring');
        });

        const MetricServiceClient = gcpMonitoring.MetricServiceClient;

        // Create client with optional key file
        const clientConfig: any = {};
        if (this.config.keyFilePath) {
          clientConfig.keyFilename = this.config.keyFilePath;
        }

        this.metricClient = new MetricServiceClient(clientConfig);

        if (this.config.debug) {
          console.error('[GCP_COST_SINK] Initialized metric client for project:', this.config.projectId);
        }
      } catch (error) {
        throw new CostSinkError(
          COST_SINK_ERROR_CODES.AUTHENTICATION_FAILED,
          `Failed to initialize GCP metric client: ${error instanceof Error ? error.message : String(error)}`,
          error
        );
      }
    }
    return this.metricClient;
  }

  /**
   * Record cost information to GCP Cloud Monitoring
   */
  async record(cost: CostInfo, metadata?: CostMetadata): Promise<void> {
    try {
      const client = await this.getMetricClient();
      const now = new Date();

      // Prepare common labels
      const labels: Record<string, string> = {
        ...this.config.labels,
      };

      if (metadata?.repository) {
        labels.repository = metadata.repository;
      }
      if (metadata?.branch) {
        labels.branch = metadata.branch;
      }
      if (metadata?.mode) {
        labels.mode = metadata.mode;
      }
      if (metadata?.pullRequest) {
        labels.pull_request = metadata.pullRequest.toString();
      }

      // Create time series for total cost
      const totalCostTimeSeries = {
        metric: {
          type: `${this.metricPrefix}/cost/total`,
          labels,
        },
        resource: {
          type: 'global',
          labels: {
            project_id: this.config.projectId,
          },
        },
        points: [{
          interval: {
            endTime: {
              seconds: Math.floor(now.getTime() / 1000),
            },
          },
          value: {
            doubleValue: cost.totalCost,
          },
        }],
      };

      // Create time series for total tokens
      const totalTokensTimeSeries = {
        metric: {
          type: `${this.metricPrefix}/tokens/total`,
          labels,
        },
        resource: {
          type: 'global',
          labels: {
            project_id: this.config.projectId,
          },
        },
        points: [{
          interval: {
            endTime: {
              seconds: Math.floor(now.getTime() / 1000),
            },
          },
          value: {
            int64Value: cost.totalTokens,
          },
        }],
      };

      // Create time series for each persona
      const personaTimeSeries: any[] = [];
      for (const [personaName, personaCost] of Object.entries(cost.byPersona)) {
        const personaLabels = {
          ...labels,
          persona: personaName,
        };

        personaTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/cost/by_persona`,
            labels: personaLabels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              doubleValue: personaCost.cost,
            },
          }],
        });

        personaTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/tokens/input_by_persona`,
            labels: personaLabels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              int64Value: personaCost.inputTokens,
            },
          }],
        });

        personaTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/tokens/output_by_persona`,
            labels: personaLabels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              int64Value: personaCost.outputTokens,
            },
          }],
        });
      }

      // Record findings count if provided
      const findingsTimeSeries: any[] = [];
      if (metadata?.totalFindings !== undefined) {
        findingsTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/findings/total`,
            labels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              int64Value: metadata.totalFindings,
            },
          }],
        });
      }

      // Record files reviewed if provided
      if (metadata?.filesReviewed !== undefined) {
        findingsTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/files/reviewed`,
            labels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              int64Value: metadata.filesReviewed,
            },
          }],
        });
      }

      // Calculate and record cache metrics
      const cacheTimeSeries: any[] = [];
      const aggregatedCacheMetrics = aggregateCacheMetrics(cost);

      // Overall cache hit rate
      const overallHitRate = calculateCacheHitRate(aggregatedCacheMetrics);
      if (overallHitRate > 0) {
        cacheTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/cache/hit_rate`,
            labels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              doubleValue: overallHitRate,
            },
          }],
        });
      }

      // Cache tokens saved
      const tokensSaved = aggregatedCacheMetrics.cacheReadTokens;
      if (tokensSaved > 0) {
        cacheTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/cache/tokens_saved`,
            labels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              int64Value: tokensSaved,
            },
          }],
        });
      }

      // Cost savings
      const costSavings = calculateCostSavings(aggregatedCacheMetrics);
      if (costSavings.savings > 0) {
        cacheTimeSeries.push({
          metric: {
            type: `${this.metricPrefix}/cache/savings_dollars`,
            labels,
          },
          resource: {
            type: 'global',
            labels: {
              project_id: this.config.projectId,
            },
          },
          points: [{
            interval: {
              endTime: {
                seconds: Math.floor(now.getTime() / 1000),
              },
            },
            value: {
              doubleValue: costSavings.savings,
            },
          }],
        });
      }

      // Per-persona cache metrics and alert tracking
      const alerts: CacheAlert[] = [];

      // Lazy load alert tracker
      if (!this.alertTracker) {
        const { CacheAlertTracker } = await import('../cache-alert-tracker.js');
        this.alertTracker = new CacheAlertTracker();
      }

      for (const [personaName, personaCost] of Object.entries(cost.byPersona)) {
        const personaCacheMetrics = extractCacheMetrics(personaCost);
        const personaLabels = {
          ...labels,
          persona: personaName,
        };

        // Persona cache hit rate
        const personaHitRate = calculateCacheHitRate(personaCacheMetrics);
        if (personaHitRate > 0) {
          cacheTimeSeries.push({
            metric: {
              type: `${this.metricPrefix}/cache/hit_rate_by_persona`,
              labels: personaLabels,
            },
            resource: {
              type: 'global',
              labels: {
                project_id: this.config.projectId,
              },
            },
            points: [{
              interval: {
                endTime: {
                  seconds: Math.floor(now.getTime() / 1000),
                },
              },
              value: {
                doubleValue: personaHitRate,
              },
            }],
          });
        }

        // Track cache alerts
        const alert = this.alertTracker.track(personaName, personaCacheMetrics);
        if (alert) {
          alerts.push(alert);

          // Send alert as a metric event
          cacheTimeSeries.push({
            metric: {
              type: `${this.metricPrefix}/cache/alert`,
              labels: {
                ...personaLabels,
                severity: alert.severity,
              },
            },
            resource: {
              type: 'global',
              labels: {
                project_id: this.config.projectId,
              },
            },
            points: [{
              interval: {
                endTime: {
                  seconds: Math.floor(now.getTime() / 1000),
                },
              },
              value: {
                int64Value: alert.consecutiveFailures,
              },
            }],
          });

          if (this.config.debug) {
            console.error('[GCP_COST_SINK] Cache alert:', {
              persona: alert.persona,
              severity: alert.severity,
              hitRate: alert.currentHitRate.toFixed(1) + '%',
              consecutiveFailures: alert.consecutiveFailures,
            });
          }
        }
      }

      // Send all time series in a single request
      const request = {
        name: this.projectPath,
        timeSeries: [
          totalCostTimeSeries,
          totalTokensTimeSeries,
          ...personaTimeSeries,
          ...findingsTimeSeries,
          ...cacheTimeSeries,
        ],
      };

      await client.createTimeSeries(request);

      if (this.config.debug) {
        console.error(
          '[GCP_COST_SINK] Recorded cost:',
          `$${cost.totalCost.toFixed(4)}`,
          `(${cost.totalTokens} tokens)`,
          `to project ${this.config.projectId}`
        );
      }
    } catch (error) {
      throw new CostSinkError(
        COST_SINK_ERROR_CODES.RECORD_FAILED,
        `Failed to record cost to GCP: ${error instanceof Error ? error.message : String(error)}`,
        error
      );
    }
  }

  /**
   * Get cache alerts
   */
  getCacheAlerts(): CacheAlert[] {
    return this.alertTracker?.getAlerts() || [];
  }

  /**
   * Close the metric client
   */
  async close(): Promise<void> {
    if (this.metricClient) {
      await this.metricClient.close();
      if (this.config.debug) {
        console.error('[GCP_COST_SINK] Closed metric client');
      }
    }
  }
}
