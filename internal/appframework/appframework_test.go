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

package appframework

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- afCommand construction ---

func TestAfCommand_BasicForm(t *testing.T) {
	cmd := afCommand("describe-framework", "-f", "/fw", "-t", ".")

	assert.Equal(t, "uvx", filepath.Base(cmd.Path))

	args := cmd.Args[1:] // cmd.Args[0] is the binary path

	assert.Equal(t, "--from", args[0])
	assert.Equal(t, AFSourcePath, args[1])
	assert.Equal(t, "dr-app-framework", args[2])
	assert.Equal(t, "describe-framework", args[3])
	assert.Equal(t, "-f", args[4])
	assert.Equal(t, "/fw", args[5])
	assert.Equal(t, "-t", args[6])
	assert.Equal(t, ".", args[7])
}

func TestAfCommand_AddModule(t *testing.T) {
	cmd := afCommand("add-module", "-m", "core.agent", "-l", "core.agent.1", "-f", "/fw", "-t", ".")

	args := cmd.Args[1:]

	assert.Equal(t, "add-module", args[3])
	assert.Contains(t, args, "-m")
	assert.Contains(t, args, "core.agent")
	assert.Contains(t, args, "-l")
	assert.Contains(t, args, "core.agent.1")
}

// --- formatDataValue ---

func TestFormatDataValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{name: "string", value: "hello", expected: "hello"},
		{name: "empty string", value: "", expected: ""},
		{name: "bool true", value: true, expected: "true"},
		{name: "bool false", value: false, expected: "false"},
		{name: "int", value: 42, expected: "42"},
		{name: "int8", value: int8(127), expected: "127"},
		{name: "int32", value: int32(100), expected: "100"},
		{name: "int64", value: int64(9223372036854775807), expected: "9223372036854775807"},
		{name: "float32", value: float32(3.14), expected: "3.14"},
		{name: "float64", value: 2.718, expected: "2.718"},
		{name: "nil", value: nil, expected: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatDataValue(tt.value))
		})
	}
}

func TestFormatYAMLList(t *testing.T) {
	result := formatYAMLList([]interface{}{"a", "b", "c"})
	assert.Equal(t, "[a, b, c]", result)
}

func TestFormatYAMLList_Empty(t *testing.T) {
	result := formatYAMLList([]interface{}{})
	assert.Equal(t, "[]", result)
}

func TestFormatYAMLMap(t *testing.T) {
	result := formatYAMLMap(map[string]interface{}{"key": "value"})
	assert.Equal(t, "{key: value}", result)
}

// --- JSON parsing: describe-framework response ---

func TestDescribeFramework_ParseModules(t *testing.T) {
	payload := `{
		"path": "/fw",
		"registries": {
			"core": {"uri": "https://example.com/registry.yml", "alias": "core",
			         "fetched_at": "2026-01-01", "owner": "datarobot", "description": ""}
		},
		"modules": {
			"core.agent": {
				"name": "agent",
				"registry": "core",
				"display_name": "Agent",
				"description": "An LLM agent component",
				"tags": ["agent"],
				"questions": [
					{
						"name": "template_name",
						"display_name": "Template Name",
						"help": "Unique name for this agent",
						"default": null,
						"type": "str",
						"choices": null
					}
				]
			}
		}
	}`

	var resp describeFrameworkResponse

	require.NoError(t, json.Unmarshal([]byte(payload), &resp))
	assert.Len(t, resp.Modules, 1)

	m := resp.Modules["core.agent"]

	assert.Equal(t, "agent", m.Name)
	assert.Equal(t, "core", m.Registry)
	assert.Equal(t, "Agent", m.DisplayName)
	assert.Equal(t, "An LLM agent component", m.Description)
	assert.Equal(t, []string{"agent"}, m.Tags)
	require.Len(t, m.Questions, 1)
	assert.Equal(t, "template_name", m.Questions[0].Name)
	assert.Equal(t, "str", m.Questions[0].Type)
}

func TestDescribeFramework_ParseAgentGuidanceAndDeps(t *testing.T) {
	repeatable := "agent_app_name"
	payload := `{
		"path": "/fw",
		"registries": {
			"core": {"uri": "https://example.com/registry.yml", "alias": "core",
			         "fetched_at": "2026-01-01", "owner": "datarobot", "description": ""}
		},
		"modules": {
			"core.agent": {
				"name": "agent",
				"registry": "core",
				"display_name": "Agent",
				"description": "Deploys a DataRobot agent application.",
				"tags": [],
				"repeatable": "agent_app_name",
				"dependencies": ["base", "llm", "datarobot_mcp"],
				"agent_guidance": {
					"summary": "Core component for every agent project."
				},
				"questions": [
					{
						"name": "agent_app_name",
						"display_name": "agent_app_name",
						"help": "The name/folder of your Agent Deployment.",
						"default": "agent",
						"type": "str",
						"choices": null,
						"agent_guidance": {
							"ask_user": false,
							"reason": "Derive from working directory name."
						}
					},
					{
						"name": "agent_template_framework",
						"display_name": "agent_template_framework",
						"help": "Choose the agentic framework template to start with:",
						"default": null,
						"type": "str",
						"choices": ["base", "crewai", "langgraph", "llamaindex", "nat"],
						"agent_guidance": {
							"ask_user": true,
							"reason": "Framework choice fundamentally changes the generated code structure."
						}
					}
				]
			}
		}
	}`

	var resp describeFrameworkResponse

	require.NoError(t, json.Unmarshal([]byte(payload), &resp))

	m := resp.Modules["core.agent"]

	assert.Equal(t, &repeatable, m.Repeatable)
	assert.Equal(t, []string{"base", "llm", "datarobot_mcp"}, m.Dependencies)
	require.NotNil(t, m.AgentGuidance)
	assert.Equal(t, "Core component for every agent project.", m.AgentGuidance.Summary)
	require.Len(t, m.Questions, 2)

	appNameQ := m.Questions[0]

	require.NotNil(t, appNameQ.AgentGuidance)
	assert.False(t, appNameQ.AgentGuidance.AskUser)
	assert.Equal(t, "Derive from working directory name.", appNameQ.AgentGuidance.Reason)

	frameworkQ := m.Questions[1]

	require.NotNil(t, frameworkQ.AgentGuidance)
	assert.True(t, frameworkQ.AgentGuidance.AskUser)
}

// --- JSON parsing: add-module response ---

func TestAddModuleResponse_Parse(t *testing.T) {
	payload := `{"added_modules": {"core.agent": "core.agent.1"}}`

	var resp AddModuleResponse

	require.NoError(t, json.Unmarshal([]byte(payload), &resp))
	assert.Equal(t, "core.agent.1", resp.AddedModules["core.agent"])
}

// --- JSON parsing: describe (instance state) response ---

func TestListInstalled_ParseInstanceState(t *testing.T) {
	payload := `{
		"labels": {
			"core.agent.1": "core.agent"
		},
		"dependencies": {
			"core.agent.1": {}
		},
		"answers": {
			"core.agent.1": {
				"core.agent.template_name": "my-agent"
			}
		},
		"module_refs": {}
	}`

	var resp instanceStateResponse

	require.NoError(t, json.Unmarshal([]byte(payload), &resp))
	assert.Equal(t, "core.agent", resp.Labels["core.agent.1"])
	assert.Equal(t, "my-agent", resp.Answers["core.agent.1"]["core.agent.template_name"])
}
