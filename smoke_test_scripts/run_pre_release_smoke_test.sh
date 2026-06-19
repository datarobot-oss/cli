#!/bin/bash

# Pre-release smoke tests.
#
# These are heavier, longer-running end-to-end tests that we do NOT want to run
# on every PR / in the fast smoke suite (run_smoke_test.sh). They are intended
# to gate promotion of a release from pre-release to stable. Add future
# slow/expensive end-to-end checks here rather than to run_smoke_test.sh.
#
# Currently covers:
#   * dr start flow for the Agentic Starter (datarobot-agent-application)
#     template — regression guard (infinite loop on `dr start`).

# Be sure to get DR_API_TOKEN from args
args=("$@")
DR_API_TOKEN=${args[0]}
if [[ -z "$DR_API_TOKEN" ]]; then
  echo "❌ The variable 'DR_API_TOKEN' must be supplied as arg."
  exit 1
fi

# Preflight: required tooling. Failing here with a clear message beats a cryptic
# downstream failure (e.g. a missing `yq` silently leaves the endpoint unset, so
# `dr templates setup` shows the URL picker instead of the template list).
for _tool in dr expect yq wget; do
  if ! command -v "$_tool" >/dev/null 2>&1; then
    echo "❌ Required tool '$_tool' not found on PATH."
    echo "   Local setup: build & install the CLI ('task build' && 'LOCAL_BINARY=./dist/dr sh install.sh'),"
    echo "   then install deps (macOS: 'brew install expect yq coreutils wget')."
    exit 1
  fi
done

export TERM="dumb"

# Timing helpers
SCRIPT_START=$(date +%s)
TEST_TIMINGS=""

start_timer() {
    TEST_NAME="$1"
    TEST_START=$(date +%s)
    echo ""
    echo "▶ $TEST_NAME"
}

stop_timer() {
    local elapsed=$(( $(date +%s) - TEST_START ))
    echo "  ⏱  ${TEST_NAME}: ${elapsed}s"
    TEST_TIMINGS="${TEST_TIMINGS}  ${elapsed}s\t${TEST_NAME}\n"
}

# Used throughout testing
testing_url="https://app.datarobot.com"

# Determine if we can access URL
wget -q --spider "$testing_url"
if [ $? -eq 0 ]; then
    url_accessible=1
else
    url_accessible=0
fi

# Using `DATAROBOT_CLI_CONFIG` to be sure we can save/update config file in GitHub Action runners
testing_dr_cli_config_dir="$(pwd)/.config/datarobot/"
mkdir -p "$testing_dr_cli_config_dir"
export DATAROBOT_CLI_CONFIG="${testing_dr_cli_config_dir}drconfig.yaml"
touch "$DATAROBOT_CLI_CONFIG"
cat "$(pwd)/smoke_test_scripts/assets/example_config.yaml" > "$DATAROBOT_CLI_CONFIG"

# Set API token and endpoint in our ephemeral config file. Unlike the fast smoke
# suite (which sets the endpoint interactively via `dr auth setURL`), this script
# is non-interactive end-to-end, so we write both keys directly.
yq -i ".token = \"$DR_API_TOKEN\"" "$DATAROBOT_CLI_CONFIG"
yq -i ".endpoint = \"${testing_url}/api/v2\"" "$DATAROBOT_CLI_CONFIG"

start_timer "dr start flow (Agentic Starter template)"
if [ "$url_accessible" -eq 0 ]; then
  # This is a release gate: if we cannot reach the endpoint we CANNOT verify the
  # `dr start` regression guard, so we must fail rather than skip. Skipping here
  # would let a release be promoted without the check ever running.
  echo "❌ URL (${testing_url}) is not accessible - cannot run the 'dr start' pre-release gate. Failing instead of skipping."
  stop_timer
  exit 1
