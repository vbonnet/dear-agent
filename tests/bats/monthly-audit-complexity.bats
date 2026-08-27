#!/usr/bin/env bats

bats_require_minimum_version 1.7.0

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  SCRIPT="${REPO_ROOT}/scripts/monthly-audit-complexity.sh"
  WORKFLOW="${REPO_ROOT}/.github/workflows/monthly-audit.yml"
  SHELL_LINT_WORKFLOW="${REPO_ROOT}/.github/workflows/shell-lint.yml"
  FIXTURES="${BATS_TEST_DIRNAME}/testdata/monthly-audit-complexity"
  WORK="${BATS_TEST_TMPDIR}/work"
  STUBS="${WORK}/stubs"
  CAPTURE_ROOT="${WORK}/capture"
  mkdir -p "${STUBS}" "${CAPTURE_ROOT}"
  export PATH="${STUBS}:${PATH}"
  export FAKE_ARG_LOG="${WORK}/args.log"
  export FAKE_COMPLETED="${WORK}/completed"
}

stub_gocognit() {
  export FAKE_GOCOGNIT_MODE="$1"
  {
    echo '#!/usr/bin/env bash'
    echo 'printf "%s\n" "$@" > "${FAKE_ARG_LOG}"'
    cat <<'STUB'
case "${FAKE_GOCOGNIT_MODE}" in
  clean) exit 0 ;;
  findings) printf '%s\n' '42 pkg complexFn file.go:1:1'; exit 1 ;;
  long)
    for i in $(seq 1 60); do
      printf '42 pkg complexFn%02d file.go:%d:1\n' "$i" "$i"
    done
    printf 'complete\n' > "${FAKE_COMPLETED}"
    exit 1
    ;;
  invalid) printf '%s\n' 'gocognit: open missing: no such file or directory' >&2; exit 1 ;;
  partial) printf '%s\n' '42 pkg partial file.go:1:1'; printf '%s\n' 'gocognit: read failed' >&2; exit 1 ;;
  unexpected) printf '%s\n' 'unexpected status-zero payload'; exit 0 ;;
  *) printf '%s\n' 'unknown fake mode' >&2; exit 70 ;;
esac
STUB
  } > "${STUBS}/gocognit"
  chmod +x "${STUBS}/gocognit"
}

run_scan() {
  run --separate-stderr env TMPDIR="${CAPTURE_ROOT}" bash "${SCRIPT}" "${1:-.}"
}

stage_real_fixture() {
  local fixture_name="$1"
  REAL_FIXTURE="${WORK}/real-fixture"
  mkdir -p "${REAL_FIXTURE}"
  cp -f "${FIXTURES}/${fixture_name}.go.txt" "${REAL_FIXTURE}/fixture.go"
}

@test "clean scan passes exact threshold and directory" {
  stub_gocognit clean
  run_scan .

  [ "${status}" -eq 0 ]
  [ -z "${output}" ]
  [ -z "${stderr}" ]
  [ "$(cat "${FAKE_ARG_LOG}")" = $'-over\n15\n.' ]
  [ -z "$(find "${CAPTURE_ROOT}" -mindepth 1 -print -quit)" ]
}

@test "threshold findings are successful report output" {
  stub_gocognit findings
  run_scan .

  [ "${status}" -eq 0 ]
  [ "${output}" = '42 pkg complexFn file.go:1:1' ]
  [ -z "${stderr}" ]
}

@test "long findings finish before rendering exactly fifty lines" {
  stub_gocognit long
  run_scan .

  [ "${status}" -eq 0 ]
  [ -z "${stderr}" ]
  [ -s "${FAKE_COMPLETED}" ]
  [ "$(printf '%s\n' "${output}" | wc -l | tr -d ' ')" -eq 50 ]
  [[ "${output}" == *'complexFn50 file.go:50:1' ]]
  [[ "${output}" != *'complexFn51 file.go:51:1' ]]
}

