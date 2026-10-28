# CI Workflows & Composite Actions

## Composite Action Changes Are Validated By The PR's Own CI

Retired 2026-10-22 (fix for the v0.15.2 release incident). Composite actions under
`.github/actions/` are now referenced **locally** (`./.github/actions/<name>`) by all
workflows, including reusable ones, with `actions/checkout` always first. A PR that
edits a composite action's `action.yaml` therefore exercises the change in its own
CI run — no temporary ref-pointing or post-merge `workflow_dispatch` verification
needed.

The old `datarobot-oss/cli/.github/actions/<name>@main` pattern is forbidden: the
runner downloads the entire repository tarball to materialize a remote action, into
a fixed 2 GiB buffer that this repo now exceeds (`docs/plugins/` tracks ~2 GiB of
binaries). Flag any new remote same-repo action ref as a bug: it will fail at job
setup with `Cannot write more bytes to the buffer than the configured maximum buffer
size` as soon as the tarball is over the limit, and it silently masks PR changes to
the action. See `.github/workflows/README.md` for the full rationale.

## The Trusted-Actions Lane Must Cover Every Composite Step On Fork Paths (#1031)

Local action refs resolve against the checked-out ref — fork-controlled for fork
smoke tests. Reusables called from `fork-smoke-tests.yaml` take a `trusted-actions`
boolean and run their composite steps from upstream main checked out into
`ci-tooling/`. Local `uses:` paths cannot be dynamic, so each composite step exists
as an `if:`-gated pair (plain + `(trusted)`).

Flag as bugs:

- A composite step added to a reusable with a `trusted-actions` lane but missing its
  `(trusted)` twin (or vice versa) — the pairs drift silently.
- A reusable-call in `fork-smoke-tests.yaml` (or any other fork-ref caller) that
  omits `trusted-actions: true` while the reusable runs local composite actions.
- `persist-credentials` left default (`true`) on the `ci-tooling` checkout.
