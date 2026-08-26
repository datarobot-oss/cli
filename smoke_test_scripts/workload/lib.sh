#!/usr/bin/env bash
# Shared helpers for the `dr workload` / `dr artifact` acceptance scenarios.
#
# Sourced by each scenario_* script and by run_workload_smoke_test.sh. It sets
# up an isolated, endpoint-agnostic environment, a run identity so repeated
# runs never collide on names, a scratch directory under mktemp(1), timing
# helpers, assertion helpers, a workload wait/poll helper, and a trap-based
# cleanup registry that stops any workloads a scenario created even when it
# fails midway.
#
# Environment contract:
#   The CLI is expected to be already authenticated via its drconfig.yaml
#   (the normal "configured in every shell" state). These env vars are
#   OPTIONAL overrides on top of that config — set them only to redirect the
#   suite at a different endpoint or to inject a token without touching config:
#     DATAROBOT_API_TOKEN / DR_API_TOKEN  — override the configured token.
#     DATAROBOT_ENDPOINT                  — override the configured endpoint.
#     DR_BIN                              — binary under test (default ./dist/dr).
#     WORKLOAD_SMOKE_INCLUDE_D            — opts scenario D into the default set.
#     RUN                                  — run identity for name uniqueness.

# shellcheck shell=bash
# shellcheck disable=SC2317  # trap/cleanup fns look "unreachable" to shellcheck.

set -euo pipefail

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

# Directory containing this file (smoke_test_scripts/workload/).
WL_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# The smoke_test_scripts/ directory one level up.
WL_SCRIPT_DIR="$(cd "$WL_LIB_DIR/.." && pwd)"
# The repository root (smoke_test_scripts/ lives at the repo root).
WL_REPO_ROOT="$(cd "$WL_SCRIPT_DIR/.." && pwd)"

# ---------------------------------------------------------------------------
# Environment
# ---------------------------------------------------------------------------

# Resolve an optional token override from either variable. Returns empty
# (no override) when neither is set — the CLI then falls back to drconfig.yaml.
wl::resolve_token() {
    printf '%s' "${DR_API_TOKEN:-${DATAROBOT_API_TOKEN:-}}"
}

# Initialise the run environment. Idempotent: safe to source from many scripts.
# Relies on the CLI's existing drconfig.yaml for auth/endpoint; env vars only
# override when explicitly set.
wl::init_env() {
    # Binary under test: the locally built one unless overridden.
    : "${DR_BIN:=$WL_REPO_ROOT/dist/dr}"
    if [[ ! -x "$DR_BIN" ]]; then
        echo "❌ dr binary not found at $DR_BIN (run 'task build' or set DR_BIN)" >&2
        return 1
    fi

    # Run identity so every created resource is uniquely named and re-runs
    # never collide. Exported so scenario scripts share it.
    : "${RUN:=$(date +%Y%m%d-%H%M)}"
    export RUN

    # Optional token override. Only exported when set, so an unset var does
    # NOT shadow the configured drconfig.yaml token with an empty string.
    local token
    token="$(wl::resolve_token)"
    if [[ -n "$token" ]]; then
        export DATAROBOT_API_TOKEN="$token"
    fi
    # DATAROBOT_ENDPOINT is only exported when the caller set it; otherwise the
    # configured endpoint wins (env binding would otherwise shadow config).
    [[ -n "${DATAROBOT_ENDPOINT:-}" ]] && export DATAROBOT_ENDPOINT

    # The workload feature gate is CLI-side and never in drconfig.yaml, so the
    # suite always sets it. Non-interactive keeps prompts out of CI/local runs.
    export DATAROBOT_CLI_FEATURE_WORKLOAD=true
    export DATAROBOT_CLI_NON_INTERACTIVE=1

    # Per-run scratch directory; removed on exit (see wl::register_cleanup).
    WL_SCRATCH="$(mktemp -d -t wl-smoke-${RUN}.XXXXXXXX)"
    export WL_SCRATCH

    # Confirm the build under test reports a version, so a misconfigured
    # binary fails fast instead of mid-scenario.
    "$DR_BIN" --version >/dev/null

    # Auth + reachability probe: a single workload list call validates that the
    # configured CLI can talk to the API before scenarios start churning. A
    # failure here is almost always config (no token, wrong endpoint).
    if ! "$DR_BIN" workload list --output-format json >/dev/null 2>&1; then
        echo "❌ Auth/reachability probe failed: 'dr workload list' did not succeed." >&2
        echo "   The CLI must be authenticated (drconfig.yaml) and the endpoint reachable." >&2
        echo "   Set DATAROBOT_API_TOKEN / DATAROBOT_ENDPOINT only to override config." >&2
        return 1
    fi
}

# ---------------------------------------------------------------------------
# Timing
# ---------------------------------------------------------------------------

WL_TEST_TIMINGS=""
WL_TEST_START=0
WL_TEST_NAME=""

wl::start_timer() {
    WL_TEST_NAME="$1"
    WL_TEST_START=$(date +%s)
    echo ""
    echo "▶ $WL_TEST_NAME"
}

wl::stop_timer() {
    local elapsed=$(( $(date +%s) - WL_TEST_START ))
    echo "  ⏱  ${WL_TEST_NAME}: ${elapsed}s"
    WL_TEST_TIMINGS+=$'\t'"${elapsed}s\t${WL_TEST_NAME}"$'\n'
}

