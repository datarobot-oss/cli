#!/usr/bin/env bash
# Ticket: none — basic acceptance
# Artifact scenario — `dr artifact` lifecycle (CLI-side focus).
#
# create → get → list → code init → code sync → code versions → del.
#
# `dr artifact lock` is deliberately NOT exercised here: a draft with
# imageBuildConfig cannot be locked until an image build completes (server
# 422 "Trigger and complete an image build before locking"), and an
# image-based artifact, while lockable without a build, becomes
# non-deletable once locked (server 409 "Cannot delete locked artifact")
# with no CLI unlock path — so automating lock leaks a permanent locked
# artifact on every run. Lock is covered manually / via the opt-in
# Scenario D build path. See COVERAGE.md.
#
# This complements workload-api/tests/acceptance (which owns API behavior):
# here we own CLI-side state — the .datarobot/workload/manifest.json sync
# index, the .drignore file, JSON output purity, and exit codes. The
# expensive `artifact build` image-build path is covered indirectly by
# Scenario D rather than a separate build run here.
#
# ~5-10 min. Source: tmp/workload-verify-runbook.md + workload-api cli_helpers.

# shellcheck shell=bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

wl::init_env
wl::register_cleanup

wl::start_timer "Artifact: create/get/list/code/sync/versions/del lifecycle"

# --- Create a draft artifact from an inline service spec --------------------
work="$WL_SCRATCH/project"
mkdir -p "$work"
name="wl-art-${RUN}"
cat > "$work/spec.json" <<EOF
{
  "name": "${name}",
  "type": "service",
  "spec": {
    "containerGroups": [
      {
        "name": "default",
        "containers": [
          {
            "name": "primary",
            "imageBuildConfig": { "dockerfile": { "path": "./Dockerfile", "source": "provided" } },
            "port": 8080,
            "primary": true,
            "readinessProbe": { "path": "/", "port": 8080 }
          }
        ]
      }
    ]
  }
}
EOF

wl::dr_capture artifact create --spec-file "$work/spec.json" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact create"
AID="$(printf '%s' "$WL_OUT" | jq -r '.id')"
wl::pass "created draft artifact $AID"

# Cleanup: best-effort HTTP delete via the CLI's own delete subcommand.
trap 'wl::dr_capture artifact delete "$AID" --yes 2>/dev/null || true' EXIT

# --- get / list round-trip --------------------------------------------------
wl::dr_capture artifact get "$AID" --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact get"
got_id="$(printf '%s' "$WL_OUT" | jq -r '.id')"
got_name="$(printf '%s' "$WL_OUT" | jq -r '.name')"
[[ "$got_id" == "$AID" ]] || wl::fail "get id mismatch: $got_id != $AID"
[[ "$got_name" == "$name" ]] || wl::fail "get name mismatch: $got_name != $name"
wl::pass "artifact get round-trips id + name"

wl::dr_capture artifact list --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact list"
printf '%s' "$WL_OUT" | jq -e --arg id "$AID" '.artifacts[] | select(.id == $id)' >/dev/null \
    || wl::fail "created artifact not present in list"
wl::pass "artifact list contains the created artifact"

# --- code init: links a project dir to the artifact -------------------------
cd "$work"
wl::dr_capture artifact code init "$AID" --dir . --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact code init"
# State dir is .datarobot/workload (with .wapi legacy fallback). The current
# CLI writes .datarobot/workload/manifest.json and a .drignore at the root.
manifest=""
for cand in .datarobot/workload/manifest.json .wapi/manifest.json; do
    [[ -f "$cand" ]] && manifest="$cand" && break
done
[[ -n "$manifest" ]] || wl::fail "code init did not create a manifest.json"
wl::pass "code init created $manifest"
[[ -f .drignore || -f .wapiignore ]] || wl::fail "code init did not create an ignore file"
wl::pass "ignore file present"

# A fresh init has no synced version and an empty files map.
sv="$(jq -r '.syncedVersionId // empty' "$manifest")"
[[ -z "$sv" ]] || wl::fail "fresh init should have null syncedVersionId, got: $sv"
files_empty="$(jq -r '.files | length' "$manifest")"
[[ "$files_empty" == "0" ]] || wl::fail "fresh init should have empty files map, got: $files_empty"
wl::pass "fresh init: null syncedVersionId, empty files map"

# --- code sync: write a Dockerfile and push ---------------------------------
printf 'FROM containous/whoami:latest\nEXPOSE 8080\nCMD ["--port", "8080"]\n' > Dockerfile
wl::dr_capture artifact code sync --dir . --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact code sync"
sv="$(jq -r '.syncedVersionId // empty' "$manifest")"
[[ -n "$sv" ]] || wl::fail "syncedVersionId should be set after sync"
jq -e '.files["Dockerfile"]' "$manifest" >/dev/null || wl::fail "Dockerfile missing from synced files"
wl::pass "code sync set syncedVersionId and synced the Dockerfile"

# --- code versions: the current version matches the manifest ----------------
wl::dr_capture artifact code versions --dir . --output-format json
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact code versions"
versions="$(printf '%s' "$WL_OUT")"
current="$(printf '%s' "$versions" | jq -r 'first(.[]? | select(.isCurrent == true)) | .versionId // empty')"
[[ -n "$current" ]] || wl::fail "no isCurrent entry in versions"
[[ "$current" == "$sv" ]] || wl::fail "current version $current != manifest.syncedVersionId $sv"
wl::pass "current version matches manifest.syncedVersionId"

# --- del: cleanup (best-effort; the EXIT trap also tries) -------------------
# The artifact is still an unlocked draft (no build was triggered), so the
# server allows delete. A locked artifact cannot be deleted (409) — another
# reason lock is not exercised here.
wl::dr_capture artifact delete "$AID" --yes
wl::assert_cmd_ok "$WL_RC" "$WL_OUT" "$WL_ERR" "artifact delete"
wl::pass "artifact deleted"

wl::stop_timer
echo "✅ Artifact scenario passed"
