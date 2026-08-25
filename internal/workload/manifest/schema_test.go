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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// VAL-SCHEMA-001: Draft.Render and Live.Render emit the same top-level key
// order, pinned against the single shared ordering definition in the field
// rule table (schema.go). Today only Live.Render's order is pinned
// (VAL-RENDER-014); Draft.Render's order can drift silently. This test
// asserts both renderers produce identical top-level key sequences matching
// topLevelOrderedFields.
func TestSchema_TopLevelKeyOrderMatches(t *testing.T) {
	expected := topLevelOrderedFields()

	// Draft.Render — serviceDraft has WorkloadID set, so all 5 keys are present.
	draftRendered, err := serviceDraft().Render()
	require.NoError(t, err)

	draftRoot := renderedRoot(t, draftRendered)
	draftKeys := mappingKeys(draftRoot)

	assert.Equal(t, expected, draftKeys,
		"Draft.Render top-level keys must match the shared ordering")

	// Live.Render — liveFixture has WorkloadID set, so all 5 keys are present.
	liveRendered, err := liveFixture(t).Render()
	require.NoError(t, err)

	liveRoot := renderedRoot(t, liveRendered)
	liveKeys := mappingKeys(liveRoot)

	assert.Equal(t, expected, liveKeys,
		"Live.Render top-level keys must match the shared ordering")

	// Both renderers emit the same sequence.
	assert.Equal(t, draftKeys, liveKeys,
		"Draft.Render and Live.Render must emit the same top-level key order")
}

// VAL-SCHEMA-002: strip/hoist behavior is unchanged after the field-metadata
// consolidation. A server document containing the full server-managed set
// (id, createdAt, updatedAt, status, endpoint, artifactId, resolvedBundle,
// imageOutdated, the build block, codeRef) renders without any of those keys,
// name still leads in containerGroup/container sequence-item mappings, and
// opaque mappings (labels) keep their original key order. This is a
// consolidation pin — it may reuse existing fixtures and asserts the same
// behavior the existing strip/hoist/order suite covers, phrased against the
// consolidated table so a regression in the table-to-consumer wiring is
// caught directly.
func TestSchema_ConsolidationPinStripAndHoist(t *testing.T) {
	// Build a server document carrying the full server-managed set plus an
	// opaque labels block (with name not as the first key) to verify that
	// hoisting is scoped to identifier mappings only.
	workloadDoc := fullServerWorkloadDoc(t)
	artifactDoc := fullServerArtifactDoc(t)

	live, err := NewLive("68b0c1d2e3f4a5b6c7d8e9f0", workloadDoc, artifactDoc)
	require.NoError(t, err)

	rendered, err := live.Render()
	require.NoError(t, err)

	// Server-managed key values must not appear in the rendered output.
	// workloadId is the exception: it is the one binding the file carries.
	for _, unwanted := range []string{
		"68a0000000000000000000a1", // artifactId value
		"app.datarobot.com",        // endpoint value
		"status: running",          // workload status
		"status: locked",           // artifact status
		"2026-01-01T00:00:00Z",     // createdAt value
		"2026-01-02T00:00:00Z",     // updatedAt value
		"resolvedBundle",           // server-managed key
		"imageOutdated",            // server-managed key
		"artifactImageBuildId",     // build output key
		"build-123",                // build output value
		"codeRef",                  // build config output key
		"built-by-server",          // imageUri value (stripped for build container)
	} {
		assert.NotContains(t, string(rendered), unwanted,
			"rendered manifest must not contain %s after consolidation", unwanted)
	}

	// The binding is the one id the file carries.
	assert.Contains(t, string(rendered), "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0")

	root := renderedRoot(t, rendered)

	// Name leads in container/containerGroup sequence-item mappings.
	assertNameLeadsInIdentifierMappings(t, root, "")

	// Opaque mappings (labels) keep their key order — name is NOT hoisted
	// because labels is not a sequence item under containerGroups/containers.
	// documentNode sorts keys alphabetically, so the labels block arrives as
	// app, name, version (alphabetical). hoistNameKeys must not promote name
	// to the front; if it did, the order would be name, app, version.
	spec := mapValue(mapValue(root, keyArtifact), keySpec)

	for _, group := range seqItems(mapValue(spec, keyContainerGroups)) {
		for _, container := range seqItems(mapValue(group, keyContainers)) {
			labels := mapValue(container, "labels")
			if labels == nil {
				continue
			}

			keys := mappingKeys(labels)
			assert.Equal(t, []string{"app", "name", "version"}, keys,
				"opaque mapping (labels) must keep alphabetical key order, not hoist name")
		}
	}

	// The rendered manifest round-trips through Parse and Validate.
	parsed, err := Parse(rendered, "")
	require.NoError(t, err)

	require.NoError(t, parsed.Validate())
}

// fullServerWorkloadDoc returns a workload GET response carrying every
// server-managed key the field rule table declares, so the consolidation pin
// can verify they are all stripped.
func fullServerWorkloadDoc(t *testing.T) map[string]any {
	t.Helper()

	const doc = `{
		"id": "68b0c1d2e3f4a5b6c7d8e9f0",
		"name": "test-workload",
		"importance": "low",
		"status": "running",
		"createdAt": "2026-01-01T00:00:00Z",
		"updatedAt": "2026-01-02T00:00:00Z",
		"endpoint": "https://app.datarobot.com/workloads/68b0/",
		"artifactId": "68a0000000000000000000a1",
		"runtime": {
			"containerGroups": [
				{
					"name": "default",
					"replicaCount": 3,
					"autoscaling": null,
					"resolvedBundle": {"catalogId": "cat1", "catalogVersionId": "ver1"},
					"containers": [
						{"name": "app", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}
					]
				}
			]
		}
	}`

	var out map[string]any

	require.NoError(t, json.Unmarshal([]byte(doc), &out))

	return out
}

// fullServerArtifactDoc returns an artifact GET response carrying every
// server-managed and build-output key, plus an opaque labels block with name
// not as the first key, so the consolidation pin can verify stripping and
// hoist scoping together.
func fullServerArtifactDoc(t *testing.T) map[string]any {
	t.Helper()

	const doc = `{
		"id": "68a0000000000000000000a1",
		"name": "test-artifact",
		"type": "service",
		"status": "locked",
		"createdAt": "2026-01-01T00:00:00Z",
		"spec": {
			"containerGroups": [
				{
					"name": "default",
					"containers": [
						{
							"name": "app",
							"primary": true,
							"port": 8080,
							"imageUri": "registry.internal/built-by-server:abc",
							"imageOutdated": true,
							"build": {
								"artifactImageBuildId": "build-123",
								"status": "completed",
								"createdAt": "2026-07-02T10:00:00Z"
							},
							"imageBuildConfig": {
								"dockerfile": {"source": "provided"},
								"codeRef": {"datarobot": {"catalogId": "cat1", "catalogVersionId": "ver1"}}
							},
							"labels": {"app": "test", "name": "not-an-id", "version": "v1"}
						}
					]
				}
			]
		}
	}`

	var out map[string]any

	require.NoError(t, json.Unmarshal([]byte(doc), &out))

	return out
}
