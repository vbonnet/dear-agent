#!/usr/bin/env python3
"""Export a SWE-bench dataset to the JSON shape `agm benchmark swe-lite` expects.

`agm benchmark swe-lite --dataset <file>` reads a JSON array of tasks with the
field names used by agm/cmd/agm/benchmark_swe.go's SWETask struct. The upstream
SWE-bench datasets live on the Hugging Face Hub; this script pulls them via the
public datasets-server REST API (no `datasets`/`pip` dependency — stdlib only)
and reshapes each row.

Usage:
  python3 scripts/export_swe_bench.py > swe-bench-lite.json          # 300 Lite
  python3 scripts/export_swe_bench.py --suite verified > sb-ver.json # Verified
  python3 scripts/export_swe_bench.py --limit 5 > sample.json        # first 5

The `--suite` values map to the canonical datasets:
  lite     -> princeton-nlp/SWE-bench_Lite       (split: test, 300 rows)
  verified -> princeton-nlp/SWE-bench_Verified    (split: test, 500 rows)
  full     -> princeton-nlp/SWE-bench             (split: test)

SWE-bench Lite is the recommended starting tier: cheapest to run, fastest to
detect regressions. Grading the produced patches still requires the Docker
SWE-bench harness (or the sb-cli cloud evaluator) — this script only exports
the task inputs.
"""

import argparse
import json
import sys
import urllib.parse
import urllib.request

DATASETS = {
    "lite": "princeton-nlp/SWE-bench_Lite",
    "verified": "princeton-nlp/SWE-bench_Verified",
    "full": "princeton-nlp/SWE-bench",
}

ROWS_API = "https://datasets-server.huggingface.co/rows"
PAGE = 100  # datasets-server caps `length` at 100 rows per request


def fetch_rows(dataset, split, offset, length):
    qs = urllib.parse.urlencode(
        {
            "dataset": dataset,
            "config": "default",
            "split": split,
            "offset": offset,
            "length": length,
        }
    )
    req = urllib.request.Request(
        f"{ROWS_API}?{qs}", headers={"User-Agent": "dear-agent-export-swe-bench"}
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)


def reshape(row):
    """Map an upstream SWE-bench row to agm's SWETask field names."""
    problem = row.get("problem_statement", "") or ""
    return {
        "instance_id": row["instance_id"],
        "repo": row["repo"],
        "issue": problem.split("\n", 1)[0][:200],
        "problem_statement": problem,
        "base_commit": row.get("base_commit", ""),
        "patch": row.get("patch", ""),
        "test_patch": row.get("test_patch", ""),
        "version": str(row.get("version", "")),
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument(
        "--suite",
        choices=sorted(DATASETS),
        default="lite",
        help="which SWE-bench dataset (default: lite)",
    )
    ap.add_argument("--split", default="test", help="dataset split (default: test)")
    ap.add_argument(
        "--limit",
        type=int,
        default=0,
        help="max tasks to export (0 = all in the split)",
    )
    ap.add_argument(
        "--offset", type=int, default=0, help="row offset to start from (default: 0)"
    )
    args = ap.parse_args()

    dataset = DATASETS[args.suite]

    first = fetch_rows(dataset, args.split, args.offset, 1)
    total = first.get("num_rows_total", 0)
    want = total - args.offset
    if args.limit > 0:
        want = min(want, args.limit)
    if want <= 0:
        print(f"no rows to export (total={total}, offset={args.offset})", file=sys.stderr)
        json.dump([], sys.stdout)
        return

    tasks = []
    fetched = 0
    while fetched < want:
        batch = min(PAGE, want - fetched)
        page = fetch_rows(dataset, args.split, args.offset + fetched, batch)
        rows = [r["row"] for r in page.get("rows", [])]
        if not rows:
            break
        tasks.extend(reshape(r) for r in rows)
        fetched += len(rows)
        print(
            f"fetched {fetched}/{want} from {dataset}:{args.split}",
            file=sys.stderr,
        )

    json.dump(tasks, sys.stdout, indent=2)
    sys.stdout.write("\n")
    print(f"exported {len(tasks)} tasks", file=sys.stderr)


if __name__ == "__main__":
    main()
