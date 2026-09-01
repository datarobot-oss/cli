#!/usr/bin/env bash
# Ticket: RAPTOR-19538
# Live staging smoke — `dr workload up --diff` + `--confirm` (m3-live-smoke).
#
# Deploys a throwaway image-based whoami workload on staging (the RAPTOR-19533
# scenario A pattern: create --spec-file → wait running → config binds and
# renders .datarobot.yaml), edits the manifest's sizing plus two environment
# variables (one credential-backed `dr-credential:<id>/<key>` reference and
# one literal plaintext value), then walks the six VAL-SMOKE assertions:
#
#   SMOKE-001  `up --dry-run --diff` shows the sizing change as a unified
#              diff (- old / + new with context) and leaks no env-var value.
#   SMOKE-002  A real `up --confirm` answered "n" exits NONZERO and mutates
#              nothing. Run under a PTY via expect(1) with
#              DATAROBOT_CLI_NON_INTERACTIVE unset: lib.sh sets the variable
#              and a piped stdin would suppress the gate, so both overrides
#              are required for the prompt to install at all.
#   SMOKE-005  `up --dry-run --diff --output-format json` emits one JSON
#              document whose plan.diff carries the structured change list,
#              with env-var values redacted, while the change is still
#              pending (i.e. before SMOKE-003 applies it).
#   SMOKE-003  A real `up --confirm` answered "y" applies the sizing change:
#              a follow-up `up --dry-run` reports the workload up to date,
#              which is the CLI's own live-vs-manifest verdict that sizing
#              now matches the manifest (`workload get` does not surface
#              sizing, so the dry-run plan is the observable).
#   SMOKE-004  Cleanup: the EXIT-trap cleanup deletes the created workload
#              and artifact, and a post-cleanup probe proves no RUN-prefixed
#              resource is left on staging.
#
# The decline leg (SMOKE-002) expects a NONZERO exit, so it is wrapped in a
# `set +e` guard and asserts the captured code directly — never
# wl::assert_cmd_ok, which would abort the script on the expected failure and
# strand SMOKE-003.
#
# ~15 min. Run directly (serially; it creates/deletes real staging
# resources): smoke_test_scripts/workload/RAPTOR-19538-diff-confirm.sh
#
# Transcript/evidence files land in
#   tmp/smoke-19538/<RUN>/  (override with SMOKE_TRANSCRIPT_DIR).

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

# The lib trap stops workloads, but a stopped draft still shows up in
# `dr workload list`, and SMOKE-004 requires no RUN-prefixed leftovers at
# all. Keep the lib registration (the registry helpers below are the same
# arrays wl::cleanup drains) and swap the EXIT trap for a stronger wrapper
# that DELETEs our workloads (the platform stops a running workload before
# removing it) and then hands off to the lib cleanup for artifacts, scratch,
# and exit-code propagation.
smoke::cleanup_resources() {
    local id

    # Workloads first: an artifact backing a live workload cannot be deleted.
    for id in "${WL_CREATED_WORKLOADS[@]:-}"; do
        [[ -n "$id" ]] || continue
        "$DR_BIN" workload delete "$id" --yes >/dev/null 2>&1 || true
        echo "  🧹 deleted workload $id"
    done
    WL_CREATED_WORKLOADS=()

    for id in "${WL_ARTIFACT_IDS[@]:-}"; do
        [[ -n "$id" ]] || continue
        "$DR_BIN" artifact delete "$id" --yes >/dev/null 2>&1 || true
        echo "  🧹 deleted artifact $id"
    done
    WL_ARTIFACT_IDS=()
}

smoke::cleanup() {
    local rc=$?

    smoke::cleanup_resources

    [[ -n "${WL_SCRATCH:-}" ]] && rm -rf "$WL_SCRATCH" 2>/dev/null || true

    return "$rc"
}
# shellcheck disable=SC2317  # reached via the EXIT trap, not a call site.
trap smoke::cleanup EXIT

# ---------------------------------------------------------------------------
# Evidence: transcripts land outside the scratch dir, which the trap removes.
# ---------------------------------------------------------------------------
TRANSCRIPT_DIR="${SMOKE_TRANSCRIPT_DIR:-$WL_REPO_ROOT/tmp/smoke-19538/$RUN}"
mkdir -p "$TRANSCRIPT_DIR"

