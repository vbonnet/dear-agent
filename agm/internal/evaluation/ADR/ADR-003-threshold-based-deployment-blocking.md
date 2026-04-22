# ADR-003: Use Threshold-Based Deployment Blocking

## Status

Accepted

## Context

LLM-based applications require quality gates to prevent regressions from reaching production. Manual review of evaluation results doesn't scale and introduces delays.

### The Problem

Without automated quality gates:
1. **Human Error**: Manual reviews miss subtle quality degradations
2. **Scalability**: Can't review every change as team/deployment frequency grows
3. **Delays**: Manual reviews slow down deployment pipeline
4. **Inconsistency**: Different reviewers apply different standards
5. **Regression Risk**: Quality can degrade gradually without detection

### Existing Approaches

**Option 1: Manual Review**
- Pros: Flexible, context-aware
- Cons: Slow, doesn't scale, inconsistent

**Option 2: Binary Pass/Fail**
- Pros: Simple, fast
- Cons: Too rigid, no nuance

**Option 3: Threshold-Based**
- Pros: Automated, configurable, provides nuance
- Cons: Requires tuning, false positives possible

## Decision

We will implement **threshold-based deployment blocking** in the offline evaluation system:

### Core Components

1. **Configurable Thresholds**
   - `MinPassRate`: Minimum percentage of test cases that must pass (e.g., 95%)
   - `MinAvgScore`: Minimum average score across all test cases (e.g., 0.85)
   - `MaxFailedCases`: Maximum number of test cases that can fail (e.g., 5)

2. **Blocking Mechanism**
   - Evaluate all test cases against golden dataset
   - Calculate aggregate metrics (pass rate, average score, failed count)
   - Compare metrics to configured thresholds
   - Block deployment if ANY threshold violated

3. **Configuration Control**
   - `BlockOnFailure`: Boolean to enable/disable blocking (default: true)
   - Allows temporary bypass for emergencies or initial setup

### Implementation

```go
type OfflineConfig struct {
    MinPassRate     float64 // e.g., 0.95 = 95%
    MinAvgScore     float64 // e.g., 0.85
    MaxFailedCases  int     // e.g., 5
    BlockOnFailure  bool    // default: true
}

func (r *OfflineReport) ShouldBlockDeployment(config *OfflineConfig) bool {
    if !config.BlockOnFailure {
        return false
    }

    if r.PassRate < config.MinPassRate {
        r.BlockReason = fmt.Sprintf("Pass rate %.2f%% below threshold %.2f%%",
            r.PassRate*100, config.MinPassRate*100)
        return true
    }

    if r.AverageScore < config.MinAvgScore {
        r.BlockReason = fmt.Sprintf("Average score %.2f below threshold %.2f",
            r.AverageScore, config.MinAvgScore)
        return true
    }

    if r.FailedCases > config.MaxFailedCases {
        r.BlockReason = fmt.Sprintf("%d failed cases exceeds maximum %d",
            r.FailedCases, config.MaxFailedCases)
        return true
    }

    return false
}
```

## Consequences

### Positive

1. **Automated Quality Gates**: No manual intervention needed
2. **Regression Prevention**: Catches quality degradation early
3. **Fast Feedback**: Developers know immediately if changes break quality
4. **Consistent Standards**: Same criteria applied to all changes
5. **Confidence**: Can deploy knowing quality thresholds met
6. **Flexible**: Thresholds can be adjusted based on experience

### Negative

1. **False Positives**: Legitimate changes may be blocked due to test variance
2. **Threshold Tuning**: Requires experimentation to find right thresholds
3. **Brittleness**: Small threshold changes can dramatically affect pass rate
4. **Test Quality Dependency**: Only as good as the golden dataset
5. **Emergency Bypass**: May need override mechanism for critical fixes

### Neutral

1. **Initial Setup**: Requires collecting baseline data
2. **Maintenance**: Thresholds may need periodic adjustment
3. **Monitoring**: Should track false positive rate

## Usage Examples

### Example 1: Strict Production System

```go
config := &evaluation.OfflineConfig{
    MinPassRate:    0.98,  // 98% must pass
    MinAvgScore:    0.90,  // Average >= 0.90
    MaxFailedCases: 2,     // Allow max 2 failures
    BlockOnFailure: true,
}
```

### Example 2: Development Environment

```go
config := &evaluation.OfflineConfig{
    MinPassRate:    0.85,  // 85% must pass
    MinAvgScore:    0.75,  // Average >= 0.75
    MaxFailedCases: 10,    // Allow max 10 failures
    BlockOnFailure: true,
}
```

### Example 3: Initial Setup (Monitoring Only)

```go
config := &evaluation.OfflineConfig{
    MinPassRate:    0.90,
    MinAvgScore:    0.80,
    MaxFailedCases: 5,
    BlockOnFailure: false, // Don't block yet, just monitor
}
```

## Threshold Selection Guidelines

### Starting Point

