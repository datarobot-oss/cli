# `dr enclave` - Enclave management

Register and operate enclaves — remote outpost clusters that run your DataRobot workloads on infrastructure you control. The `dr enclave` group wraps the Covalent Enclaves API, with one subcommand per operation. It is also available under the aliases `enclaves`, `outpost`, and `outposts`.

## Synopsis

```bash
dr enclave <command> [flags]
```

## Description

An **enclave** is a Kubernetes cluster registered with your DataRobot tenant that workloads can be scheduled onto. `register` returns the enclave's id together with its one-shot installation secrets; the installer materializes those as Kubernetes Secrets on the cluster, reports in, and the enclave activates on its own — there is no manual activate step.

`deactivate` takes an active enclave out of scheduling while keeping the record, and `reactivate` puts it back; `delete` removes it for good.

Two separate things govern who can do what:

- **Per-enclave access** (`dr enclave access`) — the `owner`, `user`, and `consumer` roles on one enclave. Deploying a workload onto an enclave requires at least the `user` role.
- **Collection-level permission** (`dr enclave permission`) — who may create enclaves at all, independent of any single one.

DataRobot system administrators bypass both and always have full access.

> [!NOTE]
> The `enclave` command is currently behind a feature gate. Enable it by exporting `DATAROBOT_CLI_FEATURE_ENCLAVE=true` before running any `dr enclave` subcommand. See [Feature gates](../development/feature-gates.md) for details.
>
> The access and permission subcommands need `ENCLAVE_RBAC_ENABLED` on the server. Where it is off, grants and revokes are accepted as no-ops and every caller shows as fully permitted.

## Quick start

```bash
export DATAROBOT_CLI_FEATURE_ENCLAVE=true

# Register an enclave and capture its one-shot installation secrets
dr enclave register --name my-enclave --output-format json > enclave.json

# Watch for it to come up on its own once the installer reports in
dr enclave list

# Let an entire organization deploy onto it
dr enclave access grant <enclave-id> --role user --org <org-id>

# Check what you can do with it
dr enclave access show <enclave-id>
```

Pin a workload to an enclave with [`dr workload create --enclave <name>`](workload.md).

## Command groups

| Command                        | Endpoint                                          | Purpose                                          |
| ------------------------------ | ------------------------------------------------- | ------------------------------------------------ |
| `dr enclave register`          | `POST   /api/v2/enclaves`                         | Register an enclave, returning install secrets.  |
| `dr enclave get`               | `GET    /api/v2/enclaves/{id}`                    | Show a single enclave.                           |
| `dr enclave list`              | `GET    /api/v2/enclaves`                         | List the enclaves you can see.                   |
| `dr enclave deactivate`        | `POST   /api/v2/enclaves/{id}/deactivate`         | Take an active enclave out of scheduling.        |
| `dr enclave reactivate`        | `POST   /api/v2/enclaves/{id}/reactivate`         | Return a deactivated enclave to service.         |
| `dr enclave delete`            | `DELETE /api/v2/enclaves/{id}`                    | Delete an enclave.                               |
| `dr enclave access grant`      | `PATCH  /api/v2/enclaves/{id}/sharedRoles`        | Grant a role on one enclave.                     |
| `dr enclave access revoke`     | `PATCH  /api/v2/enclaves/{id}/sharedRoles`        | Revoke a recipient's role on one enclave.        |
| `dr enclave access list`       | `GET    /api/v2/enclaves/{id}/sharedRoles`        | List who holds a role on one enclave.            |
| `dr enclave access show`       | `GET    /api/v2/enclaves/{id}/permissions`        | Show effective permissions on one enclave.       |
| `dr enclave permission grant`  | `PATCH  /api/v2/enclaves/createAccess`            | Allow a recipient to create enclaves.            |
| `dr enclave permission revoke` | `PATCH  /api/v2/enclaves/createAccess`            | Stop a recipient from creating enclaves.         |
| `dr enclave permission list`   | `GET    /api/v2/enclaves/createAccess`            | List who may create enclaves.                    |
| `dr enclave permission show`   | `GET    /api/v2/enclaves/permissions`             | Show your own collection-level permissions.      |

All paths are served at the root of your DataRobot host. They are new: the API
previously sat behind a `/covalent` prefix, and its access routes answered under
`/outposts`. Those spellings still work server-side as deprecated aliases, but
the CLI calls only the paths above, so `dr enclave` needs a DataRobot whose
Covalent carries that change — it landed just after Covalent 11.12.0.

## Subcommands

### `register`

Register a new enclave with your tenant. The response carries the enclave id and status along with its **one-shot installation secrets** — they are shown only once and never persisted server-side, so capture them here. `--output-format json` writes them in a form you can feed to the installer; the default text output prints the equivalent `kubectl create secret generic` command for each one.

