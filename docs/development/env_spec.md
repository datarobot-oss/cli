# `dr workload env` — implementation specification

This document specifies the `dr workload env {list,set,import,delete}`
capability well enough that it could be re-implemented from scratch, in any
language or CLI framework, without reading this repository's Go code. It
captures the external API contract (as verified live against a running
DataRobot instance — not just as documented, since the two disagree in
places), the required behavior, and a list of corner cases that a naive
implementation gets wrong, each with the reasoning for why it matters.

A working reference implementation exists in this repository:

- `internal/workload/env.go`, `internal/workload/replacement.go`,
  `internal/workload/credential.go`, `internal/workload/artifact.go` (the
  `EnvironmentVar`/`Container` types) — business logic.
- `cmd/workload/env/{list,set,import,del}/cmd.go` — CLI commands.
- `cmd/workload/env/internal/rollout/rollout.go` — the shared deploy-or-stage
  decision used by `set`, `import`, and `delete`.
- `cmd/workload/env/internal/envparse/envparse.go` — the NAME/VALUE parsing
  and validation (§7.7) shared by `set` and `import`, so the two can never
  drift on what counts as a valid name or a valid credential reference.

Treat this spec as authoritative over that code if they ever disagree — the
code should be brought back in line, not the other way around.

## 1. Problem statement

A DataRobot **workload** is a running container service. It doesn't carry
environment variables directly — it points at an **artifact**, an
immutable-once-locked definition of what to run (image, port, probes, and
env vars). Changing a workload's env vars means:

1. Finding the artifact the workload currently runs.
2. Editing that artifact's env vars — in place if it's a mutable draft, or via
   a clone if it's locked (locking is one-way and irreversible).
3. Rolling the workload onto the edited artifact via a "replacement" (a
   rolling redeploy).

Doing this by hand means juggling three-plus API calls, a non-obvious
draft-vs-locked branch, and a replacement endpoint with sharp edges (see
§3.6). The goal of `dr workload env` is to make "set an env var" one command
instead of that whole dance.

## 2. Terminology

