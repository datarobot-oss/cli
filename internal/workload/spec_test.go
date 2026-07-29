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

package workload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSpec(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// JSON specs must pass through byte-for-byte: the payload is the user's
// document, not a re-serialization (key order, number formatting, and
// whitespace all survive).
func TestReadSpecFile_JSONPassthrough(t *testing.T) {
	content := "{\n  \"name\": \"my-app\",\n  \"artifactId\": \"abc\",  \"replicas\": 1e2\n}"
	path := writeSpec(t, "spec.json", content)

	payload, err := ReadSpecFile(path)
	require.NoError(t, err)

	assert.Equal(t, content, string(payload))
}

func TestReadSpecFile_YAMLConverted(t *testing.T) {
	path := writeSpec(t, "spec.yaml", `
name: agent-service
importance: high
artifact:
  name: agent-service-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: agent
            imageUri: containous/whoami:latest
            port: 8080
            primary: true
            environmentVars:
              - name: LLM_API_KEY
                required: true
`)

	payload, err := ReadSpecFile(path)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"name": "agent-service",
		"importance": "high",
		"artifact": {
			"name": "agent-service-artifact",
			"type": "service",
			"spec": {
				"containerGroups": [{
					"name": "default",
					"containers": [{
						"name": "agent",
						"imageUri": "containous/whoami:latest",
						"port": 8080,
						"primary": true,
						"environmentVars": [{"name": "LLM_API_KEY", "required": true}]
					}]
				}]
			}
		}
	}`, string(payload))
}

func TestReadSpecFile_InvalidBothFormats(t *testing.T) {
	path := writeSpec(t, "spec.yaml", `{"name": [unclosed`)

	_, err := ReadSpecFile(path)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "neither valid JSON nor valid YAML")
}

func TestReadSpecFile_FileNotFound(t *testing.T) {
	_, err := ReadSpecFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "file not found")
}

func TestReadSpecFile_ValidateReplicaAutoscalingConflict(t *testing.T) {
	path := writeSpec(t, "spec.yaml", `
name: wl
artifactId: abc
runtime:
  containerGroups:
    - replicaCount: 3
      autoscaling:
        enabled: true
        policies: []
`)

	payload, err := ReadSpecFile(path)
	require.NoError(t, err)

	err = ValidateWorkloadCreateRequest(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime.containerGroups[0]: replicaCount and autoscaling.enabled=true are mutually exclusive")
}
