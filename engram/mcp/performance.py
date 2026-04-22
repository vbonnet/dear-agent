"""
Performance Profiling Module - Task 3.5

Measures tool invocation latency and tracks performance metrics.
Target: All tools <100ms latency.
"""

import time
import logging
from typing import Dict, List
from contextlib import contextmanager
from collections import defaultdict

logger = logging.getLogger('engram-mcp-server.performance')


class PerformanceProfiler:
    """Track and measure tool invocation performance."""

    def __init__(self):
        """Initialize performance profiler."""
        self.metrics: Dict[str, List[float]] = defaultdict(list)
        self.target_latency_ms = 100.0  # Target: <100ms per tool call

    @contextmanager
    def profile(self, tool_name: str):
        """Context manager for profiling tool execution.

        Args:
            tool_name: Name of the tool being profiled

        Yields:
            None

        Example:
            with profiler.profile('engram_retrieve'):
                result = engram_retrieve.retrieve(query)
        """
        start_time = time.perf_counter()
        try:
            yield
        finally:
            end_time = time.perf_counter()
            latency_ms = (end_time - start_time) * 1000.0
            self.metrics[tool_name].append(latency_ms)

            # Log warning if exceeds target
            if latency_ms > self.target_latency_ms:
                logger.warning(
                    f"Tool {tool_name} exceeded target latency: "
                    f"{latency_ms:.2f}ms > {self.target_latency_ms}ms"
                )

    def get_stats(self) -> Dict[str, Dict[str, float]]:
        """Get performance statistics for all tools.

        Returns:
            Dictionary mapping tool names to stats:
            {
                "tool_name": {
                    "count": <invocation count>,
                    "avg_ms": <average latency>,
                    "min_ms": <minimum latency>,
                    "max_ms": <maximum latency>,
                    "p95_ms": <95th percentile>,
                    "p99_ms": <99th percentile>,
                    "exceeds_target": <boolean>
                }
            }
        """
        stats = {}

        for tool_name, latencies in self.metrics.items():
            if not latencies:
                continue

            sorted_latencies = sorted(latencies)
            count = len(sorted_latencies)

            # Calculate percentiles
            p95_index = int(count * 0.95)
            p99_index = int(count * 0.99)

            avg_ms = sum(latencies) / count
            exceeds_target = avg_ms > self.target_latency_ms

            stats[tool_name] = {
                "count": count,
                "avg_ms": avg_ms,
                "min_ms": min(latencies),
                "max_ms": max(latencies),
                "p95_ms": sorted_latencies[p95_index] if p95_index < count else sorted_latencies[-1],
                "p99_ms": sorted_latencies[p99_index] if p99_index < count else sorted_latencies[-1],
                "exceeds_target": exceeds_target
            }

        return stats

    def get_summary(self) -> str:
        """Get human-readable performance summary.

        Returns:
            Formatted performance summary string
        """
        stats = self.get_stats()

        if not stats:
            return "No performance data collected yet."

        lines = ["Performance Summary:", "=" * 60]

        for tool_name, metrics in sorted(stats.items()):
            status = "⚠️ SLOW" if metrics["exceeds_target"] else "✅ OK"
            lines.append(f"\n{tool_name}: {status}")
            lines.append(f"  Count:    {metrics['count']}")
            lines.append(f"  Avg:      {metrics['avg_ms']:.2f}ms")
            lines.append(f"  Min/Max:  {metrics['min_ms']:.2f}ms / {metrics['max_ms']:.2f}ms")
            lines.append(f"  P95/P99:  {metrics['p95_ms']:.2f}ms / {metrics['p99_ms']:.2f}ms")

        lines.append(f"\nTarget latency: <{self.target_latency_ms}ms")

        return "\n".join(lines)

    def reset(self):
        """Reset all performance metrics."""
        self.metrics.clear()
        logger.info("Performance metrics reset")