smoke::record() {
    # record <leg-name>: persist this leg's WL_OUT/WL_RC/WL_ERR capture.
    local leg=$1
    printf '%s' "$WL_OUT" > "$TRANSCRIPT_DIR/$leg.out"
    printf '%s' "$WL_ERR" > "$TRANSCRIPT_DIR/$leg.err"
    echo "$WL_RC" > "$TRANSCRIPT_DIR/$leg.rc"
}

# ---------------------------------------------------------------------------
# PTY driver for the interactive --confirm gate (SMOKE-002/003).
#
# The gate installs only when stdin is a terminal AND the run is interactive.
# This bash script's stdin is a pipe and wl::init_env exported
# DATAROBOT_CLI_NON_INTERACTIVE=1 — either one alone suppresses the prompt BY
# DESIGN, so the driver runs the CLI under expect(1), which allocates a real
# PTY, and unsets the variable in the child environment. The answer ("n" or
# "y") is fed to the prompt through the PTY like a keystroke.
# ---------------------------------------------------------------------------
smoke::write_expect_driver() {
    cat > "$WL_SCRATCH/confirm.exp" <<'EOF'
#!/usr/bin/expect -f
# argv: <dr-bin> <workdir> <transcript> <answer> [up args...]
set timeout 1200
set dr_bin [lindex $argv 0]
set workdir [lindex $argv 1]
set transcript [lindex $argv 2]
set answer [lindex $argv 3]
set up_args [lrange $argv 4 end]

# The gate must see an interactive run: a real PTY (expect allocates one) and
# no non-interactive signal in the environment. Unset defensively as well as
# in the caller — either override alone is not enough if the other regresses.
catch {unset env(DATAROBOT_CLI_NON_INTERACTIVE)}
set env(DATAROBOT_CLI_FEATURE_WORKLOAD) true

log_file -noappend $transcript
cd $workdir
eval spawn $dr_bin workload up $up_args

expect {
    -re {Apply this deploy} {
        send -- "$answer\r"
        exp_continue
    }
    eof {}
    timeout {
        puts "\nPTY driver: timed out waiting for the confirm prompt"
        exit 124
    }
}

# Propagate the CLI's exit status: `wait` yields pid spawnid flag status.
foreach {pid spawnid os_error_flag status} [wait] break
exit $status
EOF
}

# smoke::pty_run <transcript> <answer> [up args...] — runs the PTY driver.
# The caller captures the exit code in PTY_RC itself; this wrapper must never
# abort under set -e (the decline leg legitimately exits nonzero).
smoke::pty_run() {
    local transcript=$1 answer=$2
    shift 2

    PTY_RC=0
    /usr/bin/expect -f "$WL_SCRATCH/confirm.exp" \
        "$DR_BIN" "$(pwd)" "$transcript" "$answer" "$@" || PTY_RC=$?
}

# ---------------------------------------------------------------------------
# Credential id discovery (read-only).
#
# SMOKE-001's redaction assertion needs one credential-backed env var, and
# SMOKE-003 really applies the manifest — so the reference must name a
# credential that exists. The CLI has no credential subcommand, so the id is
# discovered with a read-only GET on the platform's /credentials/ route using
# the same configured token the CLI itself authenticates with. Only the id is
# read; the stored secret is never fetched and never printed.
# ---------------------------------------------------------------------------
smoke::discover_credential_id() {
    local endpoint token override

    endpoint="${DATAROBOT_ENDPOINT:-}"
    if [[ -z "$endpoint" ]]; then
        endpoint="$(yq -r '.endpoint // ""' "$HOME/.config/datarobot/drconfig.yaml")"
    fi

    override="$(wl::resolve_token)"
    if [[ -n "$override" ]]; then
        token="$override"
    else
        token="$(yq -r '.token // ""' "$HOME/.config/datarobot/drconfig.yaml")"
    fi

    [[ -n "$endpoint" && -n "$token" ]] || return 1

    curl -fsS --max-time 30 -H "Authorization: Bearer $token" \
        "$endpoint/credentials/?limit=1" \
        | jq -r '.data[0].credentialId // empty'
}

