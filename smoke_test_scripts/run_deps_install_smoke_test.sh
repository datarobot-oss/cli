#!/bin/bash
# Smoke tests for `dr dependency install`
#
# Each test creates an isolated temp directory containing a synthetic
# .datarobot/cli/versions.yaml so the CLI reads fake tool requirements
# rather than installing real tools. Two kinds of fake tools are used:
#
#   "outdated":  command "echo 1.0.0", minimum-version "999.0.0" → always WrongVersionTools
#   "satisfied": command "echo 99.0.0", minimum-version "1.0.0"  → always passes check
#
# Scenarios:
#   1. `dr dependency install --yes`                    skips prompt, runs install, exits 0
#   2. `dr dependency install -y`                       short flag alias, same
#   3. `DATAROBOT_CLI_NON_INTERACTIVE=true dr dependency install`
#                                                       env var bypasses prompt, exits 0
#   4. All tools satisfied → "already up to date"      no install triggered
#   5. `dr dependency check` exits 0 when all satisfied
#   6. User types "y" at interactive prompt             install proceeds, exits 0
#   7. User types "n" at interactive prompt             install declined, exits 0, nothing installed
#
# Usage:
#   DR_BIN=./dist/dr bash smoke_test_scripts/run_deps_install_smoke_test.sh
#   bash smoke_test_scripts/run_deps_install_smoke_test.sh   # uses dr on PATH

set -e

export TERM="dumb"

DR_BIN="${DR_BIN:-dr}"

# Resolve relative paths so DR_BIN still works after cd into temp dirs.
case "$DR_BIN" in
    /*) ;;
    */*) DR_BIN="$(pwd)/$DR_BIN" ;;
esac

# ──────────────────────────────────────────────────────────────
# Assertion helpers
# ──────────────────────────────────────────────────────────────

PASS_COUNT=0
FAIL_COUNT=0

