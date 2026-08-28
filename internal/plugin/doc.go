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

// Package plugin implements the core plugin system for the DataRobot CLI.
//
// Plugins are external executables named dr-* that extend the CLI with
// additional top-level commands. This package provides:
//
//   - Discovery: scans managed plugin directories, the project-local
//     .dr/plugins/ directory, and PATH for plugin executables, querying
//     each for a JSON manifest via the --dr-plugin-manifest flag.
//   - Execution: runs a discovered plugin as a subprocess, forwarding
//     arguments verbatim, setting plugin-specific environment variables,
//     and optionally enforcing DataRobot authentication beforehand.
//   - Remote registry: fetches and parses the plugin registry, resolves
//     semantic-version constraints, and downloads/installs plugins from
//     the registry, local archives, or HTTP URLs with SHA256 verification.
//   - Dependency checking: reads a plugin's versions.yaml and verifies
//     that required external tools are present before installation and
//     before each invocation.
//   - Update checking: periodically checks the registry for newer versions
//     of managed plugins, with a configurable cooldown to avoid redundant
//     network requests.
//
// See the plugin system documentation at docs/development/plugins.md for
// the manifest protocol, environment variables, and packaging workflow.
package plugin
