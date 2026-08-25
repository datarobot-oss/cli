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
	"sort"

	"gopkg.in/yaml.v3"
)

// fieldRule declares the metadata for one manifest field. It is the single
// source of truth consulted by the strip, hoist, and ordering passes that
// prepare a server document for rendering.
//
// Strip scoping decision: serverManaged keys are deleted at ANY depth
// (including inside codeRef.datarobot and any other nested mapping). This
// preserves the existing behavior: the create spec has no legitimate key by
// these names anywhere, so recursive deletion is safe. Scoping to known paths
// would be a deliberate behavior change; we preserve any-depth and document
// it here.
type fieldRule struct {
	name string

	// nullable documents whether the field can arrive as null from the server
	// (Surface A — GET responses). Documentation only; the validator has its
	// own null-handling predicates (isNull/isNullish) and does not consult
	// this field. See library/api-reference.md for the authoritative summary.
	nullable bool

	// serverManaged means the field is the server's own bookkeeping and is
	// stripped at any depth by stripKeys. The create spec has no legitimate
	// key by these names anywhere, so recursive deletion is safe.
	serverManaged bool

	// buildOutput means the field is a server-assigned build output, stripped
	// by stripBuildOutputs. When buildConfigScoped is true, the field lives
	// inside imageBuildConfig; otherwise it lives on the container directly.
	// The conditional imageUri/imageBuildConfig deletion logic in
	// stripBuildOutputs is behavior, not metadata — it depends on whether
	// the build config is empty after stripping, which the table cannot
	// express.
	buildOutput       bool
	buildConfigScoped bool

	// hoistName means name is hoisted to the front of sequence-item mappings
	// found under this key by hoistNameKeys. Only containerGroups and
	// containers carry identifier mappings; opaque user/platform blocks keep
	// their original key order per the pass-through contract.
	hoistName bool

	// topLevelOrder is the position of this field in the top-level manifest
	// mapping (0 = not a top-level field). Both Live.Render and Draft.Render
	// consult this ordering so the two renderers emit the same key sequence.
	topLevelOrder int
}

// fieldRules is the single source of truth for manifest field metadata.
// Every strip, hoist, and ordering pass in the package consults this table
// rather than maintaining its own inline list.
//
// The table is intentionally not exhaustive: it covers the fields the four
// scattered knowledge sites (serverManagedKeys, stripBuildOutputs literals,
// hoistNameKeys targets, and the two parallel top-level orderings) already
// tracked. Fields that are purely pass-through (unknown blocks the package
// has never heard of) need no rule and are preserved verbatim.
var fieldRules = []fieldRule{
	// Server-managed keys — stripped at any depth by stripKeys. The create
	// spec has no legitimate key by these names anywhere, so recursive
	// deletion is safe. See the strip scoping decision in the fieldRule doc
	// comment above.
	{name: keyID, serverManaged: true},
	{name: keyCreatedAt, serverManaged: true},
	{name: keyUpdatedAt, serverManaged: true},
	{name: keyStatus, serverManaged: true},
	{name: keyEndpoint, serverManaged: true},
	{name: keyArtifactID, serverManaged: true},
	{name: keyWorkloadID, serverManaged: true, topLevelOrder: 1},
	{name: keyResolvedBundle, nullable: true, serverManaged: true},
	{name: keyImageOutdated, serverManaged: true},

	// Build outputs — stripped from containers by stripBuildOutputs. These
	// are scoped to containers (or inside imageBuildConfig) because the bare
	// word "build" is generic enough that adding it to serverManagedKeys
	// could eat a legitimate user key at some other level of the spec.
	{name: keyBuild, nullable: true, buildOutput: true},
	{name: keyCodeRef, nullable: true, buildOutput: true, buildConfigScoped: true},

	// Identifier keys — name is hoisted in sequence-item mappings under
	// these keys by hoistNameKeys. Only these two keys carry identifier
	// mappings; opaque user/platform blocks keep their original key order.
	{name: keyContainerGroups, hoistName: true},
	{name: keyContainers, hoistName: true},

	// Top-level fields — ordered for both Live.Render and Draft.Render so
	// the two renderers emit the same key sequence. workloadId is declared
	// above (it is also server-managed, stripped from spec/runtime at any
	// depth while written at the top level by Render).
	{name: keyName, topLevelOrder: 2},
	{name: keyImportance, topLevelOrder: 3},
	{name: keyArtifact, nullable: true, topLevelOrder: 4},
	{name: keyRuntime, topLevelOrder: 5},
}

// hoistNameKeySet is the set of keys whose sequence-item mappings have name
// hoisted to the front, derived from the field rule table at init time so
// the recursive hoistNameKeys walk does not allocate on every call.
var hoistNameKeySet map[string]bool

func init() {
	hoistNameKeySet = make(map[string]bool)

	for _, rule := range fieldRules {
		if rule.hoistName {
			hoistNameKeySet[rule.name] = true
		}
	}
}

// serverManagedFieldKeys returns the keys that are stripped at any depth by
// stripKeys, in the order they appear in the table.
func serverManagedFieldKeys() []string {
	var keys []string

	for _, rule := range fieldRules {
		if rule.serverManaged {
			keys = append(keys, rule.name)
		}
	}

	return keys
}

// containerBuildOutputKeys returns the build-output keys that live on a
// container directly (not inside imageBuildConfig), in table order.
func containerBuildOutputKeys() []string {
	var keys []string

	for _, rule := range fieldRules {
		if rule.buildOutput && !rule.buildConfigScoped {
			keys = append(keys, rule.name)
		}
	}

	return keys
}

// buildConfigBuildOutputKeys returns the build-output keys that live inside
// imageBuildConfig, in table order.
func buildConfigBuildOutputKeys() []string {
	var keys []string

	for _, rule := range fieldRules {
		if rule.buildOutput && rule.buildConfigScoped {
			keys = append(keys, rule.name)
		}
	}

	return keys
}

// topLevelOrderedFields returns the top-level manifest keys in the order both
// Live.Render and Draft.Render emit them, derived from the field rule table.
func topLevelOrderedFields() []string {
	var ordered []fieldRule

	for _, rule := range fieldRules {
		if rule.topLevelOrder > 0 {
			ordered = append(ordered, rule)
		}
	}

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].topLevelOrder < ordered[j].topLevelOrder
	})

	keys := make([]string, len(ordered))

	for i, rule := range ordered {
		keys[i] = rule.name
	}

	return keys
}

// orderedFields builds a field list in the order defined by the rule table's
// top-level ordering. Both Live.Render and Draft.Render call this so they
// emit the same key sequence. Keys absent from values are skipped, which is
// how Draft.Render omits workloadId when it is empty.
func orderedFields(values map[string]*yaml.Node, comments map[string]string) []field {
	order := topLevelOrderedFields()

	fields := make([]field, 0, len(order))

	for _, key := range order {
		value, ok := values[key]
		if !ok {
			continue
		}

		fields = append(fields, field{key: key, value: value, comment: comments[key]})
	}

	return fields
}
