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

# golangci-lint takes a file lock at $TMPDIR/golangci-lint.lock on every run and
# exits 3 ("parallel golangci-lint is running") when it cannot acquire it. That
# lock is global to the host: it is scoped to neither the repository nor the
# checkout. safe-pr's own transaction lock is keyed on the worktree git dir, so
# it does not serialize two agents in different worktrees: both reach
# `make preflight-full` and one dies on the other's linter lock. A global lock
# in the shift-left gate wedges the tool everyone uses to open a PR.
#
# --allow-parallel-runners releases the lock. It is only safe when each run owns
# its cache, which is why the two settings below are a pair: isolating the cache
# alone does not help (the lock is not under the cache), and allowing parallel
# runners over one shared cache is the corruption case upstream warns about.
#
# The cache is keyed on this checkout rather than made unique per invocation, so
# it stays warm across runs in one worktree while never being shared with a
# concurrent run in another. Callers may override it.
: "${GOLANGCI_LINT_CACHE:=${XDG_CACHE_HOME:-$HOME/.cache}/dear-agent/golangci-lint/$(printf '%s' "$REPO_ROOT" | cksum | cut -d' ' -f1)}"
export GOLANGCI_LINT_CACHE

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
warn "lint cache: ${GOLANGCI_LINT_CACHE}"
# Do not collapse the linter's exit codes. Exit 3 is "could not acquire the
# lock", and folding it into fail()'s exit 1 is what made months of lock
# collisions indistinguishable from real lint failures in safe-pr's audit log.
set +e
golangci-lint run --allow-parallel-runners --timeout=5m ./...
LINT_RC=$?
set -e
if [[ "$LINT_RC" -eq 3 ]]; then
  # Normalize the trailing slash: macOS sets TMPDIR with one, an unset TMPDIR
  # falls back to a bare /tmp, and naive concatenation prints /tmpgolangci-lint.lock.
  LOCK_DIR="${TMPDIR:-/tmp}"
  fail "golangci-lint could not acquire its lock (exit 3) despite --allow-parallel-runners. Another run is holding ${LOCK_DIR%/}/golangci-lint.lock; this is a tooling regression, not a lint failure."
elif [[ "$LINT_RC" -ne 0 ]]; then
  fail "lint failed (see above)"
fi
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
  step "go test ordinary performance SLA packages"
  # The broad full suite intentionally uses -race for data-race parity. Race
  # instrumentation distorts wall-clock latency, so re-run every package that
  # skips a wall-clock SLA under race as a required ordinary publication gate.
  # Serialize these latency-sensitive packages so they measure the code under
  # test rather than competing package test processes.
  # Clear inherited Go test flags as well as CI: GOFLAGS=-race, -short, -run,
  # or custom tags can otherwise skip the exact assertions this gate exists to
  # enforce while `go test` still exits successfully.
  GOFLAGS='' CI='' go test -race=false -short=false -p=1 -count=1 -timeout="${TEST_TIMEOUT}" \
    ./pkg/workflow \
    ./agm/test/performance \
    ./internal/telemetry/enrichment \
    ./pkg/validation/scope ||
    fail "ordinary performance SLA tests failed"
  ok "ordinary performance SLAs pass"

  step "govulncheck ./..."
  GOVULNCHECK_BIN="$(command -v govulncheck || true)"
  if [[ -z "$GOVULNCHECK_BIN" ]]; then
    # `go install` writes to GOBIN when configured; only an empty GOBIN falls
    # back to the first GOPATH entry's bin directory.
    GO_TOOL_INSTALL_BIN="$(go env GOBIN)"
    if [[ -z "$GO_TOOL_INSTALL_BIN" ]]; then
      GOPATH_VALUE="$(go env GOPATH)"
      GO_TOOL_INSTALL_BIN="${GOPATH_VALUE%%:*}/bin"
    fi
    GOVULNCHECK_CANDIDATE="$GO_TOOL_INSTALL_BIN/govulncheck"
    if [[ -x "$GOVULNCHECK_CANDIDATE" ]]; then
      GOVULNCHECK_BIN="$GOVULNCHECK_CANDIDATE"
    fi
  fi
  if [[ -z "$GOVULNCHECK_BIN" ]]; then
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
  "$GOVULNCHECK_BIN" -format json -scan package ./... > "$TMP_VULN" || VULN_EXIT=$?
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
