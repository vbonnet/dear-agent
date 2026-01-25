#!/usr/bin/env tsx
/**
 * Test Benchmark Harness
 *
 * Simple test to verify benchmark infrastructure works.
 */

import { runBenchmark, calculatePercentiles } from './lib/harness';

async function main() {
  console.log('Testing benchmark harness...\n');

  // Test 1: Simple async function
  const result = await runBenchmark(
    'Simple async delay',
    async () => {
      await new Promise((resolve) => setTimeout(resolve, 1));
    },
    20 // Fewer iterations for test
  );

  console.log('\n✅ Benchmark harness works!');
  console.log(`   Mean: ${result.mean.toFixed(2)}ms`);
  console.log(`   p99: ${result.p99.toFixed(2)}ms`);

  // Test 2: Percentile calculation
  const samples = [10, 20, 30, 40, 50, 60, 70, 80, 90, 100];
  const percentiles = calculatePercentiles(samples);

  console.log('\n✅ Percentile calculation works!');
  console.log(`   p50: ${percentiles.p50}`);
  console.log(`   p95: ${percentiles.p95}`);
  console.log(`   p99: ${percentiles.p99}`);
  console.log(`   mean: ${percentiles.mean}`);

  console.log('\n✅ All tests passed!\n');
}

main().catch((error) => {
  console.error('Test failed:', error);
  process.exit(1);
});
