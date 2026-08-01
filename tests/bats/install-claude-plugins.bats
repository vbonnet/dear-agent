#!/usr/bin/env bats
# Tests for scripts/install-claude-plugins.sh
#
# These tests run the install script against a stub `claude` CLI that
# records every invocation to a log file. They exercise:
#   - prerequisite checks (claude missing, manifest missing)
#   - marketplace add vs. update flow (first run vs. re-run)
#   - dry-run mode (no side effects)
#   - per-plugin install vs. update flow
#   - --uninstall, --github, --scope, --help
#   - plugin enumeration matches independent canonical and hostile-fixture expectations

setup() {
    load '../test_helper/bats-support/load'
    load '../test_helper/bats-assert/load'
    load '../test_helper/bats-file/load'

    BATS_TEST_DIRNAME="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    PROJECT_ROOT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
    INSTALL_SCRIPT="$PROJECT_ROOT/scripts/install-claude-plugins.sh"

    TEST_DIR="$(mktemp -d)"
    export TEST_DIR

    # Stub claude CLI: logs all invocations and emulates basic state.
    STUB_DIR="$TEST_DIR/stubs"
    mkdir -p "$STUB_DIR"
    export CLAUDE_LOG="$TEST_DIR/claude.log"
    : >"$CLAUDE_LOG"
    export CLAUDE_MARKETPLACES="$TEST_DIR/marketplaces"  # space-separated list
    : >"$CLAUDE_MARKETPLACES"
    export CLAUDE_PLUGINS="$TEST_DIR/plugins"            # space-separated list of <plugin>@<market>
    : >"$CLAUDE_PLUGINS"

    cat >"$STUB_DIR/claude" <<'STUB'
#!/usr/bin/env bash
echo "$*" >>"$CLAUDE_LOG"
case "$1 $2" in
  "plugin marketplace")
    case "$3" in
      list)
        # Output a line "  ❯ <name>" for each known marketplace.
        while read -r name; do
          [ -n "$name" ] && printf '  \xe2\x9d\xaf %s\n' "$name"
        done <"$CLAUDE_MARKETPLACES"
        ;;
      add)
        # $4 is the source; we use the source path basename as marketplace name.
        # The real CLI parses the marketplace name from .claude-plugin/marketplace.json,
        # but for the stub we accept a hint via CLAUDE_STUB_ADD_NAME or default to "dear-agent".
        echo "${CLAUDE_STUB_ADD_NAME:-dear-agent}" >>"$CLAUDE_MARKETPLACES"
        echo "✔ added marketplace"
        ;;
      update)
        echo "✔ updated marketplace ${4:-all}"
        ;;
    esac
    ;;
  "plugin list")
    # Output a line "  ❯ <plugin>@<marketplace>" for each installed plugin.
    while read -r spec; do
      [ -n "$spec" ] && printf '  \xe2\x9d\xaf %s\n' "$spec"
    done <"$CLAUDE_PLUGINS"
    ;;
  "plugin install")
    if [[ -n "${CLAUDE_STUB_FAIL_INSTALL_PLUGIN:-}" && "$3" == "$CLAUDE_STUB_FAIL_INSTALL_PLUGIN" ]]; then
      echo "$3 install failed" >&2
      exit 44
    fi
    if [[ "${CLAUDE_STUB_FAIL_INSTALL_SPEC:-}" == "1" && "$3" == "spec-governance@dear-agent" ]]; then
      echo "spec-governance install failed" >&2
      exit 42
    fi
    echo "$3" >>"$CLAUDE_PLUGINS"
    echo "✔ installed $3"
    ;;
  "plugin update")
    if [[ -n "${CLAUDE_STUB_FAIL_UPDATE_PLUGIN:-}" && "$3" == "$CLAUDE_STUB_FAIL_UPDATE_PLUGIN" ]]; then
      echo "$3 update failed" >&2
      exit 45
    fi
    if [[ "${CLAUDE_STUB_FAIL_UPDATE_SPEC:-}" == "1" && "$3" == "spec-governance@dear-agent" ]]; then
      echo "spec-governance update failed" >&2
      exit 43
    fi
    echo "✔ updated $3"
    ;;
  "plugin uninstall")
    grep -vxF "$3" "$CLAUDE_PLUGINS" >"$CLAUDE_PLUGINS.tmp" || true
    mv "$CLAUDE_PLUGINS.tmp" "$CLAUDE_PLUGINS"
    echo "✔ uninstalled $3"
    ;;
  "plugin validate")
    echo "✔ Validation passed"
    ;;
