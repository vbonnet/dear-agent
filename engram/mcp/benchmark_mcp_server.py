#!/usr/bin/env python3
"""
Performance Benchmarks for Engram MCP Server (Task 3.5)

Measures tool invocation latency and validates <100ms performance target.
"""

import json
import sys
import time
import subprocess
from pathlib import Path
from statistics import mean, median, stdev

# Benchmark configuration
WARMUP_RUNS = 3  # Warmup iterations (not counted)
BENCHMARK_RUNS = 20  # Actual benchmark iterations

# Test queries for engram_retrieve
RETRIEVE_QUERIES = [
    ("error handling", 3),
    ("testing patterns", 5),
    ("go concurrency", 3),
    ("python async", 5),
    ("code review", 3)
]


def run_tool_call(server_process, tool_name: str, arguments: dict):
    """Execute a tool call and measure latency."""
    request = {
        "jsonrpc": "2.0",
        "id": int(time.time() * 1000),
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    }

    start_time = time.perf_counter()

    # Send request
    server_process.stdin.write(json.dumps(request) + '\n')
    server_process.stdin.flush()

    # Read response
    response_line = server_process.stdout.readline()
    response = json.loads(response_line)

    end_time = time.perf_counter()
    latency_ms = (end_time - start_time) * 1000.0

    # Check for errors
    if 'error' in response:
        return None, response['error']

    return latency_ms, None


def benchmark_tool(server_process, tool_name: str, arguments: dict, label: str):
    """Benchmark a specific tool."""
    print(f"\n{label}")
    print("=" * 60)

    # Warmup
    print(f"Warming up ({WARMUP_RUNS} iterations)...")
    for _ in range(WARMUP_RUNS):
        latency, error = run_tool_call(server_process, tool_name, arguments)
        if error:
            print(f"❌ Warmup error: {error}")
            return None

    # Actual benchmark
    print(f"Running benchmark ({BENCHMARK_RUNS} iterations)...")
    latencies = []

    for i in range(BENCHMARK_RUNS):
        latency, error = run_tool_call(server_process, tool_name, arguments)

        if error:
            print(f"❌ Iteration {i+1} error: {error}")
            continue

        latencies.append(latency)

    if not latencies:
        print("❌ No successful iterations")
        return None

    # Calculate statistics
    avg_ms = mean(latencies)
    median_ms = median(latencies)
    min_ms = min(latencies)
    max_ms = max(latencies)
    std_dev_ms = stdev(latencies) if len(latencies) > 1 else 0

    # Calculate percentiles
    sorted_latencies = sorted(latencies)
    p95_index = int(len(sorted_latencies) * 0.95)
    p99_index = int(len(sorted_latencies) * 0.99)
    p95_ms = sorted_latencies[p95_index]
    p99_ms = sorted_latencies[p99_index]

    # Performance evaluation
    target_ms = 100.0
    meets_target = avg_ms < target_ms
    status = "✅ PASS" if meets_target else "⚠️  SLOW"

    print(f"\n{status} - Target: <{target_ms}ms")
    print(f"  Average:    {avg_ms:6.2f}ms")
    print(f"  Median:     {median_ms:6.2f}ms")
    print(f"  Min:        {min_ms:6.2f}ms")
    print(f"  Max:        {max_ms:6.2f}ms")
    print(f"  Std Dev:    {std_dev_ms:6.2f}ms")
    print(f"  P95:        {p95_ms:6.2f}ms")
    print(f"  P99:        {p99_ms:6.2f}ms")
    print(f"  Iterations: {len(latencies)}/{BENCHMARK_RUNS}")

    return {
        "tool": tool_name,
        "label": label,
        "avg_ms": avg_ms,
        "median_ms": median_ms,
        "min_ms": min_ms,
        "max_ms": max_ms,
        "std_dev_ms": std_dev_ms,
        "p95_ms": p95_ms,
        "p99_ms": p99_ms,
        "meets_target": meets_target,
        "iterations": len(latencies)
    }