# ---------------------------------------------------------------------------
# Assertions
# ---------------------------------------------------------------------------

wl::pass() { echo "  ✅ $*"; }
wl::skip() { echo "  ⏭️  $*"; }

# Fail hard: print context and exit non-zero. set -e propagates to the
# scenario, whose EXIT trap runs cleanup before the orchestrator sees 1.
wl::fail() {
    echo "  ❌ $*" >&2
    exit 1
}

# Assert a `dr` invocation succeeded; dump stdout/stderr on failure.
wl::assert_cmd_ok() {
    local rc=$1 out=$2 err=$3 ctx=$4
    if [[ "$rc" -ne 0 ]]; then
        echo "  ❌ $ctx: dr exited $rc" >&2
        echo "  --- STDOUT ---" >&2
        printf '%s\n' "$out" >&2
        echo "  --- STDERR ---" >&2
        printf '%s\n' "$err" >&2
        exit 1
    fi
}

# Assert a grep pattern is ABSENT from a file. Exits 2 on a missing file
# (the runbook's hard-won trap: grep exit 2 silently masked a missing file
# and let leaks pass). We fail loudly on a missing file instead.
wl::assert_absent() {
    local file=$1 pattern=$2 label=$3
    if [[ ! -f "$file" ]]; then
        wl::fail "$label: file not found ($file) — cannot check for $pattern"
    fi
    if grep -nE "$pattern" "$file"; then
        wl::fail "$label: forbidden pattern matched ($pattern)"
    fi
    wl::pass "$label: no match for $pattern"
}

# Assert no null-valued keys leaked into a rendered manifest.
wl::assert_no_null_keys() {
    wl::assert_absent "$1" ':\s*(null|~)\s*$' "no null keys ($1)"
}

# Assert no server bookkeeping keys leaked into a rendered manifest.
# workloadId is BY DESIGN re-added by the CLI writer as the managed binding,
# so it is intentionally not in this list.
wl::assert_no_server_keys() {
    local file=$1
    wl::assert_absent "$file" '^\s*(id|createdAt|updatedAt|status|endpoint|artifactId|resolvedBundle|imageOutdated|build|codeRef):' \
        "no server bookkeeping keys ($file)"
}

# ---------------------------------------------------------------------------
# dr wrappers
# ---------------------------------------------------------------------------

# Run dr, capturing stdout/stderr and return code into the caller-visible
# globals WL_OUT / WL_ERR / WL_RC. Callers pass --output-format json etc.
# Stderr goes to a temp file so it is never lost under process substitution.
wl::dr_capture() {
    local err_tmp
    err_tmp="$WL_SCRATCH/dr-err.$$"
    WL_OUT="$("$DR_BIN" "$@" 2>"$err_tmp")" && WL_RC=0 || WL_RC=$?
    WL_ERR="$(cat "$err_tmp")"
    rm -f "$err_tmp"
}

# Run dr ... --output-format json and parse a jq filter, printing the result.
wl::dr_jq() {
    local filter=$1; shift
    wl::dr_capture "$@" --output-format json
    wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "dr $* --output-format json"
    printf '%s' "$WL_OUT" | jq -r "$filter"
}

# ---------------------------------------------------------------------------
# Workload lifecycle helpers
# ---------------------------------------------------------------------------

# Poll `dr workload status <id>` until it equals the target or the timeout
# (seconds, default 300) elapses. Prints the final status. On `errored`,
# dumps `dr workload get` for diagnosis and fails.
wl::wait_for_status() {
    local id=$1 target=${2:-running} timeout=${3:-300}
    local elapsed=0 interval=10 status
    while (( elapsed < timeout )); do
        status="$("$DR_BIN" workload status "$id" --output-format json | jq -r '.status')"
        if [[ "$status" == "$target" ]]; then
            wl::pass "workload $id reached $status"
            printf '%s' "$status"
            return 0
        fi
        if [[ "$status" == "errored" ]]; then
            echo "  ❌ workload $id errored; dumping 'dr workload get':" >&2
            "$DR_BIN" workload get "$id" >&2 || true
            return 1
        fi
        sleep "$interval"
        elapsed=$(( elapsed + interval ))
    done
    wl::fail "workload $id never reached $target within ${timeout}s (last: $status)"
}

# ---------------------------------------------------------------------------
# Cleanup registry
# ---------------------------------------------------------------------------

# Workloads created by the current scenario, stopped best-effort on exit.
WL_CREATED_WORKLOADS=()

wl::register_workload() {
    WL_CREATED_WORKLOADS+=("$1")
}

# Best-effort stop of every registered workload. Draft workloads have an 8h
# TTL, but we don't rely on it: a wedged errored workload can block re-runs.
wl::cleanup() {
    local rc=$?
    local id
    for id in "${WL_CREATED_WORKLOADS[@]:-}"; do
        [[ -n "$id" ]] || continue
        "$DR_BIN" workload stop "$id" >/dev/null 2>&1 || true
        echo "  🧹 stopped workload $id"
    done
    [[ -n "${WL_SCRATCH:-}" ]] && rm -rf "$WL_SCRATCH" 2>/dev/null || true
    return "$rc"
}

# Register cleanup on EXIT for the current shell. Scenarios call this once.
wl::register_cleanup() {
    trap wl::cleanup EXIT
}
