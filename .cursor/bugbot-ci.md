# CI Workflows & Composite Actions

## Composite Action Changes Require Special Testing

Reusable composite actions under `.github/actions/` (e.g. `install-deps`, `setup`,
`install-dr-bin`) are referenced by workflows like `_smoke.yaml` and
`_pre-release-smoke.yaml` via a pinned `@main` ref
(`datarobot-oss/cli/.github/actions/<name>@main`), not the calling ref. This is
intentional — but it means **a PR that modifies a composite action's `action.yaml`
cannot exercise that change through its own CI run**: the workflow always pulls the
action's code from `main`, so the fix only takes effect after merge.

When reviewing a PR that touches `.github/actions/*/action.yaml`, flag it as an
**informational callout**: CI on this PR does not validate the new behavior. Confirm
the author has a plan to verify it — e.g., temporarily pointing the ref at the PR
branch to get a real CI run (then reverting before merge), or verifying immediately
after merge via a manual `workflow_dispatch` run of a workflow that exercises that
action (see `.github/workflows/README.md` for which workflows call which suites).