esac
STUB
    chmod +x "$STUB_DIR/claude"
    export CLAUDE_BIN="$STUB_DIR/claude"

    # Provide a fresh repo root by default; some tests override this.
    export DEAR_AGENT_REPO="$PROJECT_ROOT"
}

teardown() {
    rm -rf "$TEST_DIR"
}

expected_plugin_names() {
    printf '%s\n' agm spec-governance wayfinder youtube
}

expected_plugin_specs() {
    printf '%s\n' agm@dear-agent spec-governance@dear-agent wayfinder@dear-agent youtube@dear-agent
}

write_marketplace_fixture() {
    local name="$1"
    local json="$2"
    local root="$TEST_DIR/$name"
    mkdir -p "$root/.claude-plugin"
    printf '%s\n' "$json" >"$root/.claude-plugin/marketplace.json"
    printf '%s\n' "$root"
}

# ----- structural ----------------------------------------------------------

@test "install-claude-plugins.sh exists and is executable" {
    assert_file_exists "$INSTALL_SCRIPT"
    assert_file_executable "$INSTALL_SCRIPT"
}

@test "install-claude-plugins.sh sets euo pipefail" {
    run grep -F "set -euo pipefail" "$INSTALL_SCRIPT"
    assert_success
}

@test "install-claude-plugins.sh passes bash syntax check" {
    run bash -n "$INSTALL_SCRIPT"
    assert_success
}

@test "--help prints usage and exits 0" {
    run "$INSTALL_SCRIPT" --help
    assert_success
    assert_output --partial "Usage:"
    assert_output --partial "--dry-run"
    assert_output --partial "--uninstall"
}

@test "unknown flag exits with code 2" {
    run "$INSTALL_SCRIPT" --bogus
    assert_failure 2
    assert_output --partial "unknown argument"
}

# ----- prerequisite checks -------------------------------------------------

@test "missing claude CLI fails with clear error" {
    CLAUDE_BIN="/nonexistent/claude" run "$INSTALL_SCRIPT" --dry-run
    assert_failure
    assert_output --partial "claude CLI not found"
}

@test "missing marketplace manifest fails with clear error" {
    DEAR_AGENT_REPO="$TEST_DIR" run "$INSTALL_SCRIPT" --dry-run
    assert_failure
    assert_output --partial "marketplace manifest not found"
}

# ----- enumeration ---------------------------------------------------------

@test "lists every plugin declared in marketplace.json" {
    run "$INSTALL_SCRIPT" --dry-run
    assert_success
    # Pluck the line that lists plugins.
    assert_output --partial "plugins:"
    # This expected set is intentionally independent from production parsing.
    for p in $(expected_plugin_names); do
        assert_output --partial "$p"
    done
}

@test "lists every current plugin across nested component arrays" {
    run "$INSTALL_SCRIPT" --dry-run
    assert_success
    assert_output --partial "agm"
    assert_output --partial "spec-governance"
    assert_output --partial "wayfinder"
    assert_output --partial "youtube"
}

@test "JSON string braces and nested arrays cannot hide later plugins" {
    fixture_repo="$TEST_DIR/brace-marketplace"
    mkdir -p "$fixture_repo/.claude-plugin"
    cp -f "$PROJECT_ROOT/tests/bats/claude-plugin-marketplace-braces.fixture.json" "$fixture_repo/.claude-plugin/marketplace.json"

    DEAR_AGENT_REPO="$fixture_repo" CLAUDE_STUB_ADD_NAME="brace-market" run "$INSTALL_SCRIPT"
    assert_success
    assert_output --partial "marketplace: brace-market"
    assert_output --partial "plugins:     alpha beta gamma"
    assert_equal "$(grep -c '^plugin install ' "$CLAUDE_LOG")" "3"
    for spec in alpha@brace-market beta@brace-market gamma@brace-market; do
        run grep -Fx "plugin install $spec" "$CLAUDE_LOG"
        assert_success
    done
    run grep -F "nested-component@brace-market" "$CLAUDE_LOG"
    assert_failure
}

