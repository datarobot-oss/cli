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

package plugin

import "sync"

// PluginScripts maps platforms to their script paths within the plugin package
type PluginScripts struct {
	Posix   string `json:"posix,omitempty"`   // Script for Linux/macOS (e.g., "scripts/plugin.sh")
	Windows string `json:"windows,omitempty"` // Script for Windows (e.g., "scripts/plugin.ps1")
}

// BasicPluginManifest contains the core fields that all plugin manifests must have.
type BasicPluginManifest struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	Description    string `json:"description,omitempty"`
	Authentication bool   `json:"authentication,omitempty"`
}

// PluginManifest represents the full JSON manifest returned by managed plugins.
// Embeds BasicPluginManifest and adds additional fields for managed plugins.
type PluginManifest struct {
	BasicPluginManifest
	Scripts       *PluginScripts `json:"scripts,omitempty"`       // Platform-specific script paths
	MinCLIVersion string         `json:"minCLIVersion,omitempty"` // Minimum CLI version required
}

// IndexVersion represents a specific version in the plugin index
type IndexVersion struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
}

// IndexPlugin represents a plugin entry in the remote index
type IndexPlugin struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	Versions    []IndexVersion `json:"versions"`
}

// PluginIndex represents the remote plugin index structure
type PluginIndex struct {
	Version string                 `json:"version"`
	Plugins map[string]IndexPlugin `json:"plugins"`
}

// InstalledPlugin represents metadata about an installed remote plugin
type InstalledPlugin struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Source      string `json:"source"`      // URL or local path where it was installed from
	InstalledAt string `json:"installedAt"` // RFC3339 timestamp
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
