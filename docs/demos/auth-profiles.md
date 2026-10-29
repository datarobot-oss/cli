# Hands-on tour: named auth profiles

This is a beginner-friendly, self-contained walkthrough of DataRobot CLI auth
profiles. Everything here runs **offline**: it uses a throwaway configuration
directory and `.invalid` placeholder URLs, so nothing in this tour talks to
DataRobot, and your real credentials are never read or touched.

By the end you will be able to:

- Explain the difference between the root-level credentials and an explicit
  `default` profile.
- Select a profile with `--profile` or `DATAROBOT_CLI_PROFILE`.
- Create, inspect, and delete profiles with `dr auth profile`.
- Read the error the CLI prints for an unknown profile and recover from it.

## Mental model

`drconfig.yaml` has two layers of credentials:

| Layer | Where it lives | How it is selected |
|---|---|---|
| Root-level credentials | Top-level `endpoint:`/`token:` keys | **Omitting** `--profile` — the ordinary, default behavior |
| A named profile | A section under `profiles:` | `--profile <name>` or `DATAROBOT_CLI_PROFILE=<name>` |

One name deserves special care: `default`.

- Omitting the profile selection uses the **root-level** credentials. It does
  **not** activate a section called `profiles.default`.
- `default` is also a legal *explicit* profile name. If you (or
  `dr auth login --profile default`) create a `profiles.default` section, then
  `dr --profile default …` selects that section.
- So `dr auth profile show` (omitted) and
  `dr --profile default auth profile show` can show different credentials.
  The positional form `dr auth profile show default` is a compatibility
  exception: it prefers an explicit `profiles.default` section and falls back
  to the root-level credentials when none is defined.

If you know the AWS CLI: the idea of an explicit `default` profile that is
distinct from "no selection" is similar to `--profile default` there. The
analogy stops there — the DataRobot CLI keeps separate root-level
credentials, inherits `ca-cert` across profiles, and normalizes profile-name
case and whitespace, none of which match AWS behavior.

Only `endpoint`, `token`, and `ca-cert` are profile-scoped. A profile's
endpoint and token are an **atomic pair**: a profile that defines either one
never inherits the missing half from the root-level credentials. Every other
setting is global and shared by all profiles.

## Before you start

You need `dr` on your `PATH` (`dr --version` works). No DataRobot account,
token, or network access is needed for this tour.

## Setup (safe scratch environment)

Open a **new terminal window** and paste this block. It points `HOME`,
`XDG_CONFIG_HOME`, and `DATAROBOT_CLI_CONFIG` at a scratch directory, creates
an empty scratch config, and defines a `dr` alias bound to those settings.

Because of the `alias`, these commands only ever read and write
`$DEMO/drconfig.yaml`. Your real `~/.config/datarobot/drconfig.yaml` is never
opened. The exports and alias last only for this terminal session and are
cleaned up at the end of the tour.

```bash
DEMO="$(mktemp -d /tmp/dr-profiles-demo.XXXXXX)"
mkdir -p "$DEMO/home"
export HOME="$DEMO/home" XDG_CONFIG_HOME="$DEMO/home/.config"
export DATAROBOT_CLI_CONFIG="$DEMO/drconfig.yaml"
export DATAROBOT_CLI_DISABLE_TELEMETRY=true
touch "$DATAROBOT_CLI_CONFIG"
alias dr="env HOME=$DEMO/home XDG_CONFIG_HOME=$DEMO/home/.config DATAROBOT_CLI_CONFIG=$DEMO/drconfig.yaml dr"

# Seed the root-level credentials with a placeholder endpoint and a dummy token.
cat > "$DATAROBOT_CLI_CONFIG" <<'EOF'
# Root-level credentials for the tour
endpoint: https://app.acme-demo.invalid/api/v2
token: DEMO-ROOT-DUMMY-KEY-NOT-A-REAL-TOKEN

profiles:
  # EU installation
  eu:
    endpoint: https://eu.acme-demo.invalid/api/v2
    token: DEMO-EU-DUMMY-KEY-NOT-A-REAL-TOKEN
EOF
```

> The values above are placeholders: `.invalid` is a reserved top-level
> domain that cannot resolve, and the `DEMO-…-DUMMY-KEY` strings are not
> tokens. Nothing in this tour sends a credential anywhere.

**Check the starting state:**

```bash
dr auth profile list
```

