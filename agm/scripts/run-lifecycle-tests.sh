#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
mode="${1:-all}"

cd "${repo_root}"

case "${mode}" in
all)
	go test -race -count=1 -timeout=20m \
		-tags=integration ./agm/test/integration/...
	;;
portable)
	go test -race -count=1 -tags=integration \
		./agm/test/integration/portable \
		-run 'TestActiveHarnessParityContract|TestHarnessPrerequisitesAreScoped'
	;;
isolated)
	go test -race -count=1 -tags=integration \
		./agm/test/integration/isolated \
		-run '^TestCodexLifecycleUsesIsolatedSourceEnvironment$'
	;;
help|-h|--help)
	printf '%s\n' \
		"Usage: $0 [all|portable|isolated]" \
		"  all       Run the complete integration-tagged graph (default)." \
		"  portable  Run credential-free parity for every active harness." \
		"  isolated  Run the source-built Codex lifecycle on owned state."
	;;
*)
	printf 'unknown lifecycle test mode: %s\n' "${mode}" >&2
	exit 2
	;;
esac
