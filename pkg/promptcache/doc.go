// Package promptcache provides Claude API cache control headers for system
// prompts and cache break detection.
//
// Two-tier caching:
//   - Default: ephemeral (5-minute TTL, API default)
//   - Persistent: 1-hour TTL for stable system prompts
//
// Cache break detection records prompt snapshots before API calls and checks
// if cache read ratios drop below expected thresholds, indicating a break.
package promptcache
