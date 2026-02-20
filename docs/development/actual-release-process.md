# Release Process for the DR CLI

task build, task lint, task test
if you can, on the last PR before you're ready to release, run smoke tests. add the label approved-for-smoke-tests
otherwise, check the daily smoke tests
create and push a tag of the form vX.Y.Z-beta.A from the PR BEFORE merging
see if the release goes through (#cli-github  will have notification)
if you want, coordinate with Buzok to run their e2e tests
otherwise, if the release is successful, for manual QA you can try dr self update locally, or install it whichever way you prefer, and run things. do your validation here if it's not already covered in CI
merge the PR
create and push a tag of the form vX.Y.Z, see if the release goes through
communicate out. Yuriy and I usually link to the GH release page and copy paste the changelog to #dr-cli and agentic-flow-dev
