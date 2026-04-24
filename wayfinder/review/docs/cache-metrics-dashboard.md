# Cache Metrics Dashboard

This document provides sample GCP Cloud Monitoring queries for visualizing cache performance metrics from the multi-persona-review plugin.

## Overview

The multi-persona-review plugin tracks the following cache metrics when using prompt caching:

- **Cache Hit Rate**: Percentage of input tokens served from cache vs. uncached
- **Cache Savings**: Cost savings from using cached tokens (USD)
- **Cache Tokens Saved**: Number of tokens read from cache
- **Cache Hit Rate by Persona**: Per-persona cache efficiency

## Metric Names

All metrics use the prefix: `custom.googleapis.com/wayfinder/multi-persona-review`

| Metric | Type | Description |
|--------|------|-------------|
| `cache/hit_rate` | Gauge | Overall cache hit rate (%) |
| `cache/hit_rate_by_persona` | Gauge | Cache hit rate per persona (%) |
| `cache/tokens_saved` | Counter | Cumulative tokens read from cache |
| `cache/savings_dollars` | Counter | Cumulative cost savings (USD) |

## Dashboard Queries

### 1. Cache Hit Rate Over Time

**Query Type**: Line Chart

**MQL Query**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate'
| group_by 1m, [value_hit_rate_mean: mean(value.hit_rate)]
| every 1m
```

**Filter by Repository**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate'
| filter (metric.repository == 'my-repo')
| group_by 1m, [value_hit_rate_mean: mean(value.hit_rate)]
| every 1m
```

**Visualization**:
- Chart Type: Line
- Y-Axis: Cache Hit Rate (%)
- X-Axis: Time
- Goal Line: 80% (good cache performance threshold)

---

### 2. Cost Savings by Persona

**Query Type**: Stacked Bar Chart

**MQL Query**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/savings_dollars'
| group_by [metric.persona], [value_savings_dollars_sum: sum(value.savings_dollars)]
| filter (value_savings_dollars_sum > 0)
```

**Filter by Time Range**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/savings_dollars'
| within 7d
| group_by [metric.persona], [value_savings_dollars_sum: sum(value.savings_dollars)]
| filter (value_savings_dollars_sum > 0)
```

**Visualization**:
- Chart Type: Stacked Bar
- Y-Axis: Cost Savings (USD)
- X-Axis: Persona
- Color: By Persona

---

### 3. Cache Efficiency Trends

**Query Type**: Multi-Line Chart

**MQL Query**:
```mql
fetch global
| { metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate'
  ; metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/tokens_saved'
    | group_by 1h, [value_tokens_saved_sum: sum(value.tokens_saved)]
  }
| group_by 1h
| every 1h
```

**Visualization**:
- Chart Type: Multi-Line
- Left Y-Axis: Cache Hit Rate (%)
- Right Y-Axis: Tokens Saved (count)
- X-Axis: Time

---

### 4. Cumulative Savings Dashboard

**Query Type**: Scorecard

**MQL Query**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/savings_dollars'
| group_by [], [value_savings_dollars_sum: sum(value.savings_dollars)]
```

**Time Range Comparison**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/savings_dollars'
| within 30d
| group_by [], [value_savings_dollars_sum: sum(value.savings_dollars)]
```

**Visualization**:
- Chart Type: Scorecard
- Display: Total savings (USD)
- Comparison: Previous period

---

### 5. Per-Persona Cache Hit Rate Comparison

**Query Type**: Heatmap

**MQL Query**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate_by_persona'
| group_by [metric.persona], [value_hit_rate_mean: mean(value.hit_rate)]
| filter (value_hit_rate_mean > 0)
```

**Visualization**:
- Chart Type: Heatmap
- Rows: Persona
- Columns: Time (1h buckets)
- Color: Hit Rate (green = high, red = low)

---

### 6. Cache Performance by Repository

**Query Type**: Table

**MQL Query**:
```mql
fetch global
| { metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate'
    | group_by [metric.repository], [hit_rate: mean(value.hit_rate)]
  ; metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/savings_dollars'
    | group_by [metric.repository], [savings: sum(value.savings_dollars)]
  ; metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/tokens_saved'
    | group_by [metric.repository], [tokens: sum(value.tokens_saved)]
  }
| outer_join 0
```

**Visualization**:
- Chart Type: Table
- Columns: Repository, Hit Rate (%), Savings (USD), Tokens Saved
- Sort By: Savings (DESC)

---

## Alert Policies

### Low Cache Hit Rate Alert

Alert when cache hit rate drops below 50% for 10 minutes:

**MQL Query**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate'
| group_by 5m, [value_hit_rate_mean: mean(value.hit_rate)]
| every 5m
| condition value_hit_rate_mean < 50
```

**Alert Settings**:
- Condition: Cache hit rate < 50%
- Duration: 10 minutes
- Severity: Warning
- Notification: Email/Slack

---

### High Cache Savings Alert (Positive)

Alert when cumulative savings exceed $1.00 in a day (for celebration):

**MQL Query**:
```mql
fetch global
| metric 'custom.googleapis.com/wayfinder/multi-persona-review/cache/savings_dollars'
| within 1d
| group_by [], [value_savings_dollars_sum: sum(value.savings_dollars)]
| condition value_savings_dollars_sum > 1.0
```

