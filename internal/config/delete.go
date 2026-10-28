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
	"fmt"
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
// with UnknownProfileError, and a file that cannot be parsed or rewritten
// fails with its path and cause -- all without touching disk.
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

	rootNode, profilesNode, err := profilesSectionForEdit(configFile)
	if err != nil {
		return err
	}

	// A missing (or null) profiles block simply has no defined profiles.
	if profilesNode == nil {
		return &UnknownProfileError{Name: name}
	}

	if !removeProfileSection(profilesNode, name) {
		return &UnknownProfileError{Name: name, Known: profileNamesFromNode(profilesNode)}
	}

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

// removeProfileSection removes the key/value pair whose key matches name
// exactly or after normalization (a section stored as "Beta" is removed by
// "beta") from profilesNode, reporting whether a section was removed.
func removeProfileSection(profilesNode *yaml.Node, name string) bool {
	for i := 0; i+1 < len(profilesNode.Content); i += 2 {
		keyNode := profilesNode.Content[i]

		if keyNode.Value != name && NormalizeProfileName(keyNode.Value) != name {
			continue
		}

		profilesNode.Content = append(profilesNode.Content[:i], profilesNode.Content[i+2:]...)

		return true
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
