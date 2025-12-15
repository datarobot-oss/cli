# RATIONALE
<!-- Pull Request Guidelines: https://goo.gl/cnhT21 -->

<!--
For expedient and efficient review, please explain *why* you are
making this change. It is not always obvious from the code and
the review may happen while you are asleep / otherwise not able to respond quickly.
-->

## CHANGES

## PR Automation

**Comment-Commands:** Trigger CI by commenting on the PR:
- `/trigger-smoke-test` or `/trigger-test-smoke` - Run smoke tests
- `/trigger-install-test` or `/trigger-test-install` - Run installation tests

**Labels:** Apply labels to trigger workflows:
- `run-smoke-tests` or `go` - Run smoke tests on demand (only works for non-forked PRs)

> [!IMPORTANT]
> **For Forked PRs:** If you're an external contributor, the `run-smoke-tests` label won't work. Only maintainers can trigger smoke tests on forked PRs by applying the `approved-for-smoke-tests` label after security review. Please comment requesting maintainer review if you need smoke tests to run.

See [workflows README](.github/workflows/README.md) for details.

<!-- Recommended Additional Sections:
## SCREENSHOTS
## TODO
## NOTES
## TESTING
## RELATED
## REVIEWERS -->