| Term | Meaning |
| --- | --- |
| Workload | The running service resource. Has a status (`running`, `stopped`, `errored`, ...) and points at exactly one artifact via `artifactId`. |
| Artifact | The versioned definition of what a workload runs: image, port, probes, env vars. Starts `draft` (mutable), can be locked to `locked` (immutable, versioned, **never deletable again**). |
| Primary container | Within an artifact's `spec.containerGroups[].containers[]`, the one container flagged `"primary": true` (or, if none is flagged, `containerGroups[0].containers[0]`). This spec's commands only ever read/write this container's env vars. |
| Replacement | A rolling redeploy: swap the workload's running artifact for a different one, with zero (or controlled) downtime. Triggered explicitly, tracked as its own sub-resource on the workload. |
| Clone | A new draft artifact created from an existing one via a dedicated endpoint, carrying the same spec. The only way to get an editable copy of a locked artifact. |
| Stage (this spec's own term) | Prepare an edit (patch or clone+patch) without triggering a replacement — the artifact is ready but the workload keeps running whatever it was running before. |

## 3. External API contract (live-verified)

Everything in this section was confirmed by direct HTTP calls against a real
instance, not just read from documentation — several details below
contradict what the available reference docs say. Where they disagree, trust
this section.

### 3.1 Artifact resource

`GET /api/v2/artifacts/{id}/`:

```json
{
  "id": "6a2ac560dfb21a499be7632c",
  "name": "my-artifact",
  "status": "draft",
  "version": null,
  "spec": {
    "containerGroups": [
      {
        "name": "default",
        "containers": [
          {
            "name": "main",
            "primary": true,
            "port": 8080,
            "imageUri": "057600194195.dkr.ecr.../service-artifact:...",
            "environmentVars": [
              { "source": "string", "name": "FOO", "value": "bar" },
              { "source": "dr-credential", "name": "API_KEY",
                "drCredentialId": "64f0a1b2c3d4e5f6a7b8c9d0", "key": "apiToken" }
            ],
            "imageBuildConfig": { "...": "..." }
          }
        ]
      }
    ]
  }
}
```

**Correction to watch for:** `environmentVars` lives at
`spec.containerGroups[].containers[].environmentVars` — **per container**,
not on the artifact root. A naive read of some example payloads suggests a
top-level field; it does not exist. Every operation in this spec targets the
*primary container's* array specifically (§2, §6.2).

**Quirk:** a plain var's `source` field defaults to the literal string
`"string"` when omitted on write — the server always fills it in on read.
Write-side code does not need to set it explicitly for plain vars.

### 3.2 Environment variable entry shapes

Two shapes coexist in the same `environmentVars` array:

```json
// Plain (literal value, lives in plaintext in the artifact spec)
{ "name": "LOG_LEVEL", "value": "debug" }

// Credential-backed (value resolved server-side at runtime, never stored here)
{ "source": "dr-credential", "name": "API_KEY",
  "drCredentialId": "<credential id>", "key": "<field name>" }
```

For the credential-backed shape:

- `drCredentialId` is the id of a stored credential (`GET /api/v2/credentials/`).
- `key` selects **which field** of that credential to use — a single stored
  credential can bundle several secret fields. This is unrelated to the env
  var's own `name`. Example: an `s3`-type credential exposes `awsAccessKeyId`,
  `awsSecretAccessKey`, `awsSessionToken` as separate `key` values you can
  wire to different env vars from the same credential.

Partial credential-type → valid-`key` table (fetch a credential's
`credentialType` via `GET /api/v2/credentials/{id}/` and look it up):

| `credentialType` | valid `key` values |
| --- | --- |
| `s3` | `awsAccessKeyId`, `awsSecretAccessKey`, `awsSessionToken` |
| `basic` | `user`, `password` |
| `api_token` | `apiToken` |
| `bearer` | `token` |
| `oauth` | `token`, `refreshToken` |
| `gcp` | `gcpKey` |
| `azure_service_principal` | `azureTenantId`, `clientId`, `clientSecret` |
| `azure` | `azureConnectionString` |
| `databricks_access_token_account` | `databricksAccessToken` |
| `snowflake_key_pair_user_account` | `privateKeyStr`, `passphrase`, `user` |

This table is hand-maintained and can drift from the platform's real schema.
**Do not build client-side validation against it** (§7.7) — it's here only
as a reference for building `set` command syntax/examples.

### 3.3 Writing to a draft artifact

`PATCH /api/v2/artifacts/{id}/` with body `{"spec": {...}}`.

- Only legal while the artifact's `status` is `draft`. A locked artifact
  rejects with `403 {"detail": "Cannot update artifact: artifact is locked"}`.
- **The server replaces the entire `containerGroups` array on write.** There
  is no per-container or per-field merge. You must: `GET` the artifact,
  mutate only the target container's `environmentVars` in your local copy,
  and PATCH the *whole* `spec` back — including every other container and
  container group untouched. Sending a partial spec silently drops whatever
  you omitted.
- Do not include `spec.type` in the PATCH body — it's a read-only
  discriminator the write model rejects.
- No optimistic concurrency control exists on this endpoint (no ETag, no
  version field on drafts — only locked artifacts carry a `version` number).
  See §7.1.

### 3.4 Cloning a locked artifact

`POST /api/v2/artifacts/{id}/clone/`

Request body: `{"name": "<new artifact name>"}` — this is the **only**
required field (verified via a `422` on an empty body:
`{"detail":[{"path":"name","message":"Field required","code":"missing"}]}`).

Response: a full new artifact document, `status: "draft"`, with the source
artifact's `spec` copied verbatim (including its existing `environmentVars`
— important, since your mutation needs to build on top of what's already
there, not start from empty).

This is the *only* way to get an editable copy of a locked artifact — locking
is one-way (§3.5).

### 3.5 Locking an artifact

`PATCH /api/v2/artifacts/{id}/` with body `{"status": "locked"}`.