```bash
dr enclave register --name my-enclave
dr enclave register --name my-enclave --label team=research --label env=prod
dr enclave register --name my-enclave --output-format json
```

| Flag      | Description                                                     |
| --------- | --------------------------------------------------------------- |
| `--name`  | Name for the new enclave (required).                            |
| `--label` | Attach a `key=value` label. Repeatable.                         |

Creating an enclave requires the collection-level `create` permission (see [`permission grant`](#permission-grant)) or system administrator rights.

### `get`

Show one enclave by id.

```bash
dr enclave get <enclave-id>
```

### `list`

List the enclaves you can see. With RBAC enabled this is only those you hold a role on; system administrators see every enclave in the tenant.

```bash
dr enclave list
dr enclave list --output-format json
```

### `deactivate`

Take an active enclave out of scheduling without deleting it. Only an `ACTIVE` enclave can be deactivated; anything else returns a conflict.

```bash
dr enclave deactivate <enclave-id>
```

### `reactivate`

Return a deactivated enclave to service.

```bash
dr enclave reactivate <enclave-id>
```

### `delete`

Delete an enclave. Prompts for confirmation unless `--yes` is given.

```bash
dr enclave delete <enclave-id>
dr enclave delete <enclave-id> --yes
```

| Flag        | Description                    |
| ----------- | ------------------------------ |
| `-y, --yes` | Skip the confirmation prompt.  |

## Access on one enclave

`dr enclave access` manages the roles held on a single enclave. Every subcommand names the enclave by id.

The three roles:

| Role       | Grants                                                          |
| ---------- | --------------------------------------------------------------- |
| `owner`    | Full control, including sharing the enclave with others.        |
| `user`     | View and deploy onto the enclave.                               |
| `consumer` | View only — **not** enough to deploy.                           |

A recipient is exactly one of `--user` (by username), `--user-id`, `--group`, or `--org`.

### `access grant`

```bash
dr enclave access grant <enclave-id> --role user --org <org-id>
dr enclave access grant <enclave-id> --role consumer --user alice@corp.io
dr enclave access grant <enclave-id> --role owner --user-id <datarobot-user-id>
```

| Flag        | Description                                        |
| ----------- | -------------------------------------------------- |
| `--role`    | `owner`, `user`, or `consumer` (required).         |
| `--user`    | Grant a user by username.                          |
| `--user-id` | Grant a user by DataRobot user id.                 |
| `--group`   | Grant a group by id.                               |
| `--org`     | Grant an entire organization by id.                |

### `access revoke`

Remove a recipient's role. Takes the same recipient flags as `grant`.

```bash
dr enclave access revoke <enclave-id> --org <org-id>
```

### `access list`

List the recipients holding a role on the enclave, and which role each holds.

```bash
dr enclave access list <enclave-id>
```

### `access show`

Show the effective permissions on the enclave — what the subject can actually do, after roles, organization membership, and any administrator bypass are resolved. Defaults to you; pass `--user-id` to inspect somebody else (system administrators only).

```bash
dr enclave access show <enclave-id>
dr enclave access show <enclave-id> --user-id <datarobot-user-id>
```

## Permission to create enclaves

`dr enclave permission` governs who may create enclaves at all. It targets no single enclave, so these commands take no enclave id. Recipients are named by id: `--user-id`, `--group`, or `--org`.

### `permission grant`

```bash
dr enclave permission grant --permission create --org <org-id>
dr enclave permission grant --permission create --user-id <datarobot-user-id>
```

| Flag           | Description                                  |
| -------------- | -------------------------------------------- |
| `--permission` | Permission to grant: `create` (required).    |
| `--user-id`    | Grant a user by DataRobot user id.           |
| `--group`      | Grant a group by id.                         |
| `--org`        | Grant an entire organization by id.          |

### `permission revoke`

```bash
dr enclave permission revoke --permission create --org <org-id>
```

### `permission list`

List the recipients who have been granted the create permission. An empty result means nobody holds it; system administrators do not appear, because they bypass the permission rather than holding it.

```bash
dr enclave permission list
```

### `permission show`

Show whether a subject may create enclaves, and where that comes from. Defaults to you; `--user-id` inspects somebody else (system administrators only).

```bash
dr enclave permission show
dr enclave permission show --user-id <datarobot-user-id>
```

## Output formats

Every subcommand accepts `--output-format` (`text` or `json`). `text` is the default and is meant for reading; `json` is the one to use in scripts, and the only practical way to capture `register`'s installation secrets.

## See also

- [`dr workload`](workload.md) — deploy workloads, and pin one to an enclave with `--enclave <name>`
- [Feature gates](../development/feature-gates.md) — enabling `DATAROBOT_CLI_FEATURE_ENCLAVE`
