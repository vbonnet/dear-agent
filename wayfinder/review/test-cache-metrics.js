#!/usr/bin/env node
/**
 * Test script to verify cache metrics functionality
 */

import {
  calculateCacheHitRate,
  calculateCostSavings,
  extractCacheMetrics,
  aggregateCacheMetrics,
} from './dist/cost-sink.js';

// Test data
const personaCost1 = {
  persona: 'security-engineer',
  cost: 0.015,
  inputTokens: 1000,
  outputTokens: 500,
  cacheCreationInputTokens: 2000,
  cacheReadInputTokens: 7000,
};

const personaCost2 = {
  persona: 'performance-engineer',
  cost: 0.012,
  inputTokens: 800,
  outputTokens: 400,
  cacheCreationInputTokens: 0,
  cacheReadInputTokens: 5000,
};

const costInfo = {
  totalCost: 0.027,
  totalTokens: 15700,
  byPersona: {
    'security-engineer': personaCost1,
    'performance-engineer': personaCost2,
  },
};

console.log('=== Cache Metrics Test ===\n');

// Test extractCacheMetrics
console.log('1. Extract Cache Metrics:');
const metrics1 = extractCacheMetrics(personaCost1);
console.log('  Security Engineer:', JSON.stringify(metrics1, null, 2));

// Test aggregateCacheMetrics
console.log('\n2. Aggregate Cache Metrics:');
const aggregated = aggregateCacheMetrics(costInfo);
console.log('  Aggregated:', JSON.stringify(aggregated, null, 2));

// Test calculateCacheHitRate
console.log('\n3. Calculate Cache Hit Rate:');
const hitRate = calculateCacheHitRate(aggregated);
console.log(`  Hit Rate: ${hitRate.toFixed(1)}%`);
console.log(`  (${aggregated.cacheReadTokens} cache hits, ${aggregated.inputTokens} misses)`);

// Test calculateCostSavings
console.log('\n4. Calculate Cost Savings:');
const savings = calculateCostSavings(aggregated);
console.log(`  Baseline Cost: $${savings.baselineCost.toFixed(4)}`);
console.log(`  Cached Cost: $${savings.cachedCost.toFixed(4)}`);
console.log(`  Savings: $${savings.savings.toFixed(4)} (${savings.savingsPercent.toFixed(1)}% reduction)`);

console.log('\n=== Test Complete ===');
