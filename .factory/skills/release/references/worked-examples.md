# Worked examples

Anonymized end-to-end runs of this skill. Names, user IDs, and internal incident details are
left out on purpose; the mechanics are what transfer.

## 1. Patch-only range — no argument needed

Range since last stable: CI workflow bumps, a dependency bump with no behavior change, docs
rewrites, an internal refactor. Every commit classifies PATCH:

| Change | Class |
| --- | --- |
| CI: bump upload-artifact action | PATCH |
| deps: Go patch bump, no behavior change | PATCH |
| docs: rewrite workload guide | PATCH |

Proposed patch bump. Readiness verdict: release. Lint and tests green, tag pushed, workflow
promoted it stable on schedule. Announcement led with the most user-visible fix, not with the
dependency bumps — "no user-visible headline" is announced plainly as a quiet release.

## 2. Breaking change below 1.0 — MINOR, not v1.0.0

A range removes a flag and changes a default output shape. Any invocation relying on the old
flag now fails, so the release is breaking. Below `v1.0.0`, the convention: MINOR bump
(`v0.2.86` → `v0.3.0` shape), not `v1.0.0`. Migration notes went into the release notes and
the announcement carried a :warning: breaking-change thread reply.

## 3. A release pulled back to pre-release

A stable release was published, then **returned to pre-release the same evening** after a
maintainer objected that a rename broke downstream pipelines. The follow-up:

- Restored the old behavior as a deprecated alias in a patch.
- Shipped the patch as the **next patch number**, not the minor the fix arguably deserved:
  the pulled-back number was already published and had burned itself on Homebrew. Version
  continuity beat semver purity — say so in the evidence table when it happens.

Lesson encoded in the skill: a pulled-back release still burns its number; never reuse or skip
to "fix" it.

## 4. Minor release with a new command — full run

A range adds three new public commands plus filters on an existing one:

| Change | Class |
| --- | --- |
| New `dr workload diagnose` command | MINOR |
| New `dr workload settings` command | MINOR |
| New `--grep`/`--exclude` log filters | MINOR |
| Bugfixes in artifact sync | PATCH |

Version proposed and approved: minor. After promotion:

- Release notes rewritten into readable sections: Highlights (one bullet per command),
  Feature gates ("Nothing is behind a feature gate this release."), operational notes
  (JSON envelope, exit codes), full stable-to-stable range.
- `## Brought to you by` built from the compare API, bots and coding agents excluded:
  the human authors who wrote the commits, thanked in the announcement by the same names.
- Announcement: 5-line main post, bold headline ("a workload can now tell you *why it's
  broken*"), one example code block, Release link last; sibling commands and bugfixes in two
  thread replies; one human thanked with an @-mention tied to the actual commits.

The one shipped defect worth copying: the bolded headline was proofread last and shipped with
a missing apostrophe. Proofread the headline first.
