#!/usr/bin/env bash
# Ticket: RAPTOR-19533
# Scenario D — built-workload rebuild round trip (ErrImagePull path).
#
# Defect #2: binding to a SOURCE-BUILT workload wrote
# build.artifactImageBuildId into the manifest, and removing it by hand led
# to ErrImagePull on the next `up` — the workload pinned to a build the
# platform had already forgotten. Whoami (Scenario A) is image-based and
# never touches this path; this scenario needs a workload whose image the
# platform builds.
#
# Pass criteria: after re-bind, the manifest keeps imageBuildConfig but
# carries none of the build OUTPUTS, and a subsequent `up` rebuilds (no
# ErrImagePull), the rollout completes, status reaches running, and the
# endpoint answers an authenticated curl.
#
# OPT-IN: ~20-30 min (two image builds). Run via the `d` arg or
# WORKLOAD_SMOKE_INCLUDE_D=1. Source: tmp/workload-verify-runbook.md Scenario D.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "D: built-workload rebuild round trip"

# --- D.1 Create a built workload --------------------------------------------
# The CMD line is load-bearing: whoami listens on port 80 by default, and
# without it the generated readiness probe (port 8080) never passes — the
# workload goes `errored` and is wedged (no recovery short of delete). Get
# the fixture right the first time.
work="$WL_SCRATCH/project"
mkdir -p "$work"
cd "$work"
cat > Dockerfile <<'EOF'
FROM containous/whoami:latest
EXPOSE 8080
CMD ["--port", "8080"]
EOF

wl::dr_capture workload config --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config (unbound, detects Dockerfile)"
wl::pass "configured unbound project"

wl::dr_capture workload up
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up (first build + deploy)"
WID="$(grep -m1 '^workloadId:' .datarobot.yaml | awk '{print $2}')"
wl::register_workload "$WID"
wl::pass "built and deployed workload $WID"
wl::wait_for_status "$WID" running 1200 >/dev/null

# --- D.2 Re-bind — the defect #2 path ---------------------------------------
# Keep the backup OUTSIDE the project dir: any file left here is synced as
# code on the next `up`, and that change — not anything meaningful — would
# trigger the rebuild.
cp .datarobot.yaml "$WL_SCRATCH/manifest-before-rebind-${RUN}.yaml"
rm .datarobot.yaml
wl::dr_capture workload config --yes --workload-id "$WID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config (re-bind built workload)"
wl::pass "re-bound built workload"

# Assert the render: build container keeps its recipe but carries none of
# the build OUTPUTS, and no imageUri beside imageBuildConfig.
grep -nq 'imageBuildConfig:' .datarobot.yaml || wl::fail "build recipe (imageBuildConfig) lost"
wl::pass "imageBuildConfig retained"
wl::assert_absent .datarobot.yaml 'artifactImageBuildId|^\s+build:|imageOutdated|codeRef' "no build outputs leaked"
wl::assert_absent .datarobot.yaml 'imageUri' "no imageUri pinned on a build container"

# --- D.3 Re-deploy — the ErrImagePull verdict --------------------------------
# Touch a file so the synced-code change forces a rebuild (a no-op second
# `up` may skip the build path).
touch .rebuild-trigger-${RUN}
wl::dr_capture workload up
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up (rebuild after re-bind)"
wl::pass "up rebuilt after re-bind (no ErrImagePull)"
wl::wait_for_status "$WID" running 1200 >/dev/null

# Hit the endpoint: it must answer (not a stale-image 5xx). Pull auth from
# the CLI itself via `dr auth export` so this works whether the token lives
# in drconfig.yaml or an env override.
ep="$("$DR_BIN" workload endpoint "$WID")"
code="$(eval "$("$DR_BIN" auth export --shell bash)" >/dev/null 2>&1; \
    curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer $DATAROBOT_API_TOKEN" "$ep")"
[[ "$code" == "200" || "$code" == "401" || "$code" == "403" ]] \
    || wl::fail "endpoint returned HTTP $code (expected 200, or 401/403 if auth-gated)"
wl::pass "endpoint answered (HTTP $code)"

wl::stop_timer
echo "✅ Scenario D passed"