- **One-way.** A locked artifact can never be unlocked, edited, or deleted
  (`DELETE` on a locked artifact is rejected). Locking assigns it a `version`
  number.
- Locking an already-locked artifact is rejected (`403`).
- The server validates build completeness before locking: a container built
  from source must have a completed image build, or locking is rejected with
  a `422` naming what's missing.
- **Do not lock eagerly.** See §7.4 for why locking must be deferred until
  the moment a rollout is actually about to happen, not done as part of
  "prepare this edit."

### 3.6 Replacement (the rolling redeploy)

`GET /api/v2/workloads/{id}/replacement/` — the current in-flight
replacement, if any.

- **404** when none is active:
  `{"detail": "There is no active replacement for this workload."}`. This is
  the expected "nothing to guard against" answer, not an error condition.
- **200** response shape (live-verified):

```json
{
  "id": "6a613c811e179af8abc1108b",
  "name": "Replacement for workload ... - rolling strategy artifact ...",
  "createdAt": "2026-07-22T21:56:17.193000+00:00",
  "updatedAt": "2026-07-22T21:56:17.193000+00:00",
  "workloadId": "6a613c3ccd7cc265591abc39",
  "candidateArtifactId": "6a613c3ccd7cc265591abc38",
  "candidateProtonIds": ["6a613c8145ecd0232413232d"],
  "status": "submitted",
  "strategy": "rolling",
  "config": { "warmupDurationMinutes": 0, "keepOldVersionMinutes": 0 },
  "runtime": { "...": "..." },
  "healthCheckErrorCount": 0
}
```

**Correction to watch for — asymmetric field naming:** the *read* shape names
the target artifact `candidateArtifactId`. The *write* (POST, below) request
body uses `artifactId` instead. These are not the same field name on the two
sides of the same resource. Getting this wrong (e.g. assuming the GET
response also uses `artifactId`) silently produces an empty string where the
target artifact id should be.

`POST /api/v2/workloads/{id}/replacement/` — starts a rolling replacement.

Request body: `{"artifactId": "<target artifact id>", "strategy": "rolling"}`
(`"strategy"` currently only supports `"rolling"`; an optional `config` block
with `warmupDurationMinutes`/`keepOldVersionMinutes` and an optional
`runtime` override are accepted but not required).

- **Status-match rule (undocumented in the schema, enforced server-side):**
  rejected `400` unless the target artifact's `status` matches the currently
  *running* artifact's status: draft↔draft, locked↔locked. This is exactly
  why draft edits patch in place (still a draft, matches) and locked edits
  clone-then-lock (produces a locked artifact, matches) before this call.
- **Not idempotent.** Calling this while a replacement is already in flight
  for the workload *queues a second swap* rather than rejecting. Callers
  must `GET` first and treat a non-404 as "don't POST" (§7.3).

**Status vocabulary (live-verified, broader than commonly documented):**
non-terminal statuses observed include `submitted`, `initializing` (and
`candidate-warming`/`switching` per platform documentation, not directly
observed in testing but plausible intermediate states); terminal statuses are
`completed` (success) and **both** `failed` **and** `errored` (failure — see
§7.6, `errored` is not mentioned in some reference docs but was directly
observed as a genuine terminal failure state).

On failure, the workload reverts to the artifact it was running before the
replacement started.

### 3.7 Credentials

`GET /api/v2/credentials/{id}/` — a single stored credential.

- **200**: `{"credentialId": "...", "name": "...", "credentialType": "s3", "description": "", "creationDate": "...", "storageMode": "...", "secretReferences": null}`
- **404**: `{"message": "The credential item: <id> is not found"}`

Use this endpoint to validate a `drCredentialId` reference exists before
writing it into an artifact (§7.7) — existence-check only, do not attempt to
validate `key` against §3.2's table (drift risk).

## 4. Command surface

Four subcommands, operating only on the primary container (§2). Multi-
container (sidecar) support is an explicit non-goal for v1 (§8).

### 4.1 `env list <workload-id>`

