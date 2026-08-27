# Editing user-owned files

Most files the CLI writes are its own: `drconfig.yaml`, the state directory under
`.datarobot/`, cached artifacts. Two are not. `.datarobot.yaml` and `.env` are authored by
a human, committed, and reviewed &mdash; and the CLI still has to edit them in place.

That makes every such write a round trip through someone else's file. This page collects
the rules that fall out of it. The machine-readable form lives in
[.cursor/bugbot-user-files.md](../../.cursor/bugbot-user-files.md).

## The invariant

> Give back everything you moved. Take nothing that was not yours.

Both halves matter, and they are enforced in different places.

## Re-emit, never regenerate

Edits parse to a node tree, mutate it, and re-emit. They do not unmarshal into a struct
and marshal it back, which would silently drop comments, unknown keys, and key order.

`WriteWorkloadID` in `internal/workload/manifest/write.go` is the reference: it reads,
parses to `yaml.Node`, runs the alias guard, mutates one key, and encodes the same tree.
Its doc comment names it "the one edit the CLI makes to a file the user owns."

## Annotation the CLI wrote dies with the key it wrote

When the CLI adds `workloadId` it also adds a `# managed by the CLI` marker. That marker
is attached as a **line comment on the key**, so removing the key removes the marker. No
cleanup step has to remember it. The same holds for every generated key: `mapping()`
attaches field comments as `LineComment`, never as free-floating text.

Contrast with what the same function does to comments it did *not* write. Inserting
`workloadId` at the top of the mapping steals the old first key's head comment, because a
comment at the top of a file is a banner about the file rather than about whichever key
happens to be first.

That steal creates a debt. Any code that removes the binding has to hand the banner back
to whatever key becomes first again. Review insert and remove as one pair: an asymmetry
between them is not a cosmetic bug, it is data loss in a file the user committed.

The `.env` builder meets the same invariant with a different mechanism, because it works on
text rather than a node tree. `mergedDotenvChunks` in `internal/envbuilder/dotenv.go`
splits the file into prompt-backed variables, user-provided variables, and everything
else; CLI help comments are discarded on read and regenerated from `UserPrompt.HelpLines()`
on write, while user comments are carried through untouched. Same invariant, different
implementation. Do not try to unify them.

## Refuse rather than damage

Some files cannot be edited and put back soundly. Refuse them, each with the remedy that
fits it:

| Shape | Why it cannot be edited |
| --- | --- |
| More than one YAML document | Re-emitting moves comments between documents |
| Repeated keys that disagree | Removing one silently promotes the other |
| A value carrying an anchor the file aliases | Removing it strands the alias and the file never loads again |
| The edit would leave no recognized key | The result is not a manifest, and the file still exists, so nothing falls back to setup |

Do not append one generic remedy to all of them. "Remove that line" is correct advice for
one of these and walks the user into a broken file for another.

## Line endings

YAML and JSON encoders emit LF. A CRLF file re-emitted unchanged lands in review as a
whole-file diff, so a rewrite must restore the terminators the file came with. Convert
back only a file that is *wholly* CRLF &mdash; guessing from one stray terminator is how a
single CRLF line, or a CRLF inside a block scalar where it is content rather than a line
ending, converts everything around it.

## Compare through aliases, and match inside the edit

Two rules that only look like details until they bite:

- A value written as a YAML alias is resolved by the reader, so it is the value the project
  actually deploys to. Comparisons must resolve too. Comparing the raw node reads the
  anchor name, matches nothing, and leaves the file as stale as it found it while
  reporting success.
- An edit conditioned on an id must check that id inside the same parse that performs the
  edit. Checking against a separate read leaves a window for a concurrent `dr workload up`
  to write a fresh binding that the edit then removes without ever having looked at it.

## Errors that are classifications, not messages

Not every YAML validation error is user-facing, and the two must not be aligned.

`PromptFileSchema.Validate` in `internal/envbuilder/schema.go` returns errors such as
`document is empty` and `root must be a mapping (sections)`. Its only consumer is
`isPromptFile`, a boolean: the errors are discarded, and non-conforming files are skipped
silently so copier answer files and version manifests under `.datarobot/` do not raise
anything. Those strings are a classification signal, not advice.

The manifest's superficially identical refusal &mdash; `root must be a YAML mapping` in
`WriteWorkloadID` &mdash; *is* printed, so it carries an explanation and a remedy, and the read
and write paths that can both reject a file state should answer from one shared sentinel
rather than two phrasings.

Adding a remedy to the prompt-file strings would put user guidance into a string nothing
prints, about a file the CLI deliberately ignores. Before aligning two error messages that
read alike, check who consumes each one.
