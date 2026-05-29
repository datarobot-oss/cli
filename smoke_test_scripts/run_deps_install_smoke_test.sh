#!/bin/bash
# Smoke tests for `dr dependency install`
#
# Scenarios:
#   1. `dr dependency install --yes`           skips prompt, exits 0
#   2. `dr dependency install -y`              short flag alias, exits 0
#   3. `DATAROBOT_CLI_NON_INTERACTIVE=true dr dependency install`
#                                              env var bypasses prompt, exits 0
#   4. All tools present → "already up to date" path (no install triggered)
#   5. `dr dependency check` exits 0 after install
#   6. User types "y" at interactive prompt    → install proceeds, exits 0
#   7. User types "n" at interactive prompt    → install declined, exits 0, nothing installed
#
# Before each test the state of python3/uv/pulumi/task is snapshotted.
# After each test any tool that was absent beforehand is uninstalled so the
# next test starts from the same baseline.
#
# Usage:
#   DR_BIN=./dist/dr bash smoke_test_scripts/run_deps_install_smoke_test.sh
#   bash smoke_test_scripts/run_deps_install_smoke_test.sh   # uses dr on PATH

set -e

export TERM="dumb"

DR_BIN="${DR_BIN:-dr}"

TRACKED_TOOLS="python3 uv pulumi task"

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

    if echo "$output" | grep -q "$pattern"; then
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

    if ! echo "$output" | grep -q "$pattern"; then
        echo "  ✅ assert_output_not_contains: $label"
    else
        echo "  ❌ assert_output_not_contains FAILED: $label"
        echo "     Did not expect to find: $pattern"
        echo "     In output: $output"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

pass_test() {
    echo "✅ $1 PASSED"
    PASS_COUNT=$((PASS_COUNT + 1))
}

# ──────────────────────────────────────────────────────────────
# State snapshot & cleanup
# ──────────────────────────────────────────────────────────────

# Current snapshot file path — set by snapshot_tool_state, consumed by restore_tool_state.
_SNAPSHOT_FILE=""

# snapshot_tool_state — call at the top of each test.
# Writes one line per tracked tool:  <tool>:<status>:<path>
#   status = "installed" | "not_installed"
#   path   = absolute path from `command -v`, or "-" when not installed
# Also prints the state so CI logs show the baseline at test entry.
snapshot_tool_state() {
    _SNAPSHOT_FILE=$(mktemp /tmp/dr-deps-snapshot.XXXXXX)

    echo "  --- tool state before test ---"

    local tool

    for tool in $TRACKED_TOOLS; do
        local status path ver

        if command -v "$tool" >/dev/null 2>&1; then
            path=$(command -v "$tool")
            status="installed"
            case "$tool" in
                python3) ver=$(python3 --version 2>&1 | head -1) ;;
                uv)      ver=$(uv --version 2>&1 | head -1) ;;
                pulumi)  ver=$(pulumi version 2>&1 | head -1) ;;
                task)    ver=$(task --version 2>&1 | head -1) ;;
                *)       ver="unknown" ;;
            esac
            echo "    $tool: $ver  ($path)"
        else
            path="-"
            status="not_installed"
            echo "    $tool: not installed"
        fi

        echo "${tool}:${status}:${path}" >> "$_SNAPSHOT_FILE"
    done

    echo "  ------------------------------"
}

# uninstall_tool — best-effort removal of a single tool.
# Uses the reverse of the platform install commands in tools/prerequisites.go.
# Errors are suppressed so a failed uninstall never aborts the test suite.
uninstall_tool() {
    local tool="$1"

    echo "  🧹 cleanup: removing $tool..."

    case "$(uname -s)" in
        Darwin)
            case "$tool" in
                python3) brew uninstall python 2>/dev/null || true ;;
                uv)      brew uninstall uv 2>/dev/null || true ;;
                pulumi)  brew uninstall pulumi 2>/dev/null || true ;;
                task)    brew uninstall go-task/tap/go-task 2>/dev/null || true ;;
            esac
            ;;
        Linux)
            case "$tool" in
                uv)
                    # astral.sh installer puts binaries in ~/.local/bin
                    rm -f "$HOME/.local/bin/uv" "$HOME/.local/bin/uvx" 2>/dev/null || true
                    ;;
                pulumi)
                    # get.pulumi.com installer creates ~/.pulumi
                    rm -rf "$HOME/.pulumi" 2>/dev/null || true
                    local bin_path
                    bin_path=$(command -v pulumi 2>/dev/null || true)
                    [ -n "$bin_path" ] && rm -f "$bin_path" 2>/dev/null || true
                    ;;
                task)
                    # taskfile.dev installer puts task on PATH; remove it
                    local bin_path
                    bin_path=$(command -v task 2>/dev/null || true)
                    [ -n "$bin_path" ] && rm -f "$bin_path" 2>/dev/null || true
                    ;;
                python3)
                    # Do not remove python3 on Linux — system may depend on it
                    echo "  ⚠️  cleanup: skipping python3 removal on Linux (system dependency)"
                    ;;
            esac
            ;;
    esac

    echo "  🧹 cleanup: $tool removed"
}

