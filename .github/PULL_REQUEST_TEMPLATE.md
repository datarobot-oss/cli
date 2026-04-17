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
> **For Forked PRs:** The `run-smoke-tests` label won't work. A required **Smoke Tests** check will block merge until a maintainer acts:
> - A maintainer uses `/approve-smoke-tests` to run smoke tests (results will set the check)
> - A maintainer uses `/skip-smoke-tests` to bypass the check without running tests
>
> Please comment requesting a maintainer review if you need smoke tests to run.


<!-- Recommended Additional Sections:
## SCREENSHOTS
## TODO
## NOTES
## TESTING
## RELATED
## REVIEWERS -->