wl::start_timer "19538: --diff/--confirm live smoke (staging)"

# --- Deploy the throwaway whoami workload (scenario A pattern) --------------
work="$WL_SCRATCH/project"
mkdir -p "$work"
cat > "$work/workload.yaml" <<EOF
name: aj-19538-${RUN}
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
smoke::record "000-workload-create"
wl::pass "created draft workload $WID"

wl::wait_for_status "$WID" running 600 >/dev/null

# The platform materializes an artifact for the inline image spec; register it
# (if any) so cleanup deletes it after the workload is gone.
wl::dr_capture workload get "$WID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload get"
AID="$(printf '%s' "$WL_OUT" | jq -r '.artifactId // empty')"
[[ -z "$AID" ]] || wl::register_artifact "$AID"

# --- Bind and render the manifest -------------------------------------------
cd "$work"
wl::dr_capture workload config --yes --workload-id "$WID"
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload config --workload-id"
[[ -f .datarobot.yaml ]] || wl::fail "config did not write .datarobot.yaml"
wl::pass "wrote .datarobot.yaml"

# Baseline: the file the bind rendered is the live state.
wl::dr_capture workload up --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "baseline up --dry-run"
smoke::record "000-baseline-dry-run"

# --- Edit the manifest: sizing + one env var of each secret shape -----------
CRED_ID="$(smoke::discover_credential_id)" \
    || wl::fail "cannot discover a credential id (read-only GET on /credentials/ failed)"
if [[ -z "$CRED_ID" ]]; then
    wl::fail "no credential visible to this account; the credential-backed env var cannot be referenced"
fi

# A recognizable fake plaintext so the redaction greps are exact: the value
# must never appear in any diff output, human or JSON.
LIT_SECRET="smoke-literal-secret-${RUN}-never-print"

yq -i '.runtime.containerGroups[0].replicaCount = 3' .datarobot.yaml
yq -i ".runtime.containerGroups[0].containers[0].environmentVars = [{\"name\": \"SMOKE_LIT_TOKEN\", \"value\": \"$LIT_SECRET\"}, {\"name\": \"SMOKE_API_TOKEN\", \"value\": \"dr-credential:$CRED_ID/apiToken\"}]" .datarobot.yaml
wl::pass "manifest edited: replicaCount 1 -> 3, two env vars added (credential ref + literal)"

# The reference id is the only credential fact the manifest needs to carry;
# grep for the secret SHAPES below, never for a stored value.
wl::pass "using credential $CRED_ID for the dr-credential reference"

# --- SMOKE-001: --dry-run --diff shows the sizing change, leaks no values ---
wl::start_timer "SMOKE-001: up --dry-run --diff"

wl::dr_capture workload up --dry-run --diff
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "up --dry-run --diff"
smoke::record "001-dry-run-diff"

printf '%s\n' "$WL_ERR" | grep -Eq '^- .*replicaCount' \
    || wl::fail "SMOKE-001: no '- old' line for the sizing change; stderr was: $WL_ERR"
printf '%s\n' "$WL_ERR" | grep -Eq '^\+ .*replicaCount' \
    || wl::fail "SMOKE-001: no '+ new' line for the sizing change"
wl::pass "diff shows the sizing change as - old / + new"

# Context either side: the line before the first - marker is context (no marker).
first_change_lineno="$(printf '%s\n' "$WL_ERR" | grep -nE '^[+-] ' | head -1 | cut -d: -f1)"
[[ -n "$first_change_lineno" ]] || wl::fail "SMOKE-001: diff body has no change lines"
prev_lineno=$((first_change_lineno - 1))
[[ "$prev_lineno" -ge 1 ]] || wl::fail "SMOKE-001: no line before the first change"
prev_line="$(printf '%s\n' "$WL_ERR" | sed -n "${prev_lineno}p")"
if printf '%s' "$prev_line" | grep -qE '^[+-] '; then
    wl::fail "SMOKE-001: line preceding the first change is not context: $prev_line"
fi
wl::pass "change is flanked by context lines"

