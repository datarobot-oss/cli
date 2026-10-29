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

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReservedProfileError is returned when a profile deletion names "default",
// the reserved profile name. Both identities the name can address are
// protected: the root-level credentials (addressed as "default" whenever no
// explicit profiles.default section is defined) and an explicit
// profiles.default section itself. The root-level credentials are cleared by
// `dr auth logout`, which is the remedy the message points at.
type ReservedProfileError struct {
	Name string
}

// Error renders the refusal plus the logout remedy.
func (e *ReservedProfileError) Error() string {
	return strings.Join([]string{
		fmt.Sprintf("the profile name %q is reserved and cannot be deleted", NormalizeProfileName(e.Name)),
		"",
		`Omitting the profile selection uses the root-level credentials, which are`,
		`also addressed as "default"; to clear those, run:`,
		"  dr auth logout",
		"",
		`An explicit profiles.default section, when one is defined, is likewise`,
		`protected and cannot be removed by delete.`,
	}, "\n")
}

// DeleteProfile removes the named profile's section from drconfig.yaml using
// the same comment-preserving YAML-node editing as UpdateConfigFile: only the
// profiles.<name> mapping is removed, and the root-level credentials,
// sibling profiles, global keys, and every unrelated comment are re-emitted
// untouched.
//
// The file is never modified unless the deletion is sound. Invalid names and
// the reserved "default" name are refused, a name that is not defined fails
// with UnknownProfileError, and three unsound shapes are refused before any
// write because editing them would corrupt or misreport the config:
//
//   - a multi-document YAML stream, since the single-document reader and
//     writer would silently discard every later document;
//   - several profiles sections whose keys normalize to the requested name
//     (for example "Beta" and "beta"), since removing one would leave the
//     other's credentials in place while reporting the profile deleted; and
//   - a target section defining YAML anchors that surviving sections alias,
//     since removing the anchor would leave dangling aliases and an
//     unloadable config.
//
// A file that cannot be parsed or rewritten fails with its path and cause --
// all without touching disk.
func DeleteProfile(name string) error {
	name = NormalizeProfileName(name)

	if err := ValidateProfileName(name); err != nil {
		return err
	}

	if name == DefaultProfileLabel {
		return &ReservedProfileError{Name: name}
	}

	configFile, err := resolveConfigFilePath()
	if err != nil {
		return err
	}

	// The document count is checked before anything is parsed for editing:
	// readYAMLNode decodes only a stream's first document, so a multi-document
	// file would otherwise lose every later document on rewrite.
	if err := refuseMultiDocumentStream(configFile); err != nil {
		return err
	}

	rootNode, profilesNode, err := profilesSectionForEdit(configFile)
	if err != nil {
		return err
	}

	// A missing (or null) profiles block simply has no defined profiles.
	if profilesNode == nil {
		return &UnknownProfileError{Name: name}
	}

	matches := matchingProfileSectionIndices(profilesNode, name)
	if len(matches) == 0 {
		return &UnknownProfileError{Name: name, Known: profileNamesFromNode(profilesNode)}
	}

	// More than one section normalizing to the requested name is refused
	// rather than silently deleting only the first match.
	if len(matches) > 1 {
		return duplicateProfileSectionError(configFile, profilesNode, matches)
	}

	matchIndex := matches[0]
	targetValue := profilesNode.Content[matchIndex+1]

	// Deleting an anchor definition that surviving sections still alias is
	// refused rather than leaving the rewritten config unloadable.
	if err := refuseOrphanedAliases(configFile, rootNode, targetValue, name); err != nil {
		return err
	}

	profilesNode.Content = append(profilesNode.Content[:matchIndex], profilesNode.Content[matchIndex+2:]...)

	return writeYAMLNode(configFile, rootNode)
}

// profilesSectionForEdit reads the config file and returns its root node
// together with the profiles mapping node to edit. It returns a nil profiles
// node when no profiles block is defined, and an error naming the config
// path when the file cannot be read or is not sound to edit.
func profilesSectionForEdit(configFile string) (*yaml.Node, *yaml.Node, error) {
	rootNode, err := readYAMLNode(configFile)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", configFile, err)
	}

	if rootNode.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s: config file is not a YAML mapping; refusing to edit it", configFile)
	}

	profilesValue := mappingValueNode(rootNode, profilesKey)

	// "profiles" holding something other than a mapping (and not being null)
	// is an unsound config: refuse it rather than reporting a missing profile
	// that could not exist in it.
	if profilesValue != nil && profilesValue.Kind != yaml.MappingNode && !isNullNode(profilesValue) {
		return nil, nil, fmt.Errorf("%s: %q is not a mapping; refusing to edit it", configFile, profilesKey)
	}

	if profilesValue == nil || isNullNode(profilesValue) {
		return rootNode, nil, nil
	}

	return rootNode, profilesValue, nil
}

