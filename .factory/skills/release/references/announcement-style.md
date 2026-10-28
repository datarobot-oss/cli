# Slack release announcement style — #dr-cli

The shape below is distilled from the release announcements actually posted in #dr-cli. Author
voice: first person, direct, no marketing tone.

## Structure of the main post

- Opener states the version is out and leads with the **single biggest user-facing change** in
  *bold*. A user-supplied quip may open instead; the bold biggest-change sentence follows
  immediately either way.
- One or two sentences explaining the problem the change solves, so the value is obvious
  without reading the release notes.
- A short `Example` code block (1-2 commands with inline comments) when the release adds a
  command or flag.
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

## Feature gates

- Ungated release: "Nothing is behind a feature gate this release."
- When a release **lifts** a gate, call out that behavior shipped earlier but opt-in now reaches
  every user. It belongs in the release notes and as a :warning: thread reply even though the
  commits predate the stable-to-stable range.

## Tone

- First person, direct, no marketing tone. Moderate snark is welcome.
- @-mention the person to thank, tied to the actual work they did.
- Post the day the release ships, and **only after** `promote-release` succeeds.

## Verification checklist before posting

- Verify every flag and command example against the release notes or docs.
- Cross-check every thanked person against the commit-author list (the `## Brought to you by`
  output). Do not invent a thanks.
- Proofread the bolded headline hardest — it is the one line everyone reads.
- Watch for wording drift between the announcement and the release notes.
- When drafting markdown to paste, avoid nesting triple backticks; paste code blocks separately.
