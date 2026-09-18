# Artifact and workload spec reference

Field-by-field reference for the spec files `dr artifact create --spec-file` and `dr workload create --spec-file` read, plus one end-to-end walkthrough that uses both.

## How to read this document

- **Two specs, two commands.** The **artifact spec** says *what* runs: images, ports, environment variables. The **workload spec** says *how much of it* runs and *where*: replicas, CPU, memory, placement. See [`dr artifact`](artifact.md) and [`dr workload`](workload.md) for the commands themselves.
- **JSON is sent byte-for-byte; YAML is converted to JSON first.** Quote any YAML value that must stay a string (for example `"0644"`), or it arrives as a number.
- **Field-level validation is the server's.** A rejected spec comes back as a `422` naming the offending JSON path. The handful of checks the CLI makes before the request are listed under [What the CLI checks before sending](#what-the-cli-checks-before-sending).

---

## Artifact spec

### Top-level fields

| Field | Required | Notes |
| --- | --- | --- |
| `name` | yes | The artifact's display name. Names are not unique; what groups successive versions of one thing is the artifact repository, which the platform assigns. |
| `type` | no | `service` (the default) or `agent`. |
| `spec.containerGroups[]` | yes | At least one group, each with at least one container. |

`type` sits **beside** the spec, not inside it. The platform pops any `type` it finds under `spec` and re-derives the kind from the artifact's own `type`, so a type written one level down is discarded without a word: an agent silently becomes a service, and agent-only fields such as `a2aEnabled` are then rejected as unknown fields on the service variant. An artifact repository takes its kind from whichever artifact opened it, so a later version cannot change it.

```yaml
name: my-agent
type: agent            # beside spec, never under it
spec:
  containerGroups:
    - name: default
      containers: [...]
```

### Container groups and containers

```yaml
spec:
  containerGroups:
    - name: default
      containers:
        - name: primary
          primary: true
          port: 8080
          imageUri: nginx:latest
```

