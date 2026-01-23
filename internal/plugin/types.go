// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package plugin

import "sync"

// PluginManifest represents the JSON manifest returned by plugins
type PluginManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// DiscoveredPlugin pairs a manifest with its executable path
type DiscoveredPlugin struct {
	Manifest   PluginManifest
	Executable string // Full path to executable
}

// PluginRegistry holds discovered plugins with lazy initialization
type PluginRegistry struct {
	plugins []DiscoveredPlugin
	once    sync.Once
	err     error
}
