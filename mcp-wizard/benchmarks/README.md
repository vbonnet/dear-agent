# MCP Performance Benchmarks

Performance benchmarking suite for [REDACTED_EMPLOYER]-mcp lazy loading implementation.

## Overview

This benchmark suite measures:
- **Context Usage**: Token count reduction (eager vs lazy loading)
- **Latency**: p95/p99 for schema filtering and intent analysis
- **Startup Time**: Session initialization (gateway vs direct mode)
- **Memory Overhead**: RSS and heap usage (gateway vs direct mode)

## Quick Start

### Prerequisites

```bash
# Install dependencies
npm install

# Install tiktoken for accurate token counting (optional)
npm install tiktoken

# Install tsx for running TypeScript benchmarks
npm install -g tsx
```

### Run All Benchmarks

```bash
./benchmarks/run-all.sh
```

### Run Individual Benchmarks

```bash
# Context usage (token count)
tsx benchmarks/context-usage.bench.ts

# Latency (p95/p99)
tsx benchmarks/latency.bench.ts
```

## Benchmark Scripts

### context-usage.bench.ts
Measures token count for eager vs lazy loading across 4 test scenarios:
- `googledocs`: Single MCP server intent
- `atlassian`: Single MCP server intent
- `slack`: Single MCP server intent
- `ambiguous`: Fallback to all tools

**Output**: Token counts, reduction percentages

**Target**: <1000 tokens for filtered scenarios (excluding ambiguous)

---

### latency.bench.ts
Measures p50/p95/p99 latency for:
- Intent analysis
- Schema filtering

**Iterations**: 100 per operation (with 10 warm-up iterations)

**Targets**:
- Schema filter p99: <20ms
- Intent analyzer p99: <50ms

---

## Library Components

### lib/harness.ts
Reusable benchmark runner with percentile calculation.

**Functions**:
- `runBenchmark(name, fn, iterations)`: Run benchmark with N iterations
- `calculatePercentiles(samples)`: Calculate p50/p95/p99 from samples

---

### lib/token-counter.ts
Token counting with tiktoken (BPE encoding) or heuristic fallback.

**Functions**:
- `countTokens(text)`: Count tokens in text
- `getTokenCountingMethod()`: Get current method ('tiktoken' or 'heuristic')

**Fallback**: If tiktoken WASM fails, uses `chars / 4` heuristic (~90% accuracy)

---

### lib/scenarios.ts
Test scenarios for benchmarking.

**Scenarios**:
1. `googledocs`: "Read my Google Doc named Project Plan"
2. `atlassian`: "Search for recent Jira tickets"
3. `slack`: "Find slack messages about the project"
4. `ambiguous`: "Help me find information"

---

### lib/report-generator.ts
Markdown report generation from benchmark results.

**Functions**:
- `generateReport(data, outputPath)`: Generate markdown report

---

## Methodology

- **Iterations**: 100 per benchmark (warm-up: 10 iterations)
- **Timing**: `performance.now()` (high-resolution)
- **Percentile Calculation**: Sort samples, index at percentile threshold
- **Token Counting**: tiktoken (BPE encoding) or heuristic (chars / 4)
- **Test Scenarios**: 4 synthetic cases (defined in lib/scenarios.ts)

## Results

Benchmark results are saved as JSON files and can be used to generate a markdown report:

```bash
# Example: Generate report from results
tsx benchmarks/generate-report.ts
```

The report will include:
- Executive summary
- Context usage table (baseline vs lazy tokens)
- Latency percentile tables
- Recommendations based on targets

## Troubleshooting

### tiktoken WASM fails

If tiktoken fails to load:
```
[token-counter] tiktoken WASM failed, using heuristic fallback
```

**Solution**: The benchmark will automatically use the `chars / 4` heuristic. For more accurate measurements, ensure Node.js has WASM support (Node.js v20.x LTS recommended).

### TypeScript errors

If you encounter TypeScript compilation errors:
```bash
# Build the project first
npm run build

# Then run benchmarks
./benchmarks/run-all.sh
```

## Next Steps

After running benchmarks:

1. **Review Results**: Check JSON output files
2. **Validate Targets**:
   - Context reduction: >80% for filtered scenarios
   - Latency p99: <50ms (schema filter <20ms)
3. **Optimize**: If targets not met, optimize based on findings
4. **CI/CD Integration**: Add benchmarks to CI pipeline (future work)

## License

Same as parent project.
