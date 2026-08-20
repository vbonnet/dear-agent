#!/usr/bin/env bats
#
# Behavioral coverage for infra/import.sh.
#
# The script's decisions live in cmd/tofu-import-plan and are unit tested in
# internal/tofuimport. What is left here is the part only a script can get
# wrong: does it collect the right evidence, does it run exactly the imports
# the plan asked for and no others, and does it stop when the planner or the
# provider says stop.
#
# Every external command is a stub on PATH. Nothing here contacts GitHub, and
# nothing mutates real OpenTofu state.

setup() {
  # shell-matrix.yml runs this directory inside minimal bash/zsh/dash/ash
  # containers that carry no Go toolchain. Those runs prove interpreter
  # portability, which this file does not test: it invokes the script with
  # `bash` explicitly. The toolchain-bearing run is the `bats` job in
  # shell-lint.yml, which is where these cases actually execute.
  command -v go > /dev/null || skip "no Go toolchain; the shell-lint bats job runs this file"

  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  WORK="${BATS_TEST_TMPDIR}/work"
  STUBS="${WORK}/stubs"
  mkdir -p "${STUBS}"

  # Where the stubs record what they were asked to do, so a test can assert on
  # the exact command sequence rather than on log prose.
  export STUB_LOG="${WORK}/calls.log"
  : > "${STUB_LOG}"

  export PATH="${STUBS}:${PATH}"
  export GITHUB_TOKEN="stub-token"
  export PERSONAL_OWNER="vbonnet"

  # The real planner binary, built once per test run. Using the real one keeps
  # this an integration test of the script plus its planner, not of a mock.
  export TOFU_IMPORT_PLAN="${WORK}/tofu-import-plan"
  go build -o "${TOFU_IMPORT_PLAN}" "${REPO_ROOT}/cmd/tofu-import-plan"
}

# stub writes an executable named $1 whose body is $2.
stub() {
  local name="$1" body="$2"
  {
    echo '#!/usr/bin/env bash'
    echo 'printf "%s" "$(basename "$0")" >> "${STUB_LOG}"'
    echo 'printf " %s" "$@" >> "${STUB_LOG}"'
    echo 'printf "\n" >> "${STUB_LOG}"'
    echo "${body}"
  } > "${STUBS}/${name}"
  chmod +x "${STUBS}/${name}"
}

# stub_healthy_fleet installs stubs describing one active repository whose
# ruleset exists remotely and is absent from state.
stub_healthy_fleet() {
  stub tofu '
case "$1" in
  console) echo "\"{\\\"active\\\":[\\\"dear-agent\\\"],\\\"archived\\\":[]}\"" ;;
  show)    echo "{}" ;;
  import)  exit 0 ;;
esac'
  stub gh 'echo "[[{\"id\":18061003,\"name\":\"main-zero-bypass\"}]]"'
}

run_import() {
  cd "${REPO_ROOT}/infra"
  run bash ./import.sh
}

@test "imports every managed resource exactly once" {
  stub_healthy_fleet
  run_import

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"Import complete"* ]]

  # The three managed-repo resources, at their module-qualified addresses.
  grep -qF 'tofu import module.managed_repos["dear-agent"].github_repository.this dear-agent' "${STUB_LOG}"
  grep -qF 'tofu import module.managed_repos["dear-agent"].github_repository_dependabot_security_updates.this dear-agent' "${STUB_LOG}"
  grep -qF 'tofu import module.managed_repos["dear-agent"].github_repository_ruleset.branch_protection dear-agent:18061003' "${STUB_LOG}"

  # No fourth import: an extra one would be a resource nobody declared.
  [ "$(grep -c 'tofu import' "${STUB_LOG}")" -eq 3 ]
}

@test "collects the ruleset listing for each active repository before importing" {
  stub_healthy_fleet
  run_import

  [ "${status}" -eq 0 ]
  grep -qF 'gh api --paginate --slurp /repos/vbonnet/dear-agent/rulesets?per_page=100' "${STUB_LOG}"

  # Evidence first: the listing must be collected before the first mutation,
  # or an unreadable response would surface after state was already changed.
  local listing_line first_import_line
  listing_line="$(grep -n 'gh api' "${STUB_LOG}" | head -1 | cut -d: -f1)"
  first_import_line="$(grep -n 'tofu import' "${STUB_LOG}" | head -1 | cut -d: -f1)"
  [ "${listing_line}" -lt "${first_import_line}" ]
}