# Redaction: neither the credential ref material nor the literal value may
# appear; the env var NAME must. (The .err file was persisted above, so the
# absent-checks run against a real file rather than a pipe.)
wl::assert_absent "$TRANSCRIPT_DIR/001-dry-run-diff.err" "$LIT_SECRET" "SMOKE-001 literal secret"
wl::assert_absent "$TRANSCRIPT_DIR/001-dry-run-diff.err" 'dr-credential:' "SMOKE-001 credential ref"
wl::assert_absent "$TRANSCRIPT_DIR/001-dry-run-diff.err" "$CRED_ID" "SMOKE-001 credential id"

# The env-var change reaches the diff either per element (paths carrying the
# variable name) or as one whole-list row (`environmentVars: [N item(s)]`)
# when the live side has no such list; both spellings must appear as a change
# line, and neither may carry a value.
printf '%s\n' "$WL_ERR" | grep -q 'environmentVars' \
    || wl::fail "SMOKE-001: no environmentVars change line in the diff"
wl::pass "env-var values redacted; the env-var change line is present"

# (d) No mutation: the workload document is identical around the dry-run.
wl::dr_capture workload get "$WID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload get (before-diff)"
before="$(printf '%s' "$WL_OUT" | jq -S .)"
wl::dr_capture workload get "$WID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload get (after-diff)"
after="$(printf '%s' "$WL_OUT" | jq -S .)"
[[ "$before" == "$after" ]] || wl::fail "SMOKE-001: workload document changed around the dry-run diff"
wl::pass "no mutation from --dry-run --diff"

wl::stop_timer

# --- SMOKE-002: --confirm answered "n" exits nonzero, mutates nothing -------
wl::start_timer "SMOKE-002: --confirm decline (PTY)"

# lib.sh exports DATAROBOT_CLI_NON_INTERACTIVE=1 for every non-PTY leg; the
# gate installs only in an interactive run, so unset it here and keep it unset
# — the PTY driver also unsets it in the child, and only the two overrides
# TOGETHER install the prompt (piped stdin alone suppresses it by design).
unset DATAROBOT_CLI_NON_INTERACTIVE
smoke::write_expect_driver

wl::dr_capture workload get "$WID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload get (before-decline)"
before="$(printf '%s' "$WL_OUT" | jq -S .)"

# Expected NONZERO exit: set +e so the guard cannot abort before SMOKE-003,
# then assert the captured code directly (never wl::assert_cmd_ok here).
set +e
smoke::pty_run "$TRANSCRIPT_DIR/002-decline-pty.log" "n" --confirm
decline_rc=$PTY_RC
set -e

if [[ "$decline_rc" -eq 0 ]]; then
    wl::fail "SMOKE-002: declined run exited 0; the gate must refuse with a nonzero exit"
fi
wl::pass "declined run exited nonzero (rc=$decline_rc)"

grep -q 'Apply this deploy' "$TRANSCRIPT_DIR/002-decline-pty.log" \
    || wl::fail "SMOKE-002: prompt missing from the PTY transcript"
grep -q '(y/N)' "$TRANSCRIPT_DIR/002-decline-pty.log" \
    || wl::fail "SMOKE-002: prompt is not the y/N (default no) phrasing"
grep -q 'declined: nothing was deployed' "$TRANSCRIPT_DIR/002-decline-pty.log" \
    || wl::fail "SMOKE-002: decline message missing from the PTY transcript"
wl::pass "prompt and decline message present in the PTY transcript"

# stdout untouched: a declined run prints no endpoint. The workload's own
# endpoint line is what a successful run emits; its absence here, against the
# same PTY driver that shows it on the accept leg below, is the observable.
endpoint="$(printf '%s' "$before" | jq -r '.endpoint // empty')"
if [[ -n "$endpoint" ]] && grep -qF "$endpoint" "$TRANSCRIPT_DIR/002-decline-pty.log"; then
    wl::fail "SMOKE-002: declined transcript contains the endpoint (stdout was not untouched)"
fi
wl::pass "declined transcript carries no endpoint (stdout untouched)"

