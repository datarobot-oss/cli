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
up_rc="$WL_RC"

# Held now, before any other wl:: call reuses WL_OUT/WL_ERR. This is the
# message printed at the moment the workload becomes stuck; E.2 reads it.
printf '%s\n%s\n' "$WL_OUT" "$WL_ERR" > "$WL_SCRATCH/deploy-err.txt"

# The create writes the binding before the wait, so any run that got as far as
# a workload leaves an id in the file. Register it before deciding anything:
# a scenario that bails still has to clean up what it made.
wl_id="$(grep -oE '[0-9a-f]{24}' "$work/.datarobot.yaml" | head -1)"
if [[ -n "$wl_id" ]]; then
    wl::register_workload "$wl_id"
fi

# Both of these were skips, which the runner scores as a pass — a green E that
# asserted nothing, --recreate included. Neither is a tolerable outcome: this
# scenario exists to exercise recovery from a dead workload, and a run that
# never reaches one has not tested the thing it is here to test.
if [[ "$up_rc" -eq 0 ]]; then
    wl::fail "the workload came up; this scenario needs an image that exits, and busybox no longer does"
fi

if [[ -z "$wl_id" ]]; then
    wl::fail "the deploy failed before a workload existed; nothing to recover from (see deploy-err.txt)"
fi

# --- E.2 The refusal carries its own exit ----------------------------------
# It used to end at "check the logs", which is where recovery ran out.

grep -q "dr workload delete" "$WL_SCRATCH/deploy-err.txt" \
    || wl::fail "the dead-deploy message must name the delete that clears the way"
wl::pass "the dead-deploy message names 'dr workload delete'"

grep -q -- "--recreate" "$WL_SCRATCH/deploy-err.txt" \
    || wl::fail "the dead-deploy message must name the one-step recovery"
wl::pass "the dead-deploy message names --recreate"

# The regression that defines this ticket: no message may end at removing a
# file or a directory the user has to find themselves.
# The strings this guards against were "delete /abs/path/.datarobot/workload"
# and "Delete /abs/path/.datarobot/workload to re-init." — a path sits between
# the verb and the dot, so a pattern that expects them adjacent matches neither,
# and grep -E is case-sensitive besides. Anchoring on the trailing slash keeps
# it off ".datarobot.yaml", which several healthy messages do name.
wl::assert_absent "$WL_SCRATCH/deploy-err.txt" \
    '([Dd]elete|[Rr]emove|rm -rf).{0,80}\.datarobot/|[Dd]elete the (file|state)' \
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
wl::dr_capture workload up --dir "$work" --yes --recreate --dry-run --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload up --recreate --dry-run"

# Held now: the next wl:: call reuses WL_OUT.
preview_json="$WL_OUT"

still="$(wl::dr_jq '.status' workload get "$wl_id" 2>/dev/null || echo gone)"
[[ "$still" != "gone" ]] \
    || wl::fail "--dry-run --recreate deleted the workload"
wl::pass "--dry-run --recreate left the workload alone (status: $still)"

# A preview has to predict the run it previews. Planning against the workload
# that is about to be deleted described a roll onto something the very next
# check refuses, while the hint above it said a create would happen.
previewed="$(printf '%s' "$preview_json" | jq -r '.up.action')"
[[ "$previewed" == "created" ]] \
    || wl::fail "--dry-run --recreate planned '$previewed'; the delete leaves a create"
wl::pass "--dry-run --recreate plans the create the real run performs"

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
