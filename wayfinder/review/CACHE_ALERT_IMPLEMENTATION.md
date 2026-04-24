# Cache Hit Rate Alerts Implementation

**Feature**: Cache Hit Rate Monitoring and Alerts
**Bead**: oss-slwl (Task 4.2)
**Status**: ✅ Complete
**Date**: 2026-02-23

---

## Overview

This implementation adds automatic cache hit rate monitoring and alerting to the multi-persona-review plugin. The system tracks cache performance per persona across multiple reviews and generates alerts when performance degrades below acceptable thresholds.

## Requirements Met

✅ Monitor cache hit rate per persona (target ≥80%)
✅ Alert if hit rate <50% for 3 consecutive reviews
✅ Suggest persona prompt stability issues or version changes
✅ Integrate with GCP Cloud Monitoring (if enabled)
✅ Tests pass (19 new tests, all passing)
✅ Documentation updated

## Architecture

### Core Components

1. **CacheAlertTracker** (`src/cache-alert-tracker.ts`)
   - Tracks cache performance history per persona
   - Maintains rolling window of recent reviews (configurable, default: 10)
   - Detects consecutive failures (threshold: 3 by default)
   - Generates alerts with appropriate severity levels
   - Includes alert cooldown to prevent spam (default: 5 minutes)

2. **Cost Sink Integration** (`src/cost-sink.ts`)
   - Updated `CostSink` interface with `getCacheAlerts()` method
   - `StdoutCostSink`: Tracks alerts, logs to stderr with `[CACHE_ALERT]` prefix
   - `FileCostSink`: Tracks alerts, writes to JSONL file with type marker
   - All sinks lazy-load `CacheAlertTracker` to avoid initialization overhead

3. **GCP Cloud Monitoring** (`src/cost-sinks/gcp-sink.ts`)
   - Sends cache alert events as metrics
   - Metric: `cache/alert` with labels for severity and persona
   - Value: consecutive failure count
   - Debug logging for alert events

4. **CLI Integration** (`src/cli.ts`)
   - New flag: `--show-cache-metrics`
   - Displays formatted alerts after review completion
   - Integrates with existing cost sink architecture

5. **Text Formatter** (`src/formatters/text-formatter.ts`)
   - New function: `formatCacheAlerts()`
   - Color-coded severity indicators (yellow/red)
   - Detailed suggestions for remediation
   - Human-readable output format

## Alert Levels

| Hit Rate | Severity | Action |
|----------|----------|--------|
| ≥80% | None | Optimal performance |
| 50-79% | None | Acceptable performance |
| 30-49% | WARNING | Investigate recent changes |
| <30% | CRITICAL | Immediate action needed |
| <10% | CRITICAL | Cache may not be functioning |

## Alert Message Structure

```typescript
interface CacheAlert {
  persona: string;              // Persona name
  severity: 'warning' | 'critical';
  currentHitRate: number;       // Current hit rate (percentage)
  targetHitRate: number;        // Target (default: 80%)
  consecutiveFailures: number;  // Number of consecutive failures
  message: string;              // Detailed description
  suggestions: string[];        // Remediation steps
  timestamp: Date;              // When alert was generated
}
```

## Configuration

Default configuration (customizable):

```typescript
{
  targetHitRate: 80,              // Target cache hit rate (%)
  alertThreshold: 50,             // Alert when below this (%)
  consecutiveFailuresThreshold: 3,// Alert after N failures
  windowSize: 10,                 // Rolling window size
  alertCooldown: 300000,          // 5 minutes between alerts
}
```

## Usage Examples

### CLI Usage

```bash
# Basic review with cache alerts
multi-persona-review src/ --show-cache-metrics

# With GCP cost tracking
multi-persona-review src/ \
  --cost-sink gcp \
  --gcp-project my-project \
  --show-cache-metrics
```

### Programmatic Usage

```typescript
import { CacheAlertTracker } from '@wayfinder/multi-persona-review';

const tracker = new CacheAlertTracker({
  targetHitRate: 90,  // More strict threshold
  alertThreshold: 60,
  consecutiveFailuresThreshold: 2,
});

// Track cache metrics
const alert = tracker.track('security-engineer', metrics);

if (alert) {
  console.log(`Alert: ${alert.message}`);
  console.log(`Suggestions: ${alert.suggestions.join(', ')}`);
}

// Get statistics
const stats = tracker.getPersonaStats('security-engineer');
console.log(`Average hit rate: ${stats.averageHitRate}%`);
```

## GCP Cloud Monitoring

When GCP cost sink is configured, alerts are automatically sent to Cloud Monitoring:

### Metrics Created

1. **cache/alert** (int64, gauge)
   - Labels: `persona`, `severity`, plus standard labels (repository, branch, etc.)
   - Value: Number of consecutive failures
   - Use for alerting policies

