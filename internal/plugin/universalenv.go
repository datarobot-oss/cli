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

import (
	"github.com/datarobot/cli/internal/config/viperx"
)

// universalFlag describes a root-level CLI flag that is forwarded to plugins
// as a DATAROBOT_CLI_<EnvSuffix> environment variable.
//
// Bool flags emit "<prefix>=1" when true and are omitted when false.
// String flags emit "<prefix>=<value>" when non-empty and are omitted otherwise.
//
// To forward a new flag in the future:
//  1. Add an entry here.
//  2. The flag must already be bound to viper via viperx.BindPFlag in cmd/root.go.
//  3. Update docs/development/plugins.md to document the new variable.
type universalFlag struct {
	// ViperKey is the viper config key the flag is bound to (e.g. "debug").
	ViperKey string

	// EnvSuffix is appended to "DATAROBOT_CLI_" to form the env var name
	// (e.g. "DEBUG" → "DATAROBOT_CLI_DEBUG").
	EnvSuffix string

	// IsBool indicates the flag is boolean. True bool flags emit "=1";
	// false bool flags are omitted entirely.
	IsBool bool
}

// universalFlags is the canonical list of root CLI flags forwarded to plugins.
// Only flags placed BEFORE the plugin name on the command line are consumed by
// core; flags after the plugin name are never parsed by core (kubectl/helm model).
var universalFlags = []universalFlag{
	{ViperKey: "debug", EnvSuffix: "DEBUG", IsBool: true},
	{ViperKey: "disable-telemetry", EnvSuffix: "DISABLE_TELEMETRY", IsBool: true},
}

const universalEnvPrefix = "DATAROBOT_CLI_"

// universalFlagEnv returns a slice of "KEY=VALUE" strings for every universal
// flag that is currently set. These are appended to the plugin's environment so
// that plugins can optionally honour the same flags as the core CLI.
func universalFlagEnv() []string {
	var env []string

	for _, f := range universalFlags {
		if f.IsBool {
			if viperx.GetBool(f.ViperKey) {
				env = append(env, universalEnvPrefix+f.EnvSuffix+"=1")
			}

			continue
		}

		if val := viperx.GetString(f.ViperKey); val != "" {
			env = append(env, universalEnvPrefix+f.EnvSuffix+"="+val)
		}
	}

	return env
}
