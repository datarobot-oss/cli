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

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDrStartEvent(t *testing.T) {
	event := NewDrStartEvent("base")

	assert.Equal(t, "dr start execute", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
}

func TestNewDrRunEvent(t *testing.T) {
	event := NewDrRunEvent("base", "dev")

	assert.Equal(t, "dr run execute", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
	assert.Equal(t, "dev", event.EventProperties["task_name"])
}

func TestNewDrTaskEvent(t *testing.T) {
	event := NewDrTaskEvent("base", "build")

	assert.Equal(t, "dr task execute", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
	assert.Equal(t, "build", event.EventProperties["task_name"])
}

func TestNewDrPluginUpdateEvent(t *testing.T) {
	event := NewDrPluginUpdateEvent("my-plugin", "v1.0.0")

	assert.Equal(t, "dr plugin update", event.EventType)
	assert.Equal(t, "my-plugin", event.EventProperties["plugin_name"])
	assert.Equal(t, "v1.0.0", event.EventProperties["version_number"])
}

func TestNewDrTemplateSetupEvent(t *testing.T) {
	event := NewDrTemplateSetupEvent("base")

	assert.Equal(t, "dr template setup", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
}

func TestNewDrComponentAddEvent(t *testing.T) {
	event := NewDrComponentAddEvent("my-component", "base")

	assert.Equal(t, "dr component add", event.EventType)
	assert.Equal(t, "my-component", event.EventProperties["component_name"])
	assert.Equal(t, "base", event.EventProperties["template_name"])
}

func TestNewDrComponentUpdateEvent(t *testing.T) {
	event := NewDrComponentUpdateEvent("my-component", "base")

	assert.Equal(t, "dr component update", event.EventType)
	assert.Equal(t, "my-component", event.EventProperties["component_name"])
	assert.Equal(t, "base", event.EventProperties["template_name"])
}

func TestNewDrPluginExecuteEvent(t *testing.T) {
	event := NewDrPluginExecuteEvent("my-plugin", "v1.0.0")

	assert.Equal(t, "dr plugin execute", event.EventType)
	assert.Equal(t, "my-plugin", event.EventProperties["plugin_name"])
	assert.Equal(t, "v1.0.0", event.EventProperties["plugin_version"])
}

func TestNewDrPluginInstallEvent(t *testing.T) {
	event := NewDrPluginInstallEvent("my-plugin", "v1.0.0")

	assert.Equal(t, "dr plugin install", event.EventType)
	assert.Equal(t, "my-plugin", event.EventProperties["plugin_name"])
	assert.Equal(t, "v1.0.0", event.EventProperties["plugin_version"])
}

func TestNewDrPluginUninstallEvent(t *testing.T) {
	event := NewDrPluginUninstallEvent("my-plugin", "v1.0.0")

	assert.Equal(t, "dr plugin uninstall", event.EventType)
	assert.Equal(t, "my-plugin", event.EventProperties["plugin_name"])
	assert.Equal(t, "v1.0.0", event.EventProperties["plugin_version"])
}

func TestNewDrAuthSetURLEvent(t *testing.T) {
	event := NewDrAuthSetURLEvent("https://app.datarobot.com")

	assert.Equal(t, "dr auth set-url", event.EventType)
	assert.Equal(t, "https://app.datarobot.com", event.EventProperties["url"])
}

func TestNewDrDotenvUpdateEvent(t *testing.T) {
	event := NewDrDotenvUpdateEvent("base")

	assert.Equal(t, "dr dotenv update", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
}

func TestNewDrDotenvSetupEvent(t *testing.T) {
	event := NewDrDotenvSetupEvent("base")

	assert.Equal(t, "dr dotenv setup", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
}

func TestNewDrDotenvValidateEvent(t *testing.T) {
	event := NewDrDotenvValidateEvent("base")

	assert.Equal(t, "dr dotenv validate", event.EventType)
	assert.Equal(t, "base", event.EventProperties["template_name"])
}
