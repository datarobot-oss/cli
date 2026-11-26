# dr llm

Deploy LLMs on the Dynamo operator from [recipes](recipe.md), roll them onto new recipe versions, and chat with them, through the Workload API.

> **Feature-gated.** The `dr llm` commands are shown when `DATAROBOT_CLI_FEATURE_WORKLOAD_API_LLM=true` is set. The server also needs LLM serving enabled on the installation and the `ENABLE_WORKLOAD_API_TOKEN_FACTORY` feature flag granted to your user, organization or group; the CLI names whichever gate is closed when a call is refused.
>
> `llm` used to be an alias of [`dr llm-gateway`](llm-gateway.md). `dr llm list` and `dr llm select` still run the `llm-gateway` commands, with or without the gate.

## Synopsis

```bash
dr llm deploy --recipe <recipe-id> --name <name> --bundle <bundle-id> [--wait]
dr llm rollout <workload-id> --recipe-version N | --latest | --recipe <id> [--wait]
dr llm chat <workload-id> [prompt] [--stream]
dr llm status|watch|endpoint [<workload-id>]
```

## Commands

| Command    | API call                                                                                 | What it does                                             |
| ---------- | ---------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `deploy`   | `GET /llmRecipes/{id}`, then `POST /workloads/`                                          | Deploys a recipe as an llm workload                      |
| `rollout`  | `GET /workloads/{id}`, then `POST /workloads/{id}/replacement/`                          | Rolls a running llm workload onto another recipe version |
| `chat`     | `GET /workloads/{id}`, `GET /artifacts/{id}`, then `POST {endpoint}/v1/chat/completions` | Sends a chat request to a deployed LLM                   |
| `status`   | Same as `dr workload status`                                                             | Is it up                                                 |
| `watch`    | Same as `dr workload status --watch`                                                     | Follows the status until it settles                      |
| `endpoint` | Same as `dr workload endpoint`                                                           | Prints the URL                                           |

Logs, events, settings, stop, start and delete are [`dr workload`](workload.md) commands.

## `deploy`

One request: an llm artifact for the model (or `--artifact-id` to reuse one) and a runtime naming the recipe.

```bash
dr llm deploy --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --name kimi-prod --bundle gpu.b200.x8 --wait
```

- The model defaults to the recipe's own; the served name (what clients send as `model`) to its repository name, lowercased.
- An `invalid` recipe is refused before anything is created. The server also refuses a recipe that is not this tenant's, not for the cluster's engine, or not serving the model.
- `--wait` polls until the workload is running, then sends one chat request (`max_tokens: 1`), so done means the model answers. `--no-smoke` skips that request.
- `--dry-run` prints the request.

| Flag | Maps to | Default |
| --- | --- | --- |
| `--recipe` (required), `--recipe-version N` | `runtime.recipe` | latest when scheduled |
| `--name` (required), `--description` | `name`, `description` | |
| `--bundle` (repeatable, required) | `runtime.workers.resourceBundles` | |
| `--model`, `--revision`, `--model-source huggingFace\|s3` | `artifact.spec.model.source` | the recipe's model, `huggingFace` |
| `--served-name` | `artifact.spec.model.servedName` | from the repository name |
| `--tool-calling`, `--reasoning` (`auto\|on\|off`) | `artifact.spec.model.toolCalling`, `reasoning` | `auto` (omitted) |
| `--artifact-id` | `artifactId` | inline artifact |
| `--token-grid`, `--concurrency-slots`, `--tokens-per-second` | `runtime.tokenGrid` | off |
| `--use-case-id` | `useCaseId` | |
| `--wait`, `--poll-interval`, `--poll-timeout`, `--no-smoke` | | no wait |

Model weight storage and Enclave placement are not supported yet: the workload runs on this installation with its default storage.

## `rollout`

A rolling replacement on the same artifact with only `runtime.recipe` changed: the new generation starts next to the old one and traffic moves when it is ready, so the endpoint and served name do not change. A recipe change is a replacement on the server; a settings update refuses it.

```bash
dr llm rollout 68b0c1d2e3f4a5b6c7d8e9f0 --recipe-version 3 --wait
dr llm rollout 68b0c1d2e3f4a5b6c7d8e9f0 --latest
dr llm rollout 68b0c1d2e3f4a5b6c7d8e9f0 --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --recipe-version 1
```

The workload's current runtime is sent back with read-only (`resolved*`) fields removed, mounts reduced to their storage ids, and the recipe replaced. A replacement already in flight is refused. `--wait` polls until the replacement finishes and then sends one chat request; `--dry-run` prints the request.

## `chat`

```bash
dr llm chat 68b0c1d2e3f4a5b6c7d8e9f0 "Say hello in one word."
echo "Summarise Dynamo in a sentence" | dr llm chat 68b0c1d2e3f4a5b6c7d8e9f0 --stream
```

Sends `POST {endpoint}/v1/chat/completions` to the endpoint workload-api advertises for the workload, with `model` and the `x-ai-eg-model` header set to the artifact's served name (the AI gateway authenticates before it reads the body), using your DataRobot credentials.

- The prompt is the second argument, or stdin. `--system` adds a system message, `--max-tokens` defaults to 256.
- Text output prints the reply and, on stderr, the token usage; `--stream` prints the reply as it is generated; `--output-format json` prints the raw completion.
- On a `400`/`422` the command lists the models the endpoint serves, to tell a model with no chat template (an embedding model, for example) from one that is not served.
- A `429` is Token Grid refusing admission; the server's reason is printed.
- The credentials are only sent to this DataRobot installation's host. An Enclave endpoint on another host is refused for now.

## `status`, `watch`, `endpoint`

The `dr workload` commands under `dr llm`, with the same arguments, flags and output: [`status`](workload.md), `status --watch`, and [`endpoint`](workload.md).
