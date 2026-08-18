#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$script_dir"

test -f README.md
test -f SPEC.md
test -f ARCHITECTURE.md
npm test
npm run build
