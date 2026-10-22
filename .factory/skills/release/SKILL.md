---
name: release
description: "Trigger: release the CLI, cut a release, ship vX.Y.Z, bump the version, semver review, promote a pre-release. Readiness-review the repo, decide the next dr version, tag it, drive the goreleaser pipeline, write release notes, announce in Slack."
license: Apache-2.0
metadata:
  author: dr-cli maintainers
  version: "1.0"
---

# Release

Ship the `dr` CLI. The tag push is the release — everything after it is automated and public.

## Activation Contract

Load when the user asks to release, cut, ship, tag, or bump a version of this repo; to decide the
next semver number; to review release readiness; to draft release notes or the #dr-cli
announcement; or to diagnose a release stuck in pre-release.

Do NOT load for plugin `minCLIVersion` work, `internal/version` code changes, or CLI plugin
releases (they publish through their own registry, not this pipeline).

## Hard Rules

- **Confirm the exact version with the user before pushing any tag.** The push publishes binaries,
  a GHCR image, and a Homebrew cask; there is no clean undo.
- Push tags to `upstream` (`datarobot-oss/cli`), never `origin`.
- Never tag a commit that already carries a semver tag — duplicate tags make `git describe`
  ambiguous.
- Create **lightweight** tags (`git tag vX.Y.Z`), never annotated ones (`-a`/`-s`). The tag
  history is lightweight; an annotated tag is an accident, not a style.
- Never hand-edit the pre-release flag. `promote-release` clears it by **deleting and recreating**
  the GitHub release; a manual flip leaves assets and the release ID inconsistent.
- A tag matching `-[a-zA-Z]` (`-rc.1`, `-dev0`) **never** promotes to stable. That is the gate,
  not a bug.
- `CHANGELOG.md` is context, not a release gate. Its `[Unreleased]` section has gone stale
  before; verify entries against the actual diff before trusting it.
- Compare from the **latest stable tag**, never a dev or pre-release tag.
- Post the Slack announcement **only after** `promote-release` succeeds, and edit release notes
  after promotion.
- Prefer CodeGraph for the release-impact review when available, before broad file searches.

## Decision Gates

| Diff since last stable contains | Bump |
| --- | --- |
| Removed/renamed command or flag, changed default, breaking config or JSON-output change | MINOR (pre-1.0: breaking does not force `v1.0.0`) |
| New public command, subcommand, or flag; a feature gate turned on (GA) | MINOR |
| Bug fixes, perf, docs, deps, internal refactor; gated commands not yet enabled | PATCH |
| Wants a verification build before committing to a number | pre-release tag `vX.Y.Z-rc.N` |

Note: `docs/development/releasing.md` pre-1.0 mapping treats new features as PATCH. When the
range is small and feature-only, present both options at the approval step; the user decides.

Ambiguous or mixed? Present the evidence and let the user pick. Do not silently round down.

## Execution Steps

**Readiness** (ends with a go/no-go recommendation):

1. `git checkout main && git pull upstream main`. Refuse to proceed on a dirty tree.
2. Baseline: `git tag --sort=-v:refname | rg -v -- '-[a-zA-Z]' | head -1`.
3. Review `CHANGELOG.md`, `AGENTS.md`, `Taskfile.yaml`, `goreleaser.yaml`,
   `.github/workflows/release.yaml`, `.github/workflows/README.md`, `docs/properdocs.yml`,
   and `docs/commands/` against the diff in range.
4. Look for blockers: unmerged risky changes, missing validation, documentation gaps. A doc gap
   for a feature-gated command is acceptable.
5. Classify the diff per `references/version-decision.md`; propose one version with a
   per-change evidence table. **Stop and get explicit user approval.**

**Ship:**

6. Verify green: `task lint && task test` (offer `task smoke-test` if `DR_API_TOKEN` is set).
7. Tag and push: `git tag vX.Y.Z && git push upstream vX.Y.Z`.
8. Watch: `gh run watch $(gh run list --workflow=release.yaml --limit 1 --json databaseId -q '.[0].databaseId')`.
9. Confirm promotion: `gh release view vX.Y.Z --json isPrerelease` must be `false`. If it is
   still `true`, read `references/pipeline.md` before touching anything.

**Notes and announcement:**

10. Rewrite the GitHub release notes after promotion — see Output Contract for the required
    shape, including `## Brought to you by` via the compare API (exclude bots and coding agents).
11. Draft the #dr-cli announcement per `references/announcement-style.md`. Verify every
    flag/command example and every thanked person against the commit-author list before posting.
12. New announcement-style learnings go into `references/announcement-style.md` via a follow-up
    PR — not into agent-local memory.

## Output Contract

Return:

- Readiness verdict (release / no-release) with named blockers.
- Proposed version, and the rule that justifies it; evidence table: each user-visible change in
  range → its bump class → PR link.
- The exact tag commands, run only after approval.
- Final release URL and `isPrerelease` state; anything that stayed pre-release, and why.
- Release notes shape: readable sections (not commit bullets), major highlights, feature-gate
  status, operational notes for new functionality, full stable-to-stable range, and
  `## Brought to you by` listing human authors:

```sh
gh api repos/datarobot-oss/cli/compare/<old>...<new> --jq \
  '[.commits[].author.login] | map(select(. != null)) | unique | .[]'
```

- Announcement: short main post per the style guide, posted link, and any thread replies.

## References

- `references/version-decision.md` — how to classify a diff into a semver bump.
- `references/pipeline.md` — what runs after the tag push; failure modes and rollback.
- `references/announcement-style.md` — the #dr-cli Slack announcement structure, tone, and
  verification checklist.
- `references/worked-examples.md` — anonymized end-to-end release runs.
- `../../../docs/development/releasing.md` — the full human-facing release doc.
