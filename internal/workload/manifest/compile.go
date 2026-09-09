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
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// CredentialShorthandPrefix starts the manifest's compact credential
	// value form, dr-credential:<credential-id>/<key>. The compiler rewrites
	// it into the API's object form; the object form is accepted in the file
	// as well.
	CredentialShorthandPrefix = "dr-credential:"

	// credentialSource is the API's source discriminator for
	// credential-backed environment variables. It mirrors the workload
	// package's EnvironmentVarSourceDRCredential without importing it, so
	// this package stays standalone; a test locks the two together.
	credentialSource = "dr-credential"
)

// CredentialRef names one stored credential the manifest references, in
// either value form. The manifest package performs no API calls; callers
// verify each referenced credential exists (a GET per CredentialID) before
// anything mutates, so a stale id fails in milliseconds instead of after a
// build.
type CredentialRef struct {
	CredentialID string
	// Key selects which field of the credential feeds the variable.
	Key string
	// EnvName is the environment variable the reference feeds, so errors can
	// name the variable rather than only the credential id.
	EnvName string
	// Line is where the reference appears in the manifest.
	Line int
}

// Compiled is a manifest lowered to what the API accepts.
type Compiled struct {
	// Payload is valid workload-create input: the manifest with the
	// CLI-managed workloadId stripped and credential shorthand expanded,
	// unknown blocks passed through untouched.
	Payload json.RawMessage
	// WorkloadID is the stripped binding, "" when the manifest is unbound.
	WorkloadID string
	// ArtifactID is the artifact the file binds to by id, "" when the file
	// describes one inline instead. Unlike WorkloadID it stays in Payload,
	// because it is the platform's own field rather than a CLI convenience.
	// A deploy reads it to know there is nothing to create: the version being
	// rolled onto already exists.
	ArtifactID string
	// ArtifactName is what the inline block calls the artifact, "" when the
	// file describes none. Lifted out so a caller need not decode the payload.
	ArtifactName string
	// CredentialRefs lists every credential the payload references.
	CredentialRefs []CredentialRef
}

// Compile lowers the manifest to a workload-create payload. It does not run
// the validation ledger; call Validate first for line-anchored errors.
// Compile still refuses a malformed credential shorthand rather than sending
// it to the API as a literal value.
func (m *Manifest) Compile() (*Compiled, error) {
	refs, fieldErrs := collectCredentialRefs(m.root)
	if len(fieldErrs) > 0 {
		return nil, &ValidationError{File: m.fileLabel(), Errors: fieldErrs}
	}

	var doc map[string]any

	if err := m.root.Decode(&doc); err != nil {
		return nil, fmt.Errorf("cannot decode manifest: %w", err)
	}

	workloadID := m.WorkloadID()
	artifactID := topLevelString(m.root, keyArtifactID)
	artifactName, _ := mapAt(doc, keyArtifact)[keyName].(string)

	delete(doc, keyWorkloadID)
	expandCredentialShorthand(doc)

	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot convert manifest to JSON: %w", err)
	}

	return &Compiled{
		Payload:        payload,
		WorkloadID:     workloadID,
		ArtifactID:     artifactID,
		ArtifactName:   artifactName,
		CredentialRefs: refs,
	}, nil
}

// ArtifactPayload is the inline artifact block on its own, which is exactly
// what POST /artifacts/ accepts: the manifest carries the create request
// verbatim, so there is nothing to translate.
//
// A deploy needs this whenever the platform builds the image, because the
// artifact has to exist before there is anywhere to sync code to, and it has
// to be built before there is an image for a workload to run. A manifest that
// binds an existing artifact by id has no block to hand over and says so.
func (c *Compiled) ArtifactPayload() (json.RawMessage, error) {
	payload, err := c.decode()
	if err != nil {
		return nil, err
	}

	artifact, ok := payload[keyArtifact]
	if !ok {
		return nil, fmt.Errorf("the manifest has no %s block to create", keyArtifact)
	}

	raw, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the %s block to JSON: %w", keyArtifact, err)
	}

	return raw, nil
}

// ArtifactSpecPayload is the spec inside the inline artifact block, which is
// what an update takes: the block's own name and type are not an update's to
// restate. A manifest bound by id has no block and says so.
func (c *Compiled) ArtifactSpecPayload() (json.RawMessage, error) {
	payload, err := c.decode()
	if err != nil {
		return nil, err
	}

	artifact, ok := payload[keyArtifact].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the manifest has no %s block to update from", keyArtifact)
	}

	spec, ok := artifact[keySpec]
	if !ok {
		return nil, fmt.Errorf("the manifest's %s block has no %s to update from", keyArtifact, keySpec)
	}

	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the %s block to JSON: %w", keySpec, err)
	}

	return raw, nil
}

// BindArtifact is the create payload with the inline artifact swapped for a
// reference to one that already exists. The two forms are mutually exclusive
// in the API as they are in the manifest, so the block is removed rather than
// left alongside the id.
//
// This is what a built deploy sends. The artifact was created from the block,
// then filled with code and an image; sending the block again would ask the
// platform for a second, empty artifact.
func (c *Compiled) BindArtifact(artifactID string) (json.RawMessage, error) {
	payload, err := c.decode()
	if err != nil {
		return nil, err
	}

	delete(payload, keyArtifact)
	payload[keyArtifactID] = artifactID

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the manifest to JSON: %w", err)
	}

	return raw, nil
}

