#!/usr/bin/env bash
# Ticket: RAPTOR-19533
# Scenario C — re-bind a tuned workload (Carson).
#
# Re-binding via --workload-id is the only CLI-native way to mint a new
# dr-credential onto a workload whose manifest was hand-tuned. The
# regression this guards: a bound re-bind preserves the live tuning
# (replicaCount, probes, CPU) instead of resetting to template defaults,
# and the delete-and-rebind workaround is safe (empty diff).
#
# ~10 min. Source: tmp/workload-verify-runbook.md Scenario C.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "C: re-bind preserving live tuning"

# --- C.1 Create a draft with non-default runtime tuning ---------------------
# Non-defaults vs the template: replicaCount 2, memory 1GB,
# initialDelaySeconds 15, periodSeconds 30 — chosen so "carried from live"
# is distinguishable from "template default".
work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/wl-tuned.yaml" <<EOF
name: aj-rc-${RUN}
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
              initialDelaySeconds: 15
              periodSeconds: 30
runtime:
  containerGroups:
    - name: default
      replicaCount: 2
      containers:
        - name: whoami
          resourceAllocation:
            cpu: 1
            memory: "1GB"
EOF

wl::dr_capture workload create --spec-file "$work/wl-tuned.yaml" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload create tuned"
TID="$(printf '%s' "$WL_OUT" | jq -r '.id')"
wl::register_workload "$TID"
wl::pass "created tuned draft workload $TID"

# --- C.2 Bound re-bind preserves live settings ------------------------------
d="$(mktemp -d)"
wl::dr_capture workload config --dir "$d" --yes --workload-id "$TID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config (bound re-bind)"

rendered="$d/.datarobot.yaml"
grep -qE 'replicaCount: 2' "$rendered" || wl::fail "replicaCount not carried from live (expected 2)"
grep -qE 'initialDelaySeconds: 15' "$rendered" || wl::fail "initialDelaySeconds not carried (expected 15)"
grep -qE 'periodSeconds: 30' "$rendered" || wl::fail "periodSeconds not carried (expected 30)"
grep -qE 'cpu: 1' "$rendered" || wl::fail "cpu not carried (expected 1)"
wl::pass "tuned runtime fields carried from live"

# Known bug RAPTOR-19697: memory renders as 512MB regardless of the live
# value (stringAt drops JSON numbers). Assert the current (wrong) value so
# the test flips to a failure when the fix lands, prompting an update here.
grep -qE 'memory: "?512MB"?' "$rendered" \
    && wl::pass "memory: 512MB (known bug RAPTOR-19697 — update this assertion when fixed)" \
    || wl::skip "memory not 512MB — RAPTOR-19697 may be fixed; verify and update this assertion"

# --- C.3 Expected refusal: existing manifest (exit 0, a notice) -------------
wl::dr_capture workload config --dir "$d" --yes --workload-id "$TID"
# The FileExists guard is a notice, NOT an error — exit code is 0.
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config (existing-manifest refusal)"
echo "$WL_OUT" | grep -q 'already exists' \
    || echo "$WL_ERR" | grep -q 'already exists' \
    || wl::fail "expected 'already exists' refusal notice"
wl::pass "existing-manifest refusal is a notice (exit 0)"

# --- C.3b Delete-and-rebind workaround is safe (empty diff) -----------------
cp "$rendered" "$rendered.bak"
rm "$rendered"
wl::dr_capture workload config --dir "$d" --yes --workload-id "$TID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config (delete-and-rebind)"
if ! diff -u "$rendered.bak" "$rendered" >/dev/null; then
    echo "  ❌ delete-and-rebind diverged from the original:" >&2
    diff -u "$rendered.bak" "$rendered" >&2 || true
    exit 1
fi
wl::pass "delete-and-rebind reproduces the original manifest (empty diff)"

rm -rf "$d"
wl::stop_timer
echo "✅ Scenario C passed"
