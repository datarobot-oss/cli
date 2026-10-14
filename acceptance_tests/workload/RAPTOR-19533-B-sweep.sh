#!/usr/bin/env bash
# Ticket: RAPTOR-19533
# Scenario B — account sweep binding (Woj).
#
# Woj's repro: every workload on the account failed binding, 4 for 4. The
# test is "bind to everything" — but a repeatable run can't assume the
# account has anything, so create four drafts first, then dry-run bind
# every workload on the account into a temp dir seeded with a Dockerfile
# (so a built-workload's file-existence rule doesn't refuse an otherwise
# clean bind). Pass criteria: every workload prints OK/OK*.
#
# ~10 min. Source: tmp/workload-verify-runbook.md Scenario B.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "B: account sweep binding"

work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/workload.yaml" <<EOF
name: aj-rb-${RUN}-PLACEHOLDER
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

# --- B.1 Create four draft fixtures (no waiting — binding reads settings) ---
fixture_ids=()
for n in 1 2 3 4; do
    sed "s/^name: .*/name: aj-rb-${RUN}-${n}/" "$work/workload.yaml" > "$work/wl-${n}.yaml"
    wl::dr_capture workload create --spec-file "$work/wl-${n}.yaml" --output-format json
    wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload create fixture ${n}"
    id="$(printf '%s' "$WL_OUT" | jq -r '.id')"
    wl::register_workload "$id"
    fixture_ids+=("$id")
done
wl::pass "created ${#fixture_ids[@]} draft fixtures"

# --- B.2 Sweep: dry-run bind to every workload on the account ---------------
# Each bind starts in an empty directory, which a generated-build workload
# needs: a directory with files of its own must be one the platform can build
# from. A workload built from a Dockerfile is refused there for want of one,
# so it is retried with a Dockerfile seeded.
fail_count=0
ok_count=0
while read -r id; do
    [[ -n "$id" ]] || continue
    d="$(mktemp -d)"
    wl::dr_capture workload config --dir "$d" --yes --workload-id "$id" --dry-run
    if [[ "$WL_RC" -ne 0 ]] && printf '%s' "$WL_ERR" | grep -q 'no Dockerfile exists'; then
        printf 'FROM containous/whoami:latest\nEXPOSE 8080\nCMD ["--port", "8080"]\n' > "$d/Dockerfile"
        wl::dr_capture workload config --dir "$d" --yes --workload-id "$id" --dry-run
    fi
    if [[ "$WL_RC" -eq 0 ]]; then
        # --dry-run prints the manifest to stdout and writes NO file; grep the
        # captured output, not $d/.datarobot.yaml (missing file → grep exit 2
        # silently falls into the OK branch and masks leaks).
        if printf '%s' "$WL_OUT" | grep -qE ':\s*(null|~)\s*$'; then
            echo "  DIRTY  $id (null key leaked)"
            fail_count=$((fail_count + 1))
        else
            echo "  OK     $id"
            ok_count=$((ok_count + 1))
        fi
    elif printf '%s' "$WL_ERR" | grep -q 'no Dockerfile exists'; then
        # Still refused with a Dockerfile seeded: its build is not this one.
        echo "  OK*    $id (built workload, Dockerfile guard — expected)"
        ok_count=$((ok_count + 1))
    elif printf '%s' "$WL_ERR" | grep -q 'cannot read the artifact of workload.*404 Not Found'; then
        # The workload's artifact was deleted: there is no spec to render, and
        # refusing is right. Account data, not something this suite can fix.
        echo "  OK*    $id (orphaned workload, artifact gone — expected)"
        ok_count=$((ok_count + 1))
    else
        # The error, not a warning printed ahead of it.
        reason="$(printf '%s\n' "$WL_ERR" | grep -m1 '^Error:' || true)"
        echo "  FAIL   $id — ${reason:-$(printf '%s' "$WL_ERR" | head -1)}"
        fail_count=$((fail_count + 1))
    fi
    rm -rf "$d"
done < <("$DR_BIN" workload list --output-format json | jq -r '.workloads[].id')

echo "  sweep result: ${ok_count} OK/OK*, ${fail_count} DIRTY/FAIL"
[[ "$fail_count" -eq 0 ]] || wl::fail "sweep had ${fail_count} failures (regression: null-key leak or autoscaling refusal)"
wl::pass "every workload on the account bound cleanly"

wl::stop_timer
echo "✅ Scenario B passed"
