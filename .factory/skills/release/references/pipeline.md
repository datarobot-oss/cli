# What happens after the tag push

Source: `.github/workflows/release.yaml`, `goreleaser.yaml`.

## Trigger

Tag patterns `v*.*.*` and `v*.*.*.*` on push, created as **lightweight** tags
(`git tag vX.Y.Z`, no `-a`/`-s`). Nothing else starts a release — there is no
`workflow_dispatch` on this workflow.

## Job chain

1. **`release`** — goreleaser `release --clean` builds linux/darwin/windows, publishes the GHCR
   image via buildx (`Dockerfile.goreleaser`), signs and notarizes the macOS binaries, and pushes
   the Homebrew cask to `datarobot-oss/homebrew-taps`. Then it runs
   `gh release edit <tag> --prerelease` — **every** release is born a pre-release.
2. **`verify-installation`** — `_install-tests.yaml` installs the published version on
   ubuntu, macos, and windows runners.
3. **`pre-release-smoke-test`** — `_pre-release-smoke.yaml`, needs `DR_API_TOKEN`.
4. **`promote-release`** — only after both verifications pass.

## The promotion step is a delete and recreate

`promote-release` does not flip a flag. It reads the notes and name, downloads every asset,
runs `gh release delete <tag> --yes --cleanup-tag=false`, and recreates the release from the
downloaded assets. The git tag survives; the GitHub release ID does not.

Consequences:

- Notes edited between publish and promotion are carried forward by the copy, but edits made
  *during* the window can race and be lost. Edit release notes after promotion, not before.
- If `promote-release` fails midway, the release can be deleted but not yet recreated. Check
  `gh release view <tag>` before retrying anything.

## Why a release stays a pre-release

| Cause | Check |
| --- | --- |
| Tag is a semantic pre-release (`-rc.1`, `-dev0`) | Expected. The gate matches `-[a-zA-Z]` and exits 0. |
| Install tests failed | `_install-tests.yaml` logs; usually a missing or corrupt asset. |
| Smoke test failed | `_pre-release-smoke.yaml`; often an expired `DR_API_TOKEN` secret. |
| `promote-release` itself errored | Its step log; asset download or `gh release create`. |

A failed verification leaving the release as a pre-release is the system working — users
installing "latest" never resolve to it.

## The `-dev0` convention

Observed in tag history (`v0.2.86-dev0`, `v0.2.87-dev0`): intermediate tags cut on `main`
between stable releases, and ancestors of the next stable. They exercise the full publish path
without ever promoting. Not documented in `docs/development/releasing.md`, which mentions only
`-rc.N`/`-beta.N`/`-alpha.N`.

## Rollback

Full procedure in `docs/development/releasing.md` under "Rollback". Short version: delete the
GitHub release, delete the tag locally and on `upstream`, fix, then tag a **new** version. Never
retag the same commit — `git describe` drives the version string and duplicate tags make it
ambiguous.
