#!/bin/bash
#
# Simple Performance Benchmark (no external dependencies)
#
# Pure bash implementation using GNU time for when hyperfine unavailable.
# Provides basic statistical analysis (median, mean, stddev).
#
# Usage:
#   ./scripts/perf-bench-simple.sh <command> <runs>
#

set -e

COMMAND="$1"
RUNS="${2:-10}"

if [ -z "$COMMAND" ]; then
    echo "Usage: $0 <command> [runs]"
    exit 1
fi

# Array to store timings
declare -a timings

echo "Running $RUNS iterations..." >&2

# Run benchmark
for i in $(seq 1 $RUNS); do
    # Use bash built-in time with nanosecond precision
    START=$(date +%s%N)
    eval "$COMMAND" >/dev/null 2>&1 || true
    END=$(date +%s%N)

    # Convert to milliseconds
    DURATION_NS=$((END - START))
    DURATION_MS=$(echo "scale=2; $DURATION_NS / 1000000" | bc)

    timings[$i]=$DURATION_MS
    echo "  Run $i: ${DURATION_MS}ms" >&2
done

# Sort timings for median calculation
sorted_timings=($(printf '%s\n' "${timings[@]}" | sort -n))

# Calculate statistics
sum=0
for t in "${timings[@]}"; do
    sum=$(echo "$sum + $t" | bc)
done

mean=$(echo "scale=2; $sum / $RUNS" | bc)

# Median (middle value)
mid=$((RUNS / 2))
if [ $((RUNS % 2)) -eq 0 ]; then
    # Even number of runs: average two middle values
    median=$(echo "scale=2; (${sorted_timings[$mid-1]} + ${sorted_timings[$mid]}) / 2" | bc)
else
    # Odd number: middle value
    median=${sorted_timings[$mid]}
fi

# Standard deviation
sum_sq_diff=0
for t in "${timings[@]}"; do
    diff=$(echo "$t - $mean" | bc)
    sq_diff=$(echo "$diff * $diff" | bc)
    sum_sq_diff=$(echo "$sum_sq_diff + $sq_diff" | bc)
done

variance=$(echo "scale=2; $sum_sq_diff / $RUNS" | bc)
stddev=$(echo "scale=2; sqrt($variance)" | bc)

# Min/Max
min=${sorted_timings[1]}
max=${sorted_timings[$RUNS]}

# Output JSON
cat <<EOF
{
  "median_ms": $median,
  "mean_ms": $mean,
  "stddev_ms": $stddev,
  "min_ms": $min,
  "max_ms": $max,
  "runs": $RUNS
}
EOF