@test "refuses to run without a GitHub token" {
  stub_healthy_fleet
  unset GITHUB_TOKEN
  run_import

  [ "${status}" -eq 1 ]
  [[ "${output}" == *"GITHUB_TOKEN is not set"* ]]
  # Nothing was attempted.
  [ ! -s "${STUB_LOG}" ]
}

@test "an ambiguous ruleset stops the run before any state is mutated" {
  stub tofu '
case "$1" in
  console) echo "\"{\\\"active\\\":[\\\"dear-agent\\\"],\\\"archived\\\":[]}\"" ;;
  show)    echo "{}" ;;
  import)  exit 0 ;;
esac'
  # Two rulesets match: the planner cannot tell a rename from a duplicate.
  stub gh 'echo "[[{\"id\":18061003,\"name\":\"main-zero-bypass\"},{\"id\":999,\"name\":\"branch-protection\"}]]"'
  run_import

  [ "${status}" -ne 0 ]
  [ "$(grep -c 'tofu import' "${STUB_LOG}" || true)" -eq 0 ]
}

@test "a stale state binding stops the run before any state is mutated" {
  stub tofu '
case "$1" in
  console) echo "\"{\\\"active\\\":[\\\"dear-agent\\\"],\\\"archived\\\":[]}\"" ;;
  show)    cat <<JSON
{"values":{"root_module":{"child_modules":[{"resources":[
  {"address":"module.managed_repos[\"dear-agent\"].github_repository_ruleset.branch_protection",
   "values":{"repository":"dear-agent","id":"99"}}]}]}}}
JSON
  ;;
  import)  exit 0 ;;
esac'
  stub gh 'echo "[[{\"id\":18061003,\"name\":\"main-zero-bypass\"}]]"'
  run_import

  [ "${status}" -ne 0 ]
  [ "$(grep -c 'tofu import' "${STUB_LOG}" || true)" -eq 0 ]
}

@test "an absent remote object is reported, not treated as a failure" {
  stub tofu '
case "$1" in
  console) echo "\"{\\\"active\\\":[\\\"dear-agent\\\"],\\\"archived\\\":[]}\"" ;;
  show)    echo "{}" ;;
  import)  echo "Error: Not Found" >&2; exit 1 ;;
esac'
  stub gh 'echo "[[{\"id\":18061003,\"name\":\"main-zero-bypass\"}]]"'
  run_import

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"will be CREATED by plan"* ]]
  [[ "${output}" == *"Import complete"* ]]
}

@test "an unrecognized provider failure aborts the run" {
  stub tofu '
case "$1" in
  console) echo "\"{\\\"active\\\":[\\\"dear-agent\\\"],\\\"archived\\\":[]}\"" ;;
  show)    echo "{}" ;;
  import)  echo "Error: 403 Forbidden" >&2; exit 1 ;;
esac'
  stub gh 'echo "[[{\"id\":18061003,\"name\":\"main-zero-bypass\"}]]"'
  run_import

  [ "${status}" -ne 0 ]
  [[ "${output}" == *"failed to import"* ]]
  [[ "${output}" == *"403 Forbidden"* ]]
}

@test "a repository already in state is skipped rather than re-imported" {
  stub tofu '
case "$1" in
  console) echo "\"{\\\"active\\\":[\\\"dear-agent\\\"],\\\"archived\\\":[]}\"" ;;
  show)    cat <<JSON
{"values":{"root_module":{"child_modules":[{"resources":[
  {"address":"module.managed_repos[\"dear-agent\"].github_repository.this","values":{}},
  {"address":"module.managed_repos[\"dear-agent\"].github_repository_ruleset.branch_protection",
   "values":{"repository":"dear-agent","id":"18061003"}}]}]}}}
JSON
  ;;
  import)  exit 0 ;;
esac'
  stub gh 'echo "[[{\"id\":18061003,\"name\":\"main-zero-bypass\"}]]"'
  run_import

  [ "${status}" -eq 0 ]
  [[ "${output}" == *"already imported and verified"* ]]
  # Only the Dependabot resource was missing from state.
  [ "$(grep -c 'tofu import' "${STUB_LOG}")" -eq 1 ]
  grep -qF 'github_repository_dependabot_security_updates.this' "${STUB_LOG}"
}
