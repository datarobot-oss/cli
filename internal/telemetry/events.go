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

import "github.com/amplitude/analytics-go/amplitude/types"

// This file defines typed constructors for all 14 CLI telemetry events.
// Each function returns an amplitude.Event with the correct EventType
// and expected property keys. Call-site wiring happens in PR 2.

// NewDrStartEvent creates a "dr start execute" event.
func NewDrStartEvent(templateName string) types.Event {
	return types.Event{
		EventType: "dr start execute",
		EventProperties: map[string]any{
			"template_name": templateName,
		},
	}
}

// NewDrRunEvent creates a "dr run execute" event.
func NewDrRunEvent(templateName, taskName string) types.Event {
	return types.Event{
		EventType: "dr run execute",
		EventProperties: map[string]any{
			"template_name": templateName,
			"task_name":     taskName,
		},
	}
}

// NewDrTaskEvent creates a "dr task execute" event.
func NewDrTaskEvent(templateName, taskName string) types.Event {
	return types.Event{
		EventType: "dr task execute",
		EventProperties: map[string]any{
			"template_name": templateName,
			"task_name":     taskName,
		},
	}
}

// NewDrPluginUpdateEvent creates a "dr plugin update" event.
func NewDrPluginUpdateEvent(pluginName, versionNumber string) types.Event {
	return types.Event{
		EventType: "dr plugin update",
		EventProperties: map[string]any{
			"plugin_name":    pluginName,
			"version_number": versionNumber,
		},
	}
}

// NewDrTemplateSetupEvent creates a "dr template setup" event.
func NewDrTemplateSetupEvent(templateName string) types.Event {
	return types.Event{
		EventType: "dr template setup",
		EventProperties: map[string]any{
			"template_name": templateName,
		},
	}
}

// NewDrComponentAddEvent creates a "dr component add" event.
func NewDrComponentAddEvent(componentName, templateName string) types.Event {
	return types.Event{
		EventType: "dr component add",
		EventProperties: map[string]any{
			"component_name": componentName,
			"template_name":  templateName,
		},
	}
}

// NewDrComponentUpdateEvent creates a "dr component update" event.
func NewDrComponentUpdateEvent(componentName, templateName string) types.Event {
	return types.Event{
		EventType: "dr component update",
		EventProperties: map[string]any{
			"component_name": componentName,
			"template_name":  templateName,
		},
	}
}

// NewDrPluginExecuteEvent creates a "dr plugin execute" event.
func NewDrPluginExecuteEvent(pluginName, pluginVersion string) types.Event {
	return types.Event{
		EventType: "dr plugin execute",
		EventProperties: map[string]any{
			"plugin_name":    pluginName,
			"plugin_version": pluginVersion,
		},
	}
}

// NewDrPluginInstallEvent creates a "dr plugin install" event.
func NewDrPluginInstallEvent(pluginName, pluginVersion string) types.Event {
	return types.Event{
		EventType: "dr plugin install",
		EventProperties: map[string]any{
			"plugin_name":    pluginName,
			"plugin_version": pluginVersion,
		},
	}
}

// NewDrPluginUninstallEvent creates a "dr plugin uninstall" event.
func NewDrPluginUninstallEvent(pluginName, pluginVersion string) types.Event {
	return types.Event{
		EventType: "dr plugin uninstall",
		EventProperties: map[string]any{
			"plugin_name":    pluginName,
			"plugin_version": pluginVersion,
		},
	}
}

// NewDrAuthSetURLEvent creates a "dr auth set-url" event.
func NewDrAuthSetURLEvent(url string) types.Event {
	return types.Event{
		EventType: "dr auth set-url",
		EventProperties: map[string]any{
			"url": url,
		},
	}
}

// NewDrDotenvUpdateEvent creates a "dr dotenv update" event.
func NewDrDotenvUpdateEvent(templateName string) types.Event {
	return types.Event{
		EventType: "dr dotenv update",
		EventProperties: map[string]any{
			"template_name": templateName,
		},
	}
}

// NewDrDotenvSetupEvent creates a "dr dotenv setup" event.
func NewDrDotenvSetupEvent(templateName string) types.Event {
	return types.Event{
		EventType: "dr dotenv setup",
		EventProperties: map[string]any{
			"template_name": templateName,
		},
	}
}

// NewDrDotenvValidateEvent creates a "dr dotenv validate" event.
func NewDrDotenvValidateEvent(templateName string) types.Event {
	return types.Event{
		EventType: "dr dotenv validate",
		EventProperties: map[string]any{
			"template_name": templateName,
		},
	}
}
