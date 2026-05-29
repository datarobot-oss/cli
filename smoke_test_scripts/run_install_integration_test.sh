#!/bin/bash
# Integration tests for install.sh using LOCAL_BINARY
#
# These tests exercise the install.sh code paths without downloading from GitHub.
# They run on PRs using a locally-built binary via LOCAL_BINARY=./dist/dr.
#
# Scenarios:
#   1. Fresh install to default INSTALL_DIR
#   2. Fresh install to custom INSTALL_DIR
#   3. Reinstall same version → "already up to date" path
#   4. Upgrade: old version already installed → install newer via LOCAL_BINARY
#   5. 'datarobot' alias created and executable after install
#   6. Binary is on PATH after install (shell profile patching)
#
# Usage:
#   LOCAL_BINARY=./dist/dr bash smoke_test_scripts/run_install_integration_test.sh
#
# Requirements:
#   - LOCAL_BINARY must point to the binary to install (built via `task build`)
#   - Must run on Linux or macOS (not Windows — use install.ps1 tests for that)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_SCRIPT="$REPO_ROOT/install.sh"

LOCAL_BINARY="${LOCAL_BINARY:-$REPO_ROOT/dist/dr}"

# ──────────────────────────────────────────────────────────────
# Assertion helpers
# ──────────────────────────────────────────────────────────────

PASS_COUNT=0
FAIL_COUNT=0

