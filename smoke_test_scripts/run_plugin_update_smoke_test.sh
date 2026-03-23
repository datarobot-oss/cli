#!/bin/bash
# Smoke tests for the plugin auto-update-check flow.
#
# Scenario:
#   1. Install the `assist` plugin at a pinned old version (0.1.15)
#   2. Run `dr assist` → auto-update prompt appears → user declines
#   3. Assert plugin is still on the pinned version
#   4. Assert state file was written (cooldown is now active)
#   5. Delete the state file to reset the cooldown
#   6. Run `dr assist` again → auto-update prompt appears → user accepts
#   7. Assert plugin was updated to a newer version
#
# Requirements:
#   - `dr` binary must be in PATH (run `task build` first)
#   - Internet access to cli.datarobot.com
#   - `expect` installed (brew install expect / apt install expect)
#   - `python3` available for JSON parsing

export TERM="dumb"

# Ensure we use the locally-built binary, not whatever `dr` is in PATH
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
export PATH="$PROJECT_ROOT/dist:$PATH"

PLUGIN_NAME="assist"
PINNED_VERSION="0.1.15"

# ── Isolated sandbox (never touches ~/.config/datarobot) ──────────────────────
SANDBOX_DIR="$(mktemp -d)"
export XDG_CONFIG_HOME="$SANDBOX_DIR"

DR_CONFIG_DIR="$SANDBOX_DIR/datarobot"
STATE_FILE="$DR_CONFIG_DIR/state.yaml"
INSTALLED_META="$DR_CONFIG_DIR/plugins/$PLUGIN_NAME/.installed.json"

cleanup() {
  # Remove only after unsetting so DR_CONFIG_DIR is no longer live
  unset XDG_CONFIG_HOME
  rm -rf "$SANDBOX_DIR"
  echo "ℹ️  Sandbox removed: $SANDBOX_DIR"
}
trap cleanup EXIT

echo "🔌 Plugin update check smoke tests"
echo "   Sandbox : $SANDBOX_DIR"
echo "   Plugin  : $PLUGIN_NAME"
echo "   Pinned  : v$PINNED_VERSION"
echo ""

# ── Prerequisite: internet access ─────────────────────────────────────────────
if ! curl -sf --head "https://cli.datarobot.com" -o /dev/null 2>/dev/null; then
  echo "ℹ️  cli.datarobot.com is not reachable — skipping plugin update smoke tests"
  exit 0
fi

# ── Helper: read .version from an .installed.json file ───────────────────────
read_version() {
  python3 -c "import json,sys; print(json.load(open('$1'))['version'])"
}

assert_eq() {
  local label="$1" actual="$2" expected="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo "✅ $label: $actual"
  else
    echo "❌ $label: expected '$expected', got '$actual'"
    exit 1
  fi
}

assert_ne() {
  local label="$1" actual="$2" unexpected="$3"
  if [[ "$actual" != "$unexpected" ]]; then
    echo "✅ $label: $actual"
  else
    echo "❌ $label: still '$actual' — expected a different value"
    exit 1
  fi
}

# ── Step 1: Install pinned version ────────────────────────────────────────────
echo "── Step 1: Install $PLUGIN_NAME v$PINNED_VERSION ──"
if ! dr plugin install "$PLUGIN_NAME" --version "$PINNED_VERSION"; then
  echo "❌ Installation failed"
  exit 1
fi

assert_eq "Installed version" "$(read_version "$INSTALLED_META")" "$PINNED_VERSION"
echo ""

# ── Step 2: Run plugin → see prompt → decline ─────────────────────────────────
echo "── Step 2: Run $PLUGIN_NAME, decline update ──"
expect "$SCRIPT_DIR/expect_plugin_update_decline.exp" "$PLUGIN_NAME"

assert_eq "Version after decline" "$(read_version "$INSTALLED_META")" "$PINNED_VERSION"
echo ""

# ── Step 3: Verify cooldown was recorded ──────────────────────────────────────
echo "── Step 3: Verify state file (cooldown) ──"
if [[ -f "$STATE_FILE" ]]; then
  echo "✅ State file written at $STATE_FILE"
else
  echo "❌ State file not found — cooldown was not recorded"
  exit 1
fi
echo ""

# ── Step 4: Reset cooldown by deleting state file ─────────────────────────────
echo "── Step 4: Clear state file to reset cooldown ──"
rm "$STATE_FILE"
echo "✅ Cooldown reset"
echo ""

# ── Step 5: Run plugin → see prompt → accept update ──────────────────────────
echo "── Step 5: Run $PLUGIN_NAME, accept update ──"
expect "$SCRIPT_DIR/expect_plugin_update_accept.exp" "$PLUGIN_NAME"
echo ""

# ── Step 6: Verify version changed ────────────────────────────────────────────
echo "── Step 6: Verify updated version ──"
UPDATED_VERSION="$(read_version "$INSTALLED_META")"
assert_ne "Version after update" "$UPDATED_VERSION" "$PINNED_VERSION"
echo "   Updated: v$PINNED_VERSION → v$UPDATED_VERSION"
echo ""

echo "✅ All plugin update smoke tests passed!"
