#!/usr/bin/env bash
# run_workload_smoke_test.sh — orchestrator for the `dr workload` / `dr artifact`
# acceptance scenarios.
#
# Each scenario is a standalone script under smoke_test_scripts/workload/ that
# sources lib.sh, initialises its own isolated env, and cleans up on exit, so
# any subset can also be run directly:
#
#   bash smoke_test_scripts/workload/RAPTOR-19533-A-roundtrip.sh
#
# Usage:
#   DR_API_TOKEN=... ./smoke_test_scripts/run_workload_smoke_test.sh [scenario ...]
#
# Defaults to the fast set: a b c artifact. Scenario D (~20-30 min, two image
# builds) is opt-in — pass `d` as a scenario, or set
# WORKLOAD_SMOKE_INCLUDE_D=1 to add it to the default set.
#
# Exit code is non-zero if any scenario failed.

# shellcheck shell=bash
set -euo pipefail

WL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/workload"

SCRIPT_START=$(date +%s)
RESULTS=""

# Resolve the scenario list.
if [[ $# -gt 0 ]]; then
    scenarios=("$@")
else
    scenarios=(a b c e artifact)
    if [[ "${WORKLOAD_SMOKE_INCLUDE_D:-0}" == "1" ]]; then
        scenarios+=(d)
    fi
fi

# Map a short name to its script. Files are named <TICKET>-<LETTER>-<name>.sh
# (see workload/TICKETS.md); the artifact scenario has no ticket.
scenario_script() {
    case "$1" in
        a|A)        printf '%s/RAPTOR-19533-A-roundtrip.sh'  "$WL_DIR" ;;
        b|B)        printf '%s/RAPTOR-19533-B-sweep.sh'      "$WL_DIR" ;;
        c|C)        printf '%s/RAPTOR-19533-C-rebind.sh'     "$WL_DIR" ;;
        d|D)        printf '%s/RAPTOR-19533-D-built.sh'      "$WL_DIR" ;;
        e|E)        printf '%s/RAPTOR-19729-E-recovery.sh'   "$WL_DIR" ;;
        artifact)   printf '%s/artifact-lifecycle.sh'       "$WL_DIR" ;;
        *) echo "❌ unknown scenario: $1" >&2; return 1 ;;
    esac
}

failed=0
ran=0
for s in "${scenarios[@]}"; do
    script="$(scenario_script "$s")" || exit 1
    if [[ ! -x "$script" ]]; then
        # Not executable — run via bash so a missing +x bit doesn't block.
        echo ""
        echo "▶ $s ($script)"
    fi
    ran=$((ran + 1))
    start=$(date +%s)
    if bash "$script"; then
        elapsed=$(( $(date +%s) - start ))
        RESULTS+=$'\t✅'"${s}\t${elapsed}s"$'\n'
    else
        elapsed=$(( $(date +%s) - start ))
        RESULTS+=$'\t❌'"${s}\t${elapsed}s"$'\n'
        failed=$((failed + 1))
    fi
done

TOTAL=$(( $(date +%s) - SCRIPT_START ))
echo ""
echo "══════════════════════════════════════"
echo "  Workload/Artifact Acceptance Summary"
echo "══════════════════════════════════════"
printf '%b' "$RESULTS"
echo "──────────────────────────────────────"
echo "  Scenarios run: ${ran}   Failed: ${failed}   Total: ${TOTAL}s"
echo "══════════════════════════════════════"

exit "$failed"
