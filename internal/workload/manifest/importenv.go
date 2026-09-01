// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manifest

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// mergeKey is YAML's inheritance operator. A container carrying one may be
// serving variables that are written somewhere else entirely, which is a
// question this package's walks cannot answer.
const mergeKey = "<<"

// ErrNoPrimaryContainer reports a manifest with no container to carry an
// environment variable. A file that names an artifact by id has no spec of its
// own: the variables live on the artifact the id points at, and adding them
// here would write a key the deploy never reads.
var ErrNoPrimaryContainer = errors.New(
	"this manifest has no container spec of its own, because it names an artifact by id. " +
		"Set the variables on that artifact, or replace artifactId with an artifact block")

// ErrSharedEnvVars reports environmentVars this edit cannot touch without
// touching something else: an alias into a block other containers read, or a
// container inheriting keys through a merge.
//
// Appending to a shared node would add the variable to every container reading
// it, and the diff would show the change nowhere near the container that was
// supposedly edited. Refusing a file this edit cannot put back soundly is the
// rule the whole in-place path is built on.
var ErrSharedEnvVars = errors.New(
	"the primary container's environment is shared with other containers through a YAML anchor or merge key, " +
		"so editing it here would change it for all of them. Edit the block that should carry the change " +
		"by hand instead")

// ErrUnreadableEnvVars reports an environmentVars key holding something other
// than a list. Overwriting it would throw away whatever the user meant by it.
var ErrUnreadableEnvVars = errors.New(
	"the primary container's environmentVars is not a list, so this edit cannot read it. " +
		"Make it a list of name/value entries, or edit it by hand")

// EnvVarNames is the set of variable names the primary container already
// declares, which is what decides whether an entry in .env is new.
//
// Names only. The values in a manifest are a mix of literals and credential
// references, and nothing that compares the file against .env has any business
// reading either.
//
// A manifest this edit could not write to declares nothing as far as this
// answer goes, and that is not the same as declaring nothing. A container
// inheriting its variables through a merge key really is serving them, and
// this walk cannot see them. Callers must ask CanDeclareEnvVars first and say
// something else entirely when it refuses, rather than reading an empty set as
// "everything in .env is new".
func (m *Manifest) EnvVarNames() map[string]bool {
	container, err := editableContainer(m.root)
	if err != nil {
		return map[string]bool{}
	}

	return declaredEnvNodeNames(mapValue(container, keyEnvironmentVars))
}

// PendingEnvNames is the variables whose credential reference was never
// finished: the entry still names CredentialPlaceholder, so a deploy refuses
// it.
//
// The name being declared is what makes an import skip it, so without this a
// secret whose store call failed looks settled for ever after. It reads the
// same references the deploy checks, through the same collector, so the two
// cannot disagree about what counts as unfinished.
func (m *Manifest) PendingEnvNames() []string {
	refs, _ := collectCredentialRefs(m.root)

	var pending []string

	for _, ref := range refs {
		if ref.CredentialID == CredentialPlaceholder {
			pending = append(pending, ref.EnvName)
		}
	}

	return pending
}

// CanDeclareEnvVars reports whether the manifest has somewhere to put a
// variable, and why not when it does not.
//
// It exists so a caller can find that out before doing anything it cannot take
// back. The import mints a credential per secret, and a credential is created
// on the tenant for good: discovering only at the write that the file was
// never going to accept it leaves secrets behind that nothing references and
// that make every retry collide with the name they already took.
func (m *Manifest) CanDeclareEnvVars() error {
	container, err := editableContainer(m.root)
	if err != nil {
		return err
	}

	return envVarsBlocker(container)
}