| Field | Required | Notes |
| --- | --- | --- |
| `containerGroups[].name` | no, but name it | The workload spec addresses a group by this name. An unnamed group cannot be given a replica count or a resource allocation later. |
| `containers[].name` | no, but name it | Same reason: runtime overrides reference a container by name. |
| `containers[].primary` | one per spec | Marks the container that serves traffic. Its port is what the endpoint URL reaches. |
| `containers[].port` | for the primary | The port the container listens on. Must be **1024 or above**. |
| `containers[].imageUri` | one of the two | An existing image. Nothing is built. |
| `containers[].imageBuildConfig` | one of the two | Build the image from your code. See below. |
| `containers[].environmentVars` | no | See [Environment variables and secrets](#environment-variables-and-secrets). |
| `containers[].readinessProbe` | no | `{path, port}`: when the platform should treat the container as ready to serve. |

The specs the CLI generates itself use `default` for the group and `primary` for the container. Following that convention keeps the workload half short.

### Where the image comes from

Each container carries **either** `imageUri` **or** `imageBuildConfig`, never both.

**Prebuilt image.** Nothing to build, nothing to sync:

```yaml
containers:
  - name: primary
    primary: true
    port: 8080
    imageUri: nginx:latest
```

**Built from your own Dockerfile.** The build reads `./Dockerfile` out of the code you pushed with `dr artifact code sync`:

```yaml
containers:
  - name: primary
    primary: true
    port: 8080
    imageBuildConfig:
      dockerfile:
        source: provided
```

**Generated from an execution environment.** The platform writes the Dockerfile for you from a base environment, and you supply the command that starts your app:

```yaml
containers:
  - name: primary
    primary: true
    port: 8080
    imageBuildConfig:
      dockerfile:
        source: generated
        executionEnvironmentId: 68b0c1d2e3f4a5b6c7d8e9f0
        executionEnvironmentVersionId: 68b0c1d2e3f4a5b6c7d8e9f1
        entrypoint: ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080"]
```

`executionEnvironmentId` and `executionEnvironmentVersionId` are only valid when `source` is `generated`. No CLI command lists execution environments yet; take the ids from the DataRobot UI or from `GET /api/v2/executionEnvironments/`.

`imageBuildConfig.codeRef` names the catalog version the build compiles, and `dr artifact code sync` writes it for you:

```yaml
imageBuildConfig:
  codeRef:
    datarobot:
      catalogId: 68b0c1d2e3f4a5b6c7d8e9f0
      catalogVersionId: 68b0c1d2e3f4a5b6c7d8e9f1
  dockerfile:
    source: provided
```

Write it by hand only to point a new artifact at a catalog version that already exists.

### Environment variables and secrets

`environmentVars` is a list on the container. A plain setting carries its value directly:

```yaml
environmentVars:
  - name: LOG_LEVEL
    value: info
  - name: PORT
    value: "8080"        # quoted: YAML would otherwise send a number
```

A secret is stored as a credential and referenced by id, so the value never appears in the spec, in your repository, or in `dr artifact get`:

```yaml
environmentVars:
  - name: OPENAI_API_KEY
    source: dr-credential
    drCredentialId: 68b0c1d2e3f4a5b6c7d8e9f0
    key: apiToken
```

`key` selects which **field of the credential** to use, not the variable's own name: one stored credential can bundle several fields, as an S3 credential bundles `awsAccessKeyId` and `awsSecretAccessKey`. A credential holding a single secret is stored as an API token, whose field is `apiToken`.

There is no `dr credential` command yet. Create one in the DataRobot UI, or with the API, using the endpoint and token `dr auth export` already puts in your shell:

```bash
curl -X POST "$DATAROBOT_ENDPOINT/credentials/" \
  -H "Authorization: Bearer $DATAROBOT_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-app-openai", "credentialType": "api_token", "apiToken": "<the secret>"}'
```

The `id` in the response is what goes in `drCredentialId`. A name already in use comes back as a `409`, and reusing that credential would deploy whatever value it already holds, so pick a fresh name rather than assuming it is yours.

A container reads its environment once, at startup. Rotating a secret (`PATCH /api/v2/credentials/{id}/` with a new `apiToken`) therefore changes nothing for a workload that is already running until its containers restart: `dr workload stop <id>` then `dr workload start <id>`.

---

## Workload spec

### Top-level fields

| Field | Required | Notes |
| --- | --- | --- |
| `name` | yes | The workload's display name. |
| `artifactId` | one of the two | An existing artifact, normally a locked one. |
| `artifact` | one of the two | An inline artifact definition, created and deployed in the same call. Takes the same fields as the artifact spec above. |
| `importance` | no | `low` (the default), `moderate`, `high` or `critical`. |
| `runtime` | no | Replicas, resources and placement. Without it the platform applies its own defaults. |

### Runtime

```yaml
name: my-app
artifactId: 68b0c1d2e3f4a5b6c7d8e9f0
runtime:
  containerGroups:
    - name: default            # must match a group in the artifact
      replicaCount: 1
      containers:
        - name: primary        # must match a container in that group
          resourceAllocation:
            cpu: 1
            memory: 512MB
```

`runtime` addresses the artifact's containers by name, so `containerGroups[].name` must name a group in the artifact and `containers[].name` a container inside it. A name that matches nothing has nothing to size, which is why the artifact half is worth naming.

| Field | Notes |
| --- | --- |
| `replicaCount` | Fixed scale. Mutually exclusive with `autoscaling.enabled: true` on the same group. |
| `autoscaling` | Dynamic scale. See below. |
| `resourceAllocation.cpu` | CPU cores. Fractional values are allowed (`0.5`). |
| `resourceAllocation.memory` | A byte count or a **1000-based** unit: `B`, `KB`, `MB`, `GB`, `TB`. Binary units such as `Gi` are read as their decimal namesakes rather than converted, so write the decimal unit you actually mean. |

### Fixed replicas or autoscaling, not both

Replica bounds live on `autoscaling` (`minReplicaCount` / `maxReplicaCount`); each policy needs only `scalingMetric` and `target`. Omit `replicaCount` when autoscaling is enabled. A full copy-paste spec is in [workload-autoscaling.yaml](../examples/workload-autoscaling.yaml).

```yaml
runtime:
  containerGroups:
    - name: default
      autoscaling:
        enabled: true
        minReplicaCount: 1
        maxReplicaCount: 10
        policies:
          - scalingMetric: cpuAverageUtilization
            target: 80
      containers:
        - name: primary
          resourceAllocation:
            cpu: 1
            memory: 512MB
```

> [!NOTE]
> The Workload API still accepts the legacy per-policy `minCount` / `maxCount` fields on input and hoists them automatically, but responses always use `minReplicaCount` / `maxReplicaCount` on `autoscaling`. Prefer the new shape in new specs.

### Placement

`runtime.enclaveSelectionPolicy` and `runtime.enclaves` pin a workload to a named Enclave. Rather than writing them by hand, pass [`dr workload create --enclave <name>`](workload.md#create), which sets both and refuses to override a spec that already sets either. Without them, DataRobot picks the placement.

---

## Deploy a service from source, end to end

This is the whole path for a project whose image DataRobot builds, from an empty artifact to a URL that answers. Every step is a command you can paste.

```bash
# 1. Register the draft artifact. spec.yaml is the artifact spec above,
#    with imageBuildConfig.dockerfile.source: provided.
dr artifact create --spec-file spec.yaml      # prints the new artifact id

# 2. Link this directory to it. Writes .datarobot/workload/ and a starter
#    .drignore; commit the .drignore.
dr artifact code init <artifact-id>

# 3. Push the code the image is built from.
dr artifact code sync

# 4. Build the image and wait for it. On success the summary line ends with
#    the image it produced; on failure it carries the tail of the build log.
dr artifact build create --wait               # Build <id>: COMPLETED in 94s (image: …)

# 5. Confirm the build landed before locking: locking does not reliably
#    check this for you.
dr artifact build list <artifact-id>          # the newest build should be COMPLETED

# 6. Freeze the artifact. One-way: a locked artifact cannot be edited,
#    unlocked or deleted.
dr artifact lock <artifact-id>

# 7. Deploy it. workload.yaml is the workload spec above, with
#    artifactId set to the artifact you just locked.
dr workload create --spec-file workload.yaml  # prints the new workload id

# 8. Startup is asynchronous. Poll until it says running.
dr workload status <workload-id>

# 9. Call it. The endpoint URL ends in a slash, so append without one.
curl "$(dr workload endpoint <workload-id>)health"
```

To change the code afterwards, the artifact you locked stays as it is: create a new artifact, sync, build and lock it, then deploy it with a new `dr workload create`. Locked artifacts are kept rather than deleted, so each version goes on naming exactly what it ran. These commands have no in-place replacement, so the new deploy is a new workload with a new id and a new endpoint URL, and anything calling the old URL has to be repointed at it.

---

## What the CLI checks before sending

Everything else is the server's, and comes back as a `422` naming the JSON path.

**`dr artifact create --spec-file`:**

- `name` is present and not empty.
- `spec.containerGroups` has at least one entry.
- Every container group has at least one container.

**`dr workload create --spec-file`:**

- `name` is present and not empty.
- Exactly one of `artifactId` or `artifact` is set.
- Per container group, `autoscaling` is an object rather than a boolean, and `replicaCount` does not sit beside `autoscaling.enabled: true`.

Unknown fields are deliberately **not** rejected locally: the server's `422` names the offending path, which is more useful than a local complaint about a key the platform may have added since this CLI was built.

---

## See also

- [`dr artifact`](artifact.md): register, sync, build and lock the artifact.
- [`dr workload`](workload.md): deploy and operate the workload.
- [workload-autoscaling.yaml](../examples/workload-autoscaling.yaml): a complete autoscaling spec.
- [Authentication](auth.md): where `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` come from.