// RuntimePayload is the runtime block on its own: how much the workload runs
// with, and nothing about what it runs. It is what a settings update sends,
// and what rides along with a replacement when one deploy has to change both
// halves at once.
//
// A file with no runtime block has nothing to send. That is a caller error
// rather than a user one, since the change being applied was computed from the
// same block: with no block there is no drift, and with no drift nothing asks
// for this.
func (c *Compiled) RuntimePayload() (json.RawMessage, error) {
	payload, err := c.decode()
	if err != nil {
		return nil, err
	}

	runtime, ok := payload[keyRuntime]
	if !ok {
		return nil, fmt.Errorf("the manifest has no %s block to apply", keyRuntime)
	}

	raw, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the %s block to JSON: %w", keyRuntime, err)
	}

	return raw, nil
}

// decode reads the compiled payload back into a tree. It is decoded per call
// rather than cached because each caller edits its copy, and a shared one
// would let one deploy's edit leak into another's payload.
func (c *Compiled) decode() (map[string]any, error) {
	var payload map[string]any

	if err := json.Unmarshal(c.Payload, &payload); err != nil {
		return nil, fmt.Errorf("cannot read the compiled manifest: %w", err)
	}

	return payload, nil
}

// expandCredentialShorthand rewrites every environmentVars entry carrying
// the shorthand into the API object form. The walk is shape-agnostic so
// environmentVars is found wherever the spec puts it: containerGroups,
// components, or locations the platform adds later.
func expandCredentialShorthand(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == keyEnvironmentVars {
				expandVarsList(child)
			}

			expandCredentialShorthand(child)
		}
	case []any:
		for _, item := range typed {
			expandCredentialShorthand(item)
		}
	}
}

func expandVarsList(value any) {
	list, ok := value.([]any)
	if !ok {
		return
	}

	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		raw, ok := entry[keyValue].(string)
		if !ok || !strings.HasPrefix(raw, CredentialShorthandPrefix) {
			continue
		}

		id, key, ok := parseCredentialShorthand(raw)
		if !ok {
			// Unreachable after Compile's syntax gate; leaving the raw value
			// in place is still safer than inventing a partial object.
			continue
		}

		delete(entry, keyValue)
		entry[keySource] = credentialSource
		entry[keyDRCredentialID] = id
		entry[keyKey] = key
	}
}

// parseCredentialShorthand splits dr-credential:<credential-id>/<key> into
// its parts; ok is false when either part is empty. The key may itself
// contain slashes: only the first one separates.
func parseCredentialShorthand(value string) (id, key string, ok bool) {
	rest := strings.TrimPrefix(value, CredentialShorthandPrefix)

	id, key, found := strings.Cut(rest, "/")
	if !found || id == "" || key == "" {
		return "", "", false
	}

	return id, key, true
}

// collectCredentialRefs walks every environmentVars sequence in the tree and
// returns each dr-credential reference, shorthand and object form alike,
// with syntax problems collected alongside. Validate reports the problems;
// Compile returns the refs so callers can GET-verify each credential before
// anything mutates.
func collectCredentialRefs(root *yaml.Node) ([]CredentialRef, []FieldError) {
	c := &refCollector{}
	c.walk(root, "")

	return c.refs, c.errs
}

type refCollector struct {
	refs []CredentialRef
	errs []FieldError
}

func (c *refCollector) walk(node *yaml.Node, path string) {
	node = resolveAlias(node)
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1]

			if key == keyEnvironmentVars {
				c.collectFromVars(value, joinPath(path, key))
			}

			c.walk(value, joinPath(path, key))
		}
	case yaml.SequenceNode:
		for i, item := range node.Content {
			c.walk(item, fmt.Sprintf("%s[%d]", path, i))
		}
	case yaml.DocumentNode, yaml.ScalarNode, yaml.AliasNode:
		// Leaves. AliasNode is unreachable here: resolveAlias above already
		// replaced it with the node it anchors, but the exhaustive linter
		// wants every yaml.Kind accounted for.
	}
}

func (c *refCollector) collectFromVars(vars *yaml.Node, path string) {
	for i, entry := range seqItems(vars) {
		entryPath := fmt.Sprintf("%s[%d]", path, i)
		name, _ := scalarString(mapValue(entry, keyName))

		if value, ok := scalarString(mapValue(entry, keyValue)); ok && strings.HasPrefix(value, CredentialShorthandPrefix) {
			c.addShorthand(mapValue(entry, keyValue), entryPath, name, value)
			continue
		}

		if source, _ := scalarString(mapValue(entry, keySource)); source == credentialSource {
			c.addObjectForm(entry, entryPath, name)
		}
	}
}

func (c *refCollector) addShorthand(node *yaml.Node, path, name, value string) {
	id, key, ok := parseCredentialShorthand(value)
	if !ok {
		c.errs = append(c.errs, FieldError{
			Path: path + "." + keyValue,
			Line: node.Line,
			Msg:  fmt.Sprintf("credential reference %q must be %s<credential-id>/<key>", value, CredentialShorthandPrefix),
		})

		return
	}

	c.refs = append(c.refs, CredentialRef{CredentialID: id, Key: key, EnvName: name, Line: node.Line})
}

func (c *refCollector) addObjectForm(entry *yaml.Node, path, name string) {
	id, _ := scalarString(mapValue(entry, keyDRCredentialID))
	key, _ := scalarString(mapValue(entry, keyKey))

	if id == "" {
		c.errs = append(c.errs, FieldError{
			Path: path + "." + keyDRCredentialID,
			Line: entry.Line,
			Msg:  fmt.Sprintf("%s is required when source is %s", keyDRCredentialID, credentialSource),
		})
	}

	if key == "" {
		c.errs = append(c.errs, FieldError{
			Path: path + "." + keyKey,
			Line: entry.Line,
			Msg:  fmt.Sprintf("%s is required when source is %s", keyKey, credentialSource),
		})
	}

	if id != "" {
		c.refs = append(c.refs, CredentialRef{CredentialID: id, Key: key, EnvName: name, Line: entry.Line})
	}
}
