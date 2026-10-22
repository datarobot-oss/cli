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