**Alert Settings**:
- Condition: Daily savings > $1.00
- Severity: Info
- Notification: Slack (celebrate-channel)

---

## Sample Dashboard Layout

### Overview Dashboard

**Row 1: Key Metrics**
- Scorecard: Total Savings (Last 30 Days)
- Scorecard: Average Hit Rate (Last 30 Days)
- Scorecard: Total Tokens Saved (Last 30 Days)

**Row 2: Trends**
- Line Chart: Cache Hit Rate Over Time (7 days)
- Bar Chart: Cost Savings by Day (7 days)

**Row 3: Breakdowns**
- Bar Chart: Cost Savings by Persona
- Table: Cache Performance by Repository

**Row 4: Detailed Analysis**
- Heatmap: Per-Persona Cache Hit Rate
- Multi-Line: Cache Efficiency Trends

---

## Interpreting Metrics

### Cache Hit Rate

- **> 80%**: Excellent - Most prompts are being cached effectively
- **50-80%**: Good - Moderate cache reuse
- **< 50%**: Poor - Consider adjusting cache strategy or prompt structure

### Cost Savings

- Savings are calculated as: `(baseline_cost - cached_cost)`
- Baseline assumes all tokens are uncached at standard input pricing
- Higher savings indicate better cache utilization

### Tokens Saved

- Represents tokens read from cache instead of being processed fresh
- Each cached token costs 90% less than standard input tokens
- Monitor trends to ensure cache strategy is effective

---

## Integration with CI/CD

### Export Metrics to BigQuery

For long-term analysis and reporting:

1. Create a BigQuery dataset:
   ```bash
   bq mk --dataset my-project:code_review_metrics
   ```

2. Set up a Cloud Monitoring sink:
   ```bash
   gcloud logging sinks create code-review-cache \
     bigquery.googleapis.com/projects/my-project/datasets/code_review_metrics \
     --log-filter='resource.type="global" AND
                   metric.type=~"custom.googleapis.com/wayfinder/multi-persona-review/cache/.*"'
   ```

3. Query in BigQuery:
   ```sql
   SELECT
     timestamp,
     metric.labels.repository,
     metric.labels.persona,
     value.double_value AS hit_rate
   FROM `my-project.code_review_metrics.cache_hit_rate_*`
   WHERE DATE(timestamp) >= DATE_SUB(CURRENT_DATE(), INTERVAL 30 DAY)
   ORDER BY timestamp DESC
   ```

---

## Troubleshooting

### No Cache Metrics Appearing

**Possible Causes**:
1. Prompt caching not enabled in Anthropic API calls
2. GCP sink not configured (`costSink.type` must be `gcp`)
3. Permissions issue with GCP service account
4. Metrics take 1-2 minutes to appear after first write

**Check**:
```bash
# Verify metric descriptors exist
gcloud monitoring metric-descriptors list \
  --filter="type:custom.googleapis.com/wayfinder/multi-persona-review/cache"
```

### Metrics Not Updating

**Possible Causes**:
1. Review plugin not running
2. Network issues with GCP Cloud Monitoring API
3. Rate limiting

**Check Logs**:
```bash
# Enable debug logging in config
{
  "costSink": {
    "type": "gcp",
    "config": {
      "projectId": "my-project",
      "debug": true
    }
  }
}
```

---

## Cost Optimization Tips

Based on cache metrics analysis:

1. **High Hit Rate (> 80%)**
   - Cache is working well
   - Consider increasing cache TTL if patterns are stable

2. **Low Hit Rate (< 50%)**
   - Prompts may be too dynamic
   - Consider restructuring prompts to have more static content
   - Separate static context from dynamic parts

3. **Uneven Savings Across Personas**
   - Some personas may have longer prompts (better cache benefit)
   - Consider prompt engineering for low-performing personas

---

## Example: Creating a Complete Dashboard

Using the GCP Console:

1. Navigate to **Monitoring > Dashboards**
2. Click **Create Dashboard**
3. Add widgets using the queries above
4. Save as "Multi-Persona Review - Cache Performance"

Using Terraform:

```hcl
resource "google_monitoring_dashboard" "cache_performance" {
  dashboard_json = jsonencode({
    displayName = "Multi-Persona Review - Cache Performance"
    gridLayout = {
      widgets = [
        {
          title = "Cache Hit Rate"
          xyChart = {
            dataSets = [{
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "metric.type=\"custom.googleapis.com/wayfinder/multi-persona-review/cache/hit_rate\""
                  aggregation = {
                    alignmentPeriod = "60s"
                    perSeriesAligner = "ALIGN_MEAN"
                  }
                }
              }
            }]
          }
        }
      ]
    }
  })
}
```

---

## References

- [GCP Cloud Monitoring Documentation](https://cloud.google.com/monitoring/docs)
- [MQL Reference](https://cloud.google.com/monitoring/mql)
- [Anthropic Prompt Caching Documentation](https://docs.anthropic.com/claude/docs/prompt-caching)
- [Multi-Persona Review Plugin README](../README.md)