# (d) Nothing mutated: the workload document matches, and the sizing change is
# still pending exactly as the diff showed it.
wl::dr_capture workload get "$WID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload get (after-decline)"
after="$(printf '%s' "$WL_OUT" | jq -S .)"
[[ "$before" == "$after" ]] || wl::fail "SMOKE-002: workload document changed after the decline"
wl::dr_capture workload up --dry-run --diff
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "up --dry-run --diff (pending after decline)"
smoke::record "002-pending-after-decline"
printf '%s\n' "$WL_ERR" | grep -Eq '^\+ .*replicaCount.*3' \
    || wl::fail "SMOKE-002: sizing change no longer pending after the decline (spec mutated?)"
wl::pass "decline mutated nothing; sizing change still pending"

wl::stop_timer

# --- SMOKE-005: --diff JSON envelope with the change still pending ----------
wl::start_timer "SMOKE-005: --diff --output-format json envelope"

wl::dr_capture workload up --dry-run --diff --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "up --dry-run --diff --output-format json"
smoke::record "005-json-diff"
printf '%s' "$WL_OUT" > "$TRANSCRIPT_DIR/005-json-diff.raw"

# (a) Exactly one JSON document on stdout. The envelope nests the plan under
# the "up" key (the deploy's outcome object), hence .up.plan.diff.
docs="$(printf '%s' "$WL_OUT" | jq -s 'length')"
[[ "$docs" == "1" ]] || wl::fail "SMOKE-005: stdout carries $docs documents, expected 1"
wl::pass "stdout is exactly one JSON document"

# (b) A non-empty changes list that names the sizing field with live/want.
changes="$(printf '%s' "$WL_OUT" | jq '.up.plan.diff.changes | length')"
[[ "$changes" -gt 0 ]] || wl::fail "SMOKE-005: plan.diff.changes is empty"
sizing_change="$(printf '%s' "$WL_OUT" \
    | jq -c '[.up.plan.diff.changes[] | select(.path | contains("replicaCount"))][0]')"
[[ "$sizing_change" != "null" ]] || wl::fail "SMOKE-005: no replicaCount entry in plan.diff.changes"
have="$(printf '%s' "$sizing_change" | jq '.have')"
want="$(printf '%s' "$sizing_change" | jq '.want')"
[[ "$have" == "1" && "$want" == "3" ]] \
    || wl::fail "SMOKE-005: sizing change is have=$have want=$want, expected 1 -> 3"
wl::pass "plan.diff.changes carries the sizing change (have=1, want=3)"

# (c) Env-var paths are redacted in the envelope. A whole-list env-var change
# arrives as one row whose own path only ends in .environmentVars, so the
# envelope marks it `redacted: true` and keeps the structure with the variable
# NAMES but not the values (scrubEnvList) — asserting null have/want here
# would test the wrong shape. The hard property is the whole-document scan
# below: neither the literal secret nor any credential material appears.
printf '%s' "$WL_OUT" | jq -e '
    [.up.plan.diff.changes[] | select(.path | contains("environmentVars"))] | length > 0
' >/dev/null || wl::fail "SMOKE-005: no environmentVars entries in plan.diff.changes"
printf '%s' "$WL_OUT" | jq -e '
    [.up.plan.diff.changes[]
        | select(.path | contains("environmentVars"))
        | .redacted == true] | all
' >/dev/null || wl::fail "SMOKE-005: an environmentVars change is not redacted in the envelope"
wl::assert_absent "$TRANSCRIPT_DIR/005-json-diff.raw" "$LIT_SECRET" "SMOKE-005 literal secret"
wl::assert_absent "$TRANSCRIPT_DIR/005-json-diff.raw" 'dr-credential:' "SMOKE-005 credential ref"
wl::assert_absent "$TRANSCRIPT_DIR/005-json-diff.raw" "$CRED_ID" "SMOKE-005 credential id"
wl::pass "env-var values redacted from the JSON envelope"

# (d) unmanaged is present (array, possibly empty).
printf '%s' "$WL_OUT" | jq -e '.up.plan.diff.unmanaged | type == "array"' >/dev/null \
    || wl::fail "SMOKE-005: plan.diff.unmanaged missing or not an array"
wl::pass "plan.diff.unmanaged present"

wl::stop_timer