Expected output (the active marker `✓` is on the root-level row). No creation
hint prints here because this scratch config defines an explicit profile;
that hint appears on stderr only when no explicit profiles are defined. On
the very first `dr` invocation in a fresh terminal you
may also see a one-time welcome banner and a warm-up delay before the table
appears; it does not come back for the rest of the tour:

```text
DataRobot Profiles
──────────────────
╭──────────────────────┬──────────────────────────────────────┬───────┬────────╮
│ NAME                 │ ENDPOINT                             │ TOKEN │ ACTIVE │
├──────────────────────┼──────────────────────────────────────┼───────┼────────┤
│ default (root-level) │ https://app.acme-demo.invalid/api/v2 │ set   │ ✓      │
│ eu                   │ https://eu.acme-demo.invalid/api/v2  │ set   │        │
╰──────────────────────┴──────────────────────────────────────┴───────┴────────╯
```

**Talking point:** the first row is labelled `default (root-level)`. That is
*not* a profile named `default`; it is the root-level credential layer, shown
under the name `default` because "no selection" has historically been
addressed as `default`. The `eu` row is a genuine named profile.

## Selecting a profile

Selection happens three ways: omit it (root), use the flag, or use the
environment variable. The flag beats the environment variable.

```bash
# Omitted selection: root-level credentials
dr auth profile show

# Flag selection: the eu profile
dr --profile eu auth profile show

# Environment selection for a whole shell session
DATAROBOT_CLI_PROFILE=eu dr auth profile show
```

Expected output for `dr --profile eu auth profile show`:

```text
eu (active)
───────────
  config file: /tmp/dr-profiles-demo.XXXXXX/drconfig.yaml
  endpoint: https://eu.acme-demo.invalid/api/v2
  token: set
  ca-cert: - (inherited from the root-level credentials)
```

**Talking points:**

- The `config file:` line names the file the CLI actually read — here, your
  scratch config. It varies with your `$DEMO` path.
- `eu` did not define a `ca-cert`, so it shows what it would use: inherited
  from the root-level credentials. Only `endpoint` and `token` are atomic;
  other settings fall back like this one.
- The token is never printed — only `set` / `not set`.

### Root versus explicit `default`

```bash
# Positional "default": explicit section if defined, otherwise root.
dr auth profile show default

# Flag selection of an explicit default that is NOT defined in this config.
dr --profile default auth profile show
```

The first command succeeds and shows the root-level credentials (same values
as `dr auth profile show`), because this scratch config defines no
`profiles.default` section. The second fails:

```text
Error: unknown profile "default"; defined profiles: eu

Omitting the profile selection uses the root-level credentials, which are
separate from a profile named "default": omission never activates
profiles.default, and this config does not define one.

To create the explicit "default" profile, run one of:
  dr auth login --profile default
  dr auth set-url --profile default
Or drop the selection (remove --profile default, or unset
DATAROBOT_CLI_PROFILE) to use the root-level credentials.
```

**Talking point:** this is the single most common profile misconception.
`dr auth profile show` (omitted) and `dr --profile default …` are *different
selections*. They agree only when an explicit `profiles.default` section
exists, in which case the flag form selects it and the omitted form still
uses root.

## Creating a profile

There is no `create` subcommand: a profile comes into existence the first
time a write targets it — `dr auth login --profile <name>` for real
credentials, or `auth set-url` for the endpoint alone. For this offline tour
we use `set-url` with `--skip-auth`, which skips the credential check so no
network request is made:

```bash
dr auth set-url --profile work https://work.acme-demo.invalid --skip-auth
```

Expected output:

```text
Saved profile "work" to /tmp/dr-profiles-demo.XXXXXX/drconfig.yaml
WARN  Authentication checks are disabled via the '--skip-auth' flag. This may cause API calls to fail.
```

**Talking points:**

- The notice names the profile it saved and the actual config file — you can
  always tell where a write landed.
- This created `profiles.work` with an endpoint and **no token**. A real
  setup would follow with `dr --profile work auth login`, which saves the
  token into the same section.

### Endpoint/token pairing

```bash
dr auth profile show work
```

Expected output:

```text
work
────
  config file: /tmp/dr-profiles-demo.XXXXXX/drconfig.yaml
  endpoint: https://work.acme-demo.invalid/api/v2
  token: not set
  ca-cert: - (inherited from the root-level credentials)
  This profile has its own endpoint and no token of its own; it does not use the root-level token (endpoint and token are always paired).
```