Resolves workload → artifact, reads the primary container's
`environmentVars`, and displays them.

- Plain vars: display the literal value. It's already plaintext in the
  artifact spec, so withholding it in a human-readable listing adds no real
  protection — it's redundant with a machine-readable dump of the same data.
- Credential-backed vars: display `NAME` plus the reference
  (`drCredentialId`/`key`), **never** a resolved secret value — there is no
  secret value to resolve; it lives only inside the stored credential.
- A machine-readable output mode (JSON or similar) should emit the full
  entry shape verbatim (§3.2), always as an array (empty array, not `null`,
  when there are no vars) so scripted consumers don't need a null-check.

### 4.2 `env set <workload-id> NAME=VALUE [NAME=VALUE ...] [--stage] [--yes] [--wait]`

- Every `NAME=VALUE` pair is a plain var; `NAME=dr-credential:<id>/<key>` (or
  an equivalent syntax of your choosing) is a credential-backed var.
- **Accepts multiple pairs in one invocation** — this is not a nicety, it's
  load-bearing (§7.2).
- Upserts by name: an existing name is replaced in place (preserving array
  order); a new name is appended.
- See §6 for the resolve/mutate/rollout algorithm and §7 for required
  validation and error-handling behavior.

### 4.3 `env delete <workload-id> NAME [NAME ...] [--stage] [--yes] [--wait]`

- Removes each given name from the primary container's `environmentVars`.
- Accepts multiple names per invocation, same batching rationale as `set`.
- Deleting a name that isn't currently set is a no-op *for that name*; the
  command should still error if **none** of the given names were present
  (distinguishes "nothing to do" from a likely typo) — but see §7.2 for why
  this check must happen *before* any artifact is touched.

### 4.4 `env import <workload-id> [--file <path>] [--stage] [--yes] [--wait]`

Loads variables from a file instead of positional arguments, then applies
them exactly like `set` would — same validation, same upsert, same rollout.
This is not a separate code path from `set` so much as a different *source*
of the same `[NAME]→[VALUE or credential reference]` list:

- **Default source: `.env` in the current directory.** `--file <path>`
  overrides it.
- **Parse with standard dotenv syntax:** blank lines and `#`-prefixed
  comments ignored, values may be quoted (a well-tested library dependency
  is the pragmatic choice here over a hand-rolled parser — don't reinvent
  dotenv quoting/escaping rules).
- **Recognize the identical `NAME=dr-credential:<id>/<key>` value syntax**
  `set` does, applied to each parsed value — a credential reference in a
  `.env` file must be validated exactly like one typed on the command line
  (§7.7). Do not special-case file-sourced values as "more trusted."
- **Every variable found in the file is applied together in one call** —
  this falls out naturally from "load the whole file, then upsert," but call
  it out explicitly: it's what makes `import` immune to the
  cross-invocation batching gap described in §7.2, *for a single import*.
  Running `import` twice in a row still has the same batching limitation as
  running `set` twice in a row.
- **Merge semantics: the file's value wins on a name collision** with
  whatever the workload's artifact currently has. This requires no special
  logic beyond what `set` already needs — it's the same upsert-by-name
  operation (§4.2), just fed a different list. A name in the file that
  doesn't yet exist on the workload is simply added; a name that already
  exists is overwritten; every other existing name is left untouched.
- **An empty or all-comments file is an error, not a silent no-op** — it's
  far more likely to mean "wrong file" or "empty by mistake" than "the user
  really meant to do nothing."
- The upfront in-flight-replacement guard (§7.3), the never-lock-a-staged-
  clone rule (§7.4), and the orphaned-artifact-naming requirement (§7.5) all
  apply identically — none of them are specific to how the var list was
  sourced.

### 4.5 Common flags

- `--stage`: prepare the edit (patch, or clone+patch) but do not roll it out.
  No confirmation prompt is needed in this mode (§6.4). See §7.4 for why
  staging must never lock a clone.
- `--yes`: skip the rollout confirmation prompt. Should also be satisfiable
  via a non-interactive-mode environment variable, for scripting.