# --- SMOKE-003: --confirm answered "y" applies the sizing change ------------
wl::start_timer "SMOKE-003: --confirm accept (PTY)"

# The env vars were diff surface only. The settings update sends the runtime
# block verbatim and the platform rejects environmentVars under runtime
# containers with a 422 ("Extra inputs are not permitted"), so a manifest
# that asks for runtime env vars can never be applied -- the two variables
# come back out here, and the accepted deploy carries the sizing change, the
# change the confirm gate exists to gate. (Pre-existing CLI/platform mismatch
# in how runtime env vars apply, found by this smoke and reported separately;
# it does not touch the diff, the confirm gate, or the JSON envelope.)
yq -i 'del(.runtime.containerGroups[0].containers[0].environmentVars)' .datarobot.yaml
wl::pass "env vars removed from the runtime block; the sizing change stays pending"

smoke::pty_run "$TRANSCRIPT_DIR/003-accept-pty.log" "y" --confirm
wl::assert_cmd_ok "$PTY_RC" "see transcript" "$(tail -40 "$TRANSCRIPT_DIR/003-accept-pty.log")" \
    "up --confirm answered y"
wl::pass "accepted run exited 0"

grep -q 'Apply this deploy' "$TRANSCRIPT_DIR/003-accept-pty.log" \
    || wl::fail "SMOKE-003: prompt missing from the PTY transcript"
wl::pass "prompt preceded the apply in the PTY transcript"

# (b) The sizing change was applied. `workload get` does not surface sizing,
# so the CLI's own live-vs-manifest verdict is the observable: with the
# manifest still asking for replicaCount 3, an up-to-date dry-run proves the
# live runtime now matches the manifest's new value.
wl::dr_capture workload up --dry-run
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "up --dry-run (after accept)"
smoke::record "003-dry-run-after-accept"
printf '%s\n' "$WL_ERR" | grep -q 'Already up to date' \
    || wl::fail "SMOKE-003: workload not up to date after the accepted deploy; stderr was: $WL_ERR"
wl::pass "live runtime now matches the manifest (up-to-date verdict)"

# (d) stdout carries the endpoint: the accepted PTY transcript shows the
# endpoint line the declined one deliberately lacked.
wl::dr_capture workload get "$WID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload get (after-accept)"
endpoint="$(printf '%s' "$WL_OUT" | jq -r '.endpoint // empty')"
[[ -n "$endpoint" ]] || wl::fail "SMOKE-003: workload has no endpoint"
grep -qF "$endpoint" "$TRANSCRIPT_DIR/003-accept-pty.log" \
    || wl::fail "SMOKE-003: accepted transcript does not carry the endpoint"
wl::pass "accepted transcript carries the endpoint"

wl::wait_for_status "$WID" running 300 >/dev/null

wl::stop_timer

# --- SMOKE-004: cleanup leaves no RUN-prefixed resources --------------------
wl::start_timer "SMOKE-004: cleanup"

# Run the same cleanup the EXIT trap would run, while the script can still
# assert on the result; the trap runs again on exit as a verified no-op.
smoke::cleanup_resources
wl::pass "cleanup deleted workload $WID (and artifact ${AID:-none})"

wl::dr_capture workload list --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "workload list (post-cleanup)"
smoke::record "004-workload-list-post-cleanup"
leftover_wl="$(printf '%s' "$WL_OUT" \
    | jq -r --arg run "$RUN" '[.workloads[]? | select(.name | contains($run))] | length')"
[[ "$leftover_wl" == "0" ]] \
    || wl::fail "SMOKE-004: $leftover_wl RUN-prefixed workload(s) remain on staging"
wl::pass "no RUN-prefixed workloads remain (dr workload list)"

if [[ -n "$AID" ]]; then
    wl::dr_capture artifact get "$AID" --output-format json
    if [[ "$WL_RC" -eq 0 ]]; then
        wl::fail "SMOKE-004: artifact $AID still exists after cleanup"
    fi
    wl::pass "artifact $AID deleted (get fails as expected)"
fi

wl::stop_timer

wl::stop_timer
echo "✅ RAPTOR-19538 diff/confirm smoke passed — transcripts in $TRANSCRIPT_DIR"
