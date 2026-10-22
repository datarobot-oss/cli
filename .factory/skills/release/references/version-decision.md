# Deciding the next version

`docs/development/releasing.md` says "decide on the next version number" and stops there. This is
the missing procedure.

## 1. Establish the range

The baseline is the last **stable** tag, not the last tag. Dev and rc tags are cut on `main`
between stables and must not reset the comparison.

```bash
LAST_STABLE=$(git tag --sort=-v:refname | rg -v -- '-[a-zA-Z]' | head -1)
git log --oneline "$LAST_STABLE"..main
```

## 2. Filter to user-visible change

Only what a `dr` user can observe moves the version. Read each commit's actual diff — a
`fix:` prefix on a commit that removes a flag is still a breaking change, and a `feat:` prefix
on an internal refactor is a PATCH.

Surfaces that count as the public interface:

- Command and subcommand names, and their aliases.
- Flag names, shorthands, defaults, and whether a flag is required.
- `drconfig.yaml` keys and their meaning — see `config.PersistableKeys`.
- Environment variables (`DATAROBOT_CLI_*`, `DR_API_TOKEN`).
- `--output-format json` payload shape. Adding a field is additive; renaming or removing one is
  breaking.
- Exit codes.
- Feature-gated commands are **not** public until their gate defaults on. A gated command
  shipping behind `DATAROBOT_CLI_FEATURE_*` is a PATCH, not a MINOR.

Not public: internal packages, test helpers, CI workflow changes, lint config, dependency bumps
with no behavior change.

## 3. Classify and take the maximum

Classify every user-visible change independently, then take the highest class present. One
breaking change in a hundred PATCHes still moves the version.

While below `v1.0.0`, a breaking change is conventionally released as a MINOR bump rather than
`v1.0.0`. Confirm with the user rather than assuming they are ready to declare 1.0.

`docs/development/releasing.md` pre-1.0 mapping additionally treats new features as PATCH; the
maintainer's practice has been MINOR for new public commands and GA enablement. When the range
is small and feature-only, present both at the approval step.

## 4. Present evidence, then stop

Produce a table before proposing anything:

| Change | Class | PR |
| --- | --- | --- |
| `dr run` now accepts bare template markers | PATCH | #812 |
| Removed PR image publishing | PATCH | #793 |

Then state the proposed version and the single rule that drove it. Wait for approval. The user
decides; the table is what lets them decide quickly.
