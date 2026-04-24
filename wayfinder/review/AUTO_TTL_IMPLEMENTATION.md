# Automatic Cache TTL Selection - Implementation Summary

## Overview

Implemented automatic cache strategy selection in the multi-persona-review sub-agent orchestrator to optimize costs based on expected review patterns.

## Changes Made

### 1. Core Logic (`src/sub-agent-orchestrator.ts`)

#### New Functions

- **`selectCacheTTL(expectedReviewCount: number): '5min' | '1h'`**
  - Heuristic: ≥4 reviews → aggressive caching ('1h'), <4 reviews → conservative ('5min')
  - Break-even point: ~2-3 cache hits justify the +25% cache write overhead
  - Returns cache strategy recommendation

- **`detectSessionReviewCount(): number`**
  - Detects session context to estimate expected review count
  - Detection hierarchy:
    1. `MULTI_PERSONA_REVIEW_COUNT` env var (explicit count)
    2. `MULTI_PERSONA_BATCH_MODE=true` (assumes 5 reviews)
    3. `CI=true` (assumes 4 reviews)
    4. Default: 1 review (conservative)

#### Updated Types

- **`SubAgentConfig`** interface:
  - Added `autoTtl?: boolean` - Enable/disable auto-TTL (default: true)
  - Updated `cacheTtl` documentation

#### Updated Implementation

- **`SubAgentImpl.review()`** method:
  - Integrated auto-TTL selection logic
  - Conditional caching based on strategy
  - Added verbose logging for debugging
  - Note: Anthropic API uses fixed 5-min ephemeral cache TTL

### 2. CLI Interface (`src/cli.ts`)

#### New Options

- `--cache-ttl <ttl>` - Explicit cache strategy (5min|1h), overrides auto-selection
- `--no-auto-ttl` - Disable automatic TTL selection
- `--review-count <count>` - Explicit review count for auto-TTL

#### Updated Logic

- Sets `MULTI_PERSONA_REVIEW_COUNT` env var when `--review-count` provided
- Passes `cacheTtl` and `autoTtl` options to review engine
- Enhanced verbose logging for cache strategy decisions

### 3. Review Engine (`src/review-engine.ts`)

#### Updated Interface

- **`reviewFiles()` options**:
  - Added `cacheTtl?: '5min' | '1h'`
  - Added `autoTtl?: boolean`

#### Updated Implementation

- Passes cache configuration to `SubAgentPool`
- Propagates options to sub-agent config

### 4. Public API (`src/index.ts`)

#### New Exports

- `selectCacheTTL` - Cache strategy selection function
- `detectSessionReviewCount` - Session detection function

### 5. Documentation (`README.md`)

#### New Section: "Automatic Cache Strategy Selection"

- Documented heuristic and session detection
- CLI usage examples
- Cost trade-offs table
- Environment variable guide
- Best practices

### 6. Tests

#### New Test File: `tests/unit/auto-ttl.test.ts`

- **`selectCacheTTL` tests**: 8 tests covering edge cases and thresholds
- **`detectSessionReviewCount` tests**: 17 tests for environment detection
- **Integration tests**: 8 tests for realistic scenarios
- **Cost optimization tests**: 4 tests for break-even analysis

**Total: 37 new tests**

#### Updated Test File: `tests/unit/sub-agent-orchestrator.test.ts`

- Added imports for `selectCacheTTL` and `detectSessionReviewCount`
- Added 37 comprehensive auto-TTL tests
- Added environment cleanup with `beforeEach`/`afterEach`

## Cost Trade-offs

| Strategy | Write Cost | Best For | Break-even |
|----------|-----------|----------|------------|
| Conservative (5min) | +25% | 1-3 reviews | 2-3 cache hits |
| Aggressive (1h) | +25% | 4+ reviews | 2-3 cache hits |

- **Cache write cost**: +25% overhead on first request
- **Cache read cost**: -90% savings on subsequent requests
- **Break-even**: ~2-3 cache hits justify the write overhead

## Usage Examples

### Automatic (Default)

```bash
# Uses auto-detection (checks CI, batch mode, etc.)
multi-persona-review src/

# CI environment → aggressive caching
export CI=true
multi-persona-review src/

# Batch mode → aggressive caching
export MULTI_PERSONA_BATCH_MODE=true
multi-persona-review src/
```

### Explicit Control

```bash
# Explicit review count
multi-persona-review src/ --review-count 10

# Explicit strategy
multi-persona-review src/ --cache-ttl 1h

# Disable auto-TTL
multi-persona-review src/ --no-auto-ttl --cache-ttl 5min
```

### Environment Variables

```bash
# Explicit count (highest priority)
export MULTI_PERSONA_REVIEW_COUNT=8

# Batch mode indicator
export MULTI_PERSONA_BATCH_MODE=true

# CI detection
export CI=true
```

## Technical Notes

### Anthropic API Limitation

The Anthropic API currently uses a **fixed 5-minute ephemeral cache TTL**. The '5min' and '1h' values in our implementation represent cache strategy recommendations (conservative vs aggressive), not actual API parameters.

### Future Enhancements

If Anthropic adds support for custom cache TTLs:
1. Update `SubAgentImpl.review()` to pass `ttl_seconds` in `cache_control`
2. Map '5min' → 300 seconds, '1h' → 3600 seconds
3. Update documentation to reflect actual TTL control

### Verbose Logging

Enable with `--verbose` flag to see auto-TTL decisions:

```bash
multi-persona-review src/ --verbose

# Output:
# [VERBOSE]  - Cache TTL: auto-selected (based on session context)
# [SUB-AGENT] Auto-TTL: 4 reviews expected → 1h recommended
```

## Testing

Run auto-TTL tests:

```bash
npm test -- tests/unit/auto-ttl.test.ts
npm test -- tests/unit/sub-agent-orchestrator.test.ts
```

Run all tests:

```bash
npm test
```

## Success Criteria

✅ Auto-TTL selection heuristic implemented
✅ Session detection logic with environment variables
✅ `--auto-ttl`, `--cache-ttl`, `--review-count` CLI flags
✅ Cost trade-offs documented in help text and README
✅ Comprehensive test coverage (37 new tests)
✅ Backward compatible (auto-TTL enabled by default)
✅ No breaking changes to existing API

## Files Modified

1. `engram/plugins/multi-persona-review/src/sub-agent-orchestrator.ts`
2. `engram/plugins/multi-persona-review/src/cli.ts`
3. `engram/plugins/multi-persona-review/src/review-engine.ts`
4. `engram/plugins/multi-persona-review/src/index.ts`
5. `engram/plugins/multi-persona-review/README.md`
6. `engram/plugins/multi-persona-review/tests/unit/sub-agent-orchestrator.test.ts`

## Files Created

1. `engram/plugins/multi-persona-review/tests/unit/auto-ttl.test.ts`
2. `engram/plugins/multi-persona-review/AUTO_TTL_IMPLEMENTATION.md` (this file)
