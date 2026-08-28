#!/usr/bin/env bash
# Ticket: RAPTOR-19729
# Scenario E — recovery never ends at "delete the file".
#
# The loop this guards: nothing told the user to remove workloadId, removing
# it by hand made the create hit 409, and the 409 advised setting the exact
# line just deleted. Deleting .datarobot/ was the only escape people found,
# and it takes the code catalog with it.
#
# Asserts the two halves of the fix against a live tenant: every refusal on a
# dead workload names a command rather than a file, and --recreate performs
# that command inside the deploy.
#
# ~8 min.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "E: recovery never ends at the file"

# --- E.1 A workload that cannot come up ------------------------------------
# An image that exits immediately is the cheapest way to reach `errored`
# without waiting out a build.
work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/.datarobot.yaml" <<YAML
name: aj-recover-${RUN}
artifact:
  name: recover-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            imageUri: busybox:latest
            port: 8080
            primary: true
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
YAML

# The deploy is expected to fail: the container exits and the workload lands
# errored. What is under test is the message it fails with.
wl::dr_capture workload up --dir "$work" --yes
if [[ "$WL_RC" -eq 0 ]]; then
    wl::skip "the workload came up; this scenario needs one that does not"
    wl::stop_timer
    exit 0
fi

# The create writes the binding before the wait, so a run that got as far as a
# workload has an id in the file. One that failed earlier is a different bug
# and cannot exercise this scenario.
wl_id="$(grep -oE '[0-9a-f]{24}' "$work/.datarobot.yaml" | head -1)"
if [[ -z "$wl_id" ]]; then
    wl::skip "the deploy failed before a workload existed; nothing to recover from"
    wl::stop_timer
    exit 0
fi

wl::register_workload "$wl_id"

# --- E.2 The refusal carries its own exit ----------------------------------
# This is the message printed at the moment the workload becomes stuck. It
# used to end at "check the logs", which is where recovery ran out.
printf '%s\n%s\n' "$WL_OUT" "$WL_ERR" > "$WL_SCRATCH/deploy-err.txt"

grep -q "dr workload delete" "$WL_SCRATCH/deploy-err.txt" \
    || wl::fail "the dead-deploy message must name the delete that clears the way"
wl::pass "the dead-deploy message names 'dr workload delete'"

grep -q -- "--recreate" "$WL_SCRATCH/deploy-err.txt" \
    || wl::fail "the dead-deploy message must name the one-step recovery"
wl::pass "the dead-deploy message names --recreate"

# The regression that defines this ticket: no message may end at removing a
# file or a directory the user has to find themselves.
wl::assert_absent "$WL_SCRATCH/deploy-err.txt" \
    'delete \.datarobot|delete the file|Delete the file|delete the state' \
    "recovery never ends at deleting state"

# --- E.3 A second deploy refuses the same way ------------------------------
# deployable() speaks for the run once the workload is already stuck. Both
# messages have to agree, or the second run reopens the loop the first closed.
wl::dr_capture workload up --dir "$work" --yes
printf '%s\n%s\n' "$WL_OUT" "$WL_ERR" > "$WL_SCRATCH/second-err.txt"

grep -q "dr workload delete" "$WL_SCRATCH/second-err.txt" \
    || wl::fail "the errored refusal must name the delete too"
wl::pass "the errored refusal agrees with the deploy that produced it"

# --- E.4 --dry-run --recreate deletes nothing ------------------------------
# The delete happens before the plan is printed, so a dry run that deleted
# would be a dry run that mutated.
wl::dr_capture workload up --dir "$work" --yes --recreate --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up --recreate --dry-run"

still="$(wl::dr_jq '.status' workload get "$wl_id" 2>/dev/null || echo gone)"
[[ "$still" != "gone" ]] \
    || wl::fail "--dry-run --recreate deleted the workload"
wl::pass "--dry-run --recreate left the workload alone (status: $still)"

# --- E.5 --recreate performs the recovery ----------------------------------
# The workload is deleted, the binding goes with it, and the run continues
# into the create — the whole loop, closed by one command.
wl::dr_capture workload up --dir "$work" --yes --recreate
printf '%s\n%s\n' "$WL_OUT" "$WL_ERR" > "$WL_SCRATCH/recreate-err.txt"

grep -qE "Deleting the (errored|terminated) workload" "$WL_SCRATCH/recreate-err.txt" \
    || wl::fail "--recreate must say what it deleted"
wl::pass "--recreate deleted the dead workload"

new_id="$(grep -oE '[0-9a-f]{24}' "$work/.datarobot.yaml" | head -1)"
if [[ -n "$new_id" && "$new_id" != "$wl_id" ]]; then
    wl::register_workload "$new_id"
    wl::pass "--recreate rebound the manifest to a new workload ($new_id)"
else
    wl::fail "the manifest still names $wl_id after --recreate"
fi

# The state directory survives: the catalog and last-synced version are what
# deleting .datarobot/ used to throw away.
if [[ -d "$work/.datarobot/workload" ]]; then
    wl::pass "the project's state directory survived the recovery"
else
    wl::fail "--recreate must not discard the code catalog"
fi

wl::stop_timer