- `--wait`: after starting a replacement, poll until it reaches a terminal
  status instead of returning immediately after the trigger.
- An output-format flag (text/JSON or similar) for both `list` and the
  replacement-outcome renderer.

## 5. Data flow summary

```text
dr workload env set <wid> A=1 B=2
        │
        ▼
GET workload {wid}  ──► artifactId
        │
        ▼
GET artifact {artifactId}  ──► status (draft|locked), current environmentVars
        │
        ├─ draft ──────────────────────────────┐
        │                                       ▼
        │                            PATCH artifact {artifactId}
        │                            (whole spec, mutated env vars)
        │                            targetArtifactId = {artifactId}
        │                            needsLock = false
        │
        └─ locked ─────────────────────────────┐
                                                 ▼
                                      POST artifact {artifactId}/clone/
                                                 │
                                                 ▼
                                      PATCH clone (whole spec, mutated env vars)
                                      targetArtifactId = {cloneId}
                                      needsLock = true   (clone stays UNLOCKED here)
        │
        ▼
--stage? ── yes ──► print targetArtifactId, stop. (no lock, no confirm, no rollout)
        │
        no
        ▼
confirm rollout (unless --yes)
        │
        ▼
needsLock? ── yes ──► PATCH targetArtifactId {"status":"locked"}
        │
        ▼
GET workload {wid}/replacement/  ──► already active? ──► error, stop
        │
        no
        ▼
POST workload {wid}/replacement/ {artifactId: targetArtifactId, strategy: rolling}
        │
        ▼
--wait? ── yes ──► poll GET replacement until terminal, render outcome
        │
        no ──► print "started", exit
```

## 6. Core algorithm

### 6.1 Resolve workload → artifact

`GET` the workload for its bound `artifactId`, then `GET` that artifact. Two
calls, always, for every subcommand.

### 6.2 Primary container selection

Given `spec.containerGroups`, select:

1. The first container across all groups with `"primary": true`, if any.
2. Otherwise, `containerGroups[0].containers[0]`.

Apply this **identically** for reads (`list`) and writes (`set`/`import`/
`delete`) so they always agree on which container they're talking about.

### 6.3 Mutate: draft vs. locked

Given the artifact fetched in §6.1:

- **If `status == "draft"`:** re-fetch the artifact as a raw/untyped document
  (so unrelated fields round-trip untouched — see §3.3's whole-array-replace
  warning), locate the primary container, replace its `environmentVars` with
  the upserted/removed result, `PATCH` the whole `spec` back to the *same*
  artifact id. `targetArtifactId = artifactId`, `needsLock = false`.

- **If `status == "locked"`:** `POST .../clone/` with a name derived from the
  original (e.g. `<original-name>-env-<timestamp>`, just needs to be
  unique-ish and traceable back to its purpose). Apply the same mutation to
  the **clone's** environmentVars, `PATCH` the clone (still a draft at this
  point). `targetArtifactId = cloneId`, `needsLock = true`. **Do not lock the
  clone yet** — see §7.4.

Both branches return `(targetArtifactId, needsLock)` to the caller. Neither
branch decides whether/when a rollout happens — that's entirely the caller's
concern (§6.4), letting `--stage` short-circuit before any of it.

### 6.4 Rollout decision

Given `(targetArtifactId, needsLock)` from §6.3:

