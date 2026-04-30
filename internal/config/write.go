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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// PersistableKeys is the allowlist of viper keys that may be written back to
// drconfig.yaml. Any key not in this set is intentionally NOT persisted, even
// if it is currently set in the live viper config (e.g. transient flags such
// as --yes, --verbose, --force-interactive).
//
// To add a new persistable key, add it here. Keys may use viper dotted-path
// notation (e.g. "pulumi.config.passphrase") for nested values, but the
// current callers all use flat top-level keys.
var PersistableKeys = map[string]struct{}{
	DataRobotURL:               {},
	DataRobotAPIKey:            {},
	APIConsumerTrackingEnabled: {},
	"ssl_verify":               {},
	"pulumi_config_passphrase": {},
}

// UpdateConfigFile writes only the allowlisted keys from viper back to the
// drconfig.yaml file on disk, preserving any other fields and comments that
// already exist in the file but are not currently tracked by viper.
//
// This replaces direct calls to viper.WriteConfig(), which would otherwise
// serialize the entire viper.AllSettings() map -- including transient command
// flags such as --yes that should never be persisted.
//
// The keys argument optionally restricts the write to a subset of the
// allowlist. If keys is empty, all allowlisted keys currently set in viper
// are written. Any key passed in that is not in the allowlist is ignored.
func UpdateConfigFile(keys ...string) error {
	if err := CreateConfigFileDirIfNotExists(); err != nil {
		return err
	}

	configFile, err := resolveConfigFilePath()
	if err != nil {
		return err
	}

	rootNode, err := readYAMLNode(configFile)
	if err != nil {
		return err
	}

	applyAllowedKeysToNode(rootNode, keys)

	docNode := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{rootNode},
	}

	out, err := yaml.Marshal(docNode)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, out, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// resolveConfigFilePath returns the active drconfig.yaml path, falling back
// to the default location if viper has not yet recorded a config file used.
func resolveConfigFilePath() (string, error) {
	if configFile := viper.ConfigFileUsed(); configFile != "" {
		return configFile, nil
	}

	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, configFileName), nil
}

// readYAMLNode reads a YAML file into a *yaml.Node, preserving comments and
// structure. If the file does not exist or is empty, an empty map node is returned.
// Returns the root mapping node (unwrapping the document node if necessary).
func readYAMLNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
		}

		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if len(data) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}

	doc := &yaml.Node{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}

	if doc.Kind == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}

	return doc, nil
}

// applyAllowedKeysToNode updates keys in a yaml.Node, preserving comments
// and non-allowlisted keys. It navigates to nested keys using dotted notation
// (e.g. "foo.bar.baz").
func applyAllowedKeysToNode(node *yaml.Node, keys []string) {
	candidates := keys
	if len(candidates) == 0 {
		candidates = make([]string, 0, len(PersistableKeys))

		for k := range PersistableKeys {
			candidates = append(candidates, k)
		}
	}

	for _, key := range candidates {
		if _, ok := PersistableKeys[key]; !ok {
			continue
		}

		if !viper.IsSet(key) {
			continue
		}

		setNestedKeyInNode(node, key, viper.Get(key))
	}
}

// Note: Keys NOT in candidates are preserved as-is from the existing node.
// This is intentional to preserve custom fields written by users or other tools.

// setNestedKeyInNode sets a value at a dotted-path key in a yaml.Node,
// creating intermediate nodes as needed. Comments on existing keys are preserved.
func setNestedKeyInNode(node *yaml.Node, key string, value interface{}) {
	parts := strings.Split(key, ".")

	for i, part := range parts {
		if node.Kind != yaml.MappingNode {
			node.Kind = yaml.MappingNode
			node.Tag = "!!map"
			node.Content = []*yaml.Node{}
		}

		keyNode, valNode := findOrCreateKeyInNode(node, part)

		if i == len(parts)-1 {
			encodeValueToNode(valNode, value)
			return
		}

		if valNode.Kind != yaml.MappingNode {
			valNode.Kind = yaml.MappingNode
			valNode.Tag = "!!map"
			valNode.Content = []*yaml.Node{}
		}

		node = valNode
		_ = keyNode
	}
}

// findOrCreateKeyInNode finds or creates a key in a mapping node, returning
// both the key and value nodes. If the key exists, its existing value node
// is returned (preserving any comments on that node). If the key doesn't exist,
// a new entry is created.
func findOrCreateKeyInNode(node *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if node.Content == nil {
		node.Content = []*yaml.Node{}
	}

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			break
		}

		keyNode := node.Content[i]
		if keyNode.Value == key {
			return keyNode, node.Content[i+1]
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode}

	node.Content = append(node.Content, keyNode, valNode)

	return keyNode, valNode
}

// encodeValueToNode encodes a Go value into a yaml.Node, handling common types.
func encodeValueToNode(node *yaml.Node, value interface{}) {
	switch v := value.(type) {
	case string:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!str"
		node.Value = v

	case int, int32, int64:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!int"
		node.Value = fmt.Sprintf("%v", v)

	case float32, float64:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!float"
		node.Value = fmt.Sprintf("%v", v)

	case bool:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!bool"

		if v {
			node.Value = "true"
		} else {
			node.Value = "false"
		}

	case nil:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!null"
		node.Value = ""

	default:
		encoded, err := yaml.Marshal(v)
		if err != nil {
			node.Kind = yaml.ScalarNode
			node.Value = fmt.Sprintf("%v", v)

			return
		}

		tempNode := &yaml.Node{}

		if err := yaml.Unmarshal(encoded, tempNode); err != nil {
			node.Kind = yaml.ScalarNode
			node.Value = fmt.Sprintf("%v", v)

			return
		}

		*node = *tempNode
	}
}
