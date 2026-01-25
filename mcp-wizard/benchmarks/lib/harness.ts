/**
 * Benchmark Harness
 *
 * Provides reusable benchmark runner with percentile calculation.
 *
 * @module benchmarks/lib/harness
 */

export interface BenchmarkResult {
  mean: number;
  p50: number;
  p95: number;
  p99: number;
  samples: number[];
}

export interface Percentiles {
  p50: number;
  p95: number;
  p99: number;
  mean: number;
}

/**
 * Run a benchmark with N iterations and calculate percentiles
 *
 * @param name - Benchmark name for logging
 * @param fn - Async function to benchmark
 * @param iterations - Number of measurement iterations (default: 100)
 * @returns Benchmark result with percentiles
 */
export async function runBenchmark(
  name: string,
  fn: () => Promise<void>,
  iterations = 100
): Promise<BenchmarkResult> {
  console.log(`\nRunning benchmark: ${name}`);
  console.log(`Warm-up: 10 iterations...`);

  // Warm-up phase (discard results)
  for (let i = 0; i < 10; i++) {
    await fn();
  }

  console.log(`Measurement: ${iterations} iterations...`);

  // Measurement phase
  const samples: number[] = [];
  for (let i = 0; i < iterations; i++) {
    const start = performance.now();
    await fn();
    const duration = performance.now() - start;
    samples.push(duration);
  }

  const percentiles = calculatePercentiles(samples);

  console.log(`  Mean: ${percentiles.mean.toFixed(2)}ms`);
  console.log(`  p50:  ${percentiles.p50.toFixed(2)}ms`);
  console.log(`  p95:  ${percentiles.p95.toFixed(2)}ms`);
  console.log(`  p99:  ${percentiles.p99.toFixed(2)}ms`);

  return {
    ...percentiles,
    samples,
  };
}

/**
 * Calculate percentiles from array of samples
 *
 * @param samples - Array of latency samples (ms)
 * @returns Percentiles (p50, p95, p99, mean)
 */
export function calculatePercentiles(samples: number[]): Percentiles {
  const sorted = [...samples].sort((a, b) => a - b);
  const len = sorted.length;

  const mean = samples.reduce((a, b) => a + b, 0) / len;

  return {
    p50: sorted[Math.floor(len * 0.5)],
    p95: sorted[Math.floor(len * 0.95)],
    p99: sorted[Math.floor(len * 0.99)],
    mean: Math.round(mean * 100) / 100, // Round to 2 decimals
  };
}