def main():
    """Run performance benchmarks."""
    print("=" * 60)
    print("Engram MCP Server Performance Benchmarks (Task 3.5)")
    print("=" * 60)
    print(f"\nTarget: All tools <100ms latency")
    print(f"Warmup: {WARMUP_RUNS} iterations")
    print(f"Benchmark: {BENCHMARK_RUNS} iterations per test")

    # Start MCP server
    server_script = Path(__file__).parent / "engram_mcp_server.py"

    if not server_script.exists():
        print(f"\n❌ ERROR: Server script not found: {server_script}")
        sys.exit(1)

    print(f"\nStarting MCP server...")

    server_process = subprocess.Popen(
        [sys.executable, str(server_script)],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        bufsize=1
    )

    try:
        # Initialize server
        init_request = {
            "jsonrpc": "2.0",
            "id": 0,
            "method": "initialize",
            "params": {}
        }

        server_process.stdin.write(json.dumps(init_request) + '\n')
        server_process.stdin.flush()
        server_process.stdout.readline()

        # Run benchmarks
        results = []

        # 1. engram_retrieve (multiple queries for comprehensive testing)
        for query, top_k in RETRIEVE_QUERIES:
            result = benchmark_tool(
                server_process,
                "engram_retrieve",
                {"query": query, "top_k": top_k},
                f"engram_retrieve: '{query}' (top_k={top_k})"
            )
            if result:
                results.append(result)

        # 2. engram_plugins_list
        result = benchmark_tool(
            server_process,
            "engram_plugins_list",
            {},
            "engram_plugins_list"
        )
        if result:
            results.append(result)

        # 3. wayfinder_phase_status (if test project exists)
        test_project = Path.home() / "src/ws/oss/wf/workflow-improvements-batch-edit"
        if test_project.exists():
            result = benchmark_tool(
                server_process,
                "wayfinder_phase_status",
                {"project_path": str(test_project)},
                "wayfinder_phase_status"
            )
            if result:
                results.append(result)
        else:
            print(f"\n⚠️  Skipping wayfinder_phase_status (test project not found)")

        # 4. beads_create (single iteration - modifies database)
        print(f"\nbeads_create (single test iteration)")
        print("=" * 60)
        latency, error = run_tool_call(
            server_process,
            "beads_create",
            {
                "title": f"Benchmark Test Bead {int(time.time())}",
                "description": "Test bead for performance benchmarking. Safe to delete.",
                "priority": 2,
                "labels": ["benchmark", "test"],
                "estimated_minutes": 5
            }
        )

        if error:
            print(f"❌ Error: {error}")
        elif latency:
            meets_target = latency < 100.0
            status = "✅ PASS" if meets_target else "⚠️  SLOW"
            print(f"{status} - Latency: {latency:.2f}ms (target: <100ms)")

            results.append({
                "tool": "beads_create",
                "label": "beads_create",
                "avg_ms": latency,
                "meets_target": meets_target,
                "iterations": 1
            })

        # Summary
        print("\n" + "=" * 60)
        print("Performance Summary")
        print("=" * 60)

        total_tests = len(results)
        passed_tests = sum(1 for r in results if r["meets_target"])
        failed_tests = total_tests - passed_tests

        print(f"\nTests Passed: {passed_tests}/{total_tests}")
        print(f"Tests Failed: {failed_tests}/{total_tests}")

        print("\nDetailed Results:")
        print("-" * 60)

        for result in results:
            status = "✅" if result["meets_target"] else "⚠️ "
            print(f"{status} {result['label']:40s} {result['avg_ms']:6.2f}ms")

        if failed_tests == 0:
            print("\n✅ All performance targets met!")
            return 0
        else:
            print(f"\n⚠️  {failed_tests} test(s) exceeded 100ms target")
            return 1

    finally:
        # Cleanup
        server_process.terminate()
        server_process.wait(timeout=5)


if __name__ == '__main__':
    sys.exit(main())