// ImportEnvVars adds the variables the primary container does not already
// declare, and reports the names it added in the order it added them.
//
// Additive and one-directional, which is the whole contract. A name the file
// already carries keeps whatever it was given, because the manifest is the
// deployed truth and .env is a local copy allowed to drift; a name the file
// carries and .env no longer does is left alone, because deleting a line from
// a developer's own file says nothing about what the workload should run.
//
// preview renders without writing. The returned content is only meaningful
// when something was added, and it differs from the real edit in one place:
// a dry run stores no secret, so a secret's entry previews as the placeholder
// the real run replaces with a credential id. That is the right way round, a
// preview that minted credentials would not be one, but it means the preview
// understates how finished the result is.
//
// The bytes are parsed and validated before they reach the disk. The file
// belongs to the user, this is the only command that edits one in place, and a
// manifest the next deploy would refuse must not be left behind under a
// success message.
func ImportEnvVars(path string, vars []EnvVar, preview bool) ([]EnvVar, []byte, error) {
	var added []EnvVar

	changed, rendered, err := renderEdit(path, "import environment variables", false,
		func(root *yaml.Node) (bool, error) {
			container, err := editableContainer(root)
			if err != nil {
				return false, err
			}

			entries, err := envVarsTarget(container)
			if err != nil {
				return false, err
			}

			added = appendEnvVars(entries, vars)

			return len(added) > 0, nil
		})
	if err != nil || !changed {
		return nil, nil, err
	}

	if err := checkImported(rendered, path); err != nil {
		return nil, nil, err
	}

	if !preview {
		if err := atomicWrite(path, rendered); err != nil {
			return nil, nil, err
		}
	}

	return added, rendered, nil
}

// checkImported re-reads what the edit produced. Nothing downstream of here
// looks at the file again before a deploy does, so a rendering that lost the
// entries or reshaped the tree would otherwise reach the user as "✓ Updated".
func checkImported(rendered []byte, path string) error {
	parsed, err := Parse(rendered, filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("the edited %s could not be read back, so it was not written: %w", FileName, err)
	}

	if err := parsed.Validate(); err != nil {
		return fmt.Errorf("the edited %s did not validate, so it was not written: %w", FileName, err)
	}

	return nil
}

// envVarsTarget is the sequence an import may append to, creating it when the
// container has none and filling in a key left holding nothing.
//
// Everything it refuses is a shape this edit cannot add to and leave the rest
// of the file meaning what it did. envVarsBlocker answers the same question
// without touching anything, so a caller can find out before it acts.
func envVarsTarget(container *yaml.Node) (*yaml.Node, error) {
	if err := envVarsBlocker(container); err != nil {
		return nil, err
	}

	raw, at := rawValue(container, keyEnvironmentVars)

	if raw != nil && raw.Kind == yaml.SequenceNode {
		return raw, nil
	}

	entries := sequence()

	if raw == nil {
		// The new key goes after whatever the container already had, comment
		// included: a comment trailing the block stays where it was written,
		// above the addition. Carrying it past the insertion instead was
		// tried and is worse, because yaml.v3 then re-emits it outside the
		// container at the indentation of a different block entirely.
		container.Content = append(container.Content, scalar(keyEnvironmentVars), entries)

		return entries, nil
	}

	// `environmentVars:` with nothing under it is the list emptied by hand,
	// not a shape to refuse. It becomes the empty list it already reads as,
	// keeping whatever the user wrote around it.
	entries.HeadComment, entries.LineComment, entries.FootComment = raw.HeadComment, raw.LineComment, raw.FootComment
	container.Content[at] = entries

	return entries, nil
}

// envVarsBlocker is why an import cannot append to this container, or nil when
// it can. It reads and changes nothing, so the guard a caller runs before
// minting a credential is the same walk the write performs.
func envVarsBlocker(container *yaml.Node) error {
	if err := checkUnshared(container); err != nil {
		return err
	}

	raw, _ := rawValue(container, keyEnvironmentVars)

	switch {
	case raw == nil:
		return nil

	// Sharing is tested first and on the raw node. isNull resolves aliases, so
	// asking "is it empty" before "is it shared" answers yes for an alias to an
	// empty block and skips every refusal below.
	case raw.Kind == yaml.AliasNode, raw.Anchor != "":
		return ErrSharedEnvVars

	case raw.Kind == yaml.MappingNode && hasMergeKey(raw):
		// Inheritance, not a mistyped list, so it earns the refusal that
		// describes what it is.
		return ErrSharedEnvVars

	case raw.Kind == yaml.SequenceNode, isNull(raw):
		return nil

	default:
		return ErrUnreadableEnvVars
	}
}