@test "invalid scanner input fails with its diagnostic" {
  stub_gocognit invalid
  run_scan missing

  [ "${status}" -eq 1 ]
  [ -z "${output}" ]
  [[ "${stderr}" == *'open missing: no such file or directory'* ]]
  [[ "${stderr}" == *'monthly cognitive-complexity scan failed (status=1)'* ]]
}

@test "partial findings plus a diagnostic fail without publishing partial data" {
  stub_gocognit partial
  run_scan .

  [ "${status}" -eq 1 ]
  [ -z "${output}" ]
  [[ "${stderr}" == *'gocognit: read failed'* ]]
  [[ "${stderr}" != *'pkg partial'* ]]
  [ -z "$(find "${CAPTURE_ROOT}" -mindepth 1 -print -quit)" ]
}

@test "unrecognized nonzero status is preserved" {
  stub_gocognit other
  run_scan .

  [ "${status}" -eq 70 ]
  [ -z "${output}" ]
  [[ "${stderr}" == *'unknown fake mode'* ]]
  [[ "${stderr}" == *'monthly cognitive-complexity scan failed (status=70)'* ]]
}

@test "missing scanner fails instead of reporting clean" {
  run --separate-stderr -127 env PATH="${STUBS}:/usr/bin:/bin" TMPDIR="${CAPTURE_ROOT}" bash "${SCRIPT}" .

  [ "${status}" -eq 127 ]
  [ -z "${output}" ]
  [[ "${stderr}" == *'gocognit'* ]]
  [[ "${stderr}" == *'monthly cognitive-complexity scan failed (status=127)'* ]]
}

@test "unexpected status-zero output fails as protocol drift" {
  stub_gocognit unexpected
  run_scan .

  [ "${status}" -eq 2 ]
  [ -z "${output}" ]
  [[ "${stderr}" == *'monthly cognitive-complexity scan failed (status=2)'* ]]
  [[ "${output}" != *'unexpected status-zero payload'* ]]
}

@test "real scanner reports a clean Go fixture" {
  command -v gocognit >/dev/null 2>&1 || skip "gocognit is exercised by the toolchain Bats job"
  stage_real_fixture clean

  run_scan "${REAL_FIXTURE}"

  [ "${status}" -eq 0 ]
  [ -z "${output}" ]
  [ -z "${stderr}" ]
}

@test "real scanner reports an over-threshold Go fixture" {
  command -v gocognit >/dev/null 2>&1 || skip "gocognit is exercised by the toolchain Bats job"
  stage_real_fixture complex

  run_scan "${REAL_FIXTURE}"

  [ "${status}" -eq 0 ]
  [[ "${output}" == *'realComplexityFixture'* ]]
  [ -z "${stderr}" ]
}

@test "workflows pin the scanner and preserve adapter failure" {
  grep -qF 'go install github.com/uudashr/gocognit/cmd/gocognit@v1.2.1' "${WORKFLOW}"
  grep -qF 'go install github.com/uudashr/gocognit/cmd/gocognit@v1.2.1' "${SHELL_LINT_WORKFLOW}"
  grep -qF 'tests/bats/monthly-audit-complexity.bats' "${SHELL_LINT_WORKFLOW}"
  complexity_step="$(sed -n '/- name: Complexity audit/,/- name: Build summary/p' "${WORKFLOW}")"
  [[ "${complexity_step}" == *$'\n          set -euo pipefail\n'* ]]
  [[ "${complexity_step}" == *$'\n          findings=$(./scripts/monthly-audit-complexity.sh .)\n'* ]]
  [[ "${complexity_step}" != *'set +e'* ]]
  [[ "${complexity_step}" != *'continue-on-error'* ]]
  [[ "${complexity_step}" != *'|| true'* ]]
  [[ "${complexity_step}" != *'|| :'* ]]
  [[ "${complexity_step}" != *'gocognit -over 15 ./...'* ]]
  grep -qF "steps.complexity.outputs.has_complex == 'false'" "${WORKFLOW}"
}
