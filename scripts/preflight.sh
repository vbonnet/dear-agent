#!/usr/bin/env bash
# preflight.sh — run the same checks GitHub Actions CI runs, locally.
#
# Shift-left for the inner dev loop. CI on GitHub is not a substitute for
# running these gates before pushing — it is a backstop. See
# vbonnet/engram-research retrospectives/2026-05-27-ci-shift-left.md for why
# this script exists.
#
# Usage:
#   scripts/preflight.sh            # fast tier: vet + build + AI skills + lint
#   scripts/preflight.sh --tests    # fast tier + go test (no -race, no vuln)
#   scripts/preflight.sh --race     # fast tier + go test -race (no vuln)
#   scripts/preflight.sh --full     # add: race tests + ordinary performance SLA + govulncheck
#
# Exits non-zero on the first failing gate so a pre-push hook can chain it.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

MODE="fast"
case "${1:-}" in
  --full) MODE="full" ;;
  --race) MODE="race" ;;
  --tests) MODE="tests" ;;
  --fast|"") MODE="fast" ;;
  -h|--help)
    sed -n '2,16p' "$0"
    exit 0
    ;;
  *)
    echo "preflight: unknown flag '$1' (try --fast | --tests | --race | --full)" >&2
    exit 2
    ;;
esac

# Pretty output that survives non-TTY CI capture.
if [[ -t 1 ]]; then
  G=$'\033[32m'; R=$'\033[31m'; Y=$'\033[33m'; B=$'\033[1m'; N=$'\033[0m'
else
  G=""; R=""; Y=""; B=""; N=""
fi

step() { printf '%s==> %s%s\n' "$B" "$*" "$N"; }
ok()   { printf '%s✓%s %s\n' "$G" "$N" "$*"; }
warn() { printf '%s!%s %s\n' "$Y" "$N" "$*"; }
fail() { printf '%s✗%s %s\n' "$R" "$N" "$*"; exit 1; }

# Mirror CI: GOWORK=off so we don't accidentally pull in unrelated modules.
export GOWORK=off

START_TS=$(date +%s)

step "go mod download"
go mod download
ok "modules ready"

step "go vet ./..."
go vet ./... || fail "go vet failed"
ok "vet clean"

step "go build ./..."
# CI also builds three binaries with ldflags into ./build/. We replicate that
# here so build-time errors hidden by output-name collisions surface locally.
mkdir -p build
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "local")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-s -w -X github.com/vbonnet/dear-agent/pkg/version.Version=local -X github.com/vbonnet/dear-agent/pkg/version.GitCommit=${GIT_COMMIT} -X github.com/vbonnet/dear-agent/pkg/version.BuildDate=${BUILD_DATE} -X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=preflight"
go build -ldflags="${LDFLAGS}" -o build/agm ./agm/cmd/agm
go build -ldflags="${LDFLAGS}" -o build/agm-reaper ./agm/cmd/agm-reaper
go build -ldflags="${LDFLAGS}" -o build/agm-mcp-server ./agm/cmd/agm-mcp-server
go build ./...
ok "build clean"

step "validate tracked AI skills"
go run ./tools/skill-lint -repo . || fail "AI skill validation failed"
ok "AI skills valid"

step "make lint-instructions"
make lint-instructions || fail "active instruction policy integrity failed"
ok "active instruction policy guidance intact"

step "make lint-adrs"
make lint-adrs || fail "ADR identity/index/lifecycle contract failed"
ok "ADR identity/index/lifecycle contract intact"

step "make lint-headers"
make lint-headers || fail "doc header format validation failed"
ok "doc header format valid"

step "golangci-lint run ./..."
if ! command -v golangci-lint >/dev/null 2>&1; then
  fail "golangci-lint not installed. Run: brew install golangci-lint"
fi
# CI pins 'version: latest' in golangci-lint-action@v9. Local version may
# differ — drift between local and CI staticcheck versions has burned us
# before (SA5011 false-positives, PR #158). Surface the version so it's
# visible in the log.
LINT_VER="$(golangci-lint version 2>&1 | head -n 1 || true)"
warn "local linter: ${LINT_VER}"
golangci-lint run --timeout=5m ./... || fail "lint failed (see above)"
ok "lint clean"