// editableContainer walks a parsed manifest to the container traffic reaches,
// refusing every step that is shared with something else.
//
// The walk has to be its own rather than the reader's, because the reader
// resolves aliases on the way: it answers "what does this file mean", and this
// answers "what may this edit write into". A node reached through an alias, or
// carrying an anchor, or standing in a mapping that merges another, is a node
// whose edit would land somewhere the user is not looking.
func editableContainer(root *yaml.Node) (*yaml.Node, error) {
	groups, err := unsharedPath(root, keyArtifact, keySpec, keyContainerGroups)
	if err != nil || groups == nil {
		return nil, orNoContainer(err)
	}

	var first *yaml.Node

	for _, group := range groups.Content {
		primary, fallback, err := groupContainers(group)
		if err != nil {
			return nil, err
		}

		if primary != nil {
			return primary, nil
		}

		if first == nil {
			first = fallback
		}
	}

	if first == nil {
		return nil, ErrNoPrimaryContainer
	}

	// No container claims to be primary, so the first one is, matching how the
	// reader and the live-document walk both answer it.
	return first, nil
}

// groupContainers is the group's primary container when it names one, and the
// first container it lists either way.
func groupContainers(group *yaml.Node) (primary, first *yaml.Node, err error) {
	containers, err := unsharedPath(group, keyContainers)
	if err != nil {
		return nil, nil, err
	}

	// A group with no containers is a group this walk has nothing to say
	// about, not a reason to stop: another group may carry the primary.
	if containers == nil {
		return nil, nil, nil
	}

	for _, container := range containers.Content {
		if err := checkUnshared(container); err != nil {
			return nil, nil, err
		}

		if first == nil {
			first = container
		}

		if isTrue(mapValue(container, keyPrimary)) {
			return container, first, nil
		}
	}

	return nil, first, nil
}

// unsharedPath follows keys from node, refusing the first step that is shared.
// A missing key stops the walk with no error: the caller decides what its
// absence means.
func unsharedPath(node *yaml.Node, keys ...string) (*yaml.Node, error) {
	at := node

	for _, key := range keys {
		if err := checkUnshared(at); err != nil {
			return nil, err
		}

		next, err := unsharedChild(at, key)
		if err != nil || next == nil {
			return nil, err
		}

		at = next
	}

	return at, nil
}

// unsharedChild is the value at key, refusing a node this edit cannot own. A
// missing key is not an error here: the caller decides what its absence means.
func unsharedChild(node *yaml.Node, key string) (*yaml.Node, error) {
	raw, _ := rawValue(node, key)
	if raw == nil {
		return nil, nil
	}

	if err := checkUnshared(raw); err != nil {
		return nil, err
	}

	return raw, nil
}

// checkUnshared refuses a node the rest of the file may be reading through
// another name: an alias into it, an anchor on it, or a merge that folds
// another mapping into it.
func checkUnshared(node *yaml.Node) error {
	if node == nil {
		return nil
	}

	if node.Kind == yaml.AliasNode || node.Anchor != "" || hasMergeKey(node) {
		return ErrSharedEnvVars
	}

	return nil
}

// hasMergeKey reports whether a mapping folds another one into itself, which
// makes every key it appears to lack a question this walk cannot answer.
func hasMergeKey(node *yaml.Node) bool {
	raw, _ := rawValue(node, mergeKey)

	return raw != nil
}

// orNoContainer turns "the walk found nothing" into the refusal that names the
// one shape it means: a manifest binding an artifact by id.
func orNoContainer(err error) error {
	if err != nil {
		return err
	}

	return ErrNoPrimaryContainer
}

// rawValue is mapValue without the alias resolution, plus where the value sits
// in the mapping. Both matter here: an import has to be able to tell an alias
// from the block it points at, and to replace a value in place.
func rawValue(node *yaml.Node, key string) (*yaml.Node, int) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, -1
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], i + 1
		}
	}

	return nil, -1
}

// appendEnvVars adds each variable the list does not already declare, and
// reports what it added.
//
// A secret is written as a reference to the credential holding it, or to
// CredentialPlaceholder when none was stored, matching what the renderer emits
// for a fresh manifest. The two must agree: an entry added here and an entry
// written at setup describe the same variable, and a reader should not be able
// to tell which command produced it.
func appendEnvVars(entries *yaml.Node, vars []EnvVar) []EnvVar {
	declared := declaredEnvNodeNames(entries)
	added := make([]EnvVar, 0, len(vars))

	for _, v := range vars {
		if declared[v.Name] {
			continue
		}

		// Guards a .env that repeats a name: the first entry wins, and the
		// second must not be appended as a duplicate the manifest reader
		// would silently ignore.
		declared[v.Name] = true

		entries.Content = append(entries.Content, envVarNode(v))
		added = append(added, v)
	}

	return added
}