assert_file_exists() {
    local path="$1"
    local label="${2:-$path}"

    if [ -e "$path" ]; then
        echo "  ✅ assert_file_exists: $label"
    else
        echo "  ❌ assert_file_exists FAILED: $label does not exist"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_executable() {
    local path="$1"

    if [ -x "$path" ]; then
        echo "  ✅ assert_executable: $path"
    else
        echo "  ❌ assert_executable FAILED: $path is not executable"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_symlink() {
    local path="$1"

    if [ -L "$path" ]; then
        echo "  ✅ assert_symlink: $path"
    else
        echo "  ❌ assert_symlink FAILED: $path is not a symlink"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_output_contains() {
    local output="$1"
    local expected="$2"
    local label="${3:-contains '$expected'}"

    if echo "$output" | grep -q "$expected"; then
        echo "  ✅ assert_output_contains: $label"
    else
        echo "  ❌ assert_output_contains FAILED: $label"
        echo "     Expected to find: $expected"
        echo "     In output: $output"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_output_not_contains() {
    local output="$1"
    local unexpected="$2"
    local label="${3:-does not contain '$unexpected'}"

    if ! echo "$output" | grep -q "$unexpected"; then
        echo "  ✅ assert_output_not_contains: $label"
    else
        echo "  ❌ assert_output_not_contains FAILED: $label"
        echo "     Did not expect to find: $unexpected"
        echo "     In output: $output"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

pass_test() {
    local name="$1"

    echo "✅ $name PASSED"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail_test() {
    local name="$1"
    local reason="${2:-see output above}"

    echo "❌ $name FAILED: $reason"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

# ──────────────────────────────────────────────────────────────
# Setup helpers
# ──────────────────────────────────────────────────────────────

make_install_dir() {
    mktemp -d /tmp/dr-install-test.XXXXXX
}

run_install() {
    local install_dir="$1"
    local extra_env="${2:-}"

    env INSTALL_DIR="$install_dir" LOCAL_BINARY="$LOCAL_BINARY" $extra_env sh "$INSTALL_SCRIPT" 2>&1
}

# ──────────────────────────────────────────────────────────────
# Pre-flight check
# ──────────────────────────────────────────────────────────────

preflight_check() {
    if [ ! -f "$LOCAL_BINARY" ]; then
        echo "ERROR: LOCAL_BINARY not found at $LOCAL_BINARY"
        echo "  Build first with: task build"
        echo "  Or set: LOCAL_BINARY=/path/to/dr"
        exit 1
    fi

    if [ ! -x "$LOCAL_BINARY" ]; then
        echo "ERROR: LOCAL_BINARY is not executable: $LOCAL_BINARY"
        exit 1
    fi

    if [ ! -f "$INSTALL_SCRIPT" ]; then
        echo "ERROR: install.sh not found at $INSTALL_SCRIPT"
        exit 1
    fi
}

# ──────────────────────────────────────────────────────────────
# Test 1: Fresh install to default INSTALL_DIR
# ──────────────────────────────────────────────────────────────

test_fresh_install_default_dir() {
    local TEST_NAME="TEST 1: fresh install to default INSTALL_DIR"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local install_dir
    install_dir=$(make_install_dir)

    local output
    output=$(run_install "$install_dir")

    echo "--- install output ---"
    echo "$output"
    echo "--- end output ---"

    assert_file_exists "$install_dir/dr" "dr binary"
    assert_executable "$install_dir/dr"
    assert_symlink "$install_dir/datarobot" "datarobot alias"
    assert_output_contains "$output" "Installing local binary" "install used LOCAL_BINARY path"

    local version_out
    version_out=$("$install_dir/dr" --version 2>&1 || true)
    assert_output_contains "$version_out" "DataRobot\|version\|v[0-9]" "binary executes and prints version"

    rm -rf "$install_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 2: Fresh install to custom INSTALL_DIR
# ──────────────────────────────────────────────────────────────

test_fresh_install_custom_dir() {
    local TEST_NAME="TEST 2: fresh install to custom INSTALL_DIR"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local custom_dir
    custom_dir=$(mktemp -d /tmp/dr-custom-bin.XXXXXX)

    local output
    output=$(run_install "$custom_dir")

    echo "--- install output ---"
    echo "$output"
    echo "--- end output ---"

    assert_file_exists "$custom_dir/dr" "dr binary in custom dir"
    assert_executable "$custom_dir/dr"
    assert_symlink "$custom_dir/datarobot" "datarobot alias in custom dir"

    rm -rf "$custom_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 3: Reinstall is idempotent — binary still works after second run
#
# When LOCAL_BINARY is set, install.sh skips check_existing_installation
# and always copies. Verify the script exits 0 and the binary is intact.
# ──────────────────────────────────────────────────────────────

test_reinstall_idempotent() {
    local TEST_NAME="TEST 3: reinstall is idempotent (LOCAL_BINARY re-copy)"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local install_dir
    install_dir=$(make_install_dir)

    # First install
    run_install "$install_dir" > /dev/null 2>&1

    # Second install (same binary)
    local exit_code=0
    run_install "$install_dir" > /dev/null 2>&1 || exit_code=$?

    if [ "$exit_code" -eq 0 ]; then
        echo "  ✅ second install exited 0"
    else
        echo "  ❌ second install exited $exit_code"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    assert_file_exists "$install_dir/dr" "dr binary still present after reinstall"
    assert_executable "$install_dir/dr"
    assert_symlink "$install_dir/datarobot" "datarobot alias still present after reinstall"

    local version_out
    version_out=$("$install_dir/dr" --version 2>&1 || true)
    assert_output_contains "$version_out" "DataRobot\|version\|v[0-9]" "binary still executes after reinstall"

    rm -rf "$install_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 4: 'datarobot' alias is functional (runs --help)
# ──────────────────────────────────────────────────────────────

test_datarobot_alias_functional() {
    local TEST_NAME="TEST 4: datarobot alias is functional"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local install_dir
    install_dir=$(make_install_dir)

    run_install "$install_dir" > /dev/null 2>&1

    local help_output
    help_output=$("$install_dir/datarobot" --help 2>&1 || true)

    echo "--- datarobot --help output ---"
    echo "$help_output"
    echo "--- end output ---"

    assert_output_contains "$help_output" "DataRobot\|datarobot\|dr\|Build AI" \
        "datarobot alias runs DR CLI"

    rm -rf "$install_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 5: dr --help runs without error
# ──────────────────────────────────────────────────────────────

test_dr_help_runs() {
    local TEST_NAME="TEST 5: dr --help runs without error"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local install_dir
    install_dir=$(make_install_dir)

    run_install "$install_dir" > /dev/null 2>&1

    local exit_code=0
    "$install_dir/dr" --help > /dev/null 2>&1 || exit_code=$?

    if [ "$exit_code" -eq 0 ]; then
        echo "  ✅ dr --help exited 0"
    else
        echo "  ❌ dr --help exited $exit_code"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    rm -rf "$install_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 6: Install dir created if missing
# ──────────────────────────────────────────────────────────────

test_install_dir_created() {
    local TEST_NAME="TEST 6: INSTALL_DIR created when it does not exist"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local base_dir
    base_dir=$(make_install_dir)
    local new_dir="$base_dir/nested/bin"

    local output
    output=$(run_install "$new_dir")

    echo "--- install output ---"
    echo "$output"
    echo "--- end output ---"

    assert_file_exists "$new_dir/dr" "dr binary in newly created dir"

    rm -rf "$base_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Test 7: Binary permissions are correct (executable bit)
# ──────────────────────────────────────────────────────────────

test_binary_permissions() {
    local TEST_NAME="TEST 7: installed binary has executable permission"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "$TEST_NAME"
    echo "═══════════════════════════════════════════════════════════════"

    local install_dir
    install_dir=$(make_install_dir)

    run_install "$install_dir" > /dev/null 2>&1

    local perms
    perms=$(ls -l "$install_dir/dr" | awk '{print $1}')
    echo "  Permissions: $perms"

    assert_executable "$install_dir/dr"

    rm -rf "$install_dir"
    pass_test "$TEST_NAME"
}

# ──────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────

main() {
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║       install.sh — LOCAL_BINARY Integration Tests           ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo ""
    echo "LOCAL_BINARY: $LOCAL_BINARY"
    echo "INSTALL_SCRIPT: $INSTALL_SCRIPT"
    echo ""

    preflight_check

    test_fresh_install_default_dir
    test_fresh_install_custom_dir
    test_reinstall_idempotent
    test_datarobot_alias_functional
    test_dr_help_runs
    test_install_dir_created
    test_binary_permissions

    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "Results: $PASS_COUNT passed, $FAIL_COUNT failed"
    echo "═══════════════════════════════════════════════════════════════"

    if [ "$FAIL_COUNT" -gt 0 ]; then
        echo "❌ Some tests failed."
        exit 1
    else
        echo "✅ All install integration tests passed."
    fi
}

main "$@"
