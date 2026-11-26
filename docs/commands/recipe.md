# dr recipe

Manage the serving recipes LLM workloads deploy with, over the Workload API's `/api/v2/llmRecipes/`.

> **Feature-gated.** `dr recipe` is shown when `DATAROBOT_CLI_FEATURE_WORKLOAD_API_LLM=true` is set. The server also needs LLM serving enabled on the installation and the `ENABLE_WORKLOAD_API_TOKEN_FACTORY` feature flag granted to your user, organization or group; the CLI names whichever gate is closed when a call is refused.

## Synopsis

```bash
dr recipe <command> [args] [flags]
```

## Description

A **recipe** is a known-good, versioned serving configuration for an open-weights model on the Dynamo operator. Its payload is one of:

| Format | Content |
| --- | --- |
| `dynamo.dgd/v1beta1` | `objects`: a DynamoGraphDeployment and the ConfigMaps it references |
| `vllm.recipe/v1` | `recipe`: a recipes.vllm.ai model document |

Content is validated on create and patch. A recipe that fails policy is still stored, with status `invalid` and the reasons listed, and cannot be deployed. A new payload makes a new version; name and description changes do not. Recipes belong to a tenant: the creator or a tenant admin can patch or delete one, and anyone in the tenant can copy it.

## Commands

| Command | API call | What it does |
| --- | --- | --- |
| `dr recipe list` | `GET /llmRecipes/` | Lists the tenant's recipes, newest first |
| `dr recipe get <id>` | `GET /llmRecipes/{id}` | Shows a recipe, its diagnostics and descriptor; `--payload` prints only the payload |
| `dr recipe create [<file>]` | `POST /llmRecipes/` | Registers a recipe from a JSON or YAML file |
| `dr recipe copy <id> --name <name>` | `GET /llmRecipes/{id}`, then `POST /llmRecipes/` | Creates a new recipe from an existing one |
| `dr recipe patch <id> [<file>]` | `PATCH /llmRecipes/{id}` | Renames or describes, or replaces the content as a new version |
| `dr recipe delete <id>` | `DELETE /llmRecipes/{id}` | Deletes a recipe after confirmation |
| `dr recipe deployments <id>` | `GET /workloads/?type=llm` | Lists the llm workloads that run the recipe |

Every command accepts `--output-format json`.

## Recipe files

`create` and `patch` take the file positionally or with `--from-file`. The file is either a full create request or a bare payload:

```yaml
# qwen3-32b.vllm.yaml: a full request
name: qwen3-32b-tp2
description: Qwen3-32B on two H100/H200, tool calling and reasoning on
payload:
  format: vllm.recipe/v1
  recipe:
    hf_id: Qwen/Qwen3-32B
    base_args: ["--max-model-len", "32768", "--gpu-memory-utilization", "0.9"]
    features:
      tool_calling:
        args: ["--enable-auto-tool-choice", "--tool-call-parser", "hermes"]
      reasoning:
        args: ["--reasoning-parser", "qwen3"]
    variants:
      default:
        tp: 2
        vram_minimum_gb: 80
        supported_hardware: [h100, h200]
```

```bash
dr recipe create qwen3-32b.vllm.yaml

# A bare payload (a top-level "format") needs --name
dr recipe create qwen3-0.6b.dgd.yaml --name qwen3-0-6b-agg
```

A Dynamo recipe must name the installation's Dynamo runtime image; the server refuses a recipe that names another.

## Editing a recipe

```bash
dr recipe get <id> --payload > recipe.yaml
# edit recipe.yaml
dr recipe patch <id> recipe.yaml --if-version <N>
```

`--if-version` names the version being replaced. If someone changed the recipe in the meantime, the server answers `409` and the CLI suggests re-reading it. To change a recipe you cannot patch, copy it:

```bash
dr recipe copy <id> --name my-variant
```

## Flags

### `list`

- `--search <text>`: case-insensitive match on name and description.
- `--model <repo-id>`, `--kind dynamo|vllm`, `--backend <name>`, `--gpu-sku <sku>`: filter on the content.
- `--status any|available|invalid`: default `any`.
- `--created-by <user-id>`, `--id <id>` (repeatable).
- `--limit <N>` (default 50), `--offset <N>`.

### `get`

- `--version <N>`: a past version of the content.
- `--payload`: only the payload, as YAML (JSON with `--output-format json`).

### `create`

- `--from-file <path>`, `--name <text>`, `--description <text>`.
- `--descriptor-file <path>`: what the recipe needs and is for; inferred from the payload when omitted.

### `copy`

- `--name <text>` (required), `--description <text>` (default: the source's), `--version <N>` (default: latest).

### `patch`

- `--name <text>`, `--description <text>`: metadata only, no new version.
- `--from-file <path>` or a positional file: new content, a new version.
- `--if-version <N>`: required with new content.
- `--descriptor-file <path>`: only together with new content.

### `delete`

- `--yes`/`-y`: skip the confirmation (or set `DATAROBOT_CLI_NON_INTERACTIVE=1`).

### `deployments`

- `--status <status>` (repeatable), `--limit <N>` (default 50).

The Workload API has no recipe filter on workloads yet, so `deployments` reads every llm workload in the tenant and matches the recipe here.
