# Slack release announcement style — #dr-cli

The shape below is distilled from the release announcements actually posted in #dr-cli. Author
voice: first person, direct, no marketing tone.

## Structure of the main post

- Opener states the version is out and leads with the **single biggest user-facing change** in
  *bold*. A user-supplied quip may open instead; the bold biggest-change sentence follows
  immediately either way. The opener may also lead with the command a user types, then the
  bold behavior change.
- One or two sentences explaining the problem the change solves, so the value is obvious
  without reading the release notes.
- A short `Example` code block (1-2 commands with inline comments) when the release adds a
  command or flag. A bugfix-only patch can get one too when the fix changes an outcome you
  can show (the fixed behavior demonstrated in one line); "nothing to demo" means no new
  capability, not no new commit.
- Short "Notes:" bullets for behavior changes, security caveats, and compat notes.
- Ends with `Release: <link to the GitHub release>` — the link goes last.
- Keep it 5-12 lines. Long-form detail lives on the GitHub release; deep walkthroughs go in the
  thread.

## Thread patterns

- Breaking changes can go in a **thread reply** instead of the main post: end the main post with
  "Breaking-change note in :thread:", then post the :warning: details as the first thread reply.
  When posting programmatically, pass the thread timestamp explicitly or the reply lands as a
  standalone channel message.
- Sibling commands, operational notes, and bugfixes can go in 2-3 thread replies.
- Reserve thread replies for changes that break existing invocations outright. A script-facing
  change documented as compatible (stdout and exit codes unchanged) stays as a one-line
  "One script note:" in the main post.
- A bugfix-only patch posted **late** (days after the previous announcement) can be announced as
  a simplified thread reply under the previous release's main post: one short paragraph leading
  with the biggest fix in *bold*, no example block, Release link last. Same-day patches get a
  standalone main post.

## Feature gates

- Default to **not** writing "Nothing is behind a feature gate this release." when nothing is
  gated — the line has been dropped from the final post twice (kept twice in an earlier run).
  Don't re-add it after draft edits, and don't pad the gate note with reassurance.
- When exactly one command is gated, a single "One gate note: `dr enclave` still needs
  `DATAROBOT_CLI_FEATURE_ENCLAVE=true`." sentence replaces the line.
- When a release **lifts** a gate, call out that behavior shipped earlier but opt-in now reaches
  every user. It belongs in the release notes and as a :warning: thread reply even though the
  commits predate the stable-to-stable range.
- Verify gate claims empirically before writing either phrasing: run `go run . --help` with and
  without the env var. `features.SetGate` is an annotation, not a runtime call, so grepping for
  the env var finds nothing — the `CommandAdder` filtering hides the command from `--help`,
  completion and dispatch.

## Attribution: `## Brought to you by`

- The release notes section lists the humans who wrote the release. Build it from **two**
  sources, deduped:
  1. Commit **authors** — the compare API:
     `gh api repos/datarobot-oss/cli/compare/<old>...<new> --jq '[.commits[].author.login] | map(select(. != null)) | unique | .[]'`
  2. **Co-authors** — the API does not see `Co-authored-by:` trailers. Check them explicitly:
     `git log <old>..<new> --format='%(trailers:only=true,valueonly=false,separator=%x3B)'` and
     pull every `Co-authored-by:` line, mapping name+email to a GH login. A maintainer has
     explicitly asked that co-authors be credited; an author-only list is wrong by omission.
- Exclude bots and coding agents in both lists: `dependabot[bot]`, `github-actions[bot]`,
  `datarobot-devinfragit7`, `factory-droid[bot]`. Commits co-authored by an agent still carry
  the human's login, so the filter usually leaves the human untouched.
- Thank people in the announcement by the same names. No self-thanks. A "cc: @<first-name>" on
  the line touching a plugin owner's plugin is the pattern for flagging owners; resolve cc
  targets by asking, not by guessing handles.

## Mentions and rendering (Factory Connector)

- A text-form `<@U…>` in `post_message` text renders as literal text — it never pings. Real
  pings need a rich_text block edit: post with `parse: none`, then `slack__update_message`
  replacing the literal text with a `{"type": "user", "user_id": "U…"}` element. Using plain
  names instead of mentions sidesteps the limitation entirely and is the default.
- Angle-bracket placeholders (`<group-id>`) render correctly inside code spans and preformatted
  blocks; only the plain-text fallback shows `&lt;…&gt;`.
- `*bold*`, `` `code` ``, fenced blocks, and bare links all render correctly as plain mrkdwn text.

## Draft-file coordination

- Compose the announcement draft (and the release-notes draft) as files in the repo's `tmp/`
  (`tmp/announcement-vX.Y.Z.md`, `tmp/release-notes-vX.Y.Z.md`). They are the coordination
  surface: aj may edit the draft before posting.
- **Re-read the draft file immediately before posting.** Sync a meaningful aj edit across
  surfaces when it touches content that also appears in the GitHub release body (a trimmed
  gate note); leave pure-opener edits alone.
- Publish release notes **after** `promote-release` succeeds, not between publish and
  promotion (see `pipeline.md` for the race).

## Tone

- First person, direct, no marketing tone. Moderate snark is welcome.
- Thank the person whose work shipped, tied to the actual commits — by name by default; see
  Mentions and rendering for when a real @-mention is wanted.
- Post the day the release ships, and **only after** `promote-release` succeeds.

## Verification checklist before posting

- Verify every flag and command example against the release notes or docs.
- Cross-check every thanked person against the **author and co-author list** (see Attribution
  above). Do not invent a thanks.
- Proofread the bolded headline hardest — it is the one line everyone reads.
- Watch for wording drift between the announcement and the release notes.
- When drafting markdown to paste, avoid nesting triple backticks; paste code blocks separately.
