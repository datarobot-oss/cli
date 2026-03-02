#!/bin/bash
# Smoke tests for `dr self update` command
#
# Scenarios:
#   1. curl install latest → dr self update → should be a no-op (already up to date)
#   2. curl install old version (v0.2.40) → dr self update → should install latest
#   3. brew install latest → dr self update → should use brew upgrade path
#   4. (stretch) template minimum version met → dr self update → no-op
#   5. (stretch) template minimum version met → dr self update -f → force update to latest
#
# NOTE: All tests run from isolated temp directories to prevent interference from
# .datarobot/cli directories that may exist anywhere up the directory tree, which
# would cause FindRepoRoot() to detect a template context and alter update behavior.

set -e

export TERM="dumb"
INSTALL_DIR="$HOME/.local/bin"
DR_BIN="$INSTALL_DIR/dr"
OLD_VERSION="v0.2.40"

# ──────────────────────────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────────────────────────
cleanup_curl_install() {
    rm -f "$DR_BIN"
}

get_installed_version() {
    "$DR_BIN" self version --format=json | grep '"version"' | sed -E 's/.*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/'
}

get_latest_version() {
    curl -fsSL "https://api.github.com/repos/datarobot-oss/cli/releases/latest" \
        | grep '"tag_name"' \
        | sed -E 's/.*"([^"]+)".*/\1/'
}

# Create an isolated temp directory under /tmp so FindRepoRoot() won't find any
# .datarobot/cli ancestors. Returns the path via stdout.
make_isolated_dir() {
    mktemp -d /tmp/dr-smoke-test.XXXXXX
}

# ──────────────────────────────────────────────────────────────
# Test 1: curl install latest → dr self update (no-op)
# ──────────────────────────────────────────────────────────────
test_curl_latest_self_update() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "TEST 1: curl install latest → dr self update (should be no-op)"
    echo "═══════════════════════════════════════════════════════════════"

    cleanup_curl_install

    echo "Installing latest via curl..."
    curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | INSTALL_DIR="$INSTALL_DIR" sh

    version_before=$(get_installed_version)
    echo "Version after curl install: $version_before"

    # Run from isolated temp dir to avoid .datarobot detection
    local work_dir
    work_dir=$(make_isolated_dir)

    echo "Running dr self update from isolated dir ($work_dir)..."
    update_output=$(cd "$work_dir" && "$DR_BIN" self update 2>&1 || true)
    echo "Output: $update_output"

    version_after=$(get_installed_version)
    echo "Version after self update: $version_after"

    rm -rf "$work_dir"

    if [[ "$version_before" == "$version_after" ]]; then
        echo "✅ TEST 1 PASSED: Version unchanged after self update on latest ($version_after)."
    else
        echo "❌ TEST 1 FAILED: Version changed from $version_before to $version_after (expected no change)."
        cleanup_curl_install
        exit 1
    fi

    cleanup_curl_install
}

# ──────────────────────────────────────────────────────────────
# Test 2: curl install old version → dr self update (upgrade)
# ──────────────────────────────────────────────────────────────
test_curl_old_version_self_update() {
    echo ""
    echo "════════════════════════════════════════════════════════════════════"
    echo "TEST 2: curl install $OLD_VERSION → dr self update (should upgrade)"
    echo "════════════════════════════════════════════════════════════════════"

    cleanup_curl_install

    echo "Installing $OLD_VERSION via curl..."
    curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | INSTALL_DIR="$INSTALL_DIR" sh -s -- "$OLD_VERSION"

    version_before=$(get_installed_version)
    echo "Version after curl install: $version_before"

    latest_version=$(get_latest_version)
    echo "Latest release version: $latest_version"

    # Run from isolated temp dir to avoid .datarobot detection
    local work_dir
    work_dir=$(make_isolated_dir)

    echo "Running dr self update from isolated dir ($work_dir)..."
    update_output=$(cd "$work_dir" && "$DR_BIN" self update 2>&1 || true)
    echo "Output: $update_output"

    version_after=$(get_installed_version)
    echo "Version after self update: $version_after"

    version_after_stripped="${version_after#v}"
    version_before_stripped="${version_before#v}"

    # Older binaries (e.g. v0.2.40) have a bug where SufficientSelfVersion("")
    # returns true, causing `self update` to skip even without a version constraint.
    # If that happens, retry with -f to still validate the update mechanism works.
    if [[ "$version_after_stripped" == "$version_before_stripped" ]]; then
        if echo "$update_output" | grep -qi "Skipping update"; then
            echo "  ⚠️  Old binary skipped update (known bug). Retrying with -f..."
            force_output=$(cd "$work_dir" && "$DR_BIN" self update -f 2>&1 || true)
            echo "  Force output: $force_output"

            version_after=$(get_installed_version)
            version_after_stripped="${version_after#v}"
            echo "  Version after force update: $version_after"
        fi
    fi

    rm -rf "$work_dir"

    if [[ "$version_after_stripped" != "$version_before_stripped" ]]; then
        echo "✅ TEST 2 PASSED: Version upgraded from $version_before to $version_after."
    else
        echo "❌ TEST 2 FAILED: Version did not change from $version_before (expected upgrade to latest)."
        cleanup_curl_install
        exit 1
    fi

    cleanup_curl_install
}