2. **cache/hit_rate_by_persona** (double, gauge)
   - Labels: `persona`, plus standard labels
   - Value: Cache hit rate percentage (0-100)
   - Use for performance monitoring

### Example Alert Policy (gcloud)

```bash
gcloud alpha monitoring policies create \
  --notification-channels=CHANNEL_ID \
  --display-name="Multi-Persona Cache Performance Alert" \
  --condition-display-name="Cache hit rate too low" \
  --condition-threshold-value=3 \
  --condition-threshold-duration=0s \
  --condition-threshold-comparison=COMPARISON_GT \
  --condition-threshold-filter='metric.type="custom.googleapis.com/wayfinder/multi-persona-review/cache/alert"'
```

## Files Modified

### New Files
- `src/cache-alert-tracker.ts` - Core alert tracking logic
- `tests/unit/cache-alert-tracker.test.ts` - Comprehensive test suite (19 tests)
- `CACHE_ALERT_IMPLEMENTATION.md` - This document

### Modified Files
- `src/cost-sink.ts` - Added `getCacheAlerts()` method, integrated tracker
- `src/cost-sinks/gcp-sink.ts` - Added GCP metric publishing for alerts
- `src/cli.ts` - Added `--show-cache-metrics` flag and display logic
- `src/formatters/text-formatter.ts` - Added `formatCacheAlerts()` function
- `src/index.ts` - Exported new types and classes
- `tests/unit/cost-sink.test.ts` - Added tests for alert tracking
- `DOCUMENTATION.md` - Added Cache Hit Rate Alerts section

## Test Coverage

**New Tests**: 19 tests in `cache-alert-tracker.test.ts`
**Updated Tests**: 2 additional tests in `cost-sink.test.ts`
**Total**: 21 new tests, all passing

### Test Categories

1. **Basic Tracking** (2 tests)
   - Track cache performance
   - No alerts for good performance

2. **Alert Generation** (4 tests)
   - Alert after 3 consecutive failures
   - Critical severity for very low hit rates
   - Reset on good performance
   - Appropriate suggestions

3. **Alert Cooldown** (1 test)
   - Respect cooldown period

4. **Multi-Persona** (2 tests)
   - Independent tracking per persona
   - Stats for all personas

5. **Configuration** (3 tests)
   - Custom target hit rate
   - Custom failure threshold
   - Custom window size

6. **Statistics** (2 tests)
   - Average hit rate calculation
   - Recent alerts tracking

7. **Persistence** (2 tests)
   - Export tracking data
   - Import tracking data

8. **Reset** (3 tests)
   - Reset specific persona
   - Reset all tracking
   - Clear alerts only

## Performance Considerations

1. **Lazy Loading**: Alert tracker is only initialized when first needed
2. **Memory Efficient**: Rolling window limits memory growth (configurable)
3. **Minimal Overhead**: Alert checking adds ~1ms per persona per review
4. **Cooldown**: Prevents alert spam (configurable interval)

## Remediation Guide

When alerts are triggered:

### 1. Investigate Recent Changes
```bash
# Check persona file history
git log -p core/persona/library/security-engineer.ai.md
```

### 2. Verify Persona Stability
- Ensure prompts don't contain dynamic content (dates, random values)
- Check that `focusAreas` haven't changed unexpectedly
- Verify `version` field is only incremented when necessary

### 3. Review Cache Configuration
```typescript
// Check cache metadata
console.log(persona.cacheMetadata);

// Expected:
// {
//   cacheEligible: true,
//   tokenCount: 2500,
//   cacheKey: "persona:security-engineer:v1.0.0:abc123"
// }
```

### 4. Monitor Metrics
```bash
# View cache performance in GCP
gcloud monitoring time-series list \
  --filter='metric.type="custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate_by_persona"' \
  --format=json
```

## Success Criteria ✅

All requirements met:

1. ✅ Cache hit rate monitored per persona with ≥80% target
2. ✅ Alerts triggered when hit rate <50% for 3 consecutive reviews
3. ✅ Suggestions provided for prompt stability and version issues
4. ✅ GCP Cloud Monitoring integration complete
5. ✅ CLI displays alerts when `--show-cache-metrics` is used
6. ✅ All tests passing (267 total, 21 new)
7. ✅ Documentation updated with comprehensive guide

## Future Enhancements

Potential improvements for future iterations:

1. **Configurable Alert Channels**
   - Slack integration
   - Email notifications
   - Webhook support

2. **Advanced Analytics**
   - Hit rate trends over time
   - Cost impact analysis
   - Persona comparison reports

3. **Auto-Remediation**
   - Automatic persona rollback on cache failure
   - Suggested prompt fixes
   - Version management recommendations

4. **Dashboard**
   - Real-time cache performance visualization
   - Historical trend analysis
   - Per-team/per-repository breakdowns

---

**Implementation Complete** ✅
All tests passing, documentation updated, feature ready for production use.
