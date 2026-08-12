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

import "gopkg.in/yaml.v3"

// nullTag is the yaml.v3 tag for an explicit YAML null, the one scalar kind
// the helpers below reject.
const nullTag = "!!null"

// resolveAlias follows an alias to the node it anchors, so the helpers below
// see through YAML anchors/aliases the same way they see a literal value.
// yaml.v3 aliases point directly at the anchored node, never at another
// alias, so one hop is enough.
func resolveAlias(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.AliasNode {
		return node.Alias
	}

	return node
}

// mapValue returns the value node for key in a mapping node, nil when the
// key is absent or node is not a mapping. Nil-safe so lookups chain:
// mapValue(mapValue(root, "artifact"), "spec").
func mapValue(node *yaml.Node, key string) *yaml.Node {
	node = resolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return resolveAlias(node.Content[i+1])
		}
	}

	return nil
}

// seqItems returns a sequence node's items, nil for anything else.
func seqItems(node *yaml.Node) []*yaml.Node {
	node = resolveAlias(node)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}

	return node.Content
}

// scalarString returns a scalar node's string value; ok is false for nil,
// non-scalar and null nodes.
func scalarString(node *yaml.Node) (string, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag == nullTag {
		return "", false
	}

	return node.Value, true
}

// joinPath extends a YAML path with a key, leaving no leading dot on the
// root.
func joinPath(base, key string) string {
	if base == "" {
		return key
	}

	return base + "." + key
}