**Talking point:** the last line is the pairing rule in action. `work` owns
its endpoint, so it will not silently pair that endpoint with your
root-level token — sending one installation's key to another installation is
exactly the mistake the atomic pair prevents. Log in under `--profile work`
to give it its own token.

## Machine-readable output

```bash
dr auth profile show work --output-format json
```

Expected output:

```json
{
  "profile": {
    "name": "work",
    "active": false,
    "endpoint": "https://work.acme-demo.invalid/api/v2",
    "endpoint_own": true,
    "has_token": false,
    "has_token_own": true,
    "ca_cert": "",
    "ca_cert_own": false
  }
}
```

**Talking points:**

- `ca_cert` is always present in show JSON — an empty string when no
  certificate is configured. Scripts can rely on the key existing.
- `endpoint_own` and `has_token_own` describe ownership of the endpoint/token
  *pair*, not each key individually: `work` defines the endpoint half of the
  pair, so both ownership flags are true even though `has_token` is false.
  Only the resolved fields (`endpoint`, `has_token`) say which values are
  actually present.

## Unknown profiles and recovery

Ask for a profile that does not exist:

```bash
dr --profile staging auth profile show
```

Expected output:

```text
Error: unknown profile "staging"; defined profiles: eu, work

To list the defined profiles, run:
  dr auth profile list

To create this profile, run one of:
  dr auth login --profile staging
  dr auth set-url --profile staging
```

The error is the same whether you passed the name by flag, environment
variable, or as a positional argument to `auth profile show`, and it always
lists the profiles that *are* defined.

Now run the remediation it suggested (offline form):

```bash
dr auth set-url --profile staging https://staging.acme-demo.invalid --skip-auth
dr auth profile show staging
```

Both succeed, and `dr auth profile list` now shows three named profiles.
**Talking point:** every unknown-profile error ends in a command you can run
verbatim; recovery does not require re-deriving the syntax.

## Deleting profiles

```bash
# delete and its rm alias do the same thing
dr auth profile delete staging
dr auth profile rm work
```

Expected output, once per command:

```text
Deleted profile "staging".
Deleted profile "work".
```

Only the named section is removed: root-level credentials, other profiles,
global settings, and unrelated comments are kept. Names normalize like
`--profile` names, so `rm " staging "` would also have worked.

The reserved name `default` is refused:

```bash
dr auth profile delete default
```

Expected output:

```text
Error: the profile name "default" is reserved and cannot be deleted

Omitting the profile selection uses the root-level credentials, which are
also addressed as "default"; to clear those, run:
  dr auth logout

An explicit profiles.default section, when one is defined, is likewise
protected and cannot be removed by delete.
```

**Talking point:** because `default` doubles as the address of the
root-level credentials, deleting it would be ambiguous; `dr auth logout` is
the command that clears root-level credentials.

## Shell completion

Profile names complete for the global `--profile` flag and for the
`show`/`delete` positional arguments (in a shell with completions installed —
see [Shell completions](../user-guide/shell-completions.md)). `default` is
offered only when an explicit `profiles.default` section exists. Deletion
completion never suggests deleting `default`.

## Cleanup

Remove only what this tour created and undo the session-local settings:

```bash
unalias dr
unset HOME XDG_CONFIG_HOME DATAROBOT_CLI_CONFIG DATAROBOT_CLI_DISABLE_TELEMETRY
rm -rf "$DEMO"
```

Closing the terminal window achieves the same thing; the scratch directory
lives under `/tmp` and is reclaimed by your OS eventually either way. If you
want to be sure no `DEMO` variable survives, run `echo "$DEMO"` after the
cleanup — it should print one empty line.

## Optional: real login (human-operated, outside this tour)

The steps above are deliberately offline. To configure a profile against a
real DataRobot installation, you run the same commands with real URLs and
without `--skip-auth`, in your own terminal, with your own credentials:

```bash
dr auth set-url --profile work https://app.datarobot.com
dr --profile work auth login
```

This opens your browser and stores a real API token in `profiles.work` of
your real `drconfig.yaml`. It is **not** part of the offline tour, the
rehearsal, or any recording of it, and the commands above are shown for
context only — run them yourself when you are ready to authenticate.