1. **Establish Baseline**
   - Run evaluation on current production version
   - Measure: pass rate, average score, failure count
   - Use baseline - 5% as initial threshold

2. **Conservative Start**
   - Begin with lower thresholds (e.g., 85% pass rate)
   - Monitor false positive rate
   - Gradually increase as confidence grows

3. **Monitor and Adjust**
   - Track blocked deployments (true vs false positives)
   - Adjust thresholds quarterly based on data
   - Document threshold changes

### Recommended Thresholds by System Type

| System Type | MinPassRate | MinAvgScore | MaxFailedCases |
|-------------|-------------|-------------|----------------|
| Critical (financial, medical) | 0.98 | 0.90 | 2 |
| Production (user-facing) | 0.95 | 0.85 | 5 |
| Staging | 0.90 | 0.80 | 10 |
| Development | 0.85 | 0.75 | 15 |
| Experimental | N/A | N/A | N/A (BlockOnFailure: false) |

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Quality Gate

on: [pull_request]

jobs:
  evaluate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Run Offline Evaluation
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
        run: |
          go run ./cmd/evaluate \
            --golden-dir ./testdata/golden \
            --min-pass-rate 0.95 \
            --min-avg-score 0.85 \
            --max-failed-cases 5 \
            --block-on-failure

      - name: Upload Report
        if: always()
        uses: actions/upload-artifact@v2
        with:
          name: evaluation-report
          path: report.html
```

**Exit Codes:**
- `0`: All thresholds met, deployment approved
- `1`: Thresholds violated, deployment blocked
- `2`: Evaluation error (API failure, etc.)

### GitLab CI Example

```yaml
evaluate:
  stage: quality-gate
  script:
    - ./evaluate --golden-dir testdata/golden
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  artifacts:
    when: always
    paths:
      - report.html
```

## Handling False Positives

### Investigation Process

When deployment blocked:

1. **Review Report**
   - Check which test cases failed
   - Look for patterns (specific category, edge cases)
   - Review reasoning from judge

2. **Verify Legitimacy**
   - Are failures due to code changes (true positive)?
   - Are failures due to test variance (false positive)?
   - Are failures due to outdated golden dataset?

3. **Take Action**
   - **True Positive**: Fix code, re-run evaluation
   - **False Positive**: Update golden dataset or adjust threshold
   - **Flaky Test**: Investigate and fix test stability

### Emergency Override

For critical production fixes:

```go
config := &evaluation.OfflineConfig{
    MinPassRate:    0.95,
    MinAvgScore:    0.85,
    MaxFailedCases: 5,
    BlockOnFailure: false, // Temporarily disable blocking
}
```

**Process:**
1. Set `BlockOnFailure: false`
2. Deploy critical fix
3. Re-enable blocking: `BlockOnFailure: true`
4. Fix evaluation failures in follow-up PR

## Monitoring and Metrics

### Key Metrics to Track

1. **Block Rate**: % of deployments blocked
2. **False Positive Rate**: % of blocks that were incorrect
3. **True Positive Rate**: % of blocks that caught real issues
4. **Average Scores Over Time**: Trend of quality metrics

### Alerting

Set up alerts for:
- Block rate > 20% (may indicate threshold too strict)
- Block rate < 1% (may indicate threshold too loose)
- Average score declining trend
- Sudden changes in pass rate

## Alternatives Considered

### Alternative 1: Fixed Pass/Fail (No Thresholds)

Every test must pass, no tolerance.

**Rejected Reason:**
- Too strict, blocks all deployments
- No tolerance for test variance
- Doesn't account for test quality differences

### Alternative 2: Manual Approval

Human reviews evaluation results and approves/rejects.

**Rejected Reason:**
- Doesn't scale
- Introduces delays
- Inconsistent standards

### Alternative 3: Statistical Significance Testing

Compare current results to historical baseline using t-tests.

**Rejected Reason:**
- Too complex for initial implementation
- Requires significant historical data
- Harder for developers to understand
- May implement in future as enhancement

### Alternative 4: Weighted Thresholds

Different thresholds for different test categories.

**Rejected Reason:**
- More complex to configure
- Harder to understand blocking decision
- Can implement as future enhancement

## Future Enhancements

### Planned Improvements

1. **Adaptive Thresholds**
   - Automatically adjust based on historical data
   - Use statistical process control
   - Detect unusual patterns

2. **Category-Specific Thresholds**
   - Different thresholds for different test categories
   - E.g., stricter for safety tests

3. **Confidence Intervals**
   - Report confidence in blocking decision
   - Account for test variance

4. **Progressive Rollback**
   - Integration with canary deployments
   - Automatic rollback if production metrics degrade

## References

- Continuous Integration best practices
- Google's Testing Blog: Quality Gates
- Statistical Process Control in Software
- Implementation code: `offline_evaluator.go`

## Revision History

- 2024-10-01: Initial decision
- 2024-11-15: Added examples and guidelines
- 2025-01-10: Added CI/CD integration examples
- 2026-02-20: Added to EDD Framework documentation