step "verify AGM plugin hashes"
make plugin-verify-hashes || fail "AGM plugin content hashes are stale"
ok "AGM plugin hashes verified"

TEST_TIMEOUT="20m"
if [[ "$MODE" == "tests" || "$MODE" == "race" || "$MODE" == "full" ]]; then
  step "go test ./..."
  # Mirror ci.yml: CI_SKIP_TMUX=true on macOS, false on Linux. The tmux
  # tests assume an isolated tmux server; on a developer's macOS box
  # there's nearly always a stray attached tmux that turns these tests
  # into a flake parade (TmuxLock_CrossProcess et al — see
  # memory/dear-agent-ci-flakes.md). CI gets away with `false` only on
  # ubuntu where every job starts a fresh sandbox.
  if [[ "$(uname -s)" == "Darwin" ]]; then
    export CI_SKIP_TMUX=true
    warn "CI_SKIP_TMUX=true on macOS (mirrors ci.yml)"
  else
    export CI_SKIP_TMUX=false
  fi
  # --full and --race both use -race -count=1 (CI parity for data-race detection).
  # --tests skips -race for a faster contributor sanity check.
  if [[ "$MODE" == "full" || "$MODE" == "race" ]]; then
    go test -race -count=1 -timeout="${TEST_TIMEOUT}" ./... || fail "tests failed"
  else
    go test -count=1 -timeout="${TEST_TIMEOUT}" ./... || fail "tests failed"
  fi
  ok "tests pass"
fi

if [[ "$MODE" == "full" ]]; then
  step "go test ./agm/test/performance (ordinary SLA enforcement)"
  # The broad full suite intentionally uses -race for data-race parity. Race
  # instrumentation distorts wall-clock latency, so run the dedicated
  # performance package once without it as a required publication gate.
  CI= go test -count=1 -timeout="${TEST_TIMEOUT}" ./agm/test/performance ||
    fail "ordinary performance SLA tests failed"
  ok "ordinary performance SLAs pass"

  step "govulncheck ./..."
  if ! command -v govulncheck >/dev/null 2>&1; then
    fail "govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest"
  fi
  if ! command -v jq >/dev/null 2>&1; then
    fail "jq not installed. Run: brew install jq"
  fi
  # Mirror ci.yml package-scan mode and its reviewed not-called/module-only
  # findings. GO-2025-3884 is gorilla/csrf pulled in via tailscale.com/client/web.
  # GO-2026-5932 marks x/crypto/openpgp unmaintained with no fixed release;
  # govulncheck confirms this repository imports no affected package or symbol.
  # `mktemp` with no arg is portable across macOS BSD and GNU; using a
  # `-t prefix.XXX.json` template tripping GNU mktemp's "must end in XXX"
  # rule is not worth the prettier filename.
  TMP_VULN=$(mktemp)
  trap 'rm -f "$TMP_VULN"' EXIT
  # govulncheck exit codes: 0 = no findings, 3 = findings (allowlisted or
  # not). Anything else (compile error, panic, module load failure) is a
  # real failure we must not mask. A blanket `|| true` would let those
  # silently pass through with $TMP_VULN empty and jq returning "no
  # findings" — a security-critical false-positive.
  VULN_EXIT=0
  govulncheck -format json -scan package ./... > "$TMP_VULN" || VULN_EXIT=$?
  if [[ $VULN_EXIT -ne 0 && $VULN_EXIT -ne 3 ]]; then
    fail "govulncheck failed to run (exit code $VULN_EXIT)"
  fi
  unhandled=$(jq -c \
    --argjson allow '["GO-2025-3884", "GO-2026-5932"]' \
    'select(.finding != null)
     | select(.finding.osv as $id | $allow | index($id) | not)
     | .finding' \
    "$TMP_VULN")
  if [[ -n "$unhandled" ]]; then
    echo "$unhandled"
    fail "govulncheck found unallowed vulnerabilities"
  fi
  ok "no unallowed vulns"
fi

END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))
printf '\n%s✓ preflight %s passed in %ss%s\n' "$G$B" "$MODE" "$ELAPSED" "$N"