// declaredEnvNodeNames is the set of names an environmentVars sequence
// carries. It is declaredEnvNames over a node tree rather than over the
// decoded maps a live document arrives as.
func declaredEnvNodeNames(entries *yaml.Node) map[string]bool {
	items := seqItems(entries)
	declared := make(map[string]bool, len(items))

	for _, entry := range items {
		if name, ok := scalarString(mapValue(entry, keyName)); ok {
			declared[name] = true
		}
	}

	return declared
}

// DeclaredEnvVar is one entry as the manifest carries it, which is either a
// literal the file spells out or a reference to a credential holding the value.
type DeclaredEnvVar struct {
	Name string
	// Value is the literal the file carries, empty for a credential reference.
	Value string
	// CredentialID is set when the entry points at the credential store rather
	// than carrying the value. The value behind it is not readable: the
	// platform never hands a secret back, so nothing can compare it to .env.
	CredentialID string
	// CredentialKey is which field of that credential feeds the variable. A
	// reference may name any of them, and only the one this CLI writes is one
	// it may safely rewrite.
	CredentialKey string
	// Structured is set when the value is neither a literal nor a reference
	// but a mapping or a list somebody wrote by hand. The update path leaves
	// those alone, so anything comparing this against .env has to leave them
	// alone too: reporting one as drift would name a flag that then does
	// nothing, every run, for ever.
	Structured bool
}

// CredentialKeyWritten is the credential field the wizard stores a .env secret
// under, and so the only one an update may re-send to.
const CredentialKeyWritten = credentialKey

// Secret reports that the entry defers to the credential store.
func (d DeclaredEnvVar) Secret() bool {
	return d.CredentialID != ""
}

// DeclaredEnvVars is what the primary container says about each variable it
// carries, in file order.
//
// It exists so a caller can compare the file against .env. Only the literals
// can actually be compared: a secret is a reference, and the value behind it
// is write-only as far as this CLI is concerned, so a rotated secret is
// invisible and can only be re-sent, never detected.
func (m *Manifest) DeclaredEnvVars() []DeclaredEnvVar {
	container, err := editableContainer(m.root)
	if err != nil {
		return nil
	}

	entries := seqItems(mapValue(container, keyEnvironmentVars))
	declared := make([]DeclaredEnvVar, 0, len(entries))

	for _, entry := range entries {
		name, ok := scalarString(mapValue(entry, keyName))
		if !ok {
			continue
		}

		id, key := credentialRefOf(entry)

		declared = append(declared, DeclaredEnvVar{
			Name:          name,
			Value:         literalOf(entry, id),
			CredentialID:  id,
			CredentialKey: key,
			Structured:    structuredValue(entry, id),
		})
	}

	return declared
}

// structuredValue reports an entry whose value is a mapping or a list rather
// than the string an environment variable can be. Nothing here can compare one
// with a .env line, and rewriting it would throw away whatever the user meant
// by it, so the same entries are skipped by the update and by the comparison.
func structuredValue(entry *yaml.Node, credentialID string) bool {
	if credentialID != "" {
		return false
	}

	value := mapValue(entry, keyValue)

	return value != nil && value.Kind != yaml.ScalarNode
}

// literalOf is the value an entry spells out, empty when it references a
// credential in either the shorthand or the object form.
func literalOf(entry *yaml.Node, credentialID string) string {
	if credentialID != "" {
		return ""
	}

	value, _ := scalarString(mapValue(entry, keyValue))

	return value
}

// credentialRefOf is the credential an entry points at and the field of it the
// variable reads, both empty for a literal. It accepts the object form and the
// shorthand alike, the way the compiler's own collector does.
func credentialRefOf(entry *yaml.Node) (id, key string) {
	if id, _ := scalarString(mapValue(entry, keyDRCredentialID)); id != "" {
		key, _ := scalarString(mapValue(entry, keyKey))

		return id, key
	}

	value, _ := scalarString(mapValue(entry, keyValue))
	if !strings.HasPrefix(value, CredentialShorthandPrefix) {
		return "", ""
	}

	id, key, ok := strings.Cut(strings.TrimPrefix(value, CredentialShorthandPrefix), "/")
	if !ok || id == "" || key == "" {
		return "", ""
	}

	return id, key
}

