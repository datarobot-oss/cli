# `.datarobot.yaml` - the workload manifest

The file `dr workload config` writes and `dr workload up` deploys from. This page is the reference for the file itself and for what `up` does with it. The commands and their flags are documented in [`dr workload`](workload.md).

> [!NOTE]
> `config` and `up` are behind a feature gate. Set `DATAROBOT_CLI_FEATURE_WORKLOAD=true` to use them.

## What the file is

`.datarobot.yaml` sits at the root of your project and is the **workload-create spec, verbatim**: the same document `dr workload create --spec-file` accepts, with every field documented in the [spec reference](workload-spec.md#workload-spec). Keys the CLI does not know are passed to the platform untouched, so a platform addition needs no CLI release.

Two conveniences are layered on top, and both are the CLI's to manage:

- **`workloadId`** binds the file to a workload. `up` writes it after the first deploy, and every later deploy reads it to find the workload. It is stripped before anything is sent to the API. Commit it: a checkout without it creates a new workload on the next `up`.
- **`dr-credential:<credential-id>/<key>`** as an environment variable value is shorthand for the credential-backed object form (`source: dr-credential`, `drCredentialId`, `key`). The object form is accepted too.

This is what `config` writes for a project with a Dockerfile:

```yaml
workloadId: 68b0c1d2e3f4a5b6c7d8e9f0 # managed by the CLI
name: my-app
importance: low # low | moderate | high | critical

artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080 # must be 1024 or above
            imageBuildConfig: # built from this repository's Dockerfile
              dockerfile:
                source: provided

runtime:
  containerGroups:
    - name: default # matches the artifact group
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
```

No readiness probe is written unless you give a health path (`--health /ready`): a guessed path kills a healthy deploy whose framework answers 404 there.

After the first write the file is yours. `config` leaves an existing manifest alone, and `up` edits exactly one key, `workloadId`. The exception on both is `--sync-env`, which edits only the environment variables after showing what it would change. Both keep your comments, your key order and any keys they do not know.

## Where the image comes from

Each container builds its image one of three ways. `config` asks, preselecting the first when `./Dockerfile` exists; without a terminal the Dockerfile is chosen for you, and with no Dockerfile the flags have to say which.

| Build mode | In the file | `config` flags |
| --- | --- | --- |
| Your own Dockerfile | `imageBuildConfig.dockerfile.source: provided` | none needed when `./Dockerfile` exists |
| A DataRobot base image plus your code | `imageBuildConfig.dockerfile.source: generated`, with `executionEnvironmentId`, `executionEnvironmentVersionId` and an `entrypoint` | `--build-mode generated --execution-environment <name> --entrypoint "..."` |
| An image you already published | `imageUri` | `--build-mode image --image <uri>` |

The first two make `up` push the working tree and wait for a platform build on the first deploy, and again whenever the code changes. The third deploys in one call and never syncs code. A container carries `imageUri` or `imageBuildConfig`, never both.

The rest of the vocabulary is the spec's: GPU resource bundles, autoscaling, secrets, extra containers, startup, readiness and liveness probes, `agent` and `nim` types. All of it is written and deployed as it stands, whether or not the wizard has a question for it.

## What the file manages

`up` compares the file against the running workload and applies only the difference. The rule is one-directional:

- **Whatever the file says, `up` makes true.** Change a value in the UI that the file names, and the next `up` reports it as drift and puts the file's value back.
- **Whatever the file leaves out, `up` leaves alone.** A workload can carry a GPU bundle, a sidecar or an autoscaling policy the file never mentions, and none of it is drift.
- **Deleting a line stops managing that field.** It does not revert it.

Drift is measured against the running object, not against a record of the last deploy, so a fresh CI clone deploys correctly with nothing but the committed file.

## What `up` does

Every run prints a plan, then carries it out. `--dry-run` prints the plan and stops. Nothing is confirmed: the plan is the review.

A first deploy is one line:

```
  + workload   my-app will be created, with its first artifact
```

A later one lists what differs, one line per kind of change:

```
my-app (68b0c1d2), running
  ~ code       3 files changed since the last deploy
  + artifact   new version, 1 spec change
      containerGroups[default].containers[primary].environmentVars[GREETING].value: changed
  ~ runtime    containerGroups[default].replicaCount: 1 -> 3
```

When nothing differs the plan is `✓ Already up to date`.

Three kinds of change, each applied one way:

| Change | How it is detected | What `up` does |
| --- | --- | --- |
| **Code** (only when the file builds the image) | the working tree is compared with what was last pushed to the artifact; code pushed by `dr artifact code sync` but never built counts too | pushes the changed files with the sync engine, builds a new image, mints a new artifact version and rolls the workload onto it |
| **Artifact spec** (environment variables, port, probes, image) | the file's `artifact` block is compared with the artifact the workload runs | mints a new artifact version and rolls the workload onto it; a change that does not affect the image keeps the running image, so no rebuild. When the artifact is a draft and the image is kept, the change is written to that artifact in place and the workload rolled onto it again, so no new version |
| **Runtime** (replicas, CPU, memory) | the file's `runtime` block is compared with the live runtime | a settings update in place, with no new version; when a roll is happening anyway, the sizing rides along on it |

A workload that does not exist yet is created from the file in one call. A stopped one is started. An errored one is rolled when the file has something new to roll onto it; when nothing differs the run refuses and says why, since deploying the same thing again would only fail again (`--force-build` rebuilds a platform-built image). A file whose live state matches it prints `Already up to date` and exits 0 without touching anything.

A roll swaps the workload onto a version, new or the same one rewritten; the endpoint never changes, and the generation already serving keeps serving until the new one is ready. Locking is one-way: a locked artifact is never written to, so the next version of one is a new artifact in the same lineage, locked to match, and a locked workload keeps deploying. A rollout of a rewritten draft that does not land leaves the artifact ahead of what is running, and the next `up` notices, because the artifact changed after the serving generation started, and rolls it again.

### Local state

`up` keeps machine-managed state beside the code, never in the manifest, under `.datarobot/workload/`:

| File | What it holds |
| --- | --- |
| `config.json` | which artifact and code catalog this checkout pushes to, and the last synced version |
| `manifest.json` | the sync engine's index of what was last pushed, used to diff the next sync |
| `history.log` | one line per sync |
| `.checkouts/` | versions downloaded with `dr artifact code checkout` |
| `.rollback/` | copies of files a sync is about to overwrite, removed when the sync succeeds |

### Rollback

The code sync is the one step that writes to your working tree, and it does so safely. Before it replaces or deletes a local file it keeps a copy as `<path>.LOCAL.<timestamp>`, and before any write it copies the originals into `.rollback/`. A sync that fails partway restores them; a sync that was interrupted is restored by the next one before it starts. The `.LOCAL` copies are left for you to inspect and are never uploaded.

A deploy that fails after the workload exists still reports the workload, so the next run continues rather than creating a second one. A run that dies between minting a version and promoting it leaves a draft the next run picks up.

## What the CLI checks before sending

`config` validates the file before it writes it, and `up` validates it before every deploy. Every finding names the key and the line. The checks:

- `name` is present and not empty.
- exactly one of `artifact` (inline definition) or `artifactId` (an existing artifact).
- an inline artifact has `spec.containerGroups` with at least one group, and each group has at least one named container.
- each container sets either `imageUri` or `imageBuildConfig`, and `imageBuildConfig.dockerfile.source` is `provided` or `generated`.
- `generated` carries `executionEnvironmentId`, `executionEnvironmentVersionId` and a non-empty `entrypoint`; `provided` needs `./Dockerfile` beside the manifest.
- the primary container has a numeric `port` of 1024 or above, since containers run unprivileged; no other container sets one.
- at most one container per group is marked `primary`.
- every environment variable has a `value` or a credential source, and a `dr-credential:` reference is well formed.
- `runtime` groups and containers match the artifact's by name.
- a group sets `replicaCount` or `autoscaling`, not both.
- `resourceAllocation.memory` is a byte count or a 1000-based unit (`512MB`, `2GB`).

Everything else is the server's to check; a rejected deploy comes back as a `422` naming the field.

## When a deploy fails

The failures people hit most. `up` prints the platform's reason beside the errored state; re-running `up` after the fact shows it again in the plan.

| Failure | What it means | What to do |
| --- | --- | --- |
| `getpwuid(): uid not found` | containers run as a non-root user with no password-database entry, so anything calling `getpwuid` dies | set `USER` and `HOME` in the Dockerfile, or avoid the lookup |
| `exec format error` | the image has no `linux/amd64` manifest, the standard Apple-silicon mistake | build with `--platform linux/amd64` |
| endless restart during startup | liveness with no startup probe, or a startup budget shorter than the model load | add a `startupProbe`, or lengthen it |
| readiness never passes | port declared but not listened on, or the wrong path | check the port the app listens on and the probe path |
| `ImagePullBackOff` | the registry is not reachable from the platform, or a bad tag | check the registry and the tag |
| pod never schedules | bundle shape unavailable in the cluster | pick a smaller resource bundle |

## See also

- [`dr workload`](workload.md): the commands, including `config` and `up` and how to run them unattended.
- [Spec reference](workload-spec.md): every field the artifact and workload specs accept.
- [`dr artifact`](artifact.md): the artifacts, builds and code sync `up` drives for you.
