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
generic semver rule says MINOR. **Always present both numbers at the first version
recommendation, naming the runbook mapping explicitly** — do not resolve the conflict silently
in either direction. For a bugfix-only range the two rules agree and a single PATCH proposal is
fine. This is mechanical, not judgment: the tension surfaced only after the tag was up in three
separate releases (v0.15.0, v0.16.0, v0.18.0) before the rule was made unconditional, and each
time the maintainer chose to let the already-pushed tag ride.

## 4. Present evidence, then stop

Produce a table before proposing anything:

| Change | Class | PR |
| --- | --- | --- |
| `dr run` now accepts bare template markers | PATCH | #812 |
| Removed PR image publishing | PATCH | #793 |

Then state the proposed version and the single rule that drove it. Wait for approval. The user
decides; the table is what lets them decide quickly.

## 5. A pushed tag is burned

Once the tag is up and goreleaser is running, "let it ride" beats abort-and-retag — a
half-published release cannot be cleanly walked back, and re-tagging re-runs notarization and
Homebrew on an ambiguous ref. If the number turns out to be debatable, finish the release and
record the disagreement for the next one. Never retag the same commit with a different number
(see `pipeline.md`); the version string in shipped binaries comes from `git describe`.