- **`--stage`:** report `targetArtifactId` and stop. Nothing further happens.
- **Otherwise:**
  1. Confirm with the user (unless `--yes`) that this triggers a rolling
     redeploy — phrase it as exactly that, not as a generic "apply changes?"
     prompt, since it's a different risk class than editing a draft.
  2. If `needsLock`, lock `targetArtifactId` now (§3.5, §7.4).
  3. Guard: `GET` the workload's replacement; if one is already active,
     error out naming it (don't silently queue a second swap — §3.6, §7.3).
  4. `POST` the replacement.
  5. If waiting, poll until terminal (§3.6's status vocabulary) and render
     the outcome; if failed, say so and note the workload reverted.
     Otherwise, print the "started" acknowledgement and exit.

## 7. Corner cases — required behavior

Each of these was found by testing against a live instance, not by
inspection. A reimplementation that skips any of them will reproduce a real
bug.

### 7.1 Concurrent edits to the same workload can silently clobber each other

**What happens:** two `env set` calls against the *same draft artifact*,
racing — each does GET → mutate its own copy → PATCH the whole spec. There is
no version check on this endpoint (§3.3), so whichever PATCH lands last wins
outright. Verified directly: two concurrent calls setting different env vars
(`A=1` and `B=2`) on the same draft artifact resulted in only one surviving;
**both processes reported success** with no indication anything was lost.

**Required behavior:** there is no complete fix available without server-side
support that doesn't exist (no ETag/If-Match, no version field on drafts).
Document this prominently in user-facing help text: avoid running `set`/
`delete` against the same workload from two sessions at once. (A narrower
mitigation — re-checking the artifact's `updatedAt` immediately before the
final write and aborting if it moved since the initial read — shrinks the
window but does not close it; treat it as optional polish, not a fix.)

### 7.2 Batching must happen within one invocation, not across invocations

**Why:** for a *locked* artifact, every edit clones a fresh draft (§6.3).
Because the workload's bound `artifactId` doesn't change until a replacement
actually completes, a second `set`/`delete` call made shortly after a first
one (before rollout) will resolve **from the same original locked artifact**
again — not from the first call's clone. Two separate invocations produce two
independent, non-cumulative clones; the second does not build on the first.

**Required behavior:** `set` and `delete` must accept **multiple** name/value
arguments in a single invocation, applied together against one clone before
one rollout. Document plainly that batching across separate command
invocations is not supported — don't let a user discover this by losing an
edit.

**Also required — the `delete`-specific half of this:** checking "would this
removal actually change anything?" **before** calling into the mutate path
(§6.3), not after. A naive implementation that always clones-then-checks
will, for a locked artifact, create a throwaway (if harmless — still a
draft, still deletable) clone on every attempt to delete a name that was
never set, e.g. a typo. Pre-check membership against the artifact's current
`environmentVars` and fail immediately if none of the requested names are
present, before any clone or patch happens.

### 7.3 Guard against an in-flight replacement — twice, at different points, for different reasons

**Why an early check (before any mutation):** without it, a `set`/`delete`
call against a *locked* artifact whose workload already has a replacement in
flight will still fully clone and patch a new artifact (§6.3) before
discovering — only at the rollout step — that it can't be deployed right
now anyway. That's wasted work and a wasted (if harmless) clone.

**Why a late check is still required in addition (immediately before the
actual `POST .../replacement/`):** state can change in the time it takes to
do the clone/patch/lock/confirm dance. The early check is a fail-fast
optimization; the late check is the actual correctness guard against
double-posting a non-idempotent replacement (§3.6).

**The `--stage` exception:** staging never calls the replacement endpoint at
all, so it's safe to prepare an edit while a different replacement is
settling — the early check should be skipped entirely when staging, not just
downgraded to a warning.

### 7.4 Never lock a clone until the moment of actual rollout

**Why:** locking is one-way and a locked artifact can never be deleted
(§3.5). If `--stage` (or any other "prepare but don't deploy" path) locked
the clone immediately after cloning+patching, then **every** staged edit
against a workload currently running a locked artifact leaves behind a
permanent, undeletable artifact — forever, even if the user never actually
deploys it. Repeated use of a stage-first workflow would accumulate garbage
with no possible cleanup.

**Required behavior:** the clone-and-mutate step (§6.3) must always leave the
result as an unlocked draft, regardless of the source artifact's status.
Locking happens exactly once, only in the non-staged rollout path (§6.4),
immediately before the replacement is posted — at which point the
status-match rule (§3.6) requires it anyway.

### 7.5 Name the artifact an error left behind

**Why:** several failure points happen *after* a side-effecting write has
already succeeded:

- The clone succeeded, but the subsequent patch on it failed. The clone
  still exists (harmless, still a deletable draft) — but if the error
  doesn't say so, the caller has no way to find it short of listing every
  artifact.
- The edit and (if needed) the lock succeeded, but starting the replacement
  failed (e.g. a 400 status-mismatch, a 403 limit). The prepared artifact
  exists and is ready — but if the error doesn't say so, the caller doesn't
  know they can just retry the rollout instead of redoing the whole edit.

**Required behavior:** any error surfaced after a write has already
committed must name the resulting artifact id and (where relevant) what
produced it, so recovery is "retry against artifact X" instead of "start
over and hope."

### 7.6 The platform's replacement status vocabulary is broader — and includes a second failure status — beyond what's commonly documented

**What happens:** reference documentation for this API names only
`completed` and `failed` as terminal statuses. Live testing surfaced a third
terminal status, `errored`, on a candidate that never became healthy (a
readiness-probe failure) — not mentioned in some reference materials at all.

**Why it matters, concretely:** if a poller only recognizes `completed`/
`failed` as terminal and treats a 404 *after having seen a non-terminal
status* as "the platform settled the replacement and garbage-collected the
record → success" (a real, intentional, documented behavior of this API —
see next paragraph), then an `errored` replacement that later gets cleared
via 404 will be silently reported as a **success**, when it was in fact a
failure. This was reproduced directly: an `errored` status followed shortly
by a 404 produced a false "success" until the poller was corrected to treat
`errored` as terminal-failure too.

**Required behavior:**

- Treat both `failed` and `errored` (and any other status the platform might
  introduce that clearly indicates a terminal negative outcome) as
  terminal-failure, not just `failed`.
- `GET .../replacement/` returning 404 is normal in two different contexts
  that must be told apart: (a) on the very first check, meaning nothing was
  ever active — this is an error/logic bug in the caller (it should have
  just started one); (b) after having previously observed a non-nil,
  non-terminal replacement — meaning the platform settled it and cleared the
  record, which should be treated as the terminal state *last observed*
  (success only if that last-observed status was actually success).
- Default to treating an *unrecognized* status as non-terminal (keep
  polling) rather than guessing — matches the platform's own pattern of
  introducing statuses ahead of documentation.

### 7.7 The platform does not validate env var names or credential references at write time

**What happens, verified directly:** `PATCH`ing an artifact's spec with an
env var name containing a space, a leading digit, or with a `drCredentialId`
that does not exist (a syntactically-valid but nonexistent id) both succeed
with `200` and are silently stored. Nothing rejects them until much later —
either a replacement failure, or a container that fails to start with no
obvious link back to the command that introduced the bad value.

**Required behavior — validate what can be validated, client-side, before
writing anything:**

- **Env var name format:** reject a name that isn't a legal container env
  var name before it's ever sent. Use the same rule the underlying container
  runtime actually enforces (Kubernetes' env var name rule: starts with a
  letter, underscore, or dot, followed by letters/digits/underscore/dot/dash
  — i.e. `^[-._a-zA-Z][-._a-zA-Z0-9]*$`). Note that dashes and a leading dot
  or underscore are all *legal* by this rule, even though unusual for an env
  var — don't over-tighten past what the runtime actually requires.
- **Credential existence:** before accepting a credential-backed reference,
  fetch the credential by id (§3.7) and fail immediately if it 404s.
  Deduplicate repeated ids across multiple vars in one call so each distinct
  credential is only checked once.
- **Explicitly do not** attempt to validate that a credential's `key` is one
  of its valid field names against a hardcoded type→key table (§3.2). That
  table can drift from the platform's real schema; a false rejection on a
  key that's actually fine is worse than deferring to the platform's own
  (currently nonexistent, but potentially future) validation.
- This validation must run **before** resolving the workload/artifact at
  all, so a doomed request fails without any wasted network calls or side
  effects (consistent with §7.2/§7.3's "check before you touch anything"
  theme running through this whole feature).

### 7.8 Multiline values must survive intact

**What happens, verified directly:** a value containing embedded newlines
(e.g. a multi-line certificate) round-trips correctly through parsing, the
JSON write, and the JSON read, with no special handling required — JSON
naturally escapes embedded newlines, and shells pass multi-line quoted
arguments through `argv` untouched. If a value is split into `NAME` and
`VALUE` by the *first* `=` only, embedded `=` characters later in the value
(e.g. base64 padding, query strings) are also preserved correctly.

**Required behavior:** don't `TrimSpace` or otherwise mangle the value
half of a `NAME=VALUE` argument, and don't split on `=` naively in a way
that breaks on a value containing more `=` characters. A human-readable
table/list display should be prepared to show a value spanning many lines
without corrupting its own layout (a library that auto-expands row height
for multi-line cell content handles this gracefully; one that doesn't will
need explicit truncation or a "show full value" escape hatch for very long
multi-line values).

## 8. Explicit non-goals (v1 scope)

- **Multi-container (sidecar) support.** Only the primary container's env
  vars are read or written. A forward-compatible path: add an optional
  "target this specific container by name" parameter to all three
  subcommands, defaulting to the primary-container rule (§6.2) when
  unspecified, so existing callers see no behavior change. The write
  mechanics (§3.3's whole-array-replace) don't change — only which
  container gets mutated.
- **Cross-invocation staging/batching state.** No local or remote state
  tracks "there's a pending staged clone for this workload" across separate
  command invocations (§7.2). A user who wants to batch edits across
  multiple commands before one rollout is expected to include every edit in
  a single invocation instead.
- **A true fix for the concurrent-edit race (§7.1).** Only documentation and
  (optionally) a narrowed-but-not-closed window via an `updatedAt` check.

## 9. Suggested acceptance test matrix

At minimum, a reimplementation should have automated coverage for:

- Primary container selection: flagged primary found across multiple
  groups; no container flagged primary (falls back to `[0][0]`); read and
  write selection logic agree.
- Draft artifact: `set`/`delete` patch in place; the *entire* container
  array round-trips even when only one container's env vars changed.
- Locked artifact: `set`/`delete` clone, patch the clone (not the
  original — original is only ever read), leave the clone **unlocked**
  after §6.3 (only locked at rollout time, §6.4/§7.4).
- `--stage`: no confirmation prompt, no lock, no replacement call, target
  artifact reported to the user; skips the in-flight-replacement guard
  entirely (§7.3).
- Delete of a name that was never set: errors without touching any artifact
  when the artifact is locked (§7.2); succeeds (removes the rest) when some
  but not all given names are present.
- In-flight-replacement guard: fires before any artifact mutation when not
  staging; fires again immediately before the `POST` even when the earlier
  check passed; does not fire at all when staging.
- Error paths name the artifact left behind: clone-then-patch-fails names
  the clone; start-replacement-fails names the prepared/locked artifact.
- Replacement polling: recognizes `completed` as success, both `failed` and
  `errored` as failure; treats an unrecognized status as non-terminal
  (keeps polling); a 404 on the very first poll is an error, a 404 after a
  previously-observed non-terminal status is treated as the last-observed
  outcome (success only if that was actually a non-failure status).
- Env var name validation: accepts letters/digits/`_`/`-`/`.`, rejects a
  leading digit and embedded whitespace, all client-side before any network
  call.
- Credential reference validation: existing id passes; nonexistent id fails
  with the id named in the error; the same id referenced by two vars in one
  call is only checked once; a non-404 error from the check (e.g. a 5xx)
  is surfaced as a check failure, not silently treated as "confirmed
  missing."
- A multiline value round-trips unchanged through write and read.
- `import`: defaults to `.env` in the current directory; `--file` overrides
  it; a missing file, and an empty (or comments-and-blanks-only) file, both
  error rather than silently no-op-ing; a name present both in the imported
  file and already on the workload ends up with the file's value (ordinary
  upsert-by-name, same as `set`); the same name/credential validation as
  `set` applies to every variable found in the file.