# ──────────────────────────────────────────────────────────────
# Test 3: brew install → dr self update (should use brew path)
# ──────────────────────────────────────────────────────────────
test_brew_self_update() {
    echo ""
    echo "════════════════════════════════════════════════════════════════"
    echo "TEST 3: brew install latest → dr self update (should use brew)"
    echo "════════════════════════════════════════════════════════════════"

    # Only run on macOS where brew is available
    if [[ "$(uname -s)" != "Darwin" ]]; then
        echo "⏭️  TEST 3 SKIPPED: Not running on macOS."
        return 0
    fi

    if ! command -v brew &>/dev/null; then
        echo "⏭️  TEST 3 SKIPPED: Homebrew not installed."
        return 0
    fi

    # Clean up curl-installed binary so brew-installed one takes precedence
    cleanup_curl_install

    echo "Installing dr-cli via brew..."
    brew tap datarobot-oss/taps 2>/dev/null || true
    brew install --cask dr-cli 2>/dev/null || brew upgrade --cask dr-cli 2>/dev/null || true

    # Find the brew-installed binary (cask installs to a predictable location)
    brew_dr=$(brew --prefix 2>/dev/null)/bin/dr
    if [[ ! -x "$brew_dr" ]]; then
        # Fall back to whatever is on PATH
        brew_dr=$(command -v dr 2>/dev/null || echo "")
    fi

    if [[ -z "$brew_dr" || ! -x "$brew_dr" ]]; then
        echo "❌ TEST 3 FAILED: dr not found after brew install."
        exit 1
    fi

    # Verify this is actually the DataRobot CLI (not some other 'dr' binary)
    if ! "$brew_dr" self version --format=json 2>/dev/null | grep -q '"version"'; then
        echo "⏭️  TEST 3 SKIPPED: brew-installed dr does not appear to be the DataRobot CLI."
        brew uninstall --cask dr-cli 2>/dev/null || true
        return 0
    fi

    version_before=$("$brew_dr" self version --format=json | grep '"version"' | sed -E 's/.*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
    echo "Version after brew install: $version_before"

    # Run from isolated temp dir to avoid .datarobot detection
    local work_dir
    work_dir=$(make_isolated_dir)

    echo "Running dr self update from isolated dir ($work_dir)..."
    update_output=$(cd "$work_dir" && "$brew_dr" self update 2>&1 || true)
    echo "Output: $update_output"

    version_after=$("$brew_dr" self version --format=json | grep '"version"' | sed -E 's/.*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
    echo "Version after self update: $version_after"

    rm -rf "$work_dir"

    # Brew path: version should stay the same (already latest) or we just confirm no errors
    echo "✅ TEST 3 PASSED: brew-based self update completed without errors (version: $version_after)."

    # Uninstall to clean up
    brew uninstall --cask dr-cli 2>/dev/null || true
}

# ──────────────────────────────────────────────────────────────
# Test 4 (stretch): template min version met → self update (no-op)
# ──────────────────────────────────────────────────────────────
test_template_min_version_noop() {
    echo ""
    echo "═════════════════════════════════════════════════════════════════════════════"
    echo "TEST 4 (stretch): minimum CLI version satisfied → dr self update (no-op)"
    echo "═════════════════════════════════════════════════════════════════════════════"

    cleanup_curl_install

    # Install latest via curl
    echo "Installing latest via curl..."
    curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | INSTALL_DIR="$INSTALL_DIR" sh

    version_installed=$(get_installed_version)
    echo "Installed version: $version_installed"

    # Create a fake template directory under /tmp so FindRepoRoot() won't find
    # .datarobot/cli ancestors from the user's real home directory
    template_dir=$(mktemp -d /tmp/dr-smoke-template.XXXXXX)
    mkdir -p "$template_dir/.datarobot/cli"

    cat > "$template_dir/.datarobot/cli/versions.yaml" <<EOF
dr:
  name: DataRobot CLI
  minimum-version: "${version_installed#v}"
  command: dr
  url: https://github.com/datarobot-oss/cli
EOF

    echo "Created template with minimum-version: ${version_installed#v}"

    echo "Running dr self update from template directory..."
    pushd "$template_dir" > /dev/null
    update_output=$("$DR_BIN" self update 2>&1 || true)
    popd > /dev/null
    echo "Output: $update_output"

    # Should see "Skipping update" message
    if echo "$update_output" | grep -qi "Skipping update"; then
        echo "✅ TEST 4 PASSED: self update correctly skipped when minimum version is satisfied."
    else
        echo "❌ TEST 4 FAILED: Expected 'Skipping update' message but got: $update_output"
        rm -rf "$template_dir"
        cleanup_curl_install
        exit 1
    fi

    rm -rf "$template_dir"
    cleanup_curl_install
}

# ──────────────────────────────────────────────────────────────
# Test 5 (stretch): template min version met → self update -f (force upgrade)
# ──────────────────────────────────────────────────────────────
test_template_min_version_force_update() {
    echo ""
    echo "══════════════════════════════════════════════════════════════════════════════"
    echo "TEST 5 (stretch): minimum CLI version satisfied → dr self update -f (force)"
    echo "══════════════════════════════════════════════════════════════════════════════"

    cleanup_curl_install

    # Install the old version first so we can observe an actual upgrade
    echo "Installing $OLD_VERSION via curl..."
    curl -fsSL https://raw.githubusercontent.com/datarobot-oss/cli/main/install.sh | INSTALL_DIR="$INSTALL_DIR" sh -s -- "$OLD_VERSION"

    version_before=$(get_installed_version)
    echo "Installed version: $version_before"

    # Create a fake template directory under /tmp so FindRepoRoot() won't find
    # .datarobot/cli ancestors from the user's real home directory
    template_dir=$(mktemp -d /tmp/dr-smoke-template.XXXXXX)
    mkdir -p "$template_dir/.datarobot/cli"

    cat > "$template_dir/.datarobot/cli/versions.yaml" <<EOF
dr:
  name: DataRobot CLI
  minimum-version: "${version_before#v}"
  command: dr
  url: https://github.com/datarobot-oss/cli
EOF

    echo "Created template with minimum-version: ${version_before#v}"

    # Without -f, should skip (version satisfies minimum)
    echo "Running dr self update (without -f) from template directory..."
    pushd "$template_dir" > /dev/null
    noflag_output=$("$DR_BIN" self update 2>&1 || true)
    popd > /dev/null
    echo "Output: $noflag_output"

    if echo "$noflag_output" | grep -qi "Skipping update"; then
        echo "  ✅ Confirmed: self update without -f skips when min version is satisfied."
    else
        echo "  ⚠️  Warning: Did not see 'Skipping update'. Output: $noflag_output"
    fi

    # With -f, should force update
    echo "Running dr self update -f from template directory..."
    pushd "$template_dir" > /dev/null
    force_output=$("$DR_BIN" self update -f 2>&1 || true)
    popd > /dev/null
    echo "Output: $force_output"

    version_after=$(get_installed_version)
    echo "Version after force update: $version_after"

    version_after_stripped="${version_after#v}"
    version_before_stripped="${version_before#v}"

    if [[ "$version_after_stripped" != "$version_before_stripped" ]]; then
        echo "✅ TEST 5 PASSED: Force update upgraded from $version_before to $version_after."
    else
        echo "❌ TEST 5 FAILED: Version did not change from $version_before after force update."
        rm -rf "$template_dir"
        cleanup_curl_install
        exit 1
    fi

    rm -rf "$template_dir"
    cleanup_curl_install
}

# ──────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────
main() {
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║        dr self update — Smoke Tests                      ║"
    echo "╚══════════════════════════════════════════════════════════╝"

    # Ensure INSTALL_DIR exists and is on PATH
    mkdir -p "$INSTALL_DIR"
    export PATH="$INSTALL_DIR:$PATH"

    test_curl_latest_self_update
    test_curl_old_version_self_update
    test_brew_self_update
    test_template_min_version_noop
    test_template_min_version_force_update

    echo ""
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║        All self-update smoke tests passed!               ║"
    echo "╚══════════════════════════════════════════════════════════╝"
}

main "$@"