# restore_tool_state — call at the end of each test (including on failure).
# Compares current tool presence against the snapshot taken at test start.
# Any tool that was absent before the test but present now is uninstalled.
restore_tool_state() {
    if [ -z "$_SNAPSHOT_FILE" ] || [ ! -f "$_SNAPSHOT_FILE" ]; then
        return 0
    fi

    local changed=0
    local tool

    for tool in $TRACKED_TOOLS; do
        local was_installed
        was_installed=$(grep "^${tool}:" "$_SNAPSHOT_FILE" | cut -d: -f2)

        if [ "$was_installed" = "not_installed" ] && command -v "$tool" >/dev/null 2>&1; then
            changed=1
            uninstall_tool "$tool"
        fi
    done

    if [ "$changed" -eq 0 ]; then
        echo "  🧹 cleanup: no new tools to remove"
    fi

    rm -f "$_SNAPSHOT_FILE"
    _SNAPSHOT_FILE=""
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

    snapshot_tool_state

    local output
    local exit_code=0
    output=$("$DR_BIN" dependency install --yes 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install --yes" "$exit_code"

    assert_output_not_contains "$output" "Install now? (y/n)" \
        "interactive prompt is not shown when --yes is set"

    assert_output_contains "$output" "up to date\|installed\|Installing" \
        "output indicates install outcome"

    restore_tool_state
    pass_test "$TEST_NAME"
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

    snapshot_tool_state

    local output
    local exit_code=0
    output=$("$DR_BIN" dependency install -y 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install -y" "$exit_code"

    assert_output_not_contains "$output" "Install now? (y/n)" \
        "interactive prompt is not shown when -y is set"

    assert_output_contains "$output" "up to date\|installed\|Installing" \
        "output indicates install outcome"

    restore_tool_state
    pass_test "$TEST_NAME"
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

    snapshot_tool_state

    local output
    local exit_code=0
    output=$(DATAROBOT_CLI_NON_INTERACTIVE=true "$DR_BIN" dependency install 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "DATAROBOT_CLI_NON_INTERACTIVE=true dr dependency install" "$exit_code"

    assert_output_not_contains "$output" "Install now? (y/n)" \
        "interactive prompt is not shown when DATAROBOT_CLI_NON_INTERACTIVE is set"

    assert_output_contains "$output" "up to date\|installed\|Installing" \
        "output indicates install outcome"

    restore_tool_state
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 4: All tools present → "already up to date" (no install)
#
# Skips if any tool is absent — "up to date" only makes sense when all present.
# ──────────────────────────────────────────────────────────────

test_all_satisfied_no_install() {
    local TEST_NAME="TEST 4: all tools present → already up to date, no install"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    snapshot_tool_state

    # Ensure all tools are installed before asserting the "up to date" path.
    # Snapshot was taken above, so restore_tool_state will clean up anything
    # installed here together with anything installed by the assertion run.
    "$DR_BIN" dependency install --yes >/dev/null 2>&1 || true

    local output
    local exit_code=0
    output=$("$DR_BIN" dependency install --yes 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install --yes (all present)" "$exit_code"

    assert_output_contains "$output" "up to date" \
        "output says 'up to date' when all tools are satisfied"

    assert_output_not_contains "$output" "Installing" \
        "no installation is triggered when all tools are satisfied"

    restore_tool_state
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 5: dr dependency check agrees with install outcome
# ──────────────────────────────────────────────────────────────

test_check_after_install() {
    local TEST_NAME="TEST 5: dr dependency check succeeds after install"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    snapshot_tool_state

    # Ensure tools are installed before running check
    "$DR_BIN" dependency install --yes >/dev/null 2>&1 || true

    local output
    local exit_code=0
    output=$("$DR_BIN" dependency check 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency check" "$exit_code"

    assert_output_contains "$output" "up to date" \
        "dependency check reports all satisfied after install"

    restore_tool_state
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 6: User types "y" at the interactive prompt → install proceeds
#
# Pipes "y\n" to stdin so helpers.Confirm reads it and returns true.
# When all tools are already present the prompt is never shown (early exit),
# so this test is valid in both cases: tools present → "up to date", exit 0;
# tools missing → prompt shown, "y" consumed, install runs, exit 0.
# ──────────────────────────────────────────────────────────────

test_interactive_user_y() {
    local TEST_NAME="TEST 6: interactive install confirmed with 'y' from user"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    snapshot_tool_state

    local output
    local exit_code=0
    output=$(printf "y\n" | "$DR_BIN" dependency install 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install (user answered y)" "$exit_code"

    assert_output_contains "$output" "up to date\|installed\|Installing" \
        "output indicates install outcome (install ran or already satisfied)"

    # When tools were missing the prompt must have appeared and been answered.
    # Skip this sub-assertion when the "up to date" early-exit path was taken.
    if ! echo "$output" | grep -q "up to date"; then
        assert_output_contains "$output" "Install now? (y/n)" \
            "interactive prompt was displayed before install"
    fi

    restore_tool_state
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 7: User types "n" at the interactive prompt → install cancelled
#
# Pipes "n\n" to stdin so helpers.Confirm reads it and returns false.
# Command must exit 0 (decline is not an error) and must not install anything.
# When all tools are already present the prompt is skipped entirely ("up to date").
# ──────────────────────────────────────────────────────────────

test_interactive_user_n() {
    local TEST_NAME="TEST 7: interactive install declined with 'n' from user"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    snapshot_tool_state

    local output
    local exit_code=0
    output=$(printf "n\n" | "$DR_BIN" dependency install 2>&1) || exit_code=$?

    echo "--- output ---"
    echo "$output"
    echo "--- end output ---"

    assert_exit_zero "dr dependency install (user answered n)" "$exit_code"

    # Decline must never trigger an installation
    assert_output_not_contains "$output" "📦 Installing" \
        "no installation runs when user declines"

    # When tools were missing, verify the prompt was shown before the decline
    if ! echo "$output" | grep -q "up to date"; then
        assert_output_contains "$output" "Install now? (y/n)" \
            "interactive prompt was displayed before decline"
    fi

    restore_tool_state
    pass_test "$TEST_NAME"
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
    test_check_after_install
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
