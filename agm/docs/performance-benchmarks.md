# AGM Performance Benchmarks

**Last Updated**: 2026-01-24

## Overview

This document describes the performance benchmarks for AGM and how to run them.

## Running Benchmarks

### Run All Benchmarks

```bash
cd main/agm
go test -bench=. ./test/
```

### Run Specific Benchmark

```bash
# Session listing at scale
go test -bench=BenchmarkListSessionsScaled ./test/

# Search performance
go test -bench=BenchmarkSearch ./test/

# Lock performance
go test -bench=BenchmarkLock ./test/
```

### Generate Benchmark Report

```bash
go test -bench=. ./test/ -benchmem > benchmark-results.txt
```

## Benchmark Categories

### Session Listing Performance

**Benchmark**: `BenchmarkListSessionsScaled`

**Purpose**: Validates session listing meets <100ms for 1000 sessions target

**Scales Tested**:
- 100 sessions
- 500 sessions
- 1000 sessions

**Note**: The benchmark creates up to 100 sessions for stability. Full 1000-session testing requires manual setup of tmux sessions.

**Target**: <100ms for 1000 sessions

**Example Output**:
```
BenchmarkListSessionsScaled/Sessions_100-8    1000    1234567 ns/op
BenchmarkListSessionsScaled/Sessions_500-8     200    5678901 ns/op
BenchmarkListSessionsScaled/Sessions_1000-8    100   12345678 ns/op
```

### Search Performance

**Benchmarks**:
- `BenchmarkSearchCached`: Measures cache hit performance
- `BenchmarkSearchUncached`: Measures cold search performance

**Purpose**: Validates search caching effectiveness

**Target**: Cache hits should be <1ms, uncached searches depend on LLM API latency

**Example Output**:
```
BenchmarkSearchCached-8       1000000    1234 ns/op
BenchmarkSearchUncached-8        1000 1234567 ns/op
```

### Lock Performance

**Benchmarks**:
- `BenchmarkLockAcquireRelease`: Lock lifecycle
- `BenchmarkLockContention`: Lock under contention
- `BenchmarkLockNew`: Lock creation overhead

**Purpose**: Ensures lock operations don't become bottlenecks

**Target**: Lock operations should complete in microseconds

## Interpreting Results

### Understanding ns/op

- `ns/op`: Nanoseconds per operation
- 1ms = 1,000,000 ns
- 100ms = 100,000,000 ns

**Example**: If `BenchmarkListSessionsScaled/Sessions_1000` shows 50,000,000 ns/op, that's 50ms per operation (meets <100ms target).

### Benchmem Output

- `allocs/op`: Memory allocations per operation
- `B/op`: Bytes allocated per operation

Lower values indicate better memory efficiency.

## Performance Targets

| Operation | Target | Benchmark |
|-----------|--------|-----------|
| List 1000 sessions | <100ms | BenchmarkListSessionsScaled/Sessions_1000 |
| Search cache hit | <1ms | BenchmarkSearchCached |
| Lock acquire/release | <10μs | BenchmarkLockAcquireRelease |

## Profiling

### CPU Profiling

```bash
go test -bench=BenchmarkListSessionsScaled -cpuprofile=cpu.prof ./test/
go tool pprof cpu.prof
```

### Memory Profiling

```bash
go test -bench=BenchmarkListSessionsScaled -memprofile=mem.prof ./test/
go tool pprof mem.prof
```

## Continuous Benchmarking

Benchmarks should be run:
- Before and after performance optimizations
- On every release candidate
- When investigating performance issues

Store benchmark results in `benchmark-results/` directory with timestamp:
```bash
go test -bench=. ./test/ -benchmem > benchmark-results/$(date +%Y%m%d-%H%M%S).txt
```

## Limitations

1. **Tmux Dependency**: Session listing benchmarks require tmux to be installed
2. **Scale Limits**: Full 1000-session benchmarks require manual setup
3. **Environment**: Results vary based on system resources and load

## References

- Go Benchmark Documentation: https://golang.org/pkg/testing/#hdr-Benchmarks
- pprof Profiling: https://go.dev/blog/pprof
