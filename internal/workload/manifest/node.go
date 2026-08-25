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
	"fmt"

	"gopkg.in/yaml.v3"
)

// yaml.v3 scalar tags the helpers below discriminate on. nullTag is the one
// scalar kind scalarString rejects; the other two keep a typed read from
// accepting a quoted lookalike.
const (
	nullTag = "!!null"
	intTag  = "!!int"
	boolTag = "!!bool"
)

// isNullish reports whether a node is meaningfully absent for validator
// checks. mapValue returns a non-nil scalar node for a present key whose value
// is null, so callers that test `!= nil` to mean "this block is set" get a
// false positive unless they also call this helper.
func isNullish(node *yaml.Node) bool {
	// Note: this helper deliberately collapses three wire shapes
	// 1. absent,
	// 2. explicit null, and
	// 3. empty mapping
	// into a single "not set" meaning because that is how the workload
	// endpoints behave today.
	//
	// The autoscaling BLOCK is nullable (AutoscalingProperties | None,
	// protons/runtime.py:502), so GET responses echo "autoscaling: null" when
	// no policy is configured — but its fields are NOT: enabled is a non-
	// nullable bool (default True, protons/runtime.py:217), so
	// "autoscaling: {enabled: null}" can never arrive. The real nullable set
	// (verified against the Pydantic models in workload-api/schemas/) is:
	// autoscaling block, codeRef, imageBuildConfig, imageUri, entrypoint,
	// port, primary, livenessProbe/readinessProbe/startupProbe, routes,
	// gpuType, build, and the api-key env var name. The create endpoint reads
	// an empty block the same as a missing one, and the CLI never writes
	// nulls back: stripKeys purges them so a committed manifest stays
	// canonical even when the wire format is not.
	//
	// If the API ever gives these shapes distinct meanings — the classic case
	// being null-as-delete in a PATCH — this helper, its validator call sites,
	// and the nil-key purge in stripKeys are what should be updated.
	node = resolveAlias(node)
	if node == nil {
		return true
	}

	if node.Kind == yaml.ScalarNode && node.Tag == nullTag {
		return true
	}

	return node.Kind == yaml.MappingNode && len(node.Content) == 0
}

// resolveAlias follows an alias to the node it anchors, so the helpers below
// see through YAML anchors/aliases the same way they see a literal value.
// yaml.v3 aliases point directly at the anchored node, never at another
// alias, so one hop is enough.
//
// Following aliases turns the parse tree into a graph, and every walk in this
// package assumes that graph is a finite tree. checkAliases, run once in
// Parse, is what makes that assumption hold.
func resolveAlias(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.AliasNode {
		return node.Alias
	}

	return node
}

// maxExpandedNodes caps how many nodes a walk may see once aliases are
// followed. A hand-written manifest is a few hundred; a million is unreachable
// by accident and still returns in milliseconds.
const maxExpandedNodes = 1 << 20

// checkAliases rejects a document whose anchors would make a walk of the parse
// tree unbounded. It is the single guard for every walk in this package, which
// is why it runs at the entry point rather than in each of them.
//
// Two shapes are unbounded, and both parse without complaint:
//
//   - A cycle. "runtime: &rt\n  containerGroups: *rt" resolves to itself, so a
//     walk that follows aliases never terminates. yaml.v3 catches this only
//     when decoding into a Go value, which happens after the raw-tree walks in
//     Validate and Compile, so their guard never runs first.
//   - An alias bomb. Ten levels of eight-wide aliasing is a tree of a hundred
//     nodes that expands to more than a billion. Nothing is stored twice, so
//     memory looks fine while the walk runs for hours.
//
// Depth is not a third shape to guard here. Every caller reaches this through
// yaml.Unmarshal, which refuses a document nested past its own limit before any
// of this runs, and an alias chain cannot get a walk deeper than the document
// either: YAML requires an anchor to be declared before it is used, so the
// shallow end of a chain is always reached and memoised first.
// TestParse_RejectsRunawayNesting holds the library to that.
//
// A manifest is not necessarily trusted input: cloning a repository and
// running any command that locates its manifest is enough to reach here.
func checkAliases(root *yaml.Node) error {
	_, err := expandedSize(root, map[*yaml.Node]int{}, map[*yaml.Node]bool{})

	return err
}

// expandedSize counts the nodes an alias-following walk would visit, erroring
// out on a cycle or once the count passes maxExpandedNodes.
//
// sizes memoises finished nodes, which is what keeps this linear in the parse
// tree while the thing it measures is exponential. onPath holds the nodes
// between the root and here: reaching one of those again is a cycle, whereas
// reaching a finished node again is just an anchor used twice, which is fine.
func expandedSize(node *yaml.Node, sizes map[*yaml.Node]int, onPath map[*yaml.Node]bool) (int, error) {
	node = resolveAlias(node)
	if node == nil {
		return 0, nil
	}

	if size, done := sizes[node]; done {
		return size, nil
	}

	if onPath[node] {
		return 0, fmt.Errorf("anchor %q on line %d contains itself", node.Anchor, node.Line)
	}

	onPath[node] = true
	defer delete(onPath, node)

	size := 1

	for _, child := range node.Content {
		childSize, err := expandedSize(child, sizes, onPath)
		if err != nil {
			return 0, err
		}

		size += childSize

		// Checked per child rather than at the end: an unchecked running
		// total overflows long before a deep enough bomb finishes counting.
		if size > maxExpandedNodes {
			return 0, fmt.Errorf("manifest is too large: more than %d YAML nodes once aliases are expanded",
				maxExpandedNodes)
		}
	}

	sizes[node] = size

	return size, nil
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

// scalarInt returns a scalar node's integer value; ok is false for nil,
// non-scalar and non-numeric nodes. YAML's own tag resolution decides what
// counts as a number, so quoted "8080" is a string and does not pass.
func scalarInt(node *yaml.Node) (int, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != intTag {
		return 0, false
	}

	// Decoded rather than parsed by hand: YAML writes integers in more ways
	// than Atoi reads (8_080, 0x10, 0o17), and a reader that disagrees with
	// its own parser rejects files that plainly state a number.
	var value int

	if err := node.Decode(&value); err != nil {
		return 0, false
	}

	return value, true
}

// isTrue reports whether node is the YAML boolean true. Anything else,
// including a missing node and the string "true", is false.
func isTrue(node *yaml.Node) bool {
	node = resolveAlias(node)
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != boolTag {
		return false
	}

	// Decoded for the same reason as scalarInt: True and TRUE are the same
	// boolean to YAML, and comparing the raw text reads them as false.
	var value bool

	return node.Decode(&value) == nil && value
}

// joinPath extends a YAML path with a key, leaving no leading dot on the
// root.
func joinPath(base, key string) string {
	if base == "" {
		return key
	}

	return base + "." + key
}
