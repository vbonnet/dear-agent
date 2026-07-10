// Package promptcache provides cache policy for supported model families and
// model-neutral prompt cache break detection.
//
// Anthropic explicit cache-control supports two tiers:
//   - Default: ephemeral (5-minute TTL, API default)
//   - Persistent: 1-hour TTL for stable system prompts
//
// Other supported families retain provider-default cache behavior. Cache break
// detection records prompt snapshots before API calls and checks if cache read
// ratios drop below expected thresholds, indicating a break.
package promptcache
