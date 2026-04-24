#!/usr/bin/env python3
"""Export SWE-bench-lite dataset from HuggingFace to JSON for agm benchmark.

Usage:
    python3 export_swe_bench.py                     # stdout
    python3 export_swe_bench.py -o swe-bench-lite.json  # file

Requires: pip install datasets
"""

import argparse
import json
import sys


def main():
    parser = argparse.ArgumentParser(description="Export SWE-bench-lite dataset")
    parser.add_argument("-o", "--output", help="Output file (default: stdout)")
    parser.add_argument(
        "--split", default="test", help="Dataset split (default: test)"
    )
    parser.add_argument(
        "--limit", type=int, default=0, help="Limit number of tasks (0 = all)"
    )
    args = parser.parse_args()

    try:
        from datasets import load_dataset
    except ImportError:
        print(
            "Error: 'datasets' package not found. Install with: pip install datasets",
            file=sys.stderr,
        )
        sys.exit(1)

    ds = load_dataset("princeton-nlp/SWE-bench_Lite", split=args.split)

    tasks = []
    for row in ds:
        tasks.append(
            {
                "instance_id": row["instance_id"],
                "repo": row["repo"],
                "issue": (row.get("problem_statement") or "")[:200],
                "problem_statement": row.get("problem_statement", ""),
                "base_commit": row.get("base_commit", ""),
                "patch": row.get("patch", ""),
                "test_patch": row.get("test_patch", ""),
                "version": row.get("version", ""),
            }
        )

    if args.limit > 0:
        tasks = tasks[: args.limit]

    output = json.dumps(tasks, indent=2)

    if args.output:
        with open(args.output, "w") as f:
            f.write(output)
        print(f"Exported {len(tasks)} tasks to {args.output}", file=sys.stderr)
    else:
        print(output)


if __name__ == "__main__":
    main()