// isNullNode reports whether node is a YAML null scalar (e.g. a bare
// "profiles:" line).
func isNullNode(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

// matchingProfileSectionIndices returns the content indices (of key nodes;
// each value follows at i+1) of the profiles entries whose key matches name
// exactly or after normalization (a section stored as "Beta" is matched by
// "beta").
func matchingProfileSectionIndices(profilesNode *yaml.Node, name string) []int {
	matches := []int{}

	for i := 0; i+1 < len(profilesNode.Content); i += 2 {
		keyNode := profilesNode.Content[i]

		if keyNode.Value != name && NormalizeProfileName(keyNode.Value) != name {
			continue
		}

		matches = append(matches, i)
	}

	return matches
}

// duplicateProfileSectionError renders the refusal for a profiles mapping
// that defines several sections normalizing to the requested name (for
// example "Beta" and "beta"): deleting one would leave the other's
// credentials in place while reporting the profile deleted.
func duplicateProfileSectionError(configFile string, profilesNode *yaml.Node, matches []int) error {
	spellings := make([]string, 0, len(matches))

	for _, i := range matches {
		spellings = append(spellings, fmt.Sprintf("%q", profilesNode.Content[i].Value))
	}

	return fmt.Errorf(
		"%s: profiles defines %d sections whose names normalize to the requested profile (%s); refusing to delete it because removing one would leave the other's credentials in place",
		configFile, len(matches), strings.Join(spellings, ", "),
	)
}

// refuseMultiDocumentStream rejects YAML files containing more than one
// document. readYAMLNode decodes only a stream's first document, so a
// deletion that edited and rewrote a multi-document file would silently
// discard every later document; the delete path refuses such files instead.
func refuseMultiDocumentStream(configFile string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("%s: failed to read config file: %w", configFile, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))

	documents := 0

	for {
		var node yaml.Node

		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("%s: failed to parse config file: %w", configFile, err)
		}

		documents++
	}

	if documents > 1 {
		return fmt.Errorf(
			"%s: config file contains %d YAML documents; refusing to edit it because deleting a profile would discard the later documents",
			configFile, documents,
		)
	}

	return nil
}

// refuseOrphanedAliases rejects deleting a section whose subtree defines YAML
// anchors that surviving parts of the document alias. yaml.v3 re-encodes
// alias nodes as references to their anchor, so removing the anchor would
// leave a dangling alias and an unloadable config.
func refuseOrphanedAliases(configFile string, rootNode *yaml.Node, target *yaml.Node, name string) error {
	anchors := map[*yaml.Node]bool{}

	collectAnchorNodes(target, anchors)

	if len(anchors) == 0 || !documentAliasesAnchor(rootNode, target, anchors) {
		return nil
	}

	anchorNames := make([]string, 0, len(anchors))

	for node := range anchors {
		anchorNames = append(anchorNames, "&"+node.Anchor)
	}

	sort.Strings(anchorNames)

	return fmt.Errorf(
		"%s: refusing to delete %q: its section defines YAML anchor(s) %s that surviving sections alias; deleting it would leave the config unloadable",
		configFile, name, strings.Join(anchorNames, ", "),
	)
}

// collectAnchorNodes gathers every node of subtree that defines a YAML
// anchor. Alias nodes are skipped: they reference anchors defined elsewhere
// and cannot define one themselves.
func collectAnchorNodes(node *yaml.Node, anchors map[*yaml.Node]bool) {
	if node == nil || node.Kind == yaml.AliasNode {
		return
	}

	if node.Anchor != "" {
		anchors[node] = true
	}

	for _, child := range node.Content {
		collectAnchorNodes(child, anchors)
	}
}

// documentAliasesAnchor walks the document from root, skipping the deleted
// subtree, and reports whether any surviving alias node resolves to an anchor
// that the deleted subtree defined.
func documentAliasesAnchor(node *yaml.Node, skip *yaml.Node, anchors map[*yaml.Node]bool) bool {
	if node == skip {
		return false
	}

	if node.Kind == yaml.AliasNode && node.Alias != nil && anchors[node.Alias] {
		return true
	}

	for _, child := range node.Content {
		if documentAliasesAnchor(child, skip, anchors) {
			return true
		}
	}

	return false
}

// profileNamesFromNode lists the normalized names of the sections in
// profilesNode, sorted, for unknown-profile guidance after a refused
// deletion. The root profile is not a section here and never appears.
func profileNamesFromNode(profilesNode *yaml.Node) []string {
	names := make([]string, 0, len(profilesNode.Content)/2)

	for i := 0; i+1 < len(profilesNode.Content); i += 2 {
		name := NormalizeProfileName(profilesNode.Content[i].Value)
		if name == "" {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