else
  # Regression guard: CLI 0.2.69/0.2.70 went into an infinite loop on
  # `dr start` for the Agentic Starter (datarobot-agent-application) template and
  # completely broke the quickstart flow, but no pre-release test caught it.
  # This test clones that template and runs `dr start`, asserting it reaches the
  # start-command execution stage without hanging or looping.

  AGENTIC_DIR="./datarobot-agent-application"

  # Clean any leftover clone from a previous run.
  rm -rf "$AGENTIC_DIR"

  # 1. Clone the Agentic Starter template. Check the expect exit status first: a
  #    failed setup (timeout, wizard error, non-zero `dr templates setup`) can
  #    still leave a partial directory behind, so directory existence alone is
  #    not enough to call the clone successful.
  expect ./smoke_test_scripts/expect_templates_setup_agentic.exp
  setup_rc=$?
  if [ "$setup_rc" -ne 0 ]; then
    echo "❌ Assertion failed: 'dr templates setup' (Agentic Starter) failed (expect exit code: $setup_rc)."
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi
  if [ ! -d "$AGENTIC_DIR" ]; then
    echo "❌ Assertion failed: Agentic Starter directory ($AGENTIC_DIR) does not exist after setup."
    exit 1
  fi
  echo "✅ Agentic Starter template cloned to $AGENTIC_DIR."

  # 2. Run `dr start` from inside the cloned template.
  #    Reset the debug log so step counts reflect only this invocation. A busy
  #    infinite loop keeps emitting output and would never trip expect's
  #    inactivity timeout, so a hard wall-clock `timeout` is the catch-all for
  #    the hang/loop.
  DEBUG_LOG="$HOME/.dr-tui-debug.log"
  : > "$DEBUG_LOG" 2>/dev/null || true

  cd "$AGENTIC_DIR"

  START_TIMEOUT=360
  if command -v timeout >/dev/null 2>&1; then
    TIMEOUT_BIN="timeout"
  elif command -v gtimeout >/dev/null 2>&1; then
    TIMEOUT_BIN="gtimeout" # macOS via coreutils
  else
    # The hard wall-clock cap is the primary busy-loop detector for this gate, so
    # we refuse to run uncapped. On macOS install GNU coreutils for `gtimeout`.
    echo "❌ Neither 'timeout' nor 'gtimeout' is available - cannot enforce the wall-clock cap this gate requires. Install GNU coreutils (e.g. 'brew install coreutils')."
    cd ..
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi

  "$TIMEOUT_BIN" "$START_TIMEOUT" expect ../smoke_test_scripts/expect_start.exp
  start_rc=$?

  if [ "$start_rc" -eq 124 ]; then
    echo "❌ Assertion failed: 'dr start' exceeded ${START_TIMEOUT}s (hard timeout) - likely an infinite loop / hang."
    cd ..
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi
  if [ "$start_rc" -ne 0 ]; then
    echo "❌ Assertion failed: 'dr start' bounded run failed (expect exit code: $start_rc)."
    echo "   --- tail of $DEBUG_LOG ---"
    tail -n 40 "$DEBUG_LOG" 2>/dev/null || echo "   (no debug log)"
    cd ..
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi
  echo "✅ 'dr start' reached the start-command execution stage without hanging."

  # 3. Confirm via the debug log that discovery resolved to executing the start
  #    command, and that the step machine ran a bounded number of times.
  #    The log-based checks ARE the regression guard, so a missing log is a hard
  #    failure for this gate - never a silent skip.
  if [ ! -f "$DEBUG_LOG" ]; then
    echo "❌ Assertion failed: debug log ($DEBUG_LOG) not found - cannot run the log-based regression assertions. Failing instead of skipping."
    cd ..
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi

  if grep -q "execute_script=true" "$DEBUG_LOG"; then
    echo "✅ Assertion passed: 'dr start' resolved to executing the template start command (execute_script=true)."
  else
    echo "❌ Assertion failed: 'dr start' never reached start-command execution (no execute_script=true in debug log)."
    tail -n 40 "$DEBUG_LOG" 2>/dev/null
    cd ..
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi

  # A healthy run logs "start: execute step" once per step (5 steps). A loop
  # would repeat these far beyond the step count. Allow generous headroom.
  step_runs=$(grep -c "start: execute step" "$DEBUG_LOG" 2>/dev/null)
  [ -z "$step_runs" ] && step_runs=0
  echo "ℹ️ 'start: execute step' log lines: ${step_runs}"
  if [ "$step_runs" -gt 20 ]; then
    echo "❌ Assertion failed: 'dr start' executed steps ${step_runs} times (> 20) - indicates an infinite loop."
    cd ..
    rm -rf "$AGENTIC_DIR"
    exit 1
  fi

  # Clean up the cloned template.
  cd ..
  rm -rf "$AGENTIC_DIR"
  stop_timer
fi

# Print timing summary
TOTAL_ELAPSED=$(( $(date +%s) - SCRIPT_START ))
echo ""
echo "══════════════════════════════════════"
echo "  Pre-Release Smoke Test Timing Summary"
echo "══════════════════════════════════════"
printf "$TEST_TIMINGS"
echo "──────────────────────────────────────"
echo "  Total: ${TOTAL_ELAPSED}s"
echo "══════════════════════════════════════"
