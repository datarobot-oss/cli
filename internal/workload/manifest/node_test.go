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
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestIsNullish covers the null-blind-spot helper that node-based checks use
// to treat a present-but-null YAML key as absent.
func TestIsNullish(t *testing.T) {
	tests := map[string]struct {
		node     *yaml.Node
		expected bool
	}{
		"nil node": {
			node:     nil,
			expected: true,
		},
		"scalar null tag": {
			node:     &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: ""},
			expected: true,
		},
		"empty mapping": {
			node:     &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{}},
			expected: true,
		},
		"scalar string": {
			node:     &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "hello"},
			expected: false,
		},
		"scalar int": {
			node:     &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "42"},
			expected: false,
		},
		"non-empty mapping": {
			node: &yaml.Node{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "key"},
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "value"},
				},
			},
			expected: false,
		},
		"empty sequence": {
			node:     &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{}},
			expected: false,
		},
		"non-empty sequence": {
			node: &yaml.Node{
				Kind: yaml.SequenceNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "item"},
				},
			},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, isNullish(test.node))
		})
	}
}

// TestIsNull covers the narrow null predicate: true ONLY for a nil node or a
// scalar with the !!null tag. An empty mapping is NOT null — that is the key
// difference from isNullish, and it is what lets the exclusivity checks
// (checkArtifactBinding, checkImageSource, checkArtifact) reject artifact: {}
// and imageBuildConfig: {} alongside their counterparts.
func TestIsNull(t *testing.T) {
	tests := map[string]struct {
		node     *yaml.Node
		expected bool
	}{
		"nil node": {
			node:     nil,
			expected: true,
		},
		"scalar null tag": {
			node:     &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: ""},
			expected: true,
		},
		"empty mapping is NOT null": {
			node:     &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{}},
			expected: false,
		},
		"scalar string": {
			node:     &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "hello"},
			expected: false,
		},
		"scalar int": {
			node:     &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "42"},
			expected: false,
		},
		"non-empty mapping": {
			node: &yaml.Node{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "key"},
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "value"},
				},
			},
			expected: false,
		},
		"empty sequence": {
			node:     &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{}},
			expected: false,
		},
		"non-empty sequence": {
			node: &yaml.Node{
				Kind: yaml.SequenceNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Tag: "!!str", Value: "item"},
				},
			},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, isNull(test.node))
		})
	}
}
