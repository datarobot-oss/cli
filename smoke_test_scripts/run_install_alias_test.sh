#!/bin/bash
# Regression test for the 'datarobot' alias logic in install.sh (CFX-6634).
#
# In DataRobot Codespaces `dr` is installed as a *directory* on PATH, with the
# binary at dr/dr. A prior bug used `ln -sf dr datarobot`; on the second
# `dr self update` the existing `datarobot -> dr` symlink (to the directory) was
# followed and the new link written *inside* it, turning dr/dr into a
# self-referential `dr -> dr` symlink ("too many levels of symbolic links").
#
# These tests source install.sh (without running the installer) and exercise
# ensure_datarobot_alias directly, asserting it never creates a self-loop.
#
# Usage:
#   bash smoke_test_scripts/run_install_alias_test.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_SCRIPT="$REPO_ROOT/install.sh"

PASS_COUNT=0
FAIL_COUNT=0

assert() {
    local label="$1"

    if [ "$2" -eq 0 ]; then
        echo "  ✅ $label"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "  ❌ $label"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

# Source install.sh function definitions without executing main.
# shellcheck disable=SC1090
DR_INSTALL_SH_NO_MAIN=1 . "$INSTALL_SCRIPT"

BINARY_NAME="dr"

# ──────────────────────────────────────────────────────────────
# Test 1: Codespace directory layout — twice — never self-links dr/dr
# ──────────────────────────────────────────────────────────────
test_codespace_layout_no_self_link() {
    echo ""
    echo "TEST 1: Codespace dir layout (dr/dr) survives repeated alias creation"

    INSTALL_DIR=$(mktemp -d /tmp/dr-alias-test.XXXXXX)
    mkdir "$INSTALL_DIR/dr"
    printf '#!/bin/sh\necho dr\n' > "$INSTALL_DIR/dr/dr"
    chmod +x "$INSTALL_DIR/dr/dr"

    # Run-1 then run-2 effect.
    ensure_datarobot_alias
    ensure_datarobot_alias

    [ -f "$INSTALL_DIR/dr/dr" ] && [ ! -L "$INSTALL_DIR/dr/dr" ]
    assert "dr/dr is still a real (non-symlink) file" $?

    [ -L "$INSTALL_DIR/datarobot" ]
    assert "datarobot is a symlink" $?

    "$INSTALL_DIR/dr/dr" >/dev/null 2>&1
    assert "dr/dr still executes (no 'too many levels of symbolic links')" $?

    rm -rf "$INSTALL_DIR"
}

# ──────────────────────────────────────────────────────────────
# Test 2: Flat layout — alias points at the dr binary, idempotent
# ──────────────────────────────────────────────────────────────
test_flat_layout() {
    echo ""
    echo "TEST 2: flat layout (dr file) — datarobot -> dr, idempotent"

    INSTALL_DIR=$(mktemp -d /tmp/dr-alias-test.XXXXXX)
    printf '#!/bin/sh\necho dr\n' > "$INSTALL_DIR/dr"
    chmod +x "$INSTALL_DIR/dr"

    ensure_datarobot_alias
    ensure_datarobot_alias

    [ -L "$INSTALL_DIR/datarobot" ] && [ "$(readlink "$INSTALL_DIR/datarobot")" = "dr" ]
    assert "datarobot is a symlink -> dr" $?

    "$INSTALL_DIR/datarobot" >/dev/null 2>&1
    assert "datarobot alias executes the binary" $?

    rm -rf "$INSTALL_DIR"
}

main() {
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║   install.sh — datarobot alias regression tests          ║"
    echo "╚══════════════════════════════════════════════════════════╝"

    test_codespace_layout_no_self_link
    test_flat_layout

    echo ""
    echo "Results: $PASS_COUNT passed, $FAIL_COUNT failed"

    if [ "$FAIL_COUNT" -gt 0 ]; then
        echo "❌ Some tests failed."
        exit 1
    fi

    echo "✅ All alias regression tests passed."
}

main "$@"
