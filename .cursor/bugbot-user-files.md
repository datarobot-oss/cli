# Editing Files the User Owns

Applies to code that writes back a file a human authored and keeps under version
control — `.datarobot.yaml`, `.env`, and any project file the CLI edits in place
rather than regenerating.

## Re-emit, Do Not Regenerate

In-place edits must re-emit the parsed node tree, never marshal a struct. Regenerating
drops comments, unknown keys, and key order the user relies on.

## CLI Annotation Dies With the CLI Key

A marker the CLI wrote must be removed with the key it annotates, and user annotation
must survive. Attach markers so they cannot outlive their key, and regenerate CLI help
text from its source rather than parsing it back out of the file.

## Removal Must Mirror Insertion

Every node an insert moved, stole, or attached must be restored by the matching removal.
Review the insert and the delete as one pair — an asymmetry is silent data loss in the
user's file.

## Preserve Line Endings

A rewrite must give the file back the line endings it came with. Encoders that emit LF
turn a one-key edit on a CRLF file into a whole-file diff.

## Refuse Rather Than Damage

A file the edit cannot put back soundly must be refused, with the remedy that fits that
refusal: multi-document streams, repeated keys that disagree, an anchor the rest of the
file aliases, or an edit that would leave the file unrecognizable. Do not append one
generic remedy to every refusal.

## Compare Through Aliases

A value reachable through a YAML alias must be compared resolved, not as the raw node.
The reader resolves it, so a raw comparison matches nothing and leaves the file unchanged
while reporting success.

## Match Only What You Wrote

An edit conditioned on an id must verify that id inside the same parse that performs the
edit. A separate read-then-write leaves a window for a concurrent command to rebind the
file between the check and the write.