assert_exit_zero() {
    local cmd_desc="$1"
    local exit_code="$2"

    if [ "$exit_code" -eq 0 ]; then
        echo "  ✅ assert_exit_zero: $cmd_desc"
    else
        echo "  ❌ assert_exit_zero FAILED: $cmd_desc exited $exit_code"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_output_contains() {
    local output="$1"
    local pattern="$2"
    local label="${3:-contains '$pattern'}"

    if echo "$output" | grep -qE "$pattern"; then
        echo "  ✅ assert_output_contains: $label"
    else
        echo "  ❌ assert_output_contains FAILED: $label"
        echo "     Expected to find: $pattern"
        echo "     In output: $output"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_output_not_contains() {
    local output="$1"
    local pattern="$2"
    local label="${3:-does not contain '$pattern'}"

    if ! echo "$output" | grep -qE "$pattern"; then
        echo "  ✅ assert_output_not_contains: $label"
    else
        echo "  ❌ assert_output_not_contains FAILED: $label"
        echo "     Did not expect to find: $pattern"
        echo "     In output: $output"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

# ──────────────────────────────────────────────────────────────
# Fake template helpers
# ──────────────────────────────────────────────────────────────

# make_outdated_template — creates a temp directory with a versions.yaml that
# always reports the fake tool as outdated (version 1.0.0 < minimum 999.0.0).
# The install command is a no-op echo so nothing is actually installed.
make_outdated_template() {
    local dir
    dir=$(mktemp -d /tmp/dr-deps-smoke.XXXXXX)
    mkdir -p "$dir/.datarobot/cli"
    cat > "$dir/.datarobot/cli/versions.yaml" << 'YAML'
fake-dep:
  name: FakeDep
  command: "echo 1.0.0"
  minimum-version: "999.0.0"
  url: "https://example.com"
  install:
    macos: "echo FakeDep installed"
    linux: "echo FakeDep installed"
    windows: "echo FakeDep installed"
YAML
    echo "$dir"
}

# make_satisfied_template — creates a temp directory with a versions.yaml where
# the fake tool always satisfies the version requirement (99.0.0 > minimum 1.0.0).
make_satisfied_template() {
    local dir
    dir=$(mktemp -d /tmp/dr-deps-smoke.XXXXXX)
    mkdir -p "$dir/.datarobot/cli"
    cat > "$dir/.datarobot/cli/versions.yaml" << 'YAML'
fake-dep:
  name: FakeDep
  command: "echo 99.0.0"
  minimum-version: "1.0.0"
  url: "https://example.com"
  install:
    macos: "echo FakeDep installed"
    linux: "echo FakeDep installed"
    windows: "echo FakeDep installed"
YAML
    echo "$dir"
}

# ──────────────────────────────────────────────────────────────
# Pre-flight check
# ──────────────────────────────────────────────────────────────

preflight_check() {
    if ! command -v "$DR_BIN" >/dev/null 2>&1 && [ ! -x "$DR_BIN" ]; then
        echo "ERROR: dr binary not found at '$DR_BIN'"
        echo "  Build first with: task build"
        echo "  Or set: DR_BIN=./dist/dr"
        exit 1
    fi

    echo "dr binary: $("$DR_BIN" --version 2>&1 | head -1)"
    echo ""
}

# ──────────────────────────────────────────────────────────────
# Test 1: --yes flag skips interactive prompt
# ──────────────────────────────────────────────────────────────

test_yes_flag() {
    local TEST_NAME="TEST 1: dr dependency install --yes skips prompt"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_outdated_template)

    local output exit_code=0
    output=$(cd "$dir" && "$DR_BIN" dependency install --yes 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install --yes" "$exit_code"
    assert_output_not_contains "$output" "Install now\? \(y/n\)" \
        "interactive prompt not shown with --yes"
    assert_output_contains "$output" "installed|All dependencies installed" \
        "install ran without prompt"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 2: -y short flag alias
# ──────────────────────────────────────────────────────────────

test_short_yes_flag() {
    local TEST_NAME="TEST 2: dr dependency install -y short flag alias"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_outdated_template)

    local output exit_code=0
    output=$(cd "$dir" && "$DR_BIN" dependency install -y 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install -y" "$exit_code"
    assert_output_not_contains "$output" "Install now\? \(y/n\)" \
        "interactive prompt not shown with -y"
    assert_output_contains "$output" "installed|All dependencies installed" \
        "install ran without prompt"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 3: DATAROBOT_CLI_NON_INTERACTIVE bypasses prompt
# ──────────────────────────────────────────────────────────────

test_non_interactive_env() {
    local TEST_NAME="TEST 3: DATAROBOT_CLI_NON_INTERACTIVE=true bypasses prompt"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_outdated_template)

    local output exit_code=0
    output=$(cd "$dir" && DATAROBOT_CLI_NON_INTERACTIVE=true "$DR_BIN" dependency install 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "DATAROBOT_CLI_NON_INTERACTIVE=true dr dependency install" "$exit_code"
    assert_output_not_contains "$output" "Install now\? \(y/n\)" \
        "interactive prompt not shown when DATAROBOT_CLI_NON_INTERACTIVE is set"
    assert_output_contains "$output" "installed|All dependencies installed" \
        "install ran without prompt"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 4: All tools satisfied → "already up to date" (no install)
# ──────────────────────────────────────────────────────────────

test_all_satisfied_no_install() {
    local TEST_NAME="TEST 4: all tools satisfied → already up to date, no install"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_satisfied_template)

    local output exit_code=0
    output=$(cd "$dir" && "$DR_BIN" dependency install --yes 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install --yes (satisfied)" "$exit_code"
    assert_output_contains "$output" "up to date" \
        "output says 'up to date' when all tools are satisfied"
    assert_output_not_contains "$output" "Installing" \
        "no installation triggered when all tools are satisfied"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 5: dr dependency check exits 0 when all satisfied
# ──────────────────────────────────────────────────────────────

test_check_satisfied() {
    local TEST_NAME="TEST 5: dr dependency check exits 0 when all satisfied"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_satisfied_template)

    local output exit_code=0
    output=$(cd "$dir" && "$DR_BIN" dependency check 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency check" "$exit_code"
    assert_output_contains "$output" "up to date" \
        "dependency check reports all satisfied"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 6: User types "y" at the interactive prompt → install proceeds
# ──────────────────────────────────────────────────────────────

test_interactive_user_y() {
    local TEST_NAME="TEST 6: interactive install confirmed with 'y' from user"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_outdated_template)

    local output exit_code=0
    output=$(cd "$dir" && printf "y\n" | "$DR_BIN" dependency install 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install (user answered y)" "$exit_code"
    assert_output_contains "$output" "Install now\? \(y/n\)" \
        "interactive prompt was displayed"
    assert_output_contains "$output" "installed|All dependencies installed" \
        "install ran after 'y' was entered"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 7: User types "n" at the interactive prompt → install cancelled
# ──────────────────────────────────────────────────────────────

test_interactive_user_n() {
    local TEST_NAME="TEST 7: interactive install declined with 'n' from user"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local dir initial_fails=$FAIL_COUNT
    dir=$(make_outdated_template)

    local output exit_code=0
    output=$(cd "$dir" && printf "n\n" | "$DR_BIN" dependency install 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install (user answered n)" "$exit_code"
    assert_output_contains "$output" "Install now\? \(y/n\)" \
        "interactive prompt was displayed before decline"
    assert_output_not_contains "$output" "📦 Installing" \
        "no installation runs when user declines"

    rm -rf "$dir"

    if [ "$FAIL_COUNT" -eq "$initial_fails" ]; then
        echo "✅ $TEST_NAME PASSED"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ $TEST_NAME FAILED"
    fi
}

# ──────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────

main() {
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║       dr dependency install — Smoke Tests                   ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo ""

    preflight_check

    test_yes_flag
    test_short_yes_flag
    test_non_interactive_env
    test_all_satisfied_no_install
    test_check_satisfied
    test_interactive_user_y
    test_interactive_user_n

    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "Results: $PASS_COUNT passed, $FAIL_COUNT failed"
    echo "═══════════════════════════════════════════════════════════════"

    if [ "$FAIL_COUNT" -gt 0 ]; then
        echo "❌ Some tests failed."
        exit 1
    else
        echo "✅ All dependency install smoke tests passed."
    fi
}

main "$@"
