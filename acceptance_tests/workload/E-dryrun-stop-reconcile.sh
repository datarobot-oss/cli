#!/usr/bin/env bash
# Scenario E — dry-run idempotency, stop/up reconcile, delete clears binding.
#
# `up --dry-run` on a stable tree says "Already up to date"; `stop` then `up`
# reconciles the same workload instead of recreating it; `delete --yes` removes
# the workload and clears the `workloadId:` binding in .datarobot.yaml.
#
# Uses an image-based whoami workload (like Scenario A): a build would add
# 20-30 min without covering anything new here. ~15-20 min, mostly the
# stop-wait in E.3.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "E: dry-run idempotency, stop/up reconcile, delete binding"

# --- E.1 Create + bind (fast path: image-based, no build) -------------------
work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/workload.yaml" <<EOF
name: aj-re-${RUN}
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

wl::wait_for_status "$WID" running 600 >/dev/null

cd "$work"
wl::dr_capture workload config --yes --workload-id "$WID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config --workload-id"
[[ -f .datarobot.yaml ]] || wl::fail "config did not write .datarobot.yaml"
wl::pass "bound to workload $WID"

# --- E.2 Item 3: `up --dry-run` on an unchanged tree is idempotent ----------
# Twice, to rule out a one-shot fluke (e.g. a plan that only looks empty on
# the very first read after config wrote the file).
for attempt in 1 2; do
    wl::dr_capture workload up --dry-run
    wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up --dry-run (attempt $attempt)"
    printf '%s\n%s' "$WL_OUT" "$WL_ERR" | grep -q 'Already up to date' \
        || wl::fail "up --dry-run (attempt $attempt) did not report 'Already up to date'"
done
wl::pass "up --dry-run reports 'Already up to date' on a stable tree (x2)"

# --- E.3 Item 4: stop, then a plain `up` reconciles in one run --------------
wl::dr_capture workload stop "$WID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload stop"
wl::pass "stop accepted"

# 900s, not the 300s helper default: stop->stopped has taken ~11 minutes on
# staging. Do not tighten this without re-measuring.
wl::wait_for_status "$WID" stopped 900 >/dev/null
wl::pass "workload reached stopped"

wl::dr_capture workload up --yes --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up --yes (reconcile from stopped)"
GOT_WID="$(printf '%s' "$WL_OUT" | jq -r '.up.workloadId')"
[[ "$GOT_WID" == "$WID" ]] \
    || wl::fail "up after stop created/targeted a different workload: got $GOT_WID, want $WID"
wl::pass "up reconciled the stopped workload in one run (no recreate)"

wl::wait_for_status "$WID" running 600 >/dev/null

# --- E.4 Item 6: --output-format json stdout is valid JSON ------------------
status="$(wl::dr_jq '.status' workload status "$WID")"
[[ "$status" == "running" ]] || wl::fail "workload status --output-format json: expected running, got $status"
wl::pass "workload status --output-format json parses and reports running"

# --- E.5 Item 5: `delete --yes` clears the binding --------------------------
wl::dr_capture workload delete "$WID" --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload delete --yes"
printf '%s' "$WL_OUT" | grep -q "Deleted workload: $WID" \
    || wl::fail "delete did not report 'Deleted workload: $WID' (got: $WL_OUT)"
printf '%s' "$WL_ERR" | grep -q 'Removed workloadId from' \
    || wl::fail "delete did not report clearing the binding (stderr: $WL_ERR)"
wl::assert_absent .datarobot.yaml '^workloadId:' "workloadId binding cleared from .datarobot.yaml"
wl::pass "delete removed the workload and cleared the manifest binding"

wl::stop_timer
echo "✅ Scenario E passed"
