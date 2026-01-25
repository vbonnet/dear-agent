#!/bin/bash
#
# Run All Benchmarks
#
# Executes all MCP performance benchmarks and generates report.
#
# Usage: ./benchmarks/run-all.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

echo "==================================="
echo "MCP Performance Benchmarks"
echo "==================================="
echo ""

# Check for tsx
if ! command -v tsx &> /dev/null; then
    echo "Error: tsx not found. Install with: npm install -g tsx"
    exit 1
fi

# Run context usage benchmark
echo "1/2: Running context usage benchmark..."
tsx benchmarks/context-usage.bench.ts > benchmarks/context-usage.json

# Run latency benchmark
echo ""
echo "2/2: Running latency benchmark..."
tsx benchmarks/latency.bench.ts > benchmarks/latency.json

# Note: Startup and memory benchmarks require running servers
# These are commented out for now and can be run manually
# echo "3/4: Running startup benchmark..."
# tsx benchmarks/startup.bench.ts > benchmarks/startup.json
# echo "4/4: Running memory benchmark..."
# tsx benchmarks/memory.bench.ts > benchmarks/memory.json

echo ""
echo "==================================="
echo "Benchmarks Complete!"
echo "==================================="
echo ""
echo "Results saved to:"
echo "  - benchmarks/context-usage.json"
echo "  - benchmarks/latency.json"
echo ""
echo "Next steps:"
echo "  1. Review results in JSON files"
echo "  2. Run 'tsx benchmarks/generate-report.ts' to create markdown report"
echo ""