@test "invalid or ambiguous root marketplace names fail before plugin actions" {
    local fixture_repo
    for entry in \
        'duplicate|{"name":"first","name":"second","plugins":[{"name":"alpha"}]}' \
        'empty|{"name":"","plugins":[{"name":"alpha"}]}' \
        'escaped|{"name":"brace\u002dmarket","plugins":[{"name":"alpha"}]}' \
        'malformed|{"name":"brace-market","plugins":[{"name":"alpha"}]'; do
        fixture_repo="$(write_marketplace_fixture "root-${entry%%|*}" "${entry#*|}")"
        DEAR_AGENT_REPO="$fixture_repo" run "$INSTALL_SCRIPT"
        assert_failure
        assert_output --partial "could not parse an unambiguous marketplace name"
        run grep -E '^plugin (install|update) |^plugin marketplace (add|update) ' "$CLAUDE_LOG"
        assert_failure
    done
}

@test "invalid or duplicate direct plugin names fail before plugin actions" {
    local fixture_repo
    for entry in \
        'empty|{"name":"brace-market","plugins":[{"name":""}]}' \
        'duplicate-field|{"name":"brace-market","plugins":[{"name":"alpha","name":"beta"}]}' \
        'duplicate-value|{"name":"brace-market","plugins":[{"name":"alpha"},{"name":"alpha"}]}' \
        'escaped|{"name":"brace-market","plugins":[{"name":"alpha\u002dplugin"}]}' \
        'missing|{"name":"brace-market","plugins":[{"description":"no owner"}]}'; do
        fixture_repo="$(write_marketplace_fixture "plugin-${entry%%|*}" "${entry#*|}")"
        DEAR_AGENT_REPO="$fixture_repo" run "$INSTALL_SCRIPT"
        assert_failure
        assert_output --partial "could not parse an unambiguous marketplace name"
        run grep -E '^plugin (install|update) |^plugin marketplace (add|update) ' "$CLAUDE_LOG"
        assert_failure
    done
}

@test "MARKETPLACE_NAME overrides a valid parsed root name" {
    fixture_repo="$(write_marketplace_fixture override-marketplace '{"name":"native-market","plugins":[{"name":"alpha"}]}')"
    DEAR_AGENT_REPO="$fixture_repo" MARKETPLACE_NAME="override-market" CLAUDE_STUB_ADD_NAME="override-market" run "$INSTALL_SCRIPT"
    assert_success
    assert_output --partial "marketplace: override-market"
    run grep -Fx "plugin install alpha@override-market" "$CLAUDE_LOG"
    assert_success
}

# ----- marketplace add vs update -------------------------------------------

@test "first install: registers marketplace via 'marketplace add'" {
    : >"$CLAUDE_MARKETPLACES"  # no marketplaces known yet
    run "$INSTALL_SCRIPT"
    assert_success
    run grep -F "plugin marketplace add" "$CLAUDE_LOG"
    assert_success
    refute_output --partial "marketplace update"
}

@test "re-run: refreshes via 'marketplace update' instead of add" {
    echo "dear-agent" >"$CLAUDE_MARKETPLACES"
    run "$INSTALL_SCRIPT"
    assert_success
    run grep -F "plugin marketplace update dear-agent" "$CLAUDE_LOG"
    assert_success
    run grep -F "plugin marketplace add" "$CLAUDE_LOG"
    assert_failure
}

@test "--github passes the GitHub coordinate to marketplace add" {
    : >"$CLAUDE_MARKETPLACES"
    DEAR_AGENT_GH_REPO="example-org/dear-agent" run "$INSTALL_SCRIPT" --github
    assert_success
    run grep -F "plugin marketplace add example-org/dear-agent" "$CLAUDE_LOG"
    assert_success
}

@test "--local (default) passes the repo path to marketplace add" {
    : >"$CLAUDE_MARKETPLACES"
    run "$INSTALL_SCRIPT" --local
    assert_success
    run grep -F "plugin marketplace add $PROJECT_ROOT" "$CLAUDE_LOG"
    assert_success
}

# ----- per-plugin install vs update ----------------------------------------

@test "first install: calls 'plugin install' once per declared plugin" {
    : >"$CLAUDE_PLUGINS"
    run "$INSTALL_SCRIPT"
    assert_success
    assert_equal "$(grep -c "^plugin install " "$CLAUDE_LOG")" "4"
    for p in $(expected_plugin_names); do
        run grep -F "plugin install $p@dear-agent" "$CLAUDE_LOG"
        assert_success
    done
}

@test "already-installed plugin uses 'plugin update' instead of install" {
    expected_plugin_specs >"$CLAUDE_PLUGINS"
    run "$INSTALL_SCRIPT"
    assert_success
    assert_equal "$(grep -c "^plugin install " "$CLAUDE_LOG")" "0"
    assert_equal "$(grep -c "^plugin update " "$CLAUDE_LOG")" "4"
}

@test "spec-governance update failure stops without a false success message" {
    expected_plugin_specs >"$CLAUDE_PLUGINS"
    export CLAUDE_STUB_FAIL_UPDATE_SPEC=1
    run "$INSTALL_SCRIPT"
    assert_failure
    assert_output --partial "spec-governance update failed"
    refute_output --partial "✔ done."
}

@test "spec-governance install failure stops without a false success message" {
    : >"$CLAUDE_PLUGINS"
    export CLAUDE_STUB_FAIL_INSTALL_SPEC=1
    run "$INSTALL_SCRIPT"
    assert_failure
    assert_output --partial "spec-governance install failed"
    refute_output --partial "✔ done."
}

@test "non-governance update failure stops without a false success message" {
    expected_plugin_specs >"$CLAUDE_PLUGINS"
    export CLAUDE_STUB_FAIL_UPDATE_PLUGIN="agm@dear-agent"
    run "$INSTALL_SCRIPT"
    assert_failure
    assert_output --partial "update agm@dear-agent failed"
    refute_output --partial "✔ done."
}

@test "non-governance install failure stops without a false success message" {
    : >"$CLAUDE_PLUGINS"
    export CLAUDE_STUB_FAIL_INSTALL_PLUGIN="agm@dear-agent"
    run "$INSTALL_SCRIPT"
    assert_failure
    assert_output --partial "install agm@dear-agent failed"
    refute_output --partial "✔ done."
}

@test "--scope user is forwarded to plugin install" {
    : >"$CLAUDE_PLUGINS"
    run "$INSTALL_SCRIPT" --scope user
    assert_success
    run grep -F "plugin install agm@dear-agent --scope user" "$CLAUDE_LOG"
    assert_success
}

@test "--uninstall removes every declared plugin" {
    expected_plugin_specs >"$CLAUDE_PLUGINS"
    run "$INSTALL_SCRIPT" --uninstall
    assert_success
    for p in $(expected_plugin_names); do
        run grep -F "plugin uninstall $p@dear-agent" "$CLAUDE_LOG"
        assert_success
    done
}

# ----- dry-run -------------------------------------------------------------

@test "--dry-run does not invoke claude for mutating actions" {
    : >"$CLAUDE_MARKETPLACES"
    : >"$CLAUDE_PLUGINS"
    run "$INSTALL_SCRIPT" --dry-run
    assert_success
    # The dry-run output should preview the mutating actions with a '+ ' prefix.
    assert_output --partial "+ "
    # plugin marketplace list / plugin list are reads, still allowed.
    # plugin install / update / add / uninstall should NOT have been called.
    run grep -E "^plugin (install|update|uninstall) |^plugin marketplace (add|update) " "$CLAUDE_LOG"
    assert_failure
}

# ----- idempotency ---------------------------------------------------------

@test "running install twice in a row succeeds both times" {
    : >"$CLAUDE_MARKETPLACES"
    : >"$CLAUDE_PLUGINS"
    run "$INSTALL_SCRIPT"
    assert_success
    run "$INSTALL_SCRIPT"
    assert_success
}
