# `dr workload compose` — Design Spec

Status: **design finalized, not yet implemented.** No `cmd/workload/compose`, `internal/workload/compose`, or Taskfile/go.mod changes exist yet. This document is the durable record of the design and the live findings it rests on.

## Motivation

Users with a `docker-compose.yaml` describing a small multi-container app (e.g. a web service plus a cache) want to deploy it to DataRobot's Workload API without hand-writing a `.datarobot.yaml` and without learning the platform's container/networking model from scratch.

## Command

```
dr workload compose -f compose.yaml [--project-name NAME] [--dry-run] [--yes] [--output-format json]
```

- Feature-gated under `workload`, same as every other subcommand.
- No `--detach`: every container in the group starts together as part of one deploy, so there is no cross-workload wait to skip.
- `--dry-run` shows the generated manifest and rejected/unsupported fields without deploying.

## Core translation model

1. **One Compose file → one workload, one container group, one container per service.** Every service in the `services:` map becomes a container in a single `.datarobot.yaml` container group named `default`. This supersedes an earlier per-service-workload design: sibling containers in one group share a pod network namespace, and there is no cross-workload private networking to fall back on regardless.

2. **Exactly one primary.** Chosen via explicit `x-datarobot.primary: true`, else the only service declaring a Compose `ports:`/`expose:` entry, else the import fails asking for disambiguation. Only the primary gets a `port:` in the generated spec, a readiness probe, and the public HTTPS/WebSocket gateway endpoint. This mirrors the platform's own rule — the manifest validator already rejects `port` on a non-primary container — and the observed fact that the platform creates exactly one Kubernetes Service (and its DNS registration) per container group, scoped to the primary only.

3. **Static address rewrite at import time, never a runtime lookup.** Any reference to another Compose service — a bare service name used as a hostname (`redis`), `service:port`, or an explicit `{{ services.<name>.endpoint }}` template in an environment value — is rewritten once, while generating the manifest, to `127.0.0.1:<that service's container port>` (or a full `redis://127.0.0.1:6379`-style URL when a scheme is present in the template or original value). There is no real DNS, no `/etc/hosts` injection, and no dependency-ordered deploy sequencing to make this work — every container in the group starts at once. This is the single mechanism the whole feature depends on, and it has been verified end-to-end (see below).

4. **Port collisions are a hard import-time error.** Two services cannot both claim the same port in one shared network namespace.

5. **`depends_on` only validates references, never orders deployment.** It confirms the referenced service exists in the file. It never guarantees container start order — the platform does not promise one within a group. Any app that depends on another service in the group must retry/back off on connect, the same as ordinary Kubernetes pod behavior. This is a explicit, documented limitation, not an oversight.

6. **V1 build support mirrors what's proven today.** Only the primary container may build from a local Dockerfile (`source: provided`) or reference a published image; every non-primary/sidecar service must be a published `image:` — no per-sidecar build contexts in V1. This matches the existing single-artifact, primary-only build/code-ref tracking in `internal/workload/up`; extending that is out of scope until proven necessary.

7. **Explicit rejection, never silent drop.** The importer rejects, naming the exact service and key, rather than ignoring: custom `networks:`, `volumes:`, host port publishing beyond the primary's own container port, `links`, `healthcheck` (the platform's HTTP readiness probes are not shell-command checks), `command`/`entrypoint` overrides (until a verified mapping exists), multiple primaries, and any service with neither an image nor a build.

8. **Credentials stay explicit.** Compose `environment:`/`env_file` literal values pass through as manifest `environmentVars` literals. Anything that should be backed by a DataRobot-managed credential must use the existing `dr-credential` reference shorthand explicitly — there is no automatic secret detection or minting. A backing store's own authentication (e.g. Redis AUTH) is independent of the DataRobot bearer token, which only ever authenticates the HTTP gateway, never a sidecar's native protocol.

9. **The original Compose file is never modified.** The importer generates a `.datarobot.yaml` that the existing, unmodified `dr workload up` deploys. `compose` is strictly an importer/translator — no new deploy, poll, or roll code path.

## Redis + Go web acceptance example (design, not yet built)

- `redis:7.4-alpine` (or similar) as a non-primary service, with no port published beyond its own container-internal port.
- A Go web service as primary, using `github.com/redis/go-redis/v9`, exposing a small HTTP API that reads and writes a cache key.
- The web service connects via `redis.ParseURL(os.Getenv("REDIS_ENDPOINT"))`, where `REDIS_ENDPOINT: "{{ services.redis.endpoint }}"` in the Compose `environment:` block is rewritten by the importer to `redis://127.0.0.1:6379` — exactly the mechanism proven live below.
- Readiness for the web container should include an actual Redis `PING`, not just "process started."

## What's proven vs. still open

**Proven with a live deployment**, using the existing, unmodified `dr workload up` against a hand-written two-container manifest (no repository source changes; all resources torn down afterward):

- Containers in one container group share a pod network namespace: a Redis container and a Go container round-tripped `PING`/`SET`/`GET` over `127.0.0.1:6379`.
- There is no DNS or Kubernetes Service for a non-primary container. Attempting `redis:6379` failed with an authoritative NXDOMAIN from the cluster's real CoreDNS resolver (`dial tcp: lookup redis on 172.20.0.10:53: no such host`) — the resolver answered, it simply holds no record for the sidecar. This rules out a network policy silently blocking traffic; the platform genuinely creates no such record.
- The container's own environment showed Kubernetes auto-injected `LRS_<protonID>_WEB_SERVICE_HOST`/`PORT` variables only for the primary container, confirming exactly one Service is created per container group, scoped to the primary.
- The proposed mechanism itself — an env var carrying a rewritten endpoint, consumed by a real `go-redis` client via `redis.ParseURL` — was built and verified live: `PING` → `PONG`, `SET` → `OK`, `GET` → the written value.
- Multi-container manifests deploy, build, and roll cleanly through the existing `workload up` engine with no code changes required.

**Still open — must be validated during implementation, not assumed:**

- An actual Compose-file parser (e.g. `compose-go`) wired to this translation; only a hand-written manifest has been tested so far.
- Redis authentication/ACL flows.
- The readiness-probe design for a primary whose function depends on a sidecar's health.
- Behavior with three or more services.
- Cross-workload or cross-container-group networking is explicitly out of scope — this design is built to never need it.
