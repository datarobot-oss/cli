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

import "errors"

// ErrCorruptInstall marks a managed plugin installation that is broken in a
// way a reinstall repairs: an unparsable or incomplete manifest, or a missing
// platform script. Transient failures (e.g. permission errors) are NOT
// wrapped with it, so callers can distinguish "reinstall" from "report".
var ErrCorruptInstall = errors.New("corrupt managed plugin installation")

// LoadManagedPlugin resolves a single managed plugin by name without running
// full discovery (no PATH scan, no manifest subprocesses for other plugins).
// Managed plugin directories are searched in priority order. Returns
// (nil, nil) when the plugin is not installed, so callers can distinguish
// "not installed" from a broken installation.
func LoadManagedPlugin(name string) (*DiscoveredPlugin, error) {
	if err := validatePluginName(name); err != nil {
		return nil, err
	}

	dirs, err := ManagedPluginsDirs()
	if err != nil {
		return nil, err
	}

	for _, dir := range dirs {
		p, _, err := loadManagedPlugin(dir, name, map[string]bool{})
		if err != nil {
			return nil, err
		}

		if p != nil {
			return p, nil
		}
	}

	return nil, nil
}
