#!/usr/bin/env bash
# Scenario F — `delete --purge` takes a directory from deployed to never-deployed.
#
# An image-based whoami workload is created and bound, the directory is linked
# to its artifact so the state directory exists, then `delete --purge --yes`
# removes the workload, the draft artifact and the state directory, and clears
# the binding. A plain `up --dry-run` afterwards plans a fresh create. ~5 min.
#
# Credentials are left to the unit tests: minting one needs a secret in .env
# and a --sync-env run.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "F: delete --purge removes the deploy's leftovers"

# --- F.1 Create + bind (image-based, no build) ------------------------------
work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/workload.yaml" <<EOF
name: aj-purge-${RUN}
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
AID="$(printf '%s' "$WL_OUT" | jq -r '.artifactId')"
wl::register_workload "$WID"
wl::register_artifact "$AID"
wl::pass "created draft workload $WID on artifact $AID"

wl::wait_for_status "$WID" running 600 >/dev/null

cd "$work"
wl::dr_capture workload config --yes --workload-id "$WID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config --workload-id"
[[ -f .datarobot.yaml ]] || wl::fail "config did not write .datarobot.yaml"
wl::pass "bound to workload $WID"

# --- F.2 Link the directory to the artifact so the state directory exists ---
wl::dr_capture artifact code init "$AID" --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact code init"
[[ -d .datarobot/workload ]] || wl::fail "code init did not create the state directory"
wl::pass "state directory linked to artifact $AID"

# --- F.3 delete --purge removes the workload, the artifact and the state ----
wl::dr_capture workload delete "$WID" --purge --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload delete --purge --yes"
printf '%s' "$WL_OUT" | grep -q "Deleted workload: $WID" \
    || wl::fail "delete did not report 'Deleted workload: $WID' (got: $WL_OUT)"
printf '%s' "$WL_ERR" | grep -q 'Removed workloadId from' \
    || wl::fail "delete did not report clearing the binding (stderr: $WL_ERR)"
printf '%s' "$WL_ERR" | grep -q "Removed artifact $AID" \
    || wl::fail "purge did not report removing artifact $AID (stderr: $WL_ERR)"
printf '%s' "$WL_ERR" | grep -q 'Removed .*\.datarobot/workload' \
    || wl::fail "purge did not report removing the state directory (stderr: $WL_ERR)"
[[ ! -d .datarobot/workload ]] || wl::fail "state directory still exists after purge"
wl::assert_absent .datarobot.yaml '^workloadId:' "workloadId binding cleared from .datarobot.yaml"
wl::pass "purge removed the workload, the artifact and the state directory"

# --- F.4 The artifact is gone on the platform -------------------------------
wl::dr_capture artifact get "$AID" --output-format json
[[ "$WL_RC" -ne 0 ]] || wl::fail "artifact $AID still exists after purge"
wl::pass "artifact $AID no longer exists"

# --- F.5 The next deploy plans a fresh create -------------------------------
wl::dr_capture workload up --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up --dry-run after purge"
printf '%s\n%s' "$WL_OUT" "$WL_ERR" | grep -q 'will be created' \
    || wl::fail "up --dry-run after purge did not plan a fresh create"
wl::pass "the next up starts from scratch"

wl::stop_timer
echo "✅ Scenario F passed"