// UpdateEnvVars rewrites the literal values of variables the manifest already
// declares, and reports the ones it changed.
//
// The inverse of ImportEnvVars and deliberately a separate act: that one adds
// names and never touches a value, this one touches values and never adds a
// name. A run that wanted both asked for both.
//
// Only literals. A secret's entry is a reference, and rotating it means
// sending a new value to the credential the reference names, which happens in
// the store rather than in this file: the id does not change, so there is
// nothing here to rewrite.
func UpdateEnvVars(path string, vars []EnvVar, preview bool) ([]EnvVar, []byte, error) {
	var changedVars []EnvVar

	changed, rendered, err := renderEdit(path, "update environment variables", false,
		func(root *yaml.Node) (bool, error) {
			container, err := editableContainer(root)
			if err != nil {
				return false, err
			}

			if err := envVarsBlocker(container); err != nil {
				return false, err
			}

			entries, _ := rawValue(container, keyEnvironmentVars)

			changedVars, err = rewriteLiterals(seqItems(entries), vars)
			if err != nil {
				return false, err
			}

			return len(changedVars) > 0, nil
		})
	if err != nil || !changed {
		return nil, nil, err
	}

	if err := checkImported(rendered, path); err != nil {
		return nil, nil, err
	}

	if !preview {
		if err := atomicWrite(path, rendered); err != nil {
			return nil, nil, err
		}
	}

	return changedVars, rendered, nil
}

// rewriteLiterals sets each entry's value to what .env now says, skipping the
// ones that already agree and the ones that defer to a credential, and
// refusing any entry the rest of the file may be reading through another name.
//
// The scalar is edited rather than replaced, so an anchor, a comment and the
// entry's place in the file all survive an edit that only ever meant to change
// what one value is.
func rewriteLiterals(entries []*yaml.Node, vars []EnvVar) ([]EnvVar, error) {
	wanted := make(map[string]EnvVar, len(vars))

	for _, v := range vars {
		if !v.Secret {
			wanted[v.Name] = v
		}
	}

	changed := make([]EnvVar, 0, len(wanted))

	for _, entry := range entries {
		want, value, err := rewritable(entry, wanted)
		if err != nil {
			return nil, err
		}

		if value == nil {
			continue
		}

		// Styled the way a fresh write would style it, so a value that needs
		// quoting to survive another parser gets it.
		value.Value = want.Value
		value.Style = scalar(want.Value).Style
		value.Tag = "!!str"

		changed = append(changed, want)
	}

	return changed, nil
}

// rewritable is the scalar an entry's value may be written into, or nil when
// this entry is not one the update touches. It refuses rather than skips when
// the entry is shared, because rewriting a node another container reads would
// change a value in a place the diff never shows.
func rewritable(entry *yaml.Node, wanted map[string]EnvVar) (EnvVar, *yaml.Node, error) {
	name, ok := scalarString(mapValue(entry, keyName))
	if !ok {
		return EnvVar{}, nil, nil
	}

	want, asked := wanted[name]
	if id, _ := credentialRefOf(entry); !asked || id != "" {
		return EnvVar{}, nil, nil
	}

	// What the entry means is read through the walk that resolves aliases,
	// which is the walk the comparison against .env uses. Asking the raw node
	// instead would compare an anchor's name with a value and read every
	// borrowed value as already in agreement, so an entry the notice had just
	// called drift would go by without a word.
	current, isScalar := scalarString(mapValue(entry, keyValue))
	if !isScalar || current == want.Value {
		return EnvVar{}, nil, nil
	}

	if err := checkUnshared(entry); err != nil {
		return EnvVar{}, nil, err
	}

	// Written through the walk that does not resolve, because a borrowed
	// scalar is the one thing this must not write into: an alias, or an
	// anchor others read through, carries the edit to a place the diff never
	// shows. Refused rather than skipped for the same reason the entry above
	// is, and because the value differs, so a skip would be silence about
	// drift the run was asked to settle.
	value, _ := rawValue(entry, keyValue)
	if value == nil || value.Kind != yaml.ScalarNode || value.Anchor != "" {
		return EnvVar{}, nil, ErrSharedEnvVars
	}

	return want, value, nil
}
