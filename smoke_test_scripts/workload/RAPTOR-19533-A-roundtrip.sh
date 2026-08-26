#!/usr/bin/env bash
# Ticket: RAPTOR-19533
# Scenario A — fresh whoami round trip (Ben).
#
# create (image-based whoami) → wait running → bind renders .datarobot.yaml
# → assert no null/server keys leaked → `up` accepts its own file →
# validation probes (load-time + dry-run). The regression this guards:
# `up` validates at load time (internal/workload/up/load.go → Compile())
# before any server mutation, so a bad manifest fails fast.
#
# ~10 min. Source: tmp/workload-verify-runbook.md Scenario A.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "A: whoami create/bind/up round trip"

# --- A.1 Create a draft workload from an inline whoami spec -----------------
work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/workload.yaml" <<EOF
name: aj-ra-${RUN}
artifact:
  name: whoami-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: whoami
            imageUri: containous/whoami:latest
            port: 8080
            primary: true
            entrypoint: ["/whoami", "--port", "8080"]
            readinessProbe:
              path: "/"
              port: 8080
              initialDelaySeconds: 5
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: whoami
          resourceAllocation:
            cpu: 1
            memory: "512MB"
EOF

wl::dr_capture workload create --spec-file "$work/workload.yaml" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload create"
WID="$(printf '%s' "$WL_OUT" | jq -r '.id')"
wl::register_workload "$WID"
wl::pass "created draft workload $WID"

# --- A.2 Wait for running ---------------------------------------------------
wl::wait_for_status "$WID" running 600 >/dev/null

# --- A.3 Bind and render the manifest ---------------------------------------
cd "$work"
wl::dr_capture workload config --yes --workload-id "$WID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config --workload-id"
[[ -f .datarobot.yaml ]] || wl::fail "config did not write .datarobot.yaml"
wl::pass "wrote .datarobot.yaml"

wl::assert_no_null_keys .datarobot.yaml
wl::assert_no_server_keys .datarobot.yaml
# workloadId header IS expected — the CLI re-adds it as the managed binding.
grep -nq '^workloadId:' .datarobot.yaml || wl::fail "workloadId managed binding missing"
wl::pass "workloadId managed binding present"

# --- A.4 Round trip: the ledger accepts what the renderer wrote --------------
wl::dr_capture workload up
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up (accept own file)"
wl::pass "up accepted its own manifest"

# --- A.5 Validation probes --------------------------------------------------
# Path-based edits via yq (already a repo dependency) so the probes are
# robust to the rendered manifest's exact indentation. The paths target the
# runtime containerGroup, matching the runbook's "edit the runtime group".
RT='.runtime.containerGroups[0]'
RTC="$RT.containers[0]"

# Snapshot the clean manifest once; restore it between probes by copying back.
cp .datarobot.yaml "$WL_SCRATCH/manifest-clean.yaml"
restore() { cp "$WL_SCRATCH/manifest-clean.yaml" .datarobot.yaml; }

# (a) replicaCount: 3 + autoscaling: (null)              -> PASS (dry-run)
yq -i "$RT.replicaCount = 3 | $RT.autoscaling = null" .datarobot.yaml
wl::dr_capture workload up --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "probe a: replicaCount + null autoscaling"
wl::pass "probe a: replicaCount + null autoscaling passes (dry-run)"
restore

# (b) replicaCount: null + active autoscaling             -> PASS (dry-run)
yq -i "$RT.replicaCount = null | $RT.autoscaling.enabled = true | $RT.autoscaling.minReplicaCount = 1 | $RT.autoscaling.maxReplicaCount = 4" .datarobot.yaml
wl::dr_capture workload up --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "probe b: null replicaCount + active autoscaling"
wl::pass "probe b: null replicaCount + active autoscaling passes (dry-run)"
restore

# (c) replicaCount: 3 + active autoscaling                -> FAIL at load time
yq -i "$RT.replicaCount = 3 | $RT.autoscaling.enabled = true | $RT.autoscaling.minReplicaCount = 1 | $RT.autoscaling.maxReplicaCount = 4" .datarobot.yaml
wl::dr_capture workload up
if [[ "$WL_RC" -eq 0 ]]; then
    wl::fail "probe c: replicaCount + active autoscaling should have failed at load time"
fi
echo "$WL_ERR" | grep -q 'cannot be set alongside autoscaling' \
    || wl::fail "probe c: expected 'cannot be set alongside autoscaling' error, got: $WL_ERR"
wl::pass "probe c: replicaCount + active autoscaling fails at load time"
restore

# (d) environmentVars entry with neither value nor credential source -> FAIL
yq -i "$RTC.environmentVars = [{\"key\": \"FOO\"}]" .datarobot.yaml
wl::dr_capture workload up
if [[ "$WL_RC" -eq 0 ]]; then
    wl::fail "probe d: envVar without value should have failed at validate time"
fi
wl::pass "probe d: envVar without value fails at validate time"
restore

# (e) same entry with value: "" (explicitly empty)        -> PASS (dry-run)
yq -i "$RTC.environmentVars = [{\"key\": \"FOO\", \"value\": \"\"}]" .datarobot.yaml
wl::dr_capture workload up --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "probe e: envVar with explicit empty value"
wl::pass "probe e: envVar with explicit empty value passes (dry-run)"
restore

wl::stop_timer
echo "✅ Scenario A passed"
